package ingress

import (
	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/filterutil"
	"github.com/rl-io/coredns-ingress-sync/internal/hostmap"
)

// Filter provides ingress filtering functionality
type Filter struct {
	// classMappings is the ordered list of class->CNAME mappings. Slice order
	// defines the default priority tiebreak (index 0 = lowest priority).
	classMappings         []config.IngressClassMapping
	classToCNAME          map[string]string // ingress class -> target CNAME
	classToIndex          map[string]int    // ingress class -> config order index
	nsScope               filterutil.NamespaceScope
	excludeSet            filterutil.ExcludeSet
	annotationEnabledKey  string
	annotationPriorityKey string
	logger                logr.Logger
}

// NewFilter creates a new ingress filter. classMappings is the ordered list of
// ingress class -> target CNAME mappings; its order defines the default
// priority tiebreak. annotationPriorityKey, when set, lets an individual
// ingress override its priority via an integer annotation (higher wins).
func NewFilter(classMappings []config.IngressClassMapping, watchNamespacesEnv string, excludeNamespacesEnv string, excludeIngressesEnv string, annotationEnabledKey string, annotationPriorityKey string) *Filter {
	filter := &Filter{
		classMappings:         classMappings,
		classToCNAME:          make(map[string]string, len(classMappings)),
		classToIndex:          make(map[string]int, len(classMappings)),
		nsScope:               filterutil.NewNamespaceScope(watchNamespacesEnv, excludeNamespacesEnv),
		excludeSet:            filterutil.NewExcludeSet(excludeIngressesEnv),
		annotationEnabledKey:  annotationEnabledKey,
		annotationPriorityKey: annotationPriorityKey,
		logger:                ctrl.Log.WithName("ingress-filter"),
	}

	// Build lookup tables. If a class appears more than once, the first
	// occurrence wins (lowest index and its CNAME).
	for i, m := range classMappings {
		if _, exists := filter.classToIndex[m.IngressClass]; !exists {
			filter.classToIndex[m.IngressClass] = i
			filter.classToCNAME[m.IngressClass] = m.TargetCNAME
		}
	}

	return filter
}

// IsTargetIngress checks if an ingress object matches any configured ingress class
func (f *Filter) IsTargetIngress(obj client.Object) bool {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return false
	}
	return f.matchesClass(ingress)
}

// matchesClass reports whether the ingress declares a class we are configured to watch
func (f *Filter) matchesClass(ing *networkingv1.Ingress) bool {
	if ing.Spec.IngressClassName == nil {
		return false
	}
	_, ok := f.classToIndex[*ing.Spec.IngressClassName]
	return ok
}

// ShouldWatchNamespace checks if we should process objects in the given namespace
func (f *Filter) ShouldWatchNamespace(namespace string) bool {
	return f.nsScope.ShouldWatch(namespace)
}

// IsExcludedIngress returns true if the given ingress should be excluded by name/namespace
func (f *Filter) IsExcludedIngress(ing *networkingv1.Ingress) bool {
	if ing == nil {
		return false
	}
	return f.excludeSet.IsExcluded(ing.Namespace, ing.Name)
}

// ShouldProcessIngress returns true if this ingress matches class, namespace, and is not excluded
func (f *Filter) ShouldProcessIngress(ing *networkingv1.Ingress) bool {
	if ing == nil {
		return false
	}
	if !f.matchesClass(ing) {
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

// ExtractHostnameCandidates returns one hostmap.Candidate per (host, ingress)
// pair for all processable ingresses, with ClassIndex set to the ingress
// class's 0-based config-order index. Callers merging candidates from
// multiple sources (e.g. Gateway API) should offset ClassIndex before calling
// hostmap.Resolve so ties fall back to source order.
func (f *Filter) ExtractHostnameCandidates(ingresses []networkingv1.Ingress) []hostmap.Candidate {
	var candidates []hostmap.Candidate

	for i := range ingresses {
		ing := &ingresses[i]
		if !f.ShouldProcessIngress(ing) {
			continue
		}

		class := *ing.Spec.IngressClassName
		cname := f.classToCNAME[class]
		priority := resolvePriority(ing, f.annotationPriorityKey)
		index := f.classToIndex[class]
		source := "Ingress:" + ing.Namespace + "/" + ing.Name

		for _, rule := range ing.Spec.Rules {
			if rule.Host == "" {
				continue
			}
			candidates = append(candidates, hostmap.Candidate{
				Host:       rule.Host,
				CNAME:      cname,
				Priority:   priority,
				ClassIndex: index,
				Source:     source,
			})
		}
	}

	return candidates
}

// ExtractHostnameMappings returns a map of hostname -> target CNAME for all
// processable ingresses. When multiple ingresses (typically different classes)
// declare the same hostname, the one with the higher resolved priority wins.
// Priority comes from the per-ingress priority annotation when present; absent
// (or invalid), all ingresses share a baseline priority and the class config
// order decides (first listed wins). A positive annotation therefore promotes
// an otherwise-default class to win the rewrite.
func (f *Filter) ExtractHostnameMappings(ingresses []networkingv1.Ingress) map[string]string {
	return hostmap.Resolve(f.ExtractHostnameCandidates(ingresses), f.logger)
}

// ExtractHostnames extracts the set of hostnames from processable ingresses.
func (f *Filter) ExtractHostnames(ingresses []networkingv1.Ingress) []string {
	mappings := f.ExtractHostnameMappings(ingresses)
	hosts := make([]string, 0, len(mappings))
	for host := range mappings {
		hosts = append(hosts, host)
	}
	return hosts
}

// ClassCount returns the number of configured ingress class mappings. The
// reconciler uses this to offset Gateway API candidates' ClassIndex when
// merging candidates from both sources into one hostmap.Resolve call, so
// ties between an Ingress and an HTTPRoute default to Ingress winning.
func (f *Filter) ClassCount() int {
	return len(f.classMappings)
}

// resolvePriority returns the explicit priority for an ingress: the integer
// value of its priority annotation when present and valid, otherwise the
// baseline priority of 0. Higher wins; ties are broken by class config order,
// so an unannotated ingress relies on config order.
func resolvePriority(ing *networkingv1.Ingress, annotationKey string) int {
	return filterutil.ResolvePriority(ing.Annotations, annotationKey)
}

// GetWatchNamespaces returns the list of namespaces being watched
func (f *Filter) GetWatchNamespaces() []string {
	return f.nsScope.WatchNamespaces()
}

// WatchesAllNamespaces returns true if watching all namespaces
func (f *Filter) WatchesAllNamespaces() bool {
	return f.nsScope.WatchesAllNamespaces()
}

// isFalseLike returns true if the string represents a false value
func isFalseLike(s string) bool {
	return filterutil.IsFalseLike(s)
}
