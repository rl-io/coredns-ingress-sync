# Troubleshooting Guide

This guide helps diagnose and resolve common issues with the coredns-ingress-sync controller.

## Quick Diagnostic Steps

When experiencing issues, start with these diagnostic steps:

```bash
# 1. Check controller status
kubectl get pods -n coredns-ingress-sync
kubectl get deployment coredns-ingress-sync -n coredns-ingress-sync

# 2. Check controller logs
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync --tail=50

# 3. Check dynamic ConfigMap
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml

# 4. Check CoreDNS status
kubectl get pods -n kube-system -l k8s-app=kube-dns
kubectl logs -n kube-system deployment/coredns --tail=20
```

## Common Issues and Solutions

### 1. Controller Not Starting

#### Symptoms

- Pod in `CrashLoopBackOff` or `Pending` state
- Container fails to start
- No logs from controller

#### Diagnostic Steps

```bash
# Check pod status and events
kubectl describe pod -n coredns-ingress-sync -l app.kubernetes.io/name=coredns-ingress-sync

# Check events in namespace
kubectl get events -n coredns-ingress-sync --sort-by='.lastTimestamp'

# Check resource constraints
kubectl top pod -n coredns-ingress-sync
```

#### Common Causes and Solutions

##### Image Pull Errors

```bash
# Check image pull policy and availability
kubectl describe pod -n coredns-ingress-sync -l app.kubernetes.io/name=coredns-ingress-sync | grep -A 5 "Image"

# For local development, ensure image is loaded
kind load docker-image coredns-ingress-sync:latest --name your-cluster-name
```

##### Resource Constraints

```bash
# Check node resources
kubectl describe nodes

# Adjust resource requests/limits
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set resources.requests.memory=32Mi \
  --set resources.requests.cpu=5m \
  --namespace coredns-ingress-sync
```

##### RBAC Permissions

```bash
# Check service account permissions
kubectl auth can-i list ingresses --as=system:serviceaccount:coredns-ingress-sync:coredns-ingress-sync
kubectl auth can-i get configmaps --as=system:serviceaccount:coredns-ingress-sync:coredns-ingress-sync -n kube-system

# If permissions are missing, reinstall with latest chart
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync --namespace coredns-ingress-sync
```

### 2. DNS Resolution Not Working

#### DNS Resolution Symptoms

- DNS queries for ingress hostnames fail
- Hostnames resolve to incorrect IP addresses
- Intermittent DNS resolution issues

#### DNS Resolution Diagnostic Steps

```bash
# Test DNS resolution from within cluster
kubectl run test-pod --rm -i --tty --image=busybox -- nslookup your-hostname.example.com

# Check CoreDNS configuration
kubectl get configmap coredns -n kube-system -o yaml | grep -A 10 "import"

# Check dynamic ConfigMap content
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o jsonpath='{.data.dynamic\.server}'

# Check CoreDNS logs for errors
kubectl logs -n kube-system deployment/coredns | grep -i error
```

#### DNS Resolution Solutions

##### Missing Import Statement in CoreDNS

```bash
# Check if import statement exists
kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' | grep "import /etc/coredns/custom"

# If missing, the controller should auto-add it. Check controller logs:
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep -i "import"

# Manual fix if auto-configuration is disabled:
kubectl patch configmap coredns -n kube-system --type merge -p '{"data":{"Corefile":".:53 {\n    import /etc/coredns/custom/*.server\n    errors\n    health {\n       lameduck 5s\n    }\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n       pods insecure\n       fallthrough in-addr.arpa ip6.arpa\n       ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf {\n       max_concurrent 1000\n    }\n    cache 30\n    loop\n    reload\n    loadbalance\n}"}}'
```

##### Missing Volume Mount in CoreDNS

```bash
# Check if volume mount exists
kubectl get deployment coredns -n kube-system -o yaml | grep -A 5 "/etc/coredns/custom"

# If missing, check controller logs for volume mount errors:
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep -i "volume"

# Controller should automatically add this if autoConfigure is enabled
```

##### Empty or Incorrect Dynamic ConfigMap

```bash
# Check ConfigMap content
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml

# If empty or missing, check if ingresses are being processed:
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep "Successfully updated"

# Check if ingresses have correct IngressClass:
kubectl get ingress -A -o wide | grep nginx
```

##### CoreDNS Pod Issues

```bash
# Restart CoreDNS to reload configuration
kubectl rollout restart deployment/coredns -n kube-system

# Check CoreDNS pod logs for configuration errors
kubectl logs -n kube-system deployment/coredns

# Verify CoreDNS is using the mounted ConfigMap
kubectl exec -n kube-system deployment/coredns -- ls -la /etc/coredns/custom/
```

### 3. Ingresses Not Being Processed

#### Ingress Processing Symptoms

- New ingresses don't appear in dynamic ConfigMap
- Controller logs show no reconciliation activity
- Expected hostnames missing from DNS configuration

#### Ingress Processing Diagnostic Steps

```bash
# Check if controller is receiving ingress events
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep "Reconciling"

# List ingresses and their classes
kubectl get ingress -A -o custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace,CLASS:.spec.ingressClassName,HOSTS:.spec.rules[*].host

# Check controller configuration
kubectl get deployment coredns-ingress-sync -n coredns-ingress-sync -o yaml | grep -A 10 "env:"
```

#### Ingress Processing Solutions

##### Wrong IngressClass

```bash
# Check what IngressClass the controller is watching
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | head -10 | grep "ingress class"

# Check your ingresses
kubectl get ingress -A -o wide

# Update controller to watch different IngressClass:
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set controller.ingressClass=your-ingress-class \
  --namespace coredns-ingress-sync
```

##### Controller Not Running

```bash
# Check controller status
kubectl get pods -n coredns-ingress-sync

# If not running, check logs from previous runs
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync --previous

# Restart controller
kubectl rollout restart deployment/coredns-ingress-sync -n coredns-ingress-sync
```

**Leader Election Issues** (Multiple Replicas)

```bash
# Check leader election status
kubectl get leases -n coredns-ingress-sync

# Check which pod is the leader
kubectl logs -n coredns-ingress-sync -l app.kubernetes.io/name=coredns-ingress-sync | grep -i leader

# If stuck, delete the lease to force re-election
kubectl delete lease coredns-ingress-sync-leader -n coredns-ingress-sync
```

### 4. ConfigMap Update Issues

#### ConfigMap Update Symptoms

- Controller logs show reconciliation but ConfigMap doesn't update
- "Failed to update dynamic ConfigMap" errors
- Stale configuration in dynamic ConfigMap

#### ConfigMap Update Diagnostic Steps

```bash
# Check for ConfigMap update errors
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep -i "configmap"

# Check ConfigMap permissions
kubectl auth can-i update configmaps --as=system:serviceaccount:coredns-ingress-sync:coredns-ingress-sync -n kube-system

# Check ConfigMap resource version conflicts
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml | grep resourceVersion
```

#### ConfigMap Update Solutions

##### Permission Denied

```bash
# Verify RBAC permissions
kubectl get rolebinding -n kube-system | grep coredns-ingress-sync
kubectl describe rolebinding coredns-ingress-sync-coredns -n kube-system

# Reinstall with proper RBAC
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync --namespace coredns-ingress-sync
```

##### Resource Version Conflicts

This typically resolves automatically due to retry logic, but if persistent:

```bash
# Check for multiple controller instances
kubectl get pods -n coredns-ingress-sync -l app.kubernetes.io/name=coredns-ingress-sync

# If multiple pods without leader election, scale down to 1
kubectl scale deployment coredns-ingress-sync --replicas=1 -n coredns-ingress-sync

# Enable leader election for multiple replicas
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set replicaCount=2 \
  --set controller.leaderElection.enabled=true \
  --namespace coredns-ingress-sync
```

### 5. Performance Issues

#### Performance Symptoms

- Slow ingress processing
- High CPU/memory usage
- Reconciliation timeouts

#### Performance Diagnostic Steps

```bash
# Check resource usage
kubectl top pod -n coredns-ingress-sync

# Check reconciliation timing
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep "Successfully updated" | tail -10

# Run performance benchmarks
./tests/benchmark_test.sh
```

#### Performance Solutions

##### Performance Resource Constraints

```bash
# Increase resource limits
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set resources.limits.cpu=200m \
  --set resources.limits.memory=256Mi \
  --set resources.requests.cpu=20m \
  --set resources.requests.memory=128Mi \
  --namespace coredns-ingress-sync
```

##### Large Number of Ingresses

```bash
# Check number of ingresses being processed
kubectl get ingress -A | wc -l

# Consider filtering by namespace or using multiple controller instances
# with different IngressClass configurations
```

## Advanced Debugging

### Enable Debug Logging

```bash
# Enable debug logging
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set controller.logLevel=debug \
  --namespace coredns-ingress-sync

# Watch debug logs
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync -f | grep -v "Reconciling"
```

### Check Kubernetes Events

```bash
# Check all events in relevant namespaces
kubectl get events -n coredns-ingress-sync --sort-by='.lastTimestamp'
kubectl get events -n kube-system --sort-by='.lastTimestamp' | grep coredns

# Watch events in real-time
kubectl get events -n coredns-ingress-sync -w
```

### Network Debugging

```bash
# Test network connectivity from controller pod
kubectl exec -n coredns-ingress-sync deployment/coredns-ingress-sync -- nslookup kubernetes.default.svc.cluster.local

# Check if controller can reach Kubernetes API
kubectl exec -n coredns-ingress-sync deployment/coredns-ingress-sync -- wget -q -O- https://kubernetes.default.svc.cluster.local/api/v1/namespaces/default/ingresses
```

### CoreDNS Configuration Verification

```bash
# Dump complete CoreDNS configuration
kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}'

# Check if dynamic configuration is being loaded
kubectl exec -n kube-system deployment/coredns -- cat /etc/coredns/custom/dynamic.server

# Test CoreDNS configuration syntax
kubectl exec -n kube-system deployment/coredns -- coredns -conf /etc/coredns/Corefile -dns.port=0 -validate
```

## Recovery Procedures

### Complete Reset

If all else fails, perform a complete reset:

```bash
# 1. Uninstall controller
helm uninstall coredns-ingress-sync -n coredns-ingress-sync

# 2. Clean up remaining resources
kubectl delete namespace coredns-ingress-sync
kubectl delete configmap coredns-ingress-sync-rewrite-rules -n kube-system

# 3. Reset CoreDNS configuration (if needed)
kubectl patch configmap coredns -n kube-system --type merge -p '{"data":{"Corefile":".:53 {\n    errors\n    health {\n       lameduck 5s\n    }\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n       pods insecure\n       fallthrough in-addr.arpa ip6.arpa\n       ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf {\n       max_concurrent 1000\n    }\n    cache 30\n    loop\n    reload\n    loadbalance\n}"}}'

# 4. Restart CoreDNS
kubectl rollout restart deployment/coredns -n kube-system

# 5. Reinstall controller
helm install coredns-ingress-sync ./helm/coredns-ingress-sync \
  --namespace coredns-ingress-sync \
  --create-namespace \
  --set coreDNS.autoConfigure=true
```

### Backup and Restore

Before making changes, backup important configurations:

```bash
# Backup CoreDNS configuration
kubectl get configmap coredns -n kube-system -o yaml > coredns-backup.yaml

# Backup dynamic configuration
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml > coredns-ingress-sync-rewrite-rules-backup.yaml

# Restore if needed
kubectl apply -f coredns-backup.yaml
kubectl apply -f coredns-ingress-sync-rewrite-rules-backup.yaml
```

## Getting Help

### Information to Gather

When seeking help, gather this information:

```bash
# System information
kubectl version
helm version

# Controller information
kubectl get pods -n coredns-ingress-sync -o yaml
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync --tail=100

# CoreDNS information
kubectl get deployment coredns -n kube-system -o yaml
kubectl get configmap coredns -n kube-system -o yaml
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml

# Ingress information
kubectl get ingress -A -o yaml

# Events
kubectl get events -n coredns-ingress-sync
kubectl get events -n kube-system | grep coredns
```

### Support Channels

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and community support
- **Documentation**: Check the docs/ directory for detailed guides

### Creating Bug Reports

Include:

1. **Environment details**: Kubernetes version, cluster type, etc.
2. **Controller version**: Helm chart version and image tag
3. **Configuration**: Your values.yaml or custom configuration
4. **Logs**: Controller logs and relevant CoreDNS logs
5. **Steps to reproduce**: Clear steps to reproduce the issue
6. **Expected vs actual behavior**: What you expected and what happened
