package resources

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// maxListedEvents caps the events page; the overview shows fewer.
const maxListedEvents = 200

type ServiceSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ClusterIP string `json:"clusterIP"`
	// External lists addresses reachable from outside the cluster.
	External []string `json:"external"`
	// Ports are formatted as "80/TCP → 8080", with ":30080" after the port
	// when a node port is assigned.
	Ports     []string          `json:"ports"`
	Selector  map[string]string `json:"selector"`
	CreatedAt time.Time         `json:"createdAt"`
}

type IngressSummary struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Class     string    `json:"class"`
	Hosts     []string  `json:"hosts"`
	Addresses []string  `json:"addresses"`
	TLS       bool      `json:"tls"`
	CreatedAt time.Time `json:"createdAt"`
}

// Services lists services in one namespace, or all when namespace is empty.
func (s *Service) Services(ctx context.Context, namespace string) ([]ServiceSummary, error) {
	list, err := s.kube.Core.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}
	out := make([]ServiceSummary, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, summarizeService(&list.Items[i]))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Ingresses lists ingresses in one namespace, or all when namespace is empty.
func (s *Service) Ingresses(ctx context.Context, namespace string) ([]IngressSummary, error) {
	list, err := s.kube.Core.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing ingresses: %w", err)
	}
	out := make([]IngressSummary, 0, len(list.Items))
	for i := range list.Items {
		ing := &list.Items[i]
		sum := IngressSummary{
			Name: ing.Name, Namespace: ing.Namespace,
			Hosts: []string{}, Addresses: []string{},
			TLS: len(ing.Spec.TLS) > 0, CreatedAt: ing.CreationTimestamp.Time,
		}
		if ing.Spec.IngressClassName != nil {
			sum.Class = *ing.Spec.IngressClassName
		}
		for _, rule := range ing.Spec.Rules {
			sum.Hosts = append(sum.Hosts, orDefault(rule.Host, "*"))
		}
		for _, lb := range ing.Status.LoadBalancer.Ingress {
			sum.Addresses = append(sum.Addresses, orDefault(lb.IP, lb.Hostname))
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Events lists recent events, newest first, in one namespace or all.
func (s *Service) Events(ctx context.Context, namespace string) ([]EventSummary, error) {
	list, err := s.kube.Core.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	return summarizeEvents(list.Items, maxListedEvents), nil
}

func summarizeService(svc *corev1.Service) ServiceSummary {
	out := ServiceSummary{
		Name: svc.Name, Namespace: svc.Namespace,
		Type: string(svc.Spec.Type), ClusterIP: svc.Spec.ClusterIP,
		External: []string{}, Ports: []string{},
		Selector: svc.Spec.Selector, CreatedAt: svc.CreationTimestamp.Time,
	}
	if out.Selector == nil {
		out.Selector = map[string]string{}
	}
	out.External = append(out.External, svc.Spec.ExternalIPs...)
	for _, lb := range svc.Status.LoadBalancer.Ingress {
		out.External = append(out.External, orDefault(lb.IP, lb.Hostname))
	}
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		out.External = append(out.External, svc.Spec.ExternalName)
	}
	for _, p := range svc.Spec.Ports {
		port := fmt.Sprintf("%d", p.Port)
		if p.NodePort != 0 {
			port += fmt.Sprintf(":%d", p.NodePort)
		}
		out.Ports = append(out.Ports, fmt.Sprintf("%s/%s → %s", port, p.Protocol, p.TargetPort.String()))
	}
	return out
}
