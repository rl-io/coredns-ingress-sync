#!/bin/bash

# Troubleshooting script for coredns-ingress-sync preflight failures
# Usage: ./scripts/check-preflight-logs.sh [release-name] [namespace]

set -e

RELEASE_NAME="${1:-coredns-ingress-sync}"
NAMESPACE="${2:-default}"

echo "🔍 Checking preflight job logs for release: $RELEASE_NAME in namespace: $NAMESPACE"
echo "=================================================================="

# Find the preflight job
JOB_NAME="${RELEASE_NAME}-coredns-ingress-sync-preflight"
FULL_JOB_NAME=$(kubectl get jobs -n "$NAMESPACE" --no-headers -o custom-columns=NAME:.metadata.name | grep "$JOB_NAME" | head -1)

# If the full pattern doesn't match, try a simpler pattern
if [ -z "$FULL_JOB_NAME" ]; then
    JOB_NAME="${RELEASE_NAME}.*preflight"
    FULL_JOB_NAME=$(kubectl get jobs -n "$NAMESPACE" --no-headers -o custom-columns=NAME:.metadata.name | grep -E "$JOB_NAME" | head -1)
fi

if [ -z "$FULL_JOB_NAME" ]; then
    echo "❌ No preflight job found matching patterns:"
    echo "   - ${RELEASE_NAME}-coredns-ingress-sync-preflight"
    echo "   - ${RELEASE_NAME}.*preflight"
    echo ""
    echo "Available jobs in namespace $NAMESPACE:"
    kubectl get jobs -n "$NAMESPACE" --no-headers -o custom-columns=NAME:.metadata.name || echo "  (none)"
    exit 1
fi

echo "📋 Job Name: $FULL_JOB_NAME"

# Check job status
JOB_STATUS=$(kubectl get job "$FULL_JOB_NAME" -n "$NAMESPACE" -o jsonpath='{.status.conditions[0].type}' 2>/dev/null || echo "Unknown")
echo "📊 Job Status: $JOB_STATUS"

# Get job details
echo ""
echo "🔍 Job Details:"
kubectl describe job "$FULL_JOB_NAME" -n "$NAMESPACE"

echo ""
echo "📝 Job Logs:"
echo "============"

# Get pod associated with the job
POD_NAME=$(kubectl get pods -n "$NAMESPACE" --selector=job-name="$FULL_JOB_NAME" --no-headers -o custom-columns=NAME:.metadata.name | head -1)

if [ -z "$POD_NAME" ]; then
    echo "❌ No pod found for job: $FULL_JOB_NAME"
    exit 1
fi

echo "Pod: $POD_NAME"
echo ""

# Show logs
kubectl logs "$POD_NAME" -n "$NAMESPACE" || {
    echo "❌ Failed to get logs from pod: $POD_NAME"
    echo ""
    echo "Pod status:"
    kubectl describe pod "$POD_NAME" -n "$NAMESPACE"
}

echo ""
echo "💡 Tips:"
echo "- If you see RBAC permission errors, make sure the ServiceAccount and RBAC resources were created"
echo "- If you see CoreDNS deployment not found, verify CoreDNS is installed in the expected namespace"
echo "- If you see mount path conflicts, consider setting a custom mount path in your Helm values"
echo ""

# Offer to clean up the failed job
if [ "$JOB_STATUS" = "Failed" ]; then
    echo "🧹 Clean up failed job?"
    echo "The failed job '$FULL_JOB_NAME' is still present for debugging."
    read -p "Delete it now? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Deleting job $FULL_JOB_NAME..."
        kubectl delete job "$FULL_JOB_NAME" -n "$NAMESPACE"
        echo "✅ Job deleted successfully"
    else
        echo "Job left for manual cleanup. Delete it later with:"
        echo "  kubectl delete job $FULL_JOB_NAME -n $NAMESPACE"
    fi
    echo ""
fi

echo "For more troubleshooting, see: https://github.com/rl-io/coredns-ingress-sync/blob/main/docs/TROUBLESHOOTING.md"
