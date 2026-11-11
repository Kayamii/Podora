package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/Kayamii/Podora/backend/internal/kubernetes"
)

func newTestService(objects ...runtime.Object) *Service {
	return NewService(&kubernetes.Client{
		Core:    k8sfake.NewSimpleClientset(objects...),
		Metrics: metricsfake.NewSimpleClientset(),
	})
}

func testDeployment(replicas, ready int32) *appsv1.Deployment {
	labels := map[string]string{"app": "web"}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "web"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.29"}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: ready},
	}
}

func TestWorkloadsAndDetail(t *testing.T) {
	ctx := context.Background()
	own := pod("demo", "web-1", "n", corev1.PodRunning, "1m", 0, true)
	own.Labels = map[string]string{"app": "web"}
	other := pod("demo", "other", "n", corev1.PodRunning, "1m", 0, true)
	other.Labels = map[string]string{"app": "other"}
	s := newTestService(testDeployment(2, 1), &own, &other)

	list, err := s.Workloads(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Kind != KindDeployment || list[0].Healthy || !list[0].Scalable {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Images[0] != "nginx:1.29" {
		t.Errorf("images = %v", list[0].Images)
	}

	d, err := s.Workload(ctx, KindDeployment, "demo", "web")
	if err != nil {
		t.Fatal(err)
	}
	// Only pods matching the selector belong to the workload.
	if len(d.Pods) != 1 || d.Pods[0].Name != "web-1" {
		t.Errorf("pods = %+v", d.Pods)
	}
	if d.Selector["app"] != "web" {
		t.Errorf("selector = %v", d.Selector)
	}

	if _, err := s.Workload(ctx, KindDeployment, "demo", "missing"); !apierrors.IsNotFound(err) {
		t.Errorf("missing workload: err = %v, want not found", err)
	}
}

func TestScaleWorkload(t *testing.T) {
	ctx := context.Background()
	s := newTestService(testDeployment(2, 2))

	if err := s.ScaleWorkload(ctx, KindDeployment, "demo", "web", 5); err != nil {
		t.Fatal(err)
	}
	got, _ := s.kube.Core.AppsV1().Deployments("demo").Get(ctx, "web", metav1.GetOptions{})
	if *got.Spec.Replicas != 5 {
		t.Errorf("replicas = %d, want 5", *got.Spec.Replicas)
	}

	for _, bad := range []int32{-1, MaxReplicas + 1} {
		if err := s.ScaleWorkload(ctx, KindDeployment, "demo", "web", bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("replicas %d: err = %v, want ErrInvalid", bad, err)
		}
	}
	if err := s.ScaleWorkload(ctx, KindDaemonSet, "demo", "web", 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("daemonset: err = %v, want ErrInvalid", err)
	}
}

func TestRestartWorkload(t *testing.T) {
	ctx := context.Background()
	s := newTestService(testDeployment(2, 2))

	if err := s.RestartWorkload(ctx, KindDeployment, "demo", "web"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.kube.Core.AppsV1().Deployments("demo").Get(ctx, "web", metav1.GetOptions{})
	if got.Spec.Template.Annotations[restartAnnotation] == "" {
		t.Errorf("restart annotation not set: %v", got.Spec.Template.Annotations)
	}
	// The rest of the template must survive the patch.
	if got.Spec.Template.Labels["app"] != "web" || len(got.Spec.Template.Spec.Containers) != 1 {
		t.Errorf("template damaged: %+v", got.Spec.Template)
	}
}

func TestDeletePod(t *testing.T) {
	ctx := context.Background()
	p := pod("demo", "web-1", "n", corev1.PodRunning, "1m", 0, true)
	s := newTestService(&p)

	if err := s.DeletePod(ctx, "demo", "web-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.kube.Core.CoreV1().Pods("demo").Get(ctx, "web-1", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("pod still exists: err = %v", err)
	}
	if err := s.DeletePod(ctx, "demo", "web-1"); !apierrors.IsNotFound(err) {
		t.Errorf("second delete: err = %v, want not found", err)
	}
}

func TestSummarizeService(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "web"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.0.0.1",
			Ports:     []corev1.ServicePort{{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP}},
		},
	}
	got := summarizeService(svc)
	if len(got.Ports) != 1 || got.Ports[0] != "80:30080/TCP → 0" {
		t.Errorf("ports = %v", got.Ports)
	}
	if got.Selector == nil || got.External == nil {
		t.Errorf("nil collections: %+v", got)
	}
}

func TestNamespacesUsage(t *testing.T) {
	running := pod("demo", "web-1", "n", corev1.PodRunning, "250m", 0, true)
	done := pod("demo", "job-1", "n", corev1.PodSucceeded, "1", 0, false)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
	s := newTestService(ns, &running, &done)

	got, err := s.NamespacesUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Pods != 2 || got[0].RunningPods != 1 {
		t.Fatalf("usage = %+v", got)
	}
	// The finished pod's request no longer counts.
	if got[0].CPURequests != 250 {
		t.Errorf("cpu requests = %d, want 250", got[0].CPURequests)
	}
	// No metrics-server in the fake: usage is unknown, not zero.
	if got[0].CPUUsage != nil {
		t.Errorf("cpu usage = %v, want nil", *got[0].CPUUsage)
	}
}

func TestSeriesStoreForgetsDeletedKeys(t *testing.T) {
	var st seriesStore
	now := time.Now()
	st.record(now, map[string]usage{"pod:demo/a": {cpu: 1}, "pod:demo/b": {cpu: 2}})
	st.record(now.Add(time.Second), map[string]usage{"pod:demo/a": {cpu: 3}})
	if got := st.get("pod:demo/a"); len(got) != 2 || got[1].CPU != 3 {
		t.Errorf("a = %+v", got)
	}
	if got := st.get("pod:demo/b"); len(got) != 0 {
		t.Errorf("deleted pod still tracked: %+v", got)
	}
}
