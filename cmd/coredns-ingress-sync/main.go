package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
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
	if err := appsv1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
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

	// Add health check endpoints
	if err := mgr.AddHealthzCheck("healthz", func(req *http.Request) error {
		// Basic health check - always return healthy if the manager is running
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		// Ready check - always return ready since controller-runtime handles leader election internally
		// The manager will only start reconciling when it becomes the leader
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatal(err)
	}
}

func runCleanup() {
	// Load configuration
	cfg := config.Load()
	log.Printf("Cleanup mode - removing CoreDNS configuration and dynamic ConfigMap: %s/%s", cfg.CoreDNSNamespace, cfg.DynamicConfigMapName)

	// Create a simple client for cleanup operations
	clientConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		log.Fatal(err)
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatal(err)
	}

	// Create CoreDNS manager for cleanup operations
	coreDNSConfig := coredns.Config{
		Namespace:            cfg.CoreDNSNamespace,
		ConfigMapName:        cfg.CoreDNSConfigMapName,
		DynamicConfigMapName: cfg.DynamicConfigMapName,
		DynamicConfigKey:     cfg.DynamicConfigKey,
		ImportStatement:      cfg.ImportStatement,
		TargetCNAME:          cfg.TargetCNAME,
	}
	coreDNSManager := coredns.NewManager(k8sClient, coreDNSConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Remove import statement from CoreDNS Corefile
	if err := removeCoreDNSImport(ctx, coreDNSManager, cfg); err != nil {
		log.Printf("Warning: Failed to remove import statement from CoreDNS: %v", err)
	}

	// Step 2: Remove volume mount from CoreDNS deployment
	if err := removeCoreDNSVolumeMount(ctx, coreDNSManager, cfg); err != nil {
		log.Printf("Warning: Failed to remove volume mount from CoreDNS deployment: %v", err)
	}

	// Step 3: Delete the dynamic ConfigMap
	configMap := &corev1.ConfigMap{}
	configMapName := types.NamespacedName{
		Name:      cfg.DynamicConfigMapName,
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := k8sClient.Get(ctx, configMapName, configMap); err != nil {
		log.Printf("Dynamic ConfigMap %s not found or already deleted: %v", cfg.DynamicConfigMapName, err)
	} else {
		if err := k8sClient.Delete(ctx, configMap); err != nil {
			log.Printf("Failed to delete dynamic ConfigMap %s: %v", cfg.DynamicConfigMapName, err)
			os.Exit(1)
		}
		log.Printf("Successfully deleted dynamic ConfigMap %s", cfg.DynamicConfigMapName)
	}

	log.Printf("Cleanup completed successfully")
}

// removeCoreDNSImport removes the import statement from CoreDNS Corefile
func removeCoreDNSImport(ctx context.Context, coreDNSManager *coredns.Manager, cfg *config.Config) error {
	// Get the CoreDNS ConfigMap directly
	clientConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	coreDNSConfigMap := &corev1.ConfigMap{}
	coreDNSConfigMapName := types.NamespacedName{
		Name:      cfg.CoreDNSConfigMapName,
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := k8sClient.Get(ctx, coreDNSConfigMapName, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to get CoreDNS ConfigMap: %w", err)
	}

	// Check if Corefile exists
	corefile, exists := coreDNSConfigMap.Data["Corefile"]
	if !exists {
		return fmt.Errorf("corefile not found in CoreDNS ConfigMap")
	}

	// Remove import statement if it exists
	if !strings.Contains(corefile, cfg.ImportStatement) {
		log.Printf("Import statement not found in CoreDNS Corefile - already removed")
		return nil
	}

	// Remove the import statement line
	lines := strings.Split(corefile, "\n")
	var newLines []string

	for _, line := range lines {
		// Skip lines that contain the import statement
		if !strings.Contains(line, cfg.ImportStatement) {
			newLines = append(newLines, line)
		}
	}

	// Update the ConfigMap
	newCorefile := strings.Join(newLines, "\n")
	coreDNSConfigMap.Data["Corefile"] = newCorefile

	if err := k8sClient.Update(ctx, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to update CoreDNS ConfigMap: %w", err)
	}

	log.Printf("Removed import statement from CoreDNS Corefile")
	return nil
}

// removeCoreDNSVolumeMount removes the volume mount from CoreDNS deployment
func removeCoreDNSVolumeMount(ctx context.Context, coreDNSManager *coredns.Manager, cfg *config.Config) error {
	// Get the CoreDNS deployment directly
	clientConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		return err
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	deployment := &appsv1.Deployment{}
	deploymentName := types.NamespacedName{
		Name:      "coredns",
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := k8sClient.Get(ctx, deploymentName, deployment); err != nil {
		return fmt.Errorf("failed to get CoreDNS deployment: %w", err)
	}

	modified := false

	// Remove volume if it exists
	var newVolumes []corev1.Volume
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "custom-config-volume" {
			newVolumes = append(newVolumes, volume)
		} else {
			modified = true
		}
	}
	deployment.Spec.Template.Spec.Volumes = newVolumes

	// Remove volume mount from CoreDNS container
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "coredns" {
			var newVolumeMounts []corev1.VolumeMount
			for _, volumeMount := range container.VolumeMounts {
				if volumeMount.Name != "custom-config-volume" {
					newVolumeMounts = append(newVolumeMounts, volumeMount)
				} else {
					modified = true
				}
			}
			deployment.Spec.Template.Spec.Containers[i].VolumeMounts = newVolumeMounts
			break
		}
	}

	if modified {
		if err := k8sClient.Update(ctx, deployment); err != nil {
			return fmt.Errorf("failed to update CoreDNS deployment: %w", err)
		}
		log.Printf("Removed custom config volume mount from CoreDNS deployment")
	} else {
		log.Printf("Custom config volume mount not found in CoreDNS deployment - already removed")
	}

	return nil
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
				// Only trigger on the specific dynamic ConfigMap
				if obj.GetNamespace() == namespace && obj.GetName() == name {
					return []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Name:      reconcileName,
							Namespace: "default",
						},
					}}
				}
				return []reconcile.Request{}
			}),
			predicate.TypedFuncs[*corev1.ConfigMap]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.ConfigMap]) bool {
					// Don't trigger on create events - we create the ConfigMap ourselves
					return false
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
					// Only watch the specific dynamic ConfigMap
					if e.ObjectNew.GetNamespace() != namespace || e.ObjectNew.GetName() != name {
						return false
					}
					
					// Only trigger on updates that are NOT from us
					// If the ConfigMap has our management label, it means we updated it, so ignore
					labels := e.ObjectNew.GetLabels()
					if labels != nil && labels["app.kubernetes.io/managed-by"] == "coredns-ingress-sync" {
						return false // Ignore our own updates
					}
					
					// Trigger on external updates (like Terraform removing our ConfigMap)
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.ConfigMap]) bool {
					// Trigger on delete for disaster recovery
					return e.Object.GetNamespace() == namespace && e.Object.GetName() == name
				},
			}))
}
