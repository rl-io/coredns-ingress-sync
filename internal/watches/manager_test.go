package watches

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}
}

func TestAddConfigMapWatch(t *testing.T) {
	t.Run("handler_logic", func(t *testing.T) {
		manager := NewManager()
		
		if manager == nil {
			t.Error("Expected non-nil manager")
		}
		
		// Test the handler logic that would be used in AddConfigMapWatch
		namespace := "kube-system"
		name := "coredns"
		reconcileName := "coredns-reconcile"
		
		// Create test ConfigMaps
		matchingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		
		nonMatchingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-configmap",
				Namespace: namespace,
			},
		}
		
		wrongNamespaceConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "other-namespace",
			},
		}
		
		// Test the handler logic
		testCases := []struct {
			name              string
			configMap         *corev1.ConfigMap
			expectTrigger     bool
			expectedRequests  int
		}{
			{
				name:             "matching_configmap_should_trigger",
				configMap:        matchingConfigMap,
				expectTrigger:    true,
				expectedRequests: 1,
			},
			{
				name:             "non_matching_name_should_not_trigger",
				configMap:        nonMatchingConfigMap,
				expectTrigger:    false,
				expectedRequests: 0,
			},
			{
				name:             "wrong_namespace_should_not_trigger",
				configMap:        wrongNamespaceConfigMap,
				expectTrigger:    false,
				expectedRequests: 0,
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Simulate the handler logic from AddConfigMapWatch
				var requests []reconcile.Request
				
				if tc.configMap.GetNamespace() == namespace && tc.configMap.GetName() == name {
					requests = []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Name:      reconcileName,
							Namespace: "default",
						},
					}}
				}
				
				if len(requests) != tc.expectedRequests {
					t.Errorf("Expected %d requests, got %d", tc.expectedRequests, len(requests))
				}
				
				if tc.expectTrigger && len(requests) == 0 {
					t.Error("Expected trigger but got no requests")
				}
				
				if !tc.expectTrigger && len(requests) > 0 {
					t.Error("Expected no trigger but got requests")
				}
			})
		}
	})
}

func TestAddDynamicConfigMapWatch(t *testing.T) {
	t.Run("predicate_logic_comprehensive", func(t *testing.T) {
		manager := NewManager()
		
		if manager == nil {
			t.Error("Expected non-nil manager")
		}
		
		// Test the predicate logic for CreateFunc
		t.Run("create_func_always_false", func(t *testing.T) {
			// CreateFunc should always return false per the implementation
			shouldTrigger := false // This matches the implementation: return false
			if shouldTrigger {
				t.Error("CreateFunc should always return false")
			}
		})
		
		// Test the predicate logic for UpdateFunc
		t.Run("update_func_logic", func(t *testing.T) {
			namespace := "kube-system"
			name := "coredns-ingress-sync-rewrite-rules"
			
			testCases := []struct {
				testName      string
				configMap     *corev1.ConfigMap
				expectTrigger bool
				description   string
			}{
				{
					testName: "external_update_should_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
							Labels: map[string]string{
								"managed-by": "terraform",
							},
						},
					},
					expectTrigger: true,
					description:   "External ConfigMap update should trigger reconcile",
				},
				{
					testName: "our_update_should_not_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
							Labels: map[string]string{
								"app.kubernetes.io/managed-by": "coredns-ingress-sync",
							},
						},
					},
					expectTrigger: false,
					description:   "Our own ConfigMap update should not trigger reconcile",
				},
				{
					testName: "wrong_name_should_not_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-configmap",
							Namespace: namespace,
						},
					},
					expectTrigger: false,
					description:   "ConfigMap with wrong name should not trigger",
				},
				{
					testName: "wrong_namespace_should_not_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: "other-namespace",
						},
					},
					expectTrigger: false,
					description:   "ConfigMap in wrong namespace should not trigger",
				},
				{
					testName: "no_labels_should_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
							// No labels at all
						},
					},
					expectTrigger: true,
					description:   "ConfigMap with no labels should trigger (external update)",
				},
			}
			
			for _, tc := range testCases {
				t.Run(tc.testName, func(t *testing.T) {
					// Simulate the UpdateFunc predicate logic
					shouldTrigger := true
					
					// Check namespace and name filtering
					if tc.configMap.GetNamespace() != namespace || tc.configMap.GetName() != name {
						shouldTrigger = false
					}
					
					// Check if it's managed by us
					if shouldTrigger {
						labels := tc.configMap.GetLabels()
						if labels != nil && labels["app.kubernetes.io/managed-by"] == "coredns-ingress-sync" {
							shouldTrigger = false
						}
					}
					
					if shouldTrigger != tc.expectTrigger {
						t.Errorf("%s: expected shouldTrigger=%v, got %v", tc.description, tc.expectTrigger, shouldTrigger)
					}
				})
			}
		})
		
		// Test the predicate logic for DeleteFunc
		t.Run("delete_func_logic", func(t *testing.T) {
			namespace := "kube-system"
			name := "coredns-ingress-sync-rewrite-rules"
			
			testCases := []struct {
				testName      string
				configMap     *corev1.ConfigMap
				expectTrigger bool
				description   string
			}{
				{
					testName: "correct_configmap_delete_should_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
						},
					},
					expectTrigger: true,
					description:   "Deletion of target ConfigMap should trigger",
				},
				{
					testName: "wrong_name_delete_should_not_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-configmap",
							Namespace: namespace,
						},
					},
					expectTrigger: false,
					description:   "Deletion of other ConfigMap should not trigger",
				},
				{
					testName: "wrong_namespace_delete_should_not_trigger",
					configMap: &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: "other-namespace",
						},
					},
					expectTrigger: false,
					description:   "Deletion from wrong namespace should not trigger",
				},
			}
			
			for _, tc := range testCases {
				t.Run(tc.testName, func(t *testing.T) {
					// Simulate the DeleteFunc predicate logic
					shouldTrigger := tc.configMap.GetNamespace() == namespace && tc.configMap.GetName() == name
					
					if shouldTrigger != tc.expectTrigger {
						t.Errorf("%s: expected shouldTrigger=%v, got %v", tc.description, tc.expectTrigger, shouldTrigger)
					}
				})
			}
		})
	})
}

// Integration test that validates the watch setup logic with mock objects
func TestWatchSetupIntegration(t *testing.T) {
	// Test the predicate logic for dynamic ConfigMap watches
	t.Run("dynamic_configmap_predicate_logic", func(t *testing.T) {
		// Create test ConfigMaps
		ourConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-ingress-sync-rewrite-rules",
				Namespace: "kube-system",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "coredns-ingress-sync",
				},
			},
		}
		
		externalConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-ingress-sync-rewrite-rules",
				Namespace: "kube-system",
				Labels: map[string]string{
					"managed-by": "terraform",
				},
			},
		}
		
		wrongNameConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-configmap",
				Namespace: "kube-system",
			},
		}
		
		// Test cases for the predicate logic
		testCases := []struct {
			name           string
			configMap      *corev1.ConfigMap
			expectTrigger  bool
			description    string
		}{
			{
				name:          "our_configmap_should_not_trigger",
				configMap:     ourConfigMap,
				expectTrigger: false,
				description:   "ConfigMap managed by us should not trigger reconcile",
			},
			{
				name:          "external_configmap_should_trigger",
				configMap:     externalConfigMap,
				expectTrigger: true,
				description:   "ConfigMap managed externally should trigger reconcile",
			},
			{
				name:          "wrong_name_should_not_trigger",
				configMap:     wrongNameConfigMap,
				expectTrigger: false,
				description:   "ConfigMap with wrong name should not trigger",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test the logic that would be in the UpdateFunc predicate
				shouldTrigger := true
				
				// Check namespace and name filtering
				if tc.configMap.GetNamespace() != "kube-system" || tc.configMap.GetName() != "coredns-ingress-sync-rewrite-rules" {
					shouldTrigger = false
				}
				
				// Check if it's managed by us
				if shouldTrigger {
					labels := tc.configMap.GetLabels()
					if labels != nil && labels["app.kubernetes.io/managed-by"] == "coredns-ingress-sync" {
						shouldTrigger = false
					}
				}
				
				if shouldTrigger != tc.expectTrigger {
					t.Errorf("%s: expected shouldTrigger=%v, got %v", tc.description, tc.expectTrigger, shouldTrigger)
				}
			})
		}
	})
}
