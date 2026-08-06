package gatewayapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rl-io/coredns-ingress-sync/internal/config"
)

// newSingleClassFilter builds a Filter for a single GatewayClass, preserving
// the argument order of NewFilter for test brevity.
func newSingleClassFilter(class, watchNamespacesEnv, excludeNamespacesEnv, excludeHTTPRoutesEnv, annotationEnabledKey string) *Filter {
	return NewFilter(
		[]config.GatewayClassMapping{{GatewayClass: class, TargetCNAME: "traefik.traefik.svc.cluster.local."}},
		watchNamespacesEnv, excludeNamespacesEnv, excludeHTTPRoutesEnv, annotationEnabledKey, "",
	)
}

// multiClassMappings returns an ordered traefik (index 0) + istio (index 1)
// mapping for multi-class tests.
func multiClassMappings() []config.GatewayClassMapping {
	return []config.GatewayClassMapping{
		{GatewayClass: "traefik", TargetCNAME: "traefik.cluster.local."},
		{GatewayClass: "istio", TargetCNAME: "istio.cluster.local."},
	}
}

func gw(name, namespace, class string) gatewayv1.Gateway {
	return gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: gatewayv1.ObjectName(class)},
	}
}

func routeWith(name, namespace, parentName, host string, annotations map[string]string) gatewayv1.HTTPRoute {
	return gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: gatewayv1.ObjectName(parentName)},
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(host)},
		},
	}
}

func TestNewFilter(t *testing.T) {
	tests := []struct {
		name               string
		watchNamespacesEnv string
		expectedWatchAll   bool
		expectedNamespaces []string
	}{
		{
			name:               "empty watch namespaces should watch all",
			watchNamespacesEnv: "",
			expectedWatchAll:   true,
			expectedNamespaces: nil,
		},
		{
			name:               "single namespace",
			watchNamespacesEnv: "production",
			expectedWatchAll:   false,
			expectedNamespaces: []string{"production"},
		},
		{
			name:               "multiple namespaces",
			watchNamespacesEnv: "production,staging,development",
			expectedWatchAll:   false,
			expectedNamespaces: []string{"production", "staging", "development"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := newSingleClassFilter("traefik", tt.watchNamespacesEnv, "", "", "")

			assert.Equal(t, tt.expectedWatchAll, filter.WatchesAllNamespaces())
			assert.Equal(t, tt.expectedNamespaces, filter.GetWatchNamespaces())
		})
	}
}

func TestEnabled(t *testing.T) {
	assert.False(t, NewFilter(nil, "", "", "", "", "").Enabled())
	assert.True(t, newSingleClassFilter("traefik", "", "", "", "").Enabled())
}

func TestShouldWatchNamespace(t *testing.T) {
	tests := []struct {
		name               string
		watchNamespacesEnv string
		testNamespace      string
		shouldWatch        bool
	}{
		{
			name:               "watch all namespaces",
			watchNamespacesEnv: "",
			testNamespace:      "any-namespace",
			shouldWatch:        true,
		},
		{
			name:               "namespace in watch list",
			watchNamespacesEnv: "production,staging",
			testNamespace:      "production",
			shouldWatch:        true,
		},
		{
			name:               "namespace not in watch list",
			watchNamespacesEnv: "production,staging",
			testNamespace:      "development",
			shouldWatch:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := newSingleClassFilter("traefik", tt.watchNamespacesEnv, "", "", "")
			assert.Equal(t, tt.shouldWatch, filter.ShouldWatchNamespace(tt.testNamespace))
		})
	}
}

func TestBuildGatewayClassByRef(t *testing.T) {
	gateways := []gatewayv1.Gateway{
		gw("gw1", "default", "traefik"),
		gw("gw2", "other-ns", "istio"),
	}

	refs := BuildGatewayClassByRef(gateways)

	assert.Equal(t, map[types.NamespacedName]string{
		{Namespace: "default", Name: "gw1"}:  "traefik",
		{Namespace: "other-ns", Name: "gw2"}: "istio",
	}, refs)
}

func TestExtractHostnameCandidates_Basic(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "traefik")})

	routes := []gatewayv1.HTTPRoute{
		routeWith("route1", "default", "gw1", "app.example.com", nil),
	}

	candidates := filter.ExtractHostnameCandidates(routes, refs)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "app.example.com", candidates[0].Host)
	assert.Equal(t, "traefik.traefik.svc.cluster.local.", candidates[0].CNAME)
	assert.Equal(t, "default/route1", candidates[0].Source)
}

func TestExtractHostnameCandidates_UnknownGatewayClassIgnored(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	// Gateway references a class we haven't configured.
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "istio")})

	routes := []gatewayv1.HTTPRoute{
		routeWith("route1", "default", "gw1", "app.example.com", nil),
	}

	candidates := filter.ExtractHostnameCandidates(routes, refs)
	assert.Empty(t, candidates)
}

func TestExtractHostnameCandidates_UnknownParentRefIgnored(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	// No Gateway objects at all -- ref map is empty.
	refs := BuildGatewayClassByRef(nil)

	routes := []gatewayv1.HTTPRoute{
		routeWith("route1", "default", "missing-gw", "app.example.com", nil),
	}

	candidates := filter.ExtractHostnameCandidates(routes, refs)
	assert.Empty(t, candidates)
}

func TestExtractHostnameCandidates_ParentRefNamespaceDefaultsToRouteNamespace(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	// Gateway lives in the same namespace as the route; parentRef omits Namespace.
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "team-a", "traefik")})

	route := routeWith("route1", "team-a", "gw1", "app.example.com", nil)
	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)

	assert.Len(t, candidates, 1)
}

func TestExtractHostnameCandidates_ParentRefExplicitNamespace(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	// Gateway lives in a different (shared) namespace than the route.
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("shared-gw", "gateway-ns", "traefik")})

	route := routeWith("route1", "team-a", "shared-gw", "app.example.com", nil)
	route.Spec.ParentRefs[0].Namespace = namespacePtr("gateway-ns")

	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)
	assert.Len(t, candidates, 1)
}

func TestExtractHostnameCandidates_NonGatewayKindIgnored(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "traefik")})

	route := routeWith("route1", "default", "gw1", "app.example.com", nil)
	kind := gatewayv1.Kind("Service")
	route.Spec.ParentRefs[0].Kind = &kind

	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)
	assert.Empty(t, candidates)
}

func TestExtractHostnameCandidates_MismatchedGroupIgnored(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "traefik")})

	route := routeWith("route1", "default", "gw1", "app.example.com", nil)
	group := gatewayv1.Group("some-other-group.example.com")
	route.Spec.ParentRefs[0].Group = &group

	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)
	assert.Empty(t, candidates)
}

func TestExtractHostnameCandidates_MatchingGroupAccepted(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "traefik")})

	route := routeWith("route1", "default", "gw1", "app.example.com", nil)
	group := gatewayv1.Group(gatewayv1.GroupName)
	route.Spec.ParentRefs[0].Group = &group

	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)
	assert.Len(t, candidates, 1)
}

func TestExtractHostnameCandidates_EmptyHostnameIgnored(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "traefik")})

	route := routeWith("route1", "default", "gw1", "app.example.com", nil)
	route.Spec.Hostnames = append(route.Spec.Hostnames, gatewayv1.Hostname(""))

	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)
	assert.Len(t, candidates, 1)
}

func TestIsExcludedHTTPRoute_NilRoute(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "excluded-route", "")
	assert.False(t, filter.IsExcludedHTTPRoute(nil))
}

func TestShouldProcessHTTPRoute_NilRoute(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "")
	assert.False(t, filter.ShouldProcessHTTPRoute(nil))
}

func TestExtractHostnameCandidates_NamespaceFilteringAndExclusion(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "production,staging", "staging", "bad-route,production/skip-me", "", "")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{
		gw("gw1", "production", "traefik"),
		gw("gw2", "staging", "traefik"),
		gw("gw3", "development", "traefik"),
	})

	routes := []gatewayv1.HTTPRoute{
		routeWith("ok", "production", "gw1", "good.example.com", nil),
		routeWith("bad-route", "production", "gw1", "bad.example.com", nil),
		routeWith("skip-me", "production", "gw1", "skip.example.com", nil),
		routeWith("in-staging", "staging", "gw2", "staging.example.com", nil),
		routeWith("in-dev", "development", "gw3", "dev.example.com", nil),
	}

	candidates := filter.ExtractHostnameCandidates(routes, refs)
	hosts := make([]string, 0, len(candidates))
	for _, c := range candidates {
		hosts = append(hosts, c.Host)
	}
	assert.ElementsMatch(t, []string{"good.example.com"}, hosts)
}

func TestExtractHostnameCandidates_AnnotationDisabled(t *testing.T) {
	filter := newSingleClassFilter("traefik", "", "", "", "coredns-ingress-sync-enabled")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{gw("gw1", "default", "traefik")})

	route := routeWith("route1", "default", "gw1", "app.example.com", map[string]string{
		"coredns-ingress-sync-enabled": "false",
	})

	candidates := filter.ExtractHostnameCandidates([]gatewayv1.HTTPRoute{route}, refs)
	assert.Empty(t, candidates)
}

func TestExtractHostnameCandidates_PriorityAnnotation(t *testing.T) {
	filter := NewFilter(multiClassMappings(), "", "", "", "", "coredns-ingress-sync-priority")
	refs := BuildGatewayClassByRef([]gatewayv1.Gateway{
		gw("gw-traefik", "default", "traefik"),
		gw("gw-istio", "default", "istio"),
	})

	routes := []gatewayv1.HTTPRoute{
		routeWith("r1", "default", "gw-traefik", "app.example.com", nil),
		routeWith("r2", "default", "gw-istio", "app.example.com", map[string]string{
			"coredns-ingress-sync-priority": "20",
		}),
	}

	candidates := filter.ExtractHostnameCandidates(routes, refs)
	assert.Len(t, candidates, 2)

	byClass := map[int]int{}
	for _, c := range candidates {
		byClass[c.ClassIndex] = c.Priority
	}
	// traefik is index 0 with baseline priority; istio is index 1 promoted to 20.
	assert.Equal(t, 0, byClass[0])
	assert.Equal(t, 20, byClass[1])
}

func namespacePtr(ns string) *gatewayv1.Namespace {
	n := gatewayv1.Namespace(ns)
	return &n
}
