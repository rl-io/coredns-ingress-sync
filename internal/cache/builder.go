package cacheconfig

import (
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigBuilder helps build cache configuration
type ConfigBuilder struct {
	watchNamespaces  []string
	coreDNSNamespace string
}

// NewConfigBuilder creates a new cache config builder
func NewConfigBuilder(watchNamespaces []string, coreDNSNamespace string) *ConfigBuilder {
	return &ConfigBuilder{
		watchNamespaces:  watchNamespaces,
		coreDNSNamespace: coreDNSNamespace,
	}
}

// BuildCacheOptions creates cache options based on namespace configuration
func (cb *ConfigBuilder) BuildCacheOptions() cache.Options {
	var cacheOptions cache.Options

	if len(cb.watchNamespaces) > 0 {
		// Create namespace map for ingresses (only the specified namespaces)
		ingressNamespaceMap := make(map[string]cache.Config)
		for _, ns := range cb.watchNamespaces {
			ingressNamespaceMap[ns] = cache.Config{}
		}
		
		// Configure cache with ByObject for namespace-scoped resources
		cacheOptions.ByObject = map[client.Object]cache.ByObject{
			&networkingv1.Ingress{}: {
				Namespaces: ingressNamespaceMap,
			},
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					cb.coreDNSNamespace: {},
				},
			},
		}
		log.Printf("Configured to watch specific namespaces: %v (plus %s for CoreDNS)", cb.watchNamespaces, cb.coreDNSNamespace)
	} else {
		log.Printf("Configured to watch all namespaces")
	}

	return cacheOptions
}

// ParseNamespaces parses the watch namespaces environment variable
func ParseNamespaces(watchNamespacesEnv string) []string {
	var namespaces []string
	if watchNamespacesEnv != "" {
		// Split by comma and remove whitespace
		for _, ns := range splitAndTrim(watchNamespacesEnv, ",") {
			if ns != "" {
				namespaces = append(namespaces, ns)
			}
		}
	}
	return namespaces
}

// splitAndTrim splits a string by separator and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		result = append(result, trimmed)
	}
	return result
}
