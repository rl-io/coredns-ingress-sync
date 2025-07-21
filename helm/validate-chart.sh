#!/bin/bash

# Helm Chart Validation Script for coredns-ingress-sync

set -e

CHART_DIR="./helm/coredns-ingress-sync"
RELEASE_NAME="test-release"

echo "🔍 Validating Helm Chart..."

# Check if helm is installed
if ! command -v helm &> /dev/null; then
    echo "❌ Helm is not installed. Please install Helm first."
    exit 1
fi

# Lint the chart
echo "🧹 Linting chart..."
helm lint "$CHART_DIR"

# Test template rendering with basic configuration
echo "🎨 Testing template rendering..."
helm template "$RELEASE_NAME" "$CHART_DIR" \
    --set controller.env.TARGET_SERVICE="ingress-nginx-controller.ingress-nginx.svc.cluster.local" \
    --dry-run > /tmp/helm-output.yaml

echo "✅ Template rendering successful"

# Test with production values
echo "🏭 Testing production values..."
helm template "$RELEASE_NAME" "$CHART_DIR" \
    --values "$CHART_DIR/values-production.yaml" \
    --dry-run > /tmp/helm-production-output.yaml

echo "✅ Production values template successful"

# Test with dev values
echo "🧪 Testing dev values..."
helm template "$RELEASE_NAME" "$CHART_DIR" \
    --values "$CHART_DIR/values-dev.yaml" \
    --dry-run > /tmp/helm-dev-output.yaml

echo "✅ Dev values template successful"

# Validate required resources are present
echo "🔍 Validating required resources..."

resources=(
    "ServiceAccount"
    "ClusterRole"
    "ClusterRoleBinding"
    "Deployment"
    "ConfigMap"
    "Job"
)

for resource in "${resources[@]}"; do
    if grep -q "kind: $resource" /tmp/helm-output.yaml; then
        echo "✅ $resource found"
    else
        echo "❌ $resource NOT found"
        exit 1
    fi
done

# Check that we have both config and cleanup jobs
if grep -q "coredns-config" /tmp/helm-output.yaml && grep -q "coredns-cleanup" /tmp/helm-output.yaml; then
    echo "✅ Both config and cleanup jobs found"
else
    echo "❌ Config and cleanup jobs NOT found"
    exit 1
fi

# Check security context
echo "🔒 Validating security context..."
if grep -q "runAsUser: 65534" /tmp/helm-output.yaml; then
    echo "✅ Non-root user configured"
else
    echo "❌ Non-root user NOT configured"
    exit 1
fi

if grep -q "readOnlyRootFilesystem: true" /tmp/helm-output.yaml; then
    echo "✅ Read-only root filesystem configured"
else
    echo "❌ Read-only root filesystem NOT configured"
    exit 1
fi

# Check RBAC
echo "🔐 Validating RBAC..."
if grep -q "apiGroups.*networking.k8s.io" /tmp/helm-output.yaml; then
    echo "✅ Ingress RBAC permissions found"
else
    echo "❌ Ingress RBAC permissions NOT found"
    exit 1
fi

if grep -q "resources.*configmaps" /tmp/helm-output.yaml; then
    echo "✅ ConfigMap RBAC permissions found"
else
    echo "❌ ConfigMap RBAC permissions NOT found"
    exit 1
fi

# Check CoreDNS configuration
echo "🌐 Validating CoreDNS configuration..."
if grep -q "configure-coredns.sh" /tmp/helm-output.yaml; then
    echo "✅ CoreDNS configuration script found"
else
    echo "❌ CoreDNS configuration script NOT found"
    exit 1
fi

# Clean up
rm -f /tmp/helm-output.yaml /tmp/helm-production-output.yaml /tmp/helm-dev-output.yaml

echo ""
echo "🎉 All validations passed!"
echo "📦 Helm chart is ready for deployment"
echo ""
echo "Usage examples:"
echo "  # Install with basic config:"
echo "  helm install my-dns-controller $CHART_DIR --set controller.env.TARGET_SERVICE='ingress-nginx-controller.ingress-nginx.svc.cluster.local'"
echo ""
echo "  # Install with production config:"
echo "  helm install my-dns-controller $CHART_DIR --values $CHART_DIR/values-production.yaml"
echo ""
echo "  # Install with dev config:"
echo "  helm install my-dns-controller $CHART_DIR --values $CHART_DIR/values-dev.yaml"
