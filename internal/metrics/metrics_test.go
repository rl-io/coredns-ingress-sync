package metrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordReconciliationSuccess(t *testing.T) {
	// Reset counters before test
	ReconciliationTotal.Reset()
	ReconciliationDuration.Reset()

	duration := 0.5
	RecordReconciliationSuccess(duration)

	// Check counter
	metric := &dto.Metric{}
	err := ReconciliationTotal.WithLabelValues("success").Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())

	// For histogram, we just ensure the function executed without error
	// Detailed histogram verification would require more complex setup
	assert.NotPanics(t, func() {
		RecordReconciliationSuccess(0.1)
	})
}

func TestRecordReconciliationError(t *testing.T) {
	// Reset counters before test
	ReconciliationTotal.Reset()
	ReconciliationDuration.Reset()
	ReconciliationErrors.Reset()

	duration := 1.2
	errorType := "ingress_list"
	RecordReconciliationError(duration, errorType)

	// Check main counter
	metric := &dto.Metric{}
	err := ReconciliationTotal.WithLabelValues("error").Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())

	// Check error counter
	errorMetric := &dto.Metric{}
	err = ReconciliationErrors.WithLabelValues(errorType).Write(errorMetric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), errorMetric.GetCounter().GetValue())

	// For histogram, we just ensure the function executed without error
	assert.NotPanics(t, func() {
		RecordReconciliationError(0.1, "test_error")
	})
}

func TestRecordCoreDNSConfigUpdate(t *testing.T) {
	// Reset counters before test
	CoreDNSConfigUpdates.Reset()
	CoreDNSConfigUpdateDuration.Reset()

	duration := 0.3

	// Test successful update
	RecordCoreDNSConfigUpdate(duration, true)

	successMetric := &dto.Metric{}
	err := CoreDNSConfigUpdates.WithLabelValues("success").Write(successMetric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), successMetric.GetCounter().GetValue())

	// Test failed update
	RecordCoreDNSConfigUpdate(duration, false)

	errorMetric := &dto.Metric{}
	err = CoreDNSConfigUpdates.WithLabelValues("error").Write(errorMetric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), errorMetric.GetCounter().GetValue())
}

func TestRecordCoreDNSConfigDrift(t *testing.T) {
	// Reset counter before test
	CoreDNSConfigDrift.Reset()

	driftType := "import_statement"
	RecordCoreDNSConfigDrift(driftType)

	metric := &dto.Metric{}
	err := CoreDNSConfigDrift.WithLabelValues(driftType).Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestUpdateDNSRecordsCount(t *testing.T) {
	count := 5
	UpdateDNSRecordsCount(count)

	metric := &dto.Metric{}
	err := DNSRecordsManaged.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(count), metric.GetGauge().GetValue())
}

func TestUpdateIngressesWatched(t *testing.T) {
	// Reset gauge before test
	IngressesWatched.Reset()

	namespace := "default"
	count := 3
	UpdateIngressesWatched(namespace, count)

	metric := &dto.Metric{}
	err := IngressesWatched.WithLabelValues(namespace).Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(count), metric.GetGauge().GetValue())
}

func TestRecordIngressProcessed(t *testing.T) {
	// Reset counter before test
	IngressesProcessed.Reset()

	namespace := "production"
	action := "add"
	RecordIngressProcessed(namespace, action)

	metric := &dto.Metric{}
	err := IngressesProcessed.WithLabelValues(namespace, action).Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestSetLeaderElectionStatus(t *testing.T) {
	// Test leader status
	SetLeaderElectionStatus(true)

	metric := &dto.Metric{}
	err := LeaderElectionStatus.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetGauge().GetValue())

	// Test non-leader status
	SetLeaderElectionStatus(false)

	err = LeaderElectionStatus.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(0), metric.GetGauge().GetValue())
}

func TestMetricsLabels(t *testing.T) {
	tests := []struct {
		name           string
		metricFunc     func()
		expectedLabels map[string]string
	}{
		{
			name: "reconciliation_with_success_label",
			metricFunc: func() {
				RecordReconciliationSuccess(0.1)
			},
			expectedLabels: map[string]string{"result": "success"},
		},
		{
			name: "ingress_processed_with_namespace_and_action",
			metricFunc: func() {
				RecordIngressProcessed("test-namespace", "update")
			},
			expectedLabels: map[string]string{"namespace": "test-namespace", "action": "update"},
		},
		{
			name: "config_drift_with_type",
			metricFunc: func() {
				RecordCoreDNSConfigDrift("volume_mount")
			},
			expectedLabels: map[string]string{"drift_type": "volume_mount"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset all metrics before each test
			resetAllMetrics()
			
			// Execute the metric function
			tt.metricFunc()
			
			// Verify labels are correctly applied
			// This is a basic test - in real scenarios you'd inspect the metric family
			// For now, we just ensure the function executes without error
			assert.NotPanics(t, tt.metricFunc)
		})
	}
}

func TestMetricsInitialization(t *testing.T) {
	// Test that all metrics are properly initialized and can be used
	assert.NotNil(t, ReconciliationTotal)
	assert.NotNil(t, ReconciliationDuration)
	assert.NotNil(t, ReconciliationErrors)
	assert.NotNil(t, DNSRecordsManaged)
	assert.NotNil(t, CoreDNSConfigUpdates)
	assert.NotNil(t, CoreDNSConfigUpdateDuration)
	assert.NotNil(t, IngressesWatched)
	assert.NotNil(t, IngressesProcessed)
	assert.NotNil(t, LeaderElectionStatus)
	assert.NotNil(t, CoreDNSConfigDrift)
}

func BenchmarkRecordReconciliationSuccess(b *testing.B) {
	duration := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecordReconciliationSuccess(duration)
	}
}

func BenchmarkRecordIngressProcessed(b *testing.B) {
	namespace := "benchmark-namespace"
	action := "update"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecordIngressProcessed(namespace, action)
	}
}

// Helper function to reset all metrics for testing
func resetAllMetrics() {
	ReconciliationTotal.Reset()
	ReconciliationDuration.Reset()
	ReconciliationErrors.Reset()
	CoreDNSConfigUpdates.Reset()
	CoreDNSConfigUpdateDuration.Reset()
	IngressesWatched.Reset()
	IngressesProcessed.Reset()
	CoreDNSConfigDrift.Reset()
	
	// Gauges need to be set to 0
	DNSRecordsManaged.Set(0)
	LeaderElectionStatus.Set(0)
}
