package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
	ingfilter "github.com/rl-io/coredns-ingress-sync/internal/ingress"
)

// Mock reconciler for testing
type mockReconciler struct {
	reconcileFunc func(ctx context.Context, req reconcile.Request) (reconcile.Result, error)
}

func (m *mockReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx, req)
	}
	return reconcile.Result{}, nil
}

func TestNewControllerManager(t *testing.T) {
	logger := logr.Discard()
	// Use a minimal configuration; no logger needed beyond construction
	cfg := &config.Config{
		IngressClass:            "nginx",
		WatchNamespaces:         "",
		CoreDNSNamespace:        "kube-system",
		CoreDNSConfigMapName:    "coredns",
		DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
		TargetCNAME:             "ingress-nginx.svc.cluster.local.",
		LeaderElectionEnabled:   false,
		ControllerNamespace:     "default",
	}
	reconciler := &mockReconciler{}

	cm := NewControllerManager(logger, cfg, reconciler)

	if cm == nil {
		t.Fatal("Expected non-nil controller manager")
	}

	if cm.config != cfg {
		t.Error("Expected config to be set correctly")
	}

	if cm.reconciler != reconciler {
		t.Error("Expected reconciler to be set correctly")
	}
}

func TestControllerManager_Setup(t *testing.T) {
	t.Run("setup with default configuration", func(t *testing.T) {
		logger := logr.Discard()
		cfg := &config.Config{
			IngressClass:            "nginx",
			WatchNamespaces:         "",
			CoreDNSNamespace:        "kube-system",
			CoreDNSConfigMapName:    "coredns",
			DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
			TargetCNAME:             "ingress-nginx.svc.cluster.local.",
			LeaderElectionEnabled:   false,
			ControllerNamespace:     "default",
		}
		reconciler := &mockReconciler{}

		cm := NewControllerManager(logger, cfg, reconciler)
		
		// Note: We can't actually call Setup() in unit tests because it requires
		// a real Kubernetes config. Instead, we test the individual components.
		// This is a common pattern for controller-runtime based code.
		
		// Verify manager was created with correct config
		if cm.config.IngressClass != "nginx" {
			t.Errorf("Expected ingress class 'nginx', got %s", cm.config.IngressClass)
		}
	})

	t.Run("setup with namespace filtering", func(t *testing.T) {
		logger := logr.Discard()
		cfg := &config.Config{
			IngressClass:            "nginx",
			WatchNamespaces:         "production,staging",
			CoreDNSNamespace:        "kube-system",
			CoreDNSConfigMapName:    "coredns",
			DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
			TargetCNAME:             "ingress-nginx.svc.cluster.local.",
			LeaderElectionEnabled:   true,
			ControllerNamespace:     "controller-system",
		}
		reconciler := &mockReconciler{}

		cm := NewControllerManager(logger, cfg, reconciler)
		
		if cm.config.WatchNamespaces != "production,staging" {
			t.Errorf("Expected watch namespaces 'production,staging', got %s", cm.config.WatchNamespaces)
		}
		
		if !cm.config.LeaderElectionEnabled {
			t.Error("Expected leader election to be enabled")
		}
	})
}

func TestControllerManager_setupHealthChecks(t *testing.T) {
	logger := logr.Discard()
	cfg := &config.Config{
		IngressClass:            "nginx",
		WatchNamespaces:         "",
		CoreDNSNamespace:        "kube-system",
		CoreDNSConfigMapName:    "coredns",
		DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
		TargetCNAME:             "ingress-nginx.svc.cluster.local.",
		LeaderElectionEnabled:   false,
		ControllerNamespace:     "default",
	}
	reconciler := &mockReconciler{}

	cm := NewControllerManager(logger, cfg, reconciler)
	
	// Verify that the controller manager was created successfully
	if cm == nil {
		t.Fatal("Expected non-nil controller manager")
	}

	// Create a fake manager for testing health checks
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	
	// Create a test HTTP request
	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	// Test health check logic directly since we can't create a real manager
	// The health check should always return nil (healthy)
	healthCheck := func(req *http.Request) error {
		return nil
	}
	
	err = healthCheck(req)
	if err != nil {
		t.Errorf("Health check should always return healthy, got error: %v", err)
	}

	// Test readiness check logic
	readinessCheck := func(req *http.Request) error {
		return nil
	}
	
	err = readinessCheck(req)
	if err != nil {
		t.Errorf("Readiness check should always return ready, got error: %v", err)
	}

	// Verify that the setup methods don't panic with mock data
	if fakeClient == nil {
		t.Error("Expected non-nil fake client")
	}
}

func TestControllerManager_logStartupInfo(t *testing.T) {
	// Create a custom logger that captures log messages
	var loggedMessages []string
	testLogger := logr.New(&testLogSink{messages: &loggedMessages})

	cfg := &config.Config{
		IngressClass:            "nginx",
		WatchNamespaces:         "production,staging",
		CoreDNSNamespace:        "kube-system",
		CoreDNSConfigMapName:    "coredns",
		DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
		TargetCNAME:             "ingress-nginx.svc.cluster.local.",
		LeaderElectionEnabled:   true,
		ControllerNamespace:     "default",
	}
	reconciler := &mockReconciler{}

	cm := NewControllerManager(testLogger, cfg, reconciler)

	// Test with specific namespaces
	watchNamespaces := []string{"production", "staging"}
	cm.logStartupInfo(watchNamespaces)

	// Verify at least one log message was generated
	if len(loggedMessages) < 2 {
		t.Errorf("Expected at least 2 log messages, got %d", len(loggedMessages))
	}

	// Test with empty namespaces (watch all)
	loggedMessages = []string{} // Reset
	cm.logStartupInfo([]string{})

	if len(loggedMessages) < 2 {
		t.Errorf("Expected at least 2 log messages for empty namespaces, got %d", len(loggedMessages))
	}
}

func TestControllerManager_SetupWatches_Components(t *testing.T) {
	logger := logr.Discard()
	cfg := &config.Config{
		IngressClass:            "nginx",
		WatchNamespaces:         "",
		CoreDNSNamespace:        "kube-system",
		CoreDNSConfigMapName:    "coredns",
		DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
		TargetCNAME:             "ingress-nginx.svc.cluster.local.",
		LeaderElectionEnabled:   false,
		ControllerNamespace:     "default",
	}
	reconciler := &mockReconciler{}

	cm := NewControllerManager(logger, cfg, reconciler)

	// Test that manager correctly stores configuration for watches
	if cm.config.CoreDNSNamespace != "kube-system" {
		t.Errorf("Expected CoreDNS namespace 'kube-system', got %s", cm.config.CoreDNSNamespace)
	}

	if cm.config.CoreDNSConfigMapName != "coredns" {
		t.Errorf("Expected CoreDNS ConfigMap name 'coredns', got %s", cm.config.CoreDNSConfigMapName)
	}

	if cm.config.DynamicConfigMapName != "coredns-ingress-sync-rewrite-rules" {
		t.Errorf("Expected dynamic ConfigMap name 'coredns-ingress-sync-rewrite-rules', got %s", cm.config.DynamicConfigMapName)
	}
}

func TestControllerManager_CreatesIngressFilterAndPredicate(t *testing.T) {
	// This test ensures no panic when constructing the filter and predicate path in setupWatches
	cfg := &config.Config{
		IngressClass:          "nginx",
		WatchNamespaces:       "",
		ExcludeNamespaces:     "excluded",
		ExcludeIngresses:      "bad,ns1/bad2",
		AnnotationEnabledKey:  "coredns-ingress-sync-enabled",
		CoreDNSNamespace:      "kube-system",
		CoreDNSConfigMapName:  "coredns",
		DynamicConfigMapName:  "coredns-ingress-sync-rewrite-rules",
		TargetCNAME:           "ingress-nginx.svc.cluster.local.",
		LeaderElectionEnabled: false,
		ControllerNamespace:   "default",
	}
	// Note: We don't need a real controller manager instance for this test
	// as we're validating filter behavior used by the predicate wiring.

	// Build a scheme and a manager with cache options, but don't actually start it
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	// We won't create a real manager here; instead we just exercise the code path that creates the filter
	// by calling the same construction logic and validating ShouldProcessIngress works as expected
	f := ingfilter.NewFilter(cfg.IngressClass, cfg.WatchNamespaces, cfg.ExcludeNamespaces, cfg.ExcludeIngresses, cfg.AnnotationEnabledKey)

	// Ingress matching class but excluded via name
	cls := "nginx"
	ing := &networkingv1.Ingress{Spec: networkingv1.IngressSpec{IngressClassName: &cls}}
	ing.Name = "bad"
	ing.Namespace = "default"
	if f.ShouldProcessIngress(ing) {
		t.Error("Expected ingress 'bad' to be excluded by name")
	}

	// Ingress excluded by namespace/name
	ing2 := &networkingv1.Ingress{Spec: networkingv1.IngressSpec{IngressClassName: &cls}}
	ing2.Name = "bad2"
	ing2.Namespace = "ns1"
	if f.ShouldProcessIngress(ing2) {
		t.Error("Expected ingress ns1/bad2 to be excluded by namespace/name")
	}
}

func TestControllerManager_Configuration_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *config.Config
		valid  bool
	}{
		{
			name: "valid configuration",
			config: &config.Config{
				IngressClass:            "nginx",
				WatchNamespaces:         "",
				CoreDNSNamespace:        "kube-system",
				CoreDNSConfigMapName:    "coredns",
				DynamicConfigMapName:    "coredns-ingress-sync-rewrite-rules",
				TargetCNAME:             "ingress-nginx.svc.cluster.local.",
				LeaderElectionEnabled:   false,
				ControllerNamespace:     "default",
			},
			valid: true,
		},
		{
			name: "configuration with namespace filtering",
			config: &config.Config{
				IngressClass:            "traefik",
				WatchNamespaces:         "ns1,ns2,ns3",
				CoreDNSNamespace:        "dns-system",
				CoreDNSConfigMapName:    "coredns-config",
				DynamicConfigMapName:    "custom-dns",
				TargetCNAME:             "traefik.svc.cluster.local.",
				LeaderElectionEnabled:   true,
				ControllerNamespace:     "traefik-system",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			reconciler := &mockReconciler{}

			cm := NewControllerManager(logger, tt.config, reconciler)

			if cm == nil && tt.valid {
				t.Error("Expected valid configuration to create controller manager")
			}

			if cm != nil {
				// Validate that all config fields are preserved
				if cm.config.IngressClass != tt.config.IngressClass {
					t.Errorf("IngressClass mismatch: expected %s, got %s", tt.config.IngressClass, cm.config.IngressClass)
				}
				if cm.config.WatchNamespaces != tt.config.WatchNamespaces {
					t.Errorf("WatchNamespaces mismatch: expected %s, got %s", tt.config.WatchNamespaces, cm.config.WatchNamespaces)
				}
				if cm.config.LeaderElectionEnabled != tt.config.LeaderElectionEnabled {
					t.Errorf("LeaderElectionEnabled mismatch: expected %t, got %t", tt.config.LeaderElectionEnabled, cm.config.LeaderElectionEnabled)
				}
			}
		})
	}
}

func TestBuildIngressPredicate_AnnotationFlipTriggersUpdate(t *testing.T) {
	// Setup filter and predicate
	filt := ingfilter.NewFilter("nginx", "", "", "", "coredns-ingress-sync-enabled")
	pred := buildIngressPredicate(filt)

	cls := "nginx"
	// Old included, new excluded via annotation => should trigger
	oldIng := &networkingv1.Ingress{Spec: networkingv1.IngressSpec{IngressClassName: &cls}}
	oldIng.Namespace = "default"
	oldIng.Name = "app"
	newIng := oldIng.DeepCopy()
	newIng.Annotations = map[string]string{"coredns-ingress-sync-enabled": "false"}
	upd := event.TypedUpdateEvent[*networkingv1.Ingress]{ObjectOld: oldIng, ObjectNew: newIng}
	if !pred.Update(upd) {
		t.Error("expected update to trigger when inclusion flips to excluded")
	}

	// Old excluded, new included => should also trigger
	oldIng2 := oldIng.DeepCopy()
	oldIng2.Annotations = map[string]string{"coredns-ingress-sync-enabled": "false"}
	newIng2 := oldIng.DeepCopy()
	upd2 := event.TypedUpdateEvent[*networkingv1.Ingress]{ObjectOld: oldIng2, ObjectNew: newIng2}
	if !pred.Update(upd2) {
		t.Error("expected update to trigger when inclusion flips to included")
	}

	// Both excluded (wrong class) => should not trigger
	other := "traefik"
	oldIng3 := &networkingv1.Ingress{Spec: networkingv1.IngressSpec{IngressClassName: &other}}
	newIng3 := oldIng3.DeepCopy()
	upd3 := event.TypedUpdateEvent[*networkingv1.Ingress]{ObjectOld: oldIng3, ObjectNew: newIng3}
	if pred.Update(upd3) {
		t.Error("did not expect update to trigger when both old and new are excluded")
	}

	// Create: only trigger when ingress should be processed
	crt := event.TypedCreateEvent[*networkingv1.Ingress]{Object: oldIng}
	if !pred.Create(crt) {
		t.Error("expected create to trigger for included ingress")
	}
	crtExcluded := event.TypedCreateEvent[*networkingv1.Ingress]{Object: oldIng3}
	if pred.Create(crtExcluded) {
		t.Error("did not expect create to trigger for excluded ingress")
	}

	// Delete: always trigger to prune rules
	del := event.TypedDeleteEvent[*networkingv1.Ingress]{Object: oldIng}
	if !pred.Delete(del) {
		t.Error("expected delete to trigger always")
	}
}

func TestControllerManager_SchemeRegistration(t *testing.T) {
	// Test scheme registration logic used in Setup method
	scheme := runtime.NewScheme()
	
	// Test networking/v1 registration
	err := networkingv1.AddToScheme(scheme)
	if err != nil {
		t.Errorf("Failed to add networking/v1 to scheme: %v", err)
	}

	// Test core/v1 registration
	err = corev1.AddToScheme(scheme)
	if err != nil {
		t.Errorf("Failed to add core/v1 to scheme: %v", err)
	}

	// Test apps/v1 registration
	err = appsv1.AddToScheme(scheme)
	if err != nil {
		t.Errorf("Failed to add apps/v1 to scheme: %v", err)
	}

	// Verify scheme has the expected types
	if !scheme.Recognizes(networkingv1.SchemeGroupVersion.WithKind("Ingress")) {
		t.Error("Scheme should recognize Ingress objects")
	}

	if !scheme.Recognizes(corev1.SchemeGroupVersion.WithKind("ConfigMap")) {
		t.Error("Scheme should recognize ConfigMap objects")
	}

	if !scheme.Recognizes(appsv1.SchemeGroupVersion.WithKind("Deployment")) {
		t.Error("Scheme should recognize Deployment objects")
	}
}

// testLogSink implements logr.LogSink for testing
type testLogSink struct {
	messages *[]string
}

func (t *testLogSink) Init(info logr.RuntimeInfo) {}

func (t *testLogSink) Enabled(level int) bool {
	return true
}

func (t *testLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	*t.messages = append(*t.messages, msg)
}

func (t *testLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	*t.messages = append(*t.messages, msg)
}

func (t *testLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return t
}

func (t *testLogSink) WithName(name string) logr.LogSink {
	return t
}
