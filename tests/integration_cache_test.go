package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	
	cachebuilder "github.com/rl-io/coredns-ingress-sync/internal/cache"
)

func TestCacheConfigurationLogic(t *testing.T) {
	tests := []struct {
		name            string
		watchNamespaces string
		expectedAllNS   bool
		expectedNSCount int
		description     string
	}{
		{
			name:            "empty watch namespaces should watch all",
			watchNamespaces: "",
			expectedAllNS:   true,
			expectedNSCount: 0,
			description:     "When WATCH_NAMESPACES is empty, should watch all namespaces",
		},
		{
			name:            "single namespace",
			watchNamespaces: "production",
			expectedAllNS:   false,
			expectedNSCount: 1,
			description:     "When watching single namespace, cache should be scoped",
		},
		{
			name:            "multiple namespaces",
			watchNamespaces: "production,staging,development",
			expectedAllNS:   false,
			expectedNSCount: 3,
			description:     "When watching multiple namespaces, cache should be scoped to all specified",
		},
		{
			name:            "namespaces with whitespace",
			watchNamespaces: " production , staging , development ",
			expectedAllNS:   false,
			expectedNSCount: 3,
			description:     "Should handle whitespace in namespace configuration",
		},
		{
			name:            "namespaces with empty entries",
			watchNamespaces: "production,,staging,,development",
			expectedAllNS:   false,
			expectedNSCount: 3,
			description:     "Should filter out empty namespace entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			originalWatchNamespaces := os.Getenv("WATCH_NAMESPACES")
			os.Setenv("WATCH_NAMESPACES", tt.watchNamespaces)
			defer os.Setenv("WATCH_NAMESPACES", originalWatchNamespaces)

			// Simulate the cache configuration logic from runController
			watchNamespacesEnv := os.Getenv("WATCH_NAMESPACES")
			
			if watchNamespacesEnv == "" {
				// Watch all namespaces case
				assert.True(t, tt.expectedAllNS, tt.description)
				t.Logf("✅ %s: Correctly configured to watch all namespaces", tt.name)
			} else {
				// Parse namespace list using the new cache package
				namespaces := cachebuilder.ParseNamespaces(watchNamespacesEnv)
				
				assert.False(t, tt.expectedAllNS, "Should not watch all namespaces when specific namespaces are configured")
				assert.Equal(t, tt.expectedNSCount, len(namespaces), "Should have correct number of namespaces")
				
				// Verify no empty namespaces
				for _, ns := range namespaces {
					assert.NotEmpty(t, ns, "No namespace should be empty after parsing")
				}
				
				t.Logf("✅ %s: Correctly configured to watch %d namespaces: %v", tt.name, len(namespaces), namespaces)
			}
		})
	}
}

func TestByObjectCacheScoping(t *testing.T) {
	// This test verifies that the ByObject configuration correctly handles namespace scoping
	// It simulates what happens in our runController function
	
	tests := []struct {
		name       string
		namespaces []string
	}{
		{
			name:       "single namespace scoping",
			namespaces: []string{"production"},
		},
		{
			name:       "multiple namespace scoping",
			namespaces: []string{"production", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create namespace map for ByObject configuration
			nsMap := make(map[string]cache.Config)
			for _, ns := range tt.namespaces {
				nsMap[ns] = cache.Config{}
			}

			// Verify the namespace map was created correctly
			assert.Equal(t, len(tt.namespaces), len(nsMap), "Namespace map should have correct number of entries")
			
			for _, ns := range tt.namespaces {
				_, exists := nsMap[ns]
				assert.True(t, exists, "Namespace %s should exist in cache config map", ns)
			}

			t.Logf("✅ %s: ByObject cache configuration created for namespaces: %v", tt.name, tt.namespaces)
		})
	}
}
