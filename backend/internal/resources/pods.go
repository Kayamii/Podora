package resources

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// MaxLogTailLines caps how much log history one request may ask for.
const MaxLogTailLines = 5000

// PodDetail is everything shown on a pod's page.
type PodDetail struct {
	PodSummary
	Phase          string             `json:"phase"`
	QoSClass       string             `json:"qosClass"`
	ServiceAccount string             `json:"serviceAccount"`
	HostIP         string             `json:"hostIP"`
	Owner          string             `json:"owner"` // e.g. "ReplicaSet/web-5c7cbdcd5f", empty if none
	Labels         map[string]string  `json:"labels"`
	Conditions     []PodCondition     `json:"conditions"`
	Containers     []ContainerSummary `json:"containers"`
	Events         []EventSummary     `json:"events"`
}

type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type ContainerSummary struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Init  bool   `json:"init"`
	// State is "Running", "Waiting", "Terminated" or "Unknown".
	State     string     `json:"state"`
	Reason    string     `json:"reason"`
	Message   string     `json:"message"`
	Ready     bool       `json:"ready"`
	Restarts  int32      `json:"restarts"`
	StartedAt *time.Time `json:"startedAt"`
	// LastTermination describes the previous run, when the container has
	// restarted. Its logs are what the "previous" log option returns.
	LastTermination *Termination `json:"lastTermination"`
	Ports           []string     `json:"ports"`
	// Current usage; null without metrics-server.
	CPUUsage      *int64 `json:"cpuUsage"`
	MemoryUsage   *int64 `json:"memoryUsage"`
	CPURequest    int64  `json:"cpuRequest"`    // millicores
	CPULimit      int64  `json:"cpuLimit"`      // millicores
	MemoryRequest int64  `json:"memoryRequest"` // bytes
	MemoryLimit   int64  `json:"memoryLimit"`   // bytes
}

type Termination struct {
	Reason     string    `json:"reason"`
	ExitCode   int32     `json:"exitCode"`
	FinishedAt time.Time `json:"finishedAt"`
}

// LogOptions selects which part of a container's log to read.
type LogOptions struct {
	// Container may be empty when the pod has a single container.
	Container string
	// Previous reads the log of the container's previous run instead of the
	// current one.
	Previous bool
	// Follow keeps the stream open and delivers new lines as they are written.
	Follow bool
	// TailLines is how many of the most recent lines to start with.
	TailLines  int64
	Timestamps bool
}

// Pods lists pods in one namespace, or in all of them when namespace is empty.
func (s *Service) Pods(ctx context.Context, namespace string) ([]PodSummary, error) {
	pods, err := s.kube.Core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	usage := s.podUsage(ctx, namespace)
	out := make([]PodSummary, 0, len(pods.Items))
	for i := range pods.Items {
		out = append(out, withUsage(summarizePod(&pods.Items[i]), usage))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Pod returns one pod with its containers and recent events.
func (s *Service) Pod(ctx context.Context, namespace, name string) (*PodDetail, error) {
	core := s.kube.Core.CoreV1()
	p, err := core.Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod %s/%s: %w", namespace, name, err)
	}

	d := describePod(p)

	if m, err := s.kube.Metrics.MetricsV1beta1().PodMetricses(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		var cpu, mem int64
		for _, c := range m.Containers {
			ccpu, cmem := c.Usage.Cpu().MilliValue(), c.Usage.Memory().Value()
			cpu += ccpu
			mem += cmem
			for i := range d.Containers {
				if d.Containers[i].Name == c.Name && !d.Containers[i].Init {
					d.Containers[i].CPUUsage, d.Containers[i].MemoryUsage = &ccpu, &cmem
				}
			}
		}
		d.CPUUsage, d.MemoryUsage = &cpu, &mem
	}

	// Events are a troubleshooting aid; failing to read them should not hide
	// the pod itself.
	selector := fields.Set{"involvedObject.kind": "Pod", "involvedObject.name": name}.AsSelector().String()
	if events, err := core.Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: selector}); err == nil {
		d.Events = summarizeEvents(events.Items, maxEvents)
	}
	return d, nil
}

// DeletePod deletes a pod. If a workload owns it, the workload starts a
// replacement, which makes this the way to restart a single pod.
func (s *Service) DeletePod(ctx context.Context, namespace, name string) error {
	if err := s.kube.Core.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting pod %s/%s: %w", namespace, name, err)
	}
	return nil
}

type usage struct{ cpu, mem int64 }

// podUsage reads current pod usage, keyed by "namespace/name". It returns an
// empty map when metrics-server is unavailable: usage is optional everywhere.
func (s *Service) podUsage(ctx context.Context, namespace string) map[string]usage {
	out := map[string]usage{}
	m, err := s.kube.Metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for _, pm := range m.Items {
		var u usage
		for _, c := range pm.Containers {
			u.cpu += c.Usage.Cpu().MilliValue()
			u.mem += c.Usage.Memory().Value()
		}
		out[pm.Namespace+"/"+pm.Name] = u
	}
	return out
}

func withUsage(p PodSummary, byPod map[string]usage) PodSummary {
	if u, ok := byPod[p.Namespace+"/"+p.Name]; ok {
		p.CPUUsage, p.MemoryUsage = &u.cpu, &u.mem
	}
	return p
}

// PodLogs opens a container's log. The caller must close the stream. With
// Follow set, the stream stays open until ctx is cancelled or the container
// stops.
func (s *Service) PodLogs(ctx context.Context, namespace, name string, opts LogOptions) (io.ReadCloser, error) {
	tail := min(max(opts.TailLines, 1), MaxLogTailLines)
	stream, err := s.kube.Core.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		Container: opts.Container,
		Previous:  opts.Previous,
		// A previous run has ended: there is nothing to follow.
		Follow:     opts.Follow && !opts.Previous,
		TailLines:  &tail,
		Timestamps: opts.Timestamps,
	}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening logs for %s/%s: %w", namespace, name, err)
	}
	return stream, nil
}

func describePod(p *corev1.Pod) *PodDetail {
	d := &PodDetail{
		PodSummary:     summarizePod(p),
		Phase:          string(p.Status.Phase),
		QoSClass:       string(p.Status.QOSClass),
		ServiceAccount: p.Spec.ServiceAccountName,
		HostIP:         p.Status.HostIP,
		Labels:         p.Labels,
		Conditions:     []PodCondition{},
		Containers:     []ContainerSummary{},
		Events:         []EventSummary{},
	}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	if owner := metav1.GetControllerOf(p); owner != nil {
		d.Owner = owner.Kind + "/" + owner.Name
	}
	for _, c := range p.Status.Conditions {
		d.Conditions = append(d.Conditions, PodCondition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	statuses := map[string]corev1.ContainerStatus{}
	for _, cs := range p.Status.InitContainerStatuses {
		statuses["init/"+cs.Name] = cs
	}
	for _, cs := range p.Status.ContainerStatuses {
		statuses["main/"+cs.Name] = cs
	}
	for i := range p.Spec.InitContainers {
		d.Containers = append(d.Containers, describeContainer(&p.Spec.InitContainers[i], statuses, true))
	}
	for i := range p.Spec.Containers {
		d.Containers = append(d.Containers, describeContainer(&p.Spec.Containers[i], statuses, false))
	}
	return d
}

func describeContainer(c *corev1.Container, statuses map[string]corev1.ContainerStatus, init bool) ContainerSummary {
	out := ContainerSummary{
		Name:          c.Name,
		Image:         c.Image,
		Init:          init,
		State:         "Unknown",
		Ports:         []string{},
		CPURequest:    c.Resources.Requests.Cpu().MilliValue(),
		CPULimit:      c.Resources.Limits.Cpu().MilliValue(),
		MemoryRequest: c.Resources.Requests.Memory().Value(),
		MemoryLimit:   c.Resources.Limits.Memory().Value(),
	}
	for _, port := range c.Ports {
		out.Ports = append(out.Ports, fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol))
	}

	key := "main/" + c.Name
	if init {
		key = "init/" + c.Name
	}
	cs, ok := statuses[key]
	if !ok {
		return out
	}
	out.Ready = cs.Ready
	out.Restarts = cs.RestartCount
	switch {
	case cs.State.Running != nil:
		out.State = "Running"
		started := cs.State.Running.StartedAt.Time
		out.StartedAt = &started
	case cs.State.Waiting != nil:
		out.State = "Waiting"
		out.Reason = cs.State.Waiting.Reason
		out.Message = cs.State.Waiting.Message
	case cs.State.Terminated != nil:
		out.State = "Terminated"
		out.Reason = cs.State.Terminated.Reason
		out.Message = cs.State.Terminated.Message
	}
	if t := cs.LastTerminationState.Terminated; t != nil {
		out.LastTermination = &Termination{
			Reason:     orDefault(t.Reason, "Error"),
			ExitCode:   t.ExitCode,
			FinishedAt: t.FinishedAt.Time,
		}
	}
	return out
}
