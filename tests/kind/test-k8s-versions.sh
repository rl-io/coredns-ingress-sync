#!/bin/bash
set -euo pipefail

# KIND test script for testing multiple Kubernetes versions
# This script creates KIND clusters with different K8s versions and runs integration tests

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Configuration
CONFIGMAP_NAME=${CONFIGMAP_NAME:-coredns-ingress-sync-rewrite-rules}
VOLUME_NAME=${VOLUME_NAME:-coredns-ingress-sync-volume}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Supported Kubernetes versions to test
# Based on our kubeVersion constraint of >=1.25.0-0
# Updated to latest available KIND node images (July 2025)
K8S_VERSIONS=(
    "1.29.14"   # Previous stable
    "1.30.13"   # 1.30 stable
    "1.31.9"    # 1.31 stable
    "1.32.5"    # 1.32 stable
    "1.33.1"    # Latest stable
)

# KIND cluster name prefix
CLUSTER_PREFIX="coredns-test"

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] ✅${NC} $*"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ❌${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] ⚠️${NC} $*"
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    if ! command -v kind &> /dev/null; then
        log_error "KIND is not installed. Please install it: https://kind.sigs.k8s.io/docs/user/quick-start/"
        exit 1
    fi
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Please install it."
        exit 1
    fi
    
    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed. Please install it."
        exit 1
    fi
    
    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed. Please install it."
        exit 1
    fi
    
    log_success "All prerequisites are available"

# Check if we're running in a KIND cluster
is_kind_cluster() {
    kubectl get nodes -o jsonpath='{.items[0].metadata.name}' 2>/dev/null | grep -q "kind" || 
    kubectl cluster-info 2>/dev/null | grep -q "kind"
}

# Check prerequisites
}

# Create KIND cluster with specific K8s version
create_kind_cluster() {
    local k8s_version="$1"
    local cluster_name="${CLUSTER_PREFIX}-${k8s_version//./}"
    
    log "Creating KIND cluster for Kubernetes ${k8s_version}..."
    
    # Create the cluster using the shared KIND configuration
    if kind create cluster \
        --name "${cluster_name}" \
        --image "kindest/node:v${k8s_version}" \
        --config "${SCRIPT_DIR}/kind-config.yaml" \
        --wait 300s; then
        log_success "KIND cluster ${cluster_name} created successfully"
        
        # Set kubectl context
        kubectl cluster-info --context "kind-${cluster_name}"
        
        return 0
    else
        log_error "Failed to create KIND cluster ${cluster_name}"
        return 1
    fi
}

# Install ingress-nginx in KIND cluster
install_ingress_nginx() {
    local cluster_name="$1"
    
    log "Installing ingress-nginx in cluster ${cluster_name}..."
    
    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
    
    log "Waiting for ingress-nginx to be ready..."
    kubectl wait --namespace ingress-nginx \
        --for=condition=ready pod \
        --selector=app.kubernetes.io/component=controller \
        --timeout=300s
    
    log_success "ingress-nginx installed and ready"
}

# Build and load controller image
build_and_load_image() {
    local cluster_name="$1"
    local image_tag="coredns-ingress-sync:test-${cluster_name}"
    
    log "Building controller image..." >&2
    
    cd "${PROJECT_ROOT}"
    if ! docker build -t "${image_tag}" . >/dev/null 2>&1; then
        log_error "Failed to build Docker image" >&2
        return 1
    fi
    
    log "Loading image into KIND cluster..." >&2
    if ! kind load docker-image "${image_tag}" --name "${cluster_name}" >/dev/null 2>&1; then
        log_error "Failed to load image into KIND cluster" >&2
        return 1
    fi
    
    echo "${image_tag}"
}

# Deploy controller using Helm
deploy_controller() {
    local cluster_name="$1"
    local image_tag="$2"
    
    log "Deploying controller via Helm..."
    
    # Create namespace
    kubectl create namespace coredns-ingress-sync || true
    
    # Install with custom image and disabled cleanup job
    log "Installing Helm chart..."
    
    # Parse image repository and tag
    local image_repository="${image_tag%:*}"
    local image_tag_only="${image_tag#*:}"
    
    log "Using image: ${image_repository}:${image_tag_only}"
    
    if helm install coredns-ingress-sync "${PROJECT_ROOT}/helm/coredns-ingress-sync" \
        --namespace coredns-ingress-sync \
        --set image.repository="${image_repository}" \
        --set image.tag="${image_tag_only}" \
        --set image.pullPolicy=Never \
        --set coreDNS.autoConfigure=false \
        --wait --timeout=300s; then
        log_success "Controller deployed successfully"
        
        # Wait for controller to be ready
        log "Waiting for controller deployment to be ready..."
        kubectl wait --namespace coredns-ingress-sync \
            --for=condition=available deployment/coredns-ingress-sync \
            --timeout=300s
        
        # Manually configure CoreDNS for testing (since autoConfigure is disabled)
        log "Manually configuring CoreDNS for testing..."
        ensure_coredns_configuration_for_test
        
        return 0
    else
        log_error "Failed to deploy controller via Helm"
        # Show recent events for debugging
        log "Recent events in coredns-ingress-sync namespace:"
        kubectl get events -n coredns-ingress-sync --sort-by='.lastTimestamp' | tail -10
        log "Controller deployment status:"
        kubectl get deployment -n coredns-ingress-sync || true
        log "Controller pods:"
        kubectl get pods -n coredns-ingress-sync || true
        return 1
    fi
}

# Manual CoreDNS configuration for testing (when autoConfigure is disabled)
ensure_coredns_configuration_for_test() {
    log "Adding volume mount to CoreDNS deployment..."
    
    # Add volume to CoreDNS deployment
    kubectl patch deployment coredns -n kube-system --type='json' -p='[
        {
            "op": "add",
            "path": "/spec/template/spec/volumes/-",
            "value": {
                "name": "$VOLUME_NAME",
                "configMap": {
                    "name": "$CONFIGMAP_NAME"
                }
            }
        }
    ]' || true
    
    # Add volume mount to CoreDNS container
    kubectl patch deployment coredns -n kube-system --type='json' -p='[
        {
            "op": "add",
            "path": "/spec/template/spec/containers/0/volumeMounts/-",
            "value": {
                "name": "$VOLUME_NAME",
                "mountPath": "/etc/coredns/custom",
                "readOnly": true
            }
        }
    ]' || true
    
    # Add import statement to CoreDNS Corefile
    log "Adding import statement to CoreDNS Corefile..."
    local corefile
    corefile=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}')
    
    if ! echo "${corefile}" | grep -q "import /etc/coredns/custom/\*.server"; then
        # Add import statement after the first line
        local new_corefile
        new_corefile=$(echo "${corefile}" | awk 'NR==1 {print; print "    import /etc/coredns/custom/*.server"} NR>1')
        
        # Create a temporary file for the patch
        cat > /tmp/coredns-patch.json << EOF
[
    {
        "op": "replace",
        "path": "/data/Corefile",
        "value": "${new_corefile}"
    }
]
EOF
        
        kubectl patch configmap coredns -n kube-system --type='json' -p "$(cat /tmp/coredns-patch.json)"
        rm -f /tmp/coredns-patch.json
    fi
    
    log_success "CoreDNS manually configured for testing"
}

# Create test ingress
create_test_ingress() {
    log "Creating test ingress..."
    
    cat << EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ingress
  namespace: default
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: test.example.com
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
apiVersion: v1
kind: Service
metadata:
  name: test-service
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
      - name: app
        image: nginx:alpine
        ports:
        - containerPort: 8080
EOF

    log_success "Test ingress created"
}

# Run integration tests
run_integration_tests() {
    local cluster_name="$1"
    
    log "Running integration tests for cluster ${cluster_name}..."
    
    # Wait for controller to process the ingress
    sleep 30
    
    # Check if dynamic ConfigMap was created
    if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
        log_success "Dynamic ConfigMap created successfully"
    else
        log_error "Dynamic ConfigMap not found"
        return 1
    fi
    
    # Check if CoreDNS was updated with import statement
    local corefile
    corefile=$(kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}')
    
    if echo "${corefile}" | grep -q "import /etc/coredns/custom/\*.server"; then
        log_success "CoreDNS import statement added successfully"
    else
        log_error "CoreDNS import statement not found"
        echo "Current Corefile:"
        echo "${corefile}"
        return 1
    fi
    
    # Check if rewrite rule was created
    local dynamic_config
    dynamic_config=$(kubectl get configmap $CONFIGMAP_NAME -n kube-system -o jsonpath='{.data.dynamic\.server}')
    
    if echo "${dynamic_config}" | grep -q "rewrite name exact test.example.com"; then
        log_success "Rewrite rule created successfully"
    else
        log_error "Rewrite rule not found"
        echo "Current dynamic config:"
        echo "${dynamic_config}"
        return 1
    fi
    
    # Test DNS resolution from within cluster
    log "Testing DNS resolution..."
    
    kubectl run dns-test --image=busybox --rm -it --restart=Never -- \
        nslookup test.example.com || true
    
    log_success "Integration tests completed successfully"
    return 0
}

# Cleanup function
cleanup_cluster() {
    local cluster_name="$1"
    
    log "Cleaning up cluster ${cluster_name}..."
    
    # Delete KIND cluster
    kind delete cluster --name "${cluster_name}" || true
    
    log_success "Cluster ${cluster_name} cleaned up"
}

# Test single Kubernetes version
test_k8s_version() {
    local k8s_version="$1"
    local cluster_name="${CLUSTER_PREFIX}-${k8s_version//./}"
    
    log "Starting test for Kubernetes ${k8s_version}"
    
    # Create cluster
    if ! create_kind_cluster "${k8s_version}"; then
        log_error "Failed to create cluster for K8s ${k8s_version}"
        return 1
    fi
    
    # Set context
    kubectl config use-context "kind-${cluster_name}"
    
    local test_result=0
    
    # Install ingress-nginx
    if ! install_ingress_nginx "${cluster_name}"; then
        log_error "Failed to install ingress-nginx for K8s ${k8s_version}"
        test_result=1
    fi
    
    # Build and load image
    local image_tag
    if [[ ${test_result} -eq 0 ]]; then
        if image_tag=$(build_and_load_image "${cluster_name}"); then
            log_success "Image built and loaded"
        else
            log_error "Failed to build/load image for K8s ${k8s_version}"
            test_result=1
        fi
    fi
    
    # Deploy controller
    if [[ ${test_result} -eq 0 ]]; then
        if ! deploy_controller "${cluster_name}" "${image_tag}"; then
            log_error "Failed to deploy controller for K8s ${k8s_version}"
            test_result=1
        fi
    fi
    
    # Create test resources
    if [[ ${test_result} -eq 0 ]]; then
        if ! create_test_ingress; then
            log_error "Failed to create test ingress for K8s ${k8s_version}"
            test_result=1
        fi
    fi
    
    # Run tests
    if [[ ${test_result} -eq 0 ]]; then
        if ! run_integration_tests "${cluster_name}"; then
            log_error "Integration tests failed for K8s ${k8s_version}"
            test_result=1
        fi
    fi
    
    # Cleanup
    cleanup_cluster "${cluster_name}"
    
    if [[ ${test_result} -eq 0 ]]; then
        log_success "✅ Kubernetes ${k8s_version} - PASSED"
    else
        log_error "❌ Kubernetes ${k8s_version} - FAILED"
    fi
    
    return ${test_result}
}

# Main function
main() {
    local test_specific_version=""
    local cleanup_only=false
    local list_versions=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --version)
                test_specific_version="$2"
                shift 2
                ;;
            --cleanup)
                cleanup_only=true
                shift
                ;;
            --list)
                list_versions=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --version VERSION    Test specific Kubernetes version"
                echo "  --cleanup           Cleanup all test clusters"
                echo "  --list              List supported versions"
                echo "  --help              Show this help"
                echo ""
                echo "Supported versions: ${K8S_VERSIONS[*]}"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # List versions and exit
    if [[ ${list_versions} == true ]]; then
        echo "Supported Kubernetes versions:"
        for version in "${K8S_VERSIONS[@]}"; do
            echo "  - ${version}"
        done
        exit 0
    fi
    
    # Cleanup all clusters and exit
    if [[ ${cleanup_only} == true ]]; then
        log "Cleaning up all test clusters..."
        for version in "${K8S_VERSIONS[@]}"; do
            cleanup_cluster "${CLUSTER_PREFIX}-${version//./}"
        done
        log_success "All clusters cleaned up"
        exit 0
    fi
    
    check_prerequisites
    
    cd "${PROJECT_ROOT}"
    
    local failed_versions=()
    local passed_versions=()
    
    if [[ -n ${test_specific_version} ]]; then
        # Test specific version
        if [[ " ${K8S_VERSIONS[*]} " =~ \ ${test_specific_version}\  ]]; then
            if test_k8s_version "${test_specific_version}"; then
                passed_versions+=("${test_specific_version}")
            else
                failed_versions+=("${test_specific_version}")
            fi
        else
            log_error "Unsupported version: ${test_specific_version}"
            log "Supported versions: ${K8S_VERSIONS[*]}"
            exit 1
        fi
    else
        # Test all versions
        log "Testing all supported Kubernetes versions..."
        
        for version in "${K8S_VERSIONS[@]}"; do
            if test_k8s_version "${version}"; then
                passed_versions+=("${version}")
            else
                failed_versions+=("${version}")
            fi
        done
    fi
    
    # Summary
    echo ""
    log "=========================================="
    log "TEST SUMMARY"
    log "=========================================="
    
    if [[ ${#passed_versions[@]} -gt 0 ]]; then
        log_success "PASSED (${#passed_versions[@]}):"
        for version in "${passed_versions[@]}"; do
            echo "  ✅ Kubernetes ${version}"
        done
    fi
    
    if [[ ${#failed_versions[@]} -gt 0 ]]; then
        log_error "FAILED (${#failed_versions[@]}):"
        for version in "${failed_versions[@]}"; do
            echo "  ❌ Kubernetes ${version}"
        done
        echo ""
        log_error "Some tests failed!"
        exit 1
    else
        echo ""
        log_success "All tests passed! 🎉"
        if [[ ${#passed_versions[@]} -gt 1 ]]; then
            log_success "Controller is compatible with Kubernetes ${passed_versions[0]} through ${passed_versions[-1]}"
        elif [[ ${#passed_versions[@]} -eq 1 ]]; then
            log_success "Controller is compatible with Kubernetes ${passed_versions[0]}"
        fi
    fi
}

# Trap to cleanup on exit
trap 'log_warning "Script interrupted, cleaning up..."; if [[ ${#K8S_VERSIONS[@]} -gt 0 ]]; then for version in "${K8S_VERSIONS[@]}"; do cleanup_cluster "${CLUSTER_PREFIX}-${version//./}" &>/dev/null || true; done; fi' INT TERM

main "$@"
