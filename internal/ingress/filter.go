package ingress

import (
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
)

// Filter provides ingress filtering functionality
type Filter struct {
	// classMappings is the ordered list of class->CNAME mappings. Slice order
	// defines the default priority tiebreak (index 0 = lowest priority).
	classMappings    []config.IngressClassMapping
	classToCNAME     map[string]string // ingress class -> target CNAME
	classToIndex     map[string]int    // ingress class -> config order index
	watchNamespaces  []string
	watchAllNamespaces bool
	excludeNamespaces []string
	// exclude by ingress name (applies cluster-wide) and by namespace/name
	excludeIngressNames map[string]bool               // name -> true
	excludeIngressByNS  map[string]map[string]bool    // ns -> name -> true
	annotationEnabledKey string
	annotationPriorityKey string
	logger logr.Logger
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

// hostWinner tracks the ingress currently winning the rewrite for a hostname.
type hostWinner struct {
	cname    string
	priority int // resolved priority; higher wins
	index    int // class config order index; lower wins ties (first listed = default)
	source   string // namespace/name, used for tie-break logging
}

// beats reports whether candidate c should win over the current best b.
// Higher priority wins; on equal priority the lower config index wins, so the
// first-listed class is the safe default when nothing is annotated.
func (c hostWinner) beats(b hostWinner) bool {
	if c.priority != b.priority {
		return c.priority > b.priority
	}
	if c.index != b.index {
		return c.index < b.index
	}
	// Same priority and same config index (only possible for the same class,
	// hence identical CNAME). Fall back to a stable lexicographic pick so the
	// output never flaps across reconciles.
	return c.cname < b.cname
}

// ExtractHostnameMappings returns a map of hostname -> target CNAME for all
// processable ingresses. When multiple ingresses (typically different classes)
// declare the same hostname, the one with the higher resolved priority wins.
// Priority comes from the per-ingress priority annotation when present; absent
// (or invalid), all ingresses share a baseline priority and the class config
// order decides (first listed wins). A positive annotation therefore promotes
// an otherwise-default class to win the rewrite.
func (f *Filter) ExtractHostnameMappings(ingresses []networkingv1.Ingress) map[string]string {
	winners := make(map[string]hostWinner)

	for i := range ingresses {
		ing := &ingresses[i]
		if !f.ShouldProcessIngress(ing) {
			continue
		}

		class := *ing.Spec.IngressClassName
		candidate := hostWinner{
			cname:    f.classToCNAME[class],
			priority: resolvePriority(ing, f.annotationPriorityKey),
			index:    f.classToIndex[class],
			source:   ing.Namespace + "/" + ing.Name,
		}

		for _, rule := range ing.Spec.Rules {
			if rule.Host == "" {
				continue
			}

			existing, ok := winners[rule.Host]
			if !ok {
				winners[rule.Host] = candidate
				continue
			}
			if candidate.beats(existing) {
				// Only log when the resolved targets actually differ; same-class
				// duplicates produce identical CNAMEs and are not noteworthy.
				if candidate.cname != existing.cname {
					f.logger.V(1).Info("Hostname served by multiple classes; higher-priority ingress wins",
						"host", rule.Host,
						"winner", candidate.source, "winnerCNAME", candidate.cname, "winnerPriority", candidate.priority,
						"loser", existing.source, "loserCNAME", existing.cname, "loserPriority", existing.priority)
				}
				winners[rule.Host] = candidate
			}
		}
	}

	result := make(map[string]string, len(winners))
	for host, w := range winners {
		result[host] = w.cname
	}
	return result
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

// resolvePriority returns the explicit priority for an ingress: the integer
// value of its priority annotation when present and valid, otherwise the
// baseline priority of 0. Higher wins; ties are broken by class config order
// (see hostWinner.beats), so an unannotated ingress relies on config order.
func resolvePriority(ing *networkingv1.Ingress, annotationKey string) int {
	if annotationKey != "" {
		if val, ok := ing.Annotations[annotationKey]; ok {
			if p, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				return p
			}
		}
	}
	return 0
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
