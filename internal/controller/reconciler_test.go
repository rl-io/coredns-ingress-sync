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

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

// nginxFilter builds a single-class nginx filter for reconciler tests. The
// target CNAME now lives on the filter (via class mappings) rather than the
// CoreDNS manager config.
func nginxFilter(watchNamespaces string) *ingress.Filter {
	return ingress.NewFilter(
		[]config.IngressClassMapping{{IngressClass: "nginx", TargetCNAME: "ingress-nginx.svc.cluster.local."}},
		watchNamespaces, "", "", "", "",
	)
}

func TestNewIngressReconciler(t *testing.T) {
	// Create fake client and scheme
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	
	// Create dependencies
	ingressFilter := nginxFilter("")
	coreDNSConfig := coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
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
	ingressFilter := nginxFilter("")
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
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
	ingressFilter := nginxFilter("") // Looking for nginx, not traefik
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
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
		
	ingressFilter := nginxFilter("")
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
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
	ingressFilter := nginxFilter("production")
		coreDNSConfig := coredns.Config{
			Namespace:            "kube-system",
			ConfigMapName:        "coredns",
			DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
			DynamicConfigKey:     "dynamic.server",
			ImportStatement:      "import /etc/coredns/custom/*.server",
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

// TestReconcile_MultiClassPriorityFlip verifies the end-to-end behaviour: with
// an nginx and traefik Ingress for the same hostname, the generated rewrite
// targets nginx by default, and flipping the priority annotation on the traefik
// Ingress switches the rewrite target to traefik for that hostname only.
func TestReconcile_MultiClassPriorityFlip(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	const host = "app.example.com"
	const nginxCNAME = "nginx.cluster.local."
	const traefikCNAME = "traefik.cluster.local."

	mkIngress := func(name, class string, annotations map[string]string) *networkingv1.Ingress {
		cls := class
		return &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Annotations: annotations},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &cls,
				Rules:            []networkingv1.IngressRule{{Host: host}},
			},
		}
	}

	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Data:       map[string]string{"Corefile": ".:53 {\n    forward . /etc/resolv.conf\n}\n"},
	}

	const priorityKey = "coredns-ingress-sync-priority"
	filter := ingress.NewFilter([]config.IngressClassMapping{
		{IngressClass: "nginx", TargetCNAME: nginxCNAME},
		{IngressClass: "traefik", TargetCNAME: traefikCNAME},
	}, "", "", "", "", priorityKey)

	nginxIng := mkIngress("app-nginx", "nginx", nil)
	traefikIng := mkIngress("app-traefik", "traefik", nil)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nginxIng, traefikIng, coreDNSConfigMap).
		Build()

	coreDNSManager := coredns.NewManager(fakeClient, coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	})

	reconciler := &IngressReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		IngressFilter:  filter,
		CoreDNSManager: coreDNSManager,
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	ctx := context.Background()

	readConfig := func() string {
		var cm corev1.ConfigMap
		if err := fakeClient.Get(ctx, types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, &cm); err != nil {
			t.Fatalf("failed to read dynamic ConfigMap: %v", err)
		}
		return cm.Data["dynamic.server"]
	}

	// 1. No annotations: nginx (config index 0) wins.
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	got := readConfig()
	if !contains(got, "rewrite name exact "+host+" "+nginxCNAME) {
		t.Errorf("expected nginx rewrite by default, got:\n%s", got)
	}
	if contains(got, traefikCNAME) {
		t.Errorf("did not expect traefik rewrite by default, got:\n%s", got)
	}

	// 2. Promote traefik via priority annotation -> rewrite flips to traefik.
	var live networkingv1.Ingress
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "app-traefik", Namespace: "default"}, &live); err != nil {
		t.Fatalf("failed to get traefik ingress: %v", err)
	}
	live.Annotations = map[string]string{priorityKey: "20"}
	if err := fakeClient.Update(ctx, &live); err != nil {
		t.Fatalf("failed to update traefik ingress: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile after flip failed: %v", err)
	}
	got = readConfig()
	if !contains(got, "rewrite name exact "+host+" "+traefikCNAME) {
		t.Errorf("expected traefik rewrite after annotation flip, got:\n%s", got)
	}
	if contains(got, nginxCNAME) {
		t.Errorf("did not expect nginx rewrite after flip, got:\n%s", got)
	}

	// 3. Roll back by removing the annotation -> rewrite returns to nginx.
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "app-traefik", Namespace: "default"}, &live); err != nil {
		t.Fatalf("failed to re-get traefik ingress: %v", err)
	}
	live.Annotations = nil
	if err := fakeClient.Update(ctx, &live); err != nil {
		t.Fatalf("failed to clear traefik annotation: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile after rollback failed: %v", err)
	}
	got = readConfig()
	if !contains(got, "rewrite name exact "+host+" "+nginxCNAME) {
		t.Errorf("expected nginx rewrite after rollback, got:\n%s", got)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(len(substr) == 0 || 
		 strings.Contains(s, substr))
}
