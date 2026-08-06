package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
)

// newSingleClassFilter builds a Filter for a single ingress class, preserving
// the argument order of the original single-class constructor for test brevity.
func newSingleClassFilter(class, watchNamespacesEnv, excludeNamespacesEnv, excludeIngressesEnv, annotationEnabledKey string) *Filter {
	return NewFilter(
		[]config.IngressClassMapping{{IngressClass: class, TargetCNAME: "ingress-nginx-controller.ingress-nginx.svc.cluster.local."}},
		watchNamespacesEnv, excludeNamespacesEnv, excludeIngressesEnv, annotationEnabledKey, "",
	)
}

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
			filter := newSingleClassFilter(tt.ingressClass, tt.watchNamespacesEnv, "", "", "")
			
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
			filter := newSingleClassFilter("nginx", tt.watchNamespacesEnv, "", "", "")
			result := filter.ShouldWatchNamespace(tt.testNamespace)
			assert.Equal(t, tt.shouldWatch, result)
		})
	}
}

func TestIsTargetIngress(t *testing.T) {
	filter := newSingleClassFilter("nginx", "", "", "", "")
	
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
	
	// Test non-Ingress object
	t.Run("non_ingress_object", func(t *testing.T) {
		// Use a Service object as a non-Ingress client.Object
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "test-service"},
		}
		result := filter.IsTargetIngress(service)
		assert.False(t, result)
	})
}

func TestExtractHostnames(t *testing.T) {
	filter := newSingleClassFilter("nginx", "production,staging", "", "", "")
	
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
	filter := newSingleClassFilter("nginx", "", "", "", "") // Watch all namespaces
	
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

func TestExcludeNamespacesAndIngresses(t *testing.T) {
	// Exclude staging namespace and specific ingresses
	filter := newSingleClassFilter("nginx", "production,staging", "staging", "bad-ingress,production/skip-me", "")

	ingresses := []networkingv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ok",
				Namespace: "production",
			},
			Spec: networkingv1.IngressSpec{ IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{{Host: "good.example.com"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ Name: "bad-ingress", Namespace: "production" },
			Spec: networkingv1.IngressSpec{ IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{{Host: "bad.example.com"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ Name: "skip-me", Namespace: "production" },
			Spec: networkingv1.IngressSpec{ IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{{Host: "skip.example.com"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ Name: "in-staging", Namespace: "staging" },
			Spec: networkingv1.IngressSpec{ IngressClassName: stringPtr("nginx"),
				Rules: []networkingv1.IngressRule{{Host: "staging.example.com"}},
			},
		},
	}

	hosts := filter.ExtractHostnames(ingresses)
	// Only 'good.example.com' should remain
	assert.ElementsMatch(t, []string{"good.example.com"}, hosts)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

func TestAnnotationBasedExclusion(t *testing.T) {
	filter := newSingleClassFilter("nginx", "", "", "", "coredns-ingress-sync-enabled")

	// ingress with annotation set to false should be excluded
	ing := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ann-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"coredns-ingress-sync-enabled": "false",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			Rules: []networkingv1.IngressRule{{Host: "ann.example.com"}},
		},
	}
	hosts := filter.ExtractHostnames([]networkingv1.Ingress{ing})
	assert.Len(t, hosts, 0)
}

func TestShouldWatchNamespace_WithExclude_WhenWatchAll(t *testing.T) {
	// When watching all namespaces, exclude list should still be honored
	filter := newSingleClassFilter("nginx", "", "blocked,otherblocked", "", "")

	assert.False(t, filter.ShouldWatchNamespace("blocked"))
	assert.False(t, filter.ShouldWatchNamespace("otherblocked"))
	assert.True(t, filter.ShouldWatchNamespace("allowed"))
}

func TestExcludeIngressesParsingAndMatching(t *testing.T) {
	// Mix of global name and namespace/name with spaces and invalid entries that should be ignored
	filter := newSingleClassFilter("nginx", "", "", "  name-only , ns1/ing1 , ns1/ , /bad ,  , ns2/ing2  ", "")

	mk := func(ns, name string) *networkingv1.Ingress {
		return &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	}

	// Global name exclusion applies in any namespace
	assert.True(t, filter.IsExcludedIngress(mk("default", "name-only")))
	assert.True(t, filter.IsExcludedIngress(mk("ns1", "name-only")))

	// Namespace/name exclusion only in matching namespace
	assert.True(t, filter.IsExcludedIngress(mk("ns1", "ing1")))
	assert.False(t, filter.IsExcludedIngress(mk("ns2", "ing1")))

	assert.True(t, filter.IsExcludedIngress(mk("ns2", "ing2")))
	assert.False(t, filter.IsExcludedIngress(mk("ns1", "ing2")))

	// Non-listed ingress should not be excluded
	assert.False(t, filter.IsExcludedIngress(mk("default", "ok")))
}

func TestAnnotationFalseLikeVariants(t *testing.T) {
	vals := []string{"false", "0", "no", "off", "disabled", "FALSE", "No "}
	for _, v := range vals {
		filter := newSingleClassFilter("nginx", "", "", "", "coredns-ingress-sync-enabled")
		ing := networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ann-" + v,
				Namespace: "default",
				Annotations: map[string]string{
					"coredns-ingress-sync-enabled": v,
				},
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: stringPtr("nginx"),
				Rules:            []networkingv1.IngressRule{{Host: "x.example.com"}},
			},
		}
		// ShouldProcessIngress must return false for all false-like variants
		assert.False(t, filter.ShouldProcessIngress(&ing))
	}

	// True-like or missing annotation should allow processing
	filter := newSingleClassFilter("nginx", "", "", "", "coredns-ingress-sync-enabled")
	ingTrue := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ann-true",
			Namespace: "default",
			Annotations: map[string]string{
				"coredns-ingress-sync-enabled": "true",
			},
		},
		Spec: networkingv1.IngressSpec{IngressClassName: stringPtr("nginx")},
	}
	assert.True(t, filter.ShouldProcessIngress(&ingTrue))

	ingMissing := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ann-missing", Namespace: "default"},
		Spec:       networkingv1.IngressSpec{IngressClassName: stringPtr("nginx")},
	}
	assert.True(t, filter.ShouldProcessIngress(&ingMissing))
}

// multiClassMappings returns an ordered nginx (index 0) + traefik (index 1)
// mapping for multi-class tests.
func multiClassMappings() []config.IngressClassMapping {
	return []config.IngressClassMapping{
		{IngressClass: "nginx", TargetCNAME: "nginx.cluster.local."},
		{IngressClass: "traefik", TargetCNAME: "traefik.cluster.local."},
	}
}

func ingWith(name, namespace, class, host string, annotations map[string]string) networkingv1.Ingress {
	return networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr(class),
			Rules:            []networkingv1.IngressRule{{Host: host}},
		},
	}
}

func TestResolvePriority(t *testing.T) {
	const key = "coredns-ingress-sync-priority"

	t.Run("valid annotation is used", func(t *testing.T) {
		ing := ingWith("a", "default", "traefik", "h.example.com", map[string]string{key: "20"})
		assert.Equal(t, 20, resolvePriority(&ing, key))
	})

	t.Run("annotation with surrounding whitespace is parsed", func(t *testing.T) {
		ing := ingWith("a", "default", "traefik", "h.example.com", map[string]string{key: " 7 "})
		assert.Equal(t, 7, resolvePriority(&ing, key))
	})

	t.Run("negative annotation is honoured", func(t *testing.T) {
		ing := ingWith("a", "default", "traefik", "h.example.com", map[string]string{key: "-3"})
		assert.Equal(t, -3, resolvePriority(&ing, key))
	})

	t.Run("missing annotation defaults to baseline 0", func(t *testing.T) {
		ing := ingWith("a", "default", "traefik", "h.example.com", nil)
		assert.Equal(t, 0, resolvePriority(&ing, key))
	})

	t.Run("non-integer annotation defaults to baseline 0", func(t *testing.T) {
		ing := ingWith("a", "default", "traefik", "h.example.com", map[string]string{key: "high"})
		assert.Equal(t, 0, resolvePriority(&ing, key))
	})

	t.Run("empty annotation key always returns baseline 0", func(t *testing.T) {
		ing := ingWith("a", "default", "traefik", "h.example.com", map[string]string{key: "20"})
		assert.Equal(t, 0, resolvePriority(&ing, ""))
	})
}

func TestIsTargetIngress_MultiClass(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "", "coredns-ingress-sync-priority")

	nginx := ingWith("a", "default", "nginx", "h.example.com", nil)
	traefik := ingWith("b", "default", "traefik", "h.example.com", nil)
	other := ingWith("c", "default", "istio", "h.example.com", nil)

	assert.True(t, filter.IsTargetIngress(&nginx))
	assert.True(t, filter.IsTargetIngress(&traefik))
	assert.False(t, filter.IsTargetIngress(&other))
}

func TestClassCount(t *testing.T) {
	single := NewFilter([]config.IngressClassMapping{{IngressClass: "nginx", TargetCNAME: "nginx.cluster.local."}}, "", "", "", "", "")
	assert.Equal(t, 1, single.ClassCount())

	multi := NewFilter(multiClassMappings(), "", "", "", "", "")
	assert.Equal(t, 2, multi.ClassCount())
}

func TestExtractHostnameMappings_ConfigOrderWins(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "", "coredns-ingress-sync-priority")

	// Both classes serve the same host, no priority annotations.
	// nginx is first in config (index 0 < 1) so it wins by default.
	ingresses := []networkingv1.Ingress{
		ingWith("app-nginx", "default", "nginx", "app.example.com", nil),
		ingWith("app-traefik", "default", "traefik", "app.example.com", nil),
	}

	mappings := filter.ExtractHostnameMappings(ingresses)
	assert.Equal(t, map[string]string{"app.example.com": "nginx.cluster.local."}, mappings)
}

func TestExtractHostnameMappings_AnnotationPromotesTraefik(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "", "coredns-ingress-sync-priority")

	// Traefik ingress is promoted via annotation (20 > nginx's index 0).
	ingresses := []networkingv1.Ingress{
		ingWith("app-nginx", "default", "nginx", "app.example.com", nil),
		ingWith("app-traefik", "default", "traefik", "app.example.com", map[string]string{"coredns-ingress-sync-priority": "20"}),
	}

	mappings := filter.ExtractHostnameMappings(ingresses)
	assert.Equal(t, map[string]string{"app.example.com": "traefik.cluster.local."}, mappings)
}

func TestExtractHostnameMappings_PerHostIndependence(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "", "coredns-ingress-sync-priority")

	// app1 promoted to traefik, app2 left on nginx default.
	ingresses := []networkingv1.Ingress{
		ingWith("app1-nginx", "default", "nginx", "app1.example.com", nil),
		ingWith("app1-traefik", "default", "traefik", "app1.example.com", map[string]string{"coredns-ingress-sync-priority": "20"}),
		ingWith("app2-nginx", "default", "nginx", "app2.example.com", nil),
		ingWith("app2-traefik", "default", "traefik", "app2.example.com", nil),
	}

	mappings := filter.ExtractHostnameMappings(ingresses)
	assert.Equal(t, map[string]string{
		"app1.example.com": "traefik.cluster.local.",
		"app2.example.com": "nginx.cluster.local.",
	}, mappings)
}

func TestExtractHostnameMappings_TieIsDeterministic(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "", "coredns-ingress-sync-priority")

	// Both annotated with the same priority -> genuine tie. The lexicographically
	// smaller CNAME ("nginx..." < "traefik...") is chosen deterministically.
	base := []networkingv1.Ingress{
		ingWith("app-nginx", "default", "nginx", "app.example.com", map[string]string{"coredns-ingress-sync-priority": "5"}),
		ingWith("app-traefik", "default", "traefik", "app.example.com", map[string]string{"coredns-ingress-sync-priority": "5"}),
	}

	got := filter.ExtractHostnameMappings(base)
	assert.Equal(t, map[string]string{"app.example.com": "nginx.cluster.local."}, got)

	// Result must be stable regardless of ingress ordering.
	reversed := []networkingv1.Ingress{base[1], base[0]}
	assert.Equal(t, got, filter.ExtractHostnameMappings(reversed))
}

func TestExtractHostnameMappings_RespectsEnabledAnnotation(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "coredns-ingress-sync-enabled", "coredns-ingress-sync-priority")

	// Traefik would win on priority, but it's disabled, so nginx serves the host.
	ingresses := []networkingv1.Ingress{
		ingWith("app-nginx", "default", "nginx", "app.example.com", nil),
		ingWith("app-traefik", "default", "traefik", "app.example.com", map[string]string{
			"coredns-ingress-sync-priority": "20",
			"coredns-ingress-sync-enabled":  "false",
		}),
	}

	mappings := filter.ExtractHostnameMappings(ingresses)
	assert.Equal(t, map[string]string{"app.example.com": "nginx.cluster.local."}, mappings)
}

func TestIsFalseLike_Direct(t *testing.T) {
	falseVals := []string{"false", "0", "no", "off", "disabled", " FALSE ", "No"}
	for _, v := range falseVals {
		if !isFalseLike(v) {
			t.Errorf("expected %q to be false-like", v)
		}
	}

	trueVals := []string{"true", "1", "yes", "on", "enabled", " yEs ", ""}
	for _, v := range trueVals {
		if isFalseLike(v) {
			t.Errorf("did not expect %q to be false-like", v)
		}
	}
}
