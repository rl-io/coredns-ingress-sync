#!/bin/bash

# Debug script to show preflight job logs when it fails

set -e

NAMESPACE="coredns-ingress-sync"
RELEASE_NAME="coredns-ingress-sync"

echo "🔍 Checking for preflight jobs..."

# Find the preflight job
JOB_NAME="${RELEASE_NAME}-preflight"

echo "Looking for job: $JOB_NAME in namespace: $NAMESPACE"

# Check if job exists
if ! kubectl get job "$JOB_NAME" -n "$NAMESPACE" &>/dev/null; then
    echo "❌ Job $JOB_NAME not found in namespace $NAMESPACE"
    echo "Available jobs:"
    kubectl get jobs -n "$NAMESPACE" || echo "No jobs found in namespace"
    exit 1
fi

echo "✅ Found job: $JOB_NAME"

# Show job status
echo "📊 Job Status:"
kubectl get job "$JOB_NAME" -n "$NAMESPACE" -o wide

# Show pod status
echo ""
echo "📦 Pod Status:"
kubectl get pods -n "$NAMESPACE" -l job-name="$JOB_NAME" -o wide

# Get pod name
POD_NAME=$(kubectl get pods -n "$NAMESPACE" -l job-name="$JOB_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [[ -z "$POD_NAME" ]]; then
    echo "❌ No pod found for job $JOB_NAME"
    exit 1
fi

echo "📜 Pod Logs for $POD_NAME:"
echo "==============================================="

# Show logs (including previous container if it crashed)
if kubectl logs "$POD_NAME" -n "$NAMESPACE" &>/dev/null; then
    kubectl logs "$POD_NAME" -n "$NAMESPACE"
else
    echo "❌ Could not get current logs, trying previous container..."
    kubectl logs "$POD_NAME" -n "$NAMESPACE" --previous 2>/dev/null || echo "No previous logs available"
fi

echo ""
echo "==============================================="

# Show pod events
echo "📋 Pod Events:"
kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -A 20 "Events:" || echo "No events found"

echo ""
echo "🔧 Debugging complete!"
