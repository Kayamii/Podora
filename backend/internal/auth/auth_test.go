package auth

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/store"
)

func newService(t *testing.T) *Service {
	t.Helper()
	PasswordCost = bcrypt.MinCost
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

const pw = "a long enough password"

func TestLastAdminIsProtected(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	admin, err := s.CreateUser(ctx, NewUser{Username: "root", Password: pw, Role: rbac.Admin})
	if err != nil {
		t.Fatal(err)
	}
	viewer := rbac.Viewer
	if _, err := s.UpdateUser(ctx, admin.ID, UserUpdate{Role: &viewer}); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demote last admin: err = %v, want ErrLastAdmin", err)
	}
	if err := s.DeleteUser(ctx, admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("delete last admin: err = %v, want ErrLastAdmin", err)
	}
	// With a second admin, the first may step down.
	if _, err := s.CreateUser(ctx, NewUser{Username: "root2", Password: pw, Role: rbac.Admin}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateUser(ctx, admin.ID, UserUpdate{Role: &viewer}); err != nil {
		t.Errorf("demote with another admin: %v", err)
	}
}

func TestAdminsAreNeverNamespaceLimited(t *testing.T) {
	s := newService(t)
	u, err := s.CreateUser(context.Background(), NewUser{Username: "boss", Password: pw, Role: rbac.Admin, Namespaces: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Namespaces) != 0 {
		t.Errorf("admin namespaces = %v, want none (all)", u.Namespaces)
	}
}

func TestDisabledUsersCannotSignIn(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	u, _ := s.CreateUser(ctx, NewUser{Username: "dev", Password: pw, Role: rbac.Developer})
	token, _, err := s.Login(ctx, "dev", pw, "dev")
	if err != nil {
		t.Fatal(err)
	}
	disabled := true
	if _, err := s.UpdateUser(ctx, u.ID, UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	// The existing session ends, and no new one starts.
	if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("session after disable: err = %v", err)
	}
	if _, _, err := s.Login(ctx, "dev", pw, "dev"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("login after disable: err = %v", err)
	}
}

func TestChangePasswordKeepsCurrentSession(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	u, _ := s.CreateUser(ctx, NewUser{Username: "dev", Password: pw, Role: rbac.Developer})
	here, _, _ := s.Login(ctx, "dev", pw, "a")
	elsewhere, _, _ := s.Login(ctx, "dev", pw, "b")

	if err := s.ChangePassword(ctx, u.ID, here, "wrong current", "another long password"); !errors.Is(err, ErrInvalid) {
		t.Errorf("wrong current password: err = %v", err)
	}
	if err := s.ChangePassword(ctx, u.ID, here, pw, "another long password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, here); err != nil {
		t.Errorf("current session ended: %v", err)
	}
	if _, err := s.Authenticate(ctx, elsewhere); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("other session still valid: %v", err)
	}
}

func TestSessionsAreStoredHashed(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	s.CreateUser(ctx, NewUser{Username: "dev", Password: pw, Role: rbac.Viewer})
	token, _, _ := s.Login(ctx, "dev", pw, "x")
	if _, err := s.store.Session(ctx, token); !errors.Is(err, store.ErrNotFound) {
		t.Error("the raw token was found in the database")
	}
}
