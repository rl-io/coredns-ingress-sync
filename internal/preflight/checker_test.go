package preflight

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
)

func TestChecker_CheckCoreDNSDeployment(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name         string
		deployment   *appsv1.Deployment
		expectPassed bool
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
			expectPassed: true,
			expectMessage: "✅ CoreDNS deployment found",
		},
		{
			name:         "CoreDNS deployment does not exist",
			deployment:   nil,
			expectPassed: false,
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
		name           string
		deployment     *appsv1.Deployment
		config         Config
		expectPassed   bool
		expectMessage  string
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

func TestChecker_CheckDuplicateControllers(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name           string
		objects        []runtime.Object
		expectPassed   bool
		expectWarning  bool
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
			expectPassed:  true,  // Function returns true with warning, not failure
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
}
