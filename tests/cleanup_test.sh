#!/bin/bash

# Cleanup functionality test suite for coredns-ingress-sync controller

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} Cleanup tests cannot run against this cluster"
    exit 1
fi

echo "🧹 coredns-ingress-sync Cleanup Tests"
echo "====================================="
echo ""

# Configuration
TEST_RELEASE_NAME="cleanup-test-$(date +%s)"
TEST_NAMESPACE="cleanup-test"
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}

# Test 1: Basic cleanup functionality
test_basic_cleanup() {
    log_info "Testing basic cleanup functionality..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Deploy controller with autoConfigure=true
    log_info "Deploying controller with autoConfigure=true..."
    helm_install_controller "$TEST_RELEASE_NAME" "$TEST_NAMESPACE" "true"
    
    # Wait for controller to be ready
    wait_for_deployment "$TEST_RELEASE_NAME" "$TEST_NAMESPACE"
    
    # Create a test ingress to populate ConfigMap
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cleanup-test-ingress
  namespace: default
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  ingressClassName: nginx
  rules:
  - host: cleanup-test.$TEST_DOMAIN
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: dummy-service
            port:
              number: 80
EOF
    
    # Wait for controller to process ingress
    sleep 10
    
    # Verify ConfigMap was created and contains our hostname
    log_info "Verifying ConfigMap was created..."
    if ! kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME was not created"
        return 1
    fi
    
    local config_content
    config_content=$(kubectl get configmap $CONFIGMAP_NAME -n kube-system -o jsonpath='{.data.dynamic\.server}' 2>/dev/null || echo "")
    if [[ "$config_content" != *"cleanup-test.$TEST_DOMAIN"* ]]; then
        log_error "ConfigMap does not contain expected hostname cleanup-test.$TEST_DOMAIN"
        echo "ConfigMap content: $config_content"
        echo "Looking for: cleanup-test.$TEST_DOMAIN"
        # Wait a bit more and try again
        sleep 10
        config_content=$(kubectl get configmap $CONFIGMAP_NAME -n kube-system -o jsonpath='{.data.dynamic\.server}' 2>/dev/null || echo "")
        if [[ "$config_content" != *"cleanup-test.$TEST_DOMAIN"* ]]; then
            log_error "ConfigMap still does not contain expected hostname after retry"
            return 1
        fi
    fi
    
    # Verify CoreDNS import statement was added
    log_info "Verifying CoreDNS import statement..."
    check_coredns_import_exists || {
        log_error "CoreDNS import statement not found"
        return 1
    }
    
    # Test the actual cleanup process
    log_info "Testing helm uninstall with cleanup..."
    
    # Capture helm uninstall output to check for cleanup job failures
    local uninstall_output
    if ! uninstall_output=$(helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=120s 2>&1); then
        log_error "Helm uninstall failed"
        echo "Uninstall output: $uninstall_output"
        return 1
    fi
    
    # Check if there were any cleanup job failures in the output
    if echo "$uninstall_output" | grep -i "error\|failed\|forbidden" | grep -v "not found"; then
        log_error "Cleanup job encountered errors during uninstall"
        echo "Problematic output: $(echo "$uninstall_output" | grep -i "error\|failed\|forbidden" | grep -v "not found")"
        return 1
    fi
    
    # Verify ConfigMap was deleted
    log_info "Verifying ConfigMap was deleted..."
    if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME was not deleted during cleanup"
        return 1
    fi
    
    # Verify CoreDNS import statement was removed
    log_info "Verifying CoreDNS import statement was removed..."
    check_coredns_import_missing || {
        log_error "CoreDNS import statement still present after cleanup"
        return 1
    }
    
    # Cleanup test resources
    kubectl delete ingress cleanup-test-ingress -n default --ignore-not-found=true
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "Basic cleanup test passed"
    return 0
}

# Test 2: Cleanup with RBAC validation
test_cleanup_rbac_permissions() {
    log_info "Testing cleanup RBAC permissions..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Deploy controller with autoConfigure=true
    log_info "Deploying controller for RBAC test..."
    helm_install_controller "$TEST_RELEASE_NAME" "$TEST_NAMESPACE" "true"
    
    # Wait for controller to be ready
    wait_for_deployment "$TEST_RELEASE_NAME" "$TEST_NAMESPACE"
    
    # Create test ingress to ensure ConfigMap is created
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: rbac-test-ingress
  namespace: default
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  ingressClassName: nginx
  rules:
  - host: rbac-test.$TEST_DOMAIN
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: dummy-service
            port:
              number: 80
EOF
    
    # Wait for controller to process ingress
    sleep 10
    
    # Verify the ServiceAccount has correct permissions by checking ClusterRole
    log_info "Verifying RBAC permissions..."
    local coredns_role_content
    coredns_role_content=$(kubectl get clusterrole $TEST_RELEASE_NAME-coredns-ingress-sync-coredns -o yaml 2>/dev/null || echo "")
    
    # Check if delete permission is present
    if [[ "$coredns_role_content" != *"delete"* ]]; then
        log_error "ClusterRole does not contain 'delete' permission for ConfigMaps"
        echo "ClusterRole rules:"
        kubectl get clusterrole $TEST_RELEASE_NAME-coredns-ingress-sync-coredns -o jsonpath='{.rules}' | jq '.'
        return 1
    fi
    
    # Test cleanup with detailed error checking
    log_info "Testing cleanup with RBAC validation..."
    
    # Run cleanup manually to capture detailed output
    local cleanup_pod_name="manual-cleanup-test"
    kubectl run $cleanup_pod_name --namespace=$TEST_NAMESPACE \
        --image=ghcr.io/rl-io/coredns-ingress-sync:latest \
        --restart=Never \
        --serviceaccount=$TEST_RELEASE_NAME-coredns-ingress-sync \
        --env="CLEANUP_MODE=true" \
        --env="COREDNS_NAMESPACE=kube-system" \
        --env="DYNAMIC_CONFIGMAP_NAME=$CONFIGMAP_NAME" \
        --command -- /app/coredns-ingress-sync
    
    # Wait for cleanup pod to complete
    kubectl wait --for=condition=Ready --timeout=30s pod/$cleanup_pod_name -n $TEST_NAMESPACE || true
    sleep 5
    
    # Check cleanup pod logs for errors
    local cleanup_logs
    cleanup_logs=$(kubectl logs $cleanup_pod_name -n $TEST_NAMESPACE 2>/dev/null || echo "")
    
    if echo "$cleanup_logs" | grep -i "forbidden\|permission denied\|unauthorized"; then
        log_error "Cleanup encountered RBAC permission errors"
        echo "Cleanup logs:"
        echo "$cleanup_logs"
        kubectl delete pod $cleanup_pod_name -n $TEST_NAMESPACE --ignore-not-found=true
        return 1
    fi
    
    # Clean up test pod
    kubectl delete pod $cleanup_pod_name -n $TEST_NAMESPACE --ignore-not-found=true
    
    # Now do the actual helm uninstall
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=120s
    
    # Cleanup test resources
    kubectl delete ingress rbac-test-ingress -n default --ignore-not-found=true
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "RBAC cleanup test passed"
    return 0
}

# Test 3: Cleanup failure scenarios
test_cleanup_failure_scenarios() {
    log_info "Testing cleanup failure scenarios..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Deploy controller without autoConfigure (should still have RBAC for cleanup)
    log_info "Deploying controller without autoConfigure..."
    helm_install_controller "$TEST_RELEASE_NAME" "$TEST_NAMESPACE" "false"
    
    # Manually create the ConfigMap that cleanup should delete
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $CONFIGMAP_NAME
  namespace: kube-system
data:
  dynamic.server: |
    rewrite name exact test.example.com test-target.cluster.local.
EOF
    
    # Test that cleanup can still delete ConfigMap even when autoConfigure=false
    helm uninstall "$TEST_RELEASE_NAME" --namespace "$TEST_NAMESPACE" --wait --timeout=120s
    
    # Verify ConfigMap was deleted
    if kubectl get configmap "$CONFIGMAP_NAME" -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME was not deleted during cleanup (autoConfigure=false case)"
        return 1
    fi
    
    # Cleanup test resources
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "Cleanup failure scenarios test passed"
    return 0
}

# Main test runner
main() {
    local tests_passed=0
    local tests_failed=0
    
    # Run tests
    echo "Running cleanup tests..."
    echo ""
    
    if test_basic_cleanup; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_cleanup_rbac_permissions; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_cleanup_failure_scenarios; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    # Summary
    echo ""
    echo "📊 Cleanup Test Results"
    echo "======================="
    echo "Passed: $tests_passed"
    echo "Failed: $tests_failed"
    echo ""
    
    if [ $tests_failed -eq 0 ]; then
        log_info "🎉 All cleanup tests passed!"
        exit 0
    else
        log_error "❌ Some cleanup tests failed"
        exit 1
    fi
}

# Set up cleanup for the test itself
cleanup() {
    log_info "Cleaning up test resources..."
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE 2>/dev/null || true
    kubectl delete ingress cleanup-test-ingress rbac-test-ingress -n default 2>/dev/null || true
    kubectl delete namespace $TEST_NAMESPACE 2>/dev/null || true
    kubectl delete configmap $CONFIGMAP_NAME -n kube-system 2>/dev/null || true
}

trap cleanup EXIT

# Run main function
main "$@"
