package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Save original environment
	originalVars := map[string]string{
		"INGRESS_CLASS":                   os.Getenv("INGRESS_CLASS"),
		"TARGET_CNAME":                    os.Getenv("TARGET_CNAME"),
		"DYNAMIC_CONFIGMAP_NAME":          os.Getenv("DYNAMIC_CONFIGMAP_NAME"),
		"DYNAMIC_CONFIG_KEY":              os.Getenv("DYNAMIC_CONFIG_KEY"),
		"COREDNS_NAMESPACE":               os.Getenv("COREDNS_NAMESPACE"),
		"COREDNS_CONFIGMAP_NAME":          os.Getenv("COREDNS_CONFIGMAP_NAME"),
		"LEADER_ELECTION_ENABLED":         os.Getenv("LEADER_ELECTION_ENABLED"),
		"WATCH_NAMESPACES":                os.Getenv("WATCH_NAMESPACES"),
		"EXCLUDE_NAMESPACES":              os.Getenv("EXCLUDE_NAMESPACES"),
		"EXCLUDE_INGRESSES":               os.Getenv("EXCLUDE_INGRESSES"),
		"POD_NAMESPACE":                   os.Getenv("POD_NAMESPACE"),
		"DEPLOYMENT_NAME":                 os.Getenv("DEPLOYMENT_NAME"),
		"MOUNT_PATH":                      os.Getenv("MOUNT_PATH"),
		"ANNOTATION_ENABLED_KEY":          os.Getenv("ANNOTATION_ENABLED_KEY"),
		"ANNOTATION_PRIORITY_KEY":         os.Getenv("ANNOTATION_PRIORITY_KEY"),
		"INGRESS_CLASS_MAPPINGS":          os.Getenv("INGRESS_CLASS_MAPPINGS"),
		"GATEWAY_CLASS_MAPPINGS":          os.Getenv("GATEWAY_CLASS_MAPPINGS"),
		"GATEWAY_CLASS":                   os.Getenv("GATEWAY_CLASS"),
		"GATEWAY_TARGET_CNAME":            os.Getenv("GATEWAY_TARGET_CNAME"),
		"EXCLUDE_HTTPROUTES":              os.Getenv("EXCLUDE_HTTPROUTES"),
		"INGRESSROUTE_TARGET_CNAME":       os.Getenv("INGRESSROUTE_TARGET_CNAME"),
		"EXCLUDE_INGRESSROUTES":           os.Getenv("EXCLUDE_INGRESSROUTES"),
		"SERVICE_ANNOTATIONS_ENABLED":     os.Getenv("SERVICE_ANNOTATIONS_ENABLED"),
		"SERVICE_HOSTNAME_ANNOTATION_KEY": os.Getenv("SERVICE_HOSTNAME_ANNOTATION_KEY"),
		"EXCLUDE_SERVICES":                os.Getenv("EXCLUDE_SERVICES"),
		"CLUSTER_DOMAIN":                  os.Getenv("CLUSTER_DOMAIN"),
	}

	// Restore original environment after test
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	t.Run("default values", func(t *testing.T) {
		// Clear all environment variables
		for key := range originalVars {
			os.Unsetenv(key)
		}

		config := Load()

		assert.Equal(t, "nginx", config.IngressClass)
		assert.Equal(t, "ingress-nginx-controller.ingress-nginx.svc.cluster.local.", config.TargetCNAME)
		assert.Equal(t, "coredns-ingress-sync-rewrite-rules", config.DynamicConfigMapName)
		assert.Equal(t, "dynamic.server", config.DynamicConfigKey)
		assert.Equal(t, "kube-system", config.CoreDNSNamespace)
		assert.Equal(t, "coredns", config.CoreDNSConfigMapName)
		assert.Equal(t, "coredns-ingress-sync-volume", config.CoreDNSVolumeName)
		assert.True(t, config.LeaderElectionEnabled)
		assert.Equal(t, "", config.WatchNamespaces)
		assert.Equal(t, "", config.ExcludeNamespaces)
		assert.Equal(t, "", config.ExcludeIngresses)
		assert.Equal(t, "import /etc/coredns/custom/coredns-ingress-sync/*.server", config.ImportStatement)
		assert.Equal(t, "coredns-ingress-sync", config.ControllerNamespace) // Default fallback
		assert.Equal(t, "/etc/coredns/custom/coredns-ingress-sync", config.MountPath)
		assert.Equal(t, "coredns-ingress-sync", config.ReleaseInstance)
		assert.Equal(t, "coredns-ingress-sync-enabled", config.AnnotationEnabledKey)
		assert.Equal(t, "coredns-ingress-sync-priority", config.AnnotationPriorityKey)
		// Default mappings are the single-class backward-compatible shim.
		assert.Equal(t, []IngressClassMapping{{
			IngressClass: "nginx",
			TargetCNAME:  "ingress-nginx-controller.ingress-nginx.svc.cluster.local.",
		}}, config.IngressClassMappings)
		// Gateway API support is disabled by default: nil, not an empty shim.
		assert.Nil(t, config.GatewayClassMappings)
		assert.Equal(t, "", config.ExcludeHTTPRoutes)
		// Traefik IngressRoute support is disabled by default: empty target CNAME.
		assert.Equal(t, "", config.IngressRouteTargetCNAME)
		assert.Equal(t, "", config.ExcludeIngressRoutes)
		// Service annotation support is disabled by default.
		assert.False(t, config.ServiceAnnotationsEnabled)
		assert.Equal(t, "coredns-ingress-sync-hostname", config.ServiceHostnameAnnotationKey)
		assert.Equal(t, "", config.ExcludeServices)
		assert.Equal(t, "cluster.local", config.ClusterDomain)
	})

	t.Run("environment overrides", func(t *testing.T) {
		// Set custom environment variables
		os.Setenv("INGRESS_CLASS", "traefik")
		os.Setenv("TARGET_CNAME", "traefik.example.com")
		os.Setenv("DYNAMIC_CONFIGMAP_NAME", "custom-config")
		os.Setenv("DYNAMIC_CONFIG_KEY", "custom.server")
		os.Setenv("COREDNS_NAMESPACE", "dns-system")
		os.Setenv("COREDNS_CONFIGMAP_NAME", "custom-coredns")
		os.Setenv("LEADER_ELECTION_ENABLED", "false")
		os.Setenv("WATCH_NAMESPACES", "production,staging")
		os.Setenv("EXCLUDE_NAMESPACES", "dev,staging")
		os.Setenv("EXCLUDE_INGRESSES", "foo,bar/ns1,baz/qux")
		os.Setenv("POD_NAMESPACE", "custom-namespace")
		os.Setenv("DEPLOYMENT_NAME", "my-custom-deployment")
		os.Setenv("MOUNT_PATH", "/custom/mount/path")
		os.Setenv("ANNOTATION_ENABLED_KEY", "my-company.io/dns-sync-enabled")
		os.Setenv("ANNOTATION_PRIORITY_KEY", "my-company.io/dns-sync-priority")
		os.Setenv("SERVICE_ANNOTATIONS_ENABLED", "true")
		os.Setenv("SERVICE_HOSTNAME_ANNOTATION_KEY", "my-company.io/hostname")
		os.Setenv("EXCLUDE_SERVICES", "foo,bar/ns1")
		os.Setenv("CLUSTER_DOMAIN", "prod.local")

		config := Load()

		assert.Equal(t, "traefik", config.IngressClass)
		assert.Equal(t, "traefik.example.com", config.TargetCNAME)
		assert.Equal(t, "my-company.io/dns-sync-priority", config.AnnotationPriorityKey)
		// With no INGRESS_CLASS_MAPPINGS, the legacy vars form a single mapping.
		assert.Equal(t, []IngressClassMapping{{
			IngressClass: "traefik",
			TargetCNAME:  "traefik.example.com",
		}}, config.IngressClassMappings)
		assert.Equal(t, "custom-config", config.DynamicConfigMapName)
		assert.Equal(t, "custom.server", config.DynamicConfigKey)
		assert.Equal(t, "dns-system", config.CoreDNSNamespace)
		assert.Equal(t, "custom-coredns", config.CoreDNSConfigMapName)
		assert.False(t, config.LeaderElectionEnabled)
		assert.Equal(t, "production,staging", config.WatchNamespaces)
		assert.Equal(t, "dev,staging", config.ExcludeNamespaces)
		assert.Equal(t, "foo,bar/ns1,baz/qux", config.ExcludeIngresses)
		assert.Equal(t, "custom-namespace", config.ControllerNamespace)
		assert.Equal(t, "/custom/mount/path", config.MountPath)
		assert.Equal(t, "my-custom-deployment", config.ReleaseInstance)
		assert.Equal(t, "my-company.io/dns-sync-enabled", config.AnnotationEnabledKey)
		assert.True(t, config.ServiceAnnotationsEnabled)
		assert.Equal(t, "my-company.io/hostname", config.ServiceHostnameAnnotationKey)
		assert.Equal(t, "foo,bar/ns1", config.ExcludeServices)
		assert.Equal(t, "prod.local", config.ClusterDomain)
	})
}

func TestLoadIngressClassMappings(t *testing.T) {
	// Save and restore the env vars this test mutates.
	keys := []string{"INGRESS_CLASS_MAPPINGS", "INGRESS_CLASS", "TARGET_CNAME"}
	original := make(map[string]string, len(keys))
	for _, k := range keys {
		original[k] = os.Getenv(k)
	}
	defer func() {
		for k, v := range original {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	t.Run("multi-class JSON takes precedence", func(t *testing.T) {
		os.Setenv("INGRESS_CLASS", "nginx")
		os.Setenv("TARGET_CNAME", "legacy.example.com.")
		os.Setenv("INGRESS_CLASS_MAPPINGS", `[
			{"ingressClass":"nginx","targetCNAME":"nginx.svc.cluster.local."},
			{"ingressClass":"traefik","targetCNAME":"traefik.svc.cluster.local."}
		]`)

		cfg := Load()

		assert.Equal(t, []IngressClassMapping{
			{IngressClass: "nginx", TargetCNAME: "nginx.svc.cluster.local."},
			{IngressClass: "traefik", TargetCNAME: "traefik.svc.cluster.local."},
		}, cfg.IngressClassMappings)
		// First mapping mirrors the legacy single-class fields.
		assert.Equal(t, "nginx", cfg.IngressClass)
		assert.Equal(t, "nginx.svc.cluster.local.", cfg.TargetCNAME)
	})

	t.Run("invalid JSON falls back to single-class shim", func(t *testing.T) {
		os.Setenv("INGRESS_CLASS", "traefik")
		os.Setenv("TARGET_CNAME", "traefik.example.com.")
		os.Setenv("INGRESS_CLASS_MAPPINGS", "{not json")

		cfg := Load()

		assert.Equal(t, []IngressClassMapping{
			{IngressClass: "traefik", TargetCNAME: "traefik.example.com."},
		}, cfg.IngressClassMappings)
	})

	t.Run("entries missing ingressClass are dropped", func(t *testing.T) {
		os.Unsetenv("INGRESS_CLASS")
		os.Unsetenv("TARGET_CNAME")
		os.Setenv("INGRESS_CLASS_MAPPINGS", `[
			{"targetCNAME":"orphan.svc.cluster.local."},
			{"ingressClass":"traefik","targetCNAME":"traefik.svc.cluster.local."}
		]`)

		cfg := Load()

		assert.Equal(t, []IngressClassMapping{
			{IngressClass: "traefik", TargetCNAME: "traefik.svc.cluster.local."},
		}, cfg.IngressClassMappings)
	})

	t.Run("empty JSON array falls back to single-class shim", func(t *testing.T) {
		os.Setenv("INGRESS_CLASS", "nginx")
		os.Setenv("TARGET_CNAME", "nginx.example.com.")
		os.Setenv("INGRESS_CLASS_MAPPINGS", "[]")

		cfg := Load()

		assert.Equal(t, []IngressClassMapping{
			{IngressClass: "nginx", TargetCNAME: "nginx.example.com."},
		}, cfg.IngressClassMappings)
	})
}

func TestLoadGatewayClassMappings(t *testing.T) {
	keys := []string{"GATEWAY_CLASS_MAPPINGS", "GATEWAY_CLASS", "GATEWAY_TARGET_CNAME", "EXCLUDE_HTTPROUTES"}
	original := make(map[string]string, len(keys))
	for _, k := range keys {
		original[k] = os.Getenv(k)
	}
	defer func() {
		for k, v := range original {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	t.Run("unset means Gateway API disabled", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}

		cfg := Load()

		assert.Nil(t, cfg.GatewayClassMappings)
	})

	t.Run("legacy single-class vars form a mapping", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		os.Setenv("GATEWAY_CLASS", "traefik")
		os.Setenv("GATEWAY_TARGET_CNAME", "traefik.traefik.svc.cluster.local.")
		os.Setenv("EXCLUDE_HTTPROUTES", "foo,bar/ns1")

		cfg := Load()

		assert.Equal(t, []GatewayClassMapping{
			{GatewayClass: "traefik", TargetCNAME: "traefik.traefik.svc.cluster.local."},
		}, cfg.GatewayClassMappings)
		assert.Equal(t, "foo,bar/ns1", cfg.ExcludeHTTPRoutes)
	})

	t.Run("multi-class JSON takes precedence", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		os.Setenv("GATEWAY_CLASS", "traefik")
		os.Setenv("GATEWAY_TARGET_CNAME", "legacy.example.com.")
		os.Setenv("GATEWAY_CLASS_MAPPINGS", `[
			{"gatewayClass":"traefik","targetCNAME":"traefik.svc.cluster.local."},
			{"gatewayClass":"istio","targetCNAME":"istio.svc.cluster.local."}
		]`)

		cfg := Load()

		assert.Equal(t, []GatewayClassMapping{
			{GatewayClass: "traefik", TargetCNAME: "traefik.svc.cluster.local."},
			{GatewayClass: "istio", TargetCNAME: "istio.svc.cluster.local."},
		}, cfg.GatewayClassMappings)
	})

	t.Run("invalid JSON falls back to legacy single-class vars", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		os.Setenv("GATEWAY_CLASS", "traefik")
		os.Setenv("GATEWAY_TARGET_CNAME", "traefik.example.com.")
		os.Setenv("GATEWAY_CLASS_MAPPINGS", "{not json")

		cfg := Load()

		assert.Equal(t, []GatewayClassMapping{
			{GatewayClass: "traefik", TargetCNAME: "traefik.example.com."},
		}, cfg.GatewayClassMappings)
	})

	t.Run("invalid JSON with no legacy vars is disabled", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		os.Setenv("GATEWAY_CLASS_MAPPINGS", "{not json")

		cfg := Load()

		assert.Nil(t, cfg.GatewayClassMappings)
	})

	t.Run("entries missing gatewayClass are dropped", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		os.Setenv("GATEWAY_CLASS_MAPPINGS", `[
			{"targetCNAME":"orphan.svc.cluster.local."},
			{"gatewayClass":"traefik","targetCNAME":"traefik.svc.cluster.local."}
		]`)

		cfg := Load()

		assert.Equal(t, []GatewayClassMapping{
			{GatewayClass: "traefik", TargetCNAME: "traefik.svc.cluster.local."},
		}, cfg.GatewayClassMappings)
	})

	t.Run("empty JSON array with no legacy vars is disabled", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		os.Setenv("GATEWAY_CLASS_MAPPINGS", "[]")

		cfg := Load()

		assert.Nil(t, cfg.GatewayClassMappings)
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "use default when env var not set",
			key:          "NON_EXISTENT_VAR",
			defaultValue: "default_value",
			envValue:     "",
			expected:     "default_value",
		},
		{
			name:         "use env var when set",
			key:          "TEST_VAR",
			defaultValue: "default_value",
			envValue:     "env_value",
			expected:     "env_value",
		},
		{
			name:         "use default when env var is empty",
			key:          "EMPTY_VAR",
			defaultValue: "default_value",
			envValue:     "",
			expected:     "default_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv(tt.key)
			defer func() {
				if originalValue == "" {
					os.Unsetenv(tt.key)
				} else {
					os.Setenv(tt.key, originalValue)
				}
			}()

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv(tt.key)
			} else {
				os.Setenv(tt.key, tt.envValue)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
