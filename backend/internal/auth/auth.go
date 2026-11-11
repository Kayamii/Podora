// Package auth signs users in and out and manages their accounts.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/store"
)

const (
	// SessionLifetime is the longest a session lasts, active or not.
	SessionLifetime = 7 * 24 * time.Hour
	// IdleTimeout signs out a session nobody has used for this long.
	IdleTimeout = 12 * time.Hour
	// touchEvery limits last-seen writes to one per session per interval.
	touchEvery = 5 * time.Minute

	minPasswordLength = 10
	maxPasswordLength = 72 // bcrypt ignores anything longer

	maxFailures  = 5
	lockDuration = time.Minute
)

var (
	// ErrInvalid marks a request the caller got wrong; the message says why.
	ErrInvalid = errors.New("invalid request")
	// ErrBadCredentials hides whether the username or the password was wrong.
	ErrBadCredentials  = errors.New("wrong username or password")
	ErrLocked          = errors.New("too many failed sign-in attempts; wait a minute and try again")
	ErrUnauthenticated = errors.New("not signed in")
	ErrSetupDone       = errors.New("Podora is already set up")
	ErrBadSetupToken   = errors.New("the setup token is wrong")
	// ErrLastAdmin protects against locking everyone out.
	ErrLastAdmin = errors.New("this is the last active administrator; add another one first")
)

// PasswordCost is the bcrypt work factor. Tests lower it to stay fast.
var PasswordCost = bcrypt.DefaultCost

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@-]{1,62}$`)

// User is an account as the API presents it: never with its password hash.
type User struct {
	ID         int64      `json:"id"`
	Username   string     `json:"username"`
	Role       rbac.Role  `json:"role"`
	Namespaces []string   `json:"namespaces"`
	Disabled   bool       `json:"disabled"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastLogin  *time.Time `json:"lastLogin"`
}

func (u User) Subject() rbac.Subject {
	return rbac.Subject{Username: u.Username, Role: u.Role, Namespaces: u.Namespaces}
}

func present(u *store.User) User {
	out := User{
		ID: u.ID, Username: u.Username, Role: rbac.Role(u.Role),
		Namespaces: u.Namespaces, Disabled: u.Disabled, CreatedAt: u.CreatedAt,
	}
	if !u.LastLogin.IsZero() {
		t := u.LastLogin
		out.LastLogin = &t
	}
	return out
}

// Service owns accounts and sessions.
type Service struct {
	store *store.Store

	mu         sync.Mutex
	setupToken string
	failures   map[string]*failure
}

type failure struct {
	count int
	until time.Time
}

func New(st *store.Store) *Service {
	return &Service{store: st, failures: map[string]*failure{}}
}

// PrepareSetup returns a one-time token for creating the first administrator,
// or "" when users already exist. A preset token (from configuration) is used
// as given; otherwise a random one is generated.
func (s *Service) PrepareSetup(ctx context.Context, preset string) (string, error) {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return "", nil
	}
	token := preset
	if token == "" {
		token = randomToken(18)
	}
	s.mu.Lock()
	s.setupToken = token
	s.mu.Unlock()
	return token, nil
}

// SetupRequired reports whether no account exists yet.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	n, err := s.store.CountUsers(ctx)
	return n == 0, err
}

// Setup creates the first administrator and signs them in.
func (s *Service) Setup(ctx context.Context, token, username, password string) (string, *User, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return "", nil, err
	}
	if !required {
		return "", nil, ErrSetupDone
	}
	s.mu.Lock()
	expected := s.setupToken
	s.mu.Unlock()
	if expected == "" || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(expected)) != 1 {
		return "", nil, ErrBadSetupToken
	}
	u, err := s.CreateUser(ctx, NewUser{Username: username, Password: password, Role: rbac.Admin})
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	s.setupToken = ""
	s.mu.Unlock()
	_ = s.store.RecordLogin(ctx, u.ID)
	session, err := s.startSession(ctx, u.ID)
	return session, u, err
}

// Login checks credentials and starts a session. key identifies the client
// for throttling (username and address).
func (s *Service) Login(ctx context.Context, username, password, key string) (string, *User, error) {
	if s.locked(key) {
		return "", nil, ErrLocked
	}
	u, err := s.store.UserByName(ctx, strings.TrimSpace(username))
	if errors.Is(err, store.ErrNotFound) {
		// Spend the same time as a real check, so timing does not reveal
		// which usernames exist.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		s.fail(key)
		return "", nil, ErrBadCredentials
	}
	if err != nil {
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil || u.Disabled {
		s.fail(key)
		return "", nil, ErrBadCredentials
	}
	s.clearFailures(key)
	_ = s.store.RecordLogin(ctx, u.ID)
	token, err := s.startSession(ctx, u.ID)
	if err != nil {
		return "", nil, err
	}
	p := present(u)
	return token, &p, nil
}

// Authenticate resolves a session token to its user.
func (s *Service) Authenticate(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrUnauthenticated
	}
	hash := hashToken(token)
	sess, err := s.store.Session(ctx, hash)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeen) > IdleTimeout {
		_ = s.store.DeleteSession(ctx, hash)
		return nil, ErrUnauthenticated
	}
	u, err := s.store.User(ctx, sess.UserID)
	if err != nil || u.Disabled {
		_ = s.store.DeleteSession(ctx, hash)
		return nil, ErrUnauthenticated
	}
	if now.Sub(sess.LastSeen) > touchEvery {
		_ = s.store.TouchSession(ctx, hash, now)
	}
	p := present(u)
	return &p, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, hashToken(token))
}

// ChangePassword lets a user replace their own password. Their other
// sessions are signed out; the current one stays.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentToken, oldPassword, newPassword string) error {
	u, err := s.store.User(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)) != nil {
		return fmt.Errorf("%w: the current password is wrong", ErrInvalid)
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return err
	}
	return s.store.DeleteUserSessions(ctx, userID, hashToken(currentToken))
}

// --- Account management (administrators) ---------------------------------------

type NewUser struct {
	Username   string    `json:"username"`
	Password   string    `json:"password"`
	Role       rbac.Role `json:"role"`
	Namespaces []string  `json:"namespaces"`
}

func (s *Service) Users(ctx context.Context) ([]User, error) {
	list, err := s.store.Users(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]User, len(list))
	for i := range list {
		out[i] = present(&list[i])
	}
	return out, nil
}

func (s *Service) CreateUser(ctx context.Context, n NewUser) (*User, error) {
	n.Username = strings.TrimSpace(n.Username)
	if !usernamePattern.MatchString(n.Username) {
		return nil, fmt.Errorf("%w: usernames are 2 to 63 characters: letters, digits and . _ @ -", ErrInvalid)
	}
	namespaces, err := validateScope(n.Role, n.Namespaces)
	if err != nil {
		return nil, err
	}
	hash, err := hashPassword(n.Password)
	if err != nil {
		return nil, err
	}
	u := &store.User{Username: n.Username, PasswordHash: hash, Role: string(n.Role), Namespaces: namespaces}
	if err := s.store.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, fmt.Errorf("%w: the username %q is taken", ErrInvalid, n.Username)
		}
		return nil, err
	}
	created, err := s.store.User(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	p := present(created)
	return &p, nil
}

// UserUpdate changes an account. Nil fields stay as they are.
type UserUpdate struct {
	Role       *rbac.Role `json:"role"`
	Namespaces *[]string  `json:"namespaces"`
	Disabled   *bool      `json:"disabled"`
	// Password, when set, replaces the password and signs the user out.
	Password *string `json:"password"`
}

func (s *Service) UpdateUser(ctx context.Context, id int64, upd UserUpdate) (*User, error) {
	u, err := s.store.User(ctx, id)
	if err != nil {
		return nil, err
	}
	wasActiveAdmin := u.Role == string(rbac.Admin) && !u.Disabled

	role := rbac.Role(u.Role)
	if upd.Role != nil {
		role = *upd.Role
	}
	namespaces := u.Namespaces
	if upd.Namespaces != nil {
		namespaces = *upd.Namespaces
	}
	if namespaces, err = validateScope(role, namespaces); err != nil {
		return nil, err
	}
	u.Role, u.Namespaces = string(role), namespaces
	if upd.Disabled != nil {
		u.Disabled = *upd.Disabled
	}
	signOut := u.Disabled
	if upd.Password != nil {
		if u.PasswordHash, err = hashPassword(*upd.Password); err != nil {
			return nil, err
		}
		signOut = true
	}

	stillActiveAdmin := u.Role == string(rbac.Admin) && !u.Disabled
	if wasActiveAdmin && !stillActiveAdmin {
		if err := s.ensureAnotherAdmin(ctx); err != nil {
			return nil, err
		}
	}
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	if signOut {
		_ = s.store.DeleteUserSessions(ctx, id, "")
	}
	p := present(u)
	return &p, nil
}

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	u, err := s.store.User(ctx, id)
	if err != nil {
		return err
	}
	if u.Role == string(rbac.Admin) && !u.Disabled {
		if err := s.ensureAnotherAdmin(ctx); err != nil {
			return err
		}
	}
	return s.store.DeleteUser(ctx, id)
}

func (s *Service) ensureAnotherAdmin(ctx context.Context) error {
	n, err := s.store.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// CleanUp removes expired sessions.
func (s *Service) CleanUp(ctx context.Context) error {
	return s.store.DeleteExpiredSessions(ctx, time.Now().Add(-IdleTimeout))
}

// --- Helpers --------------------------------------------------------------------

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateScope checks a role and namespace list. Administrators always have
// every namespace: a limited administrator could lift their own limit.
func validateScope(role rbac.Role, namespaces []string) ([]string, error) {
	if !role.Valid() {
		return nil, fmt.Errorf("%w: role must be viewer, developer, operator or admin", ErrInvalid)
	}
	if role == rbac.Admin {
		return []string{}, nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, ns := range namespaces {
		ns = strings.TrimSpace(ns)
		if ns == "" || seen[ns] {
			continue
		}
		if !namespacePattern.MatchString(ns) || len(ns) > 63 {
			return nil, fmt.Errorf("%w: %q is not a valid namespace name", ErrInvalid, ns)
		}
		seen[ns] = true
		out = append(out, ns)
	}
	return out, nil
}

func hashPassword(p string) (string, error) {
	if len(p) < minPasswordLength {
		return "", fmt.Errorf("%w: passwords need at least %d characters", ErrInvalid, minPasswordLength)
	}
	if len(p) > maxPasswordLength {
		return "", fmt.Errorf("%w: passwords can have at most %d characters", ErrInvalid, maxPasswordLength)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(p), PasswordCost)
	return string(h), err
}

// dummyHash is compared against when a username does not exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("podora-timing-equalizer"), bcrypt.DefaultCost)

func (s *Service) startSession(ctx context.Context, userID int64) (string, error) {
	token := randomToken(32)
	now := time.Now()
	err := s.store.CreateSession(ctx, store.Session{
		TokenHash: hashToken(token), UserID: userID,
		CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(SessionLifetime),
	})
	return token, err
}

func randomToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// hashToken stores sessions by hash, so a leaked database holds no usable token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) locked(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.failures[key]
	return f != nil && time.Now().Before(f.until)
}

func (s *Service) fail(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.failures[key]
	if f == nil {
		f = &failure{}
		s.failures[key] = f
	}
	f.count++
	if f.count >= maxFailures {
		f.until = time.Now().Add(lockDuration)
		f.count = 0
	}
	// Keep the map small: forget stale entries.
	if len(s.failures) > 10000 {
		for k, v := range s.failures {
			if time.Now().After(v.until) {
				delete(s.failures, k)
			}
		}
	}
}

func (s *Service) clearFailures(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, key)
}
