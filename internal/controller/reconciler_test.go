package controller

import (
	"context"
	"os"
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

func TestNewIngressReconciler(t *testing.T) {
	// Create fake client and scheme
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	
	// Create dependencies
	ingressFilter := ingress.NewFilter("nginx", "")
	coreDNSConfig := coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
		TargetCNAME:          "ingress-nginx.svc.cluster.local.",
	}
	coreDNSManager := coredns.NewManager(fakeClient, coreDNSConfig)
	
	reconciler := NewIngressReconciler(fakeClient, scheme, ingressFilter, coreDNSManager)
	
	if reconciler == nil {
		t.Fatal("Expected non-nil reconciler")
	}
	
	if reconciler.Client != fakeClient {
		t.Error("Expected client to be set correctly")
	}
	
	if reconciler.Scheme != scheme {
		t.Error("Expected scheme to be set correctly")
	}
	
	if reconciler.IngressFilter != ingressFilter {
		t.Error("Expected ingress filter to be set correctly")
	}
	
	if reconciler.CoreDNSManager != coreDNSManager {
		t.Error("Expected CoreDNS manager to be set correctly")
	}
}

func TestExtractDomains(t *testing.T) {
	reconciler := &IngressReconciler{}
	
	tests := []struct {
		name     string
		hosts    []string
		expected []string
	}{
		{
			name:     "single_subdomain",
			hosts:    []string{"api.example.com"},
			expected: []string{"example.com"},
		},
		{
			name:     "multiple_subdomains",
			hosts:    []string{"api.example.com", "web.example.com", "admin.example.com"},
			expected: []string{"example.com"},
		},
		{
			name:     "different_domains",
			hosts:    []string{"api.example.com", "web.test.org", "admin.sample.net"},
			expected: []string{"example.com", "test.org", "sample.net"},
		},
		{
			name:     "deep_subdomains",
			hosts:    []string{"api.v1.example.com", "web.public.example.com"},
			expected: []string{"v1.example.com", "public.example.com"},
		},
		{
			name:     "no_subdomains",
			hosts:    []string{"example.com", "test.org"},
			expected: []string{"com", "org"},
		},
		{
			name:     "single_word_hosts",
			hosts:    []string{"localhost", "service"},
			expected: []string{},
		},
		{
			name:     "empty_hosts",
			hosts:    []string{},
			expected: []string{},
		},
		{
			name:     "duplicate_domains",
			hosts:    []string{"api.example.com", "web.example.com", "api.example.com"},
			expected: []string{"example.com"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.extractDomains(tt.hosts)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d domains, got %d", len(tt.expected), len(result))
				return
			}
			
			// Convert result to map for easier checking
			resultMap := make(map[string]bool)
			for _, domain := range result {
				resultMap[domain] = true
			}
			
			// Check that all expected domains are present
			for _, expected := range tt.expected {
				if !resultMap[expected] {
					t.Errorf("Expected domain %s not found in result %v", expected, result)
				}
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	// Set up test environment
	originalHostname := os.Getenv("HOSTNAME")
	defer func() {
		if originalHostname != "" {
			os.Setenv("HOSTNAME", originalHostname)
		} else {
			os.Unsetenv("HOSTNAME")
		}
	}()
	os.Setenv("HOSTNAME", "test-pod-123")
	
	t.Run("reconcile_with_nginx_ingress", func(t *testing.T) {
		// Create fake client with test resources
		scheme := runtime.NewScheme()
		_ = networkingv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		
		ingressClassName := "nginx"
		ingress1 := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress-1",
				Namespace: "default",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &ingressClassName,
				Rules: []networkingv1.IngressRule{
					{
						Host: "api.example.com",
					},
					{
						Host: "web.example.com",
					},
				},
			},
		}
		
		// Create CoreDNS ConfigMap
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"Corefile": `.:53 {
    errors
    health {
        lameduck 5s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
        ttl 30
    }
    prometheus :9153
    forward . /etc/resolv.conf {
        max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}`,
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ingress1, coreDNSConfigMap).
			Build()
		
		// Create dependencies
		ingressFilter := ingress.NewFilter("nginx", "")
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
			TargetCNAME:          "ingress-nginx.svc.cluster.local.",
		}
		coreDNSManager := coredns.NewManager(fakeClient, coreDNSConfig)
		
		reconciler := &IngressReconciler{
			Client:         fakeClient,
			Scheme:         scheme,
			IngressFilter:  ingressFilter,
			CoreDNSManager: coreDNSManager,
		}
		
		// Test reconciliation
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-ingress-1",
				Namespace: "default",
			},
		}
		
		result, err := reconciler.Reconcile(context.Background(), req)
		
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		
		if result.Requeue {
			t.Error("Expected no requeue")
		}
		
		// Verify that dynamic ConfigMap was created
		var dynamicConfigMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(), 
			types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, 
			&dynamicConfigMap)
		if err != nil {
			t.Errorf("Expected dynamic ConfigMap to be created, got error: %v", err)
		}
	})
	
	t.Run("reconcile_with_non_nginx_ingress", func(t *testing.T) {
		// Create fake client with test resources
		scheme := runtime.NewScheme()
		_ = networkingv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		
		ingressClassName := "traefik" // Different ingress class
		ingress1 := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress-1",
				Namespace: "default",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &ingressClassName,
				Rules: []networkingv1.IngressRule{
					{
						Host: "api.example.com",
					},
				},
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ingress1).
			Build()
		
		// Create dependencies
		ingressFilter := ingress.NewFilter("nginx", "") // Looking for nginx, not traefik
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
			TargetCNAME:          "ingress-nginx.svc.cluster.local.",
		}
		coreDNSManager := coredns.NewManager(fakeClient, coreDNSConfig)
		
		reconciler := &IngressReconciler{
			Client:         fakeClient,
			Scheme:         scheme,
			IngressFilter:  ingressFilter,
			CoreDNSManager: coreDNSManager,
		}
		
		// Test reconciliation
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-ingress-1",
				Namespace: "default",
			},
		}
		
		result, err := reconciler.Reconcile(context.Background(), req)
		
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		
		if result.Requeue {
			t.Error("Expected no requeue")
		}
	})
	
	t.Run("reconcile_with_no_hostname", func(t *testing.T) {
		// Unset hostname to test default behavior
		os.Unsetenv("HOSTNAME")
		
		scheme := runtime.NewScheme()
		_ = networkingv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		
		ingressFilter := ingress.NewFilter("nginx", "")
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
			TargetCNAME:          "ingress-nginx.svc.cluster.local.",
		}
		coreDNSManager := coredns.NewManager(fakeClient, coreDNSConfig)
		
		reconciler := &IngressReconciler{
			Client:         fakeClient,
			Scheme:         scheme,
			IngressFilter:  ingressFilter,
			CoreDNSManager: coreDNSManager,
		}
		
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test",
				Namespace: "default",
			},
		}
		
		// Should not error even without hostname
		result, err := reconciler.Reconcile(context.Background(), req)
		
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		
		if result.Requeue {
			t.Error("Expected no requeue")
		}
		
		// Reset hostname for other tests
		os.Setenv("HOSTNAME", "test-pod-123")
	})
	
	t.Run("reconcile_with_namespace_filtering", func(t *testing.T) {
		// Create fake client and scheme
		scheme := runtime.NewScheme()
		_ = networkingv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		
		ingressClassName := "nginx"
		// Ingress in watched namespace
		watchedIngress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "watched-ingress",
				Namespace: "production",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &ingressClassName,
				Rules: []networkingv1.IngressRule{
					{Host: "watched.example.com"},
				},
			},
		}
		
		// Ingress in unwatched namespace  
		unwatchedIngress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unwatched-ingress",
				Namespace: "development",
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &ingressClassName,
				Rules: []networkingv1.IngressRule{
					{Host: "unwatched.example.com"},
				},
			},
		}
		
		// CoreDNS ConfigMap
		coreDNSConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"Corefile": `.:53 {
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
}`,
			},
		}
		
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(watchedIngress, unwatchedIngress, coreDNSConfigMap).
			Build()
		
		// Create filter that only watches production namespace
		ingressFilter := ingress.NewFilter("nginx", "production")
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
			TargetCNAME:          "ingress-nginx.svc.cluster.local.",
		}
		coreDNSManager := coredns.NewManager(fakeClient, coreDNSConfig)
		
		reconciler := NewIngressReconciler(fakeClient, scheme, ingressFilter, coreDNSManager)
		
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-request",
				Namespace: "default",
			},
		}
		
		result, err := reconciler.Reconcile(context.Background(), req)
		
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		
		if result.Requeue {
			t.Error("Expected no requeue")
		}
		
		// Verify that dynamic ConfigMap was created with only watched namespace content
		var dynamicConfigMap corev1.ConfigMap
		err = fakeClient.Get(context.Background(), 
			types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, 
			&dynamicConfigMap)
		
		if err != nil {
			t.Errorf("Expected dynamic ConfigMap to be created, got error: %v", err)
		}
		
		// The dynamic ConfigMap should contain the watched hostname but not the unwatched one
		dynamicConfig := dynamicConfigMap.Data["dynamic.server"]
		if dynamicConfig == "" {
			t.Error("Expected dynamic config to be populated")
		}
		
		// Should contain watched.example.com but not unwatched.example.com
		if !contains(dynamicConfig, "watched.example.com") {
			t.Error("Expected dynamic config to contain watched.example.com")
		}
		if contains(dynamicConfig, "unwatched.example.com") {
			t.Error("Expected dynamic config to NOT contain unwatched.example.com")
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(len(substr) == 0 || 
		 strings.Contains(s, substr))
}
