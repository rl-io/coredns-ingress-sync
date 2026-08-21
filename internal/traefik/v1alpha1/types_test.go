package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
