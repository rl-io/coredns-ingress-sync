package coredns

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/go-logr/logr"
	
	"github.com/rl-io/coredns-ingress-sync/internal/metrics"
)

// Config holds CoreDNS configuration
type Config struct {
	Namespace           string
	ConfigMapName       string
	DynamicConfigMapName string
	DynamicConfigKey    string
	ImportStatement     string
	TargetCNAME         string
	VolumeName          string
}

// Manager handles CoreDNS configuration management
type Manager struct {
	client client.Client
	config Config
	logger logr.Logger
}

// DeploymentClient interface for Kubernetes deployment operations
type DeploymentClient interface {
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error
}

// DirectKubernetesClient wraps the Kubernetes clientset
type DirectKubernetesClient struct {
	clientset kubernetes.Interface
}

// ControllerRuntimeClient wraps the controller-runtime client for testing
type ControllerRuntimeClient struct {
	client client.Client
}

// NewManager creates a new CoreDNS manager
func NewManager(client client.Client, config Config) *Manager {
	return &Manager{
		client: client,
		config: config,
		logger: ctrl.Log.WithName("coredns-manager"),
	}
}

// UpdateDynamicConfigMap creates or updates the dynamic configuration ConfigMap
func (m *Manager) UpdateDynamicConfigMap(ctx context.Context, domains []string, hosts []string) error {
	startTime := time.Now()
	configMapName := types.NamespacedName{
		Name:      m.config.DynamicConfigMapName,
		Namespace: m.config.Namespace,
	}

	// Generate dynamic configuration
	dynamicConfig := m.generateDynamicConfig(domains, hosts)

	// Retry logic to handle concurrent updates
	for attempt := 0; attempt < 3; attempt++ {
		// Get or create the dynamic ConfigMap (fresh read each attempt)
		configMap := &corev1.ConfigMap{}
		err := m.client.Get(ctx, configMapName, configMap)

		if err != nil {
			// Create new ConfigMap if it doesn't exist
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      m.config.DynamicConfigMapName,
					Namespace: m.config.Namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "coredns-ingress-sync",
					},
				},
				Data: make(map[string]string),
			}

			// Set the content and try to create
			configMap.Data[m.config.DynamicConfigKey] = dynamicConfig

			if err := m.client.Create(ctx, configMap); err != nil {
				if attempt == 2 {
					duration := time.Since(startTime).Seconds()
					metrics.RecordCoreDNSConfigUpdate(duration, false)
					return fmt.Errorf("failed to create dynamic ConfigMap after retries: %w", err)
				}
				continue // Retry
			}
			duration := time.Since(startTime).Seconds()
			metrics.RecordCoreDNSConfigUpdate(duration, true)
			m.logger.Info("Created dynamic ConfigMap", 
				"configmap", m.config.DynamicConfigMapName, 
				"domains", len(domains))
			return nil
		}

		// Check if content has actually changed to avoid unnecessary updates
		if existingConfig, exists := configMap.Data[m.config.DynamicConfigKey]; exists && existingConfig == dynamicConfig {
			m.logger.V(1).Info("Dynamic ConfigMap is already up to date", 
				"configmap", m.config.DynamicConfigMapName)
			duration := time.Since(startTime).Seconds()
			metrics.RecordCoreDNSConfigUpdate(duration, true)
			return nil
		}

		// Update ConfigMap with fresh data
		configMap.Data[m.config.DynamicConfigKey] = dynamicConfig

		// Ensure labels are set for identification
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		configMap.Labels["app.kubernetes.io/managed-by"] = "coredns-ingress-sync"

		// Try to update
		if err := m.client.Update(ctx, configMap); err != nil {
			if attempt == 2 {
				duration := time.Since(startTime).Seconds()
				metrics.RecordCoreDNSConfigUpdate(duration, false)
				return fmt.Errorf("failed to update dynamic ConfigMap after retries: %w", err)
			}
			// Brief delay before retry to reduce contention
			time.Sleep(time.Millisecond * 100)
			continue // Retry with fresh read
		}

		duration := time.Since(startTime).Seconds()
		metrics.RecordCoreDNSConfigUpdate(duration, true)
		m.logger.Info("Updated dynamic ConfigMap", 
			"configmap", m.config.DynamicConfigMapName, 
			"domains", len(domains))
		return nil
	}

	duration := time.Since(startTime).Seconds()
	metrics.RecordCoreDNSConfigUpdate(duration, false)
	return fmt.Errorf("exhausted retries updating dynamic ConfigMap")
}

// generateDynamicConfig creates the CoreDNS configuration content
func (m *Manager) generateDynamicConfig(domains []string, hosts []string) string {
	var config strings.Builder

	// Header
	config.WriteString("# Auto-generated by coredns-ingress-sync controller\n")
	config.WriteString(fmt.Sprintf("# Last updated: %s\n", time.Now().Format(time.RFC3339)))
	config.WriteString("\n")

	// Generate individual rewrite rules for each discovered host
	for _, host := range hosts {
		config.WriteString(fmt.Sprintf("rewrite name exact %s %s\n", host, m.config.TargetCNAME))
	}

	return config.String()
}

// EnsureConfiguration ensures CoreDNS is properly configured
func (m *Manager) EnsureConfiguration(ctx context.Context) error {
	// Check if we should manage CoreDNS configuration
	if os.Getenv("COREDNS_AUTO_CONFIGURE") == "false" {
		m.logger.Info("CoreDNS auto-configuration disabled")
		return nil
	}

	// First, ensure the import statement is in the CoreDNS Corefile
	if err := m.ensureImport(ctx); err != nil {
		// Log the error but don't fail the reconciliation if CoreDNS is not available
		m.logger.Error(err, "Failed to ensure CoreDNS import statement")
		return nil
	}

	// Then, ensure the CoreDNS deployment has the volume mount
	if err := m.ensureVolumeMount(ctx); err != nil {
		// Log the error but don't fail the reconciliation if CoreDNS is not available
		m.logger.Error(err, "Failed to ensure CoreDNS volume mount")
		return nil
	}

	return nil
}

// ensureImport ensures the import statement is in the CoreDNS Corefile
func (m *Manager) ensureImport(ctx context.Context) error {
	// Get the CoreDNS ConfigMap
	coreDNSConfigMap := &corev1.ConfigMap{}
	coreDNSConfigMapName := types.NamespacedName{
		Name:      m.config.ConfigMapName,
		Namespace: m.config.Namespace,
	}

	if err := m.client.Get(ctx, coreDNSConfigMapName, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to get CoreDNS ConfigMap: %w", err)
	}

	// Check if Corefile exists
	corefile, exists := coreDNSConfigMap.Data["Corefile"]
	if !exists {
		return fmt.Errorf("corefile not found in CoreDNS ConfigMap")
	}

	// Check if import statement already exists
	if strings.Contains(corefile, m.config.ImportStatement) {
		m.logger.V(1).Info("Import statement already exists in CoreDNS Corefile")
		return nil
	}

	// Record configuration drift detection
	metrics.RecordCoreDNSConfigDrift("import_statement")
	m.logger.Info("Detected missing import statement, adding it back (defensive configuration)")

	// Add import statement after the .:53 { line
	lines := strings.Split(corefile, "\n")
	var newLines []string
	importAdded := false

	for _, line := range lines {
		newLines = append(newLines, line)
		// Add import statement after the main server block starts
		if strings.TrimSpace(line) == ".:53 {" && !importAdded {
			newLines = append(newLines, "    "+m.config.ImportStatement)
			importAdded = true
		}
	}

	if !importAdded {
		m.logger.Info("Could not find standard Corefile format, appending import statement")
		newLines = append(newLines, m.config.ImportStatement)
	}

	// Update the ConfigMap
	newCorefile := strings.Join(newLines, "\n")
	coreDNSConfigMap.Data["Corefile"] = newCorefile

	if err := m.client.Update(ctx, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to update CoreDNS ConfigMap: %w", err)
	}

	m.logger.Info("Added import statement to CoreDNS Corefile")
	return nil
}

// ensureVolumeMount ensures the CoreDNS deployment has the proper volume mount
func (m *Manager) ensureVolumeMount(ctx context.Context) error {
	// Try to create a direct Kubernetes client for deployment operations
	// If the client is a fake client (in tests), we'll use it directly
	if m.isFakeClient() {
		controllerClient := &ControllerRuntimeClient{client: m.client}
		return m.ensureVolumeMountWithClient(ctx, controllerClient)
	}

	config, err := ctrl.GetConfig()
	if err != nil {
		// In test environment, we'll simulate the deployment update using the controller-runtime client
		controllerClient := &ControllerRuntimeClient{client: m.client}
		return m.ensureVolumeMountWithClient(ctx, controllerClient)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		// In test environment, we'll simulate the deployment update using the controller-runtime client
		controllerClient := &ControllerRuntimeClient{client: m.client}
		return m.ensureVolumeMountWithClient(ctx, controllerClient)
	}

	// Create a wrapper that implements the same interface as controller-runtime client
	directClient := &DirectKubernetesClient{clientset: clientset}
	return m.ensureVolumeMountWithClient(ctx, directClient)
}

// ensureVolumeMountWithClient ensures volume mount using a deployment client
func (m *Manager) ensureVolumeMountWithClient(ctx context.Context, deploymentClient DeploymentClient) error {
	m.logger.V(1).Info("Starting volume mount configuration for CoreDNS")
	
	// Retry logic to handle resource version conflicts
	for attempt := 0; attempt < 3; attempt++ {
		m.logger.V(1).Info("Getting CoreDNS deployment", 
			"attempt", attempt+1, 
			"namespace", m.config.Namespace)
		deployment, err := deploymentClient.GetDeployment(ctx, m.config.Namespace, "coredns")
		if err != nil {
			m.logger.Error(err, "Failed to get CoreDNS deployment")
			return fmt.Errorf("failed to get CoreDNS deployment: %w", err)
		}

		m.logger.V(1).Info("Retrieved deployment, checking volumes and volume mounts")
		modified := false

		// Check if volume and volume mount already exist
		hasVolume := false
		hasVolumeMount := false
		volumeName := m.config.VolumeName

		// Check for existing volume
		m.logger.V(1).Info("Checking for existing volumes", "volume_count", len(deployment.Spec.Template.Spec.Volumes))
		for _, volume := range deployment.Spec.Template.Spec.Volumes {
			if volume.Name == volumeName {
				hasVolume = true
				m.logger.V(1).Info("Found existing volume", "name", volumeName)
				break
			}
		}

		// Check for existing volume mount
		if len(deployment.Spec.Template.Spec.Containers) > 0 {
			m.logger.V(1).Info("Checking volume mounts", "mount_count", len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
			for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
				if mount.Name == volumeName {
					hasVolumeMount = true
					m.logger.V(1).Info("Found existing volume mount", "name", volumeName)
					break
				}
			}
		}

		// If both exist, nothing to do
		if hasVolume && hasVolumeMount {
			m.logger.V(1).Info("CoreDNS deployment already has custom config volume mount")
			return nil
		}

		// Record configuration drift if volume or mount is missing
		if !hasVolume || !hasVolumeMount {
			metrics.RecordCoreDNSConfigDrift("volume_mount")
			m.logger.Info("Detected missing volume or volume mount, adding it back (defensive configuration)",
				"has_volume", hasVolume, "has_volume_mount", hasVolumeMount)
		}

		// Add volume if missing
		if !hasVolume {
			newVolume := corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: m.config.DynamicConfigMapName,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  m.config.DynamicConfigKey,
								Path: "dynamic.server",
							},
						},
					},
				},
			}
			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, newVolume)
			modified = true
			m.logger.Info("Added volume to CoreDNS deployment", "volume", volumeName)
		}

		// Add volume mount if missing
		if !hasVolumeMount && len(deployment.Spec.Template.Spec.Containers) > 0 {
			newVolumeMount := corev1.VolumeMount{
				Name:      volumeName,
				MountPath: "/etc/coredns/custom",
				ReadOnly:  true,
			}
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
				newVolumeMount,
			)
			modified = true
			m.logger.Info("Added volume mount to CoreDNS container", "volume", volumeName)
		}

		if !modified {
			m.logger.V(1).Info("No modifications needed for CoreDNS deployment")
			return nil
		}

		// Try to update the deployment
		if err := deploymentClient.UpdateDeployment(ctx, deployment); err != nil {
			if attempt == 2 {
				return fmt.Errorf("failed to update CoreDNS deployment after retries: %w", err)
			}
			m.logger.Error(err, "Failed to update CoreDNS deployment, retrying", "attempt", attempt+1)
			time.Sleep(time.Millisecond * 100) // Brief delay before retry
			continue
		}

		m.logger.Info("Updated CoreDNS deployment with custom config volume mount")
		return nil
	}

	return fmt.Errorf("exhausted retries updating CoreDNS deployment")
}

// Implementation of DeploymentClient interface

// GetDeployment gets a deployment using direct Kubernetes clientset
func (d *DirectKubernetesClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return d.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

// UpdateDeployment updates a deployment using direct Kubernetes clientset
func (d *DirectKubernetesClient) UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	_, err := d.clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// GetDeployment gets a deployment using controller-runtime client
func (c *ControllerRuntimeClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	var deployment appsv1.Deployment
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &deployment)
	return &deployment, err
}

// UpdateDeployment updates a deployment using controller-runtime client
func (c *ControllerRuntimeClient) UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	return c.client.Update(ctx, deployment)
}

// isFakeClient detects if we're using a fake client (in tests)
func (m *Manager) isFakeClient() bool {
	// Check if the client is a fake client by testing with a type assertion
	// This is a common pattern in controller-runtime tests
	clientTypeName := fmt.Sprintf("%T", m.client)
	return strings.Contains(clientTypeName, "fake")
}
