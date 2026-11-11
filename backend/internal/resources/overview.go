package resources

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/Kayamii/Podora/backend/internal/kubernetes"
)

const (
	// systemNamespace is where the cluster's own components run.
	systemNamespace = "kube-system"
	maxEvents       = 25
)

// Service reads and summarizes cluster resources.
type Service struct {
	kube    *kubernetes.Client
	history history
	series  seriesStore
}

func NewService(kube *kubernetes.Client) *Service {
	return &Service{kube: kube}
}

// snapshot is the raw data one overview is computed from.
type snapshot struct {
	version *version.Info

	nodes      *corev1.NodeList
	pods       *corev1.PodList
	namespaces *corev1.NamespaceList
	services   *corev1.ServiceList
	events     *corev1.EventList
	pvs        *corev1.PersistentVolumeList
	pvcs       *corev1.PersistentVolumeClaimList

	deployments  *appsv1.DeploymentList
	statefulSets *appsv1.StatefulSetList
	daemonSets   *appsv1.DaemonSetList
	replicaSets  *appsv1.ReplicaSetList
	jobs         *batchv1.JobList
	cronJobs     *batchv1.CronJobList

	ingresses       *networkingv1.IngressList
	ingressClasses  *networkingv1.IngressClassList
	networkPolicies *networkingv1.NetworkPolicyList
	endpointSlices  *discoveryv1.EndpointSliceList
	storageClasses  *storagev1.StorageClassList

	// nil when the metrics API is unavailable.
	nodeMetrics *metricsv1beta1.NodeMetricsList
	podMetrics  *metricsv1beta1.PodMetricsList
}

// Overview fetches a cluster-wide summary. It fails if any core API call
// fails; missing metrics only disable the usage figures.
func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	snap, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	return summarize(snap, time.Now().UTC()), nil
}

// Namespaces lists namespaces with their pod counts.
func (s *Service) Namespaces(ctx context.Context) ([]NamespaceSummary, error) {
	core := s.kube.Core.CoreV1()
	nss, err := core.Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}
	pods, err := core.Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}
	return summarizeNamespaces(nss.Items, pods.Items), nil
}

func (s *Service) fetch(ctx context.Context) (*snapshot, error) {
	var (
		snap = &snapshot{}
		all  = metav1.NamespaceAll
		opts = metav1.ListOptions{}
		c    = s.kube.Core
	)

	g, ctx := errgroup.WithContext(ctx)
	// list runs one API call concurrently and labels its error.
	list := func(what string, fn func() error) {
		g.Go(func() error {
			if err := fn(); err != nil {
				return fmt.Errorf("listing %s: %w", what, err)
			}
			return nil
		})
	}

	list("server version", func() (err error) {
		snap.version, err = c.Discovery().ServerVersion()
		return
	})
	list("nodes", func() (err error) { snap.nodes, err = c.CoreV1().Nodes().List(ctx, opts); return })
	list("pods", func() (err error) { snap.pods, err = c.CoreV1().Pods(all).List(ctx, opts); return })
	list("namespaces", func() (err error) { snap.namespaces, err = c.CoreV1().Namespaces().List(ctx, opts); return })
	list("services", func() (err error) { snap.services, err = c.CoreV1().Services(all).List(ctx, opts); return })
	list("events", func() (err error) { snap.events, err = c.CoreV1().Events(all).List(ctx, opts); return })
	list("persistent volumes", func() (err error) {
		snap.pvs, err = c.CoreV1().PersistentVolumes().List(ctx, opts)
		return
	})
	list("persistent volume claims", func() (err error) {
		snap.pvcs, err = c.CoreV1().PersistentVolumeClaims(all).List(ctx, opts)
		return
	})
	list("deployments", func() (err error) { snap.deployments, err = c.AppsV1().Deployments(all).List(ctx, opts); return })
	list("statefulsets", func() (err error) { snap.statefulSets, err = c.AppsV1().StatefulSets(all).List(ctx, opts); return })
	list("daemonsets", func() (err error) { snap.daemonSets, err = c.AppsV1().DaemonSets(all).List(ctx, opts); return })
	list("replicasets", func() (err error) { snap.replicaSets, err = c.AppsV1().ReplicaSets(all).List(ctx, opts); return })
	list("jobs", func() (err error) { snap.jobs, err = c.BatchV1().Jobs(all).List(ctx, opts); return })
	list("cronjobs", func() (err error) { snap.cronJobs, err = c.BatchV1().CronJobs(all).List(ctx, opts); return })
	list("ingresses", func() (err error) {
		snap.ingresses, err = c.NetworkingV1().Ingresses(all).List(ctx, opts)
		return
	})
	list("ingress classes", func() (err error) {
		snap.ingressClasses, err = c.NetworkingV1().IngressClasses().List(ctx, opts)
		return
	})
	list("network policies", func() (err error) {
		snap.networkPolicies, err = c.NetworkingV1().NetworkPolicies(all).List(ctx, opts)
		return
	})
	list("endpoint slices", func() (err error) {
		snap.endpointSlices, err = c.DiscoveryV1().EndpointSlices(all).List(ctx, opts)
		return
	})
	list("storage classes", func() (err error) {
		snap.storageClasses, err = c.StorageV1().StorageClasses().List(ctx, opts)
		return
	})

	// Metrics are optional: metrics-server may be absent or still starting.
	// Errors here must not fail the overview, so they stay out of the group's
	// error path.
	g.Go(func() error {
		m := s.kube.Metrics.MetricsV1beta1()
		nodeMetrics, err := m.NodeMetricses().List(ctx, opts)
		if err != nil {
			return nil
		}
		podMetrics, err := m.PodMetricses(all).List(ctx, opts)
		if err != nil {
			return nil
		}
		snap.nodeMetrics, snap.podMetrics = nodeMetrics, podMetrics
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return snap, nil
}

func summarize(s *snapshot, now time.Time) *Overview {
	o := &Overview{GeneratedAt: now}

	o.Cluster = ClusterInfo{
		Version:    s.version.GitVersion,
		Platform:   s.version.Platform,
		NodesTotal: len(s.nodes.Items),
	}

	// --- pods: counts, per-node counts, requests and limits ---
	podsPerNode := map[string]int{}
	res := &o.Resources
	for i := range s.pods.Items {
		p := &s.pods.Items[i]
		o.Pods.Total++
		switch p.Status.Phase {
		case corev1.PodRunning:
			o.Pods.Running++
		case corev1.PodPending:
			o.Pods.Pending++
		case corev1.PodSucceeded:
			o.Pods.Succeeded++
		case corev1.PodFailed:
			o.Pods.Failed++
		default:
			o.Pods.Unknown++
		}
		o.Pods.Restarts += podRestarts(p)

		// Finished pods no longer hold their node's resources.
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue
		}
		if p.Spec.NodeName != "" {
			podsPerNode[p.Spec.NodeName]++
		}
		for j := range p.Spec.Containers {
			r := p.Spec.Containers[j].Resources
			res.CPU.Requests += r.Requests.Cpu().MilliValue()
			res.CPU.Limits += r.Limits.Cpu().MilliValue()
			res.Memory.Requests += r.Requests.Memory().Value()
			res.Memory.Limits += r.Limits.Memory().Value()
		}
	}

	// --- metrics lookups ---
	var nodeUsage, podUsage map[string]usage
	if s.nodeMetrics != nil && s.podMetrics != nil {
		res.MetricsAvailable = true
		nodeUsage = make(map[string]usage, len(s.nodeMetrics.Items))
		for _, m := range s.nodeMetrics.Items {
			nodeUsage[m.Name] = usage{m.Usage.Cpu().MilliValue(), m.Usage.Memory().Value()}
		}
		podUsage = make(map[string]usage, len(s.podMetrics.Items))
		for _, m := range s.podMetrics.Items {
			var u usage
			for _, c := range m.Containers {
				u.cpu += c.Usage.Cpu().MilliValue()
				u.mem += c.Usage.Memory().Value()
			}
			podUsage[m.Namespace+"/"+m.Name] = u
		}
	}

	// --- nodes ---
	var cpuUsed, memUsed int64
	o.Nodes = make([]NodeSummary, 0, len(s.nodes.Items))
	for i := range s.nodes.Items {
		n := &s.nodes.Items[i]
		ready := nodeReady(n)
		if ready {
			o.Cluster.NodesReady++
		}
		alloc, capacity := n.Status.Allocatable, n.Status.Capacity
		res.CPU.Capacity += capacity.Cpu().MilliValue()
		res.CPU.Allocatable += alloc.Cpu().MilliValue()
		res.Memory.Capacity += capacity.Memory().Value()
		res.Memory.Allocatable += alloc.Memory().Value()
		res.PodCapacity += alloc.Pods().Value()

		ns := NodeSummary{
			Name:           n.Name,
			Ready:          ready,
			Schedulable:    !n.Spec.Unschedulable,
			Roles:          nodeRoles(n),
			KubeletVersion: n.Status.NodeInfo.KubeletVersion,
			OSImage:        n.Status.NodeInfo.OSImage,
			InternalIP:     nodeInternalIP(n),
			CreatedAt:      n.CreationTimestamp.Time,
			Pods:           podsPerNode[n.Name],
			PodCapacity:    alloc.Pods().Value(),
			CPU:            NodeUsage{Allocatable: alloc.Cpu().MilliValue()},
			Memory:         NodeUsage{Allocatable: alloc.Memory().Value()},
		}
		if u, ok := nodeUsage[n.Name]; ok {
			ns.CPU.Usage, ns.Memory.Usage = &u.cpu, &u.mem
			cpuUsed += u.cpu
			memUsed += u.mem
		}
		o.Nodes = append(o.Nodes, ns)
	}
	if res.MetricsAvailable {
		res.CPU.Usage, res.Memory.Usage = &cpuUsed, &memUsed
	}

	// --- workloads ---
	w := &o.Workloads
	for i := range s.deployments.Items {
		d := &s.deployments.Items[i]
		w.Deployments.Total++
		if d.Status.ReadyReplicas >= desired(d.Spec.Replicas) {
			w.Deployments.Ready++
		}
	}
	for i := range s.statefulSets.Items {
		st := &s.statefulSets.Items[i]
		w.StatefulSets.Total++
		if st.Status.ReadyReplicas >= desired(st.Spec.Replicas) {
			w.StatefulSets.Ready++
		}
	}
	for i := range s.daemonSets.Items {
		ds := &s.daemonSets.Items[i]
		w.DaemonSets.Total++
		if ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled {
			w.DaemonSets.Ready++
		}
	}
	// Old ReplicaSets scaled to zero are rollout history, not live workloads.
	for i := range s.replicaSets.Items {
		if desired(s.replicaSets.Items[i].Spec.Replicas) > 0 {
			w.ReplicaSets++
		}
	}
	for i := range s.jobs.Items {
		j := &s.jobs.Items[i]
		w.Jobs.Total++
		switch {
		case j.Status.Active > 0:
			w.Jobs.Active++
		case jobFailed(j):
			w.Jobs.Failed++
		case j.Status.Succeeded > 0:
			w.Jobs.Succeeded++
		}
	}
	w.CronJobs = len(s.cronJobs.Items)

	// --- namespaces and system pods ---
	o.Namespaces = summarizeNamespaces(s.namespaces.Items, s.pods.Items)

	o.SystemPods = []PodSummary{}
	for i := range s.pods.Items {
		p := &s.pods.Items[i]
		if p.Namespace != systemNamespace {
			continue
		}
		ps := summarizePod(p)
		if u, ok := podUsage[p.Namespace+"/"+p.Name]; ok {
			ps.CPUUsage, ps.MemoryUsage = &u.cpu, &u.mem
		}
		o.SystemPods = append(o.SystemPods, ps)
	}
	sort.Slice(o.SystemPods, func(i, j int) bool { return o.SystemPods[i].Name < o.SystemPods[j].Name })

	// --- network ---
	o.Network = NetworkSummary{
		Services:        len(s.services.Items),
		ServicesByType:  map[string]int{},
		Ingresses:       len(s.ingresses.Items),
		IngressClasses:  len(s.ingressClasses.Items),
		NetworkPolicies: len(s.networkPolicies.Items),
		EndpointSlices:  len(s.endpointSlices.Items),
	}
	for i := range s.services.Items {
		o.Network.ServicesByType[string(s.services.Items[i].Spec.Type)]++
	}

	// --- storage ---
	o.Storage = StorageSummary{
		PersistentVolumes:      len(s.pvs.Items),
		PersistentVolumeClaims: len(s.pvcs.Items),
		StorageClasses:         len(s.storageClasses.Items),
	}
	for i := range s.pvcs.Items {
		pvc := &s.pvcs.Items[i]
		if pvc.Status.Phase == corev1.ClaimBound {
			o.Storage.BoundClaims++
		}
		o.Storage.RequestedBytes += pvc.Spec.Resources.Requests.Storage().Value()
	}

	o.Events = summarizeEvents(s.events.Items, maxEvents)
	return o
}

func summarizeNamespaces(nss []corev1.Namespace, pods []corev1.Pod) []NamespaceSummary {
	perNS := map[string]int{}
	for i := range pods {
		perNS[pods[i].Namespace]++
	}
	out := make([]NamespaceSummary, 0, len(nss))
	for i := range nss {
		ns := &nss[i]
		out = append(out, NamespaceSummary{
			Name:      ns.Name,
			Status:    string(ns.Status.Phase),
			Pods:      perNS[ns.Name],
			CreatedAt: ns.CreationTimestamp.Time,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func summarizePod(p *corev1.Pod) PodSummary {
	ready := 0
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	status := podStatus(p)
	total := len(p.Spec.Containers)
	return PodSummary{
		Name:            p.Name,
		Namespace:       p.Namespace,
		Status:          status,
		Healthy:         status == "Completed" || (status == "Running" && ready == total),
		ReadyContainers: ready,
		Containers:      total,
		Restarts:        podRestarts(p),
		Node:            p.Spec.NodeName,
		IP:              p.Status.PodIP,
		CreatedAt:       p.CreationTimestamp.Time,
	}
}

// podStatus mirrors the STATUS column of `kubectl get pods` closely enough
// for a summary: the most specific container reason wins over the pod phase.
func podStatus(p *corev1.Pod) string {
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, cs := range p.Status.InitContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0 {
			continue
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing" {
			return "Init:" + cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil {
			return "Init:" + orDefault(cs.State.Terminated.Reason, "Error")
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" {
			return w.Reason
		}
		if t := cs.State.Terminated; t != nil && p.Status.Phase != corev1.PodSucceeded {
			return orDefault(t.Reason, "Error")
		}
	}
	if p.Status.Phase == corev1.PodSucceeded {
		return "Completed"
	}
	if p.Status.Reason != "" {
		return p.Status.Reason
	}
	return string(p.Status.Phase)
}

func podRestarts(p *corev1.Pod) int64 {
	var n int64
	for _, cs := range p.Status.ContainerStatuses {
		n += int64(cs.RestartCount)
	}
	return n
}

func summarizeEvents(events []corev1.Event, limit int) []EventSummary {
	out := make([]EventSummary, 0, len(events))
	for i := range events {
		e := &events[i]
		out = append(out, EventSummary{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Namespace: e.Namespace,
			Object:    e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name,
			Count:     max(e.Count, 1),
			LastSeen:  eventTime(e),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// eventTime picks the most recent timestamp an event carries; which fields
// are set depends on the component that emitted it.
func eventTime(e *corev1.Event) time.Time {
	switch {
	case e.Series != nil && !e.Series.LastObservedTime.IsZero():
		return e.Series.LastObservedTime.Time
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	case !e.EventTime.IsZero():
		return e.EventTime.Time
	default:
		return e.CreationTimestamp.Time
	}
}

func nodeReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func nodeRoles(n *corev1.Node) []string {
	const prefix = "node-role.kubernetes.io/"
	roles := []string{}
	for label := range n.Labels {
		if role, ok := strings.CutPrefix(label, prefix); ok && role != "" {
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	return roles
}

func nodeInternalIP(n *corev1.Node) string {
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeInternalIP {
			return a.Address
		}
	}
	return ""
}

func jobFailed(j *batchv1.Job) bool {
	for _, c := range j.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// desired resolves an optional replica count, which defaults to 1.
func desired(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
