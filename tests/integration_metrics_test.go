package main

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dto "github.com/prometheus/client_model/go"

	"github.com/rl-io/coredns-ingress-sync/internal/coredns"
	"github.com/rl-io/coredns-ingress-sync/internal/controller"
	"github.com/rl-io/coredns-ingress-sync/internal/ingress"
	"github.com/rl-io/coredns-ingress-sync/internal/metrics"
)

// TestMetricsIntegration tests the metrics integration with the reconciler
func TestMetricsIntegration(t *testing.T) {
	// Reset all metrics
	resetAllMetrics()

	// Create fake client and scheme
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	// Create test objects
	testIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &[]string{"nginx"}[0], // Use spec.ingressClassName instead of annotations
			Rules: []networkingv1.IngressRule{
				{
					Host: "app.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &[]networkingv1.PathType{networkingv1.PathTypePrefix}[0],
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "test-service",
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	testCoreDNSConfigMap := &corev1.ConfigMap{
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
		WithObjects(testIngress, testCoreDNSConfigMap).
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
		VolumeName:           "coredns-ingress-sync-volume",
	}
	coreDNSManager := coredns.NewManager(fakeClient, coreDNSConfig)

	reconciler := controller.NewIngressReconciler(fakeClient, scheme, ingressFilter, coreDNSManager)

	// Perform reconciliation
	ctx := context.Background()
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "global-ingress-reconcile",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// Verify metrics were recorded
	t.Run("reconciliation_success_metrics", func(t *testing.T) {
		metric := &dto.Metric{}
		err := metrics.ReconciliationTotal.WithLabelValues("success").Write(metric)
		require.NoError(t, err)
		assert.Equal(t, float64(1), metric.GetCounter().GetValue())
	})

	t.Run("dns_records_managed_metrics", func(t *testing.T) {
		metric := &dto.Metric{}
		err := metrics.DNSRecordsManaged.Write(metric)
		require.NoError(t, err)
		// Should have 1 DNS record for app.example.com
		assert.Equal(t, float64(1), metric.GetGauge().GetValue())
	})

	t.Run("coredns_config_update_metrics", func(t *testing.T) {
		metric := &dto.Metric{}
		err := metrics.CoreDNSConfigUpdates.WithLabelValues("success").Write(metric)
		require.NoError(t, err)
		// Should have at least 1 successful config update
		assert.GreaterOrEqual(t, metric.GetCounter().GetValue(), float64(1))
	})

	t.Run("ingresses_watched_metrics", func(t *testing.T) {
		metric := &dto.Metric{}
		err := metrics.IngressesWatched.WithLabelValues("default").Write(metric)
		require.NoError(t, err)
		// Should be watching 1 ingress in default namespace
		assert.Equal(t, float64(1), metric.GetGauge().GetValue())
	})
}

// Helper function to reset all metrics for testing
func resetAllMetrics() {
	metrics.ReconciliationTotal.Reset()
	metrics.ReconciliationDuration.Reset()
	metrics.ReconciliationErrors.Reset()
	metrics.CoreDNSConfigUpdates.Reset()
	metrics.CoreDNSConfigUpdateDuration.Reset()
	metrics.IngressesWatched.Reset()
	metrics.IngressesProcessed.Reset()
	metrics.CoreDNSConfigDrift.Reset()
	
	// Gauges need to be set to 0
	metrics.DNSRecordsManaged.Set(0)
	metrics.LeaderElectionStatus.Set(0)
}
