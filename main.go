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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	ingressClass          = getEnvOrDefault("INGRESS_CLASS", "nginx")
	targetCNAME           = getEnvOrDefault("TARGET_CNAME", "ingress-nginx-controller.ingress-nginx.svc.cluster.local.")
	dynamicConfigMapName  = getEnvOrDefault("DYNAMIC_CONFIGMAP_NAME", "coredns-custom")
	dynamicConfigKey      = getEnvOrDefault("DYNAMIC_CONFIG_KEY", "dynamic.server")
	coreDNSNamespace      = getEnvOrDefault("COREDNS_NAMESPACE", "kube-system")
	coreDNSConfigMapName  = getEnvOrDefault("COREDNS_CONFIGMAP_NAME", "coredns")
	leaderElectionEnabled = getEnvOrDefault("LEADER_ELECTION_ENABLED", "true") == "true"
	watchNamespaces       = getEnvOrDefault("WATCH_NAMESPACES", "") // Comma-separated list, empty = all namespaces
	importStatement       = "import /etc/coredns/custom/*.server"
)

type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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
	// Parse watch namespaces
	var namespaces []string
	var cacheOptions cache.Options

	if watchNamespaces != "" {
		namespaces = strings.Split(strings.ReplaceAll(watchNamespaces, " ", ""), ",")
		// Filter out empty strings
		var validNamespaces []string
		for _, ns := range namespaces {
			if ns != "" {
				validNamespaces = append(validNamespaces, ns)
			}
		}
		namespaces = validNamespaces

		// Configure cache to watch specific namespaces
		if len(namespaces) > 0 {
			// Create namespace map for ingresses (only the specified namespaces)
			ingressNamespaceMap := make(map[string]cache.Config)
			for _, ns := range namespaces {
				ingressNamespaceMap[ns] = cache.Config{}
			}
			
			// Configure cache with ByObject for namespace-scoped resources
			cacheOptions.ByObject = map[client.Object]cache.ByObject{
				&networkingv1.Ingress{}: {
					Namespaces: ingressNamespaceMap,
				},
				&corev1.ConfigMap{}: {
					Namespaces: map[string]cache.Config{
						coreDNSNamespace: {},
					},
				},
			}
			log.Printf("Configured to watch specific namespaces: %v (plus %s for CoreDNS)", namespaces, coreDNSNamespace)
		}
	} else {
		log.Printf("Configured to watch all namespaces")
	}

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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          leaderElectionEnabled,
		LeaderElectionID:        "coredns-ingress-sync-leader",
		LeaderElectionNamespace: "", // Uses the same namespace as the pod
		HealthProbeBindAddress:  ":8081",
		Cache:                   cacheOptions,
	})
	if err != nil {
		log.Fatal(err)
	}

	reconciler := &IngressReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	// Set up the controller
	c, err := controller.New("coredns-ingress-sync", mgr, controller.Options{
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
				if !isTargetIngress(obj) {
					return false
				}
				// If specific namespaces are configured, check if this ingress is in one of them
				if len(namespaces) > 0 {
					for _, ns := range namespaces {
						if obj.GetNamespace() == ns {
							return true
						}
					}
					return false
				}
				return true
			}))); err != nil {
		log.Fatal(err)
	}

	// Watch for CoreDNS ConfigMap changes
	if err := addConfigMapWatch(mgr.GetCache(), c, coreDNSNamespace, coreDNSConfigMapName, "coredns-configmap-reconcile"); err != nil {
		log.Fatal(err)
	}

	// Watch for dynamic ConfigMap changes (e.g., coredns-custom) - with smart filtering
	if err := addDynamicConfigMapWatch(mgr.GetCache(), c, coreDNSNamespace, dynamicConfigMapName, "dynamic-configmap-reconcile"); err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting coredns-ingress-sync controller")
	log.Printf("Leader election enabled: %t", leaderElectionEnabled)
	log.Printf("Watching ingresses with class: %s", ingressClass)
	if len(namespaces) > 0 {
		log.Printf("Watching namespaces: %v", namespaces)
	} else {
		log.Printf("Watching all namespaces")
	}
	log.Printf("Target CNAME: %s", targetCNAME)
	log.Printf("Dynamic ConfigMap: %s", dynamicConfigMapName)
	log.Printf("CoreDNS ConfigMap: %s/%s", coreDNSNamespace, coreDNSConfigMapName)

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

// runCleanup performs cleanup operations when uninstalling
func runCleanup() {
	log.Printf("Starting CoreDNS configuration cleanup...")

	// Wait for controller pods to terminate first
	waitForControllerTermination()

	// Perform cleanup operations
	cleanupCoreDNSConfigMap()
	cleanupCoreDNSDeployment()
	cleanupDynamicConfigMap()

	log.Printf("✅ CoreDNS cleanup completed successfully")
}

// waitForControllerTermination waits for all controller pods to terminate
func waitForControllerTermination() {
	log.Printf("Waiting for coredns-ingress-sync controller pods to terminate...")

	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create Kubernetes client: %v", err)
		return
	}

	// Get the current namespace
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		// Fallback to reading from service account token
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			namespace = string(data)
		} else {
			log.Printf("Warning: Could not determine current namespace, using 'default'")
			namespace = "default"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	maxWait := 2 * time.Minute // Reduced from 5 minutes since deployment might already be deleted
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeout := time.After(maxWait)

	// Do an initial check - if no controller pods are found immediately, proceed
	initialCheck := func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=coredns-ingress-sync",
			FieldSelector: "status.phase=Running",
		})
		if err != nil {
			log.Printf("Failed to list controller pods during initial check: %v", err)
			return false
		}

		// Filter out cleanup job pods (they contain "cleanup" in the name)
		var controllerPods []corev1.Pod
		for _, pod := range pods.Items {
			if !strings.Contains(pod.Name, "cleanup") {
				controllerPods = append(controllerPods, pod)
			}
		}

		if len(controllerPods) == 0 {
			log.Printf("✅ No controller pods found - deployment likely already deleted")
			return true
		}

		log.Printf("Found %d controller pods, waiting for termination...", len(controllerPods))
		for _, pod := range controllerPods {
			log.Printf("  - %s/%s", pod.Namespace, pod.Name)
		}
		return false
	}

	if initialCheck() {
		return
	}

	for {
		select {
		case <-timeout:
			log.Printf("⚠️  Warning: Controller pods may still be running after %v wait", maxWait)
			log.Printf("Proceeding with cleanup, but changes may be reverted by running controllers")
			listRunningControllers(clientset, ctx, namespace)
			return
		case <-ticker.C:
			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=coredns-ingress-sync",
				FieldSelector: "status.phase=Running",
			})
			if err != nil {
				log.Printf("Failed to list controller pods: %v", err)
				continue
			}

			// Filter out cleanup job pods (they contain "cleanup" in the name)
			var controllerPods []corev1.Pod
			for _, pod := range pods.Items {
				if !strings.Contains(pod.Name, "cleanup") {
					controllerPods = append(controllerPods, pod)
				}
			}

			if len(controllerPods) == 0 {
				log.Printf("✅ All coredns-ingress-sync controller pods have terminated")
				return
			}

			log.Printf("⏳ Found %d running controller pods, waiting...", len(controllerPods))
			for _, pod := range controllerPods {
				log.Printf("  - %s/%s", pod.Namespace, pod.Name)
			}
		}
	}
}

// listRunningControllers lists any remaining running controller pods
func listRunningControllers(clientset *kubernetes.Clientset, ctx context.Context, namespace string) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=coredns-ingress-sync",
	})
	if err != nil {
		log.Printf("Failed to list controller pods: %v", err)
		return
	}

	// Filter out cleanup job pods (they contain "cleanup" in the name)
	var controllerPods []corev1.Pod
	for _, pod := range pods.Items {
		if !strings.Contains(pod.Name, "cleanup") {
			controllerPods = append(controllerPods, pod)
		}
	}

	if len(controllerPods) > 0 {
		log.Printf("Remaining controller pods:")
		for _, pod := range controllerPods {
			log.Printf("  - %s/%s (status: %s)", pod.Namespace, pod.Name, pod.Status.Phase)
		}
	}
}

// cleanupCoreDNSConfigMap removes the import statement from CoreDNS ConfigMap
func cleanupCoreDNSConfigMap() {
	log.Printf("Checking for CoreDNS ConfigMap...")

	ctx := context.Background()
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset: %v", err)
		return
	}

	configMap, err := clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(ctx, coreDNSConfigMapName, metav1.GetOptions{})
	if err != nil {
		log.Printf("⚠️  CoreDNS ConfigMap not found: %v", err)
		return
	}

	log.Printf("Removing import statement from CoreDNS Corefile...")

	// Get current Corefile
	corefile, exists := configMap.Data["Corefile"]
	if !exists {
		log.Printf("⚠️  Corefile not found in ConfigMap")
		return
	}

	// Remove the import statement
	lines := strings.Split(corefile, "\n")
	var newLines []string
	for _, line := range lines {
		if !strings.Contains(line, importStatement) {
			newLines = append(newLines, line)
		}
	}
	newCorefile := strings.Join(newLines, "\n")

	// Update the ConfigMap
	configMap.Data["Corefile"] = newCorefile
	_, err = clientset.CoreV1().ConfigMaps(coreDNSNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Failed to update CoreDNS ConfigMap: %v", err)
		return
	}

	log.Printf("✅ Import statement removed from CoreDNS ConfigMap")
}

// cleanupCoreDNSDeployment removes volume mount and volume from CoreDNS deployment
func cleanupCoreDNSDeployment() {
	log.Printf("Checking for CoreDNS deployment...")

	ctx := context.Background()
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset: %v", err)
		return
	}

	deployment, err := clientset.AppsV1().Deployments(coreDNSNamespace).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		log.Printf("⚠️  CoreDNS deployment not found: %v", err)
		return
	}

	log.Printf("Removing volume mount and volume from CoreDNS deployment...")

	volumeName := "coredns-custom-volume"
	updated := false

	// Remove volume mounts from containers
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]
		var newVolumeMounts []corev1.VolumeMount
		for _, vm := range container.VolumeMounts {
			if vm.Name != volumeName {
				newVolumeMounts = append(newVolumeMounts, vm)
			} else {
				updated = true
			}
		}
		container.VolumeMounts = newVolumeMounts
	}

	// Remove volumes
	var newVolumes []corev1.Volume
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.Name != volumeName {
			newVolumes = append(newVolumes, vol)
		} else {
			updated = true
		}
	}
	deployment.Spec.Template.Spec.Volumes = newVolumes

	if updated {
		_, err = clientset.AppsV1().Deployments(coreDNSNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			log.Printf("Failed to update CoreDNS deployment: %v", err)
			return
		}
		log.Printf("✅ Volume and volume mount cleanup completed")
	} else {
		log.Printf("⚠️  No custom volumes found in CoreDNS deployment")
	}
}

// cleanupDynamicConfigMap removes the dynamic ConfigMap
func cleanupDynamicConfigMap() {
	log.Printf("Checking for dynamic ConfigMap...")

	ctx := context.Background()
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset: %v", err)
		return
	}

	err = clientset.CoreV1().ConfigMaps(coreDNSNamespace).Delete(ctx, dynamicConfigMapName, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("⚠️  Dynamic ConfigMap not found or failed to delete: %v", err)
		return
	}

	log.Printf("✅ Dynamic ConfigMap removed")
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		podName = "unknown-pod"
	}
	log.Printf("[%s] Reconciling changes for request: %s", podName, req.NamespacedName)

	// Parse watch namespaces for filtering
	var targetNamespaces []string
	if watchNamespaces != "" {
		targetNamespaces = strings.Split(strings.ReplaceAll(watchNamespaces, " ", ""), ",")
		// Filter out empty strings
		var validNamespaces []string
		for _, ns := range targetNamespaces {
			if ns != "" {
				validNamespaces = append(validNamespaces, ns)
			}
		}
		targetNamespaces = validNamespaces
	}

	// List ingresses with namespace filtering
	var ingressList networkingv1.IngressList
	if len(targetNamespaces) > 0 {
		// List ingresses from specific namespaces
		for _, ns := range targetNamespaces {
			var nsIngressList networkingv1.IngressList
			if err := r.List(ctx, &nsIngressList, client.InNamespace(ns)); err != nil {
				log.Printf("Failed to list ingresses in namespace %s: %v", ns, err)
				continue
			}
			ingressList.Items = append(ingressList.Items, nsIngressList.Items...)
		}
	} else {
		// List all ingresses
		if err := r.List(ctx, &ingressList); err != nil {
			log.Printf("Failed to list ingresses: %v", err)
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
	}

	// Extract hostnames from target ingresses
	hosts := extractHostnames(ingressList.Items)

	// Extract unique domains from hosts
	domains := extractDomains(hosts)

	// Update dynamic ConfigMap with discovered domains
	if err := r.updateDynamicConfigMap(ctx, domains, hosts); err != nil {
		log.Printf("Failed to update dynamic ConfigMap: %v", err)
		return reconcile.Result{RequeueAfter: time.Minute}, err
	}

	// Ensure CoreDNS ConfigMap has import statement and volume mount
	if err := r.ensureCoreDNSConfiguration(ctx); err != nil {
		log.Printf("Failed to ensure CoreDNS configuration: %v", err)
		return reconcile.Result{RequeueAfter: time.Minute}, err
	}

	log.Printf("[%s] Successfully updated dynamic ConfigMap with %d domains and %d hosts", podName, len(domains), len(hosts))
	log.Printf("[%s] CoreDNS configuration validated and updated if needed", podName)
	return reconcile.Result{}, nil
}

func isTargetIngress(obj client.Object) bool {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return false
	}
	return ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == ingressClass
}

func extractHostnames(ingresses []networkingv1.Ingress) []string {
	hostSet := make(map[string]bool)

	for _, ing := range ingresses {
		// Skip ingresses not matching our class
		if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != ingressClass {
			continue
		}

		// Extract hosts from rules
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				hostSet[rule.Host] = true
			}
		}
	}

	// Convert set to slice
	var hosts []string
	for host := range hostSet {
		hosts = append(hosts, host)
	}

	return hosts
}

func extractDomains(hosts []string) []string {
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

func (r *IngressReconciler) updateDynamicConfigMap(ctx context.Context, domains []string, hosts []string) error {
	configMapName := types.NamespacedName{
		Name:      dynamicConfigMapName,
		Namespace: coreDNSNamespace,
	}

	// Generate dynamic configuration
	dynamicConfig := generateDynamicConfig(domains, hosts)

	// Retry logic to handle concurrent updates
	for attempt := 0; attempt < 3; attempt++ {
		// Get or create the dynamic ConfigMap (fresh read each attempt)
		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, configMapName, configMap)

		if err != nil {
			// Create new ConfigMap if it doesn't exist
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dynamicConfigMapName,
					Namespace: coreDNSNamespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "coredns-ingress-sync",
					},
				},
				Data: make(map[string]string),
			}

			// Set the content and try to create
			configMap.Data[dynamicConfigKey] = dynamicConfig

			if err := r.Create(ctx, configMap); err != nil {
				if attempt == 2 {
					return fmt.Errorf("failed to create dynamic ConfigMap after retries: %w", err)
				}
				continue // Retry
			}
			log.Printf("Created dynamic ConfigMap %s with %d domains", dynamicConfigMapName, len(domains))
			return nil
		}

		// Check if content has actually changed to avoid unnecessary updates
		if existingConfig, exists := configMap.Data[dynamicConfigKey]; exists && existingConfig == dynamicConfig {
			log.Printf("Dynamic ConfigMap %s is already up to date", dynamicConfigMapName)
			return nil
		}

		// Update ConfigMap with fresh data
		configMap.Data[dynamicConfigKey] = dynamicConfig

		// Ensure labels are set for identification
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		configMap.Labels["app.kubernetes.io/managed-by"] = "coredns-ingress-sync"

		// Try to update
		if err := r.Update(ctx, configMap); err != nil {
			if attempt == 2 {
				return fmt.Errorf("failed to update dynamic ConfigMap after retries: %w", err)
			}
			// Brief delay before retry to reduce contention
			time.Sleep(time.Millisecond * 100)
			continue // Retry with fresh read
		}

		log.Printf("Updated dynamic ConfigMap %s with %d domains", dynamicConfigMapName, len(domains))
		return nil
	}

	return fmt.Errorf("exhausted retries updating dynamic ConfigMap")
}

func generateDynamicConfig(domains []string, hosts []string) string {
	var config strings.Builder

	// Header
	config.WriteString("# Auto-generated by coredns-ingress-sync controller\n")
	config.WriteString(fmt.Sprintf("# Last updated: %s\n", time.Now().Format(time.RFC3339)))
	config.WriteString("\n")

	// Generate individual rewrite rules for each discovered host
	for _, host := range hosts {
		config.WriteString(fmt.Sprintf("rewrite name exact %s %s\n", host, targetCNAME))
	}

	return config.String()
}

func (r *IngressReconciler) ensureCoreDNSConfiguration(ctx context.Context) error {
	// Check if we should manage CoreDNS configuration
	if os.Getenv("COREDNS_AUTO_CONFIGURE") == "false" {
		log.Printf("CoreDNS auto-configuration disabled")
		return nil
	}

	// First, ensure the import statement is in the CoreDNS Corefile
	if err := r.ensureCoreDNSImport(ctx); err != nil {
		// Log the error but don't fail the reconciliation if CoreDNS is not available
		log.Printf("Warning: Failed to ensure CoreDNS import statement: %v", err)
		return nil
	}

	// Then, ensure the CoreDNS deployment has the volume mount
	if err := r.ensureCoreDNSVolumeMount(ctx); err != nil {
		// Log the error but don't fail the reconciliation if CoreDNS is not available
		log.Printf("Warning: Failed to ensure CoreDNS volume mount: %v", err)
		return nil
	}

	return nil
}

func (r *IngressReconciler) ensureCoreDNSImport(ctx context.Context) error {
	// Get the CoreDNS ConfigMap
	coreDNSConfigMap := &corev1.ConfigMap{}
	coreDNSConfigMapName := types.NamespacedName{
		Name:      coreDNSConfigMapName,
		Namespace: coreDNSNamespace,
	}

	if err := r.Get(ctx, coreDNSConfigMapName, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to get CoreDNS ConfigMap: %w", err)
	}

	// Check if Corefile exists
	corefile, exists := coreDNSConfigMap.Data["Corefile"]
	if !exists {
		return fmt.Errorf("Corefile not found in CoreDNS ConfigMap")
	}

	// Check if import statement already exists
	if strings.Contains(corefile, importStatement) {
		log.Printf("Import statement already exists in CoreDNS Corefile")
		return nil
	}

	// Add import statement after the .:53 { line
	lines := strings.Split(corefile, "\n")
	var newLines []string
	importAdded := false

	for _, line := range lines {
		newLines = append(newLines, line)
		// Add import statement after the main server block starts
		if strings.TrimSpace(line) == ".:53 {" && !importAdded {
			newLines = append(newLines, "    "+importStatement)
			importAdded = true
		}
	}

	if !importAdded {
		log.Printf("Warning: Could not find '.:53 {' in Corefile, appending import statement")
		newLines = append(newLines, importStatement)
	}

	// Update the ConfigMap
	newCorefile := strings.Join(newLines, "\n")
	coreDNSConfigMap.Data["Corefile"] = newCorefile

	if err := r.Update(ctx, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to update CoreDNS ConfigMap: %w", err)
	}

	log.Printf("Added import statement to CoreDNS Corefile")
	return nil
}

func (r *IngressReconciler) ensureCoreDNSVolumeMount(ctx context.Context) error {
	// Try to create a direct Kubernetes client for deployment operations
	// If the client is a fake client (in tests), we'll use it directly
	if r.isFakeClient() {
		controllerClient := &ControllerRuntimeClient{client: r.Client}
		return r.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
	}

	config, err := ctrl.GetConfig()
	if err != nil {
		// In test environment, we'll simulate the deployment update using the controller-runtime client
		controllerClient := &ControllerRuntimeClient{client: r.Client}
		return r.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		// In test environment, we'll simulate the deployment update using the controller-runtime client
		controllerClient := &ControllerRuntimeClient{client: r.Client}
		return r.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
	}

	// Create a wrapper that implements the same interface as controller-runtime client
	directClient := &DirectKubernetesClient{clientset: clientset}
	return r.ensureCoreDNSVolumeMountWithClient(ctx, directClient)
}

// Interface for volume mount operations
type DeploymentClient interface {
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error
}

// Wrapper for direct Kubernetes client
type DirectKubernetesClient struct {
	clientset kubernetes.Interface
}

func (d *DirectKubernetesClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return d.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (d *DirectKubernetesClient) UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	_, err := d.clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// Wrapper for controller-runtime client
type ControllerRuntimeClient struct {
	client client.Client
}

func (c *ControllerRuntimeClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	var deployment appsv1.Deployment
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &deployment)
	return &deployment, err
}

func (c *ControllerRuntimeClient) UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	return c.client.Update(ctx, deployment)
}

func (r *IngressReconciler) ensureCoreDNSVolumeMountWithClient(ctx context.Context, deploymentClient DeploymentClient) error {
	// Get the CoreDNS deployment
	coreDNSDeployment, err := deploymentClient.GetDeployment(ctx, coreDNSNamespace, "coredns")
	if err != nil {
		return fmt.Errorf("failed to get CoreDNS deployment: %w", err)
	}

	// Check if volume and volume mount already exist
	hasVolume := false
	hasVolumeMount := false
	volumeName := "coredns-custom-volume"

	// Check for existing volume
	for _, volume := range coreDNSDeployment.Spec.Template.Spec.Volumes {
		if volume.Name == volumeName {
			hasVolume = true
			break
		}
	}

	// Check for existing volume mount
	if len(coreDNSDeployment.Spec.Template.Spec.Containers) > 0 {
		for _, mount := range coreDNSDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
			if mount.Name == volumeName {
				hasVolumeMount = true
				break
			}
		}
	}

	// If both exist, nothing to do
	if hasVolume && hasVolumeMount {
		return nil
	}

	// Add volume if missing
	if !hasVolume {
		newVolume := corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: dynamicConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  dynamicConfigKey,
							Path: "dynamic.server",
						},
					},
				},
			},
		}
		coreDNSDeployment.Spec.Template.Spec.Volumes = append(coreDNSDeployment.Spec.Template.Spec.Volumes, newVolume)
	}

	// Add volume mount if missing
	if !hasVolumeMount && len(coreDNSDeployment.Spec.Template.Spec.Containers) > 0 {
		newVolumeMount := corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/etc/coredns/custom",
			ReadOnly:  true,
		}
		coreDNSDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
			coreDNSDeployment.Spec.Template.Spec.Containers[0].VolumeMounts,
			newVolumeMount,
		)
	}

	// Update the deployment
	if err := deploymentClient.UpdateDeployment(ctx, coreDNSDeployment); err != nil {
		return fmt.Errorf("failed to update CoreDNS deployment: %w", err)
	}

	return nil
}

func isDevelopment() bool {
	// Check if we're running in development mode
	return os.Getenv("DEVELOPMENT_MODE") == "true"
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isFakeClient detects if we're using a fake client (in tests)
func (r *IngressReconciler) isFakeClient() bool {
	// Check if the client is a fake client by testing with a type assertion
	// This is a common pattern in controller-runtime tests
	clientTypeName := fmt.Sprintf("%T", r.Client)
	return strings.Contains(clientTypeName, "fake")
}

func addConfigMapWatch(cache cache.Cache, c controller.Controller, namespace, name, reconcileName string) error {
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
				return nil
			}),
			predicate.NewTypedPredicateFuncs(func(obj *corev1.ConfigMap) bool {
				return obj.GetNamespace() == namespace && obj.GetName() == name
			})),
	)
}

func addDynamicConfigMapWatch(cache cache.Cache, c controller.Controller, namespace, name, reconcileName string) error {
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
				return nil
			}),
			predicate.TypedFuncs[*corev1.ConfigMap]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.ConfigMap]) bool {
					return false // Don't trigger on create - we create it ourselves
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
					// Simple check: only trigger if the ConfigMap doesn't have our management label
					// This handles the common case where external tools overwrite our ConfigMap
					if e.ObjectNew.GetNamespace() == namespace && e.ObjectNew.GetName() == name {
						labels := e.ObjectNew.GetLabels()
						if labels == nil || labels["app.kubernetes.io/managed-by"] != "coredns-ingress-sync" {
							// External update, trigger reconciliation
							return true
						}
					}
					return false
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.ConfigMap]) bool {
					// Always trigger on delete for disaster recovery
					return e.Object.GetNamespace() == namespace && e.Object.GetName() == name
				},
			}),
	)
}
