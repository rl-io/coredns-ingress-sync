#!/bin/bash

# Leader Election Integration Test
# Tests multi-replica leader election functionality

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

echo "=== Leader Election Integration Test ==="

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    log_error "Leader election tests cannot run against this cluster"
    exit 1
fi

# Configuration
NAMESPACE="test-leader-election"
RELEASE_NAME="test-leader-election"
REPLICAS=2
TIMEOUT_SECONDS=120

# Get the project root directory
PROJECT_ROOT="$(cd "$TEST_DIR/.." && pwd)"
CHART_PATH="$PROJECT_ROOT/helm/coredns-ingress-sync"

# Verify chart exists
if [[ ! -d "$CHART_PATH" ]]; then
    log_error "Helm chart not found at $CHART_PATH"
    exit 1
fi

log_info "Using Helm chart at: $CHART_PATH"

cleanup() {
    log_info "Cleaning up test resources..."
    # More aggressive cleanup
    helm uninstall "$RELEASE_NAME" -n "$NAMESPACE" 2>/dev/null || true
    kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --timeout=30s
    log_info "Cleanup completed"
}

# Function to check if pods are actually running and ready
check_pod_status() {
    local namespace=$1
    log_info "Checking detailed pod status..."
    
    kubectl get pods -n "$namespace" -o wide
    
    # Check for any failed pods
    FAILED_PODS=$(kubectl get pods -n "$namespace" --field-selector=status.phase=Failed --no-headers 2>/dev/null | wc -l || echo "0")
    if [ "$FAILED_PODS" -gt 0 ]; then
        log_error "Found $FAILED_PODS failed pods"
        kubectl get pods -n "$namespace" --field-selector=status.phase=Failed
        return 1
    fi
    
    # Check for pending pods
    PENDING_PODS=$(kubectl get pods -n "$namespace" --field-selector=status.phase=Pending --no-headers 2>/dev/null | wc -l || echo "0")
    if [ "$PENDING_PODS" -gt 0 ]; then
        log_warn "Found $PENDING_PODS pending pods"
        kubectl describe pods -n "$namespace" --field-selector=status.phase=Pending
    fi
    
    return 0
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Check if cluster is accessible
log_info "Checking cluster connectivity..."
if ! kubectl cluster-info >/dev/null 2>&1; then
    log_error "Cannot connect to Kubernetes cluster"
    exit 1
fi

# Clean up any existing test resources
log_info "Cleaning up any existing test resources..."
helm uninstall "$RELEASE_NAME" -n "$NAMESPACE" 2>/dev/null || true
kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --timeout=30s

# Create test namespace
log_info "Creating test namespace: $NAMESPACE"
kubectl create namespace "$NAMESPACE"

# Validate Helm chart first
log_info "Validating Helm chart..."
if ! helm lint "$CHART_PATH"; then
    log_error "Helm chart validation failed"
    exit 1
fi

# Install chart with multiple replicas
log_info "Installing chart with $REPLICAS replicas..."
log_info "Using image from values-test.yaml (ensure image is built and available)"

if ! helm install "$RELEASE_NAME" "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    --set replicaCount="$REPLICAS" \
    --set coreDNS.autoConfigure=false \
    --set image.pullPolicy=IfNotPresent \
    --values "$CHART_PATH/values-test.yaml" \
    --timeout="${TIMEOUT_SECONDS}s"; then
    
    log_error "Helm install failed"
    log_info "Deployment status:"
    kubectl get deployment -n "$NAMESPACE" || true
    log_info "Pod status:"
    kubectl get pods -n "$NAMESPACE" || true
    log_info "Events:"
    kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' || true
    exit 1
fi

# Wait for deployment to be ready (with timeout)
log_info "Waiting for deployment to be ready..."
if ! kubectl wait --for=condition=Available deployment -l app.kubernetes.io/name=coredns-ingress-sync \
    -n "$NAMESPACE" --timeout="${TIMEOUT_SECONDS}s"; then
    
    log_error "Deployment did not become available within timeout"
    check_pod_status "$NAMESPACE"
    
    # Show logs from pods for debugging
    log_info "Pod logs for debugging:"
    kubectl get pods -n "$NAMESPACE" -o name | while read pod; do
        log_info "Logs from $pod:"
        kubectl logs "$pod" -n "$NAMESPACE" --tail=20 || true
        echo "---"
    done
    exit 1
fi

# Wait for pods to be ready
log_info "Waiting for pods to be ready..."
if ! kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=coredns-ingress-sync \
    -n "$NAMESPACE" --timeout=60s; then
    
    log_error "Pods did not become ready within timeout"
    check_pod_status "$NAMESPACE"
    exit 1
fi

# Check that we have the expected number of replicas
RUNNING_PODS=$(kubectl get pods -l app.kubernetes.io/name=coredns-ingress-sync -n "$NAMESPACE" -o json | jq '.items | length')
log_info "Found $RUNNING_PODS running pods (expected: $REPLICAS)"

if [ "$RUNNING_PODS" -ne "$REPLICAS" ]; then
    log_error "Expected $REPLICAS pods, but found $RUNNING_PODS"
    kubectl get pods -n "$NAMESPACE"
    exit 1
fi

# Check for leader election lease
log_info "Checking for leader election lease..."
sleep 5  # Give time for leader election to occur

# Look for leader election lease in the namespace
LEASE_COUNT=$(kubectl get leases -n "$NAMESPACE" --field-selector metadata.name=coredns-ingress-sync-leader --no-headers 2>/dev/null | wc -l || echo "0")

if [ "$LEASE_COUNT" -eq "1" ]; then
    log_info "✅ Leader election lease found"
    LEADER_INFO=$(kubectl get lease coredns-ingress-sync-leader -n "$NAMESPACE" -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "unknown")
    log_info "Current leader: $LEADER_INFO"
else
    log_warn "Leader election lease not found or multiple leases exist (count: $LEASE_COUNT)"
    kubectl get leases -n "$NAMESPACE" || true
fi

# Check pod logs for leader election messages
log_info "Checking pod logs for leader election activity..."
PODS=($(kubectl get pods -l app.kubernetes.io/name=coredns-ingress-sync -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}'))

LEADER_COUNT=0
FOLLOWER_COUNT=0

for pod in "${PODS[@]}"; do
    log_info "Checking logs for pod: $pod"
    
    # Check recent logs for leader election indicators
    LOGS=$(kubectl logs "$pod" -n "$NAMESPACE" --tail=50 2>/dev/null || echo "")
    
    if echo "$LOGS" | grep -q "became leader\|successfully acquired lease\|leader election"; then
        log_info "  ✅ Pod $pod shows leader election activity"
        LEADER_COUNT=$((LEADER_COUNT + 1))
    else
        log_info "  ⏳ Pod $pod may be follower (no leader messages yet)"
        FOLLOWER_COUNT=$((FOLLOWER_COUNT + 1))
    fi
    
    # Show any error messages
    if echo "$LOGS" | grep -qi error; then
        log_warn "  ⚠️  Pod $pod has error messages:"
        echo "$LOGS" | grep -i error | head -3
    fi
done

# Test failover by deleting the leader pod
if [ "$LEASE_COUNT" -eq "1" ] && [ ${#PODS[@]} -gt 1 ]; then
    log_info "Testing leader failover..."
    
    # Get current leader
    CURRENT_LEADER=$(kubectl get lease coredns-ingress-sync-leader -n "$NAMESPACE" -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "unknown")
    log_info "Current leader: $CURRENT_LEADER"
    
    # Find and delete the leader pod (approximate match)
    for pod in "${PODS[@]}"; do
        if [[ "$CURRENT_LEADER" == *"$pod"* ]] || [ "$pod" == "${PODS[0]}" ]; then
            log_info "Deleting leader pod: $pod"
            kubectl delete pod "$pod" -n "$NAMESPACE"
            break
        fi
    done
    
    # Wait for new pod to start
    log_info "Waiting for pod replacement..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=coredns-ingress-sync \
        -n "$NAMESPACE" --timeout=60s
    
    # Wait a bit for leader re-election
    sleep 10
    
    # Check new leader
    NEW_LEADER=$(kubectl get lease coredns-ingress-sync-leader -n "$NAMESPACE" -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "unknown")
    log_info "New leader after failover: $NEW_LEADER"
    
    if [ "$NEW_LEADER" != "unknown" ] && [ "$NEW_LEADER" != "$CURRENT_LEADER" ]; then
        log_info "✅ Leader failover successful"
    else
        log_warn "⚠️  Leader failover may not have completed yet"
    fi
fi

# Final status check
log_info "Final status check..."
kubectl get pods -n "$NAMESPACE"
kubectl get leases -n "$NAMESPACE" 2>/dev/null || log_warn "No leases found"

# Summary
log_info "=== Leader Election Test Summary ==="
log_info "Replicas deployed: $REPLICAS"
log_info "Pods running: $RUNNING_PODS"
log_info "Leader election leases: $LEASE_COUNT"

if [ "$LEASE_COUNT" -eq "1" ] && [ "$RUNNING_PODS" -eq "$REPLICAS" ]; then
    log_info "✅ Leader election test PASSED"
    exit 0
else
    log_error "❌ Leader election test FAILED"
    log_error "Expected: 1 lease and $REPLICAS pods"
    log_error "Actual: $LEASE_COUNT leases and $RUNNING_PODS pods"
    exit 1
fi
