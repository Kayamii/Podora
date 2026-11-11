package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Kayamii/Podora/backend/internal/resources"
)

// defaultLogTailLines is used when a log request does not say how many lines
// it wants.
const defaultLogTailLines = 100

func (h *handlers) pods(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	pods, err := h.cluster.Pods(ctx, r.URL.Query().Get("namespace"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, visible(r, pods, func(p resources.PodSummary) string { return p.Namespace }))
}

func (h *handlers) pod(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	pod, err := h.cluster.Pod(ctx, r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pod)
}

// podLogs streams a container's log as plain text. With follow=true the
// response stays open and new lines are flushed as they arrive, until the
// client disconnects or the container stops.
//
// Query: container, tail (lines, default 100), previous, follow, timestamps.
func (h *handlers) podLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := resources.LogOptions{
		Container:  q.Get("container"),
		Previous:   q.Get("previous") == "true",
		Follow:     q.Get("follow") == "true",
		Timestamps: q.Get("timestamps") == "true",
		TailLines:  defaultLogTailLines,
	}
	if raw := q.Get("tail"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 || n > resources.MaxLogTailLines {
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error: "tail must be a number between 1 and " + strconv.Itoa(resources.MaxLogTailLines),
			})
			return
		}
		opts.TailLines = n
	}

	// No timeout here: a followed log is open-ended. The request context ends
	// the upstream stream when the client goes away.
	stream, err := h.cluster.PodLogs(r.Context(), r.PathValue("namespace"), r.PathValue("name"), opts)
	if err != nil {
		writeClusterError(w, r, err)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Ask reverse proxies (nginx) not to buffer, or live lines would arrive
	// in delayed batches.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := stream.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return // client disconnected
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			// The status line is already sent, so a mid-stream failure can
			// only be logged; the client sees the stream end.
			if !errors.Is(readErr, io.EOF) && r.Context().Err() == nil {
				slog.Warn("log stream ended with error", "path", r.URL.Path, "error", readErr)
			}
			return
		}
	}
}
