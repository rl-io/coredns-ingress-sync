#!/bin/bash

# End-to-end test: Gateway API (Gateway + HTTPRoute) support, additive to Ingress.
#
# Covers:
#   1. A basic HTTPRoute-only hostname resolves via its parent Gateway's
#      GatewayClass -> CNAME mapping.
#   2. The cross-source migration tiebreak: an Ingress and an HTTPRoute
#      claiming the same hostname with different target CNAMEs. Ingress wins
#      by default; adding the priority annotation to the HTTPRoute flips it.

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$TEST_DIR/.." && pwd)"
export PROJECT_DIR
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} E2E gateway tests cannot run against this cluster"
    exit 1
fi

echo "🌐 End-to-End Gateway API Test"
echo "=============================="

# Configuration
TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}
GW_NS="default"
GW_NAME="gateway-test"
GATEWAY_CNAME="traefik.traefik.svc.cluster.local."
INGRESS_CNAME="test-target.cluster.local"
PRIORITY_ANN="coredns-ingress-sync-priority"

ROUTE_ONLY_NAME="gwroute-only-test"
ROUTE_ONLY_HOSTNAME="gwroute.${TEST_DOMAIN}"

TIEBREAK_ING_NAME="tiebreak-ingress-test"
TIEBREAK_ROUTE_NAME="tiebreak-route-test"
TIEBREAK_HOSTNAME="tiebreak.${TEST_DOMAIN}"

ensure_ready() {
    if ! ensure_controller_deployed; then
        return 1
    fi
}

cleanup_resources() {
    kubectl delete httproute ${ROUTE_ONLY_NAME} -n ${GW_NS} 2>/dev/null || true
    kubectl delete httproute ${TIEBREAK_ROUTE_NAME} -n ${GW_NS} 2>/dev/null || true
    kubectl delete ingress ${TIEBREAK_ING_NAME} -n ${GW_NS} 2>/dev/null || true
    kubectl delete gateway ${GW_NAME} -n ${GW_NS} 2>/dev/null || true
}

trap cleanup_resources EXIT

wait_for_hostname_present() {
    local hostname="$1"
    local cname="${2:-}"
    local attempts=0
    local max_attempts=30
    local interval=2
    while [ $attempts -lt $max_attempts ]; do
        if hostname_in_configmap "${hostname}"; then
            if [ -z "$cname" ]; then
                return 0
            fi
            local content
            content=$(get_configmap_content)
            if echo "$content" | grep -E "rewrite name exact ${hostname} ${cname}" >/dev/null 2>&1; then
                return 0
            fi
        fi
        sleep $interval
        attempts=$((attempts + 1))
    done
    return 1
}

wait_for_hostname_absent() {
    local hostname="$1"
    local attempts=0
    local max_attempts=30
    local interval=2
    while [ $attempts -lt $max_attempts ]; do
        if ! hostname_in_configmap "${hostname}"; then
            return 0
        fi
        sleep $interval
        attempts=$((attempts + 1))
    done
    return 1
}

apply_gateway() {
    kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${GW_NAME}
  namespace: ${GW_NS}
spec:
  gatewayClassName: traefik
  listeners:
  - name: http
    protocol: HTTP
    port: 80
EOF
}

test_httproute_only() {
    log_info "Testing HTTPRoute-only hostname resolves via Gateway's GatewayClass"

    kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${ROUTE_ONLY_NAME}
  namespace: ${GW_NS}
spec:
  parentRefs:
  - name: ${GW_NAME}
  hostnames:
  - ${ROUTE_ONLY_HOSTNAME}
  rules:
  - backendRefs:
    - name: test-service
      port: 80
EOF

    if ! wait_for_hostname_present "${ROUTE_ONLY_HOSTNAME}" "${GATEWAY_CNAME}"; then
        log_error "Expected HTTPRoute hostname missing or wrong CNAME: ${ROUTE_ONLY_HOSTNAME}"
        get_configmap_content | sed 's/^/[CFG] /' || true
        kubectl logs -n "$NAMESPACE" deployment/"$CONTROLLER_NAME" --tail=100 2>/dev/null | sed 's/^/[LOG] /' || true
        return 1
    fi

    log_info "HTTPRoute-only test passed"
    return 0
}

test_migration_tiebreak() {
    log_info "Testing Ingress/HTTPRoute cross-source tiebreak (Ingress wins by default)"

    kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${TIEBREAK_ING_NAME}
  namespace: ${GW_NS}
spec:
  ingressClassName: nginx
  rules:
  - host: ${TIEBREAK_HOSTNAME}
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

    kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${TIEBREAK_ROUTE_NAME}
  namespace: ${GW_NS}
spec:
  parentRefs:
  - name: ${GW_NAME}
  hostnames:
  - ${TIEBREAK_HOSTNAME}
  rules:
  - backendRefs:
    - name: test-service
      port: 80
EOF

    if ! wait_for_hostname_present "${TIEBREAK_HOSTNAME}" "${INGRESS_CNAME}"; then
        log_error "Expected Ingress to win tiebreak by default for: ${TIEBREAK_HOSTNAME}"
        get_configmap_content | sed 's/^/[CFG] /' || true
        kubectl logs -n "$NAMESPACE" deployment/"$CONTROLLER_NAME" --tail=100 2>/dev/null | sed 's/^/[LOG] /' || true
        return 1
    fi

    log_info "Promoting HTTPRoute via priority annotation"
    if ! kubectl annotate httproute ${TIEBREAK_ROUTE_NAME} -n ${GW_NS} ${PRIORITY_ANN}="100" --overwrite; then
        log_error "Failed to annotate HTTPRoute with priority"
        return 1
    fi

    if ! wait_for_hostname_present "${TIEBREAK_HOSTNAME}" "${GATEWAY_CNAME}"; then
        log_error "Expected HTTPRoute to win tiebreak after priority annotation for: ${TIEBREAK_HOSTNAME}"
        get_configmap_content | sed 's/^/[CFG] /' || true
        kubectl logs -n "$NAMESPACE" deployment/"$CONTROLLER_NAME" --tail=100 2>/dev/null | sed 's/^/[LOG] /' || true
        return 1
    fi

    log_info "Migration tiebreak test passed"
    return 0
}

main() {
    ensure_ready || {
        log_error "Controller not ready"
        exit 1
    }

    # Build local image expected by values-test.yaml (pullPolicy: Never)
    if command -v docker &>/dev/null; then
        log_info "Building local image coredns-ingress-sync:latest for test"
        (cd "$PROJECT_DIR" && docker build -t coredns-ingress-sync:latest .) || {
            log_error "Failed to build local docker image"
            exit 1
        }
    else
        log_warn "docker not found; assuming image already present locally"
    fi

    # Reinstall controller with Gateway API support enabled, on top of the
    # legacy single-class Ingress config from values-test.yaml.
    helm uninstall "$CONTROLLER_NAME" -n "$NAMESPACE" --wait 2>/dev/null || true
    helm_install_controller "$CONTROLLER_NAME" "$NAMESPACE" "true" \
        "--set controller.gatewayClassMappings[0].gatewayClass=traefik --set controller.gatewayClassMappings[0].targetCNAME=${GATEWAY_CNAME} --set controller.logLevel=debug"

    apply_gateway

    local failed=0
    if ! test_httproute_only; then
        failed=1
    fi
    if ! test_migration_tiebreak; then
        failed=1
    fi

    if [ $failed -eq 0 ]; then
        log_info "✅ Gateway API E2E passed"
        exit 0
    else
        log_error "❌ Gateway API E2E failed"
        exit 1
    fi
}

main "$@"
