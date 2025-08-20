package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	
	ingressfilter "github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

func TestIsTargetIngressFunction(t *testing.T) {
	tests := []struct {
		name         string
		ingressClass string
		expected     bool
	}{
		{
			name:         "nginx ingress should be target",
			ingressClass: "nginx",
			expected:     true,
		},
		{
			name:         "traefik ingress should not be target",
			ingressClass: "traefik",
			expected:     false,
		},
		{
			name:         "empty ingress class should not be target",
			ingressClass: "",
			expected:     false,
		},
		{
			name:         "other ingress class should not be target",
			ingressClass: "istio",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
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

			// Create ingress filter and test IsTargetIngress
			filter := ingressfilter.NewFilter("nginx", "", "", "", "")
			result := filter.IsTargetIngress(ingress)
			assert.Equal(t, tt.expected, result, "IsTargetIngress for class '%s' should be %v", tt.ingressClass, tt.expected)
		})
	}
}

func TestExtractHostnamesFunction(t *testing.T) {
	tests := []struct {
		name      string
		hosts     []string
		expected  []string
	}{
		{
			name:     "single host",
			hosts:    []string{"api.example.com"},
			expected: []string{"api.example.com"},
		},
		{
			name:     "multiple hosts",
			hosts:    []string{"api.example.com", "web.example.com"},
			expected: []string{"api.example.com", "web.example.com"},
		},
		{
			name:     "duplicate hosts should be deduplicated",
			hosts:    []string{"api.example.com", "api.example.com", "web.example.com"},
			expected: []string{"api.example.com", "web.example.com"},
		},
		{
			name:     "empty hosts",
			hosts:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ingresses []networkingv1.Ingress
			
			for _, host := range tt.hosts {
				className := "nginx"
				ingress := networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ingress-" + host,
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						IngressClassName: &className,
						Rules: []networkingv1.IngressRule{
							{
								Host: host,
							},
						},
					},
				}
				ingresses = append(ingresses, ingress)
			}

			// Create ingress filter and test ExtractHostnames
			filter := ingressfilter.NewFilter("nginx", "", "", "", "")
			hostnames := filter.ExtractHostnames(ingresses)
			
			// Convert to map for easy comparison since order doesn't matter
			expectedMap := make(map[string]bool)
			for _, h := range tt.expected {
				expectedMap[h] = true
			}
			
			actualMap := make(map[string]bool)
			for _, h := range hostnames {
				actualMap[h] = true
			}
			
			assert.Equal(t, expectedMap, actualMap, "extractHostnames should return expected hosts")
			assert.Equal(t, len(tt.expected), len(hostnames), "extractHostnames should return correct number of hosts")
		})
	}
}
