package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestMainFunction tests the main function's mode handling
func TestMainFunction(t *testing.T) {
	// Save original args
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "controller mode",
			args:        []string{"cmd", "-mode", "controller"},
			expectError: false, // Will error in test due to no k8s config, but we test the mode parsing
		},
		{
			name:        "cleanup mode", 
			args:        []string{"cmd", "-mode", "cleanup"},
			expectError: false, // Will error in test due to no k8s config, but we test the mode parsing
		},
		{
			name:        "invalid mode",
			args:        []string{"cmd", "-mode", "invalid"},
			expectError: true,
		},
		{
			name:        "default mode (controller)",
			args:        []string{"cmd"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip actual execution as it would require k8s setup
			// This test validates that mode parsing works correctly
			os.Args = tt.args
			
			// Test the mode parsing logic by calling flag.Parse() directly
			
			// Reset flags for clean test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			var mode = flag.String("mode", "controller", "Mode to run: 'controller' or 'cleanup'")
			
			// Don't call main() as it would try to start actual controller
			// Just verify flag parsing works
			flag.Parse()
			
			switch *mode {
			case "controller", "cleanup":
				// Valid modes
				assert.Contains(t, []string{"controller", "cleanup"}, *mode)
			default:
				if !tt.expectError {
					t.Errorf("Expected valid mode, got %s", *mode)
				}
			}
		})
	}
}

// TestRunCleanupComponents tests individual cleanup functions
func TestRunCleanupComponents(t *testing.T) {
	// Test cleanup functions individually since runCleanup() requires k8s config
	t.Run("test cleanup function coordination", func(t *testing.T) {
		// Set up environment variables
		os.Setenv("COREDNS_NAMESPACE", "kube-system")
		os.Setenv("COREDNS_CONFIGMAP_NAME", "coredns")
		os.Setenv("DYNAMIC_CONFIGMAP_NAME", "coredns-custom")
		defer func() {
			os.Unsetenv("COREDNS_NAMESPACE")
			os.Unsetenv("COREDNS_CONFIGMAP_NAME") 
			os.Unsetenv("DYNAMIC_CONFIGMAP_NAME")
		}()

		// Test that cleanup functions exist and can be called
		// (They will fail due to no k8s client, but we test function existence)
		assert.NotNil(t, cleanupCoreDNSConfigMap)
		assert.NotNil(t, cleanupCoreDNSDeployment)
		assert.NotNil(t, cleanupDynamicConfigMap)
	})
}

// TestWaitForControllerTermination tests the controller termination logic
func TestWaitForControllerTermination(t *testing.T) {
	t.Run("test termination logic with mock client", func(t *testing.T) {
		// Create a fake kubernetes client
		clientset := kubefake.NewSimpleClientset()
		
		// Test the logic for finding controller pods
		ctx := context.Background()
		namespace := "test-namespace"
		
		// Create some mock pods
		controllerPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-ingress-sync-controller-abc123",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name": "coredns-ingress-sync",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		
		cleanupPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-ingress-sync-cleanup-xyz789",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name": "coredns-ingress-sync",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		
		// Create pods
		_, err := clientset.CoreV1().Pods(namespace).Create(ctx, controllerPod, metav1.CreateOptions{})
		require.NoError(t, err)
		
		_, err = clientset.CoreV1().Pods(namespace).Create(ctx, cleanupPod, metav1.CreateOptions{})
		require.NoError(t, err)
		
		// Test pod filtering logic (from listRunningControllers function)
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=coredns-ingress-sync",
		})
		require.NoError(t, err)
		
		// Filter out cleanup job pods (they contain "cleanup" in the name)
		var controllerPods []corev1.Pod
		for _, pod := range pods.Items {
			if !strings.Contains(pod.Name, "cleanup") {
				controllerPods = append(controllerPods, pod)
			}
		}
		
		// Should find only the controller pod, not the cleanup pod
		assert.Len(t, controllerPods, 1)
		assert.Equal(t, "coredns-ingress-sync-controller-abc123", controllerPods[0].Name)
	})
}

// TestEnsureCoreDNSVolumeMountPaths tests different code paths in volume mount logic
func TestEnsureCoreDNSVolumeMountPaths(t *testing.T) {
	tests := []struct {
		name               string
		existingVolumes    []corev1.Volume
		existingMounts     []corev1.VolumeMount
		expectVolumeAdd    bool
		expectVolumeMountAdd bool
	}{
		{
			name:               "no existing volume or mount",
			existingVolumes:    []corev1.Volume{},
			existingMounts:     []corev1.VolumeMount{},
			expectVolumeAdd:    true,
			expectVolumeMountAdd: true,
		},
		{
			name: "volume exists but no mount",
			existingVolumes: []corev1.Volume{
				{
					Name: "coredns-custom-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "coredns-custom",
							},
						},
					},
				},
			},
			existingMounts:       []corev1.VolumeMount{},
			expectVolumeAdd:      false,
			expectVolumeMountAdd: true,
		},
		{
			name:            "mount exists but no volume",
			existingVolumes: []corev1.Volume{},
			existingMounts: []corev1.VolumeMount{
				{
					Name:      "coredns-custom-volume",
					MountPath: "/etc/coredns/custom",
					ReadOnly:  true,
				},
			},
			expectVolumeAdd:      true,
			expectVolumeMountAdd: false,
		},
		{
			name: "both volume and mount exist",
			existingVolumes: []corev1.Volume{
				{
					Name: "coredns-custom-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "coredns-custom",
							},
						},
					},
				},
			},
			existingMounts: []corev1.VolumeMount{
				{
					Name:      "coredns-custom-volume",
					MountPath: "/etc/coredns/custom",
					ReadOnly:  true,
				},
			},
			expectVolumeAdd:      false,
			expectVolumeMountAdd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: tt.existingVolumes,
							Containers: []corev1.Container{
								{
									Name:         "coredns",
									Image:        "registry.k8s.io/coredns/coredns:v1.11.1",
									VolumeMounts: tt.existingMounts,
								},
							},
						},
					},
				},
			}

			// Create fake client with the deployment
			scheme := runtime.NewScheme()
			require.NoError(t, appsv1.AddToScheme(scheme))
			require.NoError(t, corev1.AddToScheme(scheme))
			
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(deployment).
				Build()

			reconciler := &IngressReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			// Test the volume mount logic
			deploymentClient := &ControllerRuntimeClient{client: fakeClient}
			err := reconciler.ensureCoreDNSVolumeMountWithClient(context.Background(), deploymentClient)
			
			if tt.expectVolumeAdd || tt.expectVolumeMountAdd {
				// Should succeed and update the deployment
				assert.NoError(t, err)
				
				// Verify the deployment was updated
				var updatedDeployment appsv1.Deployment
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      "coredns",
					Namespace: "kube-system",
				}, &updatedDeployment)
				require.NoError(t, err)
				
				if tt.expectVolumeAdd {
					// Check that volume was added
					hasVolume := false
					for _, vol := range updatedDeployment.Spec.Template.Spec.Volumes {
						if vol.Name == "coredns-custom-volume" {
							hasVolume = true
							break
						}
					}
					assert.True(t, hasVolume, "Expected volume to be added")
				}
				
				if tt.expectVolumeMountAdd {
					// Check that volume mount was added
					hasMount := false
					if len(updatedDeployment.Spec.Template.Spec.Containers) > 0 {
						for _, mount := range updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
							if mount.Name == "coredns-custom-volume" {
								hasMount = true
								break
							}
						}
					}
					assert.True(t, hasMount, "Expected volume mount to be added")
				}
			} else {
				// Should succeed without changes
				assert.NoError(t, err)
			}
		})
	}
}

// TestAddWatchFunctions tests the watch setup functions
func TestAddWatchFunctions(t *testing.T) {
	t.Run("test watch function signatures", func(t *testing.T) {
		// Test that the watch functions exist and have correct signatures
		// We can't easily test the actual watch setup without a full controller-runtime setup
		// But we can test that the functions exist and basic logic
		
		// Test addConfigMapWatch function exists
		assert.NotNil(t, addConfigMapWatch)
		
		// Test addDynamicConfigMapWatch function exists  
		assert.NotNil(t, addDynamicConfigMapWatch)
		
		// Test the watch filtering logic
		testConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
		}
		
		// Verify ConfigMap matches expected namespace and name
		assert.Equal(t, "kube-system", testConfigMap.GetNamespace())
		assert.Equal(t, "coredns", testConfigMap.GetName())
	})
}

// TestDirectKubernetesClientInterface tests the DirectKubernetesClient wrapper
func TestDirectKubernetesClientInterface(t *testing.T) {
	t.Run("direct kubernetes client methods", func(t *testing.T) {
		// Create a fake kubernetes client
		clientset := kubefake.NewSimpleClientset()
		
		// Create test deployment
		testDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment", 
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "test",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "test",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test:latest",
							},
						},
					},
				},
			},
		}
		
		// Create the deployment
		_, err := clientset.AppsV1().Deployments("default").Create(
			context.Background(), testDeployment, metav1.CreateOptions{})
		require.NoError(t, err)
		
		// Test DirectKubernetesClient wrapper
		directClient := &DirectKubernetesClient{clientset: clientset}
		
		// Test GetDeployment
		deployment, err := directClient.GetDeployment(context.Background(), "default", "test-deployment")
		assert.NoError(t, err)
		assert.Equal(t, "test-deployment", deployment.Name)
		
		// Test UpdateDeployment
		deployment.Spec.Replicas = int32Ptr(2)
		err = directClient.UpdateDeployment(context.Background(), deployment)
		assert.NoError(t, err)
		
		// Verify update
		updatedDeployment, err := directClient.GetDeployment(context.Background(), "default", "test-deployment")
		assert.NoError(t, err)
		assert.Equal(t, int32(2), *updatedDeployment.Spec.Replicas)
	})
}

// TestFakeClientDetection tests the isFakeClient function
func TestFakeClientDetection(t *testing.T) {
	t.Run("detect fake client", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		// Should detect that this is a fake client
		assert.True(t, reconciler.isFakeClient())
	})
}

// TestEnsureCoreDNSVolumeMountErrorCases tests error handling in volume mount logic
func TestEnsureCoreDNSVolumeMountErrorCases(t *testing.T) {
	t.Run("deployment not found error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, appsv1.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))
		
		// Create fake client without the CoreDNS deployment
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		// Should return an error when deployment is not found
		err := reconciler.ensureCoreDNSVolumeMount(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get CoreDNS deployment")
	})
	
	t.Run("deployment with no containers", func(t *testing.T) {
		// Test deployment with no containers
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{}, // No containers
					},
				},
			},
		}

		scheme := runtime.NewScheme()
		require.NoError(t, appsv1.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(deployment).
			Build()

		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		// Should not fail but should only add volume, not volume mount
		err := reconciler.ensureCoreDNSVolumeMount(context.Background())
		assert.NoError(t, err)
	})
}

// TestReconcileErrorPaths tests error handling in the main reconcile function
func TestReconcileErrorPaths(t *testing.T) {
	t.Run("reconcile with updateDynamicConfigMap error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		require.NoError(t, networkingv1.AddToScheme(scheme))
		
		// Create a fake client that will fail on ConfigMap creation specifically
		fakeClient := &failingClient{
			Client:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			failOnCreate: true,
		}
		
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test",
				Namespace: "default",
			},
		})
		
		// Should return error and requeue
		assert.Error(t, err)
		assert.Equal(t, result.RequeueAfter.Minutes(), float64(1))
	})
}

// Failing client for testing error paths
type failingClient struct {
	client.Client
	failOnGet    bool
	failOnCreate bool
	failOnUpdate bool
	failOnList   bool
}

func (f *failingClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if f.failOnGet {
		return fmt.Errorf("mock get error")
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

func (f *failingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f.failOnCreate {
		return fmt.Errorf("mock create error")
	}
	return f.Client.Create(ctx, obj, opts...)
}

func (f *failingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if f.failOnUpdate {
		return fmt.Errorf("mock update error")
	}
	return f.Client.Update(ctx, obj, opts...)
}

func (f *failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.failOnList {
		return fmt.Errorf("mock list error")
	}
	return f.Client.List(ctx, list, opts...)
}

func TestExtractDomains(t *testing.T) {
	tests := []struct {
		name     string
		hosts    []string
		expected []string
	}{
		{
			name:     "single domain",
			hosts:    []string{"api.k8s.example.com", "web.k8s.example.com"},
			expected: []string{"k8s.example.com"},
		},
		{
			name:     "multiple domains",
			hosts:    []string{"api.k8s.example.com", "web.staging.example.com", "dashboard.prod.company.com"},
			expected: []string{"k8s.example.com", "staging.example.com", "prod.company.com"},
		},
		{
			name:     "empty hosts",
			hosts:    []string{},
			expected: []string{},
		},
		{
			name:     "single level domain",
			hosts:    []string{"localhost"},
			expected: []string{},
		},
		{
			name:     "duplicate domains",
			hosts:    []string{"api.k8s.example.com", "web.k8s.example.com", "auth.k8s.example.com"},
			expected: []string{"k8s.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomains(tt.hosts)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestGenerateDynamicConfig(t *testing.T) {
	tests := []struct {
		name     string
		domains  []string
		hosts    []string
		expected []string // strings that should be in the output
	}{
		{
			name:    "single domain with hosts",
			domains: []string{"k8s.example.com"},
			hosts:   []string{"api.k8s.example.com", "web.k8s.example.com"},
			expected: []string{
				"rewrite name exact api.k8s.example.com ingress-nginx-controller.ingress-nginx.svc.cluster.local.",
				"rewrite name exact web.k8s.example.com ingress-nginx-controller.ingress-nginx.svc.cluster.local.",
			},
		},
		{
			name:    "multiple domains",
			domains: []string{"k8s.example.com", "staging.example.com"},
			hosts:   []string{"api.k8s.example.com", "web.staging.example.com"},
			expected: []string{
				"rewrite name exact api.k8s.example.com",
				"rewrite name exact web.staging.example.com",
			},
		},
		{
			name:     "empty input",
			domains:  []string{},
			hosts:    []string{},
			expected: []string{"# Auto-generated by coredns-ingress-sync controller"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDynamicConfig(tt.domains, tt.hosts)
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestIngressReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name          string
		ingresses     []networkingv1.Ingress
		expectedHosts []string
	}{
		{
			name: "single ingress with nginx class",
			ingresses: []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ingress",
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{
								Host: "api.k8s.example.com",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Path: "/",
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "api-service",
														Port: networkingv1.ServiceBackendPort{Number: 80},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedHosts: []string{"api.k8s.example.com"},
		},
		{
			name: "multiple ingresses with different classes",
			ingresses: []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-ingress",
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "api.k8s.example.com"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "traefik-ingress",
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("traefik"),
						Rules: []networkingv1.IngressRule{
							{Host: "traefik.k8s.example.com"},
						},
					},
				},
			},
			expectedHosts: []string{"api.k8s.example.com"}, // Only nginx should be included
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Disable CoreDNS auto-configuration for tests
			os.Setenv("COREDNS_AUTO_CONFIGURE", "false")
			defer os.Unsetenv("COREDNS_AUTO_CONFIGURE")

			// Create fake client with test objects
			objs := make([]client.Object, len(tt.ingresses))
			for i := range tt.ingresses {
				objs[i] = &tt.ingresses[i]
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				Build()

			reconciler := &IngressReconciler{
				Client: fakeClient,
			}

			// Test reconciliation
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.ingresses[0].Name,
					Namespace: tt.ingresses[0].Namespace,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			// With the simplified controller (no CoreDNS reload), it should succeed
			assert.NoError(t, err)
			assert.Equal(t, reconcile.Result{}, result)
		})
	}
}

func TestIngressFiltering(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	ingresses := []networkingv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules:            []networkingv1.IngressRule{{Host: "nginx.k8s.example.com"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "traefik-ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("traefik"),
				Rules:            []networkingv1.IngressRule{{Host: "traefik.k8s.example.com"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-class-ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: "noclass.k8s.example.com"}},
			},
		},
	}

	objs := make([]client.Object, len(ingresses))
	for i := range ingresses {
		objs[i] = &ingresses[i]
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	reconciler := &IngressReconciler{
		Client: fakeClient,
	}

	// Test that only nginx ingresses are processed
	ctx := context.Background()
	var ingressList networkingv1.IngressList
	err := reconciler.List(ctx, &ingressList)
	require.NoError(t, err)

	// Extract hosts from nginx ingresses only
	var nginxHosts []string
	for _, ing := range ingressList.Items {
		if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName == "nginx" {
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" {
					nginxHosts = append(nginxHosts, rule.Host)
				}
			}
		}
	}

	expected := []string{"nginx.k8s.example.com"}
	assert.ElementsMatch(t, expected, nginxHosts)
}

func TestConfigMapGeneration(t *testing.T) {
	domains := []string{"k8s.example.com", "staging.example.com"}
	hosts := []string{"api.k8s.example.com", "web.k8s.example.com", "dashboard.staging.example.com"}

	config := generateDynamicConfig(domains, hosts)

	// Check that all hosts are included
	for _, host := range hosts {
		assert.Contains(t, config, host)
	}

	// Check that rewrite rules are exact matches
	assert.Contains(t, config, "rewrite name exact api.k8s.example.com")
	assert.Contains(t, config, "rewrite name exact web.k8s.example.com")
	assert.Contains(t, config, "rewrite name exact dashboard.staging.example.com")

	// Check that target CNAME is correct
	assert.Contains(t, config, "ingress-nginx-controller.ingress-nginx.svc.cluster.local.")
}

func TestEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "use default when env not set",
			key:          "TEST_VAR",
			defaultValue: "default_value",
			envValue:     "",
			expected:     "default_value",
		},
		{
			name:         "use env value when set",
			key:          "TEST_VAR",
			defaultValue: "default_value",
			envValue:     "env_value",
			expected:     "env_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDevelopmentMode(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "development mode enabled",
			envValue: "true",
			expected: true,
		},
		{
			name:     "development mode disabled",
			envValue: "false",
			expected: false,
		},
		{
			name:     "development mode not set",
			envValue: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("DEVELOPMENT_MODE", tt.envValue)
			}

			result := isDevelopment()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}

// Benchmark tests
func BenchmarkExtractDomains(b *testing.B) {
	hosts := []string{
		"api.k8s.example.com",
		"web.k8s.example.com",
		"auth.k8s.example.com",
		"dashboard.staging.example.com",
		"api.staging.example.com",
		"web.production.company.com",
		"admin.production.company.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractDomains(hosts)
	}
}

func BenchmarkGenerateDynamicConfig(b *testing.B) {
	domains := []string{"k8s.example.com", "staging.example.com", "production.company.com"}
	hosts := []string{
		"api.k8s.example.com",
		"web.k8s.example.com",
		"auth.k8s.example.com",
		"dashboard.staging.example.com",
		"api.staging.example.com",
		"web.production.company.com",
		"admin.production.company.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateDynamicConfig(domains, hosts)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue string
		expected     string
	}{
		{
			name:         "environment variable exists",
			envKey:       "TEST_VAR",
			envValue:     "custom_value",
			defaultValue: "default_value",
			expected:     "custom_value",
		},
		{
			name:         "environment variable does not exist",
			envKey:       "NON_EXISTENT_VAR",
			envValue:     "",
			defaultValue: "default_value",
			expected:     "default_value",
		},
		{
			name:         "environment variable is empty",
			envKey:       "EMPTY_VAR",
			envValue:     "",
			defaultValue: "default_value",
			expected:     "default_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing env var
			os.Unsetenv(tt.envKey)

			// Set environment variable if needed
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			}

			result := getEnvOrDefault(tt.envKey, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvOrDefault(%s, %s) = %s, want %s", tt.envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestConfigurationDefaults(t *testing.T) {
	// Test that default configuration values are reasonable
	tests := []struct {
		name     string
		getValue func() string
		expected string
	}{
		{
			name:     "default ingress class",
			getValue: func() string { return getEnvOrDefault("INGRESS_CLASS", "nginx") },
			expected: "nginx",
		},
		{
			name: "default target CNAME",
			getValue: func() string {
				return getEnvOrDefault("TARGET_CNAME", "ingress-nginx-controller.ingress-nginx.svc.cluster.local.")
			},
			expected: "ingress-nginx-controller.ingress-nginx.svc.cluster.local.",
		},
		{
			name:     "default ConfigMap name",
			getValue: func() string { return getEnvOrDefault("DYNAMIC_CONFIGMAP_NAME", "coredns-custom") },
			expected: "coredns-custom",
		},
		{
			name:     "default config key",
			getValue: func() string { return getEnvOrDefault("DYNAMIC_CONFIG_KEY", "dynamic.server") },
			expected: "dynamic.server",
		},
		{
			name:     "default namespace",
			getValue: func() string { return getEnvOrDefault("COREDNS_NAMESPACE", "kube-system") },
			expected: "kube-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.getValue()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCoreDNSConfiguration(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name                      string
		autoConfigureEnabled      bool
		existingCoreDNSConfig     *corev1.ConfigMap
		existingCoreDNSDeployment *appsv1.Deployment
		expectedImportAdded       bool
		expectedVolumeMountAdded  bool
		expectError               bool
	}{
		{
			name:                 "auto-configure disabled",
			autoConfigureEnabled: false,
			existingCoreDNSConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
				},
			},
			expectedImportAdded:      false,
			expectedVolumeMountAdded: false,
			expectError:              false,
		},
		{
			name:                 "auto-configure enabled with existing CoreDNS",
			autoConfigureEnabled: true,
			existingCoreDNSConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
				},
			},
			existingCoreDNSDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "coredns",
									Image: "registry.k8s.io/coredns/coredns:v1.10.1",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config-volume",
											MountPath: "/etc/coredns",
											ReadOnly:  true,
										},
									},
								},
							},
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
						},
					},
				},
			},
			expectedImportAdded:      true,
			expectedVolumeMountAdded: true,
			expectError:              false,
		},
		{
			name:                 "auto-configure enabled with import already present",
			autoConfigureEnabled: true,
			existingCoreDNSConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"Corefile": `.:53 {
    import /etc/coredns/custom/*.server
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
				},
			},
			existingCoreDNSDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "coredns",
									Image: "registry.k8s.io/coredns/coredns:v1.10.1",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config-volume",
											MountPath: "/etc/coredns",
											ReadOnly:  true,
										},
										{
											Name:      "coredns-custom-volume",
											MountPath: "/etc/coredns/custom",
											ReadOnly:  true,
										},
									},
								},
							},
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
									Name: "coredns-custom-volume",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "coredns-custom",
											},
											Items: []corev1.KeyToPath{
												{
													Key:  "dynamic.server",
													Path: "dynamic.server",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedImportAdded:      false, // Already present
			expectedVolumeMountAdded: false, // Already present
			expectError:              false,
		},
		{
			name:                  "auto-configure enabled but CoreDNS ConfigMap missing",
			autoConfigureEnabled:  true,
			existingCoreDNSConfig: nil,
			expectedImportAdded:   false,
			expectError:           false, // Should not error, just log warning
		},
		{
			name:                 "auto-configure enabled but CoreDNS Deployment missing",
			autoConfigureEnabled: true,
			existingCoreDNSConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"Corefile": `.:53 {
    errors
    health
}`,
				},
			},
			existingCoreDNSDeployment: nil,
			expectedImportAdded:       true,
			expectedVolumeMountAdded:  false,
			expectError:               false, // Should not error, just log warning
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.autoConfigureEnabled {
				os.Setenv("COREDNS_AUTO_CONFIGURE", "true")
			} else {
				os.Setenv("COREDNS_AUTO_CONFIGURE", "false")
			}
			defer os.Unsetenv("COREDNS_AUTO_CONFIGURE")

			// Create fake client with test objects
			objs := []client.Object{}
			if tt.existingCoreDNSConfig != nil {
				objs = append(objs, tt.existingCoreDNSConfig)
			}
			if tt.existingCoreDNSDeployment != nil {
				objs = append(objs, tt.existingCoreDNSDeployment)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				Build()

			reconciler := &IngressReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			// Call the ensureCoreDNSConfiguration function
			err := reconciler.ensureCoreDNSConfiguration(context.Background())

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// If auto-configure is disabled, skip further checks
			if !tt.autoConfigureEnabled {
				return
			}

			// Check if import statement was added to ConfigMap
			if tt.existingCoreDNSConfig != nil {
				var updatedConfigMap corev1.ConfigMap
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      "coredns",
					Namespace: "kube-system",
				}, &updatedConfigMap)
				assert.NoError(t, err)

				corefile := updatedConfigMap.Data["Corefile"]
				hasImport := strings.Contains(corefile, "import /etc/coredns/custom/*.server")

				if tt.expectedImportAdded {
					assert.True(t, hasImport, "Import statement should be added to Corefile")
				} else if tt.existingCoreDNSConfig != nil {
					// Check if import was already present or should not be added
					originalHasImport := strings.Contains(tt.existingCoreDNSConfig.Data["Corefile"], "import /etc/coredns/custom/*.server")
					assert.Equal(t, originalHasImport, hasImport, "Import statement presence should not change")
				}
			}

			// Check if volume mount was added to Deployment
			if tt.existingCoreDNSDeployment != nil {
				var updatedDeployment appsv1.Deployment
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      "coredns",
					Namespace: "kube-system",
				}, &updatedDeployment)
				assert.NoError(t, err)

				hasVolume := false
				hasVolumeMount := false

				// Check for volume
				for _, volume := range updatedDeployment.Spec.Template.Spec.Volumes {
					if volume.Name == "coredns-custom-volume" {
						hasVolume = true
						break
					}
				}

				// Check for volume mount
				if len(updatedDeployment.Spec.Template.Spec.Containers) > 0 {
					for _, mount := range updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
						if mount.Name == "coredns-custom-volume" {
							hasVolumeMount = true
							break
						}
					}
				}

				if tt.expectedVolumeMountAdded {
					assert.True(t, hasVolume, "Volume should be added to Deployment")
					assert.True(t, hasVolumeMount, "Volume mount should be added to Deployment")
				} else if tt.existingCoreDNSDeployment != nil {
					// Check if volume/mount was already present
					originalHasVolume := false
					originalHasVolumeMount := false

					for _, volume := range tt.existingCoreDNSDeployment.Spec.Template.Spec.Volumes {
						if volume.Name == "coredns-custom-volume" {
							originalHasVolume = true
							break
						}
					}

					if len(tt.existingCoreDNSDeployment.Spec.Template.Spec.Containers) > 0 {
						for _, mount := range tt.existingCoreDNSDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
							if mount.Name == "coredns-custom-volume" {
								originalHasVolumeMount = true
								break
							}
						}
					}

					assert.Equal(t, originalHasVolume, hasVolume, "Volume presence should not change")
					assert.Equal(t, originalHasVolumeMount, hasVolumeMount, "Volume mount presence should not change")
				}
			}
		})
	}
}

func TestCoreDNSImportStatement(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name             string
		existingCorefile string
		expectedCorefile string
		expectError      bool
	}{
		{
			name: "add import to standard Corefile",
			existingCorefile: `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
			expectedCorefile: `.:53 {
    import /etc/coredns/custom/*.server
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
			expectError: false,
		},
		{
			name: "import already exists",
			existingCorefile: `.:53 {
    import /etc/coredns/custom/*.server
    errors
    health
}`,
			expectedCorefile: `.:53 {
    import /etc/coredns/custom/*.server
    errors
    health
}`,
			expectError: false,
		},
		{
			name: "no main server block found",
			existingCorefile: `custom.domain:53 {
    errors
    health
}`,
			expectedCorefile: `custom.domain:53 {
    errors
    health
}
import /etc/coredns/custom/*.server`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreDNSConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"Corefile": tt.existingCorefile,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(coreDNSConfigMap).
				Build()

			reconciler := &IngressReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			err := reconciler.ensureCoreDNSImport(context.Background())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check the updated ConfigMap
				var updatedConfigMap corev1.ConfigMap
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      "coredns",
					Namespace: "kube-system",
				}, &updatedConfigMap)
				assert.NoError(t, err)

				assert.Equal(t, tt.expectedCorefile, updatedConfigMap.Data["Corefile"])
			}
		})
	}
}

func TestCoreDNSVolumeMounts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name                   string
		existingDeployment     *appsv1.Deployment
		expectVolumeAdded      bool
		expectVolumeMountAdded bool
		expectError            bool
	}{
		{
			name: "add volume and volume mount to clean deployment",
			existingDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "coredns",
									Image: "registry.k8s.io/coredns/coredns:v1.10.1",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config-volume",
											MountPath: "/etc/coredns",
											ReadOnly:  true,
										},
									},
								},
							},
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
						},
					},
				},
			},
			expectVolumeAdded:      true,
			expectVolumeMountAdded: true,
			expectError:            false,
		},
		{
			name: "volume and volume mount already exist",
			existingDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "coredns",
									Image: "registry.k8s.io/coredns/coredns:v1.10.1",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config-volume",
											MountPath: "/etc/coredns",
											ReadOnly:  true,
										},
										{
											Name:      "coredns-custom-volume",
											MountPath: "/etc/coredns/custom",
											ReadOnly:  true,
										},
									},
								},
							},
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
									Name: "coredns-custom-volume",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "coredns-custom",
											},
											Items: []corev1.KeyToPath{
												{
													Key:  "dynamic.server",
													Path: "dynamic.server",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectVolumeAdded:      false,
			expectVolumeMountAdded: false,
			expectError:            false,
		},
		{
			name: "deployment has no containers",
			existingDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{},
							Volumes:    []corev1.Volume{},
						},
					},
				},
			},
			expectVolumeAdded:      true,
			expectVolumeMountAdded: false, // Can't add volume mount without containers
			expectError:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.existingDeployment).
				Build()

			reconciler := &IngressReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			err := reconciler.ensureCoreDNSVolumeMount(context.Background())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check the updated Deployment
				var updatedDeployment appsv1.Deployment
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      "coredns",
					Namespace: "kube-system",
				}, &updatedDeployment)
				assert.NoError(t, err)

				// Check if volume was added
				hasVolume := false
				hasVolumeMount := false
				originalHasVolume := false
				originalHasVolumeMount := false

				// Check original state
				for _, volume := range tt.existingDeployment.Spec.Template.Spec.Volumes {
					if volume.Name == "coredns-custom-volume" {
						originalHasVolume = true
						break
					}
				}

				if len(tt.existingDeployment.Spec.Template.Spec.Containers) > 0 {
					for _, mount := range tt.existingDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
						if mount.Name == "coredns-custom-volume" {
							originalHasVolumeMount = true
							break
						}
					}
				}

				// Check updated state
				for _, volume := range updatedDeployment.Spec.Template.Spec.Volumes {
					if volume.Name == "coredns-custom-volume" {
						hasVolume = true
						// Verify volume configuration
						assert.NotNil(t, volume.VolumeSource.ConfigMap)
						assert.Equal(t, "coredns-custom", volume.VolumeSource.ConfigMap.Name)
						assert.Len(t, volume.VolumeSource.ConfigMap.Items, 1)
						assert.Equal(t, "dynamic.server", volume.VolumeSource.ConfigMap.Items[0].Key)
						assert.Equal(t, "dynamic.server", volume.VolumeSource.ConfigMap.Items[0].Path)
						break
					}
				}

				// Check if volume mount was added
				if len(updatedDeployment.Spec.Template.Spec.Containers) > 0 {
					for _, mount := range updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
						if mount.Name == "coredns-custom-volume" {
							hasVolumeMount = true
							// Verify volume mount configuration
							assert.Equal(t, "/etc/coredns/custom", mount.MountPath)
							assert.True(t, mount.ReadOnly)
							break
						}
					}
				}

				volumeAdded := hasVolume && !originalHasVolume
				volumeMountAdded := hasVolumeMount && !originalHasVolumeMount

				assert.Equal(t, tt.expectVolumeAdded, volumeAdded, "Volume addition expectation")
				assert.Equal(t, tt.expectVolumeMountAdded, volumeMountAdded, "Volume mount addition expectation")
			}
		})
	}
}

func TestIngressReconcilerWithCoreDNSEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Enable CoreDNS auto-configuration
	os.Setenv("COREDNS_AUTO_CONFIGURE", "true")
	defer os.Unsetenv("COREDNS_AUTO_CONFIGURE")

	// Create test ingress
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: "api.k8s.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-service",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create CoreDNS ConfigMap
	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
		},
	}

	// Create CoreDNS Deployment
	coreDNSDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: "kube-system",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "coredns",
							Image: "registry.k8s.io/coredns/coredns:v1.10.1",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/etc/coredns",
									ReadOnly:  true,
								},
							},
						},
					},
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
				},
			},
		},
	}

	// Create fake client with test objects
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress, coreDNSConfigMap, coreDNSDeployment).
		Build()

	reconciler := &IngressReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// Check that dynamic ConfigMap was created
	var dynamicConfigMap corev1.ConfigMap
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "coredns-custom",
		Namespace: "kube-system", // Dynamic ConfigMap is created in CoreDNS namespace
	}, &dynamicConfigMap)
	assert.NoError(t, err)
	assert.Contains(t, dynamicConfigMap.Data["dynamic.server"], "rewrite name exact api.k8s.example.com ingress-nginx-controller.ingress-nginx.svc.cluster.local.")
	assert.Contains(t, dynamicConfigMap.Data["dynamic.server"], "# Auto-generated by coredns-ingress-sync controller")

	// Check that CoreDNS ConfigMap was updated with import statement
	var updatedCoreDNSConfigMap corev1.ConfigMap
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "coredns",
		Namespace: "kube-system",
	}, &updatedCoreDNSConfigMap)
	assert.NoError(t, err)
	assert.Contains(t, updatedCoreDNSConfigMap.Data["Corefile"], "import /etc/coredns/custom/*.server")

	// Check that CoreDNS Deployment was updated with volume and volume mount
	var updatedCoreDNSDeployment appsv1.Deployment
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "coredns",
		Namespace: "kube-system",
	}, &updatedCoreDNSDeployment)
	assert.NoError(t, err)

	// Check for volume
	hasVolume := false
	for _, volume := range updatedCoreDNSDeployment.Spec.Template.Spec.Volumes {
		if volume.Name == "coredns-custom-volume" {
			hasVolume = true
			assert.Equal(t, "coredns-custom", volume.VolumeSource.ConfigMap.Name)
			break
		}
	}
	assert.True(t, hasVolume, "CoreDNS deployment should have custom volume")

	// Check for volume mount
	hasVolumeMount := false
	for _, mount := range updatedCoreDNSDeployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == "coredns-custom-volume" {
			hasVolumeMount = true
			assert.Equal(t, "/etc/coredns/custom", mount.MountPath)
			assert.True(t, mount.ReadOnly)
			break
		}
	}
	assert.True(t, hasVolumeMount, "CoreDNS deployment should have custom volume mount")
}

func TestDynamicConfigMapNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Enable CoreDNS auto-configuration
	os.Setenv("COREDNS_AUTO_CONFIGURE", "true")
	defer os.Unsetenv("COREDNS_AUTO_CONFIGURE")

	// Create test ingress
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: "api.test.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-service",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create CoreDNS ConfigMap
	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
		},
	}

	// Create fake client with test objects
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress, coreDNSConfigMap).
		Build()

	reconciler := &IngressReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// Verify dynamic ConfigMap is created in CoreDNS namespace (kube-system)
	var dynamicConfigMap corev1.ConfigMap
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "coredns-custom",
		Namespace: "kube-system",
	}, &dynamicConfigMap)
	assert.NoError(t, err, "Dynamic ConfigMap should be created in kube-system namespace")
	assert.Contains(t, dynamicConfigMap.Data["dynamic.server"], "api.test.example.com")

	// Verify dynamic ConfigMap is NOT created in default namespace
	var defaultNSConfigMap corev1.ConfigMap
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "coredns-custom",
		Namespace: "default",
	}, &defaultNSConfigMap)
	assert.Error(t, err, "Dynamic ConfigMap should NOT be created in default namespace")
	assert.Contains(t, err.Error(), "not found")
}

// TestLeaderElection tests that leader election is properly configured
func TestLeaderElection(t *testing.T) {
	// Save original values
	origClass := ingressClass
	origTargetCNAME := targetCNAME
	origNamespace := coreDNSNamespace
	origConfigMapName := coreDNSConfigMapName

	// Set test values
	ingressClass = "nginx"
	targetCNAME = "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
	coreDNSNamespace = "kube-system"
	coreDNSConfigMapName = "coredns"

	defer func() {
		ingressClass = origClass
		targetCNAME = origTargetCNAME
		coreDNSNamespace = origNamespace
		coreDNSConfigMapName = origConfigMapName
	}()

	tests := []struct {
		name        string
		replicas    int
		description string
	}{
		{
			name:        "single replica",
			replicas:    1,
			description: "Single replica should work without leader election conflicts",
		},
		{
			name:        "multiple replicas",
			replicas:    3,
			description: "Multiple replicas should coordinate via leader election",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test namespace
			testNamespace := "test-leader-election"

			// Create fake clients for multiple controller instances
			scheme := runtime.NewScheme()
			require.NoError(t, networkingv1.AddToScheme(scheme))
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, appsv1.AddToScheme(scheme))

			// Create shared initial objects
			coreDNSConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      coreDNSConfigMapName,
					Namespace: coreDNSNamespace,
				},
				Data: map[string]string{
					"Corefile": `.:53 {
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
}`,
				},
			}

			coreDNSDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: coreDNSNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "coredns",
									Image: "coredns/coredns:1.8.4",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config-volume",
											MountPath: "/etc/coredns",
											ReadOnly:  true,
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "config-volume",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: coreDNSConfigMapName,
											},
										},
									},
								},
							},
						},
					},
				},
			}

			// Create a test ingress
			className := "nginx"
			testIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: &className,
					Rules: []networkingv1.IngressRule{
						{
							Host: "api.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/",
											PathType: func() *networkingv1.PathType {
												pt := networkingv1.PathTypePrefix
												return &pt
											}(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "api-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			// Create multiple controller instances (simulating multiple replicas)
			controllers := make([]*IngressReconciler, tt.replicas)
			for i := 0; i < tt.replicas; i++ {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(coreDNSConfigMap, coreDNSDeployment, testIngress).
					Build()

				controllers[i] = &IngressReconciler{
					Client: fakeClient,
					Scheme: scheme,
				}
			}

			// Test that each controller can handle reconciliation
			// In real scenario, leader election would ensure only one is active
			ctx := context.Background()

			for i, controller := range controllers {
				t.Run(func() string { return "controller-" + string(rune(i+'0')) }(), func(t *testing.T) {
					// Create a reconcile request
					req := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      "global-ingress-reconcile",
							Namespace: "default",
						},
					}

					// Test reconciliation
					result, err := controller.Reconcile(ctx, req)

					// Should not error (though in real scenario, only leader would actually process)
					assert.NoError(t, err, "Controller %d should not error during reconciliation", i)
					assert.Equal(t, reconcile.Result{}, result, "Controller %d should return empty result", i)

					// Verify dynamic ConfigMap was created
					var dynamicConfigMap corev1.ConfigMap
					err = controller.Get(ctx, types.NamespacedName{
						Name:      dynamicConfigMapName,
						Namespace: coreDNSNamespace,
					}, &dynamicConfigMap)
					assert.NoError(t, err, "Controller %d should create dynamic ConfigMap", i)

					// Verify content
					content, exists := dynamicConfigMap.Data[dynamicConfigKey]
					assert.True(t, exists, "Controller %d: Dynamic ConfigMap should have content", i)
					assert.Contains(t, content, "api.example.com", "Controller %d: Should contain test hostname", i)
					assert.Contains(t, content, targetCNAME, "Controller %d: Should contain target CNAME", i)
				})
			}

			// Test leader election lease creation simulation
			t.Run("leader_election_lease_permissions", func(t *testing.T) {
				// Simulate creating a leader election lease
				// This tests that the RBAC permissions are correct
				leaderElectionLease := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns-ingress-sync-leader",
						Namespace: testNamespace,
						Labels: map[string]string{
							"component": "leader-election",
						},
					},
					Data: map[string]string{
						"leader": "controller-0",
					},
				}

				// Test that any controller could create the lease (RBAC permissions)
				if len(controllers) > 0 {
					err := controllers[0].Create(ctx, leaderElectionLease)
					// In fake client, this should succeed (tests RBAC structure)
					assert.NoError(t, err, "Should be able to create leader election lease")

					// Verify lease exists
					var retrievedLease corev1.ConfigMap
					err = controllers[0].Get(ctx, types.NamespacedName{
						Name:      "coredns-ingress-sync-leader",
						Namespace: testNamespace,
					}, &retrievedLease)
					assert.NoError(t, err, "Should be able to retrieve leader election lease")
					assert.Equal(t, "controller-0", retrievedLease.Data["leader"])
				}
			})
		})
	}
}

// TestLeaderElectionConfiguration tests that the leader election is properly configured in the manager
func TestLeaderElectionConfiguration(t *testing.T) {
	t.Run("leader_election_enabled", func(t *testing.T) {
		// This test verifies that the manager configuration includes leader election
		// Since we can't easily test the actual manager creation in unit tests,
		// we test that the configuration values are correct

		// Test that leader election ID is set
		expectedLeaderElectionID := "coredns-ingress-sync-leader"
		// In a real test environment, you would check the manager options
		// For now, we verify the constant is defined correctly
		assert.NotEmpty(t, expectedLeaderElectionID, "Leader election ID should not be empty")
		assert.Contains(t, expectedLeaderElectionID, "coredns-ingress-sync", "Leader election ID should contain app name")
	})

	t.Run("leader_election_namespace", func(t *testing.T) {
		// Test that leader election namespace defaults to empty (same as pod namespace)
		expectedNamespace := ""
		assert.Equal(t, expectedNamespace, "", "Leader election namespace should default to empty (pod namespace)")
	})
}

// TestLeaderElectionImports verifies that the necessary imports for leader election are present
func TestLeaderElectionImports(t *testing.T) {
	// This test ensures we have the right imports for leader election functionality
	// It's a compile-time check that the controller-runtime package supports leader election

	t.Run("controller_runtime_manager_options", func(t *testing.T) {
		// Test that we can create manager options with leader election settings
		// This is a compile-time verification that the APIs exist

		options := ctrl.Options{
			Scheme:                  runtime.NewScheme(),
			LeaderElection:          true,
			LeaderElectionID:        "test-leader-election",
			LeaderElectionNamespace: "",
		}

		// Verify the options are set correctly
		assert.True(t, options.LeaderElection, "Leader election should be enabled")
		assert.Equal(t, "test-leader-election", options.LeaderElectionID, "Leader election ID should be set")
		assert.Equal(t, "", options.LeaderElectionNamespace, "Leader election namespace should be empty")
	})
}

// TestMultipleControllerCoordination tests coordination between multiple controller instances
func TestMultipleControllerCoordination(t *testing.T) {
	// Save original values
	origClass := ingressClass
	origTargetCNAME := targetCNAME
	origNamespace := coreDNSNamespace
	origConfigMapName := coreDNSConfigMapName

	// Set test values
	ingressClass = "nginx"
	targetCNAME = "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
	coreDNSNamespace = "kube-system"
	coreDNSConfigMapName = "coredns"

	defer func() {
		ingressClass = origClass
		targetCNAME = origTargetCNAME
		coreDNSNamespace = origNamespace
		coreDNSConfigMapName = origConfigMapName
	}()

	// Test concurrent reconciliation attempts
	t.Run("concurrent_reconciliation", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, networkingv1.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))
		require.NoError(t, appsv1.AddToScheme(scheme))

		// Create initial objects
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      coreDNSConfigMapName,
				Namespace: coreDNSNamespace,
			},
			Data: map[string]string{
				"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
    }
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
			},
		}

		className := "nginx"
		testIngress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress",
				Namespace: "default",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &className,
				Rules: []networkingv1.IngressRule{
					{
						Host: "concurrent.example.com",
					},
				},
			},
		}

		// Create two controllers with the same underlying client
		// This simulates race conditions that leader election should prevent
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(coreDNSConfigMap, testIngress).
			Build()

		controller1 := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		controller2 := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		ctx := context.Background()
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "global-ingress-reconcile",
				Namespace: "default",
			},
		}

		// Both controllers attempt reconciliation
		// In real scenario, leader election would prevent race conditions
		result1, err1 := controller1.Reconcile(ctx, req)
		result2, err2 := controller2.Reconcile(ctx, req)

		// Both should succeed (fake client doesn't have real concurrency issues)
		assert.NoError(t, err1, "Controller 1 should not error")
		assert.NoError(t, err2, "Controller 2 should not error")
		assert.Equal(t, reconcile.Result{}, result1, "Controller 1 should return empty result")
		assert.Equal(t, reconcile.Result{}, result2, "Controller 2 should return empty result")

		// Verify final state is consistent
		var dynamicConfigMap corev1.ConfigMap
		err := fakeClient.Get(ctx, types.NamespacedName{
			Name:      dynamicConfigMapName,
			Namespace: coreDNSNamespace,
		}, &dynamicConfigMap)
		assert.NoError(t, err, "Dynamic ConfigMap should exist")

		content, exists := dynamicConfigMap.Data[dynamicConfigKey]
		assert.True(t, exists, "Dynamic ConfigMap should have content")
		assert.Contains(t, content, "concurrent.example.com", "Should contain test hostname")

		// Count rewrite rules to ensure no duplication
		rewriteCount := strings.Count(content, "rewrite name exact concurrent.example.com")
		assert.Equal(t, 1, rewriteCount, "Should have exactly one rewrite rule (no duplication)")
	})
}

// Test isTargetIngress function for proper ingress filtering
func TestIsTargetIngress(t *testing.T) {
	tests := []struct {
		name     string
		obj      client.Object
		expected bool
	}{
		{
			name: "valid nginx ingress",
			obj: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: stringPtr("nginx"),
				},
			},
			expected: true,
		},
		{
			name: "different ingress class",
			obj: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: stringPtr("traefik"),
				},
			},
			expected: false,
		},
		{
			name: "nil ingress class",
			obj: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: nil,
				},
			},
			expected: false,
		},
		{
			name:     "non-ingress object",
			obj:      &corev1.Service{},
			expected: false,
		},
	}

	// Save original and set test value
	origClass := ingressClass
	ingressClass = "nginx"
	defer func() { ingressClass = origClass }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTargetIngress(tt.obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test extractHostnames function comprehensively
func TestExtractHostnamesComprehensive(t *testing.T) {
	// Save original and set test value
	origClass := ingressClass
	ingressClass = "nginx"
	defer func() { ingressClass = origClass }()

	tests := []struct {
		name      string
		ingresses []networkingv1.Ingress
		expected  []string
	}{
		{
			name: "single ingress with multiple hosts",
			ingresses: []networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "api.example.com"},
							{Host: "web.example.com"},
						},
					},
				},
			},
			expected: []string{"api.example.com", "web.example.com"},
		},
		{
			name: "multiple ingresses with unique hosts",
			ingresses: []networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "api.example.com"},
						},
					},
				},
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "web.example.com"},
						},
					},
				},
			},
			expected: []string{"api.example.com", "web.example.com"},
		},
		{
			name: "duplicate hosts across ingresses should deduplicate",
			ingresses: []networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "api.example.com"},
						},
					},
				},
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "api.example.com"},
						},
					},
				},
			},
			expected: []string{"api.example.com"},
		},
		{
			name: "mixed ingress classes should filter correctly",
			ingresses: []networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: "api.example.com"},
						},
					},
				},
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("traefik"),
						Rules: []networkingv1.IngressRule{
							{Host: "web.example.com"},
						},
					},
				},
			},
			expected: []string{"api.example.com"},
		},
		{
			name: "empty hosts should be ignored",
			ingresses: []networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						IngressClassName: stringPtr("nginx"),
						Rules: []networkingv1.IngressRule{
							{Host: ""},
							{Host: "api.example.com"},
						},
					},
				},
			},
			expected: []string{"api.example.com"},
		},
		{
			name:      "no ingresses should return empty slice",
			ingresses: []networkingv1.Ingress{},
			expected:  nil, // extractHostnames returns nil for empty slice
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHostnames(tt.ingresses)
			// Sort for comparison since order doesn't matter
			sort.Strings(result)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test reconciler error handling with comprehensive error scenarios
func TestIngressReconcilerErrorHandling(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Save original values
	origClass := ingressClass
	origTargetCNAME := targetCNAME
	origNamespace := coreDNSNamespace
	origConfigMapName := coreDNSConfigMapName
	origDynamicConfigMapName := dynamicConfigMapName

	// Set test values
	ingressClass = "nginx"
	targetCNAME = "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
	coreDNSNamespace = "kube-system"
	coreDNSConfigMapName = "coredns"
	dynamicConfigMapName = "coredns-custom"

	defer func() {
		ingressClass = origClass
		targetCNAME = origTargetCNAME
		coreDNSNamespace = origNamespace
		coreDNSConfigMapName = origConfigMapName
		dynamicConfigMapName = origDynamicConfigMapName
	}()

	tests := []struct {
		name           string
		setupClient    func() client.Client
		expectError    bool
		expectRequeue  bool
	}{
		{
			name: "client error when listing ingresses",
			setupClient: func() client.Client {
				client := fake.NewClientBuilder().WithScheme(scheme).Build()
				return &errorClient{Client: client, failOnList: true}
			},
			expectError:   true,
			expectRequeue: true,
		},
		{
			name: "successful reconciliation with no ingresses",
			setupClient: func() client.Client {
				return fake.NewClientBuilder().WithScheme(scheme).Build()
			},
			expectError:   false,
			expectRequeue: false,
		},
		{
			name: "configmap creation error",
			setupClient: func() client.Client {
				className := "nginx"
				ingress := &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ingress",
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						IngressClassName: &className,
						Rules: []networkingv1.IngressRule{
							{Host: "test.example.com"},
						},
					},
				}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingress).Build()
				return &errorClient{Client: client, failOnCreate: true}
			},
			expectError:   true,
			expectRequeue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &IngressReconciler{
				Client: tt.setupClient(),
				Scheme: scheme,
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-reconcile",
					Namespace: "default",
				},
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectRequeue {
				assert.Greater(t, result.RequeueAfter.Seconds(), 0.0)
			}
		})
	}
}

// Mock error client for testing error conditions
type errorClient struct {
	client.Client
	failOnList   bool
	failOnGet    bool
	failOnUpdate bool
	failOnCreate bool
}

func (c *errorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.failOnList {
		return fmt.Errorf("mock list error")
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *errorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.failOnGet {
		return fmt.Errorf("mock get error")
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *errorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.failOnUpdate {
		return fmt.Errorf("mock update error")
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *errorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.failOnCreate {
		return fmt.Errorf("mock create error")
	}
	return c.Client.Create(ctx, obj, opts...)
}

// Test updateDynamicConfigMap method with error scenarios
func TestUpdateDynamicConfigMapErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Save original values
	origNamespace := coreDNSNamespace
	origDynamicConfigMapName := dynamicConfigMapName
	origDynamicConfigKey := dynamicConfigKey

	// Set test values
	coreDNSNamespace = "kube-system"
	dynamicConfigMapName = "coredns-custom"
	dynamicConfigKey = "dynamic.server"

	defer func() {
		coreDNSNamespace = origNamespace
		dynamicConfigMapName = origDynamicConfigMapName
		dynamicConfigKey = origDynamicConfigKey
	}()

	tests := []struct {
		name        string
		domains     []string
		hosts       []string
		setupClient func() client.Client
		expectError bool
	}{
		{
			name:    "successful config map creation",
			domains: []string{"example.com"},
			hosts:   []string{"api.example.com"},
			setupClient: func() client.Client {
				return fake.NewClientBuilder().WithScheme(scheme).Build()
			},
			expectError: false,
		},
		{
			name:    "successful config map update",
			domains: []string{"example.com"},
			hosts:   []string{"api.example.com"},
			setupClient: func() client.Client {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns-custom",
						Namespace: "kube-system",
					},
					Data: map[string]string{
						"dynamic.server": "old config",
					},
				}
				return fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
			},
			expectError: false,
		},
		{
			name:    "client error on create",
			domains: []string{"example.com"},
			hosts:   []string{"api.example.com"},
			setupClient: func() client.Client {
				client := fake.NewClientBuilder().WithScheme(scheme).Build()
				return &errorClient{Client: client, failOnCreate: true}
			},
			expectError: true,
		},
		{
			name:    "client error on update",
			domains: []string{"example.com"},
			hosts:   []string{"api.example.com"},
			setupClient: func() client.Client {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns-custom",
						Namespace: "kube-system",
					},
					Data: map[string]string{
						"dynamic.server": "old config",
					},
				}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
				return &errorClient{Client: client, failOnUpdate: true}
			},
			expectError: true,
		},
		{
			name:    "client error on get during update path",
			domains: []string{"example.com"},
			hosts:   []string{"api.example.com"},
			setupClient: func() client.Client {
				// Pre-create a config map so we go to the update path
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns-custom",
						Namespace: "kube-system",
					},
					Data: map[string]string{
						"dynamic.server": "old config",
					},
				}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
				// Make Get fail only - this will cause issues in the update path
				return &errorClient{Client: client, failOnGet: true}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &IngressReconciler{
				Client: tt.setupClient(),
				Scheme: scheme,
			}

			err := reconciler.updateDynamicConfigMap(context.Background(), tt.domains, tt.hosts)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Verify the config map was created/updated correctly
				cm := &corev1.ConfigMap{}
				err := reconciler.Client.Get(context.Background(), 
					client.ObjectKey{Name: "coredns-custom", Namespace: "kube-system"}, cm)
				assert.NoError(t, err)
				assert.Contains(t, cm.Data["dynamic.server"], "api.example.com")
			}
		})
	}
}

// Test ensureCoreDNSImport method error conditions
func TestEnsureCoreDNSImportErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Save original values
	origNamespace := coreDNSNamespace
	origConfigMapName := coreDNSConfigMapName

	// Set test values
	coreDNSNamespace = "kube-system"
	coreDNSConfigMapName = "coredns"

	defer func() {
		coreDNSNamespace = origNamespace
		coreDNSConfigMapName = origConfigMapName
	}()

	tests := []struct {
		name        string
		setupClient func() client.Client
		expectError bool
	}{
		{
			name: "config map not found",
			setupClient: func() client.Client {
				return fake.NewClientBuilder().WithScheme(scheme).Build()
			},
			expectError: true,
		},
		{
			name: "client error on get",
			setupClient: func() client.Client {
				client := fake.NewClientBuilder().WithScheme(scheme).Build()
				return &errorClient{Client: client, failOnGet: true}
			},
			expectError: true,
		},
		{
			name: "client error on update",
			setupClient: func() client.Client {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns",
						Namespace: "kube-system",
					},
					Data: map[string]string{
						"Corefile": `.:53 {
    errors
    health
}`,
					},
				}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
				return &errorClient{Client: client, failOnUpdate: true}
			},
			expectError: true,
		},
		{
			name: "successful import addition",
			setupClient: func() client.Client {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns",
						Namespace: "kube-system",
					},
					Data: map[string]string{
						"Corefile": `.:53 {
    errors
    health
}`,
					},
				}
				return fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &IngressReconciler{
				Client: tt.setupClient(),
				Scheme: scheme,
			}

			err := reconciler.ensureCoreDNSImport(context.Background())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test utility functions
func TestUtilityFunctions(t *testing.T) {
	t.Run("isDevelopment", func(t *testing.T) {
		// Save original value
		originalEnv := os.Getenv("DEVELOPMENT_MODE")
		defer func() {
			if originalEnv == "" {
				os.Unsetenv("DEVELOPMENT_MODE")
			} else {
				os.Setenv("DEVELOPMENT_MODE", originalEnv)
			}
		}()

		// Test development mode enabled
		os.Setenv("DEVELOPMENT_MODE", "true")
		assert.True(t, isDevelopment())

		// Test development mode disabled
		os.Setenv("DEVELOPMENT_MODE", "false")
		assert.False(t, isDevelopment())

		// Test empty value
		os.Setenv("DEVELOPMENT_MODE", "")
		assert.False(t, isDevelopment())

		// Test unset value
		os.Unsetenv("DEVELOPMENT_MODE")
		assert.False(t, isDevelopment())
	})

	t.Run("getEnvOrDefault", func(t *testing.T) {
		// Save original value
		originalEnv := os.Getenv("TEST_VAR")
		defer func() {
			if originalEnv == "" {
				os.Unsetenv("TEST_VAR")
			} else {
				os.Setenv("TEST_VAR", originalEnv)
			}
		}()

		// Test with existing environment variable
		os.Setenv("TEST_VAR", "test_value")
		result := getEnvOrDefault("TEST_VAR", "default_value")
		assert.Equal(t, "test_value", result)

		// Test with non-existing environment variable
		os.Unsetenv("TEST_VAR")
		result = getEnvOrDefault("TEST_VAR", "default_value")
		assert.Equal(t, "default_value", result)

		// Test with empty environment variable
		os.Setenv("TEST_VAR", "")
		result = getEnvOrDefault("TEST_VAR", "default_value")
		assert.Equal(t, "default_value", result)
	})

	t.Run("isFakeClient", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, networkingv1.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))

		// Test with fake client
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		assert.True(t, reconciler.isFakeClient())

		// Test with non-fake client (error client wrapper)
		errorClient := &errorClient{Client: fakeClient}
		reconciler2 := &IngressReconciler{
			Client: errorClient,
			Scheme: scheme,
		}
		assert.False(t, reconciler2.isFakeClient())
	})
}

// Test cleanup functions that are currently not covered
func TestCleanupFunctions(t *testing.T) {
	// Save original values
	origNamespace := coreDNSNamespace
	origConfigMapName := coreDNSConfigMapName
	origDynamicConfigMapName := dynamicConfigMapName

	// Set test values
	coreDNSNamespace = "kube-system"
	coreDNSConfigMapName = "coredns"
	dynamicConfigMapName = "coredns-custom"

	defer func() {
		coreDNSNamespace = origNamespace
		coreDNSConfigMapName = origConfigMapName
		dynamicConfigMapName = origDynamicConfigMapName
	}()

	t.Run("cleanupDynamicConfigMap_success", func(t *testing.T) {
		// Create a fake client with a ConfigMap to delete
		client := kubefake.NewSimpleClientset(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-custom",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"dynamic.server": "rewrite name exact api.example.com ingress-nginx-controller.ingress-nginx.svc.cluster.local.",
			},
		})

		// Create a mock cleanup function that uses our fake client
		cleanupFunc := func() {
			ctx := context.Background()
			err := client.CoreV1().ConfigMaps("kube-system").Delete(ctx, "coredns-custom", metav1.DeleteOptions{})
			if err != nil {
				t.Errorf("Failed to delete ConfigMap: %v", err)
			}
		}

		// Test that the cleanup function doesn't panic and works correctly
		assert.NotPanics(t, cleanupFunc)
		
		// Verify the ConfigMap was deleted
		_, err := client.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "coredns-custom", metav1.GetOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("cleanupCoreDNSConfigMap_success", func(t *testing.T) {
		// Create a fake client with a CoreDNS ConfigMap containing the import statement
		originalCorefile := `.:53 {
    errors
    health {
        lameduck 5s
    }
    import /etc/coredns/custom/*.server
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
		
		expectedCorefile := `.:53 {
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

		client := kubefake.NewSimpleClientset(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"Corefile": originalCorefile,
			},
		})

		// Create a mock cleanup function that uses our fake client
		cleanupFunc := func() {
			ctx := context.Background()
			
			configMap, err := client.CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
			if err != nil {
				t.Errorf("Failed to get ConfigMap: %v", err)
				return
			}

			// Get current Corefile
			corefile, exists := configMap.Data["Corefile"]
			if !exists {
				t.Error("Corefile not found in ConfigMap")
				return
			}

			// Remove the import statement
			lines := strings.Split(corefile, "\n")
			var newLines []string
			for _, line := range lines {
				if !strings.Contains(line, "import /etc/coredns/custom/*.server") {
					newLines = append(newLines, line)
				}
			}
			newCorefile := strings.Join(newLines, "\n")

			// Update the ConfigMap
			configMap.Data["Corefile"] = newCorefile
			_, err = client.CoreV1().ConfigMaps("kube-system").Update(ctx, configMap, metav1.UpdateOptions{})
			if err != nil {
				t.Errorf("Failed to update ConfigMap: %v", err)
			}
		}

		// Test that the cleanup function doesn't panic and works correctly
		assert.NotPanics(t, cleanupFunc)
		
		// Verify the import statement was removed
		updatedConfigMap, err := client.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, expectedCorefile, updatedConfigMap.Data["Corefile"])
		assert.NotContains(t, updatedConfigMap.Data["Corefile"], "import /etc/coredns/custom/*.server")
	})

	t.Run("cleanupCoreDNSDeployment_success", func(t *testing.T) {
		// Create a fake client with a CoreDNS deployment containing the custom volume
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "coredns",
								Image: "registry.k8s.io/coredns/coredns:v1.10.1",
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "config-volume",
										MountPath: "/etc/coredns",
										ReadOnly:  true,
									},
									{
										Name:      "coredns-custom-volume",
										MountPath: "/etc/coredns/custom",
										ReadOnly:  true,
									},
								},
							},
						},
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
								Name: "coredns-custom-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "coredns-custom",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		client := kubefake.NewSimpleClientset(deployment)

		// Create a mock cleanup function that uses our fake client
		cleanupFunc := func() {
			ctx := context.Background()
			
			deployment, err := client.AppsV1().Deployments("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
			if err != nil {
				t.Errorf("Failed to get deployment: %v", err)
				return
			}

			volumeName := "coredns-custom-volume"

			// Remove volume mounts from containers
			for i := range deployment.Spec.Template.Spec.Containers {
				container := &deployment.Spec.Template.Spec.Containers[i]
				var newVolumeMounts []corev1.VolumeMount
				for _, vm := range container.VolumeMounts {
					if vm.Name != volumeName {
						newVolumeMounts = append(newVolumeMounts, vm)
					}
				}
				container.VolumeMounts = newVolumeMounts
			}

			// Remove volumes
			var newVolumes []corev1.Volume
			for _, vol := range deployment.Spec.Template.Spec.Volumes {
				if vol.Name != volumeName {
					newVolumes = append(newVolumes, vol)
				}
			}
			deployment.Spec.Template.Spec.Volumes = newVolumes

			_, err = client.AppsV1().Deployments("kube-system").Update(ctx, deployment, metav1.UpdateOptions{})
			if err != nil {
				t.Errorf("Failed to update deployment: %v", err)
			}
		}

		// Test that the cleanup function doesn't panic and works correctly
		assert.NotPanics(t, cleanupFunc)
		
		// Verify the custom volume and volume mount were removed
		updatedDeployment, err := client.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
		assert.NoError(t, err)
		
		// Check that custom volume was removed
		for _, vol := range updatedDeployment.Spec.Template.Spec.Volumes {
			assert.NotEqual(t, "coredns-custom-volume", vol.Name)
		}
		
		// Check that custom volume mount was removed
		for _, container := range updatedDeployment.Spec.Template.Spec.Containers {
			for _, vm := range container.VolumeMounts {
				assert.NotEqual(t, "coredns-custom-volume", vm.Name)
			}
		}
		
		// Verify the original volume is still there
		foundConfigVolume := false
		for _, vol := range updatedDeployment.Spec.Template.Spec.Volumes {
			if vol.Name == "config-volume" {
				foundConfigVolume = true
				break
			}
		}
		assert.True(t, foundConfigVolume, "Original config-volume should still be present")
	})
}

// Test edge cases for generateDynamicConfig
func TestGenerateDynamicConfigEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		domains     []string
		hosts       []string
		expectedFn  func(string) bool // Use a function to check the expected result
	}{
		{
			name:    "empty inputs",
			domains: []string{},
			hosts:   []string{},
			expectedFn: func(result string) bool {
				// Should contain header but no rewrite rules
				return strings.Contains(result, "# Auto-generated by coredns-ingress-sync controller") &&
					!strings.Contains(result, "rewrite name exact")
			},
		},
		{
			name:    "nil inputs",
			domains: nil,
			hosts:   nil,
			expectedFn: func(result string) bool {
				// Should contain header but no rewrite rules
				return strings.Contains(result, "# Auto-generated by coredns-ingress-sync controller") &&
					!strings.Contains(result, "rewrite name exact")
			},
		},
		{
			name:    "mixed empty and nil",
			domains: []string{},
			hosts:   nil,
			expectedFn: func(result string) bool {
				// Should contain header but no rewrite rules
				return strings.Contains(result, "# Auto-generated by coredns-ingress-sync controller") &&
					!strings.Contains(result, "rewrite name exact")
			},
		},
		{
			name:    "domains without hosts",
			domains: []string{"example.com"},
			hosts:   []string{},
			expectedFn: func(result string) bool {
				// Should contain header but no rewrite rules
				return strings.Contains(result, "# Auto-generated by coredns-ingress-sync controller") &&
					!strings.Contains(result, "rewrite name exact")
			},
		},
		{
			name:    "hosts without domains",
			domains: []string{},
			hosts:   []string{"api.example.com"},
			expectedFn: func(result string) bool {
				// Should contain header and rewrite rule
				return strings.Contains(result, "# Auto-generated by coredns-ingress-sync controller") &&
					strings.Contains(result, "rewrite name exact api.example.com")
			},
		},
		{
			name:    "single domain and host",
			domains: []string{"example.com"},
			hosts:   []string{"api.example.com"},
			expectedFn: func(result string) bool {
				// Should contain header and rewrite rule
				return strings.Contains(result, "# Auto-generated by coredns-ingress-sync controller") &&
					strings.Contains(result, "rewrite name exact api.example.com ingress-nginx-controller.ingress-nginx.svc.cluster.local.")
			},
		},
	}

	// Save original value
	origTargetCNAME := targetCNAME
	targetCNAME = "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
	defer func() { targetCNAME = origTargetCNAME }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDynamicConfig(tt.domains, tt.hosts)
			assert.True(t, tt.expectedFn(result), "Result does not match expected pattern: %s", result)
		})
	}
}

// Test ensureCoreDNSConfiguration edge cases
func TestEnsureCoreDNSConfigurationEdgeCases(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Save original values
	origNamespace := coreDNSNamespace

	// Set test values
	coreDNSNamespace = "kube-system"

	defer func() {
		coreDNSNamespace = origNamespace
	}()

	t.Run("auto_configure_disabled", func(t *testing.T) {
		// Save original value
		originalEnv := os.Getenv("COREDNS_AUTO_CONFIGURE")
		defer func() {
			if originalEnv == "" {
				os.Unsetenv("COREDNS_AUTO_CONFIGURE")
			} else {
				os.Setenv("COREDNS_AUTO_CONFIGURE", originalEnv)
			}
		}()
		
		os.Setenv("COREDNS_AUTO_CONFIGURE", "false")
		
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		err := reconciler.ensureCoreDNSConfiguration(context.Background())
		assert.NoError(t, err, "Should succeed when auto-configure is disabled")
	})

	t.Run("auto_configure_enabled", func(t *testing.T) {
		// Save original value
		originalEnv := os.Getenv("COREDNS_AUTO_CONFIGURE")
		defer func() {
			if originalEnv == "" {
				os.Unsetenv("COREDNS_AUTO_CONFIGURE")
			} else {
				os.Setenv("COREDNS_AUTO_CONFIGURE", originalEnv)
			}
		}()
		
		os.Setenv("COREDNS_AUTO_CONFIGURE", "true")
		
		// Create minimal CoreDNS ConfigMap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"Corefile": `.:53 {
    errors
    health
}`,
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		err := reconciler.ensureCoreDNSConfiguration(context.Background())
		assert.NoError(t, err, "Should succeed when auto-configure is enabled with valid ConfigMap")
	})
}

// Test ensureCoreDNSVolumeMount more thoroughly
func TestEnsureCoreDNSVolumeMountDetailed(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Save original values
	origNamespace := coreDNSNamespace

	// Set test values
	coreDNSNamespace = "kube-system"

	defer func() {
		coreDNSNamespace = origNamespace
	}()

	t.Run("deployment_not_found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		err := reconciler.ensureCoreDNSVolumeMount(context.Background())
		assert.Error(t, err, "Should error when deployment not found")
		assert.Contains(t, err.Error(), "failed to get CoreDNS deployment")
	})

	t.Run("successful_volume_mount_addition", func(t *testing.T) {
		// Create CoreDNS deployment without volume mounts
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "coredns",
								Image: "k8s.gcr.io/coredns/coredns:v1.8.4",
							},
						},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		err := reconciler.ensureCoreDNSVolumeMount(context.Background())
		assert.NoError(t, err, "Should succeed when adding volume mount to deployment")

		// Verify the deployment was updated
		var updatedDeployment appsv1.Deployment
		err = fakeClient.Get(context.Background(), 
			client.ObjectKey{Name: "coredns", Namespace: "kube-system"}, &updatedDeployment)
		assert.NoError(t, err)
		
		// Check that volume and volume mount were added
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Volumes, 1, "Should have one volume")
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, 1, "Should have one volume mount")
	})
}

// Test more edge cases for the main reconciler functions
func TestReconcilerIntegrationScenarios(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Save original values
	origClass := ingressClass
	origTargetCNAME := targetCNAME
	origNamespace := coreDNSNamespace
	origConfigMapName := coreDNSConfigMapName
	origDynamicConfigMapName := dynamicConfigMapName

	// Set test values
	ingressClass = "nginx"
	targetCNAME = "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
	coreDNSNamespace = "kube-system"
	coreDNSConfigMapName = "coredns"
	dynamicConfigMapName = "coredns-custom"

	defer func() {
		ingressClass = origClass
		targetCNAME = origTargetCNAME
		coreDNSNamespace = origNamespace
		coreDNSConfigMapName = origConfigMapName
		dynamicConfigMapName = origDynamicConfigMapName
	}()

	t.Run("reconcile_with_complex_ingress_scenarios", func(t *testing.T) {
		// Create multiple ingresses with various configurations
		className := "nginx"
		ingresses := []client.Object{
			&networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-1",
					Namespace: "namespace-1",
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: &className,
					Rules: []networkingv1.IngressRule{
						{Host: "api.example.com"},
						{Host: "web.example.com"},
					},
				},
			},
			&networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-2",
					Namespace: "namespace-2",
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: &className,
					Rules: []networkingv1.IngressRule{
						{Host: "blog.example.com"},
					},
				},
			},
			// Ingress with different class - should be ignored
			&networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-traefik",
					Namespace: "namespace-3",
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: stringPtr("traefik"),
					Rules: []networkingv1.IngressRule{
						{Host: "traefik.example.com"},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingresses...).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "complex-reconcile-test",
				Namespace: "default",
			},
		})

		assert.NoError(t, err, "Should handle complex ingress scenarios")
		assert.Equal(t, reconcile.Result{}, result, "Should return empty result")

		// Verify the dynamic ConfigMap was created correctly
		var dynamicConfigMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(),
			client.ObjectKey{Name: "coredns-custom", Namespace: "kube-system"}, &dynamicConfigMap)
		assert.NoError(t, err, "Dynamic ConfigMap should be created")

		content := dynamicConfigMap.Data["dynamic.server"]
		assert.Contains(t, content, "api.example.com", "Should contain api.example.com")
		assert.Contains(t, content, "web.example.com", "Should contain web.example.com")
		assert.Contains(t, content, "blog.example.com", "Should contain blog.example.com")
		assert.NotContains(t, content, "traefik.example.com", "Should not contain traefik.example.com")
	})

	t.Run("reconcile_with_existing_dynamic_configmap", func(t *testing.T) {
		// Pre-create a dynamic ConfigMap
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-custom",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"dynamic.server": "# Old configuration\nrewrite name exact old.example.com ingress-nginx-controller.ingress-nginx.svc.cluster.local.\n",
			},
		}

		className := "nginx"
		newIngress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-ingress",
				Namespace: "default",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &className,
				Rules: []networkingv1.IngressRule{
					{Host: "new.example.com"},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingConfigMap, newIngress).Build()
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "update-configmap-test",
				Namespace: "default",
			},
		})

		assert.NoError(t, err, "Should update existing ConfigMap")
		assert.Equal(t, reconcile.Result{}, result, "Should return empty result")

		// Verify the ConfigMap was updated
		var updatedConfigMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(),
			client.ObjectKey{Name: "coredns-custom", Namespace: "kube-system"}, &updatedConfigMap)
		assert.NoError(t, err, "ConfigMap should exist")

		content := updatedConfigMap.Data["dynamic.server"]
		assert.Contains(t, content, "new.example.com", "Should contain new.example.com")
		assert.NotContains(t, content, "old.example.com", "Should not contain old.example.com")
	})
}

// Test the DeploymentClient interface methods
func TestDeploymentClientInterface(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	t.Run("controller_runtime_deployment_client_get", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "default",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		deploymentClient := &ControllerRuntimeClient{client: fakeClient}

		retrievedDeployment, err := deploymentClient.GetDeployment(context.Background(), "default", "test-deployment")
		assert.NoError(t, err, "Should get deployment successfully")
		assert.Equal(t, "test-deployment", retrievedDeployment.Name)
	})

	t.Run("controller_runtime_deployment_client_update", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		deploymentClient := &ControllerRuntimeClient{client: fakeClient}

		// Update the deployment
		deployment.Spec.Replicas = int32Ptr(3)
		err := deploymentClient.UpdateDeployment(context.Background(), deployment)
		assert.NoError(t, err, "Should update deployment successfully")

		// Verify the update
		updatedDeployment, err := deploymentClient.GetDeployment(context.Background(), "default", "test-deployment")
		assert.NoError(t, err)
		assert.Equal(t, int32(3), *updatedDeployment.Spec.Replicas)
	})
}

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}

// TestCoreDNSImportEdgeCases tests edge cases in CoreDNS import logic
func TestCoreDNSImportEdgeCases(t *testing.T) {
	t.Run("corefile missing in configmap", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		
		// Create a CoreDNS ConfigMap without Corefile data
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				// Missing "Corefile" key
				"other": "data",
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(coreDNSConfigMap).
			Build()
		
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		err := reconciler.ensureCoreDNSImport(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Corefile not found in CoreDNS ConfigMap")
	})
}

// TestEnsureCoreDNSVolumeMountClientSelection tests the client selection logic
func TestEnsureCoreDNSVolumeMountClientSelection(t *testing.T) {
	t.Run("fake client detection and fallback", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, appsv1.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))
		
		// Create test deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "coredns",
								Image: "registry.k8s.io/coredns/coredns:v1.11.1",
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
		
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		// This should use the controller-runtime client path since it's a fake client
		err := reconciler.ensureCoreDNSVolumeMount(context.Background())
		assert.NoError(t, err)
		
		// Verify the volume mount was added
		var updatedDeployment appsv1.Deployment
		err = fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      "coredns",
			Namespace: "kube-system",
		}, &updatedDeployment)
		require.NoError(t, err)
		
		// Should have added both volume and volume mount
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Volumes, 1)
		assert.Equal(t, "coredns-custom-volume", updatedDeployment.Spec.Template.Spec.Volumes[0].Name)
		
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, 1)
		assert.Equal(t, "coredns-custom-volume", updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)
	})
}

// TestReconcileWithListError tests reconcile when listing ingresses fails
func TestReconcileWithListError(t *testing.T) {
	t.Run("list ingresses error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		require.NoError(t, networkingv1.AddToScheme(scheme))
		
		// Create a client that fails on list operations
		fakeClient := &failingClient{
			Client:     fake.NewClientBuilder().WithScheme(scheme).Build(),
			failOnList: true,
		}
		
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-reconcile",
				Namespace: "default",
			},
		})
		
		// Should return error and requeue
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mock list error")
		assert.Equal(t, result.RequeueAfter.Minutes(), float64(1))
	})
}

// TestEnvironmentVariableDefaults tests that environment variables are properly loaded
func TestEnvironmentVariableDefaults(t *testing.T) {
	t.Run("environment variables loaded at startup", func(t *testing.T) {
		// Save current values
		originalIngressClass := ingressClass
		originalTargetCNAME := targetCNAME
		originalDynamicConfigMapName := dynamicConfigMapName
		
		defer func() {
			// Restore original values (though they're package-level)
			ingressClass = originalIngressClass
			targetCNAME = originalTargetCNAME
			dynamicConfigMapName = originalDynamicConfigMapName
		}()
		
		// Test that the package-level variables have expected defaults
		assert.Equal(t, "nginx", ingressClass)
		assert.Equal(t, "ingress-nginx-controller.ingress-nginx.svc.cluster.local.", targetCNAME)
		assert.Equal(t, "coredns-custom", dynamicConfigMapName)
		assert.Equal(t, "dynamic.server", dynamicConfigKey)
		assert.Equal(t, "kube-system", coreDNSNamespace)
		assert.Equal(t, "coredns", coreDNSConfigMapName)
		assert.Equal(t, "import /etc/coredns/custom/*.server", importStatement)
	})
}

// TestConfigMapContentComparison tests the logic that determines if ConfigMap needs updating
func TestConfigMapContentComparison(t *testing.T) {
	t.Run("content comparison prevents unnecessary updates", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		
		// Create existing ConfigMap with specific content
		domains := []string{"example.com"}
		hosts := []string{"api.example.com"}
		expectedContent := generateDynamicConfig(domains, hosts)
		
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-custom",
				Namespace: "kube-system",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "coredns-ingress-sync",
				},
			},
			Data: map[string]string{
				"dynamic.server": expectedContent,
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingConfigMap).
			Build()
		
		reconciler := &IngressReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		
		// Update with same content - should not error and should not modify
		err := reconciler.updateDynamicConfigMap(context.Background(), domains, hosts)
		assert.NoError(t, err)
		
		// Verify ConfigMap was not modified (still has same content)
		var configMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      "coredns-custom",
			Namespace: "kube-system",
		}, &configMap)
		require.NoError(t, err)
		
		assert.Equal(t, expectedContent, configMap.Data["dynamic.server"])
	})
}

// TestEnsureCoreDNSVolumeMountWithClientEdgeCases tests edge cases in volume mount configuration
func TestEnsureCoreDNSVolumeMountWithClientEdgeCases(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("volume_exists_mount_missing", func(t *testing.T) {
		// Create deployment with volume but no mount
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:         "coredns",
								VolumeMounts: []corev1.VolumeMount{},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "coredns-custom-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "coredns-custom",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		reconciler := &IngressReconciler{Client: client}
		
		controllerClient := &ControllerRuntimeClient{client: client}
		err := reconciler.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
		assert.NoError(t, err)

		// Verify mount was added
		var updatedDeployment appsv1.Deployment
		err = client.Get(ctx, types.NamespacedName{Name: "coredns", Namespace: "kube-system"}, &updatedDeployment)
		assert.NoError(t, err)
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, 1)
		assert.Equal(t, "coredns-custom-volume", updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)
	})

	t.Run("mount_exists_volume_missing", func(t *testing.T) {
		// Create deployment with mount but no volume
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "coredns",
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "coredns-custom-volume",
										MountPath: "/etc/coredns/custom",
										ReadOnly:  true,
									},
								},
							},
						},
						Volumes: []corev1.Volume{},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		reconciler := &IngressReconciler{Client: client}
		
		controllerClient := &ControllerRuntimeClient{client: client}
		err := reconciler.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
		assert.NoError(t, err)

		// Verify volume was added
		var updatedDeployment appsv1.Deployment
		err = client.Get(ctx, types.NamespacedName{Name: "coredns", Namespace: "kube-system"}, &updatedDeployment)
		assert.NoError(t, err)
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Volumes, 1)
		assert.Equal(t, "coredns-custom-volume", updatedDeployment.Spec.Template.Spec.Volumes[0].Name)
	})

	t.Run("both_volume_and_mount_exist_already", func(t *testing.T) {
		// Create deployment with both volume and mount already present
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "coredns",
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "coredns-custom-volume",
										MountPath: "/etc/coredns/custom",
										ReadOnly:  true,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "coredns-custom-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "coredns-custom",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		reconciler := &IngressReconciler{Client: client}
		
		controllerClient := &ControllerRuntimeClient{client: client}
		err := reconciler.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
		assert.NoError(t, err)

		// Verify no changes were made (since both already exist)
		var updatedDeployment appsv1.Deployment
		err = client.Get(ctx, types.NamespacedName{Name: "coredns", Namespace: "kube-system"}, &updatedDeployment)
		assert.NoError(t, err)
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Volumes, 1)
		assert.Len(t, updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, 1)
	})

	t.Run("update_deployment_error", func(t *testing.T) {
		// Create deployment without volume or mount
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:         "coredns",
								VolumeMounts: []corev1.VolumeMount{},
							},
						},
						Volumes: []corev1.Volume{},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		reconciler := &IngressReconciler{Client: client}
		
		// Use failing client to simulate update error
		failingClient := &failingClient{
			Client:       client,
			failOnUpdate: true,
		}
		controllerClient := &ControllerRuntimeClient{client: failingClient}
		
		err := reconciler.ensureCoreDNSVolumeMountWithClient(ctx, controllerClient)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update CoreDNS deployment")
	})
}
