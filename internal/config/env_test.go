package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Save original environment
	originalVars := map[string]string{
		"INGRESS_CLASS":           os.Getenv("INGRESS_CLASS"),
		"TARGET_CNAME":            os.Getenv("TARGET_CNAME"),
		"DYNAMIC_CONFIGMAP_NAME":  os.Getenv("DYNAMIC_CONFIGMAP_NAME"),
		"DYNAMIC_CONFIG_KEY":      os.Getenv("DYNAMIC_CONFIG_KEY"),
		"COREDNS_NAMESPACE":       os.Getenv("COREDNS_NAMESPACE"),
		"COREDNS_CONFIGMAP_NAME":  os.Getenv("COREDNS_CONFIGMAP_NAME"),
		"LEADER_ELECTION_ENABLED": os.Getenv("LEADER_ELECTION_ENABLED"),
		"WATCH_NAMESPACES":        os.Getenv("WATCH_NAMESPACES"),
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
		assert.Equal(t, "coredns-custom", config.DynamicConfigMapName)
		assert.Equal(t, "dynamic.server", config.DynamicConfigKey)
		assert.Equal(t, "kube-system", config.CoreDNSNamespace)
		assert.Equal(t, "coredns", config.CoreDNSConfigMapName)
		assert.True(t, config.LeaderElectionEnabled)
		assert.Equal(t, "", config.WatchNamespaces)
		assert.Equal(t, "import /etc/coredns/custom/*.server", config.ImportStatement)
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

		config := Load()

		assert.Equal(t, "traefik", config.IngressClass)
		assert.Equal(t, "traefik.example.com", config.TargetCNAME)
		assert.Equal(t, "custom-config", config.DynamicConfigMapName)
		assert.Equal(t, "custom.server", config.DynamicConfigKey)
		assert.Equal(t, "dns-system", config.CoreDNSNamespace)
		assert.Equal(t, "custom-coredns", config.CoreDNSConfigMapName)
		assert.False(t, config.LeaderElectionEnabled)
		assert.Equal(t, "production,staging", config.WatchNamespaces)
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
