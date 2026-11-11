package kubernetes

import (
	"fmt"

	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Client bundles the API clients Podora uses to talk to the cluster.
type Client struct {
	// Core is the typed client for built-in Kubernetes APIs.
	Core k8s.Interface
	// Metrics reads the metrics.k8s.io API (served by metrics-server, which
	// may not be installed).
	Metrics metrics.Interface
}

// New connects to the cluster. With an empty kubeconfigPath it uses the pod's
// ServiceAccount when running in-cluster, and otherwise falls back to the
// standard kubeconfig lookup (KUBECONFIG, then ~/.kube/config).
//
// No request is made here; an unreachable cluster surfaces on first use.
func New(kubeconfigPath string) (*Client, error) {
	cfg, err := restConfig(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading kubernetes config: %w", err)
	}
	// The overview fans out many list calls at once; the client-go defaults
	// (5 QPS) would throttle them.
	cfg.QPS = 50
	cfg.Burst = 100

	core, err := k8s.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}
	m, err := metrics.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}
	return &Client{Core: core, Metrics: m}, nil
}

func restConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		if cfg, err := rest.InClusterConfig(); err == nil {
			return cfg, nil
		}
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}
