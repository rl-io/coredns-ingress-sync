package controller

import (
	"context"
	"os"
	"strings"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

// IngressReconciler reconciles Ingress objects and updates CoreDNS configuration
type IngressReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	IngressFilter *ingress.Filter
	CoreDNSManager *coredns.Manager
}

// NewIngressReconciler creates a new IngressReconciler
func NewIngressReconciler(client client.Client, scheme *runtime.Scheme, ingressFilter *ingress.Filter, coreDNSManager *coredns.Manager) *IngressReconciler {
	return &IngressReconciler{
		Client:         client,
		Scheme:         scheme,
		IngressFilter:  ingressFilter,
		CoreDNSManager: coreDNSManager,
	}
}

// Reconcile handles reconciliation requests for ingress changes
func (r *IngressReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
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

	// Extract hostnames from target ingresses
	hosts := r.IngressFilter.ExtractHostnames(ingressList.Items)

	// Extract unique domains from hosts
	domains := r.extractDomains(hosts)

	logger.V(1).Info("Processing ingresses", 
		"domains", len(domains), 
		"hosts", len(hosts),
		"domainList", domains)

	// Update dynamic ConfigMap with discovered domains
	if err := r.CoreDNSManager.UpdateDynamicConfigMap(ctx, domains, hosts); err != nil {
		logger.Error(err, "Failed to update dynamic ConfigMap")
		return reconcile.Result{RequeueAfter: time.Minute}, err
	}

	// Ensure CoreDNS ConfigMap has import statement and volume mount
	if err := r.CoreDNSManager.EnsureConfiguration(ctx); err != nil {
		logger.Error(err, "Failed to ensure CoreDNS configuration")
		return reconcile.Result{RequeueAfter: time.Minute}, err
	}

	logger.Info("Successfully updated CoreDNS configuration", 
		"pod", podName,
		"domains", len(domains), 
		"hosts", len(hosts))
	return reconcile.Result{}, nil
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
