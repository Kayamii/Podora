package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Kayamii/Podora/backend/internal/auth"
	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/resources"
	"github.com/Kayamii/Podora/backend/internal/store"
)

// twoNamespaces has pods in "demo" and "other".
type twoNamespaces struct{ fakeCluster }

func (twoNamespaces) Pods(context.Context, string) ([]resources.PodSummary, error) {
	return []resources.PodSummary{{Name: "a", Namespace: "demo"}, {Name: "b", Namespace: "other"}}, nil
}

func TestEverythingNeedsSignIn(t *testing.T) {
	e := newEnv(t, fakeCluster{}, &fakeHelm{}, rbac.Admin)
	for _, path := range []string{"/api/v1/pods", "/api/v1/overview", "/api/v1/helm/releases", "/api/v1/users", "/api/v1/auth/me"} {
		if rec := e.do(http.MethodGet, path, "", nil, false, false); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without session: status = %d, want 401", path, rec.Code)
		}
	}
	// Health stays public, for probes.
	if rec := e.do(http.MethodGet, "/healthz", "", nil, false, false); rec.Code != http.StatusOK {
		t.Errorf("healthz: status = %d", rec.Code)
	}
}

func TestSetupFlow(t *testing.T) {
	st, _ := store.Open("")
	defer st.Close()
	svc := auth.New(st)
	token, err := svc.PrepareSetup(context.Background(), "")
	if err != nil || token == "" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
	e := &testEnv{t: t, store: st, auth: svc, handler: NewRouter(Deps{Cluster: fakeCluster{}, Auth: svc, Audit: st})}

	if rec := e.get("/api/v1/auth/status"); !strings.Contains(rec.Body.String(), `"setupRequired":true`) {
		t.Fatalf("status = %s", rec.Body)
	}
	body := `{"token":"%s","username":"admin","password":"` + testPassword + `"}`
	if rec := e.send(http.MethodPost, "/api/v1/auth/setup", strings.Replace(body, "%s", "wrong", 1)); rec.Code != http.StatusForbidden {
		t.Errorf("wrong token: status = %d, want 403", rec.Code)
	}
	rec := e.send(http.MethodPost, "/api/v1/auth/setup", strings.Replace(body, "%s", token, 1))
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("setup: status = %d, body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Errorf("setup user = %s", rec.Body)
	}
	if rec := e.send(http.MethodPost, "/api/v1/auth/setup", strings.Replace(body, "%s", token, 1)); rec.Code != http.StatusConflict {
		t.Errorf("second setup: status = %d, want 409", rec.Code)
	}
}

func TestLoginThrottling(t *testing.T) {
	e := newEnv(t, fakeCluster{}, nil, rbac.Viewer)
	bad := `{"username":"tester","password":"nope-nope-nope"}`
	for i := 0; i < 5; i++ {
		if rec := e.do(http.MethodPost, "/api/v1/auth/login", "application/json", strings.NewReader(bad), true, false); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	// Locked now, even with the right password.
	good := `{"username":"tester","password":"` + testPassword + `"}`
	if rec := e.do(http.MethodPost, "/api/v1/auth/login", "application/json", strings.NewReader(good), true, false); rec.Code != http.StatusTooManyRequests {
		t.Errorf("after 5 failures: status = %d, want 429", rec.Code)
	}
}

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role                rbac.Role
		del, restart, scale int
	}{
		{rbac.Viewer, 403, 403, 403},
		{rbac.Developer, 204, 204, 403},
		{rbac.Operator, 204, 204, 204},
		{rbac.Admin, 204, 204, 204},
	}
	for _, tc := range cases {
		got := &changes{scaledTo: -1}
		e := newEnv(t, fakeCluster{changes: got}, nil, tc.role)
		if rec := e.send(http.MethodDelete, "/api/v1/namespaces/demo/pods/web-1", ""); rec.Code != tc.del {
			t.Errorf("%s delete: status = %d, want %d", tc.role, rec.Code, tc.del)
		}
		if rec := e.send(http.MethodPost, "/api/v1/namespaces/demo/workloads/deployments/web/restart", ""); rec.Code != tc.restart {
			t.Errorf("%s restart: status = %d, want %d", tc.role, rec.Code, tc.restart)
		}
		if rec := e.send(http.MethodPost, "/api/v1/namespaces/demo/workloads/deployments/web/scale", `{"replicas":2}`); rec.Code != tc.scale {
			t.Errorf("%s scale: status = %d, want %d", tc.role, rec.Code, tc.scale)
		}
		// A refused request must never reach the cluster.
		if tc.scale == 403 && got.scaledTo != -1 {
			t.Errorf("%s: refused scale reached the cluster", tc.role)
		}
		if tc.del == 403 && got.deleted != "" {
			t.Errorf("%s: refused delete reached the cluster", tc.role)
		}
	}
}

func TestNamespaceLimits(t *testing.T) {
	e := newEnv(t, twoNamespaces{fakeCluster{changes: &changes{}}}, &fakeHelm{}, rbac.Operator, "demo")

	var pods []resources.PodSummary
	if err := json.Unmarshal(e.get("/api/v1/pods").Body.Bytes(), &pods); err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].Namespace != "demo" {
		t.Errorf("visible pods = %+v, want only demo", pods)
	}
	for path, want := range map[string]int{
		"/api/v1/pods?namespace=other":    403,
		"/api/v1/namespaces/other/pods/b": 403,
		"/api/v1/namespaces/demo/pods/a":  200,
		"/api/v1/overview":                403, // cluster-wide
		"/api/v1/metrics/history":         403,
	} {
		if rec := e.get(path); rec.Code != want {
			t.Errorf("GET %s: status = %d, want %d", path, rec.Code, want)
		}
	}
	if rec := e.send(http.MethodDelete, "/api/v1/namespaces/other/pods/b", ""); rec.Code != 403 {
		t.Errorf("delete in other namespace: status = %d, want 403", rec.Code)
	}
	if rec := e.send(http.MethodDelete, "/api/v1/namespaces/demo/pods/a", ""); rec.Code != 204 {
		t.Errorf("delete in own namespace: status = %d, want 204", rec.Code)
	}
	// Helm needs every namespace, even for an operator.
	if rec := e.send(http.MethodPost, "/api/v1/helm/releases", `{"chartId":"abc","name":"x","namespace":"demo"}`); rec.Code != 403 {
		t.Errorf("scoped helm install: status = %d, want 403", rec.Code)
	}
}

func TestChangesAreAudited(t *testing.T) {
	e := newEnv(t, fakeCluster{changes: &changes{}}, nil, rbac.Developer)
	e.send(http.MethodDelete, "/api/v1/namespaces/demo/pods/web-1", "")
	entries, err := e.store.Audit(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range entries {
		if a.Action == "pod.delete" {
			found = true
			if a.Username != "tester" || a.Namespace != "demo" || a.Target != "Pod/web-1" || !a.Success {
				t.Errorf("entry = %+v", a)
			}
		}
	}
	if !found {
		t.Errorf("no pod.delete entry in %+v", entries)
	}
	// Developers cannot read the audit log.
	if rec := e.get("/api/v1/audit"); rec.Code != 403 {
		t.Errorf("developer audit: status = %d, want 403", rec.Code)
	}
}

func TestUserManagement(t *testing.T) {
	e := newEnv(t, fakeCluster{}, nil, rbac.Admin)
	rec := e.send(http.MethodPost, "/api/v1/users", `{"username":"dev1","password":"`+testPassword+`","role":"developer","namespaces":["demo"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", rec.Code, rec.Body)
	}
	var u auth.User
	json.Unmarshal(rec.Body.Bytes(), &u)
	if u.Role != rbac.Developer || len(u.Namespaces) != 1 {
		t.Errorf("created = %+v", u)
	}
	if rec := e.send(http.MethodPost, "/api/v1/users", `{"username":"dev1","password":"`+testPassword+`","role":"viewer"}`); rec.Code != 400 {
		t.Errorf("duplicate: status = %d, want 400", rec.Code)
	}
	if rec := e.send(http.MethodPost, "/api/v1/users", `{"username":"short","password":"abc","role":"viewer"}`); rec.Code != 400 {
		t.Errorf("weak password: status = %d, want 400", rec.Code)
	}

	// Administrators cannot lock themselves out.
	me := e.get("/api/v1/auth/me")
	var who struct{ User auth.User }
	json.Unmarshal(me.Body.Bytes(), &who)
	self := "/api/v1/users/" + itoa(who.User.ID)
	if rec := e.send(http.MethodPatch, self, `{"role":"viewer"}`); rec.Code != 400 {
		t.Errorf("self demote: status = %d, want 400", rec.Code)
	}
	if rec := e.send(http.MethodDelete, self, ""); rec.Code != 400 {
		t.Errorf("self delete: status = %d, want 400", rec.Code)
	}
	if rec := e.send(http.MethodDelete, "/api/v1/users/"+itoa(u.ID), ""); rec.Code != 204 {
		t.Errorf("delete dev1: status = %d", rec.Code)
	}
}

func TestLogoutEndsSession(t *testing.T) {
	e := newEnv(t, fakeCluster{}, nil, rbac.Viewer)
	if rec := e.send(http.MethodPost, "/api/v1/auth/logout", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: status = %d", rec.Code)
	}
	if rec := e.get("/api/v1/auth/me"); rec.Code != http.StatusUnauthorized {
		t.Errorf("after logout: status = %d, want 401", rec.Code)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
