#!/bin/bash

# Test runner script - orchestrates all tests

set -e

# Configuration
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$TEST_DIR/.." && pwd)"
RESULTS_DIR="$PROJECT_DIR/test_results"

# Source test helpers
source "$TEST_DIR/test_helpers.sh"

# Additional colors for this script
BLUE='\033[0;34m'

log_section() { echo -e "${BLUE}[SECTION]${NC} $1"; }

# Create results directory
mkdir -p "$RESULTS_DIR"

echo "🧪 coredns-ingress-sync controller - Test Suite"
echo "=================================================="
echo ""

# Usage information
show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -u, --unit           Run unit tests only"
    echo "  -i, --integration    Run integration tests only"
    echo "  -e, --e2e           Run end-to-end tests only"
    echo "  -p, --performance   Run performance benchmarks only"
    echo "  -a, --all           Run all tests (default)"
    echo "  -c, --coverage      Generate code coverage report"
    echo "  -h, --help          Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --unit           # Run only unit tests"
    echo "  $0 --all --coverage # Run all tests with coverage"
    echo "  $0 -i -e           # Run integration and e2e tests"
    echo ""
    echo "For local multi-version Kubernetes testing:"
    echo "  make kind-test-all-versions     # Test all supported K8s versions"
    echo "  make kind-test-version K8S_VERSION=1.29.4  # Test specific version"
    echo "  See tests/kind/README.md for details"
    echo ""
}

# Parse command line arguments
RUN_UNIT=false
RUN_INTEGRATION=false
RUN_E2E=false
RUN_PERFORMANCE=false
RUN_ALL=true
GENERATE_COVERAGE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -u|--unit)
            RUN_UNIT=true
            RUN_ALL=false
            shift
            ;;
        -i|--integration)
            RUN_INTEGRATION=true
            RUN_ALL=false
            shift
            ;;
        -e|--e2e)
            RUN_E2E=true
            RUN_ALL=false
            shift
            ;;
        -p|--performance)
            RUN_PERFORMANCE=true
            RUN_ALL=false
            shift
            ;;
        -a|--all)
            RUN_ALL=true
            shift
            ;;
        -c|--coverage)
            GENERATE_COVERAGE=true
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# If RUN_ALL is true, enable all test types
if [ "$RUN_ALL" = true ]; then
    RUN_UNIT=true
    RUN_INTEGRATION=true
    RUN_E2E=true
    RUN_PERFORMANCE=true
fi

# Test results tracking
UNIT_RESULT=0
INTEGRATION_RESULT=0
E2E_RESULT=0
PERFORMANCE_RESULT=0

# Pre-flight checks
preflight_checks() {
    log_section "Pre-flight Checks"
    
    # Verify test environment safety (kubecontext check)
    if ! verify_test_environment; then
        log_error "Test environment safety check failed"
        exit 1
    fi
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed - required for unit tests"
        exit 1
    fi
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed - required for integration tests"
        exit 1
    fi
    
    # Check if cluster is accessible (for non-unit tests)
    if [ "$RUN_INTEGRATION" = true ] || [ "$RUN_E2E" = true ]; then
        if ! kubectl cluster-info &> /dev/null; then
            log_error "Cannot access Kubernetes cluster - required for integration/e2e tests"
            exit 1
        fi
    fi
    
    # Check if controller image exists (for integration tests)
    if [ "$RUN_INTEGRATION" = true ] || [ "$RUN_E2E" = true ]; then
        if ! docker images | grep -q "coredns-ingress-sync"; then
            log_warn "Controller image not found - building it now..."
            cd "$PROJECT_DIR"
            if ! docker build -t coredns-ingress-sync:latest .; then
                log_error "Failed to build controller image"
                exit 1
            fi
        fi
    fi
    
    log_info "Pre-flight checks passed"
}

# Run unit tests
run_unit_tests() {
    log_section "Unit Tests"
    
    cd "$PROJECT_DIR"
    
    if [ "$GENERATE_COVERAGE" = true ]; then
        log_info "Running unit tests with coverage..."
        if go test -v -coverprofile="$RESULTS_DIR/coverage.out" -covermode=atomic ./...; then
            log_info "✅ Unit tests passed"
            
            # Generate coverage report
            go tool cover -html="$RESULTS_DIR/coverage.out" -o "$RESULTS_DIR/coverage.html"
            go tool cover -func="$RESULTS_DIR/coverage.out" > "$RESULTS_DIR/coverage.txt"
            
            local coverage_percent
            coverage_percent=$(grep "total:" "$RESULTS_DIR/coverage.txt" | awk '{print $3}')
            log_info "Code coverage: $coverage_percent"
            
            UNIT_RESULT=0
        else
            log_error "❌ Unit tests failed"
            UNIT_RESULT=1
        fi
    else
    log_info "Running unit tests..."
        if go test -v ./...; then
            log_info "✅ Unit tests passed"
            UNIT_RESULT=0
        else
            log_error "❌ Unit tests failed"
            UNIT_RESULT=1
        fi
    fi
}

# Run integration tests
run_integration_tests() {
    log_section "Integration Tests"
    
    cd "$TEST_DIR"
    
    # Make sure script is executable
    chmod +x integration_test.sh
    
    if ./integration_test.sh; then
        log_info "✅ Integration tests passed"
        INTEGRATION_RESULT=0
    else
        log_error "❌ Integration tests failed"
        INTEGRATION_RESULT=1
        return
    fi
    
    # Run leader election tests
    log_section "Leader Election Tests"
    chmod +x leader_election_test.sh
    
    if ./leader_election_test.sh; then
        log_info "✅ Leader election tests passed"
    else
        log_error "❌ Leader election tests failed"
        INTEGRATION_RESULT=1
    fi
    
    # Run RBAC leader election validation
    log_section "RBAC Leader Election Validation"
    chmod +x rbac_leader_election_test.sh
    
    if ./rbac_leader_election_test.sh; then
        log_info "✅ RBAC leader election validation passed"
    else
        log_error "❌ RBAC leader election validation failed"
        INTEGRATION_RESULT=1
    fi
}

# Run end-to-end tests
run_e2e_tests() {
    log_section "End-to-End Tests"
    
    cd "$TEST_DIR"
    
    # Make sure script is executable
    chmod +x e2e_test.sh
    
    if ./e2e_test.sh; then
        log_info "✅ End-to-end tests passed"
        E2E_RESULT=0
    else
        log_error "❌ End-to-end tests failed"
        E2E_RESULT=1
    fi
}

# Run performance benchmarks
run_performance_tests() {
    log_section "Performance Benchmarks"
    
    cd "$TEST_DIR"
    
    # Make sure script is executable
    chmod +x benchmark_test.sh
    
    if ./benchmark_test.sh; then
        log_info "✅ Performance benchmarks completed"
        PERFORMANCE_RESULT=0
    else
        log_error "❌ Performance benchmarks failed"
        PERFORMANCE_RESULT=1
    fi
}

# Generate final report
generate_report() {
    log_section "Test Report"
    
    local total_tests=0
    local passed_tests=0
    local failed_tests=0
    
    # Count test results
    if [ "$RUN_UNIT" = true ]; then
        total_tests=$((total_tests + 1))
        if [ $UNIT_RESULT -eq 0 ]; then
            passed_tests=$((passed_tests + 1))
        else
            failed_tests=$((failed_tests + 1))
        fi
    fi
    
    if [ "$RUN_INTEGRATION" = true ]; then
        total_tests=$((total_tests + 1))
        if [ $INTEGRATION_RESULT -eq 0 ]; then
            passed_tests=$((passed_tests + 1))
        else
            failed_tests=$((failed_tests + 1))
        fi
    fi
    
    if [ "$RUN_E2E" = true ]; then
        total_tests=$((total_tests + 1))
        if [ $E2E_RESULT -eq 0 ]; then
            passed_tests=$((passed_tests + 1))
        else
            failed_tests=$((failed_tests + 1))
        fi
    fi
    
    if [ "$RUN_PERFORMANCE" = true ]; then
        total_tests=$((total_tests + 1))
        if [ $PERFORMANCE_RESULT -eq 0 ]; then
            passed_tests=$((passed_tests + 1))
        else
            failed_tests=$((failed_tests + 1))
        fi
    fi
    
    # Generate summary report
    cat > "$RESULTS_DIR/test_summary.txt" <<EOF
coredns-ingress-sync controller - Test Summary
================================================

Test Date: $(date)
Test Types Run: $([ "$RUN_UNIT" = true ] && echo -n "Unit "; [ "$RUN_INTEGRATION" = true ] && echo -n "Integration "; [ "$RUN_E2E" = true ] && echo -n "E2E "; [ "$RUN_PERFORMANCE" = true ] && echo -n "Performance ")

Results:
--------
Total Test Suites: $total_tests
Passed: $passed_tests
Failed: $failed_tests

Detailed Results:
----------------
$([ "$RUN_UNIT" = true ] && echo "Unit Tests: $([ $UNIT_RESULT -eq 0 ] && echo "PASSED" || echo "FAILED")")
$([ "$RUN_INTEGRATION" = true ] && echo "Integration Tests: $([ $INTEGRATION_RESULT -eq 0 ] && echo "PASSED" || echo "FAILED")")
$([ "$RUN_E2E" = true ] && echo "End-to-End Tests: $([ $E2E_RESULT -eq 0 ] && echo "PASSED" || echo "FAILED")")
$([ "$RUN_PERFORMANCE" = true ] && echo "Performance Tests: $([ $PERFORMANCE_RESULT -eq 0 ] && echo "COMPLETED" || echo "FAILED")")

$([ "$GENERATE_COVERAGE" = true ] && [ -f "$RESULTS_DIR/coverage.txt" ] && echo "Code Coverage: $(grep "total:" "$RESULTS_DIR/coverage.txt" | awk '{print $3}')")

Test Artifacts:
--------------
- Test results: $RESULTS_DIR/
$([ "$GENERATE_COVERAGE" = true ] && echo "- Coverage report: $RESULTS_DIR/coverage.html")
$([ "$RUN_PERFORMANCE" = true ] && echo "- Performance benchmarks: $RESULTS_DIR/benchmark_summary.txt")

EOF
    
    # Display summary
    echo ""
    echo "======================================"
    echo "🧪 Test Summary"
    echo "======================================"
    echo "Total Test Suites: $total_tests"
    echo "Passed: $passed_tests"
    echo "Failed: $failed_tests"
    echo ""
    
    if [ "$RUN_UNIT" = true ]; then
        echo "Unit Tests: $([ $UNIT_RESULT -eq 0 ] && echo -e "${GREEN}PASSED${NC}" || echo -e "${RED}FAILED${NC}")"
    fi
    
    if [ "$RUN_INTEGRATION" = true ]; then
        echo "Integration Tests: $([ $INTEGRATION_RESULT -eq 0 ] && echo -e "${GREEN}PASSED${NC}" || echo -e "${RED}FAILED${NC}")"
    fi
    
    if [ "$RUN_E2E" = true ]; then
        echo "End-to-End Tests: $([ $E2E_RESULT -eq 0 ] && echo -e "${GREEN}PASSED${NC}" || echo -e "${RED}FAILED${NC}")"
    fi
    
    if [ "$RUN_PERFORMANCE" = true ]; then
        echo "Performance Tests: $([ $PERFORMANCE_RESULT -eq 0 ] && echo -e "${GREEN}COMPLETED${NC}" || echo -e "${RED}FAILED${NC}")"
    fi
    
    echo ""
    echo "📊 Test artifacts saved to: $RESULTS_DIR/"
    
    if [ "$GENERATE_COVERAGE" = true ] && [ -f "$RESULTS_DIR/coverage.html" ]; then
        echo "📈 Coverage report: $RESULTS_DIR/coverage.html"
    fi
    
    if [ "$RUN_PERFORMANCE" = true ] && [ -f "$RESULTS_DIR/benchmark_summary.txt" ]; then
        echo "⚡ Performance report: $RESULTS_DIR/benchmark_summary.txt"
    fi
}

# Main execution
main() {
    log_info "Starting test suite..."
    
    # Run pre-flight checks
    preflight_checks
    
    # Run selected tests
    if [ "$RUN_UNIT" = true ]; then
        run_unit_tests
    fi
    
    if [ "$RUN_INTEGRATION" = true ]; then
        run_integration_tests
    fi
    
    if [ "$RUN_E2E" = true ]; then
        run_e2e_tests
    fi
    
    if [ "$RUN_PERFORMANCE" = true ]; then
        run_performance_tests
    fi
    
    # Generate final report
    generate_report
    
    # Exit with appropriate code
    if [ $UNIT_RESULT -eq 0 ] && [ $INTEGRATION_RESULT -eq 0 ] && [ $E2E_RESULT -eq 0 ] && [ $PERFORMANCE_RESULT -eq 0 ]; then
        log_info "🎉 All tests completed successfully!"
        exit 0
    else
        log_error "❌ Some tests failed"
        exit 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test environment..."
    # Add any cleanup logic here
}

trap cleanup EXIT

# Run main function
main "$@"
