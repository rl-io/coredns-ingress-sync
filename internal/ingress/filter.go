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
	excludeNamespaces []string
	// exclude by ingress name (applies cluster-wide) and by namespace/name
	excludeIngressNames map[string]bool               // name -> true
	excludeIngressByNS  map[string]map[string]bool    // ns -> name -> true
	annotationEnabledKey string
}

// NewFilter creates a new ingress filter
func NewFilter(ingressClass string, watchNamespacesEnv string, excludeNamespacesEnv string, excludeIngressesEnv string, annotationEnabledKey string) *Filter {
	filter := &Filter{
		ingressClass: ingressClass,
		annotationEnabledKey: annotationEnabledKey,
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

	// Parse exclude namespaces
	if excludeNamespacesEnv != "" {
		namespaces := strings.Split(strings.ReplaceAll(excludeNamespacesEnv, " ", ""), ",")
		for _, ns := range namespaces {
			if ns != "" {
				filter.excludeNamespaces = append(filter.excludeNamespaces, ns)
			}
		}
	}

	// Parse exclude ingresses (supports name or namespace/name)
	filter.excludeIngressNames = make(map[string]bool)
	filter.excludeIngressByNS = make(map[string]map[string]bool)
	if excludeIngressesEnv != "" {
		parts := strings.Split(strings.TrimSpace(excludeIngressesEnv), ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.Contains(p, "/") {
				// namespace/name form
				segs := strings.SplitN(p, "/", 2)
				ns := strings.TrimSpace(segs[0])
				name := strings.TrimSpace(segs[1])
				if ns == "" || name == "" {
					continue
				}
				if _, ok := filter.excludeIngressByNS[ns]; !ok {
					filter.excludeIngressByNS[ns] = make(map[string]bool)
				}
				filter.excludeIngressByNS[ns][name] = true
			} else {
				// name only (global)
				filter.excludeIngressNames[p] = true
			}
		}
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
		// If watching all, still respect exclude list
		for _, ex := range f.excludeNamespaces {
			if ex == namespace {
				return false
			}
		}
		return true
	}
	// Specific watch list: must be included and not excluded
	allowed := false
	for _, ns := range f.watchNamespaces {
		if ns == namespace {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, ex := range f.excludeNamespaces {
		if ex == namespace {
			return false
		}
	}
	return true
}

// IsExcludedIngress returns true if the given ingress should be excluded by name/namespace
func (f *Filter) IsExcludedIngress(ing *networkingv1.Ingress) bool {
	if ing == nil {
		return false
	}
	if f.excludeIngressNames[ing.Name] {
		return true
	}
	if byNS, ok := f.excludeIngressByNS[ing.Namespace]; ok {
		if byNS[ing.Name] {
			return true
		}
	}
	return false
}

// ShouldProcessIngress returns true if this ingress matches class, namespace, and is not excluded
func (f *Filter) ShouldProcessIngress(ing *networkingv1.Ingress) bool {
	if ing == nil {
		return false
	}
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != f.ingressClass {
		return false
	}
	if !f.ShouldWatchNamespace(ing.Namespace) {
		return false
	}
	if f.IsExcludedIngress(ing) {
		return false
	}
	// Annotation-based exclusion: if annotation key is set and value is false-like, exclude
	if f.annotationEnabledKey != "" {
		if ann := ing.GetAnnotations(); ann != nil {
			if val, ok := ann[f.annotationEnabledKey]; ok {
				if isFalseLike(val) {
					return false
				}
			}
		}
	}
	return true
}

// ExtractHostnames extracts all hostnames from a list of ingresses that match our criteria
func (f *Filter) ExtractHostnames(ingresses []networkingv1.Ingress) []string {
	hostSet := make(map[string]bool)

	for _, ing := range ingresses {
		// Skip ingresses that shouldn't be processed
		if !f.ShouldProcessIngress(&ing) {
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

// isFalseLike returns true if the string represents a false value
func isFalseLike(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	switch v {
	case "false", "0", "no", "off", "disabled":
		return true
	}
	return false
}
