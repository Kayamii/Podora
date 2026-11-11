package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kayamii/Podora/backend/internal/auth"
	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/store"
)

const sessionCookie = "podora_session"

// auditLimit is how many audit entries the log page shows.
const auditLimit = 1000

// scope says which namespace a guarded route acts in.
type scope int

const (
	// scopePath checks the {namespace} in the URL.
	scopePath scope = iota
	// scopeQuery checks ?namespace= when given; lists without it are filtered
	// to what the user may see.
	scopeQuery
	// scopeCluster needs access to every namespace (nodes, cluster totals).
	scopeCluster
	// scopeRole checks the role only; the permission itself is cluster-wide
	// or the handler filters.
	scopeRole
)

type userKey struct{}

// userFrom returns the signed-in user of a guarded request.
func userFrom(r *http.Request) *auth.User {
	u, _ := r.Context().Value(userKey{}).(*auth.User)
	return u
}

func subjectFrom(r *http.Request) rbac.Subject {
	if u := userFrom(r); u != nil {
		return u.Subject()
	}
	return rbac.Subject{}
}

// guard admits a request only for a signed-in user holding perm in the
// route's namespace.
func (s *server) guard(perm rbac.Permission, sc scope, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.authenticate(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "sign in to continue"})
			return
		}
		sub := u.Subject()
		var ok bool
		switch sc {
		case scopePath:
			ok = sub.Can(perm, r.PathValue("namespace"))
		case scopeQuery:
			if ns := r.URL.Query().Get("namespace"); ns != "" {
				ok = sub.Can(perm, ns)
			} else {
				ok = sub.Has(perm)
			}
		case scopeCluster:
			ok = sub.Can(perm, "")
		case scopeRole:
			ok = sub.Has(perm)
		}
		if !ok {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "your account is not allowed to do this here"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	}
}

func (s *server) authenticate(r *http.Request) (*auth.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, auth.ErrUnauthenticated
	}
	return s.auth.Authenticate(r.Context(), c.Value)
}

// visible keeps the items the user may see.
func visible[T any](r *http.Request, items []T, namespace func(T) string) []T {
	sub := subjectFrom(r)
	if sub.AllNamespaces() {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if sub.InNamespace(namespace(it)) {
			out = append(out, it)
		}
	}
	return out
}

// --- Audit ----------------------------------------------------------------------

// Auditor stores audit entries.
type Auditor interface {
	AddAudit(ctx context.Context, e store.AuditEntry) error
	Audit(ctx context.Context, limit int) ([]store.AuditEntry, error)
}

type recorder struct {
	store Auditor
}

// record writes one audit entry for an attempted change, successful or not.
func (rc recorder) record(r *http.Request, action, namespace, target, detail string, err error) {
	e := store.AuditEntry{
		Username:  "anonymous",
		Action:    action,
		Namespace: namespace,
		Target:    target,
		Detail:    detail,
		Success:   err == nil,
		Remote:    clientIP(r),
	}
	if u := userFrom(r); u != nil {
		e.Username = u.Username
	}
	if err != nil {
		e.Error = err.Error()
	}
	slog.Info("audit", "user", e.Username, "action", action, "namespace", namespace, "target", target, "detail", detail, "success", e.Success, "error", e.Error)
	if rc.store == nil {
		return
	}
	// The change already happened; a failed audit write is logged, not returned.
	if werr := rc.store.AddAudit(context.WithoutCancel(r.Context()), e); werr != nil {
		slog.Error("writing audit entry", "error", werr)
	}
}

// clientIP prefers the address set by the bundled nginx (X-Real-IP), falling
// back to the connection's peer. It is informational only.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *server) auditLog(w http.ResponseWriter, r *http.Request) {
	limit := auditLimit
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < auditLimit {
		limit = v
	}
	entries, err := s.rec.store.Audit(r.Context(), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// --- Sign-in --------------------------------------------------------------------

type me struct {
	User          auth.User         `json:"user"`
	Permissions   []rbac.Permission `json:"permissions"`
	AllNamespaces bool              `json:"allNamespaces"`
}

func describeMe(u *auth.User) me {
	sub := u.Subject()
	return me{User: *u, Permissions: sub.Permissions(), AllNamespaces: sub.AllNamespaces()}
}

func (s *server) registerAuth(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/status", s.status)
	mux.HandleFunc("POST /api/v1/auth/setup", change(s.setup))
	mux.HandleFunc("POST /api/v1/auth/login", change(s.login))
	mux.HandleFunc("POST /api/v1/auth/logout", change(s.logout))
	mux.HandleFunc("GET /api/v1/auth/me", s.guard(rbac.Read, scopeRole, s.me))
	mux.HandleFunc("POST /api/v1/auth/password", change(s.guard(rbac.Read, scopeRole, s.changePassword)))

	mux.HandleFunc("GET /api/v1/users", s.guard(rbac.ManageUsers, scopeRole, s.users))
	mux.HandleFunc("POST /api/v1/users", change(s.guard(rbac.ManageUsers, scopeRole, s.createUser)))
	mux.HandleFunc("PATCH /api/v1/users/{id}", change(s.guard(rbac.ManageUsers, scopeRole, s.updateUser)))
	mux.HandleFunc("DELETE /api/v1/users/{id}", change(s.guard(rbac.ManageUsers, scopeRole, s.deleteUser)))
	mux.HandleFunc("GET /api/v1/audit", s.guard(rbac.ViewAudit, scopeRole, s.auditLog))
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	required, err := s.auth.SetupRequired(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"setupRequired": required})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Token is the one-time setup token, for setup only.
	Token string `json:"token"`
}

func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	token, u, err := s.auth.Setup(r.Context(), c.Token, c.Username, c.Password)
	if err != nil {
		s.rec.record(r, "auth.setup", "", c.Username, "", err)
		writeError(w, r, err)
		return
	}
	setSessionCookie(w, r, token)
	r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
	s.rec.record(r, "auth.setup", "", u.Username, "created the first administrator", nil)
	writeJSON(w, http.StatusOK, describeMe(u))
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	key := strings.ToLower(strings.TrimSpace(c.Username))
	token, u, err := s.auth.Login(r.Context(), c.Username, c.Password, key)
	if err != nil {
		s.rec.record(r, "auth.login", "", c.Username, "", err)
		writeError(w, r, err)
		return
	}
	setSessionCookie(w, r, token)
	r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
	s.rec.record(r, "auth.login", "", u.Username, "", nil)
	writeJSON(w, http.StatusOK, describeMe(u))
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if u, err := s.auth.Authenticate(r.Context(), c.Value); err == nil {
			r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
			s.rec.record(r, "auth.logout", "", u.Username, "", nil)
		}
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, describeMe(userFrom(r)))
}

type passwordChange struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req passwordChange
	if !decode(w, r, &req) {
		return
	}
	c, _ := r.Cookie(sessionCookie)
	err := s.auth.ChangePassword(r.Context(), userFrom(r).ID, c.Value, req.Current, req.New)
	s.rec.record(r, "user.password", "", userFrom(r).Username, "changed own password", err)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(auth.SessionLifetime.Seconds()),
		HttpOnly: true,
		// Lax keeps the cookie off cross-site POSTs, on top of the
		// X-Podora-Request check.
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

// --- Users ----------------------------------------------------------------------

func (s *server) users(w http.ResponseWriter, r *http.Request) {
	list, err := s.auth.Users(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	var n auth.NewUser
	if !decode(w, r, &n) {
		return
	}
	u, err := s.auth.CreateUser(r.Context(), n)
	s.rec.record(r, "user.create", "", n.Username, describeScope(n.Role, n.Namespaces), err)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return
	}
	var upd auth.UserUpdate
	if !decode(w, r, &upd) {
		return
	}
	// Changing your own role or disabling yourself is how admins lock
	// themselves out; another administrator has to do it.
	if self := userFrom(r); self.ID == id && (upd.Role != nil && *upd.Role != self.Role || upd.Disabled != nil && *upd.Disabled) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "you cannot change your own role or disable your own account"})
		return
	}
	u, err := s.auth.UpdateUser(r.Context(), id, upd)
	target := r.PathValue("id")
	if u != nil {
		target = u.Username
	}
	s.rec.record(r, "user.update", "", target, describeUpdate(upd), err)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return
	}
	if userFrom(r).ID == id {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "you cannot delete your own account"})
		return
	}
	target := r.PathValue("id")
	if users, err := s.auth.Users(r.Context()); err == nil {
		for _, u := range users {
			if u.ID == id {
				target = u.Username
			}
		}
	}
	err = s.auth.DeleteUser(r.Context(), id)
	s.rec.record(r, "user.delete", "", target, "", err)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func describeScope(role rbac.Role, namespaces []string) string {
	if len(namespaces) == 0 || role == rbac.Admin {
		return "role " + string(role) + ", all namespaces"
	}
	return "role " + string(role) + ", namespaces " + strings.Join(namespaces, ", ")
}

func describeUpdate(u auth.UserUpdate) string {
	parts := []string{}
	if u.Role != nil {
		parts = append(parts, "role "+string(*u.Role))
	}
	if u.Namespaces != nil {
		if len(*u.Namespaces) == 0 {
			parts = append(parts, "all namespaces")
		} else {
			parts = append(parts, "namespaces "+strings.Join(*u.Namespaces, ", "))
		}
	}
	if u.Disabled != nil {
		if *u.Disabled {
			parts = append(parts, "disabled")
		} else {
			parts = append(parts, "enabled")
		}
	}
	if u.Password != nil {
		parts = append(parts, "password reset")
	}
	return strings.Join(parts, "; ")
}

// writeError maps account and storage errors, then falls back to cluster errors.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := 0
	switch {
	case errors.Is(err, auth.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, auth.ErrBadCredentials), errors.Is(err, auth.ErrUnauthenticated):
		status = http.StatusUnauthorized
	case errors.Is(err, auth.ErrLocked):
		status = http.StatusTooManyRequests
	case errors.Is(err, auth.ErrBadSetupToken):
		status = http.StatusForbidden
	case errors.Is(err, auth.ErrSetupDone), errors.Is(err, auth.ErrLastAdmin):
		status = http.StatusConflict
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	}
	if status == 0 {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, status, errorResponse{Error: strings.TrimPrefix(err.Error(), "invalid request: ")})
}
