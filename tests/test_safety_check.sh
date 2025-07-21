#!/bin/bash

# Test script to verify the kubecontext safety check

echo "🔒 Testing Kubecontext Safety Check"
echo "==================================="

# Get test directory and source helpers
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/test_helpers.sh"

# Test 1: Check current context
echo "Current kubectl context:"
kubectl config current-context 2>/dev/null || echo "No context found"

# Test 2: Run safety check
echo ""
echo "Running safety check..."
if check_kubecontext_safety; then
    echo "✅ Safety check passed"
else
    echo "❌ Safety check failed"
fi

# Test 3: Show override option
echo ""
echo "To override safety check (not recommended):"
echo "  export SKIP_KUBECONTEXT_CHECK=1"
echo ""

# Test 4: Show safe context examples
echo "Safe test contexts include:"
echo "  - kind-test"
echo "  - kind-coredns-test"
echo "  - minikube"
echo "  - docker-desktop"
echo "  - k3s-default"
echo "  - localhost"
echo "  - test"
echo "  - dev"
echo "  - development"
echo ""

# Test 5: Show how to create a safe test context
echo "To create a safe test context:"
echo "  kind create cluster --name test"
echo "  kubectl config use-context kind-test"
echo ""
