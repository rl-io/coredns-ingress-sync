package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// The two fixtures below mirror real-world IngressRoute manifests: a health
// check route matched on a bare Host()+Path(), and a route with PathPrefix,
// middlewares, and an empty tls block. Both include fields (services,
// entryPoints, middlewares, tls, observability, priority) this package
// doesn't model, to confirm they decode without error and are simply
// discarded.
// These fixtures use concatenated interpreted strings rather than a raw
// (backtick-delimited) string literal because the YAML content itself
// contains literal backticks (Traefik's router-rule DSL), which would
// otherwise prematurely terminate a backtick-quoted Go string.
const readingEggsUpRouteYAML = "apiVersion: traefik.io/v1alpha1\n" +
	"kind: IngressRoute\n" +
	"metadata:\n" +
	"  name: readingeggs-staging-route-up-staging\n" +
	"  namespace: blake-staging\n" +
	"spec:\n" +
	"  entryPoints:\n" +
	"    - websecure\n" +
	"  routes:\n" +
	"    - kind: Rule\n" +
	"      match: \"Host(`readingeggs-staging.blake-staging.com`) && Path(`/up`)\"\n" +
	"      priority: 100\n" +
	"      services:\n" +
	"        - name: readingeggs-staging\n" +
	"          port: 80\n"

const assignmentsAPIRouteYAML = "apiVersion: traefik.io/v1alpha1\n" +
	"kind: IngressRoute\n" +
	"metadata:\n" +
	"  name: assignments-api-staging\n" +
	"  namespace: blake-staging\n" +
	"spec:\n" +
	"  entryPoints:\n" +
	"    - websecure\n" +
	"  routes:\n" +
	"    - kind: Rule\n" +
	"      match: \"Host(`api.blake-staging.com`) && PathPrefix(`/assignments-api`)\"\n" +
	"      middlewares:\n" +
	"        - name: assignments-api-stripprefix\n" +
	"      services:\n" +
	"        - name: assignments-api-staging\n" +
	"          port: 80\n" +
	"      observability:\n" +
	"        accessLogs: true\n" +
	"  tls: {}\n"

func TestIngressRoute_DecodesRealWorldManifests(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantName      string
		wantNamespace string
		wantMatch     string
	}{
		{
			name:          "readingeggs up health check route",
			yaml:          readingEggsUpRouteYAML,
			wantName:      "readingeggs-staging-route-up-staging",
			wantNamespace: "blake-staging",
			wantMatch:     "Host(`readingeggs-staging.blake-staging.com`) && Path(`/up`)",
		},
		{
			name:          "assignments-api route with middlewares and tls",
			yaml:          assignmentsAPIRouteYAML,
			wantName:      "assignments-api-staging",
			wantNamespace: "blake-staging",
			wantMatch:     "Host(`api.blake-staging.com`) && PathPrefix(`/assignments-api`)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ir IngressRoute
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &ir))

			assert.Equal(t, tt.wantName, ir.Name)
			assert.Equal(t, tt.wantNamespace, ir.Namespace)
			require.Len(t, ir.Spec.Routes, 1)
			assert.Equal(t, tt.wantMatch, ir.Spec.Routes[0].Match)
		})
	}
}

func TestIngressRouteDeepCopy(t *testing.T) {
	original := &IngressRoute{
		Spec: IngressRouteSpec{
			Routes: []Route{{Match: "Host(`example.com`)"}},
		},
	}
	original.Name = "example"

	clone := original.DeepCopy()
	require.Equal(t, original, clone)

	clone.Spec.Routes[0].Match = "Host(`changed.com`)"
	assert.Equal(t, "Host(`example.com`)", original.Spec.Routes[0].Match, "mutating the clone must not affect the original")
}

func TestIngressRouteDeepCopy_Nil(t *testing.T) {
	var ir *IngressRoute
	assert.Nil(t, ir.DeepCopy())

	var list *IngressRouteList
	assert.Nil(t, list.DeepCopy())
}

func TestIngressRouteDeepCopy_EmptyRoutes(t *testing.T) {
	spec := IngressRouteSpec{}
	assert.Equal(t, IngressRouteSpec{}, spec.DeepCopy())
}

func TestIngressRoute_DeepCopyObject(t *testing.T) {
	original := &IngressRoute{Spec: IngressRouteSpec{Routes: []Route{{Match: "Host(`example.com`)"}}}}

	obj := original.DeepCopyObject()

	clone, ok := obj.(*IngressRoute)
	require.True(t, ok, "DeepCopyObject must return a *IngressRoute")
	assert.Equal(t, original, clone)
	assert.NotSame(t, original, clone)
}

func TestIngressRouteList_DeepCopy(t *testing.T) {
	original := &IngressRouteList{
		Items: []IngressRoute{
			{Spec: IngressRouteSpec{Routes: []Route{{Match: "Host(`a.com`)"}}}},
			{Spec: IngressRouteSpec{Routes: []Route{{Match: "Host(`b.com`)"}}}},
		},
	}

	clone := original.DeepCopy()
	require.Equal(t, original, clone)

	clone.Items[0].Spec.Routes[0].Match = "Host(`changed.com`)"
	assert.Equal(t, "Host(`a.com`)", original.Items[0].Spec.Routes[0].Match, "mutating the clone must not affect the original")

	empty := &IngressRouteList{}
	assert.Nil(t, empty.DeepCopy().Items)
}

func TestIngressRouteList_DeepCopyObject(t *testing.T) {
	original := &IngressRouteList{Items: []IngressRoute{{Spec: IngressRouteSpec{Routes: []Route{{Match: "Host(`a.com`)"}}}}}}

	obj := original.DeepCopyObject()

	clone, ok := obj.(*IngressRouteList)
	require.True(t, ok, "DeepCopyObject must return a *IngressRouteList")
	assert.Equal(t, original, clone)
	assert.NotSame(t, original, clone)
}

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()

	require.NoError(t, AddToScheme(scheme))

	assert.True(t, scheme.Recognizes(GroupVersion.WithKind("IngressRoute")))
	assert.True(t, scheme.Recognizes(GroupVersion.WithKind("IngressRouteList")))
}
