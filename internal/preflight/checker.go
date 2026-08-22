package preflight

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	traefikv1alpha1 "github.com/rl-io/coredns-ingress-sync/internal/traefik/v1alpha1"
)

// Config holds the preflight check configuration
type Config struct {
	DeploymentName       string
	ReleaseInstance      string
	MountPath            string
	VolumeName           string
	DynamicConfigMapName string
	CoreDNSNamespace     string
	IngressClass         string
	TargetCNAME          string
	// GatewayAPIEnabled gates the Gateway API preflight check. When false
	// (no GatewayClassMappings configured), no List/Get calls are made
	// against Gateway/HTTPRoute types, since their CRDs may not be
	// installed in a pure-Ingress cluster.
	GatewayAPIEnabled bool
	// IngressRouteEnabled gates the Traefik IngressRoute preflight check. When
	// false (no IngressRouteTargetCNAME configured), no List/Get calls are
	// made against IngressRoute types, since the CRD may not be installed in
	// a cluster that doesn't use Traefik.
	IngressRouteEnabled bool
}

// Checker performs preflight checks for deployment conflicts
type Checker struct {
	client client.Client
	config Config
	logger logr.Logger
}

// NewChecker creates a new preflight checker
func NewChecker(client client.Client, config Config, logger logr.Logger) *Checker {
	return &Checker{
		client: client,
		config: config,
		logger: logger,
	}
}

// CheckResult represents the result of a preflight check
type CheckResult struct {
	Passed   bool
	Warning  bool
	Message  string
	Severity string // "error", "warning", "info"
}

// RunChecks performs all preflight checks and returns results
func (c *Checker) RunChecks(ctx context.Context) ([]CheckResult, error) {
	var results []CheckResult
	start := time.Now()

	c.logger.Info("🔍 Running preflight checks for CoreDNS ingress sync deployment",
		"deployment", c.config.DeploymentName,
		"mountPath", c.config.MountPath,
		"volumeName", c.config.VolumeName)

	// Check 1: CoreDNS deployment exists (with retry for RBAC issues)
	checkStart := time.Now()
	result, err := c.checkCoreDNSDeploymentWithRetry(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check CoreDNS deployment after %v: %w", time.Since(checkStart), err)
	}
	c.logger.Info("✓ CoreDNS deployment check completed", "duration", time.Since(checkStart), "passed", result.Passed)
	results = append(results, result)
	if !result.Passed {
		c.logger.Info("🏃 Early exit due to CoreDNS deployment check failure", "totalDuration", time.Since(start))
		return results, nil // Early exit if CoreDNS doesn't exist
	}

	// Check 2: Mount path conflicts
	checkStart = time.Now()
	result, err = c.checkMountPathConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check mount path conflicts after %v: %w", time.Since(checkStart), err)
	}
	c.logger.Info("✓ Mount path check completed", "duration", time.Since(checkStart), "passed", result.Passed)
	results = append(results, result)

	// Check 3: ConfigMap conflicts
	checkStart = time.Now()
	result, err = c.checkConfigMapConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check ConfigMap conflicts after %v: %w", time.Since(checkStart), err)
	}
	c.logger.Info("✓ ConfigMap check completed", "duration", time.Since(checkStart), "passed", result.Passed)
	results = append(results, result)

	// Check 4: Duplicate controllers
	checkStart = time.Now()
	result, err = c.checkDuplicateControllers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate controllers after %v: %w", time.Since(checkStart), err)
	}
	c.logger.Info("✓ Duplicate controllers check completed", "duration", time.Since(checkStart), "passed", result.Passed)
	results = append(results, result)

	// Check 5: Gateway API availability, only when configured -- no List/Get
	// calls against Gateway/HTTPRoute types otherwise.
	if c.config.GatewayAPIEnabled {
		checkStart = time.Now()
		result, err = c.checkGatewayAPI(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check Gateway API availability after %v: %w", time.Since(checkStart), err)
		}
		c.logger.Info("✓ Gateway API check completed", "duration", time.Since(checkStart), "passed", result.Passed)
		results = append(results, result)
	}

	// Check 6: Traefik IngressRoute availability, only when configured -- no
	// List/Get calls against IngressRoute types otherwise.
	if c.config.IngressRouteEnabled {
		checkStart = time.Now()
		result, err = c.checkTraefikIngressRoute(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check Traefik IngressRoute availability after %v: %w", time.Since(checkStart), err)
		}
		c.logger.Info("✓ Traefik IngressRoute check completed", "duration", time.Since(checkStart), "passed", result.Passed)
		results = append(results, result)
	}

	c.logger.Info("🎉 All preflight checks completed", "totalDuration", time.Since(start))
	return results, nil
}

// checkCoreDNSDeployment verifies CoreDNS deployment exists
func (c *Checker) checkCoreDNSDeployment(ctx context.Context) (CheckResult, error) {
	deployment := &appsv1.Deployment{}
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      "coredns",
		Namespace: c.config.CoreDNSNamespace,
	}, deployment)

	if err != nil {
		// Check if this is a permission/RBAC error
		if errors.IsForbidden(err) {
			return CheckResult{
				Passed:   false,
				Message:  fmt.Sprintf("❌ Permission denied accessing CoreDNS deployment in namespace %s. This usually means RBAC resources are not yet created. Try again in a few seconds.", c.config.CoreDNSNamespace),
				Severity: "error",
			}, nil
		}

		if errors.IsNotFound(err) {
			return CheckResult{
				Passed:   false,
				Message:  fmt.Sprintf("❌ CoreDNS deployment not found in namespace %s", c.config.CoreDNSNamespace),
				Severity: "error",
			}, nil
		}

		// Other errors
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ Error accessing CoreDNS deployment: %v", err),
			Severity: "error",
		}, nil
	}

	return CheckResult{
		Passed:   true,
		Message:  "✅ CoreDNS deployment found",
		Severity: "info",
	}, nil
}

// checkCoreDNSDeploymentWithRetry performs the CoreDNS deployment check with retry for RBAC propagation
func (c *Checker) checkCoreDNSDeploymentWithRetry(ctx context.Context) (CheckResult, error) {
	const maxRetries = 2               // Reduced from 3 for faster failure
	const retryDelay = 1 * time.Second // Reduced from 2 seconds

	for attempt := 1; attempt <= maxRetries; attempt++ {
		attemptStart := time.Now()
		c.logger.Info("Attempting CoreDNS deployment check", "attempt", attempt, "maxRetries", maxRetries)

		result, err := c.checkCoreDNSDeployment(ctx)
		duration := time.Since(attemptStart)

		// If no error or not a permission error, return immediately
		if err != nil || result.Passed || !strings.Contains(result.Message, "Permission denied") {
			c.logger.Info("CoreDNS deployment check completed",
				"attempt", attempt,
				"duration", duration,
				"passed", result.Passed,
				"error", err != nil)
			return result, err
		}

		// If it's a permission error and we have retries left, wait and retry
		if attempt < maxRetries {
			c.logger.Info("RBAC permissions not ready, retrying...",
				"attempt", attempt,
				"maxRetries", maxRetries,
				"duration", duration,
				"retryDelay", retryDelay)
			time.Sleep(retryDelay)
			continue
		}

		// Final attempt failed
		c.logger.Info("All retry attempts exhausted", "totalAttempts", maxRetries, "finalDuration", duration)
		return result, err
	}

	// Should never reach here, but satisfy compiler
	return CheckResult{Passed: false, Message: "Unexpected error in retry logic", Severity: "error"}, nil
}

// checkMountPathConflicts checks for mount path conflicts
func (c *Checker) checkMountPathConflicts(ctx context.Context) (CheckResult, error) {
	deployment := &appsv1.Deployment{}
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      "coredns",
		Namespace: c.config.CoreDNSNamespace,
	}, deployment)

	if err != nil {
		// Check if this is a permission/RBAC error
		if errors.IsForbidden(err) {
			return CheckResult{
				Passed:   false,
				Message:  "❌ Permission denied accessing CoreDNS deployment for mount path check. RBAC resources may not be ready yet.",
				Severity: "error",
			}, nil
		}

		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ Could not retrieve CoreDNS deployment for mount path check: %v", err),
			Severity: "error",
		}, nil
	}

	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return CheckResult{
			Passed:   false,
			Message:  "❌ CoreDNS deployment has no containers",
			Severity: "error",
		}, nil
	}

	container := deployment.Spec.Template.Spec.Containers[0]
	for _, mount := range container.VolumeMounts {
		if mount.MountPath == c.config.MountPath && mount.Name != c.config.VolumeName {
			return CheckResult{
				Passed:   false,
				Message:  fmt.Sprintf("❌ Mount path conflict detected!\n   Path: %s\n   Existing volume: %s\n   Our volume: %s\n\n💡 Suggested solutions:\n   1. Set a custom mount path in Helm values\n   2. Use a different deployment name\n   3. Remove the conflicting mount from CoreDNS", c.config.MountPath, mount.Name, c.config.VolumeName),
				Severity: "error",
			}, nil
		}
	}

	return CheckResult{
		Passed:   true,
		Message:  "✅ No mount path conflicts detected",
		Severity: "info",
	}, nil
}

// checkConfigMapConflicts checks for ConfigMap conflicts
func (c *Checker) checkConfigMapConflicts(ctx context.Context) (CheckResult, error) {
	configMap := &corev1.ConfigMap{}
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      c.config.DynamicConfigMapName,
		Namespace: c.config.CoreDNSNamespace,
	}, configMap)

	if err != nil {
		// ConfigMap doesn't exist, no conflict
		return CheckResult{
			Passed:   true,
			Message:  "✅ No ConfigMap conflicts detected",
			Severity: "info",
		}, nil
	}

	// Check if managed by a different instance
	managedBy := ""
	if configMap.Labels != nil {
		managedBy = configMap.Labels["app.kubernetes.io/instance"]
	}

	if managedBy != "" && managedBy != c.config.ReleaseInstance {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ ConfigMap conflict detected!\n   ConfigMap: %s\n   Managed by instance: %s\n   Our instance: %s\n\n💡 Suggested solutions:\n   1. Set a custom ConfigMap name in Helm values\n   2. Use a different release name", c.config.DynamicConfigMapName, managedBy, c.config.ReleaseInstance),
			Severity: "error",
		}, nil
	}

	return CheckResult{
		Passed:   true,
		Message:  "✅ No ConfigMap conflicts detected",
		Severity: "info",
	}, nil
}

// checkDuplicateControllers checks for other similar controllers
func (c *Checker) checkDuplicateControllers(ctx context.Context) (CheckResult, error) {
	deploymentList := &appsv1.DeploymentList{}
	err := c.client.List(ctx, deploymentList, client.MatchingLabels{
		"app.kubernetes.io/name": "coredns-ingress-sync",
	})

	if err != nil {
		return CheckResult{
			Passed:   true,
			Warning:  true,
			Message:  "⚠️  Could not check for duplicate controllers (non-critical)",
			Severity: "warning",
		}, nil
	}

	var otherDeployments []string
	for _, deployment := range deploymentList.Items {
		if deployment.Name != c.config.DeploymentName {
			otherDeployments = append(otherDeployments, fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name))
		}
	}

	if len(otherDeployments) > 0 {
		message := "⚠️  Found other coredns-ingress-sync deployments:\n"
		for _, dep := range otherDeployments {
			message += fmt.Sprintf("   - %s\n", dep)
		}
		message += "\n💡 Make sure each deployment watches different:\n"
		message += "   - Ingress classes\n"
		message += "   - Namespaces\n"
		message += "   - Or targets different CNAMEs"

		return CheckResult{
			Passed:   true,
			Warning:  true,
			Message:  message,
			Severity: "warning",
		}, nil
	}

	return CheckResult{
		Passed:   true,
		Message:  "✅ No duplicate controllers detected",
		Severity: "info",
	}, nil
}

// checkGatewayAPI verifies the Gateway API CRDs (Gateway, HTTPRoute) are
// installed and reachable. Callers must only invoke this when Gateway API
// support is configured (GatewayAPIEnabled), since a bounded List against
// these types is otherwise unnecessary and would fail on clusters that
// don't have the CRDs installed at all.
func (c *Checker) checkGatewayAPI(ctx context.Context) (CheckResult, error) {
	var gatewayList gatewayv1.GatewayList
	if err := c.client.List(ctx, &gatewayList, client.Limit(1)); err != nil {
		return gatewayAPICheckFailure("Gateway", err), nil
	}

	var routeList gatewayv1.HTTPRouteList
	if err := c.client.List(ctx, &routeList, client.Limit(1)); err != nil {
		return gatewayAPICheckFailure("HTTPRoute", err), nil
	}

	return CheckResult{
		Passed:   true,
		Message:  "✅ Gateway API CRDs (Gateway, HTTPRoute) are installed and reachable",
		Severity: "info",
	}, nil
}

// gatewayAPICheckFailure builds a CheckResult for a failed Gateway API List
// call, distinguishing "CRD not installed" from "insufficient RBAC" so the
// operator knows which one to fix.
func gatewayAPICheckFailure(kind string, err error) CheckResult {
	if meta.IsNoMatchError(err) {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ Gateway API %s CRD not found in the cluster. Install the Gateway API CRDs (e.g. https://github.com/kubernetes-sigs/gateway-api releases) before enabling gatewayClassMappings, or clear gatewayClassMappings to disable Gateway API support.", kind),
			Severity: "error",
		}
	}
	if errors.IsForbidden(err) {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ Permission denied listing %s resources. RBAC for gateway.networking.k8s.io may not be ready yet, or the chart's RBAC template wasn't applied with gatewayClassMappings set.", kind),
			Severity: "error",
		}
	}
	return CheckResult{
		Passed:   false,
		Message:  fmt.Sprintf("❌ Error accessing Gateway API %s resources: %v", kind, err),
		Severity: "error",
	}
}

// checkTraefikIngressRoute verifies the Traefik IngressRoute CRD is installed
// and reachable. Callers must only invoke this when IngressRoute support is
// configured (IngressRouteEnabled), since a bounded List against this type
// is otherwise unnecessary and would fail on clusters that don't have the
// CRD installed at all.
func (c *Checker) checkTraefikIngressRoute(ctx context.Context) (CheckResult, error) {
	var routeList traefikv1alpha1.IngressRouteList
	if err := c.client.List(ctx, &routeList, client.Limit(1)); err != nil {
		return traefikIngressRouteCheckFailure("IngressRoute", err), nil
	}

	return CheckResult{
		Passed:   true,
		Message:  "✅ Traefik IngressRoute CRD is installed and reachable",
		Severity: "info",
	}, nil
}

// traefikIngressRouteCheckFailure builds a CheckResult for a failed
// IngressRoute List call, distinguishing "CRD not installed" from
// "insufficient RBAC" so the operator knows which one to fix.
func traefikIngressRouteCheckFailure(kind string, err error) CheckResult {
	if meta.IsNoMatchError(err) {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ Traefik %s CRD not found in the cluster. Install Traefik's CRDs before setting ingressRouteTargetCNAME, or clear ingressRouteTargetCNAME to disable IngressRoute support.", kind),
			Severity: "error",
		}
	}
	if errors.IsForbidden(err) {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("❌ Permission denied listing %s resources. RBAC for traefik.io may not be ready yet, or the chart's RBAC template wasn't applied with ingressRouteTargetCNAME set.", kind),
			Severity: "error",
		}
	}
	return CheckResult{
		Passed:   false,
		Message:  fmt.Sprintf("❌ Error accessing Traefik %s resources: %v", kind, err),
		Severity: "error",
	}
}

// PrintResults prints the check results in a formatted way
func (c *Checker) PrintResults(results []CheckResult) {
	c.logger.Info("")
	c.logger.Info("📋 Preflight Check Results:")
	c.logger.Info("============================")

	passed := 0
	warnings := 0
	errors := 0

	for _, result := range results {
		// Split message into lines for better formatting
		lines := strings.Split(result.Message, "\n")
		for i, line := range lines {
			if i == 0 {
				c.logger.Info(line)
			} else if strings.TrimSpace(line) != "" {
				c.logger.Info("   " + line)
			}
		}

		switch result.Severity {
		case "error":
			if !result.Passed {
				errors++
			}
		case "warning":
			warnings++
		case "info":
			if result.Passed {
				passed++
			}
		}
	}

	c.logger.Info("")
	c.logger.Info("📊 Summary:")
	c.logger.Info(fmt.Sprintf("   ✅ Passed: %d", passed))
	if warnings > 0 {
		c.logger.Info(fmt.Sprintf("   ⚠️  Warnings: %d", warnings))
	}
	if errors > 0 {
		c.logger.Info(fmt.Sprintf("   ❌ Errors: %d", errors))
	}

	if errors == 0 {
		c.logger.Info("")
		c.logger.Info("🎉 All critical checks passed! Deployment can proceed safely.")
	} else {
		c.logger.Info("")
		c.logger.Info("❌ PREFLIGHT CHECKS FAILED - Deployment cannot proceed")
		c.logger.Info("Please resolve the above errors before installing/upgrading.")
	}
}

// HasErrors returns true if any check failed with an error
func HasErrors(results []CheckResult) bool {
	for _, result := range results {
		if !result.Passed && result.Severity == "error" {
			return true
		}
	}
	return false
}

// ConfigFromEnv creates a preflight config from the current environment
func ConfigFromEnv(cfg *config.Config) Config {
	return Config{
		DeploymentName:       cfg.ControllerNamespace, // This will be set by Helm
		ReleaseInstance:      cfg.ControllerNamespace, // This will be set by Helm
		MountPath:            cfg.MountPath,
		VolumeName:           cfg.CoreDNSVolumeName,
		DynamicConfigMapName: cfg.DynamicConfigMapName,
		CoreDNSNamespace:     cfg.CoreDNSNamespace,
		IngressClass:         cfg.IngressClass,
		TargetCNAME:          cfg.TargetCNAME,
		GatewayAPIEnabled:    len(cfg.GatewayClassMappings) > 0,
		IngressRouteEnabled:  cfg.IngressRouteTargetCNAME != "",
	}
}
