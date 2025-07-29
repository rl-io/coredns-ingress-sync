package cleanup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
)

// Manager handles cleanup operations for the controller
type Manager struct {
	client client.Client
	logger logr.Logger
}

// NewManager creates a new cleanup manager
func NewManager(logger logr.Logger) (*Manager, error) {
	// Create a simple client for cleanup operations
	clientConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core/v1 to scheme: %w", err)
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add networking/v1 to scheme: %w", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add apps/v1 to scheme: %w", err)
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &Manager{
		client: k8sClient,
		logger: logger,
	}, nil
}

// Run performs all cleanup operations
func (m *Manager) Run(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create CoreDNS manager for cleanup operations
	coreDNSConfig := coredns.Config{
		Namespace:            cfg.CoreDNSNamespace,
		ConfigMapName:        cfg.CoreDNSConfigMapName,
		DynamicConfigMapName: cfg.DynamicConfigMapName,
		DynamicConfigKey:     cfg.DynamicConfigKey,
		ImportStatement:      cfg.ImportStatement,
		TargetCNAME:          cfg.TargetCNAME,
	}
	coreDNSManager := coredns.NewManager(m.client, coreDNSConfig)

	// Step 1: Remove import statement from CoreDNS Corefile
	if err := m.removeCoreDNSImport(ctx, coreDNSManager, cfg); err != nil {
		m.logger.Error(err, "Failed to remove import statement from CoreDNS")
	}

	// Step 2: Remove volume mount from CoreDNS deployment
	if err := m.removeCoreDNSVolumeMount(ctx, coreDNSManager, cfg); err != nil {
		m.logger.Error(err, "Failed to remove volume mount from CoreDNS deployment")
	}

	// Step 3: Delete the dynamic ConfigMap
	if err := m.deleteDynamicConfigMap(ctx, cfg); err != nil {
		m.logger.Error(err, "Failed to delete dynamic ConfigMap", "configmap", cfg.DynamicConfigMapName)
		return err
	}

	m.logger.Info("Cleanup completed successfully")
	return nil
}

// removeCoreDNSImport removes the import statement from CoreDNS Corefile
func (m *Manager) removeCoreDNSImport(ctx context.Context, coreDNSManager *coredns.Manager, cfg *config.Config) error {
	coreDNSConfigMap := &corev1.ConfigMap{}
	coreDNSConfigMapName := types.NamespacedName{
		Name:      cfg.CoreDNSConfigMapName,
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := m.client.Get(ctx, coreDNSConfigMapName, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to get CoreDNS ConfigMap: %w", err)
	}

	// Check if Corefile exists
	corefile, exists := coreDNSConfigMap.Data["Corefile"]
	if !exists {
		return fmt.Errorf("corefile not found in CoreDNS ConfigMap")
	}

	// Remove import statement if it exists
	if !strings.Contains(corefile, cfg.ImportStatement) {
		m.logger.Info("Import statement not found in CoreDNS Corefile - already removed")
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

	if err := m.client.Update(ctx, coreDNSConfigMap); err != nil {
		return fmt.Errorf("failed to update CoreDNS ConfigMap: %w", err)
	}

	m.logger.Info("Removed import statement from CoreDNS Corefile")
	return nil
}

// removeCoreDNSVolumeMount removes the volume mount from CoreDNS deployment
func (m *Manager) removeCoreDNSVolumeMount(ctx context.Context, coreDNSManager *coredns.Manager, cfg *config.Config) error {
	deployment := &appsv1.Deployment{}
	deploymentName := types.NamespacedName{
		Name:      "coredns",
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := m.client.Get(ctx, deploymentName, deployment); err != nil {
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
		if err := m.client.Update(ctx, deployment); err != nil {
			return fmt.Errorf("failed to update CoreDNS deployment: %w", err)
		}
		m.logger.Info("Removed custom config volume mount from CoreDNS deployment")
	} else {
		m.logger.Info("Custom config volume mount not found in CoreDNS deployment - already removed")
	}

	return nil
}

// deleteDynamicConfigMap deletes the dynamic ConfigMap
func (m *Manager) deleteDynamicConfigMap(ctx context.Context, cfg *config.Config) error {
	configMap := &corev1.ConfigMap{}
	configMapName := types.NamespacedName{
		Name:      cfg.DynamicConfigMapName,
		Namespace: cfg.CoreDNSNamespace,
	}

	if err := m.client.Get(ctx, configMapName, configMap); err != nil {
		m.logger.Info("Dynamic ConfigMap not found or already deleted", 
			"configmap", cfg.DynamicConfigMapName, 
			"error", err.Error())
		return nil
	}

	if err := m.client.Delete(ctx, configMap); err != nil {
		return fmt.Errorf("failed to delete dynamic ConfigMap: %w", err)
	}

	m.logger.Info("Successfully deleted dynamic ConfigMap", "configmap", cfg.DynamicConfigMapName)
	return nil
}
