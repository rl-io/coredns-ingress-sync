#!/bin/bash

# Simple RBAC test for cleanup functionality

set -e

# Configuration  
TEST_RELEASE_NAME="rbac-test-$(date +%s)"
TEST_NAMESPACE="rbac-test"

echo "🔒 Testing RBAC Cleanup Permissions"
echo "===================================="

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE 2>/dev/null || true
    kubectl delete namespace $TEST_NAMESPACE 2>/dev/null || true
    kubectl delete configmap $CONFIGMAP_NAME -n kube-system 2>/dev/null || true
}

trap cleanup EXIT

# Create test namespace
kubectl create namespace $TEST_NAMESPACE

# Deploy with RBAC
echo "Deploying controller with autoConfigure=false to test RBAC..."
helm install $TEST_RELEASE_NAME ./helm/coredns-ingress-sync \
    --namespace $TEST_NAMESPACE \
    --set coreDNS.autoConfigure=false \
    --set controller.targetCNAME=test-target.cluster.local \
    --wait --timeout=60s

# Check RBAC includes delete permission
echo "Checking if ClusterRole includes delete permission..."
kubectl get clusterrole $TEST_RELEASE_NAME-coredns-ingress-sync-coredns -o yaml | grep -A 10 -B 5 delete

# Manually create ConfigMap to test cleanup can delete it
echo "Creating test ConfigMap..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $CONFIGMAP_NAME
  namespace: kube-system
data:
  dynamic.server: |
    rewrite name exact test.example.com test-target.cluster.local.
EOF

echo "Testing helm uninstall (which should run cleanup job)..."
helm uninstall $TEST_RELEASE_NAME --namespace $TEST_NAMESPACE --wait --timeout=60s

# Check if ConfigMap was deleted
if kubectl get configmap $CONFIGMAP_NAME -n kube-system &>/dev/null; then
    echo "❌ FAILED: ConfigMap was not deleted - RBAC issue likely"
    exit 1
else
    echo "✅ SUCCESS: ConfigMap was deleted - RBAC permissions work!"
fi
