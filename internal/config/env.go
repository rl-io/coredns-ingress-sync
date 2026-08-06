package config

import (
	"encoding/json"
	"os"
)

// IngressClassMapping maps an ingress class to the target CNAME that CoreDNS
// should rewrite matching hostnames to. The order of mappings acts as the
// default tiebreak: the first entry has the lowest implicit priority.
type IngressClassMapping struct {
	IngressClass string `json:"ingressClass"`
	TargetCNAME  string `json:"targetCNAME"`
}

// GatewayClassMapping maps a Gateway API GatewayClass to the target CNAME
// that CoreDNS should rewrite matching HTTPRoute hostnames to. The order of
// mappings acts as the default tiebreak: the first entry has the lowest
// implicit priority. Unlike IngressClassMappings, an empty/nil
// GatewayClassMappings is a valid, expected state: it means Gateway API
// support is disabled.
type GatewayClassMapping struct {
	GatewayClass string `json:"gatewayClass"`
	TargetCNAME  string `json:"targetCNAME"`
}

// Config holds all configuration values for the coredns-ingress-sync controller
type Config struct {
	// IngressClass and TargetCNAME mirror the first (default, lowest-priority)
	// entry of IngressClassMappings. They are retained for backward
	// compatibility with single-class consumers (logging, preflight, cleanup).
	IngressClass string
	TargetCNAME  string
	// IngressClassMappings is the ordered list of class->CNAME mappings the
	// controller watches. Order defines the default priority tiebreak.
	IngressClassMappings []IngressClassMapping
	// GatewayClassMappings is the ordered list of Gateway API GatewayClass ->
	// CNAME mappings the controller watches. Empty/nil means Gateway API
	// support is disabled: no Gateway/HTTPRoute watches, caches, or List/Get
	// calls are made.
	GatewayClassMappings   []GatewayClassMapping
	ExcludeHTTPRoutes      string // Comma-separated list of HTTPRoute names or namespace/name
	DynamicConfigMapName   string
	DynamicConfigKey       string
	CoreDNSNamespace       string
	CoreDNSConfigMapName   string
	CoreDNSVolumeName      string
	LeaderElectionEnabled  bool
	WatchNamespaces        string
	ExcludeNamespaces      string // Comma-separated list of namespaces to exclude
	ExcludeIngresses       string // Comma-separated list of ingress names or namespace/name
	AnnotationEnabledKey   string // Annotation key to enable/disable processing (false disables)
	AnnotationPriorityKey  string // Annotation key to override per-ingress priority (integer, higher wins)
	ExcludeAnnotationKey   string // Annotation key to trigger exclusion when present
	ExcludeAnnotationValue string // Optional value to require for exclusion; empty means any value
	ImportStatement        string
	ControllerNamespace    string // Namespace where the controller is deployed
	MountPath              string // Configurable mount path for the volume
	ReleaseInstance        string // Helm release instance name
}

// Load creates a new Config instance with values loaded from environment variables
func Load() *Config {
	// Get mount path or create from deployment name
	mountPath := getEnvOrDefault("MOUNT_PATH", "")
	if mountPath == "" {
		// Create unique mount path based on deployment name
		deploymentName := getEnvOrDefault("DEPLOYMENT_NAME", "coredns-ingress-sync")
		mountPath = "/etc/coredns/custom/" + deploymentName
	}

	// Create import statement based on mount path
	importStatement := "import " + mountPath + "/*.server"

	// Resolve class mappings (multi-class config takes precedence over the
	// legacy single-class INGRESS_CLASS + TARGET_CNAME pair).
	mappings := loadIngressClassMappings()
	gatewayMappings := loadGatewayClassMappings()

	return &Config{
		IngressClass:           mappings[0].IngressClass,
		TargetCNAME:            mappings[0].TargetCNAME,
		IngressClassMappings:   mappings,
		GatewayClassMappings:   gatewayMappings,
		ExcludeHTTPRoutes:      getEnvOrDefault("EXCLUDE_HTTPROUTES", ""),
		DynamicConfigMapName:   getEnvOrDefault("DYNAMIC_CONFIGMAP_NAME", "coredns-ingress-sync-rewrite-rules"),
		DynamicConfigKey:       getEnvOrDefault("DYNAMIC_CONFIG_KEY", "dynamic.server"),
		CoreDNSNamespace:       getEnvOrDefault("COREDNS_NAMESPACE", "kube-system"),
		CoreDNSConfigMapName:   getEnvOrDefault("COREDNS_CONFIGMAP_NAME", "coredns"),
		CoreDNSVolumeName:      getEnvOrDefault("COREDNS_VOLUME_NAME", "coredns-ingress-sync-volume"),
		LeaderElectionEnabled:  getEnvOrDefault("LEADER_ELECTION_ENABLED", "true") == "true",
		WatchNamespaces:        getEnvOrDefault("WATCH_NAMESPACES", ""), // Comma-separated list, empty = all namespaces
		ExcludeNamespaces:      getEnvOrDefault("EXCLUDE_NAMESPACES", ""),
		ExcludeIngresses:       getEnvOrDefault("EXCLUDE_INGRESSES", ""),
		AnnotationEnabledKey:   getEnvOrDefault("ANNOTATION_ENABLED_KEY", "coredns-ingress-sync-enabled"),
		AnnotationPriorityKey:  getEnvOrDefault("ANNOTATION_PRIORITY_KEY", "coredns-ingress-sync-priority"),
		ExcludeAnnotationKey:   getEnvOrDefault("EXCLUDE_ANNOTATION_KEY", ""),
		ExcludeAnnotationValue: getEnvOrDefault("EXCLUDE_ANNOTATION_VALUE", ""),
		ImportStatement:        importStatement,
		ControllerNamespace:    getEnvOrDefault("POD_NAMESPACE", "coredns-ingress-sync"), // Default fallback
		MountPath:              mountPath,
		ReleaseInstance:        getEnvOrDefault("RELEASE_INSTANCE", getEnvOrDefault("DEPLOYMENT_NAME", "coredns-ingress-sync")),
	}
}

// loadIngressClassMappings resolves the ordered list of ingress class -> CNAME
// mappings. The JSON env var INGRESS_CLASS_MAPPINGS takes precedence when set
// and valid; otherwise it falls back to a single-entry mapping built from the
// legacy INGRESS_CLASS + TARGET_CNAME variables. The returned slice always
// contains at least one entry.
func loadIngressClassMappings() []IngressClassMapping {
	if raw := os.Getenv("INGRESS_CLASS_MAPPINGS"); raw != "" {
		var parsed []IngressClassMapping
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			// Drop entries missing an ingress class; they cannot be matched.
			valid := make([]IngressClassMapping, 0, len(parsed))
			for _, m := range parsed {
				if m.IngressClass != "" {
					valid = append(valid, m)
				}
			}
			if len(valid) > 0 {
				return valid
			}
		}
	}

	// Backward-compatible single-class shim.
	return []IngressClassMapping{{
		IngressClass: getEnvOrDefault("INGRESS_CLASS", "nginx"),
		TargetCNAME:  getEnvOrDefault("TARGET_CNAME", "ingress-nginx-controller.ingress-nginx.svc.cluster.local."),
	}}
}

// loadGatewayClassMappings resolves the ordered list of GatewayClass -> CNAME
// mappings. The JSON env var GATEWAY_CLASS_MAPPINGS takes precedence when set
// and valid; otherwise it falls back to a single-entry mapping built from
// GATEWAY_CLASS + GATEWAY_TARGET_CNAME, but only if GATEWAY_CLASS is
// explicitly set. Unlike loadIngressClassMappings, the zero-value result is
// nil (no entries) — this is the "Gateway API disabled" default, since Ingress
// support must keep working unchanged for deployments that never configure
// Gateway API.
func loadGatewayClassMappings() []GatewayClassMapping {
	if raw := os.Getenv("GATEWAY_CLASS_MAPPINGS"); raw != "" {
		var parsed []GatewayClassMapping
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			valid := make([]GatewayClassMapping, 0, len(parsed))
			for _, m := range parsed {
				if m.GatewayClass != "" {
					valid = append(valid, m)
				}
			}
			if len(valid) > 0 {
				return valid
			}
		}
	}

	if gatewayClass := os.Getenv("GATEWAY_CLASS"); gatewayClass != "" {
		return []GatewayClassMapping{{
			GatewayClass: gatewayClass,
			TargetCNAME:  getEnvOrDefault("GATEWAY_TARGET_CNAME", ""),
		}}
	}

	return nil
}

// getEnvOrDefault returns the value of the environment variable or the default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
