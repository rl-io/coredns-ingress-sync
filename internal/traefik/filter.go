// Package traefik provides Traefik IngressRoute (traefik.io/v1alpha1)
// filtering, mirroring internal/ingress and internal/gatewayapi's Filter
// shape. Unlike Ingress/HTTPRoute, IngressRoute has no class-like field, so
// this filter is keyed on a single target CNAME rather than a class ->
// CNAME mapping list: when configured, every processable IngressRoute's
// hosts resolve to that one CNAME.
//
// IngressRoute doesn't carry its hostname in a structured field -- it's
// embedded in spec.routes[].match, a router-rule DSL string such as
// "Host(`example.com`) && PathPrefix(`/api`)". This package extracts
// literal, backtick-quoted arguments to Host() matchers only; HostRegexp,
// HostSNI, and other matchers are not parsed, and IngressRouteTCP/UDP are
// out of scope entirely.
package traefik

import (
	"regexp"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	traefikv1alpha1 "github.com/rl-io/coredns-ingress-sync/internal/traefik/v1alpha1"

	"github.com/rl-io/coredns-ingress-sync/internal/filterutil"
	"github.com/rl-io/coredns-ingress-sync/internal/hostmap"
)

// Filter provides Traefik IngressRoute filtering functionality.
type Filter struct {
	targetCNAME           string
	nsScope               filterutil.NamespaceScope
	excludeSet            filterutil.ExcludeSet
	annotationEnabledKey  string
	annotationPriorityKey string
	logger                logr.Logger
}

// NewFilter creates a new Traefik IngressRoute filter. targetCNAME is the
// single CNAME every processable IngressRoute's hosts resolve to; an empty
// targetCNAME means the feature is disabled (see Enabled).
// annotationPriorityKey, when set, lets an individual IngressRoute override
// its priority via an integer annotation (higher wins).
func NewFilter(targetCNAME, watchNamespacesEnv, excludeNamespacesEnv, excludeIngressRoutesEnv, annotationEnabledKey, annotationPriorityKey string) *Filter {
	return &Filter{
		targetCNAME:           targetCNAME,
		nsScope:               filterutil.NewNamespaceScope(watchNamespacesEnv, excludeNamespacesEnv),
		excludeSet:            filterutil.NewExcludeSet(excludeIngressRoutesEnv),
		annotationEnabledKey:  annotationEnabledKey,
		annotationPriorityKey: annotationPriorityKey,
		logger:                ctrl.Log.WithName("traefik-filter"),
	}
}

// Enabled reports whether a target CNAME is configured. When false, the
// reconciler must skip Traefik IngressRoute support entirely (no List/Get
// calls).
func (f *Filter) Enabled() bool {
	return f.targetCNAME != ""
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

// IsExcludedIngressRoute returns true if the given IngressRoute should be
// excluded by name/namespace.
func (f *Filter) IsExcludedIngressRoute(ir *traefikv1alpha1.IngressRoute) bool {
	if ir == nil {
		return false
	}
	return f.excludeSet.IsExcluded(ir.Namespace, ir.Name)
}

// ShouldProcessIngressRoute returns true if this IngressRoute's namespace is
// watched, it is not excluded, and it is not annotated disabled.
func (f *Filter) ShouldProcessIngressRoute(ir *traefikv1alpha1.IngressRoute) bool {
	if ir == nil {
		return false
	}
	if !f.ShouldWatchNamespace(ir.Namespace) {
		return false
	}
	if f.IsExcludedIngressRoute(ir) {
		return false
	}
	if f.annotationEnabledKey != "" {
		if ann := ir.GetAnnotations(); ann != nil {
			if val, ok := ann[f.annotationEnabledKey]; ok {
				if filterutil.IsFalseLike(val) {
					return false
				}
			}
		}
	}
	return true
}

// hostMatcher finds each Host(...) matcher call within a router rule match
// expression. Matcher arguments are backtick-quoted string literals only, so
// they never contain a ")", making a non-greedy scan up to the first ")"
// safe.
var hostMatcher = regexp.MustCompile(`(?i)Host\(([^)]*)\)`)

// hostArg extracts a single backtick-quoted argument.
var hostArg = regexp.MustCompile("`([^`]*)`")

// extractHosts returns the deduplicated set of literal hostnames referenced
// by Host(...) matchers in a Traefik router rule match expression, e.g.
// "Host(`a.com`,`b.com`) && PathPrefix(`/x`)" -> ["a.com", "b.com"]. Matchers
// other than Host() (HostRegexp, HostSNI, etc.) are ignored, and a match
// expression with no Host() matcher returns nil.
func extractHosts(match string) []string {
	if match == "" {
		return nil
	}

	var hosts []string
	seen := make(map[string]bool)
	for _, hostCall := range hostMatcher.FindAllStringSubmatch(match, -1) {
		for _, arg := range hostArg.FindAllStringSubmatch(hostCall[1], -1) {
			host := arg[1]
			if host == "" || seen[host] {
				continue
			}
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// ExtractHostnameCandidates returns one hostmap.Candidate per (host,
// IngressRoute) pair for all processable IngressRoutes, deduplicated within
// each IngressRoute across its spec.routes[] entries. Every candidate's
// ClassIndex is 0 (there is only one source-internal tier, since IngressRoute
// has no class-like field); callers merging candidates from other sources
// should offset ClassIndex before calling hostmap.Resolve so ties fall back
// to source order.
func (f *Filter) ExtractHostnameCandidates(routes []traefikv1alpha1.IngressRoute) []hostmap.Candidate {
	var candidates []hostmap.Candidate

	for i := range routes {
		ir := &routes[i]
		if !f.ShouldProcessIngressRoute(ir) {
			continue
		}

		source := "IngressRoute:" + ir.Namespace + "/" + ir.Name
		priority := filterutil.ResolvePriority(ir.Annotations, f.annotationPriorityKey)

		seen := make(map[string]bool)
		for _, route := range ir.Spec.Routes {
			for _, host := range extractHosts(route.Match) {
				if seen[host] {
					continue
				}
				seen[host] = true
				candidates = append(candidates, hostmap.Candidate{
					Host:       host,
					CNAME:      f.targetCNAME,
					Priority:   priority,
					ClassIndex: 0,
					Source:     source,
				})
			}
		}
	}

	return candidates
}
