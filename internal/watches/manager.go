package watches

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Manager handles watch setup for different Kubernetes resources
type Manager struct{}

// NewManager creates a new watch manager
func NewManager() *Manager {
	return &Manager{}
}

// AddConfigMapWatch adds a watch for a specific ConfigMap
func (m *Manager) AddConfigMapWatch(cache cache.Cache, c ctrlcontroller.Controller, namespace, name, reconcileName string) error {
	return c.Watch(
		source.Kind(cache, &corev1.ConfigMap{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj *corev1.ConfigMap) []reconcile.Request {
				if obj.GetNamespace() == namespace && obj.GetName() == name {
					return []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Name:      reconcileName,
							Namespace: "default",
						},
					}}
				}
				return []reconcile.Request{}
			})))
}

// AddDynamicConfigMapWatch adds a watch for dynamic ConfigMap changes with smart filtering
func (m *Manager) AddDynamicConfigMapWatch(cache cache.Cache, c ctrlcontroller.Controller, namespace, name, reconcileName string) error {
	return c.Watch(
		source.Kind(cache, &corev1.ConfigMap{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj *corev1.ConfigMap) []reconcile.Request {
				// Only trigger on the specific dynamic ConfigMap
				if obj.GetNamespace() == namespace && obj.GetName() == name {
					return []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Name:      reconcileName,
							Namespace: "default",
						},
					}}
				}
				return []reconcile.Request{}
			}),
			predicate.TypedFuncs[*corev1.ConfigMap]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.ConfigMap]) bool {
					// Don't trigger on create events - we create the ConfigMap ourselves
					return false
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
					// Only watch the specific dynamic ConfigMap
					if e.ObjectNew.GetNamespace() != namespace || e.ObjectNew.GetName() != name {
						return false
					}
					
					// Only trigger on updates that are NOT from us
					// If the ConfigMap has our management label, it means we updated it, so ignore
					labels := e.ObjectNew.GetLabels()
					if labels != nil && labels["app.kubernetes.io/managed-by"] == "coredns-ingress-sync" {
						return false // Ignore our own updates
					}
					
					// Trigger on external updates (like Terraform removing our ConfigMap)
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.ConfigMap]) bool {
					// Trigger on delete for disaster recovery
					return e.Object.GetNamespace() == namespace && e.Object.GetName() == name
				},
			}))
}
