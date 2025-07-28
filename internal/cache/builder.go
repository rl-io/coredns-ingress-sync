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
		// Namespace-scoped cache configuration - works for single or multiple namespaces
		// Create a namespace map for each namespace we want to watch
		ingressNamespaceMap := make(map[string]cache.Config)
		for _, ns := range cb.watchNamespaces {
			ingressNamespaceMap[ns] = cache.Config{}
		}
		
		// Always need access to CoreDNS namespace for ConfigMap operations
		configMapNamespaceMap := map[string]cache.Config{
			cb.coreDNSNamespace: {},
		}
		
		// If we're watching namespaces that include the CoreDNS namespace,
		// we need to merge the configs to avoid conflicts
		if cb.coreDNSNamespace != "" {
			for _, ns := range cb.watchNamespaces {
				if ns == cb.coreDNSNamespace {
					// CoreDNS namespace is in our watch list, so we can use the same config
					configMapNamespaceMap[cb.coreDNSNamespace] = cache.Config{}
					break
				}
			}
		}
		
		cacheOptions.ByObject = map[client.Object]cache.ByObject{
			&networkingv1.Ingress{}: {
				Namespaces: ingressNamespaceMap,
			},
			&corev1.ConfigMap{}: {
				Namespaces: configMapNamespaceMap,
			},
		}
		
		if len(cb.watchNamespaces) == 1 {
			log.Printf("Using namespace-scoped cache for single namespace: %s", cb.watchNamespaces[0])
		} else {
			log.Printf("Using namespace-scoped cache for multiple namespaces: %v", cb.watchNamespaces)
		}
		
		log.Printf("CoreDNS ConfigMap access configured for namespace: %s", cb.coreDNSNamespace)
	} else {
		// Cluster-wide watching - no namespace restrictions
		log.Printf("Using cluster-wide cache - watching all namespaces")
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
