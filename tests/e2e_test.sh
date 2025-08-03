#!/bin/bash

# End-to-end test suite for coredns-ingress-sync controller

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} E2E tests cannot run against this cluster"
    exit 1
fi

echo "🎯 End-to-End Test Suite"
echo "========================"

# Configuration
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}
CONTROLLER_IMAGE=${CONTROLLER_IMAGE:-coredns-ingress-sync:latest}

# Test 1: Full deployment test
test_full_deployment() {
    log_info "Testing full deployment from scratch..."
    
    # Ensure controller is deployed  
    if ! ensure_controller_deployed; then
        return 1
    fi
    
    log_info "Full deployment test passed"
    return 0
}

# Test 2: Real ingress scenario
test_real_ingress_scenario() {
    log_info "Testing real ingress scenario..."
    
    # Create a complete ingress setup
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: test-app-service
  namespace: default
spec:
  selector:
    app: test-app
  ports:
  - port: 80
    targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-app
        image: nginx:alpine
        ports:
        - containerPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-app-ingress
  namespace: default
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: test-app.$TEST_DOMAIN
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-app-service
            port:
              number: 80
EOF
    
    # Wait for controller to process
    wait_for_controller_sync
    
    # Check if hostname appears in ConfigMap
    if hostname_in_configmap "test-app.$TEST_DOMAIN"; then
        log_info "Controller successfully processed real ingress"
        
        # Clean up
        kubectl delete deployment test-app -n default
        kubectl delete service test-app-service -n default
        kubectl delete ingress test-app-ingress -n default
        
        return 0
    else
        log_error "Controller did not process real ingress"
        local config
        config=$(get_configmap_content)
        log_error "ConfigMap content: $config"
        
        # Clean up
        kubectl delete deployment test-app -n default
        kubectl delete service test-app-service -n default
        kubectl delete ingress test-app-ingress -n default
        
        return 1
    fi
}

# Test 3: CoreDNS integration test
test_coredns_integration() {
    log_info "Testing CoreDNS integration..."
    
    # Check if CoreDNS can read the ConfigMap
    local coredns_logs
    coredns_logs=$(kubectl logs deployment/coredns -n $COREDNS_NAMESPACE --tail=50)
    
    # Look for import-related errors
    if echo "$coredns_logs" | grep -i "import.*error\|import.*fail"; then
        log_error "CoreDNS import errors detected"
        log_error "Logs: $coredns_logs"
        return 1
    fi
    
    # Check if CoreDNS is healthy
    if ! kubectl get pods -n $COREDNS_NAMESPACE -l k8s-app=kube-dns --field-selector=status.phase=Running | grep -q Running; then
        log_error "CoreDNS pods not running"
        return 1
    fi
    
    log_info "CoreDNS integration test passed"
    return 0
}

# Test 4: DNS resolution is handled by integration tests
# (Removed duplicate - integration test has better implementation)

# Test 5: Load test
test_load_handling() {
    log_info "Testing load handling with many ingresses..."
    
    # Create 20 test ingresses
    for i in {1..20}; do
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: load-test-$i
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: load-test-$i.$TEST_DOMAIN
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-service
            port:
              number: 80
EOF
    done
    
    # Wait for controller to process all ingresses
    wait_for_controller_sync 5
    
    # Check if all hostnames are in ConfigMap
    local config
    config=$(get_configmap_content)
    
    local missing_count=0
    for i in {1..20}; do
        if [[ "$config" != *"load-test-$i.$TEST_DOMAIN"* ]]; then
            missing_count=$((missing_count + 1))
        fi
    done
    
    # Clean up
    for i in {1..20}; do
        kubectl delete ingress load-test-$i -n default 2>/dev/null || true
    done
    
    if [ $missing_count -eq 0 ]; then
        log_info "Load test passed - all 20 ingresses processed"
        return 0
    else
        log_error "Load test failed - $missing_count ingresses missing from ConfigMap"
        return 1
    fi
}

# Test 6: Disaster recovery test
test_disaster_recovery() {
    log_info "Testing disaster recovery..."
    
    # Create some test ingresses
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: recovery-test-1
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: recovery-1.$TEST_DOMAIN
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: recovery-test-2
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: recovery-2.$TEST_DOMAIN
EOF
    
    # Wait for initial processing
    wait_for_controller_sync 10
    
    # Simulate disaster by deleting ConfigMap
    kubectl delete configmap $CONFIGMAP_NAME -n $COREDNS_NAMESPACE
    
    # Delete controller pod to trigger recreation
    kubectl delete pod -l app=coredns-ingress-sync -n $NAMESPACE
    
    # Wait for recovery
    wait_for_controller_sync 10
    
    # Check if ConfigMap was recreated
    if ! kubectl get configmap $CONFIGMAP_NAME -n $COREDNS_NAMESPACE &>/dev/null; then
        log_error "ConfigMap was not recreated after disaster"
        return 1
    fi
    
    # Check if ingresses were rediscovered
    local config
    config=$(get_configmap_content)
    
    if [[ "$config" == *"recovery-1.$TEST_DOMAIN"* ]] && [[ "$config" == *"recovery-2.$TEST_DOMAIN"* ]]; then
        log_info "Disaster recovery test passed"
        
        # Clean up
        kubectl delete ingress recovery-test-1 recovery-test-2 -n default
        
        return 0
    else
        log_error "Disaster recovery failed - ingresses not rediscovered"
        
        # Clean up
        kubectl delete ingress recovery-test-1 recovery-test-2 -n default
        
        return 1
    fi
}

# Main test execution
main() {
    local tests_passed=0
    local tests_failed=0
    
    # Run all tests
    if test_full_deployment; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_real_ingress_scenario; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_coredns_integration; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    # DNS resolution is tested in integration tests (better implementation)
    
    if test_load_handling; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    if test_disaster_recovery; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi
    
    # Summary
    echo ""
    echo "======================================"
    echo "🎯 End-to-End Test Results"
    echo "======================================"
    echo "Passed: $tests_passed"
    echo "Failed: $tests_failed"
    echo ""
    
    if [ $tests_failed -eq 0 ]; then
        log_info "🎉 All E2E tests passed!"
        exit 0
    else
        log_error "❌ Some E2E tests failed"
        exit 1
    fi
}

# Set up cleanup
cleanup() {
    log_info "Cleaning up E2E test resources..."
    kubectl delete ingress test-app-ingress -n default 2>/dev/null || true
    kubectl delete deployment test-app -n default 2>/dev/null || true
    kubectl delete service test-app-service -n default 2>/dev/null || true
    kubectl delete ingress recovery-test-1 recovery-test-2 -n default 2>/dev/null || true
    for i in {1..20}; do
        kubectl delete ingress load-test-$i -n default 2>/dev/null || true
    done
    
    # Cleanup controller if it was deployed by this test run
    cleanup_controller
}

trap cleanup EXIT

# Run main function
main "$@"
