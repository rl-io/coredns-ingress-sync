# Configuration Guide

This document provides detailed configuration options for the coredns-ingress-sync controller.

## Helm Chart Configuration

The controller is configured through Helm values. All configuration options are available in `helm/coredns-ingress-sync/values.yaml`.

### Controller Configuration

```yaml
controller:
  # Ingress class to watch for changes
  ingressClass: "nginx"
  
  # Target service for DNS resolution (where ingress hostnames should resolve)
  targetCNAME: "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
  
  # Namespace filtering - controls which namespaces to monitor for ingresses
  # Empty string = watch all namespaces cluster-wide (default)
  # Comma-separated list = watch only specific namespaces
  watchNamespaces: ""
  # Examples:
  # watchNamespaces: "production,staging"  # Watch only production and staging
  # watchNamespaces: "default"             # Watch only default namespace
  
  # Dynamic ConfigMap configuration
  dynamicConfigMap:
    name: "coredns-ingress-sync-rewrite-rules"
    key: "dynamic.server"
  
  # Leader election (for multiple replicas)
  leaderElection:
    enabled: true
    
  # Logging configuration
  logLevel: "info"
```

### CoreDNS Integration

```yaml
coreDNS:
  # Automatically configure CoreDNS
  # IMPORTANT: Default is false for safety - set to true to enable
  autoConfigure: false
  
  # CoreDNS namespace
  namespace: "kube-system"
  
  # CoreDNS ConfigMap name
  configMapName: "coredns"
```

**⚠️ Safety First**: By default, `autoConfigure` is `false` to prevent unexpected changes to your CoreDNS configuration. You must explicitly enable it.

### Metrics Configuration

```yaml
# Metrics and monitoring configuration
metrics:
  # Enable Prometheus metrics endpoint (default: true)
  enabled: true
  port: 8080
  path: /metrics
  
  # Service configuration for metrics endpoint
  service:
    annotations: {}
    labels: {}
  
  # ServiceMonitor configuration (requires Prometheus Operator)
  serviceMonitor:
    enabled: false
    interval: 30s
    scrapeTimeout: 10s
    labels: {}
    annotations: {}
```

**Available Metrics:**

- `coredns_ingress_sync_reconciliation_total{result}` - Reconciliation attempts
- `coredns_ingress_sync_reconciliation_duration_seconds{result}` - Reconciliation latency  
- `coredns_ingress_sync_dns_records_managed_total` - Current DNS records managed
- `coredns_ingress_sync_coredns_config_updates_total{result}` - CoreDNS config updates
- `coredns_ingress_sync_leader_election_status` - Leader election status
- `coredns_ingress_sync_coredns_config_drift_total{drift_type}` - Configuration drift events

### Volume Mount Configuration

```yaml
controller:
  # Custom volume name for mounting dynamic configuration
  volumeName: "coredns-ingress-sync-volume"
  
  # Custom mount path for dynamic configuration  
  # If empty, defaults to: /etc/coredns/custom/{deployment-name}
  # This allows multiple deployments with unique mount paths
  mountPath: ""
  
  # Dynamic ConfigMap configuration
  dynamicConfigMap:
    name: "coredns-ingress-sync-rewrite-rules"
    key: "dynamic.server"
```

### Job Configuration

```yaml
jobs:
  # How long to keep failed preflight jobs for debugging (in seconds)
  # Set to 0 to delete immediately, or increase for longer debugging time
  failedJobTTL: 300  # 5 minutes (default)
```

### Health Check Configuration

```yaml
# Health check configuration
healthCheck:
  enabled: true
  port: 8081
  path: /healthz
```

### Resource Configuration

```yaml
# Pod resource limits and requests
resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi

# Replica count (supports leader election)
replicaCount: 1

# Security context
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
  readOnlyRootFilesystem: true

podSecurityContext:
  fsGroup: 65534
  runAsGroup: 65534
  runAsNonRoot: true
  runAsUser: 65534
```

## Environment Variables

The controller supports configuration through environment variables (set via Helm values):

| Variable | Description | Default |
|----------|-------------|---------|
| `INGRESS_CLASS` | IngressClass to watch | `nginx` |
| `TARGET_CNAME` | Target service for DNS resolution | `ingress-nginx-controller.ingress-nginx.svc.cluster.local.` |
| `WATCH_NAMESPACES` | Namespaces to monitor (empty = all) | `""` |
| `COREDNS_NAMESPACE` | CoreDNS namespace | `kube-system` |
| `COREDNS_CONFIGMAP_NAME` | CoreDNS ConfigMap name | `coredns` |
| `COREDNS_VOLUME_NAME` | CoreDNS volume name | `coredns-ingress-sync-volume` |
| `MOUNT_PATH` | Custom mount path for dynamic config | `""` (auto-generated) |
| `DYNAMIC_CONFIGMAP_NAME` | Dynamic ConfigMap name | `coredns-ingress-sync-rewrite-rules` |
| `DYNAMIC_CONFIG_KEY` | Key in dynamic ConfigMap | `dynamic.server` |
| `LEADER_ELECTION_ENABLED` | Enable leader election | `true` |
| `LOG_LEVEL` | Logging level | `info` |
| `COREDNS_AUTO_CONFIGURE` | Auto-configure CoreDNS | `false` |
| `METRICS_ENABLED` | Enable metrics endpoint | `true` |
| `METRICS_PORT` | Metrics endpoint port | `8080` |
| `HEALTH_CHECK_ENABLED` | Enable health check endpoint | `true` |
| `HEALTH_CHECK_PORT` | Health check endpoint port | `8081` |

## Custom Configuration Examples

### Multiple Deployments with Unique Mount Paths

When deploying multiple instances, each gets a unique mount path to prevent conflicts:

```bash
# First deployment: mount path = /etc/coredns/custom/coredns-ingress-sync-nginx
helm install coredns-ingress-sync-nginx ./helm/coredns-ingress-sync \
  --set coreDNS.autoConfigure=true \
  --set controller.ingressClass=nginx \
  --set controller.dynamicConfigMap.name=coredns-nginx \
  --namespace coredns-ingress-sync \
  --create-namespace

# Second deployment: mount path = /etc/coredns/custom/coredns-ingress-sync-traefik  
helm install coredns-ingress-sync-traefik ./helm/coredns-ingress-sync \
  --set coreDNS.autoConfigure=true \
  --set controller.ingressClass=traefik \
  --set controller.dynamicConfigMap.name=coredns-traefik \
  --set controller.targetCNAME=traefik.traefik.svc.cluster.local. \
  --namespace coredns-ingress-sync
```

**Mount Path Generation**:

- Default: `/etc/coredns/custom/{deployment-name}`
- Custom: Set `controller.mountPath` explicitly
- Prevents mount path conflicts between multiple deployments

### Preflight Checks

The Helm chart includes preflight checks that validate the environment before deployment:

```bash
# View preflight job logs if installation fails
kubectl logs job/coredns-ingress-sync-preflight -n coredns-ingress-sync

# Manual preflight check (during development)
helm install test ./helm/coredns-ingress-sync \
  --dry-run --debug \
  --set coreDNS.autoConfigure=true \
  --namespace coredns-ingress-sync
```

### Namespace Filtering

Control which namespaces the controller monitors for ingress resources:

```yaml
# Watch all namespaces (cluster-wide monitoring)
controller:
  watchNamespaces: ""

# Watch specific namespaces only
controller:
  watchNamespaces: "production,staging"

# Watch only the default namespace
controller:
  watchNamespaces: "default"
```

**RBAC Requirements by Configuration**:

- **Cluster-wide** (`watchNamespaces: ""`): Requires `ClusterRole` with ingress read permissions
- **Namespace-scoped** (`watchNamespaces: "ns1,ns2"`): Requires `Role` in each specified namespace

```bash
# Deploy with namespace filtering
helm install coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set coreDNS.autoConfigure=true \
  --set controller.watchNamespaces="production,staging" \
  --namespace coredns-ingress-sync \
  --create-namespace
```

### Custom Target Service

```yaml
# values-custom.yaml
controller:
  targetCNAME: "my-ingress-controller.my-namespace.svc.cluster.local."
  ingressClass: "my-ingress-class"
```

### High Availability Setup

```yaml
# values-ha.yaml
replicaCount: 3

controller:
  leaderElection:
    enabled: true

resources:
  limits:
    cpu: 200m
    memory: 256Mi
  requests:
    cpu: 20m
    memory: 128Mi

# Pod anti-affinity for spread across nodes
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: coredns-ingress-sync
        topologyKey: kubernetes.io/hostname
```

### Resource Constraints

For clusters with limited resources:

```yaml
# values-minimal.yaml
resources:
  limits:
    cpu: 50m
    memory: 64Mi
  requests:
    cpu: 5m
    memory: 32Mi

# Disable auto-configuration if CoreDNS management is handled externally
coreDNS:
  autoConfigure: false
```

### Development/Testing Configuration

```yaml
# values-dev.yaml
controller:
  logLevel: "debug"

# Use local image
image:
  tag: "latest"
  pullPolicy: "Never"

# Single replica for testing
replicaCount: 1
```

## Validation

After configuration changes, validate the setup:

```bash
# Check controller status
kubectl get pods -n coredns-ingress-sync
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync

# Verify CoreDNS configuration
kubectl get configmap coredns -n kube-system -o yaml | grep -A 5 "import"

# Check dynamic ConfigMap
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml

# Test DNS resolution
kubectl run test-pod --rm -i --tty --image=busybox -- nslookup your-hostname.example.com
```

## Configuration Best Practices

1. **Resource Limits**: Always set appropriate resource limits based on your cluster size
2. **Leader Election**: Enable leader election for production deployments with multiple replicas
3. **Security Context**: Use the provided security context for least privilege
4. **Monitoring**: Configure log level appropriately (`info` for production, `debug` for troubleshooting)
5. **Backup**: Keep copies of your custom values files in version control
6. **Testing**: Test configuration changes in a non-production environment first

## Troubleshooting Configuration

Common configuration issues and solutions:

### Controller Not Starting

Check resource constraints and RBAC permissions:

```bash
kubectl describe pod -n coredns-ingress-sync -l app.kubernetes.io/name=coredns-ingress-sync
kubectl get events -n coredns-ingress-sync
```

### CoreDNS Integration Issues

Verify CoreDNS configuration:

```bash
# Check if import statement was added
kubectl get configmap coredns -n kube-system -o yaml | grep "import /etc/coredns/custom"

# Check volume mount
kubectl get deployment coredns -n kube-system -o yaml | grep -A 10 "volumeMounts"
```

### DNS Resolution Not Working

Verify the complete configuration chain:

```bash
# 1. Check ingress is being processed
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync | grep "Successfully updated"

# 2. Check dynamic ConfigMap has content
kubectl get configmap coredns-ingress-sync-rewrite-rules -n kube-system -o yaml

# 3. Test DNS resolution from within cluster
kubectl run test-pod --rm -i --tty --image=busybox -- nslookup your-hostname.example.com
```
