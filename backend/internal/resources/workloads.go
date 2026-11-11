package resources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
)

// ErrInvalid marks a request the caller got wrong (bad kind, bad replica
// count). The API reports it as 400 rather than as a cluster failure.
var ErrInvalid = errors.New("invalid request")

// MaxReplicas caps what a scale request may ask for, as a guard against
// typos such as 1000 for 10.
const MaxReplicas = 500

// restartAnnotation is the pod-template annotation `kubectl rollout restart`
// sets; changing it makes the controller roll every pod.
const restartAnnotation = "kubectl.kubernetes.io/restartedAt"

type WorkloadKind string

const (
	KindDeployment  WorkloadKind = "Deployment"
	KindStatefulSet WorkloadKind = "StatefulSet"
	KindDaemonSet   WorkloadKind = "DaemonSet"
)

// ParseWorkloadKind maps the URL form ("deployments") to a kind.
func ParseWorkloadKind(s string) (WorkloadKind, error) {
	switch s {
	case "deployments":
		return KindDeployment, nil
	case "statefulsets":
		return KindStatefulSet, nil
	case "daemonsets":
		return KindDaemonSet, nil
	}
	return "", fmt.Errorf("%w: unknown workload kind %q", ErrInvalid, s)
}

type WorkloadSummary struct {
	Kind      WorkloadKind `json:"kind"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
	// Desired is the replica count asked for; for a DaemonSet, the number of
	// nodes it should run on.
	Desired   int32     `json:"desired"`
	Ready     int32     `json:"ready"`
	Updated   int32     `json:"updated"`
	Available int32     `json:"available"`
	Healthy   bool      `json:"healthy"`
	Scalable  bool      `json:"scalable"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"createdAt"`
}

type WorkloadDetail struct {
	WorkloadSummary
	Strategy   string            `json:"strategy"`
	Selector   map[string]string `json:"selector"`
	Labels     map[string]string `json:"labels"`
	Conditions []PodCondition    `json:"conditions"`
	Pods       []PodSummary      `json:"pods"`
	Events     []EventSummary    `json:"events"`
}

// Workloads lists Deployments, StatefulSets and DaemonSets in one namespace,
// or in all of them when namespace is empty.
func (s *Service) Workloads(ctx context.Context, namespace string) ([]WorkloadSummary, error) {
	var (
		apps = s.kube.Core.AppsV1()
		opts = metav1.ListOptions{}
		deps *appsv1.DeploymentList
		sts  *appsv1.StatefulSetList
		dss  *appsv1.DaemonSetList
	)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { deps, err = apps.Deployments(namespace).List(ctx, opts); return })
	g.Go(func() (err error) { sts, err = apps.StatefulSets(namespace).List(ctx, opts); return })
	g.Go(func() (err error) { dss, err = apps.DaemonSets(namespace).List(ctx, opts); return })
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("listing workloads: %w", err)
	}

	out := make([]WorkloadSummary, 0, len(deps.Items)+len(sts.Items)+len(dss.Items))
	for i := range deps.Items {
		out = append(out, summarizeDeployment(&deps.Items[i]))
	}
	for i := range sts.Items {
		out = append(out, summarizeStatefulSet(&sts.Items[i]))
	}
	for i := range dss.Items {
		out = append(out, summarizeDaemonSet(&dss.Items[i]))
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Kind < b.Kind
	})
	return out, nil
}

// Workload returns one workload with the pods it owns and its recent events.
func (s *Service) Workload(ctx context.Context, kind WorkloadKind, namespace, name string) (*WorkloadDetail, error) {
	apps := s.kube.Core.AppsV1()
	var (
		d        WorkloadDetail
		selector *metav1.LabelSelector
	)
	switch kind {
	case KindDeployment:
		o, err := apps.Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting deployment %s/%s: %w", namespace, name, err)
		}
		d.WorkloadSummary, d.Labels, selector = summarizeDeployment(o), o.Labels, o.Spec.Selector
		d.Strategy = string(o.Spec.Strategy.Type)
		for _, c := range o.Status.Conditions {
			d.Conditions = append(d.Conditions, PodCondition{string(c.Type), string(c.Status), c.Reason, c.Message})
		}
	case KindStatefulSet:
		o, err := apps.StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting statefulset %s/%s: %w", namespace, name, err)
		}
		d.WorkloadSummary, d.Labels, selector = summarizeStatefulSet(o), o.Labels, o.Spec.Selector
		d.Strategy = string(o.Spec.UpdateStrategy.Type)
		for _, c := range o.Status.Conditions {
			d.Conditions = append(d.Conditions, PodCondition{string(c.Type), string(c.Status), c.Reason, c.Message})
		}
	case KindDaemonSet:
		o, err := apps.DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting daemonset %s/%s: %w", namespace, name, err)
		}
		d.WorkloadSummary, d.Labels, selector = summarizeDaemonSet(o), o.Labels, o.Spec.Selector
		d.Strategy = string(o.Spec.UpdateStrategy.Type)
		for _, c := range o.Status.Conditions {
			d.Conditions = append(d.Conditions, PodCondition{string(c.Type), string(c.Status), c.Reason, c.Message})
		}
	default:
		return nil, fmt.Errorf("%w: unknown workload kind %q", ErrInvalid, kind)
	}

	// Never serialize nil collections: the frontend iterates them.
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	if d.Conditions == nil {
		d.Conditions = []PodCondition{}
	}
	d.Selector, d.Pods, d.Events = map[string]string{}, []PodSummary{}, []EventSummary{}

	core := s.kube.Core.CoreV1()
	if selector != nil {
		d.Selector = selector.MatchLabels
		sel, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return nil, fmt.Errorf("reading selector of %s/%s: %w", namespace, name, err)
		}
		pods, err := core.Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
		if err != nil {
			return nil, fmt.Errorf("listing pods of %s/%s: %w", namespace, name, err)
		}
		usage := s.podUsage(ctx, namespace)
		for i := range pods.Items {
			d.Pods = append(d.Pods, withUsage(summarizePod(&pods.Items[i]), usage))
		}
		sort.Slice(d.Pods, func(i, j int) bool { return d.Pods[i].Name < d.Pods[j].Name })
	}

	// Events help troubleshooting but must not hide the workload itself.
	fs := fields.Set{"involvedObject.kind": string(kind), "involvedObject.name": name}.AsSelector().String()
	if events, err := core.Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: fs}); err == nil {
		d.Events = summarizeEvents(events.Items, maxEvents)
	}
	return &d, nil
}

// ScaleWorkload sets the replica count of a Deployment or StatefulSet.
func (s *Service) ScaleWorkload(ctx context.Context, kind WorkloadKind, namespace, name string, replicas int32) error {
	if replicas < 0 || replicas > MaxReplicas {
		return fmt.Errorf("%w: replicas must be between 0 and %d", ErrInvalid, MaxReplicas)
	}
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
	apps := s.kube.Core.AppsV1()
	var err error
	switch kind {
	case KindDeployment:
		_, err = apps.Deployments(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	case KindStatefulSet:
		_, err = apps.StatefulSets(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	case KindDaemonSet:
		return fmt.Errorf("%w: a DaemonSet runs one pod per node and cannot be scaled", ErrInvalid)
	default:
		return fmt.Errorf("%w: unknown workload kind %q", ErrInvalid, kind)
	}
	if err != nil {
		return fmt.Errorf("scaling %s %s/%s: %w", kind, namespace, name, err)
	}
	return nil
}

// RestartWorkload replaces every pod of a workload following its update
// strategy, the same way `kubectl rollout restart` does.
func (s *Service) RestartWorkload(ctx context.Context, kind WorkloadKind, namespace, name string) error {
	patch := []byte(fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		restartAnnotation, time.Now().UTC().Format(time.RFC3339),
	))
	apps := s.kube.Core.AppsV1()
	opts := metav1.PatchOptions{}
	var err error
	switch kind {
	case KindDeployment:
		_, err = apps.Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, patch, opts)
	case KindStatefulSet:
		_, err = apps.StatefulSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, patch, opts)
	case KindDaemonSet:
		_, err = apps.DaemonSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, patch, opts)
	default:
		return fmt.Errorf("%w: unknown workload kind %q", ErrInvalid, kind)
	}
	if err != nil {
		return fmt.Errorf("restarting %s %s/%s: %w", kind, namespace, name, err)
	}
	return nil
}

func summarizeDeployment(d *appsv1.Deployment) WorkloadSummary {
	want := desired(d.Spec.Replicas)
	return WorkloadSummary{
		Kind: KindDeployment, Name: d.Name, Namespace: d.Namespace,
		Desired: want, Ready: d.Status.ReadyReplicas,
		Updated: d.Status.UpdatedReplicas, Available: d.Status.AvailableReplicas,
		Healthy: d.Status.ReadyReplicas >= want, Scalable: true,
		Images: images(&d.Spec.Template.Spec), CreatedAt: d.CreationTimestamp.Time,
	}
}

func summarizeStatefulSet(s *appsv1.StatefulSet) WorkloadSummary {
	want := desired(s.Spec.Replicas)
	return WorkloadSummary{
		Kind: KindStatefulSet, Name: s.Name, Namespace: s.Namespace,
		Desired: want, Ready: s.Status.ReadyReplicas,
		Updated: s.Status.UpdatedReplicas, Available: s.Status.AvailableReplicas,
		Healthy: s.Status.ReadyReplicas >= want, Scalable: true,
		Images: images(&s.Spec.Template.Spec), CreatedAt: s.CreationTimestamp.Time,
	}
}

func summarizeDaemonSet(d *appsv1.DaemonSet) WorkloadSummary {
	want := d.Status.DesiredNumberScheduled
	return WorkloadSummary{
		Kind: KindDaemonSet, Name: d.Name, Namespace: d.Namespace,
		Desired: want, Ready: d.Status.NumberReady,
		Updated: d.Status.UpdatedNumberScheduled, Available: d.Status.NumberAvailable,
		Healthy: d.Status.NumberReady >= want, Scalable: false,
		Images: images(&d.Spec.Template.Spec), CreatedAt: d.CreationTimestamp.Time,
	}
}

func images(spec *corev1.PodSpec) []string {
	out := make([]string, 0, len(spec.Containers))
	for i := range spec.Containers {
		out = append(out, spec.Containers[i].Image)
	}
	return out
}
