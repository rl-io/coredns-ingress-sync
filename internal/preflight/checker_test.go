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
)

func TestChecker_CheckCoreDNSDeployment(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name         string
		deployment   *appsv1.Deployment
		expectPassed bool
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
		},
		{
			name:         "CoreDNS deployment does not exist",
			deployment:   nil,
			expectPassed: false,
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
