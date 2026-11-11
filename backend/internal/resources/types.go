package resources

import "time"

// Overview is the cluster-wide summary shown on the dashboard.
type Overview struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	Cluster     ClusterInfo        `json:"cluster"`
	Resources   ResourceSummary    `json:"resources"`
	Nodes       []NodeSummary      `json:"nodes"`
	Pods        PodCounts          `json:"pods"`
	Workloads   Workloads          `json:"workloads"`
	Namespaces  []NamespaceSummary `json:"namespaces"`
	SystemPods  []PodSummary       `json:"systemPods"`
	Network     NetworkSummary     `json:"network"`
	Storage     StorageSummary     `json:"storage"`
	Events      []EventSummary     `json:"events"`
}

type ClusterInfo struct {
	Version    string `json:"version"`
	Platform   string `json:"platform"`
	NodesTotal int    `json:"nodesTotal"`
	NodesReady int    `json:"nodesReady"`
}

// ResourceSummary aggregates compute across all nodes.
type ResourceSummary struct {
	// MetricsAvailable is false when metrics-server is missing or not ready;
	// Usage fields are then null.
	MetricsAvailable bool        `json:"metricsAvailable"`
	CPU              Measurement `json:"cpu"`    // millicores
	Memory           Measurement `json:"memory"` // bytes
	PodCapacity      int64       `json:"podCapacity"`
}

// Measurement describes one resource. CPU values are millicores, memory bytes.
type Measurement struct {
	Capacity    int64  `json:"capacity"`
	Allocatable int64  `json:"allocatable"`
	Requests    int64  `json:"requests"`
	Limits      int64  `json:"limits"`
	Usage       *int64 `json:"usage"`
}

type NodeSummary struct {
	Name           string    `json:"name"`
	Ready          bool      `json:"ready"`
	Schedulable    bool      `json:"schedulable"`
	Roles          []string  `json:"roles"`
	KubeletVersion string    `json:"kubeletVersion"`
	OSImage        string    `json:"osImage"`
	InternalIP     string    `json:"internalIP"`
	CreatedAt      time.Time `json:"createdAt"`
	Pods           int       `json:"pods"`
	PodCapacity    int64     `json:"podCapacity"`
	CPU            NodeUsage `json:"cpu"`    // millicores
	Memory         NodeUsage `json:"memory"` // bytes
}

type NodeUsage struct {
	Allocatable int64  `json:"allocatable"`
	Usage       *int64 `json:"usage"`
}

type PodCounts struct {
	Total     int   `json:"total"`
	Running   int   `json:"running"`
	Pending   int   `json:"pending"`
	Succeeded int   `json:"succeeded"`
	Failed    int   `json:"failed"`
	Unknown   int   `json:"unknown"`
	Restarts  int64 `json:"restarts"`
}

// ReadyCount is a "ready out of total" pair.
type ReadyCount struct {
	Total int `json:"total"`
	Ready int `json:"ready"`
}

type Workloads struct {
	Deployments  ReadyCount `json:"deployments"`
	StatefulSets ReadyCount `json:"statefulSets"`
	DaemonSets   ReadyCount `json:"daemonSets"`
	ReplicaSets  int        `json:"replicaSets"`
	Jobs         JobCounts  `json:"jobs"`
	CronJobs     int        `json:"cronJobs"`
}

type JobCounts struct {
	Total     int `json:"total"`
	Active    int `json:"active"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type NamespaceSummary struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Pods      int       `json:"pods"`
	CreatedAt time.Time `json:"createdAt"`
}

type PodSummary struct {
	Name            string    `json:"name"`
	Namespace       string    `json:"namespace"`
	Status          string    `json:"status"`
	Healthy         bool      `json:"healthy"`
	ReadyContainers int       `json:"readyContainers"`
	Containers      int       `json:"containers"`
	Restarts        int64     `json:"restarts"`
	Node            string    `json:"node"`
	IP              string    `json:"ip"`
	CreatedAt       time.Time `json:"createdAt"`
	CPUUsage        *int64    `json:"cpuUsage"`    // millicores
	MemoryUsage     *int64    `json:"memoryUsage"` // bytes
}

type NetworkSummary struct {
	Services        int            `json:"services"`
	ServicesByType  map[string]int `json:"servicesByType"`
	Ingresses       int            `json:"ingresses"`
	IngressClasses  int            `json:"ingressClasses"`
	NetworkPolicies int            `json:"networkPolicies"`
	EndpointSlices  int            `json:"endpointSlices"`
}

type StorageSummary struct {
	PersistentVolumes      int   `json:"persistentVolumes"`
	PersistentVolumeClaims int   `json:"persistentVolumeClaims"`
	BoundClaims            int   `json:"boundClaims"`
	StorageClasses         int   `json:"storageClasses"`
	RequestedBytes         int64 `json:"requestedBytes"`
}

type EventSummary struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Namespace string    `json:"namespace"`
	Object    string    `json:"object"`
	Count     int32     `json:"count"`
	LastSeen  time.Time `json:"lastSeen"`
}
