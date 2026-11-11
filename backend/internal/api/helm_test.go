package api

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kayamii/Podora/backend/internal/helm"
	"github.com/Kayamii/Podora/backend/internal/rbac"
)

type fakeHelm struct {
	installed *helm.InstallRequest
	rolledTo  int
}

func (f *fakeHelm) LoadArchive(data []byte) (*helm.ChartInfo, error) {
	if string(data) != "tgz" {
		return nil, fmt.Errorf("%w: not a chart", helm.ErrInvalid)
	}
	return &helm.ChartInfo{ID: "abc", Name: "demo"}, nil
}

func (f *fakeHelm) FetchFromRepository(_ context.Context, repo, name, _ string) (*helm.ChartInfo, error) {
	return &helm.ChartInfo{ID: "def", Name: name, Description: repo}, nil
}

func (f *fakeHelm) Releases() ([]helm.ReleaseSummary, error) {
	return []helm.ReleaseSummary{{Name: "web"}}, nil
}

func (f *fakeHelm) Release(_, name string) (*helm.ReleaseDetail, error) {
	if name == "missing" {
		return nil, fmt.Errorf("%w: release not found", helm.ErrNotFound)
	}
	return &helm.ReleaseDetail{ReleaseSummary: helm.ReleaseSummary{Name: name}}, nil
}

func (f *fakeHelm) Install(req helm.InstallRequest) (*helm.ReleaseSummary, *helm.Rendered, error) {
	if req.DryRun {
		return nil, &helm.Rendered{Manifest: "kind: Service"}, nil
	}
	f.installed = &req
	return &helm.ReleaseSummary{Name: req.Name}, nil, nil
}

func (f *fakeHelm) Upgrade(_, name string, _ helm.UpgradeRequest) (*helm.ReleaseSummary, *helm.Rendered, error) {
	return &helm.ReleaseSummary{Name: name, Revision: 2}, nil, nil
}

func (f *fakeHelm) Rollback(_, _ string, revision int) error {
	f.rolledTo = revision
	return nil
}

func (f *fakeHelm) Uninstall(_, _ string) error { return nil }

func helmRequest(t *testing.T, hm HelmManager, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	e := newEnv(t, fakeCluster{}, hm, rbac.Admin)
	return e.do(method, path, contentType, bytes.NewReader(body), true, true)
}

func TestHelmUpload(t *testing.T) {
	upload := func(content string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("chart", "demo.tgz")
		fw.Write([]byte(content))
		mw.Close()
		return helmRequest(t, &fakeHelm{}, http.MethodPost, "/api/v1/helm/charts", mw.FormDataContentType(), body.Bytes())
	}
	if rec := upload("tgz"); rec.Code != http.StatusOK {
		t.Errorf("valid chart: status = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := upload("junk"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid chart: status = %d, want 400", rec.Code)
	}
	if rec := helmRequest(t, &fakeHelm{}, http.MethodPost, "/api/v1/helm/charts", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("no file: status = %d, want 400", rec.Code)
	}
}

func TestHelmInstallAndDryRun(t *testing.T) {
	hm := &fakeHelm{}
	rec := helmRequest(t, hm, http.MethodPost, "/api/v1/helm/releases", "application/json",
		[]byte(`{"chartId":"abc","name":"web","namespace":"demo","values":"a: 1","dryRun":true}`))
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"manifest":"kind: Service"`)) {
		t.Fatalf("dry run: status = %d, body = %s", rec.Code, rec.Body)
	}
	if hm.installed != nil {
		t.Fatal("dry run installed something")
	}

	rec = helmRequest(t, hm, http.MethodPost, "/api/v1/helm/releases", "application/json",
		[]byte(`{"chartId":"abc","name":"web","namespace":"demo","createNamespace":true}`))
	if rec.Code != http.StatusOK || hm.installed == nil || !hm.installed.CreateNamespace {
		t.Fatalf("install: status = %d, installed = %+v", rec.Code, hm.installed)
	}
}

func TestHelmReleaseAndRollback(t *testing.T) {
	hm := &fakeHelm{}
	if rec := helmRequest(t, hm, http.MethodGet, "/api/v1/helm/releases/demo/missing", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing release: status = %d, want 404", rec.Code)
	}
	rec := helmRequest(t, hm, http.MethodPost, "/api/v1/helm/releases/demo/web/rollback", "application/json", []byte(`{"revision":3}`))
	if rec.Code != http.StatusNoContent || hm.rolledTo != 3 {
		t.Errorf("rollback: status = %d, rolledTo = %d", rec.Code, hm.rolledTo)
	}
}

func TestHelmChangesRequireHeader(t *testing.T) {
	e := newEnv(t, fakeCluster{}, &fakeHelm{}, rbac.Admin)
	rec := e.do(http.MethodDelete, "/api/v1/helm/releases/demo/web", "", nil, false, true)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
