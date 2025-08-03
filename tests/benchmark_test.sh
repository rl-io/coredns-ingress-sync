#!/bin/bash

# Performance benchmark tests for coredns-ingress-sync controller

set -e

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Safety check - verify we're not running against a live cluster
if ! check_kubecontext_safety; then
    echo -e "${RED}[ERROR]${NC} Performance tests cannot run against this cluster"
    exit 1
fi

TEST_DOMAIN=${TEST_DOMAIN:-k8s.example.com}
RESULTS_DIR="test_results"

# Create results directory
mkdir -p $RESULTS_DIR

echo "⚡ Performance Benchmark Tests"
echo "=============================="

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

# Benchmark 1: Ingress processing speed
benchmark_ingress_processing() {
    log_info "Benchmarking ingress processing speed..."
    
    local ingress_count=100
    local start_time=$(date +%s.%N)
    
    # Create many ingresses quickly
    for i in $(seq 1 $ingress_count); do
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: bench-ingress-$i
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: bench-$i.$TEST_DOMAIN
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
    done
    
    local creation_time=$(date +%s.%N)
    local creation_duration=$(echo "$creation_time - $start_time" | bc)
    
    # Wait for controller to process all ingresses
    local processed_count=0
    local timeout=300  # 5 minutes
    local elapsed=0
    
    while [ $processed_count -lt $ingress_count ] && [ $elapsed -lt $timeout ]; do
        sleep 5
        elapsed=$((elapsed + 5))
        
        local config
        config=$(get_configmap_content 2>/dev/null || echo "")
        
        processed_count=0
        for i in $(seq 1 $ingress_count); do
            if [[ "$config" == *"bench-$i.$TEST_DOMAIN"* ]]; then
                processed_count=$((processed_count + 1))
            fi
        done
        
        log_info "Processed: $processed_count/$ingress_count ingresses"
    done
    
    local end_time=$(date +%s.%N)
    local total_duration=$(echo "$end_time - $start_time" | bc)
    local processing_duration=$(echo "$end_time - $creation_time" | bc)
    
    # Calculate metrics
    local ingresses_per_second=$(echo "scale=2; $processed_count / $processing_duration" | bc)
    local avg_processing_time=$(echo "scale=3; $processing_duration / $processed_count" | bc)
    
    # Save results
    cat > "$RESULTS_DIR/ingress_processing_benchmark.txt" <<EOF
Ingress Processing Benchmark Results
====================================
Total Ingresses: $ingress_count
Successfully Processed: $processed_count
Creation Time: ${creation_duration}s
Processing Time: ${processing_duration}s
Total Time: ${total_duration}s
Throughput: ${ingresses_per_second} ingresses/second
Average Processing Time: ${avg_processing_time}s per ingress
EOF
    
    log_info "Ingress processing benchmark completed"
    log_info "Throughput: $ingresses_per_second ingresses/second"
    log_info "Average processing time: ${avg_processing_time}s per ingress"
    
    # Clean up
    for i in $(seq 1 $ingress_count); do
        kubectl delete ingress bench-ingress-$i -n default 2>/dev/null || true
    done
}

# Benchmark 2: ConfigMap update frequency
benchmark_configmap_updates() {
    log_info "Benchmarking ConfigMap update frequency..."
    
    local update_count=0
    local start_time=$(date +%s)
    local duration=60  # 1 minute test
    
    # Monitor ConfigMap changes
    kubectl get configmap $CONFIGMAP_NAME -n $NAMESPACE -w -o jsonpath='{.metadata.resourceVersion}' > /tmp/configmap_changes.log &
    local watch_pid=$!
    
    # Create and delete ingresses rapidly
    for i in {1..20}; do
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: update-test-$i
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: update-test-$i.$TEST_DOMAIN
EOF
        sleep 1
        kubectl delete ingress update-test-$i -n default
        sleep 1
    done
    
    # Stop monitoring
    kill $watch_pid 2>/dev/null || true
    
    # Count updates
    update_count=$(wc -l < /tmp/configmap_changes.log)
    local end_time=$(date +%s)
    local actual_duration=$((end_time - start_time))
    
    local updates_per_second=$(echo "scale=2; $update_count / $actual_duration" | bc)
    
    # Save results
    cat > "$RESULTS_DIR/configmap_update_benchmark.txt" <<EOF
ConfigMap Update Benchmark Results
==================================
Test Duration: ${actual_duration}s
Total Updates: $update_count
Update Rate: ${updates_per_second} updates/second
EOF
    
    log_info "ConfigMap update benchmark completed"
    log_info "Update rate: $updates_per_second updates/second"
    
    # Clean up
    rm -f /tmp/configmap_changes.log
}

# Benchmark 3: Memory usage
benchmark_memory_usage() {
    log_info "Benchmarking memory usage..."
    
    # Get initial memory usage
    local initial_memory
    initial_memory=$(kubectl top pod -n $NAMESPACE -l app=coredns-ingress-sync --no-headers | awk '{print $3}' | sed 's/Mi//')
    
    # Create many ingresses
    for i in {1..500}; do
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: memory-test-$i
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: memory-test-$i.$TEST_DOMAIN
EOF
    done
    
    # Wait for processing
    sleep 30
    
    # Get peak memory usage
    local peak_memory
    peak_memory=$(kubectl top pod -n $NAMESPACE -l app=coredns-ingress-sync --no-headers | awk '{print $3}' | sed 's/Mi//')
    
    # Calculate memory increase
    local memory_increase=$((peak_memory - initial_memory))
    local memory_per_ingress=$(echo "scale=2; $memory_increase / 500" | bc)
    
    # Save results
    cat > "$RESULTS_DIR/memory_usage_benchmark.txt" <<EOF
Memory Usage Benchmark Results
==============================
Initial Memory: ${initial_memory}Mi
Peak Memory: ${peak_memory}Mi
Memory Increase: ${memory_increase}Mi
Memory per Ingress: ${memory_per_ingress}Mi
EOF
    
    log_info "Memory usage benchmark completed"
    log_info "Memory increase: ${memory_increase}Mi for 500 ingresses"
    log_info "Memory per ingress: ${memory_per_ingress}Mi"
    
    # Clean up
    for i in {1..500}; do
        kubectl delete ingress memory-test-$i -n default 2>/dev/null || true
    done
}

# Benchmark 4: CPU usage
benchmark_cpu_usage() {
    log_info "Benchmarking CPU usage..."
    
    # Monitor CPU usage during load
    kubectl top pod -n $NAMESPACE -l app=coredns-ingress-sync --no-headers > /tmp/cpu_usage.log &
    local monitor_pid=$!
    
    # Create load
    for i in {1..100}; do
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cpu-test-$i
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: cpu-test-$i.$TEST_DOMAIN
EOF
        sleep 0.1
    done
    
    # Continue monitoring
    sleep 30
    
    # Stop monitoring
    kill $monitor_pid 2>/dev/null || true
    
    # Analyze CPU usage
    local avg_cpu
    avg_cpu=$(awk '{sum += $2; count++} END {print sum/count}' /tmp/cpu_usage.log | sed 's/m//')
    
    # Save results
    cat > "$RESULTS_DIR/cpu_usage_benchmark.txt" <<EOF
CPU Usage Benchmark Results
===========================
Average CPU Usage: ${avg_cpu}m
Test Duration: 30s
Workload: 100 ingresses created rapidly
EOF
    
    log_info "CPU usage benchmark completed"
    log_info "Average CPU usage: ${avg_cpu}m"
    
    # Clean up
    for i in {1..100}; do
        kubectl delete ingress cpu-test-$i -n default 2>/dev/null || true
    done
    rm -f /tmp/cpu_usage.log
}

# Benchmark 5: Reconciliation latency
benchmark_reconciliation_latency() {
    log_info "Benchmarking reconciliation latency..."
    
    local latencies=()
    
    # Test 10 individual ingress creations
    for i in {1..10}; do
        local start_time=$(date +%s.%N)
        
        # Create ingress
        kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: latency-test-$i
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: latency-test-$i.$TEST_DOMAIN
EOF
        
        # Wait for it to appear in ConfigMap
        while true; do
            local config
            config=$(get_configmap_content)
            
            if [[ "$config" == *"latency-test-$i.$TEST_DOMAIN"* ]]; then
                break
            fi
            
            sleep 0.1
        done
        
        local end_time=$(date +%s.%N)
        local latency=$(echo "$end_time - $start_time" | bc)
        latencies+=($latency)
        
        log_info "Reconciliation $i latency: ${latency}s"
        
        # Clean up
        kubectl delete ingress latency-test-$i -n default
    done
    
    # Calculate statistics
    local total_latency=0
    local min_latency=${latencies[0]}
    local max_latency=${latencies[0]}
    
    for latency in "${latencies[@]}"; do
        total_latency=$(echo "$total_latency + $latency" | bc)
        if (( $(echo "$latency < $min_latency" | bc -l) )); then
            min_latency=$latency
        fi
        if (( $(echo "$latency > $max_latency" | bc -l) )); then
            max_latency=$latency
        fi
    done
    
    local avg_latency=$(echo "scale=3; $total_latency / 10" | bc)
    
    # Save results
    cat > "$RESULTS_DIR/reconciliation_latency_benchmark.txt" <<EOF
Reconciliation Latency Benchmark Results
=======================================
Average Latency: ${avg_latency}s
Min Latency: ${min_latency}s
Max Latency: ${max_latency}s
Sample Size: 10 ingresses
EOF
    
    log_info "Reconciliation latency benchmark completed"
    log_info "Average latency: ${avg_latency}s"
    log_info "Min latency: ${min_latency}s"
    log_info "Max latency: ${max_latency}s"
}

# Main benchmark execution
main() {
    log_info "Starting performance benchmarks..."
    
    # Check prerequisites and ensure controller is deployed
    if ! test_prerequisites; then
        log_error "Prerequisites check failed"
        exit 1
    fi
    
    # Check if bc is available for calculations
    if ! command -v bc &> /dev/null; then
        log_error "bc (calculator) is required for benchmarks"
        exit 1
    fi
    
    # Run benchmarks
    benchmark_ingress_processing
    benchmark_configmap_updates
    benchmark_memory_usage
    benchmark_cpu_usage
    benchmark_reconciliation_latency
    
    # Generate summary report
    cat > "$RESULTS_DIR/benchmark_summary.txt" <<EOF
coredns-ingress-sync controller - Performance Benchmark Summary
================================================================

Test Date: $(date)
Kubernetes Namespace: $NAMESPACE
Test Domain: $TEST_DOMAIN

Benchmark Results:
-----------------
$(cat $RESULTS_DIR/ingress_processing_benchmark.txt | tail -n +3)

$(cat $RESULTS_DIR/configmap_update_benchmark.txt | tail -n +3)

$(cat $RESULTS_DIR/memory_usage_benchmark.txt | tail -n +3)

$(cat $RESULTS_DIR/cpu_usage_benchmark.txt | tail -n +3)

$(cat $RESULTS_DIR/reconciliation_latency_benchmark.txt | tail -n +3)

Performance Assessment:
----------------------
- Ingress Processing: $([ $(echo "$(grep "Throughput:" $RESULTS_DIR/ingress_processing_benchmark.txt | awk '{print $2}') > 10" | bc) -eq 1 ] && echo "GOOD" || echo "NEEDS IMPROVEMENT")
- Memory Efficiency: $([ $(echo "$(grep "Memory per Ingress:" $RESULTS_DIR/memory_usage_benchmark.txt | awk '{print $4}' | sed 's/Mi//') < 1" | bc) -eq 1 ] && echo "GOOD" || echo "NEEDS IMPROVEMENT")
- Reconciliation Speed: $([ $(echo "$(grep "Average Latency:" $RESULTS_DIR/reconciliation_latency_benchmark.txt | awk '{print $3}' | sed 's/s//') < 2" | bc) -eq 1 ] && echo "GOOD" || echo "NEEDS IMPROVEMENT")

Recommendations:
---------------
- Monitor memory usage with large numbers of ingresses
- Consider implementing batching for better throughput
- Add metrics endpoint for continuous monitoring
EOF
    
    log_info "Performance benchmarks completed"
    log_info "Results saved to: $RESULTS_DIR/"
    log_info "Summary report: $RESULTS_DIR/benchmark_summary.txt"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up benchmark resources..."
    
    # Clean up any remaining test ingresses
    kubectl get ingress -n default -o name | grep -E "(bench-ingress|update-test|memory-test|cpu-test|latency-test)" | xargs kubectl delete -n default 2>/dev/null || true
    
    # Clean up temp files
    rm -f /tmp/configmap_changes.log /tmp/cpu_usage.log
}

trap cleanup EXIT

# Run main function
main "$@"
