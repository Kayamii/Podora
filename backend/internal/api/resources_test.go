package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/resources"
)

// changes records what the fake cluster was asked to change.
type changes struct {
	deleted   string
	scaledTo  int32
	restarted resources.WorkloadKind
}

func (f fakeCluster) Workloads(_ context.Context, ns string) ([]resources.WorkloadSummary, error) {
	return []resources.WorkloadSummary{{Name: "web", Namespace: ns}}, f.err
}

func (f fakeCluster) Workload(_ context.Context, kind resources.WorkloadKind, ns, name string) (*resources.WorkloadDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &resources.WorkloadDetail{WorkloadSummary: resources.WorkloadSummary{Kind: kind, Name: name, Namespace: ns}}, nil
}

func (f fakeCluster) Services(context.Context, string) ([]resources.ServiceSummary, error) {
	return []resources.ServiceSummary{}, f.err
}

func (f fakeCluster) Ingresses(context.Context, string) ([]resources.IngressSummary, error) {
	return []resources.IngressSummary{}, f.err
}

func (f fakeCluster) Events(context.Context, string) ([]resources.EventSummary, error) {
	return []resources.EventSummary{}, f.err
}

func (f fakeCluster) DeletePod(_ context.Context, ns, name string) error {
	if f.changes != nil {
		f.changes.deleted = ns + "/" + name
	}
	return f.err
}

func (f fakeCluster) ScaleWorkload(_ context.Context, _ resources.WorkloadKind, _, _ string, replicas int32) error {
	if f.changes != nil {
		f.changes.scaledTo = replicas
	}
	return f.err
}

func (f fakeCluster) RestartWorkload(_ context.Context, kind resources.WorkloadKind, _, _ string) error {
	if f.changes != nil {
		f.changes.restarted = kind
	}
	return f.err
}

func (f fakeCluster) NamespacesUsage(context.Context) ([]resources.NamespaceUsage, error) {
	return []resources.NamespaceUsage{{Name: "demo"}}, f.err
}

func (f fakeCluster) UsageHistory(namespace, pod string) []resources.UsagePoint {
	return []resources.UsagePoint{{CPU: int64(len(namespace + pod))}}
}

func send(t *testing.T, cluster ClusterReader, method, path, body string, withHeader bool) *httptest.ResponseRecorder {
	t.Helper()
	e := newEnv(t, cluster, nil, rbac.Admin)
	return e.do(method, path, "application/json", strings.NewReader(body), withHeader, true)
}

func TestListEndpoints(t *testing.T) {
	for _, path := range []string{"/api/v1/workloads", "/api/v1/services", "/api/v1/ingresses", "/api/v1/events?namespace=demo"} {
		if rec := get(t, fakeCluster{}, path); rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
	}
}

func TestWorkloadRejectsUnknownKind(t *testing.T) {
	if rec := get(t, fakeCluster{}, "/api/v1/namespaces/demo/workloads/cronjobs/x"); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if rec := get(t, fakeCluster{}, "/api/v1/namespaces/demo/workloads/deployments/web"); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// A change without the custom header must be refused and must not reach the
// cluster: this is what stops cross-site requests.
func TestChangesRequireHeader(t *testing.T) {
	cases := []struct{ method, path, body string }{
		{http.MethodDelete, "/api/v1/namespaces/demo/pods/web-1", ""},
		{http.MethodPost, "/api/v1/namespaces/demo/workloads/deployments/web/scale", `{"replicas":3}`},
		{http.MethodPost, "/api/v1/namespaces/demo/workloads/deployments/web/restart", ""},
	}
	for _, tc := range cases {
		got := &changes{scaledTo: -1}
		rec := send(t, fakeCluster{changes: got}, tc.method, tc.path, tc.body, false)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", tc.method, tc.path, rec.Code)
		}
		if got.deleted != "" || got.scaledTo != -1 || got.restarted != "" {
			t.Errorf("%s %s: change reached the cluster: %+v", tc.method, tc.path, got)
		}
	}
}

func TestChanges(t *testing.T) {
	got := &changes{}
	cluster := fakeCluster{changes: got}

	if rec := send(t, cluster, http.MethodDelete, "/api/v1/namespaces/demo/pods/web-1", "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", rec.Code)
	}
	if got.deleted != "demo/web-1" {
		t.Errorf("deleted = %q", got.deleted)
	}

	path := "/api/v1/namespaces/demo/workloads/statefulsets/db/"
	if rec := send(t, cluster, http.MethodPost, path+"scale", `{"replicas":4}`, true); rec.Code != http.StatusNoContent {
		t.Fatalf("scale: status = %d, want 204", rec.Code)
	}
	if got.scaledTo != 4 {
		t.Errorf("scaledTo = %d, want 4", got.scaledTo)
	}

	if rec := send(t, cluster, http.MethodPost, path+"restart", "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("restart: status = %d, want 204", rec.Code)
	}
	if got.restarted != resources.KindStatefulSet {
		t.Errorf("restarted = %q", got.restarted)
	}
}

func TestScaleRejectsBadBody(t *testing.T) {
	path := "/api/v1/namespaces/demo/workloads/deployments/web/scale"
	for _, body := range []string{"", "{}", `{"replicas":"many"}`, "not json"} {
		got := &changes{scaledTo: -1}
		rec := send(t, fakeCluster{changes: got}, http.MethodPost, path, body, true)
		if rec.Code != http.StatusBadRequest || got.scaledTo != -1 {
			t.Errorf("body %q: status = %d, scaledTo = %d", body, rec.Code, got.scaledTo)
		}
	}
}

func TestUsageHistory(t *testing.T) {
	if rec := get(t, fakeCluster{}, "/api/v1/metrics/usage-history"); rec.Code != http.StatusBadRequest {
		t.Errorf("without namespace: status = %d, want 400", rec.Code)
	}
	rec := get(t, fakeCluster{}, "/api/v1/metrics/usage-history?namespace=demo&pod=web")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cpu":7`) {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, fakeCluster{}, "/api/v1/namespaces/usage"); rec.Code != http.StatusOK {
		t.Errorf("namespaces usage: status = %d", rec.Code)
	}
}
