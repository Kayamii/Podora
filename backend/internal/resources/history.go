package resources

import (
	"context"
	"log/slog"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SampleInterval is how often cluster usage is recorded.
	SampleInterval = 10 * time.Second
	// historyWindow is how far back usage history reaches.
	historyWindow = time.Hour
	maxSamples    = int(historyWindow / SampleInterval)
)

// Sample is cluster-wide compute usage at one moment. CPU values are
// millicores, memory values are bytes.
type Sample struct {
	Time              time.Time `json:"t"`
	CPUUsage          int64     `json:"cpuUsage"`
	CPUAllocatable    int64     `json:"cpuAllocatable"`
	MemoryUsage       int64     `json:"memoryUsage"`
	MemoryAllocatable int64     `json:"memoryAllocatable"`
}

// history is a bounded, in-memory record of recent samples. It is lost on
// restart by design: Podora keeps no metrics store of its own.
type history struct {
	mu      sync.RWMutex
	samples []Sample
}

func (h *history) add(s Sample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.samples = append(h.samples, s)
	if extra := len(h.samples) - maxSamples; extra > 0 {
		// Copy down rather than re-slice so the backing array cannot grow
		// without bound.
		h.samples = append(h.samples[:0], h.samples[extra:]...)
	}
}

func (h *history) snapshot() []Sample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Sample, len(h.samples))
	copy(out, h.samples)
	return out
}

// History returns the recorded usage samples, oldest first.
func (s *Service) History() []Sample {
	return s.history.snapshot()
}

// RunSampler records usage every SampleInterval until ctx is cancelled.
// Failed samples (cluster unreachable, metrics-server missing) are skipped,
// leaving a gap in the history rather than a made-up value.
func (s *Service) RunSampler(ctx context.Context) {
	ticker := time.NewTicker(SampleInterval)
	defer ticker.Stop()

	failing := false
	for {
		sample, err := s.sample(ctx)
		switch {
		case err == nil:
			s.history.add(sample)
			s.recordWorkloadUsage(ctx, sample.Time)
			failing = false
		case ctx.Err() != nil:
			return
		case !failing:
			// Log the first failure of a streak only, not every tick.
			slog.Warn("usage sampling failed; history will have a gap", "error", err)
			failing = true
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) sample(ctx context.Context) (Sample, error) {
	ctx, cancel := context.WithTimeout(ctx, SampleInterval)
	defer cancel()

	nodes, err := s.kube.Core.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Sample{}, err
	}
	usage, err := s.kube.Metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Sample{}, err
	}

	out := Sample{Time: time.Now().UTC()}
	for i := range nodes.Items {
		alloc := nodes.Items[i].Status.Allocatable
		out.CPUAllocatable += alloc.Cpu().MilliValue()
		out.MemoryAllocatable += alloc.Memory().Value()
	}
	for i := range usage.Items {
		u := usage.Items[i].Usage
		out.CPUUsage += u.Cpu().MilliValue()
		out.MemoryUsage += u.Memory().Value()
	}
	return out, nil
}
