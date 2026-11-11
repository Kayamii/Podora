package resources

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// maxTrackedPods bounds per-pod history so memory stays predictable on large
// clusters (each pod costs about 9 KB for an hour of samples).
const maxTrackedPods = 2000

// NamespaceUsage is what one namespace is using right now. CPU values are
// millicores, memory values are bytes. Usage is null without metrics-server.
type NamespaceUsage struct {
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	Pods           int       `json:"pods"`
	RunningPods    int       `json:"runningPods"`
	CPUUsage       *int64    `json:"cpuUsage"`
	MemoryUsage    *int64    `json:"memoryUsage"`
	CPURequests    int64     `json:"cpuRequests"`
	CPULimits      int64     `json:"cpuLimits"`
	MemoryRequests int64     `json:"memoryRequests"`
	MemoryLimits   int64     `json:"memoryLimits"`
	CreatedAt      time.Time `json:"createdAt"`
}

// UsagePoint is the usage of a namespace or pod at one moment.
type UsagePoint struct {
	Time   time.Time `json:"t"`
	CPU    int64     `json:"cpu"`    // millicores
	Memory int64     `json:"memory"` // bytes
}

// NamespacesUsage lists every namespace with its pods' requests, limits and
// current usage.
func (s *Service) NamespacesUsage(ctx context.Context) ([]NamespaceUsage, error) {
	var (
		nss  *corev1.NamespaceList
		pods *corev1.PodList
		core = s.kube.Core.CoreV1()
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { nss, err = core.Namespaces().List(gctx, metav1.ListOptions{}); return })
	g.Go(func() (err error) {
		pods, err = core.Pods(metav1.NamespaceAll).List(gctx, metav1.ListOptions{})
		return
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("listing namespaces and pods: %w", err)
	}
	usage := s.podUsage(ctx, metav1.NamespaceAll)
	metrics := len(usage) > 0

	byName := make(map[string]*NamespaceUsage, len(nss.Items))
	out := make([]NamespaceUsage, len(nss.Items))
	for i := range nss.Items {
		ns := &nss.Items[i]
		out[i] = NamespaceUsage{Name: ns.Name, Status: string(ns.Status.Phase), CreatedAt: ns.CreationTimestamp.Time}
		if metrics {
			var cpu, mem int64
			out[i].CPUUsage, out[i].MemoryUsage = &cpu, &mem
		}
		byName[ns.Name] = &out[i]
	}

	for i := range pods.Items {
		p := &pods.Items[i]
		nu, ok := byName[p.Namespace]
		if !ok {
			continue
		}
		nu.Pods++
		if p.Status.Phase == corev1.PodRunning {
			nu.RunningPods++
		}
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue // finished pods hold no resources
		}
		for j := range p.Spec.Containers {
			r := p.Spec.Containers[j].Resources
			nu.CPURequests += r.Requests.Cpu().MilliValue()
			nu.CPULimits += r.Limits.Cpu().MilliValue()
			nu.MemoryRequests += r.Requests.Memory().Value()
			nu.MemoryLimits += r.Limits.Memory().Value()
		}
		if u, ok := usage[p.Namespace+"/"+p.Name]; ok && metrics {
			*nu.CPUUsage += u.cpu
			*nu.MemoryUsage += u.mem
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// UsageHistory returns recorded usage for a namespace, or for one pod when
// pod is set, oldest first. It is empty until the sampler has run.
func (s *Service) UsageHistory(namespace, pod string) []UsagePoint {
	key := "ns:" + namespace
	if pod != "" {
		key = "pod:" + namespace + "/" + pod
	}
	return s.series.get(key)
}

// seriesStore keeps a bounded history per namespace and per pod.
type seriesStore struct {
	mu     sync.RWMutex
	series map[string][]UsagePoint
}

func (st *seriesStore) get(key string) []UsagePoint {
	st.mu.RLock()
	defer st.mu.RUnlock()
	points := st.series[key]
	out := make([]UsagePoint, len(points))
	copy(out, points)
	return out
}

// record appends one point per key and forgets keys that were not seen in
// this round (deleted pods and namespaces), so the store cannot grow without
// bound.
func (st *seriesStore) record(t time.Time, values map[string]usage) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.series == nil {
		st.series = map[string][]UsagePoint{}
	}
	for key := range st.series {
		if _, ok := values[key]; !ok {
			delete(st.series, key)
		}
	}
	for key, u := range values {
		points := append(st.series[key], UsagePoint{Time: t, CPU: u.cpu, Memory: u.mem})
		if extra := len(points) - maxSamples; extra > 0 {
			points = append(points[:0], points[extra:]...)
		}
		st.series[key] = points
	}
}

// recordWorkloadUsage samples per-pod and per-namespace usage for the history.
func (s *Service) recordWorkloadUsage(ctx context.Context, t time.Time) {
	byPod := s.podUsage(ctx, metav1.NamespaceAll)
	if len(byPod) == 0 {
		return // metrics unavailable: leave a gap
	}
	values := make(map[string]usage, len(byPod)+16)
	tracked := 0
	for key, u := range byPod {
		ns := key[:indexByte(key, '/')]
		agg := values["ns:"+ns]
		agg.cpu += u.cpu
		agg.mem += u.mem
		values["ns:"+ns] = agg
		if tracked < maxTrackedPods {
			values["pod:"+key] = u
			tracked++
		}
	}
	s.series.record(t, values)
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return len(s)
}
