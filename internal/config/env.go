package config

import "os"

// Config holds all configuration values for the coredns-ingress-sync controller
type Config struct {
	IngressClass          string
	TargetCNAME           string
	DynamicConfigMapName  string
	DynamicConfigKey      string
	CoreDNSNamespace      string
	CoreDNSConfigMapName  string
	CoreDNSVolumeName     string
	LeaderElectionEnabled bool
	WatchNamespaces       string
	ImportStatement       string
	ControllerNamespace   string // Namespace where the controller is deployed
}

// Load creates a new Config instance with values loaded from environment variables
func Load() *Config {
	return &Config{
		IngressClass:          getEnvOrDefault("INGRESS_CLASS", "nginx"),
		TargetCNAME:           getEnvOrDefault("TARGET_CNAME", "ingress-nginx-controller.ingress-nginx.svc.cluster.local."),
		DynamicConfigMapName:  getEnvOrDefault("DYNAMIC_CONFIGMAP_NAME", "coredns-ingress-sync-rewrite-rules"),
		DynamicConfigKey:      getEnvOrDefault("DYNAMIC_CONFIG_KEY", "dynamic.server"),
		CoreDNSNamespace:      getEnvOrDefault("COREDNS_NAMESPACE", "kube-system"),
		CoreDNSConfigMapName:  getEnvOrDefault("COREDNS_CONFIGMAP_NAME", "coredns"),
		CoreDNSVolumeName:     getEnvOrDefault("COREDNS_VOLUME_NAME", "coredns-ingress-sync-volume"),
		LeaderElectionEnabled: getEnvOrDefault("LEADER_ELECTION_ENABLED", "true") == "true",
		WatchNamespaces:       getEnvOrDefault("WATCH_NAMESPACES", ""), // Comma-separated list, empty = all namespaces
		ImportStatement:       "import /etc/coredns/custom/*.server",
		ControllerNamespace:   getEnvOrDefault("POD_NAMESPACE", "coredns-ingress-sync"), // Default fallback
	}
}

// getEnvOrDefault returns the value of the environment variable or the default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
