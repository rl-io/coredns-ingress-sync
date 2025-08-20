# coredns-ingress-sync Helm Chart

This Helm chart deploys the CoreDNS Dynamic Internal Ingress Controller, which automatically creates internal DNS records for Kubernetes Ingress resources.

## Features

- ✅ **Automatic CoreDNS Configuration**: Patches CoreDNS ConfigMap with import statement
- ✅ **Automatic Cleanup**: Removes import statement when uninstalled
- ✅ **Secure Deployment**: Non-root user, read-only filesystem, minimal privileges
- ✅ **Security Best Practices**: RBAC, resource limits, security contexts
- ✅ **Configurable**: Support for multiple zones and environments
- ✅ **Lightweight**: Minimal resource usage and no persistent storage required

## Prerequisites

- Kubernetes 1.25+
- Helm 3.0+
- CoreDNS deployed in your cluster (standard in most Kubernetes distributions)
- RBAC enabled

> **Note**: Built with Kubernetes client libraries v0.33.3. Compatible with Kubernetes 1.25+ due to client-go's backward compatibility.

## Installation

### Quick Start

```bash
# Install directly from GitHub Packages (OCI registry)
helm install my-dns-controller \
  oci://ghcr.io/rl-io/charts/coredns-ingress-sync \
  --version 0.1.14 \
  --set coreDNS.autoConfigure=true \
  --set controller.targetCname="ingress-nginx-controller.ingress-nginx.svc.cluster.local."
```

### Local Installation

```bash
# Clone the repository
git clone https://github.com/rl-io/coredns-ingress-sync.git
cd coredns-ingress-sync

# Install from local chart
helm install my-dns-controller ./helm/coredns-ingress-sync \
  --set coreDNS.autoConfigure=true \
  --set controller.targetCname="ingress-nginx-controller.ingress-nginx.svc.cluster.local."
```

### Alternative: Traditional Helm Repository

If you prefer using a traditional Helm repository (requires GitHub Pages setup):

```bash
# Add the helm repository
helm repo add rl-io https://rl-io.github.io/coredns-ingress-sync

# Update repository
helm repo update

# Install the chart
helm install my-dns-controller rl-io/coredns-ingress-sync \
  --set coreDNS.autoConfigure=true \
  --set controller.targetCname="ingress-nginx-controller.ingress-nginx.svc.cluster.local."
```

## Configuration

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Controller image repository | `ghcr.io/rl-io/coredns-ingress-sync` |
| `image.tag` | Controller image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of replicas | `1` |
| `controller.ingressClass` | Ingress class to watch | `nginx` |
| `controller.targetCname` | Target service for DNS resolution | `ingress-nginx-controller.ingress-nginx.svc.cluster.local.` |
| `controller.watchNamespaces` | Namespaces to monitor (empty = all) | `""` |
| `controller.excludeNamespaces` | Namespaces to exclude | `""` |
| `controller.excludeIngresses` | Ingresses to exclude (name or namespace/name) | `""` |
| `controller.annotationEnabledKey` | Annotation key treated as boolean to enable/disable syncing | `coredns-ingress-sync-enabled` |
| `controller.logLevel` | Controller log level | `info` |

### Advanced Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.dynamicConfigMap.name` | Dynamic ConfigMap name | `coredns-ingress-sync-rewrite-rules` |
| `controller.dynamicConfigMap.key` | Dynamic ConfigMap key | `dynamic.server` |

### High Availability Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas (uses leader election for coordination) | `1` |

### CoreDNS Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `coreDNS.autoConfigure` | Automatically configure CoreDNS | `false` |
| `coreDNS.namespace` | CoreDNS namespace | `kube-system` |
| `coreDNS.configMapName` | CoreDNS ConfigMap name | `coredns` |

**Important**: By default, `coreDNS.autoConfigure` is `false` to prevent automatic changes to coreDNS. Set to `true` to enable automatic CoreDNS management.

### Resources Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Security Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext.runAsUser` | User ID to run as | `65534` |
| `podSecurityContext.runAsGroup` | Group ID to run as | `65534` |
| `podSecurityContext.runAsNonRoot` | Run as non-root user | `true` |
| `podSecurityContext.fsGroup` | File system group | `65534` |
| `securityContext.readOnlyRootFilesystem` | Read-only root filesystem | `true` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |
| `securityContext.capabilities.drop` | Dropped capabilities | `ALL` |

### RBAC Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbac.create` | Create RBAC resources | `true` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` (auto-generated) |
| `serviceAccount.annotations` | Service account annotations | `{}` |

### Metrics Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable Prometheus metrics endpoint | `true` |
| `metrics.port` | Metrics service port | `8080` |
| `metrics.path` | Metrics endpoint path | `/metrics` |
| `metrics.service.annotations` | Custom annotations for metrics service | `{}` |
| `metrics.service.labels` | Custom labels for metrics service | `{}` |
| `metrics.serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `metrics.serviceMonitor.interval` | Scrape interval | `30s` |
| `metrics.serviceMonitor.scrapeTimeout` | Scrape timeout | `10s` |
| `metrics.serviceMonitor.labels` | ServiceMonitor labels | `{}` |
| `metrics.serviceMonitor.annotations` | ServiceMonitor annotations | `{}` |

## Examples

### Multiple Ingress Classes

```yaml
# values.yaml
controller:
  ingressClass: "nginx"
  targetCname: "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
```

### Production Configuration

```yaml
# values-production.yaml
replicaCount: 2

image:
  repository: ghcr.io/rl-io/coredns-ingress-sync
  tag: "v1.0.0"
  pullPolicy: Always

resources:
  limits:
    cpu: 200m
    memory: 256Mi
  requests:
    cpu: 50m
    memory: 128Mi

controller:
  logLevel: "info"
```

### Metrics Configuration with Custom Annotations

```yaml
# values-metrics.yaml
metrics:
  enabled: true
  service:
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "8080"
      prometheus.io/path: "/metrics"
    labels:
      monitoring: "enabled"
  serviceMonitor:
    enabled: true
    interval: 30s
    labels:
      release: prometheus
```

### Development Configuration

```yaml
# values-dev.yaml
resources:
  limits:
    cpu: 50m
    memory: 64Mi
  requests:
    cpu: 10m
    memory: 32Mi

controller:
  logLevel: "debug"
```

## High Availability

The controller supports running multiple replicas for high availability using Kubernetes leader election:

```yaml
# values-ha.yaml
replicaCount: 2

# Spread replicas across nodes
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - coredns-ingress-sync
        topologyKey: kubernetes.io/hostname
```

**Note**: Only the leader replica will actively reconcile resources. Follower replicas will remain on standby until the leader becomes unavailable.

## Deployment

### Install with Custom Values

```bash
helm install my-dns-controller ./helm/coredns-ingress-sync \
  --values values-production.yaml
```

### Upgrade

```bash
helm upgrade my-dns-controller ./helm/coredns-ingress-sync \
  --values values-production.yaml
```

### Uninstall

```bash
helm uninstall my-dns-controller
```

## Lifecycle Management

The Helm chart includes automatic lifecycle management for CoreDNS configuration:

### Install Process

1. **Pre-install hook**: Adds `import /etc/coredns/custom/*.server` to CoreDNS Corefile
2. **Controller deployment**: Starts watching Ingress resources and creating DNS records
3. **CoreDNS restart**: Applies the new configuration

### Uninstall Process

1. **Pre-delete hook**: Removes the import statement from CoreDNS Corefile
2. **Controller cleanup**: Removes the controller deployment and associated resources  
3. **CoreDNS restart**: Applies the cleaned configuration

This ensures that your CoreDNS configuration remains clean and doesn't have any leftover import statements after uninstallation.

## Troubleshooting

### Check Controller Status

```bash
kubectl get pods -l app.kubernetes.io/name=coredns-ingress-sync
kubectl logs -l app.kubernetes.io/name=coredns-ingress-sync
```

### Verify CoreDNS Configuration

```bash
kubectl get configmap coredns -n kube-system -o yaml
```

### Check Zone Files

```bash
# Since zone files are stored in ConfigMaps, check the ConfigMaps instead
kubectl get configmaps -l app.kubernetes.io/name=coredns-ingress-sync
```

### Common Issues

1. **CoreDNS not restarting**: Check if the hook job completed successfully
2. **DNS resolution not working**: Verify the import statement was added to CoreDNS Corefile
3. **Controller not starting**: Check RBAC permissions and resource limits
4. **Zone files not created**: Verify Ingress resources match the configured IngressClass

## Security Considerations

- The controller runs as non-root user (65534)
- Read-only root filesystem with minimal writable volumes
- RBAC with minimal required permissions
- Security contexts follow Kubernetes security best practices

## License

This project is licensed under the MIT License - see the LICENSE file for details.
