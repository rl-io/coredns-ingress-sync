#!/bin/bash

# End-to-end exclusion tests for coredns-ingress-sync controller

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Ensure PROJECT_DIR is set so helpers derive HELM_CHART_PATH correctly
PROJECT_DIR="$(cd "$TEST_DIR/.." && pwd)"
export PROJECT_DIR
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} E2E exclusion tests cannot run against this cluster"
    exit 1
fi

echo "🚫 End-to-End Exclusions Test Suite"
echo "==================================="

# Configuration
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}

ensure_ready() {
    # Make sure controller exists and is ready
    if ! ensure_controller_deployed; then
        return 1
    fi
}

# Test 1: Annotation-based exclusion using controller.annotationEnabledKey (false-like disables)
test_annotation_exclusion() {
    log_info "Testing annotation-based exclusion..."

  ensure_ready || return 1

  # Ensure default annotation key is used (can be overridden here if needed)
  # Reinstall to make the test isolated and deterministic
  helm uninstall "$CONTROLLER_NAME" -n "$NAMESPACE" --wait 2>/dev/null || true
  helm_install_controller "$CONTROLLER_NAME" "$NAMESPACE" "true" "--set controller.annotationEnabledKey=coredns-ingress-sync-enabled"

    # Create control ingress (should be included)
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: anno-allowed
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: anno-allowed.${TEST_DOMAIN}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-service
            port:
              number: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: anno-excluded
  namespace: default
  annotations:
    coredns-ingress-sync-enabled: "false"
spec:
  ingressClassName: nginx
  rules:
  - host: anno-excluded.${TEST_DOMAIN}
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

    wait_for_controller_sync 5

    # Verify control hostname present
    if ! hostname_in_configmap "anno-allowed.${TEST_DOMAIN}"; then
        log_error "Expected hostname missing: anno-allowed.${TEST_DOMAIN}"
        local cfg
        cfg=$(get_configmap_content)
        log_error "ConfigMap content: $cfg"
        kubectl delete ingress anno-allowed anno-excluded -n default 2>/dev/null || true
        return 1
    fi

    # Verify annotated excluded hostname absent
    if hostname_in_configmap "anno-excluded.${TEST_DOMAIN}"; then
        log_error "Excluded-by-annotation hostname found unexpectedly: anno-excluded.${TEST_DOMAIN}"
        local cfg
        cfg=$(get_configmap_content)
        log_error "ConfigMap content: $cfg"
        kubectl delete ingress anno-allowed anno-excluded -n default 2>/dev/null || true
        return 1
    fi

    # Cleanup
    kubectl delete ingress anno-allowed anno-excluded -n default 2>/dev/null || true
    log_info "Annotation-based exclusion test passed"
    return 0
}

# Test 2: Exclude by namespace
test_namespace_exclusion() {
    log_info "Testing namespace exclusion..."

    ensure_ready || return 1

    local ns="excluded-ns"
    create_test_namespace "$ns"

  # Reinstall controller with excludeNamespaces configured for a clean state
  helm uninstall "$CONTROLLER_NAME" -n "$NAMESPACE" --wait 2>/dev/null || true
  helm_install_controller "$CONTROLLER_NAME" "$NAMESPACE" "true" "--set controller.excludeNamespaces=${ns}"

    # Create one ingress in excluded namespace and one in default
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-in-excluded-ns
  namespace: ${ns}
spec:
  ingressClassName: nginx
  rules:
  - host: excluded-ns-app.${TEST_DOMAIN}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-service
            port:
              number: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-in-default-ns
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: included-ns-app.${TEST_DOMAIN}
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

  wait_for_controller_sync 5

  # Verify included (allow time for async reconciliation)
  if ! wait_for_condition "hostname_in_configmap \"included-ns-app.${TEST_DOMAIN}\"" 30 2; then
        log_error "Expected hostname missing: included-ns-app.${TEST_DOMAIN}"
        local cfg
        cfg=$(get_configmap_content)
        log_error "ConfigMap content: $cfg"
        kubectl delete ingress app-in-default-ns -n default 2>/dev/null || true
        kubectl delete ingress app-in-excluded-ns -n "$ns" 2>/dev/null || true
        kubectl delete namespace "$ns" --ignore-not-found=true 2>/dev/null || true
        return 1
    fi

    # Verify excluded
    if hostname_in_configmap "excluded-ns-app.${TEST_DOMAIN}"; then
        log_error "Excluded-by-namespace hostname found unexpectedly: excluded-ns-app.${TEST_DOMAIN}"
        local cfg
        cfg=$(get_configmap_content)
        log_error "ConfigMap content: $cfg"
        kubectl delete ingress app-in-default-ns -n default 2>/dev/null || true
        kubectl delete ingress app-in-excluded-ns -n "$ns" 2>/dev/null || true
        kubectl delete namespace "$ns" --ignore-not-found=true 2>/dev/null || true
        return 1
    fi

    # Cleanup resources used in this test
    kubectl delete ingress app-in-default-ns -n default 2>/dev/null || true
    kubectl delete ingress app-in-excluded-ns -n "$ns" 2>/dev/null || true
    kubectl delete namespace "$ns" --ignore-not-found=true 2>/dev/null || true
    log_info "Namespace exclusion test passed"
    return 0
}

# Test 3: Exclude by ingress name (global and namespace/name forms)
test_name_exclusion() {
    log_info "Testing ingress name exclusion..."

    ensure_ready || return 1

    local other_ns="other-ns"
    create_test_namespace "$other_ns"

  # Reinstall controller with excludeIngresses configured (escape comma for Helm)
  helm uninstall "$CONTROLLER_NAME" -n "$NAMESPACE" --wait 2>/dev/null || true
  helm_install_controller "$CONTROLLER_NAME" "$NAMESPACE" "true" "--set controller.excludeIngresses=excluded-ingress\\,${other_ns}/specific-excluded"

    # Create ingresses
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: excluded-ingress
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: excluded-name.${TEST_DOMAIN}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-service
            port:
              number: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: specific-excluded
  namespace: ${other_ns}
spec:
  ingressClassName: nginx
  rules:
  - host: excluded-specific.${TEST_DOMAIN}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-service
            port:
              number: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: allowed-ingress
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: allowed-name.${TEST_DOMAIN}
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

    wait_for_controller_sync 5

    # Allowed should be present
    if ! hostname_in_configmap "allowed-name.${TEST_DOMAIN}"; then
        log_error "Expected hostname missing: allowed-name.${TEST_DOMAIN}"
        local cfg
        cfg=$(get_configmap_content)
        log_error "ConfigMap content: $cfg"
        kubectl delete ingress allowed-ingress excluded-ingress -n default 2>/dev/null || true
        kubectl delete ingress specific-excluded -n "$other_ns" 2>/dev/null || true
        kubectl delete namespace "$other_ns" --ignore-not-found=true 2>/dev/null || true
        return 1
    fi

    # Excluded should be absent
    if hostname_in_configmap "excluded-name.${TEST_DOMAIN}" || hostname_in_configmap "excluded-specific.${TEST_DOMAIN}"; then
        log_error "One or more excluded-by-name hostnames found unexpectedly"
        local cfg
        cfg=$(get_configmap_content)
        log_error "ConfigMap content: $cfg"
        kubectl delete ingress allowed-ingress excluded-ingress -n default 2>/dev/null || true
        kubectl delete ingress specific-excluded -n "$other_ns" 2>/dev/null || true
        kubectl delete namespace "$other_ns" --ignore-not-found=true 2>/dev/null || true
        return 1
    fi

    # Cleanup
    kubectl delete ingress allowed-ingress excluded-ingress -n default 2>/dev/null || true
    kubectl delete ingress specific-excluded -n "$other_ns" 2>/dev/null || true
    kubectl delete namespace "$other_ns" --ignore-not-found=true 2>/dev/null || true
    log_info "Ingress name exclusion test passed"
    return 0
}

main() {
    local tests_passed=0
    local tests_failed=0

    if test_annotation_exclusion; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi

    if test_namespace_exclusion; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi

    if test_name_exclusion; then
        tests_passed=$((tests_passed + 1))
    else
        tests_failed=$((tests_failed + 1))
    fi

    echo ""
    echo "======================================"
    echo "🚫 Exclusions E2E Test Results"
    echo "======================================"
    echo "Passed: $tests_passed"
    echo "Failed: $tests_failed"
    echo ""

    if [ $tests_failed -eq 0 ]; then
        log_info "🎉 All exclusions E2E tests passed!"
        exit 0
    else
        log_error "❌ Some exclusions E2E tests failed"
        exit 1
    fi
}

cleanup() {
    log_info "Cleaning up exclusions E2E resources..."
    kubectl delete ingress anno-allowed anno-excluded -n default 2>/dev/null || true
    kubectl delete ingress app-in-default-ns -n default 2>/dev/null || true
    kubectl delete ingress app-in-excluded-ns -n excluded-ns 2>/dev/null || true
    kubectl delete namespace excluded-ns --ignore-not-found=true 2>/dev/null || true
    kubectl delete ingress allowed-ingress excluded-ingress -n default 2>/dev/null || true
    kubectl delete ingress specific-excluded -n other-ns 2>/dev/null || true
    kubectl delete namespace other-ns --ignore-not-found=true 2>/dev/null || true
}

trap cleanup EXIT

main "$@"
