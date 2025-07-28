package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewFilter(t *testing.T) {
	tests := []struct {
		name              string
		ingressClass      string
		watchNamespacesEnv string
		expectedWatchAll  bool
		expectedNamespaces []string
	}{
		{
			name:              "empty watch namespaces should watch all",
			ingressClass:      "nginx",
			watchNamespacesEnv: "",
			expectedWatchAll:  true,
			expectedNamespaces: nil,
		},
		{
			name:              "single namespace",
			ingressClass:      "nginx",
			watchNamespacesEnv: "production",
			expectedWatchAll:  false,
			expectedNamespaces: []string{"production"},
		},
		{
			name:              "multiple namespaces",
			ingressClass:      "nginx", 
			watchNamespacesEnv: "production,staging,development",
			expectedWatchAll:  false,
			expectedNamespaces: []string{"production", "staging", "development"},
		},
		{
			name:              "namespaces with spaces",
			ingressClass:      "nginx",
			watchNamespacesEnv: "production, staging , development",
			expectedWatchAll:  false,
			expectedNamespaces: []string{"production", "staging", "development"},
		},
		{
			name:              "namespaces with empty entries",
			ingressClass:      "nginx",
			watchNamespacesEnv: "production,,staging,",
			expectedWatchAll:  false,
			expectedNamespaces: []string{"production", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFilter(tt.ingressClass, tt.watchNamespacesEnv)
			
			assert.Equal(t, tt.expectedWatchAll, filter.WatchesAllNamespaces())
			assert.Equal(t, tt.expectedNamespaces, filter.GetWatchNamespaces())
		})
	}
}

func TestShouldWatchNamespace(t *testing.T) {
	tests := []struct {
		name              string
		watchNamespacesEnv string
		testNamespace     string
		shouldWatch       bool
	}{
		{
			name:              "watch all namespaces",
			watchNamespacesEnv: "",
			testNamespace:     "any-namespace",
			shouldWatch:       true,
		},
		{
			name:              "namespace in watch list",
			watchNamespacesEnv: "production,staging",
			testNamespace:     "production",
			shouldWatch:       true,
		},
		{
			name:              "namespace not in watch list",
			watchNamespacesEnv: "production,staging",
			testNamespace:     "development",
			shouldWatch:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFilter("nginx", tt.watchNamespacesEnv)
			result := filter.ShouldWatchNamespace(tt.testNamespace)
			assert.Equal(t, tt.shouldWatch, result)
		})
	}
}

func TestIsTargetIngress(t *testing.T) {
	filter := NewFilter("nginx", "")
	
	tests := []struct {
		name           string
		ingress        *networkingv1.Ingress
		expectedResult bool
	}{
		{
			name: "matching ingress class",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: stringPtr("nginx"),
				},
			},
			expectedResult: true,
		},
		{
			name: "non-matching ingress class",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: stringPtr("traefik"),
				},
			},
			expectedResult: false,
		},
		{
			name: "nil ingress class",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: nil,
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsTargetIngress(tt.ingress)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestExtractHostnames(t *testing.T) {
	filter := NewFilter("nginx", "production,staging")
	
	ingresses := []networkingv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress1",
				Namespace: "production",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{
					{Host: "app1.example.com"},
					{Host: "app2.example.com"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress2",
				Namespace: "staging",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{
					{Host: "app3.example.com"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress3",
				Namespace: "development", // Not in watch list
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{
					{Host: "app4.example.com"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress4",
				Namespace: "production",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("traefik"), // Wrong class
				Rules: []networkingv1.IngressRule{
					{Host: "app5.example.com"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress5",
				Namespace: "production",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{
					{Host: "app1.example.com"}, // Duplicate host
				},
			},
		},
	}

	hostnames := filter.ExtractHostnames(ingresses)
	
	// Should include hosts from production and staging namespaces with nginx class
	expectedHosts := []string{"app1.example.com", "app2.example.com", "app3.example.com"}
	assert.ElementsMatch(t, expectedHosts, hostnames)
	
	// Should not include duplicates
	assert.Len(t, hostnames, 3)
}

func TestExtractHostnamesWatchAll(t *testing.T) {
	filter := NewFilter("nginx", "") // Watch all namespaces
	
	ingresses := []networkingv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress1",
				Namespace: "any-namespace",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{
					{Host: "app1.example.com"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress2",
				Namespace: "another-namespace",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("traefik"), // Wrong class
				Rules: []networkingv1.IngressRule{
					{Host: "app2.example.com"},
				},
			},
		},
	}

	hostnames := filter.ExtractHostnames(ingresses)
	
	// Should include only nginx class ingresses from any namespace
	expectedHosts := []string{"app1.example.com"}
	assert.ElementsMatch(t, expectedHosts, hostnames)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
