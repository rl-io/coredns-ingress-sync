#!/bin/bash

# End-to-end test: annotation flip should prune and re-add rewrites without restart

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$TEST_DIR/.." && pwd)"
export PROJECT_DIR
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} E2E annotation flip tests cannot run against this cluster"
    exit 1
fi

echo "🔁 End-to-End Annotation Flip Test"
echo "=================================="

# Configuration
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}
ING_NAME="flip-test"
ING_NS="default"
ANN_KEY="coredns-ingress-sync-enabled"
HOSTNAME="flip.${TEST_DOMAIN}"

ensure_ready() {
    if ! ensure_controller_deployed; then
        return 1
    fi
}

cleanup_resources() {
    kubectl delete ingress ${ING_NAME} -n ${ING_NS} 2>/dev/null || true
}

trap cleanup_resources EXIT

test_annotation_flip() {
    log_info "Testing annotation flip true -> false -> true"

    ensure_ready || return 1

    # Build local image expected by values-test.yaml (pullPolicy: Never)
    if command -v docker &>/dev/null; then
        log_info "Building local image coredns-ingress-sync:latest for test"
        (cd "$PROJECT_DIR" && docker build -t coredns-ingress-sync:latest .) || {
            log_error "Failed to build local docker image"
            return 1
        }
    else
        log_warn "docker not found; assuming image already present locally"
    fi

    # Reinstall controller cleanly with explicit annotation key
    helm uninstall "$CONTROLLER_NAME" -n "$NAMESPACE" --wait 2>/dev/null || true
    helm_install_controller "$CONTROLLER_NAME" "$NAMESPACE" "true" "--set controller.annotationEnabledKey=${ANN_KEY} --set controller.logLevel=debug"

    # Create ingress with annotation explicitly true
    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${ING_NAME}
  namespace: ${ING_NS}
  annotations:
    ${ANN_KEY}: "true"
spec:
  ingressClassName: nginx
  rules:
  - host: ${HOSTNAME}
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

    # Wait for hostname to appear (robust polling)
    local attempts=0
    local max_attempts=30
    local interval=2
    while [ $attempts -lt $max_attempts ]; do
        if hostname_in_configmap "${HOSTNAME}"; then
            break
        fi
        sleep $interval
        attempts=$((attempts + 1))
    done
    if [ $attempts -ge $max_attempts ]; then
        log_error "Expected hostname missing after create: ${HOSTNAME}"
        get_configmap_content | sed 's/^/[CFG] /' || true
        return 1
    fi

    # Flip annotation to false
    log_info "Patching annotation to false"
    if ! kubectl annotate ingress ${ING_NAME} -n ${ING_NS} ${ANN_KEY}="false" --overwrite; then
        log_error "Failed to annotate ingress to false"
        return 1
    fi

    # Wait for hostname to be removed
    attempts=0
    while [ $attempts -lt $max_attempts ]; do
        if ! hostname_in_configmap "${HOSTNAME}"; then
            break
        fi
        sleep $interval
        attempts=$((attempts + 1))
    done
    if [ $attempts -ge $max_attempts ]; then
        log_error "Hostname still present after disabling: ${HOSTNAME}"
        get_configmap_content | sed 's/^/[CFG] /' || true
        kubectl logs -n "$NAMESPACE" deployment/"$CONTROLLER_NAME" --tail=100 2>/dev/null | sed 's/^/[LOG] /' || true
        return 1
    fi

    # Flip annotation back to true to ensure re-add works
    log_info "Patching annotation back to true"
    if ! kubectl annotate ingress ${ING_NAME} -n ${ING_NS} ${ANN_KEY}="true" --overwrite; then
        log_error "Failed to annotate ingress back to true"
        return 1
    fi

    # Wait for hostname to be re-added
    attempts=0
    while [ $attempts -lt $max_attempts ]; do
        if hostname_in_configmap "${HOSTNAME}"; then
            break
        fi
        sleep $interval
        attempts=$((attempts + 1))
    done
    if [ $attempts -ge $max_attempts ]; then
        log_error "Expected hostname missing after re-enable: ${HOSTNAME}"
        get_configmap_content | sed 's/^/[CFG] /' || true
        kubectl logs -n "$NAMESPACE" deployment/"$CONTROLLER_NAME" --tail=100 2>/dev/null | sed 's/^/[LOG] /' || true
        return 1
    fi

    log_info "Annotation flip test passed"
    return 0
}

main() {
    if test_annotation_flip; then
        log_info "✅ Annotation flip E2E passed"
        exit 0
    else
        log_error "❌ Annotation flip E2E failed"
        exit 1
    fi
}

main "$@"
