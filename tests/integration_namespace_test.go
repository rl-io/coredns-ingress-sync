package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	
	ingressfilter "github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

func TestNamespaceFilteringIsolated(t *testing.T) {
	tests := []struct {
		name            string
		watchNamespaces string
		testNamespace   string
		shouldProcess   bool
		description     string
	}{
		{
			name:            "all namespaces - should process",
			watchNamespaces: "",
			testNamespace:   "production",
			shouldProcess:   true,
			description:     "Empty WATCH_NAMESPACES should process all namespaces",
		},
		{
			name:            "specific namespace - should process",
			watchNamespaces: "production",
			testNamespace:   "production",
			shouldProcess:   true,
			description:     "Should process when namespace matches",
		},
		{
			name:            "specific namespace - should not process",
			watchNamespaces: "production",
			testNamespace:   "staging",
			shouldProcess:   false,
			description:     "Should not process when namespace doesn't match",
		},
		{
			name:            "multiple namespaces - should process first",
			watchNamespaces: "production,staging",
			testNamespace:   "production",
			shouldProcess:   true,
			description:     "Should process when namespace is in list",
		},
		{
			name:            "multiple namespaces - should process second",
			watchNamespaces: "production,staging",
			testNamespace:   "staging",
			shouldProcess:   true,
			description:     "Should process when namespace is in list",
		},
		{
			name:            "multiple namespaces - should not process",
			watchNamespaces: "production,staging",
			testNamespace:   "development",
			shouldProcess:   false,
			description:     "Should not process when namespace not in list",
		},
		{
			name:            "whitespace handling",
			watchNamespaces: " production , staging ",
			testNamespace:   "staging",
			shouldProcess:   true,
			description:     "Should handle whitespace in namespace configuration",
		},
		{
			name:            "empty namespace in list",
			watchNamespaces: "production,,staging",
			testNamespace:   "staging",
			shouldProcess:   true,
			description:     "Should filter out empty namespace entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			originalWatchNamespaces := os.Getenv("WATCH_NAMESPACES")
			os.Setenv("WATCH_NAMESPACES", tt.watchNamespaces)
			defer os.Setenv("WATCH_NAMESPACES", originalWatchNamespaces)

			// Test the namespace parsing logic from runController
			var namespaces []string
			if tt.watchNamespaces != "" {
				namespaces = strings.Split(strings.ReplaceAll(tt.watchNamespaces, " ", ""), ",")
				// Filter out empty strings
				var validNamespaces []string
				for _, ns := range namespaces {
					if ns != "" {
						validNamespaces = append(validNamespaces, ns)
					}
				}
				namespaces = validNamespaces
			}

			// Simulate the filtering logic
			shouldProcess := len(namespaces) == 0 // true if watching all namespaces
			if !shouldProcess {
				for _, ns := range namespaces {
					if ns == tt.testNamespace {
						shouldProcess = true
						break
					}
				}
			}

			assert.Equal(t, tt.shouldProcess, shouldProcess, tt.description)
			t.Logf("✅ %s: %s", tt.name, tt.description)
		})
	}
}

func TestNamespaceFilteringWithIngress(t *testing.T) {
	tests := []struct {
		name            string
		watchNamespaces string
		ingressNS       string
		ingressClass    string
		shouldTrigger   bool
		description     string
	}{
		{
			name:            "all namespaces - nginx ingress should trigger",
			watchNamespaces: "",
			ingressNS:       "production",
			ingressClass:    "nginx",
			shouldTrigger:   true,
			description:     "Should trigger reconciliation for nginx ingress in any namespace when watching all",
		},
		{
			name:            "specific namespace - nginx ingress should trigger",
			watchNamespaces: "production",
			ingressNS:       "production",
			ingressClass:    "nginx",
			shouldTrigger:   true,
			description:     "Should trigger when nginx ingress is in watched namespace",
		},
		{
			name:            "specific namespace - nginx ingress should not trigger",
			watchNamespaces: "production",
			ingressNS:       "staging",
			ingressClass:    "nginx",
			shouldTrigger:   false,
			description:     "Should not trigger when nginx ingress is not in watched namespace",
		},
		{
			name:            "multiple namespaces - nginx ingress should trigger",
			watchNamespaces: "production,staging",
			ingressNS:       "staging",
			ingressClass:    "nginx",
			shouldTrigger:   true,
			description:     "Should trigger when nginx ingress is in one of the watched namespaces",
		},
		{
			name:            "wrong ingress class - should not trigger",
			watchNamespaces: "production",
			ingressNS:       "production",
			ingressClass:    "traefik",
			shouldTrigger:   false,
			description:     "Should not trigger for wrong ingress class regardless of namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			originalWatchNamespaces := os.Getenv("WATCH_NAMESPACES")
			os.Setenv("WATCH_NAMESPACES", tt.watchNamespaces)
			defer os.Setenv("WATCH_NAMESPACES", originalWatchNamespaces)

			// Create test ingress
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: tt.ingressNS,
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: &tt.ingressClass,
					Rules: []networkingv1.IngressRule{
						{
							Host: "test.example.com",
						},
					},
				},
			}

			// Create ingress filter for testing
			filter := ingressfilter.NewFilter("nginx", tt.watchNamespaces)

			// Test IsTargetIngress function
			isTarget := filter.IsTargetIngress(ingress)
			expectedTarget := tt.ingressClass == "nginx"
			assert.Equal(t, expectedTarget, isTarget, "IsTargetIngress check failed for class %s", tt.ingressClass)

			// Test the combined logic: ingress class matching AND namespace filtering
			shouldTriggerPredicate := isTarget && filter.ShouldWatchNamespace(ingress.GetNamespace())

			assert.Equal(t, tt.shouldTrigger, shouldTriggerPredicate, tt.description)
			t.Logf("✅ %s: %s", tt.name, tt.description)
		})
	}
}
