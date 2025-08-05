# Frequently Asked Questions (FAQ)

## General Questions

### Q: What is coredns-ingress-sync?

A: coredns-ingress-sync is a Kubernetes controller that automatically configures
CoreDNS to resolve ingress hostnames to internal cluster services. This enables
secure, efficient internal service-to-service communication without relying on
external DNS resolution.

### Q: Why would I need this instead of just using external DNS?

A: External DNS requires internet connectivity and adds latency and security risks to internal communications. This controller allows services within your cluster to resolve ingress hostnames directly to internal IPs (like your ingress controller's ClusterIP), improving performance and security.

### Q: Is this compatible with external-dns?

A: Yes, completely compatible. This controller only affects internal DNS resolution within your cluster. External DNS continues to manage public DNS records normally, and both can operate on the same ingress resources.

### Q: Which Kubernetes versions are supported?

A: The controller is built using controller-runtime and supports Kubernetes 1.19+. It has been tested with:

- EKS 1.21-1.33
- Kind/Minikube for development

## Installation and Configuration

### Q: Do I need to modify my existing ingress resources?

A: No, the controller automatically discovers existing ingress resources based on the `spec.ingressClassName` field. No modifications to your ingress resources are required.

### Q: Can I use this with ingress controllers other than nginx?

A: Yes, you can configure any ingress class via the `controller.ingressClass` Helm value. Common examples:

- `nginx` (default)
- `traefik`
- `haproxy`
- `istio`

### Q: How do I configure a custom target for DNS resolution?

A: Use the `controller.targetCNAME` Helm value:

```yaml
controller:
  targetCNAME: my-custom-ingress.my-namespace.svc.cluster.local.
```

### Q: Can I disable the automatic CoreDNS configuration?

A: Yes, automatic CoreDNS configuration is disabled by default (`coreDNS.autoConfigure: false`). To enable automatic configuration, set:

```yaml
coreDNS:
  autoConfigure: true
```

When disabled, you'll need to manually add the import statement to your CoreDNS configuration.

### Q: How do I configure multiple replicas?

A: Enable leader election and set the replica count:

```yaml
replicaCount: 2
controller:
  leaderElection:
    enabled: true
```

## Operation and Behavior

### Q: How quickly are DNS changes propagated?

A: Changes are typically propagated within 1-2 seconds of ingress modifications. The controller watches for ingress changes in real-time and immediately updates the DNS configuration.

### Q: What happens if I delete an ingress?

A: The corresponding DNS entry is automatically removed from the dynamic configuration within seconds of the ingress deletion.

### Q: Does this work with ingress resources in any namespace?

A: Yes, the controller watches ingress resources cluster-wide across all namespaces. You don't need to install the controller in the same namespace as your ingresses.

### Q: How does this handle duplicate hostnames across multiple ingresses?

A: All hostnames are mapped to the same target (your ingress controller), so duplicates don't cause conflicts. The ingress controller handles routing based on host headers.

### Q: What happens during controller downtime?

A: DNS resolution continues to work normally during controller downtime since the configuration is stored in CoreDNS. Only new ingress changes won't be processed until the controller restarts.

## CoreDNS Integration

### Q: Will this break my existing CoreDNS configuration?

A: No, the controller only adds an import statement and volume mount. Your existing CoreDNS configuration remains unchanged. The controller uses a defensive approach to ensure compatibility.

### Q: What if Terraform manages my CoreDNS configuration?

A: The controller includes defensive configuration management that automatically restores its configuration if external tools (like Terraform) remove it. This ensures zero-downtime operation with existing automation.

### Q: How can I verify the CoreDNS integration is working?

A: Check that the import statement exists in the CoreDNS Corefile:

```bash
kubectl get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' | grep import
```

And verify the dynamic configuration:

```bash
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml
```

### Q: Can I customize the CoreDNS import path?

A: The import path is hardcoded to `/etc/coredns/custom/*.server` for consistency and reliability. This path is automatically mounted via the volume configuration.

## Troubleshooting

### Q: Why isn't my ingress hostname resolving internally?

A: Check these common issues:

1. **IngressClass mismatch**: Verify your ingress uses the class configured in the controller
2. **Missing import**: Ensure CoreDNS has the import statement
3. **Controller status**: Check if the controller pod is running and has no errors

See the [Troubleshooting Guide](TROUBLESHOOTING.md) for detailed diagnostics.

### Q: How do I test if internal DNS resolution is working?

A: Run a test pod and try resolving your hostname:

```bash
kubectl run test-pod --rm -i --tty --image=busybox -- nslookup your-hostname.example.com
```

### Q: The controller logs show "Successfully updated" but DNS isn't working. Why?

A: This usually indicates a CoreDNS configuration issue:

1. Check if CoreDNS picked up the configuration: `kubectl logs -n kube-system deployment/coredns`
2. Verify the volume mount exists: `kubectl describe deployment coredns -n kube-system`
3. Restart CoreDNS to reload: `kubectl rollout restart deployment/coredns -n kube-system`

### Q: Can I see what hostnames are currently configured?

A: Yes, check the dynamic ConfigMap:

```bash
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o jsonpath='{.data.dynamic\.server}'
```

## Development and Customization

### Q: How do I build and test the controller locally?

A: See the [Development Guide](DEVELOPMENT.md) for detailed instructions. Quick start:

```bash
# Build
make build

# Run tests
make test

# Run locally
export KUBECONFIG=~/.kube/config
./bin/controller
```

### Q: Can I modify the DNS rewrite rules?

A: The current implementation uses simple hostname-to-target mapping. For custom rewrite rules, you would need to modify the `generateConfigMapData()` function in `main.go`.

### Q: How do I contribute to the project?

A: See the [Development Guide](DEVELOPMENT.md) for contribution guidelines, including:

- Setting up the development environment
- Running tests
- Submitting pull requests

### Q: Is there an API or webhook interface?

A: The controller uses the standard Kubernetes controller pattern with watch-based reconciliation. There's no separate API - it responds automatically to ingress resource changes.

## Security

### Q: What permissions does the controller need?

A: The controller requires minimal RBAC permissions:

- Read ingresses cluster-wide
- Read/write specific ConfigMaps in kube-system
- Read/write deployments in kube-system (for auto-configuration)

See the [Configuration Guide](CONFIGURATION.md) for detailed RBAC requirements.

### Q: Is it safe to run multiple replicas?

A: Yes, when leader election is enabled. Only one replica actively reconciles resources, while others remain on standby for high availability.

### Q: Does this create security risks by modifying CoreDNS?

A: The controller uses a least-privilege approach and only adds DNS resolution rules. It doesn't modify existing DNS resolution behavior or expose external services.

## Performance

### Q: How many ingresses can this handle?

A: The controller has been tested with hundreds of ingresses. Performance depends on:

- Kubernetes cluster size and performance
- Frequency of ingress changes
- Controller resource allocation

See the [Architecture Guide](ARCHITECTURE.md) for performance characteristics.

### Q: Does this add latency to DNS resolution?

A: No, it actually reduces latency for internal communications by eliminating external DNS lookups. DNS queries are resolved directly by CoreDNS using local configuration.

### Q: How often does the controller reconcile?

A: The controller uses event-driven reconciliation and only processes changes when:

- Ingress resources are created, updated, or deleted
- CoreDNS configuration is modified externally
- Controller startup/restart

## Deployment Environments

### Q: Does this work with managed Kubernetes services?

A: Yes, it's specifically designed for managed services like EKS, GKE, and AKS where direct etcd access isn't available. The controller uses standard Kubernetes APIs only.

### Q: Can I use this in development environments like Kind or Minikube?

A: Yes, it works perfectly in development environments. See the [Development Guide](DEVELOPMENT.md) for local setup instructions.

### Q: Is this production-ready?

A: Yes, the controller includes:

- Comprehensive error handling and logging
- Leader election for high availability
- Defensive configuration management
- Extensive test suite
- Resource cleanup procedures

### Q: How do I upgrade the controller?

A: Use Helm upgrade with your existing values:

```bash
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --namespace coredns-ingress-sync \
  --reuse-values
```

### Q: What's the upgrade/rollback strategy?

A: The controller supports standard Kubernetes rolling updates. During upgrades:

1. New controller version starts
2. Leader election ensures only one active controller
3. Old controller gracefully shuts down
4. DNS resolution continues uninterrupted

For rollbacks, use standard Helm rollback procedures.

## Advanced Use Cases

### Q: Can I filter ingresses by namespace or labels?

A: **Namespace filtering is fully supported**. You can configure the controller to:

- **Watch all namespaces** (cluster-wide): Set `controller.watchNamespaces: ""` (default)
- **Watch specific namespaces**: Set `controller.watchNamespaces: "production,staging,default"`

Example configuration:

```yaml
# Watch only production and staging namespaces
controller:
  watchNamespaces: "production,staging"
```

**RBAC Requirements**:

- Cluster-wide monitoring requires `ClusterRole` permissions
- Namespace-scoped monitoring uses `Role` permissions in each specified namespace

For label-based filtering beyond namespace and ingress class, you would need to modify the controller's predicate functions.

### Q: Can I use multiple target CNAMEs for different ingresses?

A: The current implementation uses a single target CNAME for all ingresses. Supporting multiple targets would require code modifications to handle per-ingress or per-namespace configuration.

### Q: How do I integrate this with service mesh?

A: The controller works alongside service mesh solutions. Configure the target CNAME to point to your service mesh ingress gateway instead of the nginx controller.

### Q: Can I use this for TCP/UDP services?

A: The controller currently only handles HTTP/HTTPS ingresses. For TCP/UDP services, you would need custom CoreDNS configuration or a different approach.

### Q: What are preflight checks and when do they run?

A: Preflight checks are validation tests that run before the main controller starts, ensuring the environment is properly configured:

- **When**: Automatically during Helm install/upgrade as a pre-install hook
- **What they check**: CoreDNS deployment existence, mount path conflicts, ConfigMap conflicts, duplicate controllers
- **Fast-fail**: Jobs are configured with `backoffLimit: 0` to fail quickly and provide immediate feedback
- **Debugging**: Failed jobs are kept for 5 minutes (configurable via `jobs.failedJobTTL`) for log inspection

```bash
# View preflight job logs if installation fails
kubectl logs job/<release-name>-preflight -n <namespace>
```

### Q: How does the unique mount path feature work?

A: Each deployment gets a unique mount path to prevent conflicts when running multiple instances:

- **Default pattern**: `/etc/coredns/custom/{deployment-name}`
- **Automatic**: No configuration needed - paths are auto-generated
- **Custom**: Override with `controller.mountPath` if needed
- **Benefits**: Allows multiple controllers (different ingress classes) in the same cluster

Example with two deployments:

```bash
# nginx controller: /etc/coredns/custom/my-nginx-controller
# traefik controller: /etc/coredns/custom/my-traefik-controller
```

### Q: How long are failed preflight jobs kept?

A: Failed preflight jobs are kept for debugging purposes:

- **Default**: 5 minutes (300 seconds)
- **Configurable**: Set `jobs.failedJobTTL` in Helm values
- **Immediate cleanup**: Set to `0` to delete failed jobs immediately
- **Extended debugging**: Increase value for longer troubleshooting time

```yaml
jobs:
  failedJobTTL: 600  # 10 minutes
```

## Monitoring and Observability

### Q: Are there metrics available?

A: **Yes!** The controller provides comprehensive Prometheus metrics for monitoring and alerting:

**Reconciliation Metrics:**

- `coredns_ingress_sync_reconciliation_total` - Total reconciliations (success/error)
- `coredns_ingress_sync_reconciliation_duration_seconds` - Reconciliation latency
- `coredns_ingress_sync_reconciliation_errors_total` - Reconciliation errors by type

**DNS Management Metrics:**

- `coredns_ingress_sync_dns_records_managed_total` - Current DNS records count
- `coredns_ingress_sync_coredns_config_updates_total` - CoreDNS config updates
- `coredns_ingress_sync_coredns_config_update_duration_seconds` - Config update latency

**Ingress Monitoring Metrics:**

- `coredns_ingress_sync_ingresses_watched_total` - Watched ingresses per namespace
- `coredns_ingress_sync_ingresses_processed_total` - Processed ingresses by action

**System Metrics:**

- `coredns_ingress_sync_leader_election_status` - Leader election status (1=leader, 0=follower)
- `coredns_ingress_sync_coredns_config_drift_total` - Configuration drift detection/correction

**Metrics Configuration:**

```yaml
# Enable metrics (default: true)
metrics:
  enabled: true
  port: 8080
  path: /metrics

  # Prometheus ServiceMonitor
  serviceMonitor:
    enabled: true
    interval: 30s
    scrapeTimeout: 10s
```

**Accessing Metrics:**

```bash
# Port-forward to access metrics
kubectl port-forward deployment/coredns-ingress-sync 8080:8080 -n coredns-ingress-sync

# View metrics
curl http://localhost:8080/metrics
```

### Q: How do I monitor controller health?

A: The controller provides multiple health monitoring approaches:

**Health Endpoints:**

- `/healthz` (port 8081) - Liveness probe
- `/readyz` (port 8081) - Readiness probe
- `/metrics` (port 8080) - Prometheus metrics

**Key Metrics to Monitor:**

- `coredns_ingress_sync_reconciliation_errors_total` - Alert on increases
- `coredns_ingress_sync_reconciliation_duration_seconds` - Monitor latency
- `coredns_ingress_sync_leader_election_status` - Leader election health
- `coredns_ingress_sync_coredns_config_drift_total` - Configuration drift incidents

**Example Prometheus Alerts:**

```yaml
groups:
- name: coredns-ingress-sync
  rules:
  - alert: CorednsIngressSyncReconciliationErrors
    expr: increase(coredns_ingress_sync_reconciliation_errors_total[5m]) > 0
    for: 2m
    annotations:
      summary: "CoreDNS ingress sync reconciliation errors detected"

  - alert: CorednsIngressSyncHighLatency
    expr: histogram_quantile(0.95, coredns_ingress_sync_reconciliation_duration_seconds) > 5
    for: 5m
    annotations:
      summary: "CoreDNS ingress sync high reconciliation latency"
```

### Q: What logs are available?

A: The controller provides structured logging at multiple levels:

- `error` - Critical errors requiring attention
- `info` - Important operational events (default)
- `debug` - Detailed debugging information

Configure log level via:

```yaml
controller:
  logLevel: "info"  # debug, info, warn, error
```

Monitor CoreDNS metrics (enabled by default)

### Q: How do I monitor DNS resolution performance?

A: Use standard DNS monitoring tools:

- Monitor CoreDNS metrics (enabled by default)
- Test DNS resolution from application pods
- Track DNS query response times

### Q: What logs should I monitor in production?

A: Key log patterns to monitor:

- `Successfully updated dynamic ConfigMap` (successful operations)
- `Failed to` (error conditions)
- `Leader election` (leadership changes in multi-replica setups)

See the [Troubleshooting Guide](TROUBLESHOOTING.md) for detailed log analysis.
