#!/bin/bash

# Enhanced E2E test suite to test both cluster-wide and namespace-scoped RBAC configurations

# Note: We don't use set -e here because we need to handle some expected failures gracefully
# in the controller readiness checks

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} E2E RBAC tests cannot run against this cluster"
    exit 1
fi

echo "🔐 End-to-End RBAC Test Suite"
echo "============================="

# Configuration
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}
CONTROLLER_IMAGE=${CONTROLLER_IMAGE:-coredns-ingress-sync:latest}
PROJECT_ROOT="$(cd "$TEST_DIR/.." && pwd)"
HELM_CHART_PATH="$PROJECT_ROOT/helm/coredns-ingress-sync"

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

test_result() {
    if [ $1 -eq 0 ]; then
        ((TESTS_PASSED++))
        log_info "✅ $2 PASSED"
    else
        ((TESTS_FAILED++))
        log_error "❌ $2 FAILED"
    fi
}

create_test_ingress() {
    local name=$1
    local namespace=$2
    local hostname=$3
    
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ${name}-service
  namespace: ${namespace}
spec:
  selector:
    app: ${name}
  ports:
  - port: 80
    targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${name}
  namespace: ${namespace}
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: ${hostname}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: ${name}-service
            port:
              number: 80
EOF
}

cleanup_test_ingress() {
    local name=$1
    local namespace=$2
    
    kubectl delete ingress ${name} -n ${namespace} --ignore-not-found=true
    kubectl delete service ${name}-service -n ${namespace} --ignore-not-found=true
}

# Test 1: Cluster-wide RBAC (watchNamespaces not set)
test_cluster_wide_rbac() {
    log_info "Testing cluster-wide RBAC configuration..."
    
    # Clean up any existing deployment
    helm uninstall coredns-ingress-sync -n coredns-ingress-sync --ignore-not-found=true 2>/dev/null || true
    kubectl delete namespace coredns-ingress-sync --ignore-not-found=true 2>/dev/null || true
    sleep 5
    
    # Deploy with cluster-wide configuration (no watchNamespaces)
    log_info "Deploying controller with cluster-wide RBAC..."
    if ! helm install coredns-ingress-sync "$HELM_CHART_PATH" \
        --namespace coredns-ingress-sync \
        --create-namespace \
        --wait \
        --timeout=300s \
        --values "$HELM_CHART_PATH/values-test.yaml"; then
        log_error "Failed to deploy controller with cluster-wide RBAC"
        return 1
    fi
    
    # Wait for controller to be ready
    if ! kubectl wait --for=condition=available --timeout=120s deployment/coredns-ingress-sync -n coredns-ingress-sync; then
        log_error "Controller failed to become ready"
        return 1
    fi
    
    # Create test ingresses in multiple namespaces
    log_info "Creating test ingresses in multiple namespaces..."
    
    # Create test namespace
    kubectl create namespace rbac-test-1 --dry-run=client -o yaml | kubectl apply -f -
    kubectl create namespace rbac-test-2 --dry-run=client -o yaml | kubectl apply -f -
    
    # Create ingresses in different namespaces
    create_test_ingress "cluster-test-1" "default" "cluster-test-1.$TEST_DOMAIN"
    create_test_ingress "cluster-test-2" "rbac-test-1" "cluster-test-2.$TEST_DOMAIN"
    create_test_ingress "cluster-test-3" "rbac-test-2" "cluster-test-3.$TEST_DOMAIN"
    
    # Wait for controller to process
    wait_for_controller_sync
    sleep 5
    
    # Check if all hostnames appear in ConfigMap
    local success=true
    for hostname in "cluster-test-1.$TEST_DOMAIN" "cluster-test-2.$TEST_DOMAIN" "cluster-test-3.$TEST_DOMAIN"; do
        if ! hostname_in_configmap "$hostname"; then
            log_error "Hostname $hostname not found in ConfigMap"
            success=false
        fi
    done
    
    # Check controller logs for RBAC errors
    local logs
    logs=$(kubectl logs deployment/coredns-ingress-sync -n coredns-ingress-sync --tail=50)
    if echo "$logs" | grep -i "forbidden" > /dev/null; then
        log_error "Found RBAC forbidden errors in controller logs"
        echo "$logs" | grep -i "forbidden"
        success=false
    fi
    
    # Clean up test resources
    cleanup_test_ingress "cluster-test-1" "default"
    cleanup_test_ingress "cluster-test-2" "rbac-test-1"
    cleanup_test_ingress "cluster-test-3" "rbac-test-2"
    kubectl delete namespace rbac-test-1 rbac-test-2 --ignore-not-found=true
    
    if [ "$success" = true ]; then
        log_info "Cluster-wide RBAC test passed - controller can watch all namespaces"
        return 0
    else
        log_error "Cluster-wide RBAC test failed"
        local config
        config=$(get_configmap_content)
        log_error "ConfigMap content: $config"
        return 1
    fi
}

# Test 2: Namespace-scoped RBAC (watchNamespaces set)
test_namespace_scoped_rbac() {
    log_info "🔧 Starting namespace-scoped RBAC test..."
    log_info "Testing namespace-scoped RBAC configuration..."
    
    # Clean up any existing deployment
    log_info "Cleaning up existing deployments..."
    helm uninstall coredns-ingress-sync -n coredns-ingress-sync --ignore-not-found=true 2>/dev/null || true
    kubectl delete namespace coredns-ingress-sync --ignore-not-found=true 2>/dev/null || true
    
    # Wait for namespace deletion to complete
    local timeout=60
    local elapsed=0
    while kubectl get namespace coredns-ingress-sync >/dev/null 2>&1; do
        if [ $elapsed -ge $timeout ]; then
            log_warn "Namespace deletion timeout after ${timeout}s, continuing anyway"
            break
        fi
        sleep 2
        elapsed=$((elapsed + 2))
        log_info "Waiting for namespace deletion... (${elapsed}s)"
    done
    log_info "✅ Namespace cleanup completed"
    
    # Deploy with namespace-scoped configuration
    log_info "Deploying controller with namespace-scoped RBAC (watching: default,test-namespace)..."
    
    # Create temporary values file for namespace-scoped test
    local temp_values
    temp_values=$(mktemp)
    cat "$HELM_CHART_PATH/values-test.yaml" > "$temp_values"
    cat >> "$temp_values" <<EOF

# Namespace-scoped configuration for RBAC test
controller:
  watchNamespaces:
    - default
    - test-namespace
EOF
    
    if ! helm install coredns-ingress-sync "$HELM_CHART_PATH" \
        --namespace coredns-ingress-sync \
        --create-namespace \
        --wait \
        --timeout=300s \
        --values "$temp_values"; then
        log_error "Failed to deploy controller with namespace-scoped RBAC"
        rm -f "$temp_values"
        return 1
    fi
    
    rm -f "$temp_values"
    
    # Wait for controller to be ready
    if ! kubectl wait --for=condition=available --timeout=120s deployment/coredns-ingress-sync -n coredns-ingress-sync; then
        log_error "Controller failed to become ready"
        return 1
    fi
    
    # Create test ingresses in watched and unwatched namespaces
    log_info "Creating test ingresses in watched and unwatched namespaces..."
    
    # Create test namespaces
    log_info "Creating test namespaces..."
    log_info "Creating test-namespace..."
    if ! kubectl create namespace test-namespace --dry-run=client -o yaml | kubectl apply -f -; then
        log_error "Failed to create test-namespace"
        return 1
    fi
    
    log_info "Creating rbac-test-unwatched namespace..."
    if ! kubectl create namespace rbac-test-unwatched --dry-run=client -o yaml | kubectl apply -f -; then
        log_error "Failed to create rbac-test-unwatched namespace"
        return 1
    fi
    
    # Create ingresses
    log_info "Creating test ingresses..."
    log_info "Creating scoped-test-1 ingress in default namespace..."
    create_test_ingress "scoped-test-1" "default" "scoped-test-1.$TEST_DOMAIN"      # watched
    
    log_info "Creating scoped-test-2 ingress in test-namespace..."
    create_test_ingress "scoped-test-2" "test-namespace" "scoped-test-2.$TEST_DOMAIN"     # watched
    
    log_info "Creating scoped-test-3 ingress in rbac-test-unwatched namespace..."
    create_test_ingress "scoped-test-3" "rbac-test-unwatched" "scoped-test-3.$TEST_DOMAIN"  # unwatched
    
    # Wait for controller to process
    wait_for_controller_sync
    sleep 5
    
    # Check that watched namespaces are processed
    local success=true
    for hostname in "scoped-test-1.$TEST_DOMAIN" "scoped-test-2.$TEST_DOMAIN"; do
        if ! hostname_in_configmap "$hostname"; then
            log_error "Hostname $hostname from watched namespace not found in ConfigMap"
            success=false
        fi
    done
    
    # Check that unwatched namespace is NOT processed
    if hostname_in_configmap "scoped-test-3.$TEST_DOMAIN"; then
        log_error "Hostname scoped-test-3.$TEST_DOMAIN from unwatched namespace should NOT be in ConfigMap"
        success=false
    else
        log_info "✅ Correctly ignored ingress from unwatched namespace"
    fi
    
    # Check controller logs for RBAC errors
    local logs
    logs=$(kubectl logs deployment/coredns-ingress-sync -n coredns-ingress-sync --tail=50)
    if echo "$logs" | grep -i "forbidden" > /dev/null; then
        log_error "Found RBAC forbidden errors in controller logs"
        echo "$logs" | grep -i "forbidden"
        success=false
    fi
    
    # Clean up test resources
    cleanup_test_ingress "scoped-test-1" "default"
    cleanup_test_ingress "scoped-test-2" "test-namespace"
    cleanup_test_ingress "scoped-test-3" "rbac-test-unwatched"
    kubectl delete namespace test-namespace --ignore-not-found=true
    kubectl delete namespace rbac-test-unwatched --ignore-not-found=true
    
    if [ "$success" = true ]; then
        log_info "Namespace-scoped RBAC test passed - controller respects watchNamespaces"
        return 0
    else
        log_error "Namespace-scoped RBAC test failed"
        local config
        config=$(get_configmap_content)
        log_error "ConfigMap content: $config"
        return 1
    fi
}

# Test 3: RBAC permissions validation
test_rbac_permissions() {
    log_info "Testing RBAC permissions validation..."
    
    # Get current deployment configuration - handle both string and array formats
    local watch_namespaces_raw
    watch_namespaces_raw=$(helm get values coredns-ingress-sync -n coredns-ingress-sync -o json | jq -r '.controller.watchNamespaces')
    
    local watch_namespaces
    if [ "$watch_namespaces_raw" = "null" ] || [ "$watch_namespaces_raw" = "" ]; then
        watch_namespaces="cluster-wide"
        log_info "Current watchNamespaces setting: cluster-wide (not set)"
    elif echo "$watch_namespaces_raw" | jq -e 'type == "array"' > /dev/null 2>&1; then
        # It's a JSON array, extract the values
        watch_namespaces=$(echo "$watch_namespaces_raw" | jq -r '.[]' | tr '\n' ',' | sed 's/,$//')
        log_info "Current watchNamespaces setting: [$watch_namespaces] (array)"
    else
        # It's a string
        watch_namespaces="$watch_namespaces_raw"
        log_info "Current watchNamespaces setting: $watch_namespaces (string)"
    fi
    
    # Check RBAC resources exist
    local success=true
    
    if [ "$watch_namespaces" = "cluster-wide" ]; then
        # Should have cluster-wide permissions
        if ! kubectl get clusterrole coredns-ingress-sync-cluster > /dev/null 2>&1; then
            log_error "Expected cluster-wide ClusterRole not found"
            success=false
        else
            log_info "✅ Found cluster-wide ClusterRole"
        fi
        if ! kubectl get clusterrolebinding coredns-ingress-sync-cluster > /dev/null 2>&1; then
            log_error "Expected cluster-wide ClusterRoleBinding not found"
            success=false
        else
            log_info "✅ Found cluster-wide ClusterRoleBinding"
        fi
    else
        # Should have namespace-scoped permissions
        # Split the namespaces by comma
        IFS=',' read -ra NAMESPACES <<< "$watch_namespaces"
        for ns in "${NAMESPACES[@]}"; do
            ns=$(echo "$ns" | xargs) # trim whitespace
            if [ -n "$ns" ]; then
                if ! kubectl get role coredns-ingress-sync-${ns} -n ${ns} > /dev/null 2>&1; then
                    log_error "Expected namespace-scoped Role not found in namespace ${ns}"
                    success=false
                else
                    log_info "✅ Found namespace-scoped Role in namespace ${ns}"
                fi
                if ! kubectl get rolebinding coredns-ingress-sync-${ns} -n ${ns} > /dev/null 2>&1; then
                    log_error "Expected namespace-scoped RoleBinding not found in namespace ${ns}"
                    success=false
                else
                    log_info "✅ Found namespace-scoped RoleBinding in namespace ${ns}"
                fi
            fi
        done
    fi
    
    # Should always have CoreDNS cluster permissions
    if ! kubectl get clusterrole coredns-ingress-sync-coredns > /dev/null 2>&1; then
        log_error "Expected CoreDNS ClusterRole not found"
        success=false
    fi
    if ! kubectl get clusterrolebinding coredns-ingress-sync-coredns > /dev/null 2>&1; then
        log_error "Expected CoreDNS ClusterRoleBinding not found"
        success=false
    fi
    
    # Should always have leader election permissions
    if ! kubectl get role coredns-ingress-sync-leader-election -n coredns-ingress-sync > /dev/null 2>&1; then
        log_error "Expected leader election Role not found"
        success=false
    fi
    if ! kubectl get rolebinding coredns-ingress-sync-leader-election -n coredns-ingress-sync > /dev/null 2>&1; then
        log_error "Expected leader election RoleBinding not found"
        success=false
    fi
    
    if [ "$success" = true ]; then
        log_info "RBAC permissions validation passed"
        return 0
    else
        log_error "RBAC permissions validation failed"
        return 1
    fi
}

# Run all tests
echo ""
log_info "Starting RBAC configuration tests..."

log_info "🧪 Running Test 1: Cluster-wide RBAC test..."
test_cluster_wide_rbac
test_result $? "Cluster-wide RBAC test"

log_info "🧪 Running Test 2: Namespace-scoped RBAC test..."
test_namespace_scoped_rbac  
test_result $? "Namespace-scoped RBAC test"

log_info "🧪 Running Test 3: RBAC permissions validation..."
test_rbac_permissions
test_result $? "RBAC permissions validation"

# Clean up final deployment
log_info "Cleaning up test deployment..."
helm uninstall coredns-ingress-sync -n coredns-ingress-sync --ignore-not-found=true 2>/dev/null || true
kubectl delete namespace coredns-ingress-sync --ignore-not-found=true 2>/dev/null || true

# Print results
echo ""
echo "======================================="
echo "🔐 E2E RBAC Test Results"
echo "======================================="
echo "Passed: $TESTS_PASSED"
echo "Failed: $TESTS_FAILED"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    log_info "✅ All RBAC tests passed!"
    exit 0
else
    log_error "❌ Some RBAC tests failed"
    exit 1
fi
