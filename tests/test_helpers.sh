#!/bin/bash

# Test helper utilities and mock functions

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration variables
NAMESPACE=${NAMESPACE:-coredns-ingress-sync}
CONTROLLER_NAME=${CONTROLLER_NAME:-coredns-ingress-sync}
CONFIGMAP_NAME=${CONFIGMAP_NAME:-coredns-ingress-sync-rewrite-rules}
VOLUME_NAME=${VOLUME_NAME:-coredns-ingress-sync-volume}
COREDNS_NAMESPACE=${COREDNS_NAMESPACE:-kube-system}
CONTROLLER_DEPLOYED_BY_TEST=${CONTROLLER_DEPLOYED_BY_TEST:-false}

# Derived configuration variables
HELM_CHART_PATH=${HELM_CHART_PATH:-"$PROJECT_DIR/helm/coredns-ingress-sync"}
DEPLOYMENT_FULL_NAME="${CONTROLLER_NAME}"
CLUSTER_ROLE_NAME="${CONTROLLER_NAME}-coredns"
EXPECTED_IMPORT_STATEMENT="import /etc/coredns/custom/${CONTROLLER_NAME}/*.server"
EXPECTED_MOUNT_PATH="/etc/coredns/custom/${CONTROLLER_NAME}"

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Kubecontext safety check
check_kubecontext_safety() {
    local current_context
    local allowed_contexts=(
        "kind-test"
        "kind-coredns-test"
        "minikube"
        "docker-desktop"
        "k3s-default"
        "localhost"
        "orbstack"
        "dev"
        "development"
    )
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        echo -e "${YELLOW}[WARN]${NC} kubectl not found - skipping kubecontext safety check"
        return 0
    fi
    
    # Get current context
    current_context=$(kubectl config current-context 2>/dev/null)
    if [ $? -ne 0 ]; then
        echo -e "${YELLOW}[WARN]${NC} No kubernetes context found - tests will use mocks"
        return 0
    fi
    
    # Check if current context is safe for testing
    local is_safe=false
    for safe_context in "${allowed_contexts[@]}"; do
        if [[ "$current_context" == *"$safe_context"* ]]; then
            is_safe=true
            break
        fi
    done
    
    if [ "$is_safe" = false ]; then
        echo -e "${RED}[ERROR]${NC} Unsafe kubecontext detected: $current_context"
        echo -e "${RED}[ERROR]${NC} This appears to be a live/production cluster!"
        echo ""
        echo "Safe contexts for testing include:"
        for safe_context in "${allowed_contexts[@]}"; do
            echo "  - $safe_context"
        done
        echo ""
        echo "Current context: $current_context"
        echo ""
        echo "To run tests safely:"
        echo "  1. Switch to a test cluster: kubectl config use-context kind-test"
        echo "  2. Or set SKIP_KUBECONTEXT_CHECK=1 to override (NOT recommended)"
        echo "  3. Or add your test context to the allowed list in test_helpers.sh"
        echo ""
        
        # Check for override
        if [ "$SKIP_KUBECONTEXT_CHECK" = "1" ]; then
            echo -e "${YELLOW}[WARN]${NC} SKIP_KUBECONTEXT_CHECK=1 - proceeding anyway"
            return 0
        fi
        
        return 1
    fi
    
    echo -e "${GREEN}[INFO]${NC} Kubecontext safety check passed: $current_context"
    return 0
}

# Verify test environment is safe
verify_test_environment() {
    echo "🔒 Verifying test environment safety..."
    
    # Check kubecontext
    if ! check_kubecontext_safety; then
        echo -e "${RED}[ERROR]${NC} Test environment safety check failed"
        return 1
    fi
    
    # Check if we're in a test directory
    if [[ ! "$PWD" == *"test"* ]] && [[ ! "$PWD" == *"coredns-ingress-sync"* ]]; then
        echo -e "${YELLOW}[WARN]${NC} Not running from expected test directory"
    fi
    
    # Check for required tools
    local required_tools=("go" "kubectl")
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            echo -e "${YELLOW}[WARN]${NC} $tool not found - some tests may be skipped"
        fi
    done
    
    echo -e "${GREEN}[INFO]${NC} Test environment safety checks completed"
    return 0
}

# Mock Kubernetes client for testing
create_mock_k8s_client() {
    local mock_dir="/tmp/mock_k8s"
    mkdir -p "$mock_dir"
    
    # Create mock kubectl that returns predefined responses
    cat > "$mock_dir/kubectl" <<'EOF'
#!/bin/bash

# Mock kubectl for testing
case "$1 $2" in
    "get ingress")
        echo "test-ingress   nginx   example.com"
        ;;
    "get configmap")
        echo "coredns-ingress-sync-rewrite-rules   1      1m"
        ;;
    "apply -f")
        echo "ingress.networking.k8s.io/test-ingress created"
        ;;
    "delete ingress")
        echo "ingress.networking.k8s.io/test-ingress deleted"
        ;;
    *)
        echo "Mock kubectl: $*"
        ;;
esac
EOF
    
    chmod +x "$mock_dir/kubectl"
    export PATH="$mock_dir:$PATH"
}

# Generate test ingress YAML
generate_test_ingress() {
    local name="$1"
    local hostname="$2"
    local class="${3:-nginx}"
    
    cat <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: $name
  namespace: default
spec:
  ingressClassName: $class
  rules:
  - host: $hostname
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
}

# Create test namespace
create_test_namespace() {
    local namespace="$1"
    kubectl create namespace "$namespace" 2>/dev/null || true
}

# Clean up test namespace
cleanup_test_namespace() {
    local namespace="$1"
    kubectl delete namespace "$namespace" --ignore-not-found=true
}

# Wait for condition with timeout
wait_for_condition() {
    local condition="$1"
    local timeout="${2:-60}"
    local interval="${3:-2}"
    
    local count=0
    while [ $count -lt $timeout ]; do
        if eval "$condition"; then
            return 0
        fi
        sleep "$interval"
        count=$((count + interval))
    done
    
    return 1
}

# Generate random string
random_string() {
    local length="${1:-8}"
    head /dev/urandom | tr -dc A-Za-z0-9 | head -c "$length"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Log with timestamp
log_with_timestamp() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Create test ConfigMap
create_test_configmap() {
    local name="$1"
    local namespace="$2"
    local data="$3"
    
    kubectl create configmap "$name" --from-literal=data="$data" -n "$namespace"
}

# Get ConfigMap data
get_configmap_data() {
    local name="$1"
    local namespace="$2"
    local key="$3"
    
    kubectl get configmap "$name" -n "$namespace" -o jsonpath="{.data.$key}" 2>/dev/null || echo ""
}

# Check if resource exists
resource_exists() {
    local resource="$1"
    local name="$2"
    local namespace="${3:-default}"
    
    kubectl get "$resource" "$name" -n "$namespace" >/dev/null 2>&1
}

# Wait for pod to be ready
wait_for_pod_ready() {
    local pod_name="$1"
    local namespace="${2:-default}"
    local timeout="${3:-60}"
    
    kubectl wait --for=condition=ready pod/"$pod_name" -n "$namespace" --timeout="${timeout}s"
}

# Get pod logs
get_pod_logs() {
    local pod_name="$1"
    local namespace="${2:-default}"
    local lines="${3:-50}"
    
    kubectl logs "$pod_name" -n "$namespace" --tail="$lines"
}

# Create test service
create_test_service() {
    local name="$1"
    local namespace="${2:-default}"
    local port="${3:-80}"
    
    kubectl create service clusterip "$name" --tcp="$port:$port" -n "$namespace"
}

# Port forward service
port_forward_service() {
    local service="$1"
    local namespace="${2:-default}"
    local local_port="${3:-8080}"
    local remote_port="${4:-80}"
    
    kubectl port-forward service/"$service" -n "$namespace" "$local_port:$remote_port" &
    local pid=$!
    
    # Wait for port forward to be ready
    sleep 2
    
    echo "$pid"
}

# Test HTTP endpoint
test_http_endpoint() {
    local url="$1"
    local expected_code="${2:-200}"
    local timeout="${3:-10}"
    
    local actual_code
    actual_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout "$timeout" "$url")
    
    [ "$actual_code" = "$expected_code" ]
}

# Generate load test data
generate_load_test_data() {
    local count="$1"
    local prefix="${2:-test}"
    local domain="${3:-example.com}"
    
    for i in $(seq 1 "$count"); do
        echo "$prefix-$i.$domain"
    done
}

# Measure execution time
measure_time() {
    local command="$1"
    local start_time
    local end_time
    
    start_time=$(date +%s.%N)
    eval "$command"
    end_time=$(date +%s.%N)
    
    echo "$(echo "$end_time - $start_time" | bc)"
}

# Validate DNS name
validate_dns_name() {
    local name="$1"
    
    # Basic DNS name validation
    if [[ "$name" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$ ]]; then
        return 0
    else
        return 1
    fi
}

# Extract domain from hostname
extract_domain() {
    local hostname="$1"
    echo "$hostname" | cut -d'.' -f2-
}

# Check if string contains substring
contains() {
    local string="$1"
    local substring="$2"
    
    [[ "$string" == *"$substring"* ]]
}

# Retry command with exponential backoff
retry_with_backoff() {
    local command="$1"
    local max_attempts="${2:-5}"
    local base_delay="${3:-1}"
    
    local attempt=1
    local delay="$base_delay"
    
    while [ $attempt -le $max_attempts ]; do
        if eval "$command"; then
            return 0
        fi
        
        if [ $attempt -eq $max_attempts ]; then
            return 1
        fi
        
        sleep "$delay"
        delay=$((delay * 2))
        attempt=$((attempt + 1))
    done
    
    return 1
}

# Create temporary file
create_temp_file() {
    local prefix="${1:-test}"
    local suffix="${2:-.tmp}"
    
    mktemp -t "${prefix}.XXXXXX${suffix}"
}

# Check if port is available
port_available() {
    local port="$1"
    
    ! lsof -i ":$port" >/dev/null 2>&1
}

# Find available port
find_available_port() {
    local start_port="${1:-8080}"
    local end_port="${2:-9000}"
    
    for port in $(seq "$start_port" "$end_port"); do
        if port_available "$port"; then
            echo "$port"
            return 0
        fi
    done
    
    return 1
}

# Remove duplicate configuration and logging functions - they are already defined above

# Controller deployment functions
verify_controller_running() {
    log_info "Verifying controller is running and healthy..."
    
    # Check if controller deployment exists
    if ! kubectl get deployment $CONTROLLER_NAME -n $NAMESPACE &> /dev/null; then
        log_error "Controller deployment '$CONTROLLER_NAME' not found in namespace '$NAMESPACE'"
        return 1
    fi
    
    # Check if deployment is available
    if ! kubectl wait --for=condition=available --timeout=60s deployment/$CONTROLLER_NAME -n $NAMESPACE &> /dev/null; then
        log_error "Controller deployment not ready within 60 seconds"
        
        # Show pod status for debugging
        log_info "Pod status for debugging:"
        kubectl get pods -n $NAMESPACE -l app=$CONTROLLER_NAME || true
        kubectl describe deployment $CONTROLLER_NAME -n $NAMESPACE || true
        return 1
    fi
    
    # Check if pods are actually running
    local ready_pods
    ready_pods=$(kubectl get pods -n $NAMESPACE -l app=$CONTROLLER_NAME -o jsonpath='{.items[?(@.status.phase=="Running")].metadata.name}' | wc -w)
    
    if [ "$ready_pods" -eq 0 ]; then
        log_error "No controller pods are in Running state"
        return 1
    fi
    
    log_info "Controller is running with $ready_pods pod(s)"
    return 0
}

ensure_controller_deployed() {
    log_info "Ensuring controller is deployed..."
    
    # First try to verify if already running
    if verify_controller_running; then
        log_info "Controller is already deployed and running"
        return 0
    fi
    
    # Check if controller deployment exists but not ready
    if kubectl get deployment $CONTROLLER_NAME -n $NAMESPACE &> /dev/null; then
        log_warn "Controller deployment exists but not ready, waiting..."
        
        # Wait longer and verify again
        if kubectl wait --for=condition=available --timeout=120s deployment/$CONTROLLER_NAME -n $NAMESPACE &> /dev/null; then
            log_info "Controller deployment is now ready"
            return 0
        else
            log_error "Controller deployment still not ready after 120 seconds"
            return 1
        fi
    else
        log_info "Controller deployment not found. Deploying via Helm..."
        
        # Deploy controller using Helm
        if deploy_controller; then
            log_info "Controller deployed successfully"
            CONTROLLER_DEPLOYED_BY_TEST=true
            return 0
        else
            log_error "Failed to deploy controller"
            return 1
        fi
    fi
}

deploy_controller() {
    log_info "Deploying controller using Helm chart..."
    
    # Check if helm is available
    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed"
        return 1
    fi
    
    # Get the project root directory
    PROJECT_ROOT="$(cd "$TEST_DIR/.." && pwd)"
    HELM_CHART_PATH="$PROJECT_ROOT/helm/coredns-ingress-sync"
    
    # Check if helm chart exists
    if [[ ! -d "$HELM_CHART_PATH" ]]; then
        log_error "Helm chart not found at $HELM_CHART_PATH"
        return 1
    fi
    
    # Install the helm chart with test values for local image
    HELM_VALUES_FILE="$HELM_CHART_PATH/values-test.yaml"
    if [[ -f "$HELM_VALUES_FILE" ]]; then
        log_info "Using test values file: $HELM_VALUES_FILE"
        VALUES_ARGS="--values $HELM_VALUES_FILE"
    else
        log_info "Test values file not found, using default values"
        VALUES_ARGS=""
    fi
    
    if helm install $CONTROLLER_NAME "$HELM_CHART_PATH" \
        --namespace $NAMESPACE \
        --create-namespace \
        --wait \
        --timeout=300s \
        $VALUES_ARGS; then
        log_info "Helm chart installed successfully"
        
        # Wait for controller to be ready
        if kubectl wait --for=condition=available --timeout=120s deployment/$CONTROLLER_NAME -n $NAMESPACE; then
            log_info "Controller is ready"
            return 0
        else
            log_error "Controller failed to become ready"
            return 1
        fi
    else
        log_error "Failed to install Helm chart"
        return 1
    fi
}

cleanup_controller() {
    log_info "Cleaning up controller deployment..."
    
    # Only cleanup if we deployed it (check if it was deployed by this test run)
    if [[ "$CONTROLLER_DEPLOYED_BY_TEST" == "true" ]]; then
        log_info "Uninstalling controller (deployed by this test run)..."
        helm uninstall $CONTROLLER_NAME --wait --namespace $NAMESPACE 2>/dev/null || true
        kubectl delete namespace $NAMESPACE --ignore-not-found=true 2>/dev/null || true
    else
        log_info "Controller was pre-existing, leaving it running"
    fi
}

# ConfigMap interaction functions
get_configmap_content() {
    kubectl get configmap $CONFIGMAP_NAME -n $COREDNS_NAMESPACE -o jsonpath='{.data.dynamic\.server}' 2>/dev/null || echo ""
}

hostname_in_configmap() {
    local hostname="$1"
    local config_content
    config_content=$(get_configmap_content)
    
    [[ "$config_content" == *"$hostname"* ]]
}

wait_for_controller_sync() {
    local timeout=${1:-5}
    local max_wait=30  # Maximum wait time in seconds
    local wait_interval=2
    local elapsed=0
    
    log_info "Waiting for controller to sync (timeout: ${timeout}s, max: ${max_wait}s)..."
    
    # First, do a basic sleep to allow immediate processing
    sleep "$timeout"
    
    # Then, wait for the controller to actually be responsive
    while [ $elapsed -lt $max_wait ]; do
        # Check if controller is processing by looking for recent log activity
        if kubectl logs -n "$NAMESPACE" deployment/"$CONTROLLER_NAME" --tail=10 --since=30s 2>/dev/null | grep -q "Successfully updated CoreDNS configuration\|Reconciling changes"; then
            log_info "Controller is actively processing resources"
            return 0
        fi
        
        sleep $wait_interval
        elapsed=$((elapsed + wait_interval))
    done
    
    log_warn "Controller sync wait completed after ${elapsed}s"
}

# Test prerequisites check
check_test_prerequisites() {
    log_info "Checking test prerequisites..."
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed"
        return 1
    fi
    
    # Check if cluster is accessible
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot access Kubernetes cluster"
        return 1
    fi
    
    # Check if helm is available
    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed"
        return 1
    fi
    
    # Check if nginx ingress controller exists
    if ! kubectl get deployment ingress-nginx-controller -n ingress-nginx &> /dev/null; then
        log_warn "ingress-nginx-controller not found - some tests may not work as expected"
    fi
    
    return 0
}

# Test runner function
run_test() {
    local test_name="$1"
    local test_function="$2"
    
    log_info "Running test: $test_name"
    
    if $test_function; then
        log_info "✅ PASSED: $test_name"
        if [[ -n "$TESTS_PASSED" ]]; then
            TESTS_PASSED=$((TESTS_PASSED + 1))
        fi
    else
        log_error "❌ FAILED: $test_name"
        if [[ -n "$TESTS_FAILED" ]]; then
            TESTS_FAILED=$((TESTS_FAILED + 1))
        fi
    fi
    
    if [[ -n "$TESTS_TOTAL" ]]; then
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
    fi
}

# Export all functions
export -f create_mock_k8s_client
export -f generate_test_ingress
export -f create_test_namespace
export -f cleanup_test_namespace
export -f wait_for_condition
export -f random_string
export -f command_exists
export -f log_with_timestamp
export -f create_test_configmap
export -f get_configmap_data
export -f resource_exists
export -f wait_for_pod_ready
export -f get_pod_logs
export -f create_test_service
export -f port_forward_service
export -f test_http_endpoint
export -f generate_load_test_data
export -f measure_time
export -f validate_dns_name
export -f extract_domain
export -f contains
export -f retry_with_backoff
export -f create_temp_file
export -f port_available
export -f find_available_port

# Export logging functions
export -f log_info
export -f log_warn
export -f log_error

# Export controller management functions
export -f verify_controller_running
export -f ensure_controller_deployed
export -f deploy_controller
export -f cleanup_controller

# Export ConfigMap interaction functions
export -f get_configmap_content
export -f hostname_in_configmap

# Export test utility functions
export -f wait_for_controller_sync

# Helper functions for common test operations

# Check if CoreDNS contains the expected import statement
check_coredns_import_exists() {
    local corefile_content
    corefile_content=$(kubectl get configmap coredns -n "$COREDNS_NAMESPACE" -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    [[ "$corefile_content" == *"$EXPECTED_IMPORT_STATEMENT"* ]]
}

# Check if CoreDNS does NOT contain the import statement
check_coredns_import_missing() {
    local corefile_content
    corefile_content=$(kubectl get configmap coredns -n "$COREDNS_NAMESPACE" -o jsonpath='{.data.Corefile}' 2>/dev/null || echo "")
    [[ "$corefile_content" != *"$EXPECTED_IMPORT_STATEMENT"* ]]
}

# Get the full deployment name for a release
get_deployment_name() {
    local release_name="${1:-$CONTROLLER_NAME}"
    echo "${release_name}-${CONTROLLER_NAME}"
}

# Get the full cluster role name for a release
get_cluster_role_name() {
    local release_name="${1:-$CONTROLLER_NAME}"
    echo "${release_name}-${CONTROLLER_NAME}-coredns"
}

# Wait for deployment to be ready
wait_for_deployment() {
    local deployment_name="$1"
    local namespace="$2"
    local timeout="${3:-60s}"
    
    kubectl wait --for=condition=available --timeout="$timeout" "deployment/$deployment_name" -n "$namespace"
}

# Helm install wrapper with common parameters
helm_install_controller() {
    local release_name="$1"
    local namespace="$2"
    local auto_configure="$3"
    local additional_args="${4:-}"
    
    helm install "$release_name" "$HELM_CHART_PATH" \
        --namespace "$namespace" \
        --set "coreDNS.autoConfigure=$auto_configure" \
        --set "controller.targetCNAME=test-target.cluster.local" \
        $additional_args \
        --wait --timeout=120s
}

# Helm upgrade wrapper with common parameters
helm_upgrade_controller() {
    local release_name="$1"
    local namespace="$2"
    local auto_configure="$3"
    local additional_args="${4:-}"
    
    helm upgrade "$release_name" "$HELM_CHART_PATH" \
        --namespace "$namespace" \
        --set "coreDNS.autoConfigure=$auto_configure" \
        --set "controller.targetCNAME=test-target.cluster.local" \
        $additional_args \
        --wait --timeout=120s
}

# Check if volume exists in CoreDNS deployment
# Check if CoreDNS deployment has the expected volume mount
check_coredns_mount_exists() {
    local volume_name="${1:-$VOLUME_NAME}"
    local mount_path="${2:-$EXPECTED_MOUNT_PATH}"
    local deployment_json
    deployment_json=$(kubectl get deployment coredns -n "$COREDNS_NAMESPACE" -o json 2>/dev/null || echo "{}")
    
    # Check if volume mount exists with the expected path
    echo "$deployment_json" | jq -e ".spec.template.spec.containers[].volumeMounts[]? | select(.name == \"$volume_name\" and .mountPath == \"$mount_path\")" &>/dev/null
}

# Check if CoreDNS deployment is missing the volume mount
check_coredns_mount_missing() {
    ! check_coredns_mount_exists "$@"
}

# Check if CoreDNS deployment has the expected volume
check_coredns_volume_exists() {
    local volume_name="${1:-$VOLUME_NAME}"
    local deployment_json
    deployment_json=$(kubectl get deployment coredns -n "$COREDNS_NAMESPACE" -o json 2>/dev/null || echo "{}")
    echo "$deployment_json" | jq -e ".spec.template.spec.volumes[]? | select(.name == \"$volume_name\")" &>/dev/null
}

# Export the new helper functions
export -f check_coredns_import_exists
export -f check_coredns_import_missing
export -f get_deployment_name
export -f get_cluster_role_name
export -f wait_for_deployment
export -f helm_install_controller
export -f helm_upgrade_controller
export -f check_coredns_mount_exists
export -f check_coredns_mount_missing
export -f check_coredns_volume_exists
export -f check_test_prerequisites
export -f run_test
