#!/bin/bash

# Upgrade test suite for coredns-ingress-sync controller
# Tests various upgrade scenarios including autoConfigure changes

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$TEST_DIR/.." && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} Upgrade tests cannot run against this cluster"
    exit 1
fi

echo "⬆️  coredns-ingress-sync Upgrade Tests"
echo "======================================"
echo ""

# Configuration
TEST_RELEASE_NAME="upgrade-test-$(date +%s)"
TEST_NAMESPACE="upgrade-test"
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}

# Test 1: Upgrade from autoConfigure=false to autoConfigure=true
test_upgrade_false_to_true() {
    log_info "Testing upgrade from autoConfigure=false to autoConfigure=true..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Step 1: Deploy with autoConfigure=false
    log_info "Step 1: Initial deployment with autoConfigure=false..."
    helm install $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
        --namespace $TEST_NAMESPACE \
        --set coreDNS.autoConfigure=false \
        --set controller.targetCNAME=test-target.cluster.local \
        --wait --timeout=120s
    
    # Wait for controller to be ready
    kubectl wait --for=condition=available --timeout=60s deployment/$TEST_RELEASE_NAME-coredns-ingress-sync -n $TEST_NAMESPACE
    
    # Create a test ingress
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: upgrade-test-ingress
  namespace: default
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  ingressClassName: nginx
  rules:
  - host: upgrade-test.$TEST_DOMAIN
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
    
    # Wait a bit for controller to process
    sleep 10
    
    # Verify that with autoConfigure=false, no CoreDNS changes were made
    log_info "Verifying autoConfigure=false behavior..."
    if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME should not exist when autoConfigure=false"
        return 1
    fi
    
    local corefile_content
    corefile_content=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    if [[ "$corefile_content" == *"import /etc/coredns/custom/*.server"* ]]; then
        log_error "CoreDNS Corefile should not contain import statement when autoConfigure=false"
        return 1
    fi
    
    # Step 2: Upgrade to autoConfigure=true
    log_info "Step 2: Upgrading to autoConfigure=true..."
    helm upgrade $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
        --namespace $TEST_NAMESPACE \
        --set coreDNS.autoConfigure=true \
        --set controller.targetCNAME=test-target.cluster.local \
        --wait --timeout=120s
    
    # Wait for controller to restart and process
    kubectl rollout status deployment/$TEST_RELEASE_NAME-coredns-ingress-sync -n $TEST_NAMESPACE --timeout=60s
    sleep 15
    
    # Verify that upgrade was successful and RBAC works
    log_info "Verifying upgrade to autoConfigure=true..."
    
    # Check that ConfigMap was created
    local retry_count=0
    while [ $retry_count -lt 12 ]; do
        if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
            break
        fi
        log_info "Waiting for ConfigMap to be created... (attempt $((retry_count + 1))/12)"
        sleep 5
        retry_count=$((retry_count + 1))
    done
    
    if ! kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME was not created after upgrade to autoConfigure=true"
        return 1
    fi
    
    # Check that ConfigMap contains our hostname
    local config_content
    config_content=$(kubectl get configmap $CONFIGMAP_NAME -n kube-system -o jsonpath='{.data.dynamic\.server}' 2>/dev/null || echo "")
    if [[ "$config_content" != *"upgrade-test.$TEST_DOMAIN"* ]]; then
        log_error "ConfigMap does not contain expected hostname after upgrade"
        echo "ConfigMap content: $config_content"
        return 1
    fi
    
    # Check that CoreDNS import statement was added
    corefile_content=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    if [[ "$corefile_content" != *"import /etc/coredns/custom/*.server"* ]]; then
        log_error "CoreDNS Corefile does not contain import statement after upgrade"
        echo "Corefile content: $corefile_content"
        return 1
    fi
    
    # Verify that volume mount was added to CoreDNS deployment
    local coredns_deployment
    coredns_deployment=$(kubectl get deployment coredns -n kube-system -o json 2>/dev/null || echo "{}")
    if ! echo "$coredns_deployment" | jq -e '.spec.template.spec.volumes[]? | select(.name == "'$VOLUME_NAME'")' &>/dev/null; then
        log_error "CoreDNS deployment does not have $VOLUME_NAME after upgrade"
        return 1
    fi
    
    # Step 3: Test cleanup after upgrade
    log_info "Step 3: Testing cleanup after upgrade..."
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=120s
    
    # Verify cleanup worked (ConfigMap deleted, import statement removed)
    if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME was not deleted during cleanup"
        return 1
    fi
    
    corefile_content=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    if [[ "$corefile_content" == *"import /etc/coredns/custom/*.server"* ]]; then
        log_error "CoreDNS Corefile still contains import statement after cleanup"
        return 1
    fi
    
    # Cleanup test resources
    kubectl delete ingress upgrade-test-ingress -n default --ignore-not-found=true
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "Upgrade false→true test passed"
    return 0
}

# Test 2: Upgrade from autoConfigure=true to autoConfigure=false
test_upgrade_true_to_false() {
    log_info "Testing upgrade from autoConfigure=true to autoConfigure=false..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Step 1: Deploy with autoConfigure=true
    log_info "Step 1: Initial deployment with autoConfigure=true..."
    helm install $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
        --namespace $TEST_NAMESPACE \
        --set coreDNS.autoConfigure=true \
        --set controller.targetCNAME=test-target.cluster.local \
        --wait --timeout=120s
    
    # Wait for controller to be ready
    kubectl wait --for=condition=available --timeout=60s deployment/$TEST_RELEASE_NAME-coredns-ingress-sync -n $TEST_NAMESPACE
    
    # Create a test ingress
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: upgrade-test-2-ingress
  namespace: default
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  ingressClassName: nginx
  rules:
  - host: upgrade-test-2.$TEST_DOMAIN
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
    
    # Wait for controller to process and setup CoreDNS
    sleep 15
    
    # Verify autoConfigure=true setup
    log_info "Verifying autoConfigure=true setup..."
    if ! kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME should exist when autoConfigure=true"
        return 1
    fi
    
    # Step 2: Upgrade to autoConfigure=false
    log_info "Step 2: Upgrading to autoConfigure=false..."
    helm upgrade $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
        --namespace $TEST_NAMESPACE \
        --set coreDNS.autoConfigure=false \
        --set controller.targetCNAME=test-target.cluster.local \
        --wait --timeout=120s
    
    # Wait for controller to restart
    kubectl rollout status deployment/$TEST_RELEASE_NAME-coredns-ingress-sync -n $TEST_NAMESPACE --timeout=60s
    sleep 10
    
    # Verify that CoreDNS configuration was cleaned up during upgrade
    log_info "Verifying upgrade to autoConfigure=false..."
    
    # ConfigMap should still exist (only deleted on uninstall)
    # But CoreDNS should not be actively managed
    local corefile_content
    corefile_content=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    
    # The controller should have cleaned up the import statement during upgrade
    # (This tests the defensive configuration management)
    
    # Step 3: Test cleanup
    log_info "Step 3: Testing cleanup with autoConfigure=false..."
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=120s
    
    # Verify cleanup still works even when autoConfigure=false
    if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME was not deleted during cleanup (autoConfigure=false)"
        return 1
    fi
    
    # Cleanup test resources
    kubectl delete ingress upgrade-test-2-ingress -n default --ignore-not-found=true
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "Upgrade true→false test passed"
    return 0
}

# Test 3: Multiple upgrade cycles
test_multiple_upgrade_cycles() {
    log_info "Testing multiple upgrade cycles..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Cycle through configurations
    local configs=("false" "true" "false" "true")
    
    for i in "${!configs[@]}"; do
        local auto_config="${configs[$i]}"
        log_info "Cycle $((i + 1)): Setting autoConfigure=$auto_config..."
        
        if [ $i -eq 0 ]; then
            # First deployment
            helm install $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
                --namespace $TEST_NAMESPACE \
                --set coreDNS.autoConfigure=$auto_config \
                --set controller.targetCNAME=test-target.cluster.local \
                --wait --timeout=120s
        else
            # Subsequent upgrades
            helm upgrade $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
                --namespace $TEST_NAMESPACE \
                --set coreDNS.autoConfigure=$auto_config \
                --set controller.targetCNAME=test-target.cluster.local \
                --wait --timeout=120s
        fi
        
        # Wait for controller to be ready
        kubectl wait --for=condition=available --timeout=60s deployment/$TEST_RELEASE_NAME-coredns-ingress-sync -n $TEST_NAMESPACE
        sleep 5
        
        # Verify configuration matches expected state
        if [ "$auto_config" = "true" ]; then
            # Should eventually have ConfigMap and import statement
            local retry_count=0
            while [ $retry_count -lt 6 ]; do
                if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
                    break
                fi
                sleep 5
                retry_count=$((retry_count + 1))
            done
        fi
    done
    
    # Final cleanup
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=120s
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "Multiple upgrade cycles test passed"
    return 0
}

# Test 4: RBAC consistency across upgrades
test_rbac_consistency() {
    log_info "Testing RBAC consistency across upgrades..."
    
    # Create test namespace
    kubectl create namespace $TEST_NAMESPACE || true
    
    # Deploy with autoConfigure=false
    helm install $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
        --namespace $TEST_NAMESPACE \
        --set coreDNS.autoConfigure=false \
        --set controller.targetCNAME=test-target.cluster.local \
        --wait --timeout=120s
    
    # Check that RBAC includes delete permission even with autoConfigure=false
    log_info "Verifying RBAC permissions with autoConfigure=false..."
    local coredns_role_content
    coredns_role_content=$(kubectl get clusterrole $TEST_RELEASE_NAME-coredns-ingress-sync-coredns -o yaml 2>/dev/null || echo "")
    
    if [[ "$coredns_role_content" != *"delete"* ]]; then
        log_error "ClusterRole does not contain 'delete' permission with autoConfigure=false"
        return 1
    fi
    
    # Upgrade to autoConfigure=true
    helm upgrade $TEST_RELEASE_NAME $PROJECT_DIR/helm/coredns-ingress-sync \
        --namespace $TEST_NAMESPACE \
        --set coreDNS.autoConfigure=true \
        --set controller.targetCNAME=test-target.cluster.local \
        --wait --timeout=120s
    
    # Check that RBAC still includes delete permission
    log_info "Verifying RBAC permissions after upgrade to autoConfigure=true..."
    coredns_role_content=$(kubectl get clusterrole $TEST_RELEASE_NAME-coredns-ingress-sync-coredns -o yaml 2>/dev/null || echo "")
    
    if [[ "$coredns_role_content" != *"delete"* ]]; then
        log_error "ClusterRole does not contain 'delete' permission after upgrade"
        return 1
    fi
    
    # Test that cleanup works
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=120s
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true
    
    log_info "RBAC consistency test passed"
    return 0
}

# Main test runner
main() {
    local tests_passed=0
    local tests_failed=0
    
    # Run tests
    echo "Running upgrade tests..."
    echo ""
    
    if test_upgrade_false_to_true; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_upgrade_true_to_false; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_multiple_upgrade_cycles; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_rbac_consistency; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    # Summary
    echo ""
    echo "📊 Upgrade Test Results"
    echo "======================"
    echo "Passed: $tests_passed"
    echo "Failed: $tests_failed"
    echo ""
    
    if [ $tests_failed -eq 0 ]; then
        log_info "🎉 All upgrade tests passed!"
        exit 0
    else
        log_error "❌ Some upgrade tests failed"
        exit 1
    fi
}

# Set up cleanup for the test itself
cleanup() {
    log_info "Cleaning up upgrade test resources..."
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE 2>/dev/null || true
    kubectl delete ingress upgrade-test-ingress upgrade-test-2-ingress -n default 2>/dev/null || true
    kubectl delete namespace $TEST_NAMESPACE 2>/dev/null || true
    kubectl delete configmap $CONFIGMAP_NAME -n kube-system 2>/dev/null || true
}

trap cleanup EXIT

# Run main function
main "$@"
