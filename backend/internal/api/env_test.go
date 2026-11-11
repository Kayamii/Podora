package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kayamii/Podora/backend/internal/auth"
	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/store"
)

func init() {
	auth.PasswordCost = bcrypt.MinCost
}

const testPassword = "correct horse battery"

// testEnv is a router backed by a real in-memory account store, with one
// signed-in user.
type testEnv struct {
	t       *testing.T
	handler http.Handler
	store   *store.Store
	auth    *auth.Service
	cookie  *http.Cookie
}

// newEnv builds a router and signs in a user with the given role. With no
// namespaces the user sees every namespace.
func newEnv(t *testing.T, cluster ClusterReader, hm HelmManager, role rbac.Role, namespaces ...string) *testEnv {
	t.Helper()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := auth.New(st)
	if _, err := svc.CreateUser(context.Background(), auth.NewUser{
		Username: "tester", Password: testPassword, Role: role, Namespaces: namespaces,
	}); err != nil {
		t.Fatal(err)
	}
	e := &testEnv{t: t, store: st, auth: svc, handler: NewRouter(Deps{Cluster: cluster, Helm: hm, Auth: svc, Audit: st})}
	rec := e.do(http.MethodPost, "/api/v1/auth/login", "application/json", strings.NewReader(`{"username":"tester","password":"`+testPassword+`"}`), true, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, body = %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			e.cookie = c
		}
	}
	if e.cookie == nil {
		t.Fatal("login set no session cookie")
	}
	return e
}

// do sends a request; withHeader adds the change header, signedIn the cookie.
func (e *testEnv) do(method, path, contentType string, body io.Reader, withHeader, signedIn bool) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if withHeader {
		req.Header.Set(changeHeader, "1")
	}
	if signedIn && e.cookie != nil {
		req.AddCookie(e.cookie)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) get(path string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodGet, path, "", nil, false, true)
}

func (e *testEnv) send(method, path, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(method, path, "application/json", strings.NewReader(body), true, true)
}
