package cacheconfig

import (
	"testing"
)

func TestNewConfigBuilder(t *testing.T) {
	tests := []struct {
		name            string
		watchNamespaces []string
		coreDNSNS       string
	}{
		{
			name:            "empty_watch_namespaces",
			watchNamespaces: []string{},
			coreDNSNS:       "kube-system",
		},
		{
			name:            "single_watch_namespace",
			watchNamespaces: []string{"production"},
			coreDNSNS:       "kube-system",
		},
		{
			name:            "multiple_watch_namespaces",
			watchNamespaces: []string{"production", "staging", "development"},
			coreDNSNS:       "kube-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewConfigBuilder(tt.watchNamespaces, tt.coreDNSNS)
			
			if builder == nil {
				t.Fatal("Expected non-nil ConfigBuilder")
			}
			
			// Verify the builder was created with the correct fields
			if len(builder.watchNamespaces) != len(tt.watchNamespaces) {
				t.Errorf("Expected %d watch namespaces, got %d", len(tt.watchNamespaces), len(builder.watchNamespaces))
			}
			
			if builder.coreDNSNamespace != tt.coreDNSNS {
				t.Errorf("Expected CoreDNS namespace %s, got %s", tt.coreDNSNS, builder.coreDNSNamespace)
			}
		})
	}
}

func TestBuildCacheOptions(t *testing.T) {
	tests := []struct {
		name               string
		watchNamespaces    []string
		coreDNSNS          string
		expectDefaultCache bool
	}{
		{
			name:               "empty_watch_namespaces_should_watch_all",
			watchNamespaces:    []string{},
			coreDNSNS:          "kube-system",
			expectDefaultCache: true,
		},
		{
			name:               "single_namespace_should_scope_cache",
			watchNamespaces:    []string{"production"},
			coreDNSNS:          "kube-system",
			expectDefaultCache: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewConfigBuilder(tt.watchNamespaces, tt.coreDNSNS)
			options := builder.BuildCacheOptions()
			
			// For empty watch namespaces, ByObject should be nil (default cache)
			isDefaultCache := options.ByObject == nil
			
			if tt.expectDefaultCache && !isDefaultCache {
				t.Error("Expected default cache options for watching all namespaces")
			}
			
			if !tt.expectDefaultCache && isDefaultCache {
				t.Error("Expected scoped cache options for specific namespaces")
			}
		})
	}
}

func TestParseNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty_string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single_namespace",
			input:    "production",
			expected: []string{"production"},
		},
		{
			name:     "multiple_namespaces",
			input:    "production,staging,development",
			expected: []string{"production", "staging", "development"},
		},
		{
			name:     "namespaces_with_spaces",
			input:    " production , staging , development ",
			expected: []string{"production", "staging", "development"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseNamespaces(tt.input)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d namespaces, got %d. Expected: %v, Got: %v", len(tt.expected), len(result), tt.expected, result)
				return
			}
			
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected namespace[%d] = %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "simple_split",
			input:    "a,b,c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "split_with_spaces",
			input:    " a , b , c ",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.sep)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d items, got %d. Expected: %v, Got: %v", len(tt.expected), len(result), tt.expected, result)
				return
			}
			
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected item[%d] = %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}
