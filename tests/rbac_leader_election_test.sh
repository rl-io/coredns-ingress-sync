#!/bin/bash

# RBAC Leader Election Test
# Tests that Helm RBAC templates include proper leader election permissions

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

echo "=== RBAC Leader Election Test ==="

# Get the project root directory
PROJECT_ROOT="$(cd "$TEST_DIR/.." && pwd)"
CHART_PATH="$PROJECT_ROOT/helm/coredns-ingress-sync"

# Verify chart exists
if [[ ! -d "$CHART_PATH" ]]; then
    log_error "Helm chart not found at $CHART_PATH"
    exit 1
fi

if [[ ! -f "$CHART_PATH/Chart.yaml" ]]; then
    log_error "Chart.yaml not found at $CHART_PATH/Chart.yaml"
    exit 1
fi

log_info "Using Helm chart at: $CHART_PATH"

# Test that RBAC templates include leader election permissions
TEMPLATE_OUTPUT=$(helm template test-rbac "$CHART_PATH" --set rbac.create=true)

log_info "Checking RBAC permissions for leader election..."

# Check that coordination.k8s.io API group is included
if echo "$TEMPLATE_OUTPUT" | grep -q "coordination.k8s.io"; then
    log_info "✅ coordination.k8s.io API group found in RBAC"
else
    log_error "❌ coordination.k8s.io API group missing from RBAC"
    exit 1
fi

# Check that leases resource is included
if echo "$TEMPLATE_OUTPUT" | grep -q "leases"; then
    log_info "✅ leases resource found in RBAC"
else
    log_error "❌ leases resource missing from RBAC"
    exit 1
fi

# Check that necessary verbs are included for leases
REQUIRED_VERBS=("get" "list" "watch" "create" "update" "patch" "delete")
MISSING_VERBS=()

for verb in "${REQUIRED_VERBS[@]}"; do
    # Look for the verb in the leases section
    if echo "$TEMPLATE_OUTPUT" | grep -A 10 -B 10 "leases" | grep -q "\"$verb\""; then
        log_info "✅ Verb '$verb' found for leases"
    else
        log_warn "⚠️  Verb '$verb' not found for leases"
        MISSING_VERBS+=("$verb")
    fi
done

if [ ${#MISSING_VERBS[@]} -eq 0 ]; then
    log_info "✅ All required verbs found for leader election"
else
    log_error "❌ Missing verbs for leader election: ${MISSING_VERBS[*]}"
    exit 1
fi

# Check that leases permissions are in the local Role (not ClusterRole)
if echo "$TEMPLATE_OUTPUT" | awk '/kind: Role/,/---/' | grep -q "leases"; then
    log_info "✅ Leader election permissions correctly placed in local Role"
else
    log_error "❌ Leader election permissions not found in local Role"
    exit 1
fi

log_info "✅ RBAC leader election test PASSED"
