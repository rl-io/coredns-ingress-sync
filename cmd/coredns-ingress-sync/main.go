package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cacheconfig "github.com/rl-io/coredns-ingress-sync/internal/cache"
	"github.com/rl-io/coredns-ingress-sync/internal/config"
	ingresscontroller "github.com/rl-io/coredns-ingress-sync/internal/controller"
	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

func main() {
	// Parse command line arguments
	var mode = flag.String("mode", "controller", "Mode to run: 'controller' or 'cleanup'")
	flag.Parse()

	// Setup logging
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	switch *mode {
	case "cleanup":
		log.Printf("Starting cleanup mode...")
		runCleanup()
		return
	case "controller":
		log.Printf("Starting controller mode...")
		runController()
		return
	default:
		log.Fatalf("Invalid mode: %s. Use 'controller' or 'cleanup'", *mode)
	}
}

func runController() {
	// Load configuration
	cfg := config.Load()
	
	// Parse watch namespaces
	watchNamespaces := cacheconfig.ParseNamespaces(cfg.WatchNamespaces)
	
	// Build cache options
	cacheBuilder := cacheconfig.NewConfigBuilder(watchNamespaces, cfg.CoreDNSNamespace)
	cacheOptions := cacheBuilder.BuildCacheOptions()

	// Create scheme and register all types before creating the manager
	scheme := runtime.NewScheme()
	if err := networkingv1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
	}

	// Create the manager
	mgr, err := manager.New(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:                  scheme,
		LeaderElection:          cfg.LeaderElectionEnabled,
		LeaderElectionID:        "coredns-ingress-sync-leader",
		LeaderElectionNamespace: cfg.CoreDNSNamespace,
		Cache:                   cacheOptions,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create ingress filter
	ingressFilter := ingress.NewFilter(cfg.IngressClass, cfg.WatchNamespaces)

	// Create CoreDNS manager
	coreDNSConfig := coredns.Config{
		Namespace:            cfg.CoreDNSNamespace,
		ConfigMapName:        cfg.CoreDNSConfigMapName,
		DynamicConfigMapName: cfg.DynamicConfigMapName,
		DynamicConfigKey:     cfg.DynamicConfigKey,
		ImportStatement:      cfg.ImportStatement,
		TargetCNAME:          cfg.TargetCNAME,
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
		log.Fatal(err)
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
				// Check if ingress matches our class and namespace filtering
				if !ingressFilter.IsTargetIngress(obj) {
					return false
				}
				// If specific namespaces are configured, check if this ingress is in one of them
				return ingressFilter.ShouldWatchNamespace(obj.GetNamespace())
			}))); err != nil {
		log.Fatal(err)
	}

	// Watch for CoreDNS ConfigMap changes
	if err := addConfigMapWatch(mgr.GetCache(), c, cfg.CoreDNSNamespace, cfg.CoreDNSConfigMapName, "coredns-configmap-reconcile"); err != nil {
		log.Fatal(err)
	}

	// Watch for dynamic ConfigMap changes (e.g., coredns-custom) - with smart filtering
	if err := addDynamicConfigMapWatch(mgr.GetCache(), c, cfg.CoreDNSNamespace, cfg.DynamicConfigMapName, "dynamic-configmap-reconcile"); err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting coredns-ingress-sync controller")
	log.Printf("Leader election enabled: %t", cfg.LeaderElectionEnabled)
	log.Printf("Watching ingresses with class: %s", cfg.IngressClass)
	if len(watchNamespaces) > 0 {
		log.Printf("Watching namespaces: %v", watchNamespaces)
	} else {
		log.Printf("Watching all namespaces")
	}
	log.Printf("Target CNAME: %s", cfg.TargetCNAME)
	log.Printf("Dynamic ConfigMap: %s", cfg.DynamicConfigMapName)
	log.Printf("CoreDNS ConfigMap: %s/%s", cfg.CoreDNSNamespace, cfg.CoreDNSConfigMapName)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatal(err)
	}
}

func runCleanup() {
	// Load configuration
	cfg := config.Load()
	log.Printf("Cleanup mode - removing dynamic ConfigMap: %s/%s", cfg.CoreDNSNamespace, cfg.DynamicConfigMapName)

	// Create a simple client for cleanup operations
	clientConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatal(err)
	}

	// Create CoreDNS manager for cleanup (not used but structured for future cleanup logic)
	coreDNSConfig := coredns.Config{
		Namespace:            cfg.CoreDNSNamespace,
		ConfigMapName:        cfg.CoreDNSConfigMapName,
		DynamicConfigMapName: cfg.DynamicConfigMapName,
		DynamicConfigKey:     cfg.DynamicConfigKey,
		ImportStatement:      cfg.ImportStatement,
		TargetCNAME:          cfg.TargetCNAME,
	}
	_ = coredns.NewManager(k8sClient, coreDNSConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete the dynamic ConfigMap
	configMap := &corev1.ConfigMap{}
	configMapName := types.NamespacedName{
		Name:      cfg.DynamicConfigMapName,
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := k8sClient.Get(ctx, configMapName, configMap); err != nil {
		log.Printf("Dynamic ConfigMap %s not found or already deleted: %v", cfg.DynamicConfigMapName, err)
		return
	}

	if err := k8sClient.Delete(ctx, configMap); err != nil {
		log.Printf("Failed to delete dynamic ConfigMap %s: %v", cfg.DynamicConfigMapName, err)
		os.Exit(1)
	}

	log.Printf("Successfully deleted dynamic ConfigMap %s", cfg.DynamicConfigMapName)
}

func addConfigMapWatch(cache cache.Cache, c ctrlcontroller.Controller, namespace, name, reconcileName string) error {
	return c.Watch(
		source.Kind(cache, &corev1.ConfigMap{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj *corev1.ConfigMap) []reconcile.Request {
				if obj.GetNamespace() == namespace && obj.GetName() == name {
					return []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Name:      reconcileName,
							Namespace: "default",
						},
					}}
				}
				return []reconcile.Request{}
			})))
}

func addDynamicConfigMapWatch(cache cache.Cache, c ctrlcontroller.Controller, namespace, name, reconcileName string) error {
	return c.Watch(
		source.Kind(cache, &corev1.ConfigMap{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj *corev1.ConfigMap) []reconcile.Request {
				// Only trigger on the specific dynamic ConfigMap and filter for our managed ConfigMaps
				if obj.GetNamespace() == namespace && obj.GetName() == name {
					if labels := obj.GetLabels(); labels != nil && labels["app.kubernetes.io/managed-by"] == "coredns-ingress-sync" {
						return []reconcile.Request{{
							NamespacedName: types.NamespacedName{
								Name:      reconcileName,
								Namespace: "default",
							},
						}}
					}
				}
				return []reconcile.Request{}
			})))
}
