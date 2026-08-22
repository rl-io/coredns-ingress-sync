package controller

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/gatewayapi"
	"github.com/rl-io/coredns-ingress-sync/internal/hostmap"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
	"github.com/rl-io/coredns-ingress-sync/internal/metrics"
	"github.com/rl-io/coredns-ingress-sync/internal/traefik"
	traefikv1alpha1 "github.com/rl-io/coredns-ingress-sync/internal/traefik/v1alpha1"
)

// IngressReconciler reconciles Ingress, Gateway API, and Traefik IngressRoute
// objects and updates CoreDNS configuration.
type IngressReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	IngressFilter *ingress.Filter
	// GatewayFilter is nil when Gateway API support is disabled (no
	// GatewayClassMappings configured). When nil, Reconcile makes no List/Get
	// calls of any kind against Gateway/HTTPRoute types, so pure-Ingress
	// deployments are unaffected even if those CRDs aren't installed.
	GatewayFilter *gatewayapi.Filter
	// TraefikFilter is nil when Traefik IngressRoute support is disabled (no
	// IngressRouteTargetCNAME configured). When nil, Reconcile makes no
	// List/Get calls of any kind against IngressRoute types.
	TraefikFilter  *traefik.Filter
	CoreDNSManager *coredns.Manager
}

// NewIngressReconciler creates a new IngressReconciler. gatewayFilter and
// traefikFilter may be nil to disable their respective support entirely.
func NewIngressReconciler(client client.Client, scheme *runtime.Scheme, ingressFilter *ingress.Filter, gatewayFilter *gatewayapi.Filter, traefikFilter *traefik.Filter, coreDNSManager *coredns.Manager) *IngressReconciler {
	return &IngressReconciler{
		Client:         client,
		Scheme:         scheme,
		IngressFilter:  ingressFilter,
		GatewayFilter:  gatewayFilter,
		TraefikFilter:  traefikFilter,
		CoreDNSManager: coreDNSManager,
	}
}

// Reconcile handles reconciliation requests for ingress changes
func (r *IngressReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	startTime := time.Now()
	logger := ctrl.LoggerFrom(ctx)

	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		podName = "unknown-pod"
	}

	logger.Info("Reconciling changes",
		"pod", podName,
		"request", req.NamespacedName.String())

	// List ingresses with namespace filtering
	var ingressList networkingv1.IngressList
	watchNamespaces := r.IngressFilter.GetWatchNamespaces()

	if r.IngressFilter.WatchesAllNamespaces() {
		// List all ingresses
		if err := r.List(ctx, &ingressList); err != nil {
			logger.Error(err, "Failed to list ingresses")
			duration := time.Since(startTime).Seconds()
			metrics.RecordReconciliationError(duration, "ingress_list")
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
	} else {
		// List ingresses from specific namespaces
		for _, ns := range watchNamespaces {
			var nsIngressList networkingv1.IngressList
			if err := r.List(ctx, &nsIngressList, client.InNamespace(ns)); err != nil {
				logger.Error(err, "Failed to list ingresses in namespace", "namespace", ns)
				continue
			}
			ingressList.Items = append(ingressList.Items, nsIngressList.Items...)
		}
	}

	// Collect hostname candidates from Ingress, offset Gateway API
	// candidates so Ingress wins ties by default, then resolve the combined
	// set to hostname -> target CNAME mappings.
	candidates := r.IngressFilter.ExtractHostnameCandidates(ingressList.Items)

	if r.GatewayFilter != nil && r.GatewayFilter.Enabled() {
		gatewayCandidates, err := r.extractGatewayCandidates(ctx, logger)
		if err != nil {
			duration := time.Since(startTime).Seconds()
			metrics.RecordReconciliationError(duration, "gateway_list")
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
		candidates = append(candidates, gatewayCandidates...)
	}

	if r.TraefikFilter != nil && r.TraefikFilter.Enabled() {
		traefikCandidates, err := r.extractTraefikCandidates(ctx, logger)
		if err != nil {
			duration := time.Since(startTime).Seconds()
			metrics.RecordReconciliationError(duration, "traefik_ingressroute_list")
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
		candidates = append(candidates, traefikCandidates...)
	}

	hostCNAMEMap := hostmap.Resolve(candidates, logger)

	// Collect hostnames for domain extraction and metrics.
	hosts := make([]string, 0, len(hostCNAMEMap))
	for host := range hostCNAMEMap {
		hosts = append(hosts, host)
	}

	// Extract unique domains from hosts
	domains := r.extractDomains(hosts)

	logger.V(1).Info("Processing ingresses",
		"domains", len(domains),
		"hosts", len(hosts),
		"domainList", domains)

	// Update metrics for ingresses and DNS records
	metrics.UpdateDNSRecordsCount(len(hosts))

	// Count ingresses per namespace
	namespaceCount := make(map[string]int)
	for _, ingress := range ingressList.Items {
		if r.IngressFilter.ShouldProcessIngress(&ingress) {
			namespaceCount[ingress.Namespace]++
		}
	}
	for namespace, count := range namespaceCount {
		metrics.UpdateIngressesWatched(namespace, count)
	}

	// Update dynamic ConfigMap with discovered domains
	if err := r.CoreDNSManager.UpdateDynamicConfigMap(ctx, domains, hostCNAMEMap); err != nil {
		logger.Error(err, "Failed to update dynamic ConfigMap")
		duration := time.Since(startTime).Seconds()
		metrics.RecordReconciliationError(duration, "dns_update")
		return reconcile.Result{RequeueAfter: time.Minute}, err
	}

	// Ensure CoreDNS ConfigMap has import statement and volume mount
	if err := r.CoreDNSManager.EnsureConfiguration(ctx); err != nil {
		logger.Error(err, "Failed to ensure CoreDNS configuration")
		duration := time.Since(startTime).Seconds()
		metrics.RecordReconciliationError(duration, "config_update")
		return reconcile.Result{RequeueAfter: time.Minute}, err
	}

	// Record successful reconciliation
	duration := time.Since(startTime).Seconds()
	metrics.RecordReconciliationSuccess(duration)

	logger.Info("Successfully updated CoreDNS configuration",
		"pod", podName,
		"domains", len(domains),
		"hosts", len(hosts))
	return reconcile.Result{}, nil
}

// extractGatewayCandidates lists Gateway and HTTPRoute objects (namespace
// scoped per r.GatewayFilter) and returns the resulting hostname candidates,
// with ClassIndex offset by the number of configured Ingress class mappings
// so Ingress wins ties by default when both sources claim the same hostname.
// Callers must only invoke this when r.GatewayFilter is non-nil and enabled.
func (r *IngressReconciler) extractGatewayCandidates(ctx context.Context, logger logr.Logger) ([]hostmap.Candidate, error) {
	watchNamespaces := r.GatewayFilter.GetWatchNamespaces()

	var gatewayList gatewayv1.GatewayList
	if r.GatewayFilter.WatchesAllNamespaces() {
		if err := r.List(ctx, &gatewayList); err != nil {
			logger.Error(err, "Failed to list gateways")
			return nil, err
		}
	} else {
		for _, ns := range watchNamespaces {
			var nsGatewayList gatewayv1.GatewayList
			if err := r.List(ctx, &nsGatewayList, client.InNamespace(ns)); err != nil {
				logger.Error(err, "Failed to list gateways in namespace", "namespace", ns)
				continue
			}
			gatewayList.Items = append(gatewayList.Items, nsGatewayList.Items...)
		}
	}

	var routeList gatewayv1.HTTPRouteList
	if r.GatewayFilter.WatchesAllNamespaces() {
		if err := r.List(ctx, &routeList); err != nil {
			logger.Error(err, "Failed to list httproutes")
			return nil, err
		}
	} else {
		for _, ns := range watchNamespaces {
			var nsRouteList gatewayv1.HTTPRouteList
			if err := r.List(ctx, &nsRouteList, client.InNamespace(ns)); err != nil {
				logger.Error(err, "Failed to list httproutes in namespace", "namespace", ns)
				continue
			}
			routeList.Items = append(routeList.Items, nsRouteList.Items...)
		}
	}

	refs := gatewayapi.BuildGatewayClassByRef(gatewayList.Items)
	candidates := r.GatewayFilter.ExtractHostnameCandidates(routeList.Items, refs)

	offset := r.IngressFilter.ClassCount()
	for i := range candidates {
		candidates[i].ClassIndex += offset
	}

	return candidates, nil
}

// extractTraefikCandidates lists IngressRoute objects (namespace scoped per
// r.TraefikFilter) and returns the resulting hostname candidates, with
// ClassIndex offset by the number of configured Ingress class mappings plus
// Gateway API GatewayClass mappings, so Ingress and then Gateway API win ties
// by default when multiple sources claim the same hostname. Callers must
// only invoke this when r.TraefikFilter is non-nil and enabled.
func (r *IngressReconciler) extractTraefikCandidates(ctx context.Context, logger logr.Logger) ([]hostmap.Candidate, error) {
	watchNamespaces := r.TraefikFilter.GetWatchNamespaces()

	var routeList traefikv1alpha1.IngressRouteList
	if r.TraefikFilter.WatchesAllNamespaces() {
		if err := r.List(ctx, &routeList); err != nil {
			logger.Error(err, "Failed to list ingressroutes")
			return nil, err
		}
	} else {
		for _, ns := range watchNamespaces {
			var nsRouteList traefikv1alpha1.IngressRouteList
			if err := r.List(ctx, &nsRouteList, client.InNamespace(ns)); err != nil {
				logger.Error(err, "Failed to list ingressroutes in namespace", "namespace", ns)
				continue
			}
			routeList.Items = append(routeList.Items, nsRouteList.Items...)
		}
	}

	candidates := r.TraefikFilter.ExtractHostnameCandidates(routeList.Items)

	offset := r.IngressFilter.ClassCount()
	if r.GatewayFilter != nil {
		offset += r.GatewayFilter.ClassCount()
	}
	for i := range candidates {
		candidates[i].ClassIndex += offset
	}

	return candidates, nil
}

// extractDomains extracts unique domains from a list of hostnames
func (r *IngressReconciler) extractDomains(hosts []string) []string {
	domainSet := make(map[string]bool)

	for _, host := range hosts {
		// Extract domain from hostname (everything after the first dot)
		parts := strings.Split(host, ".")
		if len(parts) > 1 {
			// Join all parts except the first (subdomain)
			domain := strings.Join(parts[1:], ".")
			domainSet[domain] = true
		}
	}

	var domains []string
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	return domains
}
