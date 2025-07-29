package ingress

import (
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Filter provides ingress filtering functionality
type Filter struct {
	ingressClass     string
	watchNamespaces  []string
	watchAllNamespaces bool
}

// NewFilter creates a new ingress filter
func NewFilter(ingressClass string, watchNamespacesEnv string) *Filter {
	filter := &Filter{
		ingressClass: ingressClass,
	}

	// Parse watch namespaces
	if watchNamespacesEnv != "" {
		namespaces := strings.Split(strings.ReplaceAll(watchNamespacesEnv, " ", ""), ",")
		// Filter out empty strings
		var validNamespaces []string
		for _, ns := range namespaces {
			if ns != "" {
				validNamespaces = append(validNamespaces, ns)
			}
		}
		filter.watchNamespaces = validNamespaces
		filter.watchAllNamespaces = len(validNamespaces) == 0
	} else {
		filter.watchAllNamespaces = true
	}

	return filter
}

// IsTargetIngress checks if an ingress object matches our ingress class
func (f *Filter) IsTargetIngress(obj client.Object) bool {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return false
	}
	return ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == f.ingressClass
}

// ShouldWatchNamespace checks if we should process objects in the given namespace
func (f *Filter) ShouldWatchNamespace(namespace string) bool {
	if f.watchAllNamespaces {
		return true
	}
	
	for _, ns := range f.watchNamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

// ExtractHostnames extracts all hostnames from a list of ingresses that match our criteria
func (f *Filter) ExtractHostnames(ingresses []networkingv1.Ingress) []string {
	hostSet := make(map[string]bool)

	for _, ing := range ingresses {
		// Skip ingresses not in watched namespaces
		if !f.ShouldWatchNamespace(ing.Namespace) {
			continue
		}

		// Skip ingresses not matching our class
		if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != f.ingressClass {
			continue
		}

		// Extract hosts from rules
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				hostSet[rule.Host] = true
			}
		}
	}

	// Convert set to slice
	var hosts []string
	for host := range hostSet {
		hosts = append(hosts, host)
	}

	return hosts
}

// GetWatchNamespaces returns the list of namespaces being watched
func (f *Filter) GetWatchNamespaces() []string {
	if f.watchAllNamespaces {
		return nil // nil indicates all namespaces
	}
	return f.watchNamespaces
}

// WatchesAllNamespaces returns true if watching all namespaces
func (f *Filter) WatchesAllNamespaces() bool {
	return f.watchAllNamespaces
}
