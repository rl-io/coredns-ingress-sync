package cleanup

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
)

func TestNewManager(t *testing.T) {
	t.Log("DEBUG: Starting TestNewManager")
	
	// Create a test logger with debug level
	t.Log("DEBUG: Creating test logger")
	logger := ctrl.Log.WithName("test")
	
	t.Log("DEBUG: Calling NewManager")
	manager, err := NewManager(logger)
	
	t.Logf("DEBUG: NewManager returned - err: %v, manager: %v", err, manager != nil)
	
	// In CI environments without a cluster, we expect this to fail with a specific error
	if err != nil {
		t.Logf("INFO: NewManager failed as expected in test environment: %v", err)
		
		// Verify it's the expected kubeconfig error
		if !strings.Contains(err.Error(), "failed to create Kubernetes client") && 
		   !strings.Contains(err.Error(), "unable to load in-cluster configuration") &&
		   !strings.Contains(err.Error(), "no configuration has been provided") {
			t.Fatalf("Unexpected error creating manager: %v", err)
		}
		
		t.Log("DEBUG: Test passed - expected kubeconfig error in CI environment")
		return
	}
	
	// If we get here, we're in an environment with valid kubeconfig
	if manager == nil {
		t.Fatal("Expected non-nil manager when no error occurred")
	}
	
	t.Log("DEBUG: Checking manager.client")
	if manager.client == nil {
		t.Error("Expected non-nil client in manager")
	}
	
	t.Log("DEBUG: Checking manager.logger")
	if manager.logger.GetSink() == nil {
		t.Error("Expected non-nil logger in manager")
	}
	
	t.Log("DEBUG: TestNewManager completed successfully")
}

func TestRun(t *testing.T) {
	// Create a test logger
	logger := ctrl.Log.WithName("test")
	
	// Create test configuration
	cfg := &config.Config{
		CoreDNSNamespace:     "kube-system",
		CoreDNSConfigMapName: "coredns",
		DynamicConfigMapName: "coredns-custom",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	}
	
	t.Run("cleanup_with_no_existing_resources", func(t *testing.T) {
		// Create manager with fake client that has no resources
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Run cleanup - should not error even if resources don't exist
		err := manager.Run(cfg)
		if err != nil {
			t.Errorf("Expected no error during cleanup, got: %v", err)
		}
	})
	
	t.Run("cleanup_with_existing_dynamic_configmap", func(t *testing.T) {
		// Create manager with fake client that has a dynamic ConfigMap
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		dynamicConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.DynamicConfigMapName,
				Namespace: cfg.CoreDNSNamespace,
			},
			Data: map[string]string{
				"dynamic.server": "rewrite name exact api.example.com ingress-nginx.svc.cluster.local.",
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(dynamicConfigMap).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Run cleanup - should delete the dynamic ConfigMap
		err := manager.Run(cfg)
		if err != nil {
			t.Errorf("Expected no error during cleanup, got: %v", err)
		}
		
		// Verify ConfigMap was deleted
		var deletedConfigMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(), 
			client.ObjectKey{Name: cfg.DynamicConfigMapName, Namespace: cfg.CoreDNSNamespace}, 
			&deletedConfigMap)
		if err == nil {
			t.Error("Expected ConfigMap to be deleted, but it still exists")
		}
	})
}

func TestDeleteDynamicConfigMap(t *testing.T) {
	logger := ctrl.Log.WithName("test")
	
	cfg := &config.Config{
		CoreDNSNamespace:     "kube-system", 
		DynamicConfigMapName: "coredns-custom",
	}
	
	t.Run("delete_existing_configmap", func(t *testing.T) {
		// Create manager with fake client that has a dynamic ConfigMap
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		dynamicConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.DynamicConfigMapName,
				Namespace: cfg.CoreDNSNamespace,
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(dynamicConfigMap).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Delete the ConfigMap
		err := manager.deleteDynamicConfigMap(context.Background(), cfg)
		if err != nil {
			t.Errorf("Expected no error deleting ConfigMap, got: %v", err)
		}
	})
	
	t.Run("delete_nonexistent_configmap", func(t *testing.T) {
		// Create manager with fake client that has no ConfigMap
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Delete nonexistent ConfigMap - should not error
		err := manager.deleteDynamicConfigMap(context.Background(), cfg)
		if err != nil {
			t.Errorf("Expected no error deleting nonexistent ConfigMap, got: %v", err)
		}
	})
}

func TestRemoveCoreDNSImport(t *testing.T) {
	logger := ctrl.Log.WithName("test")
	
	cfg := &config.Config{
		CoreDNSNamespace:     "kube-system",
		CoreDNSConfigMapName: "coredns",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	}
	
	t.Run("remove_existing_import_statement", func(t *testing.T) {
		// Create manager with fake client that has CoreDNS ConfigMap with import
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		corefile := `.:53 {
    import /etc/coredns/custom/*.server
    errors
    health {
        lameduck 5s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
        ttl 30
    }
    prometheus :9153
    forward . /etc/resolv.conf {
        max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}`
		
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.CoreDNSConfigMapName,
				Namespace: cfg.CoreDNSNamespace,
			},
			Data: map[string]string{
				"Corefile": corefile,
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(coreDNSConfigMap).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Mock CoreDNS manager
		coreDNSManager := &coredns.Manager{}
		
		// Remove import statement
		err := manager.removeCoreDNSImport(context.Background(), coreDNSManager, cfg)
		if err != nil {
			t.Errorf("Expected no error removing import statement, got: %v", err)
		}
		
		// Verify import statement was removed
		var updatedConfigMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(), 
			client.ObjectKey{Name: cfg.CoreDNSConfigMapName, Namespace: cfg.CoreDNSNamespace}, 
			&updatedConfigMap)
		if err != nil {
			t.Fatalf("Failed to get updated ConfigMap: %v", err)
		}
		
		updatedCorefile := updatedConfigMap.Data["Corefile"]
		if strings.Contains(updatedCorefile, cfg.ImportStatement) {
			t.Error("Expected import statement to be removed from Corefile")
		}
	})
	
	t.Run("remove_import_when_not_present", func(t *testing.T) {
		// Create manager with fake client that has CoreDNS ConfigMap without import
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		corefile := `.:53 {
    errors
    health {
        lameduck 5s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
        ttl 30
    }
    prometheus :9153
    forward . /etc/resolv.conf {
        max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}`
		
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.CoreDNSConfigMapName,
				Namespace: cfg.CoreDNSNamespace,
			},
			Data: map[string]string{
				"Corefile": corefile,
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(coreDNSConfigMap).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Mock CoreDNS manager
		coreDNSManager := &coredns.Manager{}
		
		// Remove import statement - should not error
		err := manager.removeCoreDNSImport(context.Background(), coreDNSManager, cfg)
		if err != nil {
			t.Errorf("Expected no error when import statement not present, got: %v", err)
		}
	})
	
	t.Run("missing_corefile", func(t *testing.T) {
		// Create manager with fake client that has CoreDNS ConfigMap without Corefile
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.CoreDNSConfigMapName,
				Namespace: cfg.CoreDNSNamespace,
			},
			Data: map[string]string{
				// No Corefile key
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(coreDNSConfigMap).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Mock CoreDNS manager
		coreDNSManager := &coredns.Manager{}
		
		// Remove import statement - should error due to missing Corefile
		err := manager.removeCoreDNSImport(context.Background(), coreDNSManager, cfg)
		if err == nil {
			t.Error("Expected error when Corefile is missing")
		}
		if !strings.Contains(err.Error(), "corefile not found") {
			t.Errorf("Expected error about missing Corefile, got: %v", err)
		}
	})
}

func TestRemoveCoreDNSVolumeMount(t *testing.T) {
	logger := ctrl.Log.WithName("test")
	
	cfg := &config.Config{
		CoreDNSNamespace:  "kube-system",
		CoreDNSVolumeName: "coredns-ingress-sync-volume",
	}
	
	t.Run("remove_existing_volume_mount", func(t *testing.T) {
		// Create manager with fake client that has CoreDNS deployment with custom volume
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = appsv1.AddToScheme(scheme)
		
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: cfg.CoreDNSNamespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "coredns",
										},
									},
								},
							},
							{
								Name: cfg.CoreDNSVolumeName,
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "coredns-custom",
										},
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name: "coredns",
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "config-volume",
										MountPath: "/etc/coredns",
										ReadOnly:  true,
									},
									{
										Name:      cfg.CoreDNSVolumeName,
										MountPath: "/etc/coredns/custom",
										ReadOnly:  true,
									},
								},
							},
						},
					},
				},
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(deployment).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Mock CoreDNS manager
		coreDNSManager := &coredns.Manager{}
		
		// Remove volume mount
		err := manager.removeCoreDNSVolumeMount(context.Background(), coreDNSManager, cfg)
		if err != nil {
			t.Errorf("Expected no error removing volume mount, got: %v", err)
		}
		
		// Verify volume mount was removed
		var updatedDeployment appsv1.Deployment
		err = fakeClient.Get(context.Background(), 
			client.ObjectKey{Name: "coredns", Namespace: cfg.CoreDNSNamespace}, 
			&updatedDeployment)
		if err != nil {
			t.Fatalf("Failed to get updated deployment: %v", err)
		}
		
		// Check that the configurable volume was removed
		for _, volume := range updatedDeployment.Spec.Template.Spec.Volumes {
			if volume.Name == cfg.CoreDNSVolumeName {
				t.Errorf("Expected %s to be removed from deployment", cfg.CoreDNSVolumeName)
			}
		}
		
		// Check that the configurable volume mount was removed from container
		for _, container := range updatedDeployment.Spec.Template.Spec.Containers {
			if container.Name == "coredns" {
				for _, volumeMount := range container.VolumeMounts {
					if volumeMount.Name == cfg.CoreDNSVolumeName {
						t.Errorf("Expected %s mount to be removed from coredns container", cfg.CoreDNSVolumeName)
					}
				}
			}
		}
	})
	
	t.Run("remove_volume_when_not_present", func(t *testing.T) {
		// Create manager with fake client that has CoreDNS deployment without custom volume
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = appsv1.AddToScheme(scheme)
		
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: cfg.CoreDNSNamespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "coredns",
										},
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name: "coredns",
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "config-volume",
										MountPath: "/etc/coredns",
										ReadOnly:  true,
									},
								},
							},
						},
					},
				},
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(deployment).
			Build()
		
		manager := &Manager{
			client: fakeClient,
			logger: logger,
		}
		
		// Mock CoreDNS manager
		coreDNSManager := &coredns.Manager{}
		
		// Remove volume mount - should not error when not present
		err := manager.removeCoreDNSVolumeMount(context.Background(), coreDNSManager, cfg)
		if err != nil {
			t.Errorf("Expected no error when volume mount not present, got: %v", err)
		}
	})
}
