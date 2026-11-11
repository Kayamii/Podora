package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Kayamii/Podora/backend/internal/resources"
)

// changeHeader must be present on every request that changes the cluster.
// Browsers do not let another site send a custom header without a CORS
// preflight, which Podora never approves, so this stops a malicious page from
// triggering changes through a user's browser. It is not authentication.
const changeHeader = "X-Podora-Request"

// change wraps a handler that modifies the cluster.
func change(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(changeHeader) == "" {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "missing " + changeHeader + " header"})
			return
		}
		next(w, r)
	}
}

// list serves a list endpoint, filtered by ?namespace= and by what the
// signed-in user may see.
func list[T any](fetch func(context.Context, string) ([]T, error), namespace func(T) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		items, err := fetch(ctx, r.URL.Query().Get("namespace"))
		if err != nil {
			writeClusterError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, visible(r, items, namespace))
	}
}

func (h *handlers) workloads(w http.ResponseWriter, r *http.Request) {
	list(h.cluster.Workloads, func(x resources.WorkloadSummary) string { return x.Namespace })(w, r)
}

func (h *handlers) services(w http.ResponseWriter, r *http.Request) {
	list(h.cluster.Services, func(x resources.ServiceSummary) string { return x.Namespace })(w, r)
}

func (h *handlers) ingresses(w http.ResponseWriter, r *http.Request) {
	list(h.cluster.Ingresses, func(x resources.IngressSummary) string { return x.Namespace })(w, r)
}

func (h *handlers) events(w http.ResponseWriter, r *http.Request) {
	list(h.cluster.Events, func(x resources.EventSummary) string { return x.Namespace })(w, r)
}

func (h *handlers) workload(w http.ResponseWriter, r *http.Request) {
	kind, err := resources.ParseWorkloadKind(r.PathValue("kind"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	detail, err := h.cluster.Workload(ctx, kind, r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *handlers) deletePod(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	err := h.cluster.DeletePod(ctx, r.PathValue("namespace"), r.PathValue("name"))
	h.rec.record(r, "pod.delete", r.PathValue("namespace"), "Pod/"+r.PathValue("name"), "", err)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scaleRequest struct {
	Replicas *int32 `json:"replicas"`
}

func (h *handlers) scaleWorkload(w http.ResponseWriter, r *http.Request) {
	kind, err := resources.ParseWorkloadKind(r.PathValue("kind"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	var req scaleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.Replicas == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: `body must be JSON like {"replicas": 3}`})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	err = h.cluster.ScaleWorkload(ctx, kind, r.PathValue("namespace"), r.PathValue("name"), *req.Replicas)
	h.rec.record(r, "workload.scale", r.PathValue("namespace"), string(kind)+"/"+r.PathValue("name"), fmt.Sprintf("to %d replicas", *req.Replicas), err)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) restartWorkload(w http.ResponseWriter, r *http.Request) {
	kind, err := resources.ParseWorkloadKind(r.PathValue("kind"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	err = h.cluster.RestartWorkload(ctx, kind, r.PathValue("namespace"), r.PathValue("name"))
	h.rec.record(r, "workload.restart", r.PathValue("namespace"), string(kind)+"/"+r.PathValue("name"), "", err)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) namespacesUsage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	usage, err := h.cluster.NamespacesUsage(ctx)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, visible(r, usage, func(n resources.NamespaceUsage) string { return n.Name }))
}

type usageHistoryResponse struct {
	IntervalSeconds int                    `json:"intervalSeconds"`
	Points          []resources.UsagePoint `json:"points"`
}

// usageHistory serves recorded usage for ?namespace=X, or for one pod with
// ?namespace=X&pod=Y. It reads memory only, never the cluster.
func (h *handlers) usageHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("namespace") == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "namespace is required"})
		return
	}
	writeJSON(w, http.StatusOK, usageHistoryResponse{
		IntervalSeconds: int(resources.SampleInterval.Seconds()),
		Points:          h.cluster.UsageHistory(q.Get("namespace"), q.Get("pod")),
	})
}
