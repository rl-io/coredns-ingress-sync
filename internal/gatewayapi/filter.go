// Package gatewayapi provides Gateway API (Gateway + HTTPRoute) filtering,
// mirroring internal/ingress's Filter but keyed on GatewayClassName instead
// of IngressClassName. HTTPRoute does not carry its class inline: it
// references a Gateway via spec.parentRefs, so callers must first list
// Gateway objects and build a ref -> GatewayClassName map (see
// BuildGatewayClassByRef) before extracting hostname candidates.
package gatewayapi

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/filterutil"
	"github.com/rl-io/coredns-ingress-sync/internal/hostmap"
)

// Filter provides Gateway API filtering functionality.
type Filter struct {
	// classMappings is the ordered list of GatewayClass->CNAME mappings.
	// Slice order defines the default priority tiebreak (index 0 = lowest
	// priority) within Gateway API candidates; the reconciler additionally
	// offsets these indices against Ingress's when merging both sources.
	classMappings         []config.GatewayClassMapping
	classToCNAME          map[string]string // GatewayClass -> target CNAME
	classToIndex          map[string]int    // GatewayClass -> config order index
	nsScope               filterutil.NamespaceScope
	excludeSet            filterutil.ExcludeSet
	annotationEnabledKey  string
	annotationPriorityKey string
	logger                logr.Logger
}

// NewFilter creates a new Gateway API filter. classMappings is the ordered
// list of GatewayClass -> target CNAME mappings; its order defines the
// default priority tiebreak. annotationPriorityKey, when set, lets an
// individual HTTPRoute override its priority via an integer annotation
// (higher wins).
func NewFilter(classMappings []config.GatewayClassMapping, watchNamespacesEnv string, excludeNamespacesEnv string, excludeHTTPRoutesEnv string, annotationEnabledKey string, annotationPriorityKey string) *Filter {
	filter := &Filter{
		classMappings:         classMappings,
		classToCNAME:          make(map[string]string, len(classMappings)),
		classToIndex:          make(map[string]int, len(classMappings)),
		nsScope:               filterutil.NewNamespaceScope(watchNamespacesEnv, excludeNamespacesEnv),
		excludeSet:            filterutil.NewExcludeSet(excludeHTTPRoutesEnv),
		annotationEnabledKey:  annotationEnabledKey,
		annotationPriorityKey: annotationPriorityKey,
		logger:                ctrl.Log.WithName("gatewayapi-filter"),
	}

	// Build lookup tables. If a class appears more than once, the first
	// occurrence wins (lowest index and its CNAME).
	for i, m := range classMappings {
		if _, exists := filter.classToIndex[m.GatewayClass]; !exists {
			filter.classToIndex[m.GatewayClass] = i
			filter.classToCNAME[m.GatewayClass] = m.TargetCNAME
		}
	}

	return filter
}

// Enabled reports whether any GatewayClass mappings are configured. When
// false, the reconciler must skip Gateway API entirely (no List/Get calls).
func (f *Filter) Enabled() bool {
	return len(f.classMappings) > 0
}

// ClassCount returns the number of configured GatewayClass mappings. The
// reconciler uses this to offset Traefik IngressRoute candidates' ClassIndex
// when merging candidates from all sources into one hostmap.Resolve call, so
// ties between an HTTPRoute and an IngressRoute default to HTTPRoute winning.
func (f *Filter) ClassCount() int {
	return len(f.classMappings)
}

// ShouldWatchNamespace checks if we should process objects in the given namespace.
func (f *Filter) ShouldWatchNamespace(namespace string) bool {
	return f.nsScope.ShouldWatch(namespace)
}

// GetWatchNamespaces returns the list of namespaces being watched.
func (f *Filter) GetWatchNamespaces() []string {
	return f.nsScope.WatchNamespaces()
}

// WatchesAllNamespaces returns true if watching all namespaces.
func (f *Filter) WatchesAllNamespaces() bool {
	return f.nsScope.WatchesAllNamespaces()
}

// IsExcludedHTTPRoute returns true if the given HTTPRoute should be excluded by name/namespace.
func (f *Filter) IsExcludedHTTPRoute(route *gatewayv1.HTTPRoute) bool {
	if route == nil {
		return false
	}
	return f.excludeSet.IsExcluded(route.Namespace, route.Name)
}

// ShouldProcessHTTPRoute returns true if this HTTPRoute's namespace is
// watched, it is not excluded, and it is not annotated disabled. It does NOT
// check whether the route attaches to a configured GatewayClass -- that
// depends on the Gateway ref map and is resolved per-parentRef in
// ExtractHostnameCandidates.
func (f *Filter) ShouldProcessHTTPRoute(route *gatewayv1.HTTPRoute) bool {
	if route == nil {
		return false
	}
	if !f.ShouldWatchNamespace(route.Namespace) {
		return false
	}
	if f.IsExcludedHTTPRoute(route) {
		return false
	}
	if f.annotationEnabledKey != "" {
		if ann := route.GetAnnotations(); ann != nil {
			if val, ok := ann[f.annotationEnabledKey]; ok {
				if filterutil.IsFalseLike(val) {
					return false
				}
			}
		}
	}
	return true
}

// GatewayInfo holds the data ExtractHostnameCandidates needs from a
// HTTPRoute's parent Gateway: its class (for matching against configured
// GatewayClass mappings) and its listener hostnames (used as a fallback
// when a HTTPRoute doesn't specify its own hostnames).
type GatewayInfo struct {
	ClassName string
	Hostnames []string
}

// BuildGatewayClassByRef lists Gateway objects and builds a
// namespace/name -> GatewayInfo lookup, used to resolve which HTTPRoutes
// attach to a configured GatewayClass and, when a route omits
// spec.hostnames, which listener hostname(s) it should inherit per the
// Gateway API spec. Gateways are included regardless of their class; class
// matching happens in ExtractHostnameCandidates so unconfigured classes are
// simply ignored there, mirroring how Ingress ignores unconfigured
// IngressClassNames.
func BuildGatewayClassByRef(gateways []gatewayv1.Gateway) map[types.NamespacedName]GatewayInfo {
	refs := make(map[types.NamespacedName]GatewayInfo, len(gateways))
	for i := range gateways {
		gw := &gateways[i]

		seen := make(map[string]bool, len(gw.Spec.Listeners))
		var hostnames []string
		for _, listener := range gw.Spec.Listeners {
			if listener.Hostname == nil {
				continue
			}
			host := string(*listener.Hostname)
			if host == "" || seen[host] {
				continue
			}
			seen[host] = true
			hostnames = append(hostnames, host)
		}

		refs[types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}] = GatewayInfo{
			ClassName: string(gw.Spec.GatewayClassName),
			Hostnames: hostnames,
		}
	}
	return refs
}

// ExtractHostnameCandidates returns one hostmap.Candidate per (hostname,
// parentRef) pair for all processable HTTPRoutes that attach to a Gateway
// with a configured GatewayClass, per gatewayClassByRef (see
// BuildGatewayClassByRef). SectionName and Port on parentRefs are
// intentionally ignored: routes are matched at Gateway granularity, not
// listener granularity. When a HTTPRoute doesn't specify spec.hostnames, it
// inherits the parent Gateway's listener hostname(s) per the Gateway API
// spec; if the Gateway also has no listener hostname, the route contributes
// no candidates (there is nothing host-specific to rewrite).
func (f *Filter) ExtractHostnameCandidates(routes []gatewayv1.HTTPRoute, gatewayClassByRef map[types.NamespacedName]GatewayInfo) []hostmap.Candidate {
	var candidates []hostmap.Candidate

	for i := range routes {
		route := &routes[i]
		if !f.ShouldProcessHTTPRoute(route) {
			continue
		}

		source := route.Namespace + "/" + route.Name
		priority := filterutil.ResolvePriority(route.Annotations, f.annotationPriorityKey)

		for _, parentRef := range route.Spec.ParentRefs {
			if parentRef.Kind != nil && string(*parentRef.Kind) != "Gateway" {
				continue
			}
			if parentRef.Group != nil && string(*parentRef.Group) != "" && string(*parentRef.Group) != gatewayv1.GroupName {
				continue
			}

			// Namespace defaults to the HTTPRoute's own namespace when unset,
			// per the Gateway API spec.
			ns := route.Namespace
			if parentRef.Namespace != nil && string(*parentRef.Namespace) != "" {
				ns = string(*parentRef.Namespace)
			}
			ref := types.NamespacedName{Namespace: ns, Name: string(parentRef.Name)}

			gwInfo, ok := gatewayClassByRef[ref]
			if !ok {
				continue
			}
			index, ok := f.classToIndex[gwInfo.ClassName]
			if !ok {
				continue
			}
			cname := f.classToCNAME[gwInfo.ClassName]

			hostnames := route.Spec.Hostnames
			if len(hostnames) == 0 {
				for _, host := range gwInfo.Hostnames {
					candidates = append(candidates, hostmap.Candidate{
						Host:       host,
						CNAME:      cname,
						Priority:   priority,
						ClassIndex: index,
						Source:     source,
					})
				}
				continue
			}

			for _, hostname := range hostnames {
				host := string(hostname)
				if host == "" {
					continue
				}
				candidates = append(candidates, hostmap.Candidate{
					Host:       host,
					CNAME:      cname,
					Priority:   priority,
					ClassIndex: index,
					Source:     source,
				})
			}
		}
	}

	return candidates
}
