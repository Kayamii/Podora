package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Kayamii/Podora/backend/internal/helm"
	"github.com/Kayamii/Podora/backend/internal/rbac"
)

// HelmManager is the Helm access the API needs.
type HelmManager interface {
	LoadArchive(data []byte) (*helm.ChartInfo, error)
	FetchFromRepository(ctx context.Context, repoURL, name, version string) (*helm.ChartInfo, error)
	Releases() ([]helm.ReleaseSummary, error)
	Release(namespace, name string) (*helm.ReleaseDetail, error)
	Install(req helm.InstallRequest) (*helm.ReleaseSummary, *helm.Rendered, error)
	Upgrade(namespace, name string, req helm.UpgradeRequest) (*helm.ReleaseSummary, *helm.Rendered, error)
	Rollback(namespace, name string, revision int) error
	Uninstall(namespace, name string) error
}

func registerHelm(mux *http.ServeMux, s *server, hm HelmManager) {
	h := &helmHandlers{helm: hm, rec: s.rec}
	g := s.guard
	mux.HandleFunc("GET /api/v1/helm/releases", g(rbac.Read, scopeRole, h.releases))
	mux.HandleFunc("GET /api/v1/helm/releases/{namespace}/{name}", g(rbac.Read, scopePath, h.release))

	// Helm management needs access to every namespace (see rbac.ManageHelm).
	// Staging a chart is not a change yet, but it only leads to one.
	mux.HandleFunc("POST /api/v1/helm/charts", change(g(rbac.ManageHelm, scopeRole, h.uploadChart)))
	mux.HandleFunc("POST /api/v1/helm/charts/fetch", change(g(rbac.ManageHelm, scopeRole, h.fetchChart)))

	mux.HandleFunc("POST /api/v1/helm/releases", change(g(rbac.ManageHelm, scopeRole, h.install)))
	mux.HandleFunc("POST /api/v1/helm/releases/{namespace}/{name}/upgrade", change(g(rbac.ManageHelm, scopePath, h.upgrade)))
	mux.HandleFunc("POST /api/v1/helm/releases/{namespace}/{name}/rollback", change(g(rbac.ManageHelm, scopePath, h.rollback)))
	mux.HandleFunc("DELETE /api/v1/helm/releases/{namespace}/{name}", change(g(rbac.ManageHelm, scopePath, h.uninstall)))
}

type helmHandlers struct {
	helm HelmManager
	rec  recorder
}

// operationResponse answers install and upgrade: a release, or with dryRun
// the rendered manifest.
type operationResponse struct {
	Release  *helm.ReleaseSummary `json:"release,omitempty"`
	Rendered *helm.Rendered       `json:"rendered,omitempty"`
}

func (h *helmHandlers) releases(w http.ResponseWriter, r *http.Request) {
	list, err := h.helm.Releases()
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, visible(r, list, func(x helm.ReleaseSummary) string { return x.Namespace }))
}

func (h *helmHandlers) release(w http.ResponseWriter, r *http.Request) {
	d, err := h.helm.Release(r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// uploadChart accepts a multipart form with the archive in field "chart".
func (h *helmHandlers) uploadChart(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, helm.MaxChartBytes+1<<20)
	file, _, err := r.FormFile("chart")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("upload a .tgz chart archive of at most %d MB in the form field \"chart\"", helm.MaxChartBytes>>20),
		})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, helm.MaxChartBytes+1))
	if err != nil || len(data) > helm.MaxChartBytes {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("the chart archive must be at most %d MB", helm.MaxChartBytes>>20)})
		return
	}
	info, err := h.helm.LoadArchive(data)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type fetchRequest struct {
	RepoURL string `json:"repoURL"`
	Chart   string `json:"chart"`
	Version string `json:"version"`
}

func (h *helmHandlers) fetchChart(w http.ResponseWriter, r *http.Request) {
	var req fetchRequest
	if !decode(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*requestTimeout)
	defer cancel()
	info, err := h.helm.FetchFromRepository(ctx, req.RepoURL, req.Chart, req.Version)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *helmHandlers) install(w http.ResponseWriter, r *http.Request) {
	var req helm.InstallRequest
	if !decode(w, r, &req) {
		return
	}
	rel, rendered, err := h.helm.Install(req)
	if !req.DryRun {
		h.rec.record(r, "helm.install", req.Namespace, "Release/"+req.Name, chartDetail(rel), err)
	}
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, operationResponse{Release: rel, Rendered: rendered})
}

func (h *helmHandlers) upgrade(w http.ResponseWriter, r *http.Request) {
	var req helm.UpgradeRequest
	if !decode(w, r, &req) {
		return
	}
	rel, rendered, err := h.helm.Upgrade(r.PathValue("namespace"), r.PathValue("name"), req)
	if !req.DryRun {
		h.rec.record(r, "helm.upgrade", r.PathValue("namespace"), "Release/"+r.PathValue("name"), chartDetail(rel), err)
	}
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, operationResponse{Release: rel, Rendered: rendered})
}

type rollbackRequest struct {
	Revision int `json:"revision"`
}

func (h *helmHandlers) rollback(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if !decode(w, r, &req) {
		return
	}
	err := h.helm.Rollback(r.PathValue("namespace"), r.PathValue("name"), req.Revision)
	h.rec.record(r, "helm.rollback", r.PathValue("namespace"), "Release/"+r.PathValue("name"), fmt.Sprintf("to revision %d", req.Revision), err)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *helmHandlers) uninstall(w http.ResponseWriter, r *http.Request) {
	err := h.helm.Uninstall(r.PathValue("namespace"), r.PathValue("name"))
	h.rec.record(r, "helm.uninstall", r.PathValue("namespace"), "Release/"+r.PathValue("name"), "", err)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decode reads a JSON body of at most 4 MB (values files can be large).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the request body must be valid JSON: " + err.Error()})
		return false
	}
	return true
}

// chartDetail describes the chart a release ended up on, for the audit log.
func chartDetail(rel *helm.ReleaseSummary) string {
	if rel == nil {
		return ""
	}
	return fmt.Sprintf("chart %s %s, revision %d", rel.Chart, rel.ChartVersion, rel.Revision)
}

func isHelmInvalid(err error) bool  { return errors.Is(err, helm.ErrInvalid) }
func isHelmNotFound(err error) bool { return errors.Is(err, helm.ErrNotFound) }
func isHelmNoAccess(err error) bool { return errors.Is(err, helm.ErrNoAccess) }
