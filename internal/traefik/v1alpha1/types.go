// Package v1alpha1 provides minimal, hand-written types for Traefik's native
// IngressRoute CRD (traefik.io/v1alpha1). No lightweight official Go module
// exists for Traefik's CRD types -- the only option is
// github.com/traefik/traefik/v3, which pulls in Traefik's entire application
// dependency tree for a handful of fields. These types cover only what the
// controller needs (the router-rule match expression) and rely on
// unstructured JSON decoding to silently discard the rest (services, tls,
// middlewares, entryPoints, observability, priority, etc.), which is safe
// since these objects are never re-serialized.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the Traefik CRD group/version this package models.
var GroupVersion = schema.GroupVersion{Group: "traefik.io", Version: "v1alpha1"}

// SchemeBuilder collects the types to add to a runtime.Scheme.
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

// AddToScheme registers the types in this package with a scheme.
var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&IngressRoute{},
		&IngressRouteList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// IngressRoute is a minimal representation of Traefik's IngressRoute CRD,
// carrying only the fields needed to extract hostname matchers. Unknown
// fields (services, tls, middlewares, entryPoints, observability, priority)
// are discarded on decode.
type IngressRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec IngressRouteSpec `json:"spec,omitempty"`
}

// IngressRouteSpec holds the routes configured on an IngressRoute.
type IngressRouteSpec struct {
	Routes []Route `json:"routes,omitempty"`
}

// Route is a single entry in spec.routes. Match is Traefik's router-rule DSL,
// e.g. "Host(`example.com`) && PathPrefix(`/api`)".
type Route struct {
	Match string `json:"match,omitempty"`
}

// IngressRouteList is a list of IngressRoute objects.
type IngressRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []IngressRoute `json:"items"`
}

// DeepCopyObject implements runtime.Object.
func (in *IngressRoute) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopy returns a deep copy of IngressRoute.
func (in *IngressRoute) DeepCopy() *IngressRoute {
	if in == nil {
		return nil
	}
	out := new(IngressRoute)
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec.DeepCopy()
	return out
}

// DeepCopy returns a deep copy of IngressRouteSpec.
func (in IngressRouteSpec) DeepCopy() IngressRouteSpec {
	if in.Routes == nil {
		return IngressRouteSpec{}
	}
	routes := make([]Route, len(in.Routes))
	copy(routes, in.Routes)
	return IngressRouteSpec{Routes: routes}
}

// DeepCopyObject implements runtime.Object.
func (in *IngressRouteList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopy returns a deep copy of IngressRouteList.
func (in *IngressRouteList) DeepCopy() *IngressRouteList {
	if in == nil {
		return nil
	}
	out := new(IngressRouteList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]IngressRoute, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

// DeepCopyInto copies in into out.
func (in *IngressRoute) DeepCopyInto(out *IngressRoute) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec.DeepCopy()
}
