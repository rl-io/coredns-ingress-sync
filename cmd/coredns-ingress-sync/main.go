package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/rl-io/coredns-ingress-sync/internal/cache"
	"github.com/rl-io/coredns-ingress-sync/internal/cleanup"
	"github.com/rl-io/coredns-ingress-sync/internal/config"
	ingresscontroller "github.com/rl-io/coredns-ingress-sync/internal/controller"
	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
	"github.com/rl-io/coredns-ingress-sync/internal/logging"
	"github.com/rl-io/coredns-ingress-sync/internal/metrics"
	"github.com/rl-io/coredns-ingress-sync/internal/preflight"
	"github.com/rl-io/coredns-ingress-sync/internal/watches"
)

func main() {
	// Parse command line arguments
	var mode = flag.String("mode", "controller", "Mode to run: 'controller', 'cleanup', or 'preflight'")
	flag.Parse()

	// Setup logging with configurable level
	logging.Setup()

	// Get structured logger
	logger := ctrl.Log.WithName("main")

	switch *mode {
	case "cleanup":
		logger.Info("Starting cleanup mode")
		runCleanup(logger)
		return
	case "preflight":
		logger.Info("Starting preflight check mode")
		runPreflight(logger)
		return
	case "controller":
		logger.Info("Starting controller mode")
		runController(logger)
		return
	default:
		logger.Error(fmt.Errorf("invalid mode: %s", *mode), "Invalid mode specified. Use 'controller', 'cleanup', or 'preflight'", "mode", *mode)
		os.Exit(1)
	}
}

func runController(logger logr.Logger) {
	// Load configuration
	cfg := config.Load()

	// Parse watch namespaces
	watchNamespaces := cache.ParseNamespaces(cfg.WatchNamespaces)

	// Build cache options
	cacheBuilder := cache.NewConfigBuilder(watchNamespaces, cfg.CoreDNSNamespace)
	cacheOptions := cacheBuilder.BuildCacheOptions()

	// Create scheme and register all types before creating the manager
	scheme := runtime.NewScheme()
	if err := networkingv1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add networking/v1 to scheme")
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add core/v1 to scheme")
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add apps/v1 to scheme")
		os.Exit(1)
	}

	// Create the manager
	mgr, err := manager.New(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:                  scheme,
		LeaderElection:          cfg.LeaderElectionEnabled,
		LeaderElectionID:        "coredns-ingress-sync-leader",
		LeaderElectionNamespace: cfg.ControllerNamespace, // Use controller's own namespace, not CoreDNS namespace
		HealthProbeBindAddress:  ":8081",
		Cache:                   cacheOptions,
	})
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	// Create ingress filter
	ingressFilter := ingress.NewFilter(cfg.IngressClass, cfg.WatchNamespaces, cfg.ExcludeNamespaces, cfg.ExcludeIngresses, cfg.AnnotationEnabledKey)

	// Create CoreDNS manager
	coreDNSConfig := coredns.Config{
		Namespace:            cfg.CoreDNSNamespace,
		ConfigMapName:        cfg.CoreDNSConfigMapName,
		DynamicConfigMapName: cfg.DynamicConfigMapName,
		DynamicConfigKey:     cfg.DynamicConfigKey,
		ImportStatement:      cfg.ImportStatement,
		TargetCNAME:          cfg.TargetCNAME,
		VolumeName:           cfg.CoreDNSVolumeName,
		MountPath:            cfg.MountPath,
	}
	coreDNSManager := coredns.NewManager(mgr.GetClient(), coreDNSConfig)

	// Create the reconciler
	reconciler := ingresscontroller.NewIngressReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		ingressFilter,
		coreDNSManager,
	)

	// Set up the controller
	c, err := ctrlcontroller.New("coredns-ingress-sync", mgr, ctrlcontroller.Options{
		Reconciler: reconciler,
	})
	if err != nil {
		logger.Error(err, "Failed to create controller")
		os.Exit(1)
	}

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
			predicate.NewTypedPredicateFuncs(func(obj *networkingv1.Ingress) bool {
				// Comprehensive check including exclusions
				return ingressFilter.ShouldProcessIngress(obj)
			}))); err != nil {
		logger.Error(err, "Failed to set up ingress watch")
		os.Exit(1)
	}

	// Watch for CoreDNS ConfigMap changes
	watchManager := watches.NewManager()
	if err := watchManager.AddConfigMapWatch(mgr.GetCache(), c, cfg.CoreDNSNamespace, cfg.CoreDNSConfigMapName, "coredns-configmap-reconcile"); err != nil {
		logger.Error(err, "Failed to set up CoreDNS ConfigMap watch")
		os.Exit(1)
	}

	// Watch for dynamic ConfigMap changes (e.g., coredns-ingress-sync-rewrite-rules) - with smart filtering
	if err := watchManager.AddDynamicConfigMapWatch(mgr.GetCache(), c, cfg.CoreDNSNamespace, cfg.DynamicConfigMapName, "dynamic-configmap-reconcile"); err != nil {
		logger.Error(err, "Failed to set up dynamic ConfigMap watch")
		os.Exit(1)
	}

	logger.Info("Starting coredns-ingress-sync controller",
		"leader_election", cfg.LeaderElectionEnabled,
		"ingress_class", cfg.IngressClass,
		"target_cname", cfg.TargetCNAME,
		"dynamic_configmap", cfg.DynamicConfigMapName,
		"coredns_configmap", fmt.Sprintf("%s/%s", cfg.CoreDNSNamespace, cfg.CoreDNSConfigMapName))

	if len(watchNamespaces) > 0 {
		logger.Info("Watching specific namespaces", "namespaces", watchNamespaces)
	} else {
		logger.Info("Watching all namespaces")
	}

	// Add health check endpoints
	if err := mgr.AddHealthzCheck("healthz", func(req *http.Request) error {
		// Basic health check - always return healthy if the manager is running
		return nil
	}); err != nil {
		logger.Error(err, "Failed to add health check endpoint")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		// Ready check - always return ready since controller-runtime handles leader election internally
		// The manager will only start reconciling when it becomes the leader
		return nil
	}); err != nil {
		logger.Error(err, "Failed to add readiness check endpoint")
		os.Exit(1)
	}

	// Initialize leader election metrics
	if cfg.LeaderElectionEnabled {
		// Set initial leader status to false
		metrics.SetLeaderElectionStatus(false)

		// Add a callback to update leader election status
		// Note: controller-runtime doesn't provide direct leader election callbacks,
		// but we can monitor this in the reconciler or use a simple check
		logger.Info("Leader election enabled, metrics will be updated")
	} else {
		// If leader election is disabled, this instance is always the leader
		metrics.SetLeaderElectionStatus(true)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Failed to start manager")
		os.Exit(1)
	}
}

func runCleanup(logger logr.Logger) {
	// Load configuration
	cfg := config.Load()
	logger.Info("Starting cleanup mode",
		"coredns_namespace", cfg.CoreDNSNamespace,
		"dynamic_configmap", cfg.DynamicConfigMapName)

	// Create cleanup manager
	cleanupManager, err := cleanup.NewManager(logger)
	if err != nil {
		logger.Error(err, "Failed to create cleanup manager")
		os.Exit(1)
	}

	// Run cleanup operations
	if err := cleanupManager.Run(cfg); err != nil {
		logger.Error(err, "Cleanup failed")
		os.Exit(1)
	}
}

func runPreflight(logger logr.Logger) {
	// Load configuration
	cfg := config.Load()
	logger.Info("Starting preflight checks")

	// Create scheme for Kubernetes client
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add apps/v1 to scheme")
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add core/v1 to scheme")
		os.Exit(1)
	}

	// Create direct Kubernetes client (not using manager/cache for one-shot operation)
	kubeConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(kubeConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client for preflight checks")
		os.Exit(1)
	}

	// Create preflight config from environment
	preflightConfig := preflight.ConfigFromEnv(cfg)

	// Override with environment variables specific to preflight
	if deploymentName := os.Getenv("DEPLOYMENT_NAME"); deploymentName != "" {
		preflightConfig.DeploymentName = deploymentName
	}
	if releaseInstance := os.Getenv("RELEASE_INSTANCE"); releaseInstance != "" {
		preflightConfig.ReleaseInstance = releaseInstance
	}

	// Create preflight checker with direct client
	checker := preflight.NewChecker(k8sClient, preflightConfig, logger)

	// Run preflight checks
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	
	logger.Info("Starting preflight checks with timeout", "timeout", "90s")
	results, err := checker.RunChecks(ctx)
	if err != nil {
		logger.Error(err, "Failed to run preflight checks")
		os.Exit(1)
	}

	// Print results
	checker.PrintResults(results)

	// Exit with error code if any checks failed
	if preflight.HasErrors(results) {
		os.Exit(1)
	}
}
