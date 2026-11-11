package resources

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func emptySnapshot() *snapshot {
	return &snapshot{
		version:         &version.Info{GitVersion: "v1.35.0"},
		nodes:           &corev1.NodeList{},
		pods:            &corev1.PodList{},
		namespaces:      &corev1.NamespaceList{},
		services:        &corev1.ServiceList{},
		events:          &corev1.EventList{},
		pvs:             &corev1.PersistentVolumeList{},
		pvcs:            &corev1.PersistentVolumeClaimList{},
		deployments:     &appsv1.DeploymentList{},
		statefulSets:    &appsv1.StatefulSetList{},
		daemonSets:      &appsv1.DaemonSetList{},
		replicaSets:     &appsv1.ReplicaSetList{},
		jobs:            &batchv1.JobList{},
		cronJobs:        &batchv1.CronJobList{},
		ingresses:       &networkingv1.IngressList{},
		ingressClasses:  &networkingv1.IngressClassList{},
		networkPolicies: &networkingv1.NetworkPolicyList{},
		endpointSlices:  &discoveryv1.EndpointSliceList{},
		storageClasses:  &storagev1.StorageClassList{},
	}
}

func rl(cpu, mem string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
		corev1.ResourcePods:   resource.MustParse("110"),
	}
}

func pod(ns, name, node string, phase corev1.PodPhase, cpuReq string, restarts int32, ready bool) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: corev1.PodSpec{
			NodeName: node,
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpuReq)},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase:             phase,
			ContainerStatuses: []corev1.ContainerStatus{{Name: "c", Ready: ready, RestartCount: restarts}},
		},
	}
}

func TestSummarize(t *testing.T) {
	s := emptySnapshot()
	s.nodes.Items = []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
		},
		Status: corev1.NodeStatus{
			Capacity:    rl("4", "8Gi"),
			Allocatable: rl("4", "8Gi"),
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}}
	s.pods.Items = []corev1.Pod{
		pod("kube-system", "coredns", "node-a", corev1.PodRunning, "100m", 2, true),
		pod("default", "web", "node-a", corev1.PodRunning, "250m", 0, true),
		pod("default", "stuck", "", corev1.PodPending, "500m", 0, false),
		// A finished pod must not count towards requests or the node's pods.
		pod("default", "done", "node-a", corev1.PodSucceeded, "1", 0, false),
	}
	s.namespaces.Items = []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
	}
	one, zero := int32(1), int32(0)
	s.deployments.Items = []appsv1.Deployment{
		{Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
		{Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ReadyReplicas: 0}},
	}
	s.replicaSets.Items = []appsv1.ReplicaSet{
		{Spec: appsv1.ReplicaSetSpec{Replicas: &one}},
		{Spec: appsv1.ReplicaSetSpec{Replicas: &zero}}, // rollout history
	}
	s.services.Items = []corev1.Service{
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}},
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}},
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort}},
	}

	o := summarize(s, time.Now())

	if o.Cluster.NodesTotal != 1 || o.Cluster.NodesReady != 1 {
		t.Errorf("nodes = %d/%d ready, want 1/1", o.Cluster.NodesReady, o.Cluster.NodesTotal)
	}
	if got := o.Pods; got.Total != 4 || got.Running != 2 || got.Pending != 1 || got.Succeeded != 1 || got.Restarts != 2 {
		t.Errorf("pod counts = %+v", got)
	}
	if got, want := o.Resources.CPU.Requests, int64(850); got != want {
		t.Errorf("cpu requests = %dm, want %dm", got, want)
	}
	if got, want := o.Resources.CPU.Allocatable, int64(4000); got != want {
		t.Errorf("cpu allocatable = %dm, want %dm", got, want)
	}
	if o.Resources.MetricsAvailable || o.Resources.CPU.Usage != nil {
		t.Errorf("usage reported without metrics")
	}
	if got := o.Nodes[0]; got.Pods != 2 || len(got.Roles) != 1 || got.Roles[0] != "control-plane" {
		t.Errorf("node summary = %+v", got)
	}
	if got := o.Workloads; got.Deployments != (ReadyCount{Total: 2, Ready: 1}) || got.ReplicaSets != 1 {
		t.Errorf("workloads = %+v", got)
	}
	if len(o.SystemPods) != 1 || o.SystemPods[0].Name != "coredns" || !o.SystemPods[0].Healthy {
		t.Errorf("system pods = %+v", o.SystemPods)
	}
	if got := o.Network; got.Services != 3 || got.ServicesByType["ClusterIP"] != 2 || got.ServicesByType["NodePort"] != 1 {
		t.Errorf("network = %+v", got)
	}
	// Namespaces are sorted by name and carry pod counts.
	if len(o.Namespaces) != 2 || o.Namespaces[0].Name != "default" || o.Namespaces[0].Pods != 3 {
		t.Errorf("namespaces = %+v", o.Namespaces)
	}
}

func TestSummarizeWithMetrics(t *testing.T) {
	s := emptySnapshot()
	s.nodes.Items = []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     corev1.NodeStatus{Capacity: rl("4", "8Gi"), Allocatable: rl("4", "8Gi")},
	}}
	s.pods.Items = []corev1.Pod{pod("kube-system", "coredns", "node-a", corev1.PodRunning, "100m", 0, true)}
	s.nodeMetrics = &metricsv1beta1.NodeMetricsList{Items: []metricsv1beta1.NodeMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Usage:      rl("500m", "2Gi"),
	}}}
	s.podMetrics = &metricsv1beta1.PodMetricsList{Items: []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns"},
		Containers: []metricsv1beta1.ContainerMetrics{{Name: "c", Usage: rl("3m", "20Mi")}},
	}}}

	o := summarize(s, time.Now())

	if !o.Resources.MetricsAvailable {
		t.Fatal("metrics not reported as available")
	}
	if got := *o.Resources.CPU.Usage; got != 500 {
		t.Errorf("cpu usage = %dm, want 500m", got)
	}
	if got := *o.Resources.Memory.Usage; got != 2<<30 {
		t.Errorf("memory usage = %d, want %d", got, 2<<30)
	}
	if got := o.SystemPods[0]; got.CPUUsage == nil || *got.CPUUsage != 3 || *got.MemoryUsage != 20<<20 {
		t.Errorf("pod usage = %+v", got)
	}
}

func TestPodStatus(t *testing.T) {
	crash := pod("default", "crash", "n", corev1.PodRunning, "1m", 5, false)
	crash.Status.ContainerStatuses[0].State.Waiting = &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}

	done := pod("default", "done", "n", corev1.PodSucceeded, "1m", 0, false)
	done.Status.ContainerStatuses[0].State.Terminated = &corev1.ContainerStateTerminated{Reason: "Completed"}

	now := metav1.Now()
	terminating := pod("default", "bye", "n", corev1.PodRunning, "1m", 0, true)
	terminating.DeletionTimestamp = &now

	cases := []struct {
		pod     corev1.Pod
		status  string
		healthy bool
	}{
		{pod("default", "ok", "n", corev1.PodRunning, "1m", 0, true), "Running", true},
		{pod("default", "notready", "n", corev1.PodRunning, "1m", 0, false), "Running", false},
		{crash, "CrashLoopBackOff", false},
		{done, "Completed", true},
		{terminating, "Terminating", false},
		{pod("default", "pending", "", corev1.PodPending, "1m", 0, false), "Pending", false},
	}
	for _, tc := range cases {
		got := summarizePod(&tc.pod)
		if got.Status != tc.status || got.Healthy != tc.healthy {
			t.Errorf("%s: status = %q healthy = %v, want %q %v", tc.pod.Name, got.Status, got.Healthy, tc.status, tc.healthy)
		}
	}
}

func TestHistoryIsBounded(t *testing.T) {
	var h history
	for i := 0; i < maxSamples+25; i++ {
		h.add(Sample{CPUUsage: int64(i)})
	}
	got := h.snapshot()
	if len(got) != maxSamples {
		t.Fatalf("kept %d samples, want %d", len(got), maxSamples)
	}
	// The oldest samples are dropped, order is preserved.
	if got[0].CPUUsage != 25 || got[len(got)-1].CPUUsage != int64(maxSamples+24) {
		t.Errorf("range = %d..%d", got[0].CPUUsage, got[len(got)-1].CPUUsage)
	}
}
