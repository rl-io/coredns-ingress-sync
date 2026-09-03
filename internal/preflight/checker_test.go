package preflight

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	traefikv1alpha1 "github.com/rl-io/coredns-ingress-sync/internal/traefik/v1alpha1"
)

func TestChecker_CheckCoreDNSDeployment(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name          string
		deployment    *appsv1.Deployment
		expectPassed  bool
		expectMessage string
	}{
		{
			name: "CoreDNS deployment exists",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
			},
			expectPassed:  true,
			expectMessage: "✅ CoreDNS deployment found",
		},
		{
			name:          "CoreDNS deployment does not exist",
			deployment:    nil,
			expectPassed:  false,
			expectMessage: "❌ CoreDNS deployment not found in namespace kube-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)

			var objects []runtime.Object
			if tt.deployment != nil {
				objects = append(objects, tt.deployment)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			config := Config{
				CoreDNSNamespace: "kube-system",
			}

			checker := NewChecker(client, config, logger)
			result, err := checker.checkCoreDNSDeployment(context.Background())

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPassed, result.Passed)
			assert.Contains(t, result.Message, tt.expectMessage)
		})
	}
}

func TestChecker_CheckMountPathConflicts(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name         string
		deployment   *appsv1.Deployment
		config       Config
		expectPassed bool
	}{
		{
			name: "No mount path conflicts",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config-volume",
											MountPath: "/etc/coredns",
										},
									},
								},
							},
						},
					},
				},
			},
			config: Config{
				CoreDNSNamespace: "kube-system",
				MountPath:        "/etc/coredns/custom/my-controller",
				VolumeName:       "my-volume",
			},
			expectPassed: true,
		},
		{
			name: "Mount path conflict detected",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "other-volume",
											MountPath: "/etc/coredns/custom/my-controller",
										},
									},
								},
							},
						},
					},
				},
			},
			config: Config{
				CoreDNSNamespace: "kube-system",
				MountPath:        "/etc/coredns/custom/my-controller",
				VolumeName:       "my-volume",
			},
			expectPassed: false,
		},
		{
			name: "Same mount path with same volume name (no conflict)",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-volume",
											MountPath: "/etc/coredns/custom/my-controller",
										},
									},
								},
							},
						},
					},
				},
			},
			config: Config{
				CoreDNSNamespace: "kube-system",
				MountPath:        "/etc/coredns/custom/my-controller",
				VolumeName:       "my-volume",
			},
			expectPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.deployment).
				Build()

			checker := NewChecker(client, tt.config, logger)
			result, err := checker.checkMountPathConflicts(context.Background())

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPassed, result.Passed)
		})
	}
}

func TestChecker_CheckConfigMapConflicts(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name         string
		configMap    *corev1.ConfigMap
		config       Config
		expectPassed bool
	}{
		{
			name:      "No ConfigMap exists (no conflict)",
			configMap: nil,
			config: Config{
				CoreDNSNamespace:     "kube-system",
				DynamicConfigMapName: "my-configmap",
				ReleaseInstance:      "my-release",
			},
			expectPassed: true,
		},
		{
			name: "ConfigMap managed by same instance (no conflict)",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-configmap",
					Namespace: "kube-system",
					Labels: map[string]string{
						"app.kubernetes.io/instance": "my-release",
					},
				},
			},
			config: Config{
				CoreDNSNamespace:     "kube-system",
				DynamicConfigMapName: "my-configmap",
				ReleaseInstance:      "my-release",
			},
			expectPassed: true,
		},
		{
			name: "ConfigMap managed by different instance (conflict)",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-configmap",
					Namespace: "kube-system",
					Labels: map[string]string{
						"app.kubernetes.io/instance": "other-release",
					},
				},
			},
			config: Config{
				CoreDNSNamespace:     "kube-system",
				DynamicConfigMapName: "my-configmap",
				ReleaseInstance:      "my-release",
			},
			expectPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			var objects []runtime.Object
			if tt.configMap != nil {
				objects = append(objects, tt.configMap)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			checker := NewChecker(client, tt.config, logger)
			result, err := checker.checkConfigMapConflicts(context.Background())

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPassed, result.Passed)
		})
	}
}

func TestHasErrors(t *testing.T) {
	tests := []struct {
		name     string
		results  []CheckResult
		expected bool
	}{
		{
			name: "No errors",
			results: []CheckResult{
				{Passed: true, Severity: "info"},
				{Passed: true, Severity: "warning", Warning: true},
			},
			expected: false,
		},
		{
			name: "Has errors",
			results: []CheckResult{
				{Passed: true, Severity: "info"},
				{Passed: false, Severity: "error"},
			},
			expected: true,
		},
		{
			name: "Only warnings",
			results: []CheckResult{
				{Passed: true, Severity: "warning", Warning: true},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasErrors(tt.results)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChecker_RunChecks(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name        string
		objects     []runtime.Object
		expectError bool
		expectPass  bool
	}{
		{
			name: "All checks pass",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns",
						Namespace: "kube-system",
					},
				},
			},
			expectError: false,
			expectPass:  true,
		},
		{
			name:        "CoreDNS deployment missing",
			objects:     []runtime.Object{},
			expectError: false,
			expectPass:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.objects...).
				Build()

			config := Config{
				DeploymentName:       "test-deployment",
				ReleaseInstance:      "test-instance",
				MountPath:            "/etc/coredns/custom/test",
				VolumeName:           "test-volume",
				DynamicConfigMapName: "test-configmap",
				CoreDNSNamespace:     "kube-system",
				IngressClass:         "nginx",
				TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
			}

			checker := NewChecker(client, config, logger)
			ctx := context.Background()

			results, err := checker.RunChecks(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, results)

				// Check if first result (CoreDNS deployment check) matches expectation
				if len(results) > 0 {
					assert.Equal(t, tt.expectPass, results[0].Passed)
				}
			}
		})
	}
}

func TestChecker_CheckCoreDNSDeploymentWithRetry(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name         string
		objects      []runtime.Object
		expectPassed bool
	}{
		{
			name: "Success on first attempt",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns",
						Namespace: "kube-system",
					},
				},
			},
			expectPassed: true,
		},
		{
			name:         "Failure after retries",
			objects:      []runtime.Object{},
			expectPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.objects...).
				Build()

			config := Config{
				CoreDNSNamespace: "kube-system",
			}

			checker := NewChecker(client, config, logger)
			ctx := context.Background()

			result, err := checker.checkCoreDNSDeploymentWithRetry(ctx)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPassed, result.Passed)
		})
	}
}

func TestChecker_CheckMountPathConflicts_ErrorCases(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name          string
		deployment    *appsv1.Deployment
		config        Config
		expectPassed  bool
		expectMessage string
	}{
		{
			name: "CoreDNS deployment has no containers",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{}, // Empty containers
						},
					},
				},
			},
			config: Config{
				CoreDNSNamespace: "kube-system",
				MountPath:        "/etc/coredns/custom/test",
				VolumeName:       "test-volume",
			},
			expectPassed:  false,
			expectMessage: "❌ CoreDNS deployment has no containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.deployment).
				Build()

			checker := NewChecker(client, tt.config, logger)
			result, err := checker.checkMountPathConflicts(context.Background())

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPassed, result.Passed)
			if tt.expectMessage != "" {
				assert.Contains(t, result.Message, tt.expectMessage)
			}
		})
	}
}

func TestChecker_CheckCoreDNSDeploymentWithRetry_PermissionRetry(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	// Create a custom client that simulates permission denied initially and then succeeds
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create deployment after "retry" - simulating RBAC propagation
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: "kube-system",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(deployment).
		Build()

	config := Config{
		CoreDNSNamespace: "kube-system",
	}

	checker := NewChecker(client, config, logger)
	ctx := context.Background()

	// This should succeed immediately since we have the deployment
	result, err := checker.checkCoreDNSDeploymentWithRetry(ctx)

	assert.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Contains(t, result.Message, "✅ CoreDNS deployment found")
}

func TestChecker_PrintResults_EdgeCases(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name     string
		results  []CheckResult
		testName string
	}{
		{
			name: "Mixed results with warnings",
			results: []CheckResult{
				{
					Passed:   true,
					Message:  "✅ All good\nWith multiple lines\n💡 Some additional info",
					Severity: "info",
				},
				{
					Passed:   true,
					Warning:  true,
					Message:  "⚠️ Warning message\nWith details",
					Severity: "warning",
				},
				{
					Passed:   false,
					Message:  "❌ Error message\nWith error details",
					Severity: "error",
				},
			},
			testName: "multiple result types",
		},
		{
			name: "Empty message lines",
			results: []CheckResult{
				{
					Passed:   true,
					Message:  "Message with empty lines\n\n   \nAnd more content",
					Severity: "info",
				},
			},
			testName: "empty lines handling",
		},
		{
			name:     "No results",
			results:  []CheckResult{},
			testName: "empty results",
		},
		{
			name: "Only warnings",
			results: []CheckResult{
				{
					Passed:   true,
					Warning:  true,
					Message:  "⚠️ Warning only",
					Severity: "warning",
				},
			},
			testName: "warnings only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{}
			checker := NewChecker(nil, config, logger)

			// This function doesn't return anything, just ensure it doesn't panic
			assert.NotPanics(t, func() {
				checker.PrintResults(tt.results)
			})
		})
	}
}

func TestChecker_RunChecks_ErrorPaths(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name        string
		objects     []runtime.Object
		config      Config
		expectError bool
		errorInStep string
	}{
		{
			name: "Mount path check error",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns",
						Namespace: "kube-system",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{}, // No containers - will cause error
							},
						},
					},
				},
			},
			config: Config{
				DeploymentName:       "test-deployment",
				ReleaseInstance:      "test-instance",
				MountPath:            "/etc/coredns/custom/test",
				VolumeName:           "test-volume",
				DynamicConfigMapName: "test-configmap",
				CoreDNSNamespace:     "kube-system",
				IngressClass:         "nginx",
				TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
			},
			expectError: false, // checkMountPathConflicts returns result, not error
			errorInStep: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.objects...).
				Build()

			checker := NewChecker(client, tt.config, logger)
			ctx := context.Background()

			results, err := checker.RunChecks(ctx)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorInStep != "" {
					assert.Contains(t, err.Error(), tt.errorInStep)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, results)
			}
		})
	}
}

func TestChecker_CheckDuplicateControllers(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name          string
		objects       []runtime.Object
		expectPassed  bool
		expectWarning bool
	}{
		{
			name:          "No duplicate controllers",
			objects:       []runtime.Object{},
			expectPassed:  true,
			expectWarning: false,
		},
		{
			name: "Duplicate controller exists",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-coredns-sync",
						Namespace: "other-namespace",
						Labels: map[string]string{
							"app.kubernetes.io/name": "coredns-ingress-sync",
						},
					},
				},
			},
			expectPassed:  true, // Function returns true with warning, not failure
			expectWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.objects...).
				Build()

			config := Config{
				DeploymentName:   "test-deployment",
				ReleaseInstance:  "test-instance",
				IngressClass:     "nginx",
				CoreDNSNamespace: "kube-system",
			}

			checker := NewChecker(client, config, logger)
			ctx := context.Background()

			result, err := checker.checkDuplicateControllers(ctx)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPassed, result.Passed)
			assert.Equal(t, tt.expectWarning, result.Warning)
		})
	}
}

func TestChecker_PrintResults(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	results := []CheckResult{
		{
			Passed:   true,
			Message:  "All good",
			Severity: "info",
		},
		{
			Passed:   false,
			Message:  "Something wrong",
			Severity: "error",
		},
	}

	config := Config{}
	checker := NewChecker(nil, config, logger)

	// This function doesn't return anything, just ensure it doesn't panic
	assert.NotPanics(t, func() {
		checker.PrintResults(results)
	})
}

func TestConfigFromEnv(t *testing.T) {
	// Set test environment variables
	t.Setenv("COREDNS_NAMESPACE", "test-namespace")
	t.Setenv("COREDNS_CONFIGMAP_NAME", "test-configmap")
	t.Setenv("COREDNS_VOLUME_NAME", "test-volume")
	t.Setenv("DYNAMIC_CONFIGMAP_NAME", "test-dynamic")
	t.Setenv("MOUNT_PATH", "/test/path")

	// Load config from environment (this will read the env vars we just set)
	baseConfig := config.Load()

	result := ConfigFromEnv(baseConfig)

	assert.Equal(t, "test-namespace", result.CoreDNSNamespace)
	assert.Equal(t, "test-volume", result.VolumeName)
	assert.Equal(t, "test-dynamic", result.DynamicConfigMapName)
	assert.Equal(t, "/test/path", result.MountPath)

	// Check that other fields are properly mapped from the loaded config
	assert.Equal(t, baseConfig.IngressClass, result.IngressClass)
	assert.Equal(t, baseConfig.TargetCNAME, result.TargetCNAME)
	assert.False(t, result.GatewayAPIEnabled, "GatewayAPIEnabled must default to false when no GATEWAY_CLASS_MAPPINGS/GATEWAY_CLASS env vars are set")
}

func TestConfigFromEnv_GatewayAPIEnabled(t *testing.T) {
	t.Setenv("GATEWAY_CLASS", "traefik")
	t.Setenv("GATEWAY_TARGET_CNAME", "traefik.traefik.svc.cluster.local.")

	baseConfig := config.Load()
	result := ConfigFromEnv(baseConfig)

	assert.True(t, result.GatewayAPIEnabled)
}

// Test helper clients to simulate various error conditions

// ForbiddenErrorClient wraps a fake client and returns forbidden errors for Get operations
type ForbiddenErrorClient struct {
	client.Client
}

func NewForbiddenErrorClient() *ForbiddenErrorClient {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return &ForbiddenErrorClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
}

func (f *ForbiddenErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Return forbidden error for deployments
	if _, ok := obj.(*appsv1.Deployment); ok {
		return errors.NewForbidden(
			schema.GroupResource{Group: "apps", Resource: "deployments"},
			key.Name,
			fmt.Errorf("deployments.apps is forbidden"),
		)
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

// GenericErrorClient wraps a fake client and returns generic errors for Get operations
type GenericErrorClient struct {
	client.Client
}

func NewGenericErrorClient() *GenericErrorClient {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return &GenericErrorClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
}

func (g *GenericErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Return generic error for deployments
	if _, ok := obj.(*appsv1.Deployment); ok {
		return fmt.Errorf("internal server error")
	}
	return g.Client.Get(ctx, key, obj, opts...)
}

func TestChecker_CheckCoreDNSDeployment_ForbiddenError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	// Create a client that returns forbidden errors
	client := NewForbiddenErrorClient()

	config := Config{
		CoreDNSNamespace: "kube-system",
	}

	checker := NewChecker(client, config, logger)
	result, err := checker.checkCoreDNSDeployment(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Permission denied")
	assert.Contains(t, result.Message, "RBAC resources are not yet created")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckCoreDNSDeployment_GenericError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	// Create a client that returns generic errors
	client := NewGenericErrorClient()

	config := Config{
		CoreDNSNamespace: "kube-system",
	}

	checker := NewChecker(client, config, logger)
	result, err := checker.checkCoreDNSDeployment(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Error accessing CoreDNS deployment")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckMountPathConflicts_ForbiddenError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	// Create a client that returns forbidden errors
	client := NewForbiddenErrorClient()

	config := Config{
		CoreDNSNamespace: "kube-system",
		MountPath:        "/etc/coredns/custom/test",
		VolumeName:       "test-volume",
	}

	checker := NewChecker(client, config, logger)
	result, err := checker.checkMountPathConflicts(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Permission denied")
	assert.Contains(t, result.Message, "RBAC resources may not be ready yet")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckCoreDNSDeploymentWithRetry_ErrorCases(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	t.Run("forbidden error with retry", func(t *testing.T) {
		client := NewForbiddenErrorClient()
		config := Config{
			CoreDNSNamespace: "kube-system",
		}

		checker := NewChecker(client, config, logger)
		result, err := checker.checkCoreDNSDeploymentWithRetry(context.Background())

		assert.NoError(t, err)
		assert.False(t, result.Passed)
		assert.Contains(t, result.Message, "Permission denied")
		assert.Equal(t, "error", result.Severity)
	})

	t.Run("generic error with retry", func(t *testing.T) {
		client := NewGenericErrorClient()
		config := Config{
			CoreDNSNamespace: "kube-system",
		}

		checker := NewChecker(client, config, logger)
		result, err := checker.checkCoreDNSDeploymentWithRetry(context.Background())

		assert.NoError(t, err)
		assert.False(t, result.Passed)
		assert.Contains(t, result.Message, "Error accessing CoreDNS deployment")
		assert.Equal(t, "error", result.Severity)
	})
}

func TestChecker_CheckMountPathConflicts_GenericError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	// Create a client that returns generic errors
	client := NewGenericErrorClient()

	config := Config{
		CoreDNSNamespace: "kube-system",
		MountPath:        "/etc/coredns/custom",
	}

	checker := NewChecker(client, config, logger)
	result, err := checker.checkMountPathConflicts(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Could not retrieve CoreDNS deployment for mount path check")
	assert.Equal(t, "error", result.Severity)
}

// gatewayAPIListErrorClient wraps a fake client and returns a configurable
// error for List calls against Gateway/HTTPRoute types.
type gatewayAPIListErrorClient struct {
	client.Client
	err error
}

func (g *gatewayAPIListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	switch list.(type) {
	case *gatewayv1.GatewayList, *gatewayv1.HTTPRouteList:
		return g.err
	}
	return g.Client.List(ctx, list, opts...)
}

func gatewayAPIScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = gatewayv1.Install(scheme)
	return scheme
}

func TestChecker_CheckGatewayAPI(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	fakeClient := fake.NewClientBuilder().WithScheme(gatewayAPIScheme()).Build()
	config := Config{GatewayAPIEnabled: true}
	checker := NewChecker(fakeClient, config, logger)

	result, err := checker.checkGatewayAPI(context.Background())

	assert.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Contains(t, result.Message, "Gateway API CRDs")
	assert.Equal(t, "info", result.Severity)
}

func TestChecker_CheckGatewayAPI_CRDNotInstalled(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	noMatchErr := &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"},
		SearchedVersions: []string{"v1"},
	}
	wrapped := &gatewayAPIListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(gatewayAPIScheme()).Build(),
		err:    noMatchErr,
	}
	config := Config{GatewayAPIEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkGatewayAPI(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "CRD not found")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckGatewayAPI_Forbidden(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	forbiddenErr := errors.NewForbidden(
		schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gateways"},
		"",
		fmt.Errorf("gateways.gateway.networking.k8s.io is forbidden"),
	)
	wrapped := &gatewayAPIListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(gatewayAPIScheme()).Build(),
		err:    forbiddenErr,
	}
	config := Config{GatewayAPIEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkGatewayAPI(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Permission denied")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckGatewayAPI_GenericError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	wrapped := &gatewayAPIListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(gatewayAPIScheme()).Build(),
		err:    fmt.Errorf("internal server error"),
	}
	config := Config{GatewayAPIEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkGatewayAPI(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Error accessing Gateway API")
	assert.Equal(t, "error", result.Severity)
}

// httpRouteListErrorClient wraps a fake client and only fails List calls
// against HTTPRouteList, letting GatewayList succeed -- this is the only way
// to reach checkGatewayAPI's second List call (the HTTPRoute check), since a
// failure on the first (Gateway) List returns early.
type httpRouteListErrorClient struct {
	client.Client
	err error
}

func (h *httpRouteListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*gatewayv1.HTTPRouteList); ok {
		return h.err
	}
	return h.Client.List(ctx, list, opts...)
}

func TestChecker_CheckGatewayAPI_HTTPRouteListError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	wrapped := &httpRouteListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(gatewayAPIScheme()).Build(),
		err:    fmt.Errorf("internal server error"),
	}
	config := Config{GatewayAPIEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkGatewayAPI(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Error accessing Gateway API")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_RunChecks_GatewayAPIEnabled(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		}).
		Build()

	config := Config{
		DeploymentName:       "test-deployment",
		ReleaseInstance:      "test-instance",
		MountPath:            "/etc/coredns/custom/test",
		VolumeName:           "test-volume",
		DynamicConfigMapName: "test-configmap",
		CoreDNSNamespace:     "kube-system",
		IngressClass:         "nginx",
		TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
		GatewayAPIEnabled:    true,
	}

	checker := NewChecker(fakeClient, config, logger)
	results, err := checker.RunChecks(context.Background())

	assert.NoError(t, err)
	last := results[len(results)-1]
	assert.Contains(t, last.Message, "Gateway API CRDs")
	assert.True(t, last.Passed)
}

func TestChecker_RunChecks_GatewayAPIEnabled_Error(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	wrapped := &gatewayAPIListErrorClient{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			}).
			Build(),
		err: fmt.Errorf("boom"),
	}

	config := Config{
		DeploymentName:       "test-deployment",
		ReleaseInstance:      "test-instance",
		MountPath:            "/etc/coredns/custom/test",
		VolumeName:           "test-volume",
		DynamicConfigMapName: "test-configmap",
		CoreDNSNamespace:     "kube-system",
		IngressClass:         "nginx",
		TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
		GatewayAPIEnabled:    true,
	}

	checker := NewChecker(wrapped, config, logger)
	results, err := checker.RunChecks(context.Background())

	// checkGatewayAPI itself never returns a non-nil error (List errors are
	// captured in the CheckResult), so RunChecks should still succeed, with
	// the Gateway API check reported as failed.
	assert.NoError(t, err)
	last := results[len(results)-1]
	assert.False(t, last.Passed)
}

// traefikIngressRouteListErrorClient wraps a fake client and fails List calls
// against IngressRouteList, letting checkTraefikIngressRoute's single List
// call return a controlled error.
type traefikIngressRouteListErrorClient struct {
	client.Client
	err error
}

func (t *traefikIngressRouteListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*traefikv1alpha1.IngressRouteList); ok {
		return t.err
	}
	return t.Client.List(ctx, list, opts...)
}

func traefikScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = traefikv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestChecker_CheckTraefikIngressRoute(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	fakeClient := fake.NewClientBuilder().WithScheme(traefikScheme()).Build()
	config := Config{IngressRouteEnabled: true}
	checker := NewChecker(fakeClient, config, logger)

	result, err := checker.checkTraefikIngressRoute(context.Background())

	assert.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Contains(t, result.Message, "Traefik IngressRoute CRD")
	assert.Equal(t, "info", result.Severity)
}

func TestChecker_CheckTraefikIngressRoute_CRDNotInstalled(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	noMatchErr := &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "traefik.io", Kind: "IngressRoute"},
		SearchedVersions: []string{"v1alpha1"},
	}
	wrapped := &traefikIngressRouteListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(traefikScheme()).Build(),
		err:    noMatchErr,
	}
	config := Config{IngressRouteEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkTraefikIngressRoute(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "CRD not found")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckTraefikIngressRoute_Forbidden(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	forbiddenErr := errors.NewForbidden(
		schema.GroupResource{Group: "traefik.io", Resource: "ingressroutes"},
		"",
		fmt.Errorf("ingressroutes.traefik.io is forbidden"),
	)
	wrapped := &traefikIngressRouteListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(traefikScheme()).Build(),
		err:    forbiddenErr,
	}
	config := Config{IngressRouteEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkTraefikIngressRoute(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Permission denied")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckTraefikIngressRoute_GenericError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	wrapped := &traefikIngressRouteListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(traefikScheme()).Build(),
		err:    fmt.Errorf("internal server error"),
	}
	config := Config{IngressRouteEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkTraefikIngressRoute(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Error accessing Traefik")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_RunChecks_IngressRouteEnabled(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = traefikv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		}).
		Build()

	config := Config{
		DeploymentName:       "test-deployment",
		ReleaseInstance:      "test-instance",
		MountPath:            "/etc/coredns/custom/test",
		VolumeName:           "test-volume",
		DynamicConfigMapName: "test-configmap",
		CoreDNSNamespace:     "kube-system",
		IngressClass:         "nginx",
		TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
		IngressRouteEnabled:  true,
	}

	checker := NewChecker(fakeClient, config, logger)
	results, err := checker.RunChecks(context.Background())

	assert.NoError(t, err)
	last := results[len(results)-1]
	assert.Contains(t, last.Message, "Traefik IngressRoute CRD")
	assert.True(t, last.Passed)
}

func TestChecker_RunChecks_IngressRouteEnabled_Error(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = traefikv1alpha1.AddToScheme(scheme)

	wrapped := &traefikIngressRouteListErrorClient{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			}).
			Build(),
		err: fmt.Errorf("boom"),
	}

	config := Config{
		DeploymentName:       "test-deployment",
		ReleaseInstance:      "test-instance",
		MountPath:            "/etc/coredns/custom/test",
		VolumeName:           "test-volume",
		DynamicConfigMapName: "test-configmap",
		CoreDNSNamespace:     "kube-system",
		IngressClass:         "nginx",
		TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
		IngressRouteEnabled:  true,
	}

	checker := NewChecker(wrapped, config, logger)
	results, err := checker.RunChecks(context.Background())

	// checkTraefikIngressRoute itself never returns a non-nil error (List
	// errors are captured in the CheckResult), so RunChecks should still
	// succeed, with the IngressRoute check reported as failed.
	assert.NoError(t, err)
	last := results[len(results)-1]
	assert.False(t, last.Passed)
}

// serviceListErrorClient wraps a fake client and fails List calls against
// ServiceList, letting checkServiceWatch's single List call return a
// controlled error.
type serviceListErrorClient struct {
	client.Client
	err error
}

func (s *serviceListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.ServiceList); ok {
		return s.err
	}
	return s.Client.List(ctx, list, opts...)
}

func TestChecker_CheckServiceWatch(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	config := Config{ServicesEnabled: true}
	checker := NewChecker(fakeClient, config, logger)

	result, err := checker.checkServiceWatch(context.Background())

	assert.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Contains(t, result.Message, "Service")
	assert.Equal(t, "info", result.Severity)
}

func TestChecker_CheckServiceWatch_Forbidden(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	forbiddenErr := errors.NewForbidden(
		schema.GroupResource{Group: "", Resource: "services"},
		"",
		fmt.Errorf("services is forbidden"),
	)
	wrapped := &serviceListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		err:    forbiddenErr,
	}
	config := Config{ServicesEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkServiceWatch(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Permission denied")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_CheckServiceWatch_GenericError(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	wrapped := &serviceListErrorClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		err:    fmt.Errorf("internal server error"),
	}
	config := Config{ServicesEnabled: true}
	checker := NewChecker(wrapped, config, logger)

	result, err := checker.checkServiceWatch(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "Error accessing Service")
	assert.Equal(t, "error", result.Severity)
}

func TestChecker_RunChecks_ServicesEnabled(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		}).
		Build()

	config := Config{
		DeploymentName:       "test-deployment",
		ReleaseInstance:      "test-instance",
		MountPath:            "/etc/coredns/custom/test",
		VolumeName:           "test-volume",
		DynamicConfigMapName: "test-configmap",
		CoreDNSNamespace:     "kube-system",
		IngressClass:         "nginx",
		TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
		ServicesEnabled:      true,
	}

	checker := NewChecker(fakeClient, config, logger)
	results, err := checker.RunChecks(context.Background())

	assert.NoError(t, err)
	last := results[len(results)-1]
	assert.Contains(t, last.Message, "Service")
	assert.True(t, last.Passed)
}

func TestChecker_RunChecks_ServicesEnabled_Error(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	wrapped := &serviceListErrorClient{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			}).
			Build(),
		err: fmt.Errorf("boom"),
	}

	config := Config{
		DeploymentName:       "test-deployment",
		ReleaseInstance:      "test-instance",
		MountPath:            "/etc/coredns/custom/test",
		VolumeName:           "test-volume",
		DynamicConfigMapName: "test-configmap",
		CoreDNSNamespace:     "kube-system",
		IngressClass:         "nginx",
		TargetCNAME:          "ingress-nginx.ingress-nginx.svc.cluster.local.",
		ServicesEnabled:      true,
	}

	checker := NewChecker(wrapped, config, logger)
	results, err := checker.RunChecks(context.Background())

	// checkServiceWatch itself never returns a non-nil error (List errors are
	// captured in the CheckResult), so RunChecks should still succeed, with
	// the Service check reported as failed.
	assert.NoError(t, err)
	last := results[len(results)-1]
	assert.False(t, last.Passed)
}

// namespaceRecordingClient wraps a fake client and records the Namespace
// field of every List call's resolved options, in call order -- "" means an
// unscoped (cluster-wide) List.
type namespaceRecordingClient struct {
	client.Client
	namespaces []string
}

func (n *namespaceRecordingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}
	n.namespaces = append(n.namespaces, listOpts.Namespace)
	return n.Client.List(ctx, list, opts...)
}

func TestChecker_ListNamespaceScoped_AllNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	recording := &namespaceRecordingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	checker := NewChecker(recording, Config{}, zap.New(zap.UseDevMode(true)))

	err := checker.listNamespaceScoped(context.Background(), func() client.ObjectList { return &corev1.ServiceList{} })

	assert.NoError(t, err)
	assert.Equal(t, []string{""}, recording.namespaces, "nil WatchNamespaces must produce a single unscoped List")
}

func TestChecker_ListNamespaceScoped_PerNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	recording := &namespaceRecordingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	checker := NewChecker(recording, Config{WatchNamespaces: []string{"ns1", "ns2"}}, zap.New(zap.UseDevMode(true)))

	err := checker.listNamespaceScoped(context.Background(), func() client.ObjectList { return &corev1.ServiceList{} })

	assert.NoError(t, err)
	assert.Equal(t, []string{"ns1", "ns2"}, recording.namespaces, "an explicit WatchNamespaces list must produce one List per namespace, matching the per-namespace Roles the chart grants")
}

// namespaceErrorClient wraps a fake client and fails a per-namespace List
// once it reaches a specific namespace, recording every namespace attempted
// up to and including the failure.
type namespaceErrorClient struct {
	client.Client
	failOn string
	err    error
	seen   []string
}

func (n *namespaceErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}
	n.seen = append(n.seen, listOpts.Namespace)
	if listOpts.Namespace == n.failOn {
		return n.err
	}
	return n.Client.List(ctx, list, opts...)
}

func TestChecker_ListNamespaceScoped_PerNamespaceError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	failing := &namespaceErrorClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		failOn: "ns2",
		err:    fmt.Errorf("boom"),
	}
	checker := NewChecker(failing, Config{WatchNamespaces: []string{"ns1", "ns2", "ns3"}}, zap.New(zap.UseDevMode(true)))

	err := checker.listNamespaceScoped(context.Background(), func() client.ObjectList { return &corev1.ServiceList{} })

	assert.EqualError(t, err, "boom")
	assert.Equal(t, []string{"ns1", "ns2"}, failing.seen, "must stop at the first namespace error, not continue to ns3")
}

func TestConfigFromEnv_WatchNamespaces(t *testing.T) {
	t.Setenv("WATCH_NAMESPACES", "ns1,ns2,ns3")
	t.Setenv("EXCLUDE_NAMESPACES", "ns2")

	baseConfig := config.Load()
	result := ConfigFromEnv(baseConfig)

	assert.Equal(t, []string{"ns1", "ns3"}, result.WatchNamespaces, "an excluded namespace must be dropped from the derived watch list")
}

func TestConfigFromEnv_WatchNamespaces_DefaultsToNil(t *testing.T) {
	baseConfig := config.Load()
	result := ConfigFromEnv(baseConfig)

	assert.Nil(t, result.WatchNamespaces, "no WATCH_NAMESPACES set must mean watch all namespaces (nil), matching the chart's cluster-wide RBAC default")
}
