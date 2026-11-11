package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/resources"
)

type fakeCluster struct {
	overview *resources.Overview
	err      error
	logs     string
	// logOpts records the options of the last PodLogs call.
	logOpts *resources.LogOptions
	// changes records cluster changes; see resources_test.go.
	changes *changes
}

func (f fakeCluster) Pods(_ context.Context, namespace string) ([]resources.PodSummary, error) {
	return []resources.PodSummary{{Name: "web", Namespace: namespace}}, f.err
}

func (f fakeCluster) Pod(_ context.Context, namespace, name string) (*resources.PodDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &resources.PodDetail{PodSummary: resources.PodSummary{Name: name, Namespace: namespace}}, nil
}

func (f fakeCluster) PodLogs(_ context.Context, _, _ string, opts resources.LogOptions) (io.ReadCloser, error) {
	if f.logOpts != nil {
		*f.logOpts = opts
	}
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.logs)), nil
}

func (f fakeCluster) Overview(context.Context) (*resources.Overview, error) {
	return f.overview, f.err
}

func (f fakeCluster) Namespaces(context.Context) ([]resources.NamespaceSummary, error) {
	return nil, f.err
}

func (f fakeCluster) History() []resources.Sample {
	return []resources.Sample{{CPUUsage: 100}}
}

func get(t *testing.T, cluster ClusterReader, path string) *httptest.ResponseRecorder {
	t.Helper()
	// An administrator: these tests are about responses, not permissions.
	return newEnv(t, cluster, nil, rbac.Admin).get(path)
}

func TestHealth(t *testing.T) {
	// Health must answer even when the cluster is unreachable.
	cluster := fakeCluster{err: errors.New("cluster down")}
	for _, path := range []string{"/healthz", "/api/v1/health"} {
		rec := get(t, cluster, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		var body healthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decoding body: %v", path, err)
		}
		if body.Status != "ok" {
			t.Errorf("%s: status = %q, want ok", path, body.Status)
		}
	}
}

func TestOverview(t *testing.T) {
	want := &resources.Overview{Cluster: resources.ClusterInfo{Version: "v1.35.0"}}
	rec := get(t, fakeCluster{overview: want}, "/api/v1/overview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got resources.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if got.Cluster.Version != want.Cluster.Version {
		t.Errorf("version = %q, want %q", got.Cluster.Version, want.Cluster.Version)
	}
}

func TestClusterErrorIsBadGateway(t *testing.T) {
	cluster := fakeCluster{err: errors.New("connection refused")}
	for _, path := range []string{"/api/v1/overview", "/api/v1/namespaces"} {
		rec := get(t, cluster, path)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("%s: status = %d, want 502", path, rec.Code)
		}
		var body errorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decoding body: %v", path, err)
		}
		if body.Error == "" {
			t.Errorf("%s: empty error message", path)
		}
	}
}

func TestHistory(t *testing.T) {
	rec := get(t, fakeCluster{}, "/api/v1/metrics/history")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body historyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body.IntervalSeconds != 10 || len(body.Samples) != 1 || body.Samples[0].CPUUsage != 100 {
		t.Errorf("body = %+v", body)
	}
}

func TestPods(t *testing.T) {
	rec := get(t, fakeCluster{}, "/api/v1/pods?namespace=demo")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var pods []resources.PodSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &pods); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if len(pods) != 1 || pods[0].Namespace != "demo" {
		t.Errorf("pods = %+v", pods)
	}
}

func TestPodNotFoundIs404(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "gone")
	rec := get(t, fakeCluster{err: notFound}, "/api/v1/namespaces/demo/pods/gone")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPodLogs(t *testing.T) {
	var got resources.LogOptions
	cluster := fakeCluster{logs: "line one\nline two\n", logOpts: &got}

	rec := get(t, cluster, "/api/v1/namespaces/demo/pods/web/logs?tail=50&previous=true&container=app")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != cluster.logs {
		t.Errorf("body = %q, want %q", body, cluster.logs)
	}
	want := resources.LogOptions{Container: "app", Previous: true, TailLines: 50}
	if got != want {
		t.Errorf("options = %+v, want %+v", got, want)
	}

	// Without tail, the default applies.
	get(t, cluster, "/api/v1/namespaces/demo/pods/web/logs")
	if got.TailLines != defaultLogTailLines {
		t.Errorf("default tail = %d, want %d", got.TailLines, defaultLogTailLines)
	}
}

func TestPodLogsRejectsBadTail(t *testing.T) {
	for _, tail := range []string{"0", "-5", "abc", "999999"} {
		rec := get(t, fakeCluster{}, "/api/v1/namespaces/demo/pods/web/logs?tail="+tail)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("tail=%s: status = %d, want 400", tail, rec.Code)
		}
	}
}
