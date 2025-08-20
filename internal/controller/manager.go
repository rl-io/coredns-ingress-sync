package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rl-io/coredns-ingress-sync/internal/cache"
	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/watches"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

// Reconciler interface to avoid import cycle
type Reconciler interface {
	Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error)
}

// ReconcilerFactory creates reconcilers to avoid import cycles
type ReconcilerFactory interface {
	NewIngressReconciler(client, scheme, ingressFilter, coreDNSManager interface{}) Reconciler
}

// ControllerManager manages the setup and running of the controller
type ControllerManager struct {
	logger     logr.Logger
	config     *config.Config
	reconciler Reconciler
}

// NewControllerManager creates a new controller manager
func NewControllerManager(logger logr.Logger, cfg *config.Config, reconciler Reconciler) *ControllerManager {
	return &ControllerManager{
		logger:     logger,
		config:     cfg,
		reconciler: reconciler,
	}
}

// Setup creates and configures the controller manager and all watches
func (cm *ControllerManager) Setup() (manager.Manager, error) {
	// Parse watch namespaces
	watchNamespaces := cache.ParseNamespaces(cm.config.WatchNamespaces)
	
	// Build cache options
	cacheBuilder := cache.NewConfigBuilder(watchNamespaces, cm.config.CoreDNSNamespace)
	cacheOptions := cacheBuilder.BuildCacheOptions()

	// Create scheme and register all types before creating the manager
	scheme := runtime.NewScheme()
	if err := networkingv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add networking/v1 to scheme: %w", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core/v1 to scheme: %w", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add apps/v1 to scheme: %w", err)
	}

	// Create the manager
	mgr, err := manager.New(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:                  scheme,
		LeaderElection:          cm.config.LeaderElectionEnabled,
		LeaderElectionID:        "coredns-ingress-sync-leader",
		LeaderElectionNamespace: cm.config.ControllerNamespace, // Use controller's own namespace, not CoreDNS namespace
		HealthProbeBindAddress:  ":8081",
		Cache:                   cacheOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create manager: %w", err)
	}

	// Create ingress filter for watches
	ingressFilter := ingress.NewFilter(cm.config.IngressClass, cm.config.WatchNamespaces, cm.config.ExcludeNamespaces, cm.config.ExcludeIngresses, cm.config.AnnotationEnabledKey)

	// Set up the controller using the provided reconciler
	c, err := ctrlcontroller.New("coredns-ingress-sync", mgr, ctrlcontroller.Options{
		Reconciler: cm.reconciler,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller: %w", err)
	}

	// Set up watches
	if err := cm.setupWatches(mgr, c, ingressFilter); err != nil {
		return nil, fmt.Errorf("failed to setup watches: %w", err)
	}

	// Add health checks
	if err := cm.setupHealthChecks(mgr); err != nil {
		return nil, fmt.Errorf("failed to setup health checks: %w", err)
	}

	// Log startup information
	cm.logStartupInfo(watchNamespaces)

	return mgr, nil
}

// setupWatches configures all the controller watches
func (cm *ControllerManager) setupWatches(mgr manager.Manager, c ctrlcontroller.Controller, ingressFilter *ingress.Filter) error {
	// Watch for Ingress changes
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &networkingv1.Ingress{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj *networkingv1.Ingress) []reconcile.Request {
				// Always trigger a reconcile for any ingress change
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{
						Name:      "global-ingress-reconcile",
						Namespace: "default",
					},
				}}
			}),
			buildIngressPredicate(ingressFilter))); err != nil {
		return fmt.Errorf("failed to set up ingress watch: %w", err)
	}

	// Watch for CoreDNS ConfigMap changes
	watchManager := watches.NewManager()
	if err := watchManager.AddConfigMapWatch(mgr.GetCache(), c, cm.config.CoreDNSNamespace, cm.config.CoreDNSConfigMapName, "coredns-configmap-reconcile"); err != nil {
		return fmt.Errorf("failed to set up CoreDNS ConfigMap watch: %w", err)
	}

	// Watch for dynamic ConfigMap changes (e.g., coredns-ingress-sync-rewrite-rules) - with smart filtering
	if err := watchManager.AddDynamicConfigMapWatch(mgr.GetCache(), c, cm.config.CoreDNSNamespace, cm.config.DynamicConfigMapName, "dynamic-configmap-reconcile"); err != nil {
		return fmt.Errorf("failed to set up dynamic ConfigMap watch: %w", err)
	}

	return nil
}

// buildIngressPredicate creates a predicate that triggers reconciles for:
// - Create: only if the ingress should be processed
// - Update: if either the old or new ingress should be processed (captures transitions)
// - Delete: always trigger so we can recompute rules on removal
// This ensures annotation toggles or exclusion changes still enqueue a reconcile.
func buildIngressPredicate(ingressFilter *ingress.Filter) predicate.TypedPredicate[*networkingv1.Ingress] {
	return predicate.TypedFuncs[*networkingv1.Ingress]{
		CreateFunc: func(e event.TypedCreateEvent[*networkingv1.Ingress]) bool {
			return ingressFilter.ShouldProcessIngress(e.Object)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*networkingv1.Ingress]) bool {
			// Trigger when either old or new state qualifies, so transitions (e.g., annotation flips) enqueue reconciles
			return ingressFilter.ShouldProcessIngress(e.ObjectOld) || ingressFilter.ShouldProcessIngress(e.ObjectNew)
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*networkingv1.Ingress]) bool {
			// Always reconcile on delete to prune rewrite rules
			return true
		},
	}
}

// setupHealthChecks adds health and readiness check endpoints
func (cm *ControllerManager) setupHealthChecks(mgr manager.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", func(req *http.Request) error {
		// Basic health check - always return healthy if the manager is running
		return nil
	}); err != nil {
		return fmt.Errorf("failed to add health check endpoint: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		// Ready check - always return ready since controller-runtime handles leader election internally
		// The manager will only start reconciling when it becomes the leader
		return nil
	}); err != nil {
		return fmt.Errorf("failed to add readiness check endpoint: %w", err)
	}

	return nil
}

// logStartupInfo logs information about the controller startup
func (cm *ControllerManager) logStartupInfo(watchNamespaces []string) {
	cm.logger.Info("Starting coredns-ingress-sync controller",
		"leader_election", cm.config.LeaderElectionEnabled,
		"ingress_class", cm.config.IngressClass,
		"target_cname", cm.config.TargetCNAME,
		"dynamic_configmap", cm.config.DynamicConfigMapName,
		"coredns_configmap", fmt.Sprintf("%s/%s", cm.config.CoreDNSNamespace, cm.config.CoreDNSConfigMapName))
	
	if len(watchNamespaces) > 0 {
		cm.logger.Info("Watching specific namespaces", "namespaces", watchNamespaces)
	} else {
		cm.logger.Info("Watching all namespaces")
	}
}
