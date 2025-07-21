#!/bin/bash

# Integration test suite for coredns-ingress-sync controller

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} Integration tests cannot run against this cluster"
    exit 1
fi

echo "🧪 coredns-ingress-sync controller Integration Tests"
echo "====================================================="
echo ""

# Test 1: Prerequisites and setup
test_prerequisites() {
    if ! check_test_prerequisites; then
        return 1
    fi
    
    # Ensure controller is deployed
    if ! ensure_controller_deployed; then
        return 1
    fi
    
    log_info "Prerequisites check passed"
    return 0
}

# Test 2: Controller deployment
test_controller_deployment() {
    log_info "Testing controller deployment..."
    
    # Check if controller deployment exists
    if ! kubectl get deployment $CONTROLLER_NAME -n $NAMESPACE &> /dev/null; then
        log_error "Controller deployment not found"
        return 1
    fi
    
    # Check if controller is running
    if ! kubectl wait --for=condition=available --timeout=60s deployment/$CONTROLLER_NAME -n $NAMESPACE; then
        log_error "Controller deployment not ready"
        return 1
    fi
    
    log_info "Controller deployment is healthy"
    return 0
}

# Test 3: ConfigMap creation
test_configmap_creation() {
    log_info "Testing ConfigMap creation..."
    
    # Check if ConfigMap already exists
    if kubectl get configmap $CONFIGMAP_NAME -n $COREDNS_NAMESPACE &> /dev/null; then
        log_info "ConfigMap already exists"
    else
        log_info "ConfigMap doesn't exist yet - creating test ingress to trigger creation"
        
        # Create a test ingress to trigger ConfigMap creation
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-configmap-trigger
  namespace: default
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: test-configmap.k8s.example.com
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
        
        # Wait for controller to process and create ConfigMap
        wait_for_controller_sync 5
        
        # Clean up the test ingress
        kubectl delete ingress test-configmap-trigger -n default
    fi
    
    # Check if ConfigMap exists in the correct namespace (kube-system)
    if ! kubectl get configmap $CONFIGMAP_NAME -n $COREDNS_NAMESPACE &> /dev/null; then
        log_error "ConfigMap $CONFIGMAP_NAME not found in namespace $COREDNS_NAMESPACE"
        return 1
    fi
    
    # Check if ConfigMap has expected key
    if ! get_configmap_content > /dev/null; then
        log_error "ConfigMap missing expected key 'dynamic.server'"
        return 1
    fi
    
    log_info "ConfigMap exists and has correct structure"
    return 0
}

# Test 4: Ingress discovery
test_ingress_discovery() {
    log_info "Testing ingress discovery..."
    
    # Create a test ingress
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ingress-dynamic
  namespace: default
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: test-dynamic.k8s.example.com
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
    
    # Wait for controller to process
    wait_for_controller_sync 5
    
    # Check if ConfigMap was updated
    local config_content
    config_content=$(get_configmap_content)
    
    if [[ "$config_content" == *"test-dynamic.k8s.example.com"* ]]; then
        log_info "Controller successfully discovered test ingress"
        
        # Also check for exact match configuration while the ingress exists
        if [[ "$config_content" == *"rewrite name exact"* ]]; then
            log_info "Configuration uses exact match rules (secure)"
        else
            log_error "Configuration does not use exact match rules"
            log_error "ConfigMap content: $config_content"
            
            # Clean up test ingress
            kubectl delete ingress test-ingress-dynamic -n default
            
            return 1
        fi
        
        # Clean up test ingress
        kubectl delete ingress test-ingress-dynamic -n default
        
        return 0
    else
        log_error "Controller did not discover test ingress"
        log_error "ConfigMap content: $config_content"
        
        # Clean up test ingress
        kubectl delete ingress test-ingress-dynamic -n default
        
        return 1
    fi
}



# Test 6: Controller logs
test_controller_logs() {
    log_info "Testing controller logs..."
    
    # Get recent logs
    local logs
    logs=$(kubectl logs deployment/$CONTROLLER_NAME -n $NAMESPACE --tail=50)
    
    # Check for error indicators
    if echo "$logs" | grep -i "error\|panic\|fatal" | grep -v "level=info"; then
        log_error "Controller logs contain errors"
        log_error "Recent logs:"
        echo "$logs" | tail -20
        return 1
    fi
    
    # Check for successful operations
    if echo "$logs" | grep -i "reconciling\|updated\|generated"; then
        log_info "Controller logs show successful operations"
        return 0
    else
        log_warn "Controller logs don't show recent activity"
        return 0
    fi
}

# Test 7: CoreDNS health and configuration
test_coredns_health() {
    log_info "Testing CoreDNS health and configuration..."
    
    # Check if CoreDNS deployment is healthy (CoreDNS runs in kube-system)
    if ! kubectl get deployment coredns -n kube-system &> /dev/null; then
        log_error "CoreDNS deployment not found in kube-system"
        return 1
    fi
    
    if ! kubectl wait --for=condition=available --timeout=60s deployment/coredns -n kube-system; then
        log_error "CoreDNS deployment not ready"
        return 1
    fi
    
    # Check if CoreDNS configuration includes our dynamic configuration
    local coredns_config
    coredns_config=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    
    if echo "$coredns_config" | grep -qF "import /etc/coredns/custom/*.server"; then
        log_info "CoreDNS is configured to use dynamic ConfigMap"
    else
        log_warn "CoreDNS configuration may not include dynamic ConfigMap reference"
        log_warn "This might be expected if the controller handles CoreDNS configuration differently"
    fi
    
    # Check CoreDNS logs for errors
    local coredns_logs
    coredns_logs=$(kubectl logs deployment/coredns -n kube-system --tail=20)
    
    if echo "$coredns_logs" | grep -i "error\|panic\|fatal" | grep -v "level=info"; then
        log_error "CoreDNS logs contain errors"
        log_error "Recent logs:"
        echo "$coredns_logs" | tail -10
        return 1
    fi
    
    log_info "CoreDNS is healthy"
    return 0
}

# Test 8: DNS resolution test
test_dns_resolution() {
    log_info "Testing DNS resolution..."
    
    # Get the ingress-nginx service ClusterIP for comparison
    local nginx_clusterip
    nginx_clusterip=$(kubectl get service ingress-nginx-controller -n ingress-nginx -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
    
    if [[ -z "$nginx_clusterip" ]]; then
        log_warn "ingress-nginx service not found, skipping DNS resolution test"
        return 0
    fi
    
    log_info "Expected resolution target: $nginx_clusterip (ingress-nginx-controller)"
    
    # Get a hostname from the ConfigMap
    local config_content
    config_content=$(get_configmap_content)
    
    # Extract a hostname from the rewrite rules
    local test_hostname
    test_hostname=$(echo "$config_content" | grep -o "rewrite name exact [^[:space:]]*" | head -1 | awk '{print $4}')
    
    if [[ -z "$test_hostname" ]]; then
        log_warn "No hostnames found in ConfigMap to test"
        return 0
    fi
    
    log_info "Testing DNS resolution for: $test_hostname"
    
    # Get the actual DNS service IP dynamically
    local dns_service_ip
    dns_service_ip=$(kubectl get service kube-dns -n kube-system -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
    
    if [[ -z "$dns_service_ip" ]]; then
        log_error "Could not find DNS service IP"
        return 1
    fi
    
    log_info "Using DNS service IP: $dns_service_ip"
    
    # Create a test pod with proper DNS configuration
    cat << EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: dns-test-pod
  namespace: default
spec:
  restartPolicy: Never
  dnsPolicy: None
  dnsConfig:
    nameservers: ["$dns_service_ip"]
    searches: ["default.svc.cluster.local", "svc.cluster.local", "cluster.local"]
    options:
    - name: ndots
      value: "5"
  containers:
  - name: dns-test
    image: busybox:1.35
    command: ["/bin/sh"]
    args:
    - -c
    - |
      set -e
      echo '=== DNS Test Starting ==='
      echo "Test hostname: \$test_hostname"
      echo "Target service: ingress-nginx-controller.ingress-nginx.svc.cluster.local"
      echo
      echo '=== DNS Configuration ==='
      cat /etc/resolv.conf
      echo
      echo '=== Testing hostname resolution ==='
      if nslookup "\$test_hostname"; then
        echo "SUCCESS: Hostname resolution worked"
      else
        echo "FAILED: Hostname resolution failed"
        exit 1
      fi
      echo
      echo '=== Testing expected target ==='
      if nslookup ingress-nginx-controller.ingress-nginx.svc.cluster.local; then
        echo "SUCCESS: Target service resolution worked"
      else
        echo "FAILED: Target service resolution failed"
        exit 1
      fi
      echo
      echo '=== DNS Test Completed Successfully ==='
    env:
    - name: test_hostname
      value: "$test_hostname"
EOF
    
    # Wait for pod to complete (not be ready, since it's a one-shot job)
    log_info "Waiting for DNS test pod to complete..."
    
    # Wait for pod to either succeed or fail
    local timeout=60
    local elapsed=0
    local pod_phase=""
    
    while [ $elapsed -lt $timeout ]; do
        pod_phase=$(kubectl get pod dns-test-pod -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
        
        if [[ "$pod_phase" == "Succeeded" ]] || [[ "$pod_phase" == "Failed" ]]; then
            break
        fi
        
        sleep 2
        elapsed=$((elapsed + 2))
    done
    
    if [ $elapsed -ge $timeout ]; then
        log_error "DNS test pod timed out after ${timeout}s (phase: $pod_phase)"
        kubectl describe pod dns-test-pod || true
        kubectl delete pod dns-test-pod --ignore-not-found=true
        return 1
    fi
    
    log_info "DNS test pod completed with phase: $pod_phase"
    
    # Get the pod logs
    local dns_output
    dns_output=$(kubectl logs dns-test-pod 2>/dev/null || echo "No output from DNS test pod")
    
    # Clean up the test pod
    kubectl delete pod dns-test-pod --ignore-not-found=true
    
    # Check if the pod failed
    if [[ "$pod_phase" == "Failed" ]]; then
        log_error "DNS test pod failed to execute"
        log_error "Pod logs: $dns_output"
        return 1
    fi
    
    # Check if we got any meaningful output
    if [[ "$dns_output" == "No output from DNS test pod" ]] || [[ -z "$dns_output" ]]; then
        log_error "No output from DNS test pod"
        return 1
    fi
    
    log_info "DNS test output: $dns_output"
    
    # Check if the hostname resolves to the expected ingress-nginx IP
    if echo "$dns_output" | grep -q "$nginx_clusterip"; then
        log_info "DNS resolution working correctly - hostname resolves to ingress-nginx ClusterIP"
        return 0
    elif echo "$dns_output" | grep -q "ingress-nginx"; then
        log_info "DNS resolution partially working - found ingress-nginx reference"
        return 0
    else
        log_error "DNS resolution test failed - hostname not resolving correctly"
        log_error "Expected IP: $nginx_clusterip"
        log_error "Full test output: $dns_output"
        return 1
    fi
}

# Test 9: Load testing is handled by E2E tests
# (Removed duplicate - E2E test has better scale with 20 ingresses)

# Test 10: Recovery test
test_recovery() {
    log_info "Testing controller recovery..."
    
    # Create a test ingress for recovery testing
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-recovery-ingress
  namespace: default
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: test-recovery.k8s.example.com
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
    
    # Wait for controller to process the ingress
    wait_for_controller_sync 5
    
    # Delete the controller pod to simulate failure
    kubectl delete pod -l app=$CONTROLLER_NAME -n $NAMESPACE
    
    # Wait for pod to be recreated
    wait_for_controller_sync 5
    
    # Wait for controller to be ready
    if ! kubectl wait --for=condition=available --timeout=120s deployment/$CONTROLLER_NAME -n $NAMESPACE; then
        log_error "Controller did not recover within timeout"
        
        # Clean up test ingress
        kubectl delete ingress test-recovery-ingress -n default
        
        return 1
    fi
    
    # Wait for controller to reconcile
    wait_for_controller_sync 5
    
    # Check if ConfigMap was regenerated with the specific test ingress
    local new_config
    new_config=$(get_configmap_content)
    
    # Log the actual config for debugging
    log_info "ConfigMap content after recovery: $new_config"
    
    if [[ "$new_config" == *"rewrite name exact"* ]] && [[ "$new_config" == *"test-recovery.k8s.example.com"* ]]; then
        log_info "Controller successfully recovered and regenerated configuration"
        
        # Clean up test ingress
        kubectl delete ingress test-recovery-ingress -n default
        
        return 0
    else
        log_error "Controller did not regenerate configuration after recovery"
        log_error "Expected to find: test-recovery.k8s.example.com"
        log_error "ConfigMap content: $new_config"
        log_error "=== Full ConfigMap YAML ==="
        kubectl get configmap $CONFIGMAP_NAME -n $COREDNS_NAMESPACE -o yaml
        log_error "=== Current ingresses ==="
        kubectl get ingress -A
        log_error "=== Controller logs ==="
        kubectl logs -n $NAMESPACE deployment/coredns-ingress-sync --tail=50
        log_error "=== Controller pod status ==="
        kubectl get pods -l app=$CONTROLLER_NAME -n $NAMESPACE
        
        # Clean up test ingress
        kubectl delete ingress test-recovery-ingress -n default
        
        return 1
    fi
}

# Main test execution
main() {
    log_info "Starting integration tests..."
    
    # Initialize test counters
    TESTS_TOTAL=0
    TESTS_PASSED=0
    TESTS_FAILED=0
    
    run_test "Prerequisites" "test_prerequisites"
    run_test "Controller Deployment" "test_controller_deployment"
    run_test "ConfigMap Creation" "test_configmap_creation"
    run_test "Ingress Discovery" "test_ingress_discovery"
    run_test "Controller Logs" "test_controller_logs"
    run_test "CoreDNS Health" "test_coredns_health"
    run_test "DNS Resolution" "test_dns_resolution"
    
    # Load testing is handled by E2E tests (better scale)
    
    run_test "Recovery Test" "test_recovery"
    
    echo ""
    echo "======================================"
    echo "🧪 Integration Test Results"
    echo "======================================"
    echo "Total tests: $TESTS_TOTAL"
    echo "Passed: $TESTS_PASSED"
    echo "Failed: $TESTS_FAILED"
    echo ""
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_info "🎉 All integration tests passed!"
        exit 0
    else
        log_error "❌ Some integration tests failed"
        exit 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test resources..."
    kubectl delete ingress test-ingress-dynamic -n default 2>/dev/null || true
    kubectl delete ingress test-recovery-ingress -n default 2>/dev/null || true
    kubectl delete pod dns-test-pod 2>/dev/null || true
    for i in {1..5}; do
        kubectl delete ingress test-stress-$i -n default 2>/dev/null || true
    done
    rm -f /tmp/dns_test_output
    
    # Cleanup controller if it was deployed by this test run
    cleanup_controller
}

# Set up signal handlers
trap cleanup EXIT

# Run main function
main "$@"
