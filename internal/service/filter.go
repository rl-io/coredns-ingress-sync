// Package service provides annotation-driven Service filtering, letting a
// plain corev1.Service declare a hostname to answer to without any
// Ingress/HTTPRoute/IngressRoute in front of it. Unlike the other three
// sources, there's no shared per-class/controller CNAME to configure -- each
// annotated Service is its own target
// (<name>.<namespace>.svc.<cluster-domain>.).
package service

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rl-io/coredns-ingress-sync/internal/filterutil"
	"github.com/rl-io/coredns-ingress-sync/internal/hostmap"
)

// Filter provides annotation-driven Service filtering functionality.
type Filter struct {
	clusterDomain         string
	hostnameAnnotationKey string
	nsScope               filterutil.NamespaceScope
	excludeSet            filterutil.ExcludeSet
	annotationEnabledKey  string
	annotationPriorityKey string
	logger                logr.Logger
}

// NewFilter creates a new Service filter. clusterDomain is used to build each
// eligible Service's synthesized target CNAME
// (<name>.<namespace>.svc.<clusterDomain>.). hostnameAnnotationKey is the
// annotation whose presence (and value) makes a Service a candidate at all;
// annotationPriorityKey, when set, lets an individual Service override its
// priority via an integer annotation (higher wins).
func NewFilter(clusterDomain, hostnameAnnotationKey, watchNamespacesEnv, excludeNamespacesEnv, excludeServicesEnv, annotationEnabledKey, annotationPriorityKey string) *Filter {
	return &Filter{
		clusterDomain:         clusterDomain,
		hostnameAnnotationKey: hostnameAnnotationKey,
		nsScope:               filterutil.NewNamespaceScope(watchNamespacesEnv, excludeNamespacesEnv),
		excludeSet:            filterutil.NewExcludeSet(excludeServicesEnv),
		annotationEnabledKey:  annotationEnabledKey,
		annotationPriorityKey: annotationPriorityKey,
		logger:                ctrl.Log.WithName("service-filter"),
	}
}

// Enabled always reports true once a Filter has been constructed; the
// reconciler's own nil-pointer check (ServiceFilter == nil) is the real
// on/off switch, kept symmetric with the other sources' "!= nil &&
// .Enabled()" idiom.
func (f *Filter) Enabled() bool {
	return true
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

// IsExcludedService returns true if the given Service should be excluded by
// name/namespace.
func (f *Filter) IsExcludedService(svc *corev1.Service) bool {
	if svc == nil {
		return false
	}
	return f.excludeSet.IsExcluded(svc.Namespace, svc.Name)
}

// ShouldProcessService returns true if this Service's namespace is watched,
// it is not excluded, it carries a non-empty hostname annotation, and it is
// not annotated disabled. The hostname annotation's presence is the
// participation signal (mirroring ingressClassName/gatewayClassName/
// IngressRoute-presence on the other sources) -- annotationEnabledKey keeps
// its existing opt-out meaning, unchanged from the other three sources.
func (f *Filter) ShouldProcessService(svc *corev1.Service) bool {
	if svc == nil {
		return false
	}
	if !f.ShouldWatchNamespace(svc.Namespace) {
		return false
	}
	if f.IsExcludedService(svc) {
		return false
	}
	if f.hostnameAnnotationKey == "" || svc.GetAnnotations()[f.hostnameAnnotationKey] == "" {
		return false
	}
	if f.annotationEnabledKey != "" {
		if ann := svc.GetAnnotations(); ann != nil {
			if val, ok := ann[f.annotationEnabledKey]; ok {
				if filterutil.IsFalseLike(val) {
					return false
				}
			}
		}
	}
	return true
}

// ExtractHostnameCandidates returns one hostmap.Candidate per eligible
// Service. Every candidate's ClassIndex is 0 (there is only one
// source-internal tier); callers merging candidates from other sources
// should offset ClassIndex before calling hostmap.Resolve so ties fall back
// to source order.
func (f *Filter) ExtractHostnameCandidates(services []corev1.Service) []hostmap.Candidate {
	var candidates []hostmap.Candidate

	for i := range services {
		svc := &services[i]
		if !f.ShouldProcessService(svc) {
			continue
		}

		candidates = append(candidates, hostmap.Candidate{
			Host:       svc.Annotations[f.hostnameAnnotationKey],
			CNAME:      fmt.Sprintf("%s.%s.svc.%s.", svc.Name, svc.Namespace, f.clusterDomain),
			Priority:   filterutil.ResolvePriority(svc.Annotations, f.annotationPriorityKey),
			ClassIndex: 0,
			Source:     "Service:" + svc.Namespace + "/" + svc.Name,
		})
	}

	return candidates
}
