package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/gatewayapi"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
	"github.com/rl-io/coredns-ingress-sync/internal/traefik"
	traefikv1alpha1 "github.com/rl-io/coredns-ingress-sync/internal/traefik/v1alpha1"
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

	reconciler := NewIngressReconciler(fakeClient, scheme, ingressFilter, nil, nil, coreDNSManager)

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

		reconciler := NewIngressReconciler(fakeClient, scheme, ingressFilter, nil, nil, coreDNSManager)

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

// TestReconcile_IngressAndGatewayAPIMerge verifies the reconciler merges
// Ingress and Gateway API (Gateway + HTTPRoute) candidates into one set of
// rewrite rules, and that Ingress wins a same-host tiebreak by default (per
// the offset-ClassIndex design).
func TestReconcile_IngressAndGatewayAPIMerge(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	const nginxCNAME = "nginx.cluster.local."
	const traefikCNAME = "traefik.traefik.svc.cluster.local."
	const sharedHost = "shared.example.com"
	const gatewayOnlyHost = "gw-only.example.com"

	ingressClassName := "nginx"
	sharedIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules:            []networkingv1.IngressRule{{Host: sharedHost}},
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik-gw", Namespace: "default"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "traefik"},
	}
	sharedRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "traefik-gw"}},
			},
			Hostnames: []gatewayv1.Hostname{sharedHost},
		},
	}
	gatewayOnlyRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "gw-only-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "traefik-gw"}},
			},
			Hostnames: []gatewayv1.Hostname{gatewayOnlyHost},
		},
	}

	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Data:       map[string]string{"Corefile": ".:53 {\n    forward . /etc/resolv.conf\n}\n"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sharedIngress, gw, sharedRoute, gatewayOnlyRoute, coreDNSConfigMap).
		Build()

	ingressFilter := ingress.NewFilter([]config.IngressClassMapping{
		{IngressClass: "nginx", TargetCNAME: nginxCNAME},
	}, "", "", "", "", "")
	gatewayFilter := gatewayapi.NewFilter([]config.GatewayClassMapping{
		{GatewayClass: "traefik", TargetCNAME: traefikCNAME},
	}, "", "", "", "", "")

	coreDNSManager := coredns.NewManager(fakeClient, coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	})

	reconciler := NewIngressReconciler(fakeClient, scheme, ingressFilter, gatewayFilter, nil, coreDNSManager)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var cm corev1.ConfigMap
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, &cm); err != nil {
		t.Fatalf("failed to read dynamic ConfigMap: %v", err)
	}
	got := cm.Data["dynamic.server"]

	if !contains(got, "rewrite name exact "+sharedHost+" "+nginxCNAME) {
		t.Errorf("expected Ingress to win the shared-host tiebreak, got:\n%s", got)
	}
	if contains(got, "rewrite name exact "+sharedHost+" "+traefikCNAME) {
		t.Errorf("did not expect the HTTPRoute's CNAME to win the shared-host tiebreak, got:\n%s", got)
	}
	if !contains(got, "rewrite name exact "+gatewayOnlyHost+" "+traefikCNAME) {
		t.Errorf("expected the Gateway-API-only host to be present, got:\n%s", got)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(len(substr) == 0 ||
			strings.Contains(s, substr))
}

// noGatewayAPIListClient wraps a client.Client and fails the test if List is
// ever called with a Gateway API list type. It embeds the client.Client
// interface so all other methods delegate to the wrapped client unchanged.
type noGatewayAPIListClient struct {
	client.Client
	t *testing.T
}

func (c *noGatewayAPIListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	switch list.(type) {
	case *gatewayv1.GatewayList, *gatewayv1.HTTPRouteList:
		c.t.Fatalf("unexpected List call against Gateway API type %T when GatewayFilter is nil", list)
	}
	return c.Client.List(ctx, list, opts...)
}

// TestReconcile_GatewayFilterNil_NoGatewayAPIListCalls is the load-bearing
// regression test for Gateway API being purely additive: when GatewayFilter
// is nil (the default, unconfigured state), Reconcile must never List
// Gateway or HTTPRoute objects, since those CRDs may not even be installed
// in a pure-Ingress cluster.
func TestReconcile_GatewayFilterNil_NoGatewayAPIListCalls(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	ingressClassName := "nginx"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules:            []networkingv1.IngressRule{{Host: "app.example.com"}},
		},
	}
	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Data: map[string]string{
			"Corefile": `.:53 {
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
}`,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ing, coreDNSConfigMap).Build()
	wrappedClient := &noGatewayAPIListClient{Client: fakeClient, t: t}

	ingressFilter := nginxFilter("")
	coreDNSConfig := coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	}
	coreDNSManager := coredns.NewManager(wrappedClient, coreDNSConfig)

	reconciler := NewIngressReconciler(wrappedClient, scheme, ingressFilter, nil, nil, coreDNSManager)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"},
	}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if reconciler.GatewayFilter != nil {
		t.Fatal(fmt.Sprintf("expected GatewayFilter to be nil, got: %v", reconciler.GatewayFilter))
	}
}

// noTraefikIngressRouteListClient wraps a client.Client and fails the test if
// List is ever called with a Traefik IngressRoute list type. It embeds the
// client.Client interface so all other methods delegate to the wrapped
// client unchanged.
type noTraefikIngressRouteListClient struct {
	client.Client
	t *testing.T
}

func (c *noTraefikIngressRouteListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	switch list.(type) {
	case *traefikv1alpha1.IngressRouteList:
		c.t.Fatalf("unexpected List call against Traefik IngressRoute type %T when TraefikFilter is nil", list)
	}
	return c.Client.List(ctx, list, opts...)
}

// TestReconcile_TraefikFilterNil_NoIngressRouteListCalls is the load-bearing
// regression test for Traefik IngressRoute support being purely additive:
// when TraefikFilter is nil (the default, unconfigured state), Reconcile
// must never List IngressRoute objects, since that CRD may not even be
// installed in a pure-Ingress (or Ingress+Gateway-API) cluster.
func TestReconcile_TraefikFilterNil_NoIngressRouteListCalls(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
	_ = traefikv1alpha1.AddToScheme(scheme)

	ingressClassName := "nginx"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules:            []networkingv1.IngressRule{{Host: "app.example.com"}},
		},
	}
	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Data: map[string]string{
			"Corefile": `.:53 {
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
}`,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ing, coreDNSConfigMap).Build()
	wrappedClient := &noTraefikIngressRouteListClient{Client: fakeClient, t: t}

	ingressFilter := nginxFilter("")
	coreDNSConfig := coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	}
	coreDNSManager := coredns.NewManager(wrappedClient, coreDNSConfig)

	reconciler := NewIngressReconciler(wrappedClient, scheme, ingressFilter, nil, nil, coreDNSManager)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"},
	}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if reconciler.TraefikFilter != nil {
		t.Fatal(fmt.Sprintf("expected TraefikFilter to be nil, got: %v", reconciler.TraefikFilter))
	}
}

// TestReconcile_IngressGatewayAPIAndTraefikMerge extends the
// Ingress+Gateway-API merge test with a third source, a Traefik IngressRoute
// that also claims the shared host, confirming the 3-way ClassIndex offset
// keeps Ingress winning the default tiebreak over both Gateway API and
// Traefik IngressRoute.
func TestReconcile_IngressGatewayAPIAndTraefikMerge(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
	_ = traefikv1alpha1.AddToScheme(scheme)

	const nginxCNAME = "nginx.cluster.local."
	const gatewayCNAME = "gateway-traefik.traefik.svc.cluster.local."
	const traefikCNAME = "traefik.traefik.svc.cluster.local."
	const sharedHost = "shared.example.com"
	const traefikOnlyHost = "ir-only.example.com"

	ingressClassName := "nginx"
	sharedIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules:            []networkingv1.IngressRule{{Host: sharedHost}},
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik-gw", Namespace: "default"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "traefik"},
	}
	sharedRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "traefik-gw"}},
			},
			Hostnames: []gatewayv1.Hostname{sharedHost},
		},
	}

	sharedAndOnlyRoute := &traefikv1alpha1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-ingressroute", Namespace: "default"},
		Spec: traefikv1alpha1.IngressRouteSpec{
			Routes: []traefikv1alpha1.Route{
				{Match: "Host(`" + sharedHost + "`)"},
				{Match: "Host(`" + traefikOnlyHost + "`)"},
			},
		},
	}

	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Data:       map[string]string{"Corefile": ".:53 {\n    forward . /etc/resolv.conf\n}\n"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sharedIngress, gw, sharedRoute, sharedAndOnlyRoute, coreDNSConfigMap).
		Build()

	ingressFilter := ingress.NewFilter([]config.IngressClassMapping{
		{IngressClass: "nginx", TargetCNAME: nginxCNAME},
	}, "", "", "", "", "")
	gatewayFilter := gatewayapi.NewFilter([]config.GatewayClassMapping{
		{GatewayClass: "traefik", TargetCNAME: gatewayCNAME},
	}, "", "", "", "", "")
	traefikFilter := traefik.NewFilter(traefikCNAME, "", "", "", "", "")

	coreDNSManager := coredns.NewManager(fakeClient, coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	})

	reconciler := NewIngressReconciler(fakeClient, scheme, ingressFilter, gatewayFilter, traefikFilter, coreDNSManager)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var cm corev1.ConfigMap
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, &cm); err != nil {
		t.Fatalf("failed to read dynamic ConfigMap: %v", err)
	}
	got := cm.Data["dynamic.server"]

	if !contains(got, "rewrite name exact "+sharedHost+" "+nginxCNAME) {
		t.Errorf("expected Ingress to win the shared-host tiebreak over Gateway API and IngressRoute, got:\n%s", got)
	}
	if contains(got, "rewrite name exact "+sharedHost+" "+gatewayCNAME) {
		t.Errorf("did not expect the HTTPRoute's CNAME to win the shared-host tiebreak, got:\n%s", got)
	}
	if contains(got, "rewrite name exact "+sharedHost+" "+traefikCNAME) {
		t.Errorf("did not expect the IngressRoute's CNAME to win the shared-host tiebreak, got:\n%s", got)
	}
	if !contains(got, "rewrite name exact "+traefikOnlyHost+" "+traefikCNAME) {
		t.Errorf("expected the IngressRoute-only host to be present, got:\n%s", got)
	}
}

// gatewayAPIListErrorClient fails List calls against Gateway/HTTPRoute list
// types, optionally scoped to a single namespace (matching the per-namespace
// listing branch of extractGatewayCandidates). An empty failNamespace fails
// the targeted type regardless of namespace (the watch-all branch).
type gatewayAPIListErrorClient struct {
	client.Client
	failGatewayList   bool
	failHTTPRouteList bool
	failNamespace     string
	err               error
}

func (g *gatewayAPIListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	matchesNamespace := func() bool {
		if g.failNamespace == "" {
			return true
		}
		listOpts := &client.ListOptions{}
		for _, o := range opts {
			o.ApplyToList(listOpts)
		}
		return listOpts.Namespace == g.failNamespace
	}

	switch list.(type) {
	case *gatewayv1.GatewayList:
		if g.failGatewayList && matchesNamespace() {
			return g.err
		}
	case *gatewayv1.HTTPRouteList:
		if g.failHTTPRouteList && matchesNamespace() {
			return g.err
		}
	}
	return g.Client.List(ctx, list, opts...)
}

// gatewayReconcilerFixture builds a scheme + fake client + reconciler wired
// with the given Gateway/HTTPRoute objects (plus a base CoreDNS ConfigMap),
// wrapping the client with wrap if non-nil.
func gatewayReconcilerFixture(t *testing.T, watchNamespaces string, objs []client.Object, wrap func(client.Client) client.Client) (*IngressReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	coreDNSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Data:       map[string]string{"Corefile": ".:53 {\n    forward . /etc/resolv.conf\n}\n"},
	}
	allObjs := append([]client.Object{coreDNSConfigMap}, objs...)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(allObjs...).Build()
	var c client.Client = fakeClient
	if wrap != nil {
		c = wrap(fakeClient)
	}

	gatewayFilter := gatewayapi.NewFilter([]config.GatewayClassMapping{
		{GatewayClass: "traefik", TargetCNAME: "traefik.traefik.svc.cluster.local."},
	}, watchNamespaces, "", "", "", "")

	coreDNSManager := coredns.NewManager(c, coredns.Config{
		Namespace:            "kube-system",
		ConfigMapName:        "coredns",
		DynamicConfigMapName: "coredns-ingress-sync-rewrite-rules",
		DynamicConfigKey:     "dynamic.server",
		ImportStatement:      "import /etc/coredns/custom/*.server",
	})

	reconciler := NewIngressReconciler(c, scheme, nginxFilter(watchNamespaces), gatewayFilter, nil, coreDNSManager)
	return reconciler, c
}

func TestReconcile_GatewayNamespaceScoped(t *testing.T) {
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik-gw", Namespace: "ns-b"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "traefik"},
	}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "ns-b"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "traefik-gw"}},
			},
			Hostnames: []gatewayv1.Hostname{"scoped.example.com"},
		},
	}

	reconciler, c := gatewayReconcilerFixture(t, "ns-a,ns-b", []client.Object{gw, route}, nil)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, &cm); err != nil {
		t.Fatalf("failed to read dynamic ConfigMap: %v", err)
	}
	if !contains(cm.Data["dynamic.server"], "rewrite name exact scoped.example.com traefik.traefik.svc.cluster.local.") {
		t.Errorf("expected namespace-scoped HTTPRoute host to be present, got:\n%s", cm.Data["dynamic.server"])
	}
}

func TestReconcile_GatewayListError_WatchAll(t *testing.T) {
	reconciler, _ := gatewayReconcilerFixture(t, "", nil, func(c client.Client) client.Client {
		return &gatewayAPIListErrorClient{Client: c, failGatewayList: true, err: fmt.Errorf("gateway list boom")}
	})

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err == nil {
		t.Fatal("expected reconcile to return an error when the watch-all Gateway List fails")
	}
}

func TestReconcile_HTTPRouteListError_WatchAll(t *testing.T) {
	reconciler, _ := gatewayReconcilerFixture(t, "", nil, func(c client.Client) client.Client {
		return &gatewayAPIListErrorClient{Client: c, failHTTPRouteList: true, err: fmt.Errorf("httproute list boom")}
	})

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err == nil {
		t.Fatal("expected reconcile to return an error when the watch-all HTTPRoute List fails")
	}
}

func TestReconcile_GatewayListError_PerNamespace_Continues(t *testing.T) {
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik-gw", Namespace: "ns-b"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "traefik"},
	}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "ns-b"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "traefik-gw"}},
			},
			Hostnames: []gatewayv1.Hostname{"scoped.example.com"},
		},
	}

	reconciler, c := gatewayReconcilerFixture(t, "ns-a,ns-b", []client.Object{gw, route}, func(c client.Client) client.Client {
		return &gatewayAPIListErrorClient{Client: c, failGatewayList: true, failNamespace: "ns-a", err: fmt.Errorf("gateway list boom in ns-a")}
	})

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("expected reconcile to continue past a single namespace's List error, got: %v", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, &cm); err != nil {
		t.Fatalf("failed to read dynamic ConfigMap: %v", err)
	}
	if !contains(cm.Data["dynamic.server"], "rewrite name exact scoped.example.com traefik.traefik.svc.cluster.local.") {
		t.Errorf("expected ns-b's HTTPRoute host to still resolve despite ns-a's Gateway List failing, got:\n%s", cm.Data["dynamic.server"])
	}
}

func TestReconcile_HTTPRouteListError_PerNamespace_Continues(t *testing.T) {
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik-gw", Namespace: "ns-b"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "traefik"},
	}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "ns-b"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "traefik-gw"}},
			},
			Hostnames: []gatewayv1.Hostname{"scoped.example.com"},
		},
	}

	reconciler, c := gatewayReconcilerFixture(t, "ns-a,ns-b", []client.Object{gw, route}, func(c client.Client) client.Client {
		return &gatewayAPIListErrorClient{Client: c, failHTTPRouteList: true, failNamespace: "ns-a", err: fmt.Errorf("httproute list boom in ns-a")}
	})

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "global-ingress-reconcile", Namespace: "default"}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("expected reconcile to continue past a single namespace's List error, got: %v", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: "coredns-ingress-sync-rewrite-rules", Namespace: "kube-system"}, &cm); err != nil {
		t.Fatalf("failed to read dynamic ConfigMap: %v", err)
	}
	if !contains(cm.Data["dynamic.server"], "rewrite name exact scoped.example.com traefik.traefik.svc.cluster.local.") {
		t.Errorf("expected ns-b's HTTPRoute host to still resolve despite ns-a's HTTPRoute List failing, got:\n%s", cm.Data["dynamic.server"])
	}
}
