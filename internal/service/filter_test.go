package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func svc(name, namespace string, annotations map[string]string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
	}
}

func TestFilter_Enabled(t *testing.T) {
	assert.True(t, NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "").Enabled())
}

func TestFilter_IsExcludedService(t *testing.T) {
	filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "excluded-svc,other-ns/other-svc", "", "")

	assert.False(t, filter.IsExcludedService(nil))

	excluded := svc("excluded-svc", "default", nil)
	assert.True(t, filter.IsExcludedService(&excluded))

	excludedByNamespace := svc("other-svc", "other-ns", nil)
	assert.True(t, filter.IsExcludedService(&excludedByNamespace))

	kept := svc("kept-svc", "default", nil)
	assert.False(t, filter.IsExcludedService(&kept))
}

func TestFilter_ShouldProcessService(t *testing.T) {
	t.Run("nil service", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		assert.False(t, filter.ShouldProcessService(nil))
	})

	t.Run("namespace not watched", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "watched-ns", "", "", "", "")
		s := svc("svc", "other-ns", map[string]string{"coredns-ingress-sync-hostname": "svc.example.com"})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("excluded by name", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "excluded-svc", "", "")
		s := svc("excluded-svc", "default", map[string]string{"coredns-ingress-sync-hostname": "svc.example.com"})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("empty hostname annotation key never processes any service", func(t *testing.T) {
		filter := NewFilter("cluster.local", "", "", "", "", "", "")
		s := svc("svc", "default", map[string]string{"coredns-ingress-sync-hostname": "svc.example.com"})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("no hostname annotation is never a candidate", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		s := svc("svc", "default", nil)
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("empty hostname annotation value is never a candidate", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		s := svc("svc", "default", map[string]string{"coredns-ingress-sync-hostname": ""})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("hostname annotation present is processed", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		s := svc("svc", "default", map[string]string{"coredns-ingress-sync-hostname": "svc.example.com"})
		assert.True(t, filter.ShouldProcessService(&s))
	})

	t.Run("hostname annotation with embedded whitespace is rejected", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		s := svc("svc", "default", map[string]string{"coredns-ingress-sync-hostname": "svc.example.com evil.example.com"})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("hostname annotation with embedded newline is rejected", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		s := svc("svc", "default", map[string]string{"coredns-ingress-sync-hostname": "svc.example.com\nrewrite name exact evil.example.com target.example.com."})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("hostname annotation with invalid characters is rejected", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")
		s := svc("svc", "default", map[string]string{"coredns-ingress-sync-hostname": "svc_example!.com"})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("disabled via annotation despite hostname annotation present", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "coredns-ingress-sync-enabled", "")
		s := svc("svc", "default", map[string]string{
			"coredns-ingress-sync-hostname": "svc.example.com",
			"coredns-ingress-sync-enabled":  "false",
		})
		assert.False(t, filter.ShouldProcessService(&s))
	})

	t.Run("annotation present but not false-like is processed", func(t *testing.T) {
		filter := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "coredns-ingress-sync-enabled", "")
		s := svc("svc", "default", map[string]string{
			"coredns-ingress-sync-hostname": "svc.example.com",
			"coredns-ingress-sync-enabled":  "true",
		})
		assert.True(t, filter.ShouldProcessService(&s))
	})
}

func TestFilter_ExtractHostnameCandidates(t *testing.T) {
	f := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")

	services := []corev1.Service{
		svc("student-usage-reports", "blake-staging", map[string]string{
			"coredns-ingress-sync-hostname": "student-usage-reports.blake-staging.com",
		}),
		svc("no-annotation", "blake-staging", nil),
	}

	candidates := f.ExtractHostnameCandidates(services)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "student-usage-reports.blake-staging.com", candidates[0].Host)
	assert.Equal(t, "student-usage-reports.blake-staging.svc.cluster.local.", candidates[0].CNAME)
	assert.Equal(t, 0, candidates[0].ClassIndex)
	assert.Equal(t, "Service:blake-staging/student-usage-reports", candidates[0].Source)
}

func TestFilter_ExtractHostnameCandidates_CustomClusterDomain(t *testing.T) {
	f := NewFilter("cluster.example", "coredns-ingress-sync-hostname", "", "", "", "", "")

	s := svc("api", "default", map[string]string{"coredns-ingress-sync-hostname": "api.example.com"})
	candidates := f.ExtractHostnameCandidates([]corev1.Service{s})

	assert.Len(t, candidates, 1)
	assert.Equal(t, "api.default.svc.cluster.example.", candidates[0].CNAME)
}

func TestFilter_ExtractHostnameCandidates_ClusterDomainTrailingDotStripped(t *testing.T) {
	f := NewFilter("cluster.example.", "coredns-ingress-sync-hostname", "", "", "", "", "")

	s := svc("api", "default", map[string]string{"coredns-ingress-sync-hostname": "api.example.com"})
	candidates := f.ExtractHostnameCandidates([]corev1.Service{s})

	assert.Len(t, candidates, 1)
	assert.Equal(t, "api.default.svc.cluster.example.", candidates[0].CNAME, "a trailing dot on clusterDomain must not double up with the CNAME's own trailing dot")
}

func TestFilter_ExtractHostnameCandidates_RejectsInvalidHostname(t *testing.T) {
	f := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "")

	invalid := svc("bad", "default", map[string]string{"coredns-ingress-sync-hostname": "not a hostname"})
	valid := svc("good", "default", map[string]string{"coredns-ingress-sync-hostname": "good.example.com"})

	candidates := f.ExtractHostnameCandidates([]corev1.Service{invalid, valid})
	assert.Len(t, candidates, 1)
	assert.Equal(t, "good.example.com", candidates[0].Host)
}

func TestFilter_ExtractHostnameCandidates_RespectsNamespaceExcludeAndAnnotation(t *testing.T) {
	f := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "allowed", "", "excluded-svc", "coredns-ingress-sync-enabled", "")

	wrongNamespace := svc("svc-a", "other", map[string]string{"coredns-ingress-sync-hostname": "a.example.com"})
	excluded := svc("excluded-svc", "allowed", map[string]string{"coredns-ingress-sync-hostname": "b.example.com"})
	disabled := svc("svc-c", "allowed", map[string]string{
		"coredns-ingress-sync-hostname": "c.example.com",
		"coredns-ingress-sync-enabled":  "false",
	})
	allowed := svc("svc-d", "allowed", map[string]string{"coredns-ingress-sync-hostname": "d.example.com"})

	candidates := f.ExtractHostnameCandidates([]corev1.Service{wrongNamespace, excluded, disabled, allowed})
	assert.Len(t, candidates, 1)
	assert.Equal(t, "d.example.com", candidates[0].Host)
}

func TestFilter_ExtractHostnameCandidates_PriorityAnnotation(t *testing.T) {
	f := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "", "", "", "", "coredns-ingress-sync-priority")

	s := svc("svc", "default", map[string]string{
		"coredns-ingress-sync-hostname": "example.com",
		"coredns-ingress-sync-priority": "5",
	})

	candidates := f.ExtractHostnameCandidates([]corev1.Service{s})
	assert.Len(t, candidates, 1)
	assert.Equal(t, 5, candidates[0].Priority)
}

func TestFilter_WatchNamespaces(t *testing.T) {
	f := NewFilter("cluster.local", "coredns-ingress-sync-hostname", "ns1,ns2", "", "", "", "")
	assert.False(t, f.WatchesAllNamespaces())
	assert.ElementsMatch(t, []string{"ns1", "ns2"}, f.GetWatchNamespaces())
	assert.True(t, f.ShouldWatchNamespace("ns1"))
	assert.False(t, f.ShouldWatchNamespace("ns3"))
}

func TestValidateClusterDomain(t *testing.T) {
	t.Run("default is valid", func(t *testing.T) {
		assert.NoError(t, ValidateClusterDomain("cluster.local"))
	})

	t.Run("trailing dot is valid", func(t *testing.T) {
		assert.NoError(t, ValidateClusterDomain("cluster.local."))
	})

	t.Run("custom subdomain is valid", func(t *testing.T) {
		assert.NoError(t, ValidateClusterDomain("cluster.example"))
	})

	t.Run("empty string is invalid", func(t *testing.T) {
		assert.Error(t, ValidateClusterDomain(""))
	})

	t.Run("embedded whitespace is invalid", func(t *testing.T) {
		assert.Error(t, ValidateClusterDomain("cluster.local evil"))
	})

	t.Run("embedded newline is invalid", func(t *testing.T) {
		assert.Error(t, ValidateClusterDomain("cluster.local\nrewrite name exact evil.example.com target.example.com."))
	})

	t.Run("invalid characters are rejected", func(t *testing.T) {
		assert.Error(t, ValidateClusterDomain("cluster_local!"))
	})
}
