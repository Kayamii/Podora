// Package api exposes Podora's REST API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/Kayamii/Podora/backend/internal/auth"
	"github.com/Kayamii/Podora/backend/internal/rbac"
	"github.com/Kayamii/Podora/backend/internal/resources"
)

// Version is the build version, overridden at link time with
// -ldflags "-X github.com/Kayamii/Podora/backend/internal/api.Version=...".
var Version = "dev"

// requestTimeout bounds how long a handler waits on the Kubernetes API.
const requestTimeout = 20 * time.Second

// ClusterReader is the read access to the cluster the API needs.
type ClusterReader interface {
	Overview(ctx context.Context) (*resources.Overview, error)
	Namespaces(ctx context.Context) ([]resources.NamespaceSummary, error)
	// History returns recent cluster usage samples, oldest first.
	History() []resources.Sample
	// Pods lists pods in one namespace, or all namespaces when it is empty.
	Pods(ctx context.Context, namespace string) ([]resources.PodSummary, error)
	Pod(ctx context.Context, namespace, name string) (*resources.PodDetail, error)
	// PodLogs opens a container log; the caller closes it.
	PodLogs(ctx context.Context, namespace, name string, opts resources.LogOptions) (io.ReadCloser, error)

	Workloads(ctx context.Context, namespace string) ([]resources.WorkloadSummary, error)
	Workload(ctx context.Context, kind resources.WorkloadKind, namespace, name string) (*resources.WorkloadDetail, error)
	Services(ctx context.Context, namespace string) ([]resources.ServiceSummary, error)
	Ingresses(ctx context.Context, namespace string) ([]resources.IngressSummary, error)
	Events(ctx context.Context, namespace string) ([]resources.EventSummary, error)
	NamespacesUsage(ctx context.Context) ([]resources.NamespaceUsage, error)
	UsageHistory(namespace, pod string) []resources.UsagePoint

	// Changes to the cluster.
	DeletePod(ctx context.Context, namespace, name string) error
	ScaleWorkload(ctx context.Context, kind resources.WorkloadKind, namespace, name string, replicas int32) error
	RestartWorkload(ctx context.Context, kind resources.WorkloadKind, namespace, name string) error
}

// Deps are what the API is built from.
type Deps struct {
	Cluster ClusterReader
	// Helm may be nil, which leaves the Helm endpoints out.
	Helm  HelmManager
	Auth  *auth.Service
	Audit Auditor
}

type server struct {
	auth *auth.Service
	rec  recorder
}

// NewRouter returns the HTTP handler serving all API routes. Everything except
// health checks and sign-in needs a session, and each route checks the
// signed-in user's role and namespaces before doing anything.
func NewRouter(d Deps) http.Handler {
	s := &server{auth: d.Auth, rec: recorder{store: d.Audit}}
	h := &handlers{cluster: d.Cluster, rec: s.rec}
	g := s.guard

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /api/v1/health", h.health)
	s.registerAuth(mux)

	// Cluster-wide views need access to every namespace.
	mux.HandleFunc("GET /api/v1/overview", g(rbac.Read, scopeCluster, h.overview))
	mux.HandleFunc("GET /api/v1/metrics/history", g(rbac.Read, scopeCluster, h.history))

	// Lists are filtered to the namespaces the user may see.
	mux.HandleFunc("GET /api/v1/namespaces", g(rbac.Read, scopeRole, h.namespaces))
	mux.HandleFunc("GET /api/v1/namespaces/usage", g(rbac.Read, scopeRole, h.namespacesUsage))
	mux.HandleFunc("GET /api/v1/pods", g(rbac.Read, scopeQuery, h.pods))
	mux.HandleFunc("GET /api/v1/workloads", g(rbac.Read, scopeQuery, h.workloads))
	mux.HandleFunc("GET /api/v1/services", g(rbac.Read, scopeQuery, h.services))
	mux.HandleFunc("GET /api/v1/ingresses", g(rbac.Read, scopeQuery, h.ingresses))
	mux.HandleFunc("GET /api/v1/events", g(rbac.Read, scopeQuery, h.events))
	mux.HandleFunc("GET /api/v1/metrics/usage-history", g(rbac.Read, scopeQuery, h.usageHistory))

	// Single resources check their own namespace.
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pods/{name}", g(rbac.Read, scopePath, h.pod))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pods/{name}/logs", g(rbac.Read, scopePath, h.podLogs))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/workloads/{kind}/{name}", g(rbac.Read, scopePath, h.workload))

	// Everything below changes the cluster.
	mux.HandleFunc("DELETE /api/v1/namespaces/{namespace}/pods/{name}", change(g(rbac.DeletePod, scopePath, h.deletePod)))
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/workloads/{kind}/{name}/scale", change(g(rbac.ScaleWork, scopePath, h.scaleWorkload)))
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/workloads/{kind}/{name}/restart", change(g(rbac.RestartWork, scopePath, h.restartWorkload)))

	if d.Helm != nil {
		registerHelm(mux, s, d.Helm)
	}
	return mux
}

type handlers struct {
	cluster ClusterReader
	rec     recorder
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// health reports that the process is up. It deliberately does not touch the
// cluster, so a Kubernetes outage does not get Podora's own pod restarted.
func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: Version})
}

func (h *handlers) overview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	o, err := h.cluster.Overview(ctx)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *handlers) namespaces(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	nss, err := h.cluster.Namespaces(ctx)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, visible(r, nss, func(n resources.NamespaceSummary) string { return n.Name }))
}

type historyResponse struct {
	IntervalSeconds int                `json:"intervalSeconds"`
	Samples         []resources.Sample `json:"samples"`
}

// history serves usage recorded in memory since the backend started; it does
// not call the cluster.
func (h *handlers) history(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, historyResponse{
		IntervalSeconds: int(resources.SampleInterval / time.Second),
		Samples:         h.cluster.History(),
	})
}

// writeClusterError reports a failed Kubernetes call. Answers the cluster
// gave about the request itself (no such pod, invalid option) keep their
// meaning; anything else is 502: Podora is fine, the cluster API is not.
func writeClusterError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, resources.ErrInvalid), isHelmInvalid(err):
		status = http.StatusBadRequest
	case isHelmNotFound(err):
		status = http.StatusNotFound
	case apierrors.IsNotFound(err):
		status = http.StatusNotFound
	case apierrors.IsBadRequest(err), apierrors.IsInvalid(err):
		status = http.StatusBadRequest
	case apierrors.IsForbidden(err):
		status = http.StatusForbidden
	}
	if status == http.StatusBadGateway {
		slog.Error("cluster request failed", "path", r.URL.Path, "error", err)
	}
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writing response", "error", err)
	}
}
