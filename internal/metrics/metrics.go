package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Reconciliation metrics
	ReconciliationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "coredns_ingress_sync_reconciliation_total",
			Help: "Total number of reconciliation attempts",
		},
		[]string{"result"}, // success, error
	)

	ReconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "coredns_ingress_sync_reconciliation_duration_seconds",
			Help:    "Time spent on reconciliation in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"result"}, // success, error
	)

	ReconciliationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "coredns_ingress_sync_reconciliation_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"error_type"}, // ingress_list, dns_update, config_update
	)

	// DNS management metrics
	DNSRecordsManaged = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "coredns_ingress_sync_dns_records_managed_total",
			Help: "Current number of DNS records being managed",
		},
	)

	CoreDNSConfigUpdates = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "coredns_ingress_sync_coredns_config_updates_total",
			Help: "Total number of CoreDNS configuration updates",
		},
		[]string{"result"}, // success, error
	)

	CoreDNSConfigUpdateDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "coredns_ingress_sync_coredns_config_update_duration_seconds",
			Help:    "Time spent updating CoreDNS configuration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"result"}, // success, error
	)

	// Ingress monitoring metrics
	IngressesWatched = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "coredns_ingress_sync_ingresses_watched_total",
			Help: "Current number of ingresses being watched",
		},
		[]string{"namespace"},
	)

	IngressesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "coredns_ingress_sync_ingresses_processed_total",
			Help: "Total number of ingresses processed",
		},
		[]string{"namespace", "action"}, // add, update, delete
	)

	// System metrics
	LeaderElectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "coredns_ingress_sync_leader_election_status",
			Help: "Leader election status (1 if leader, 0 if not)",
		},
	)

	// CoreDNS defensive configuration metrics
	CoreDNSConfigDrift = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "coredns_ingress_sync_coredns_config_drift_total",
			Help: "Total number of times CoreDNS configuration drift was detected and corrected",
		},
		[]string{"drift_type"}, // import_statement, volume_mount
	)
)

// RecordReconciliationSuccess records a successful reconciliation
func RecordReconciliationSuccess(duration float64) {
	ReconciliationTotal.WithLabelValues("success").Inc()
	ReconciliationDuration.WithLabelValues("success").Observe(duration)
}

// RecordReconciliationError records a failed reconciliation
func RecordReconciliationError(duration float64, errorType string) {
	ReconciliationTotal.WithLabelValues("error").Inc()
	ReconciliationDuration.WithLabelValues("error").Observe(duration)
	ReconciliationErrors.WithLabelValues(errorType).Inc()
}

// RecordCoreDNSConfigUpdate records a CoreDNS configuration update
func RecordCoreDNSConfigUpdate(duration float64, success bool) {
	result := "error"
	if success {
		result = "success"
	}
	CoreDNSConfigUpdates.WithLabelValues(result).Inc()
	CoreDNSConfigUpdateDuration.WithLabelValues(result).Observe(duration)
}

// RecordCoreDNSConfigDrift records detection and correction of configuration drift
func RecordCoreDNSConfigDrift(driftType string) {
	CoreDNSConfigDrift.WithLabelValues(driftType).Inc()
}

// UpdateDNSRecordsCount updates the current count of managed DNS records
func UpdateDNSRecordsCount(count int) {
	DNSRecordsManaged.Set(float64(count))
}

// UpdateIngressesWatched updates the count of watched ingresses per namespace
func UpdateIngressesWatched(namespace string, count int) {
	IngressesWatched.WithLabelValues(namespace).Set(float64(count))
}

// RecordIngressProcessed records processing of an ingress
func RecordIngressProcessed(namespace, action string) {
	IngressesProcessed.WithLabelValues(namespace, action).Inc()
}

// SetLeaderElectionStatus sets the leader election status
func SetLeaderElectionStatus(isLeader bool) {
	if isLeader {
		LeaderElectionStatus.Set(1)
	} else {
		LeaderElectionStatus.Set(0)
	}
}

// init registers all metrics with the controller-runtime metrics registry
func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		ReconciliationTotal,
		ReconciliationDuration,
		ReconciliationErrors,
		DNSRecordsManaged,
		CoreDNSConfigUpdates,
		CoreDNSConfigUpdateDuration,
		IngressesWatched,
		IngressesProcessed,
		LeaderElectionStatus,
		CoreDNSConfigDrift,
	)
}
