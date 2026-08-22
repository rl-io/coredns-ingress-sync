package traefik

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	traefikv1alpha1 "github.com/rl-io/coredns-ingress-sync/internal/traefik/v1alpha1"
)

func ir(name, namespace string, matches ...string) traefikv1alpha1.IngressRoute {
	routes := make([]traefikv1alpha1.Route, len(matches))
	for i, m := range matches {
		routes[i] = traefikv1alpha1.Route{Match: m}
	}
	return traefikv1alpha1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       traefikv1alpha1.IngressRouteSpec{Routes: routes},
	}
}

func TestFilter_Enabled(t *testing.T) {
	assert.False(t, NewFilter("", "", "", "", "", "").Enabled())
	assert.True(t, NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "", "").Enabled())
}

func TestFilter_IsExcludedIngressRoute(t *testing.T) {
	filter := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "excluded-route,other-ns/other-route", "", "")

	assert.False(t, filter.IsExcludedIngressRoute(nil))

	excluded := ir("excluded-route", "default")
	assert.True(t, filter.IsExcludedIngressRoute(&excluded))

	excludedByNamespace := ir("other-route", "other-ns")
	assert.True(t, filter.IsExcludedIngressRoute(&excludedByNamespace))

	kept := ir("kept-route", "default")
	assert.False(t, filter.IsExcludedIngressRoute(&kept))
}

func TestFilter_ShouldProcessIngressRoute(t *testing.T) {
	t.Run("nil route", func(t *testing.T) {
		filter := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "", "")
		assert.False(t, filter.ShouldProcessIngressRoute(nil))
	})

	t.Run("namespace not watched", func(t *testing.T) {
		filter := NewFilter("traefik.traefik.svc.cluster.local.", "watched-ns", "", "", "", "")
		route := ir("route", "other-ns")
		assert.False(t, filter.ShouldProcessIngressRoute(&route))
	})

	t.Run("excluded by name", func(t *testing.T) {
		filter := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "excluded-route", "", "")
		route := ir("excluded-route", "default")
		assert.False(t, filter.ShouldProcessIngressRoute(&route))
	})

	t.Run("disabled via annotation", func(t *testing.T) {
		filter := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "coredns-ingress-sync-enabled", "")
		route := ir("route", "default")
		route.Annotations = map[string]string{"coredns-ingress-sync-enabled": "false"}
		assert.False(t, filter.ShouldProcessIngressRoute(&route))
	})

	t.Run("annotation present but not false-like is processed", func(t *testing.T) {
		filter := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "coredns-ingress-sync-enabled", "")
		route := ir("route", "default")
		route.Annotations = map[string]string{"coredns-ingress-sync-enabled": "true"}
		assert.True(t, filter.ShouldProcessIngressRoute(&route))
	})

	t.Run("no annotation key configured is processed", func(t *testing.T) {
		filter := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "", "")
		route := ir("route", "default")
		assert.True(t, filter.ShouldProcessIngressRoute(&route))
	})
}

func TestExtractHosts(t *testing.T) {
	tests := []struct {
		name  string
		match string
		want  []string
	}{
		{
			name:  "bare host and path, real-world readingeggs health check",
			match: "Host(`readingeggs-staging.blake-staging.com`) && Path(`/up`)",
			want:  []string{"readingeggs-staging.blake-staging.com"},
		},
		{
			name:  "host and path prefix, real-world assignments-api route",
			match: "Host(`api.blake-staging.com`) && PathPrefix(`/assignments-api`)",
			want:  []string{"api.blake-staging.com"},
		},
		{
			name:  "multi-host OR within one Host() call",
			match: "Host(`a.example.com`,`b.example.com`) && PathPrefix(`/x`)",
			want:  []string{"a.example.com", "b.example.com"},
		},
		{
			name:  "duplicate host across matchers dedupes",
			match: "Host(`a.example.com`) || Host(`a.example.com`)",
			want:  []string{"a.example.com"},
		},
		{
			name:  "case-insensitive matcher name",
			match: "host(`a.example.com`)",
			want:  []string{"a.example.com"},
		},
		{
			name:  "no Host matcher present",
			match: "PathPrefix(`/api`)",
			want:  nil,
		},
		{
			name:  "empty match",
			match: "",
			want:  nil,
		},
		{
			name:  "whitespace inside Host call",
			match: "Host( `a.example.com` )",
			want:  []string{"a.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractHosts(tt.match))
		})
	}
}

func TestFilter_ExtractHostnameCandidates(t *testing.T) {
	f := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "", "")

	routes := []traefikv1alpha1.IngressRoute{
		ir("readingeggs-staging-route-up-staging", "blake-staging",
			"Host(`readingeggs-staging.blake-staging.com`) && Path(`/up`)"),
		ir("assignments-api-staging", "blake-staging",
			"Host(`api.blake-staging.com`) && PathPrefix(`/assignments-api`)"),
	}

	candidates := f.ExtractHostnameCandidates(routes)
	require := map[string]bool{
		"readingeggs-staging.blake-staging.com": false,
		"api.blake-staging.com":                 false,
	}
	for _, c := range candidates {
		assert.Equal(t, "traefik.traefik.svc.cluster.local.", c.CNAME)
		assert.Equal(t, 0, c.ClassIndex)
		if _, ok := require[c.Host]; ok {
			require[c.Host] = true
		}
	}
	for host, found := range require {
		assert.True(t, found, "expected candidate for host %s", host)
	}
}

func TestFilter_ExtractHostnameCandidates_DedupesAcrossRoutesInOneIngressRoute(t *testing.T) {
	f := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "", "")

	route := ir("multi-route", "default",
		"Host(`example.com`) && PathPrefix(`/a`)",
		"Host(`example.com`) && PathPrefix(`/b`)",
	)

	candidates := f.ExtractHostnameCandidates([]traefikv1alpha1.IngressRoute{route})
	assert.Len(t, candidates, 1)
	assert.Equal(t, "example.com", candidates[0].Host)
}

func TestFilter_ExtractHostnameCandidates_RespectsNamespaceExcludeAndAnnotation(t *testing.T) {
	f := NewFilter("traefik.traefik.svc.cluster.local.", "allowed", "", "excluded-route", "coredns-ingress-sync-enabled", "")

	wrongNamespace := ir("route-a", "other", "Host(`a.example.com`)")
	excluded := ir("excluded-route", "allowed", "Host(`b.example.com`)")
	disabled := ir("route-c", "allowed", "Host(`c.example.com`)")
	disabled.Annotations = map[string]string{"coredns-ingress-sync-enabled": "false"}
	allowed := ir("route-d", "allowed", "Host(`d.example.com`)")

	candidates := f.ExtractHostnameCandidates([]traefikv1alpha1.IngressRoute{wrongNamespace, excluded, disabled, allowed})
	assert.Len(t, candidates, 1)
	assert.Equal(t, "d.example.com", candidates[0].Host)
}

func TestFilter_ExtractHostnameCandidates_PriorityAnnotation(t *testing.T) {
	f := NewFilter("traefik.traefik.svc.cluster.local.", "", "", "", "", "coredns-ingress-sync-priority")

	route := ir("route-a", "default", "Host(`example.com`)")
	route.Annotations = map[string]string{"coredns-ingress-sync-priority": "5"}

	candidates := f.ExtractHostnameCandidates([]traefikv1alpha1.IngressRoute{route})
	require := 1
	assert.Len(t, candidates, require)
	assert.Equal(t, 5, candidates[0].Priority)
}

func TestFilter_WatchNamespaces(t *testing.T) {
	f := NewFilter("traefik.traefik.svc.cluster.local.", "ns1,ns2", "", "", "", "")
	assert.False(t, f.WatchesAllNamespaces())
	assert.ElementsMatch(t, []string{"ns1", "ns2"}, f.GetWatchNamespaces())
	assert.True(t, f.ShouldWatchNamespace("ns1"))
	assert.False(t, f.ShouldWatchNamespace("ns3"))
}
