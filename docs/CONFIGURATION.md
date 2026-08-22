# Configuration Guide

This document provides detailed configuration options for the coredns-ingress-sync controller.

## Helm Chart Configuration

The controller is configured through Helm values. All configuration options are available in `helm/coredns-ingress-sync/values.yaml`.

### Controller Configuration

```yaml
controller:
  # Multi-class configuration (preferred). An ordered list of ingress class ->
  # target CNAME mappings. When set, it takes precedence over the legacy
  # ingressClass/targetCNAME pair below. List order is the default priority
  # tiebreak: the first entry has the lowest implicit priority, so when two
  # ingresses share a hostname the first-listed class wins unless a per-ingress
  # priority annotation overrides it.
  ingressClassMappings: []
  #  - ingressClass: nginx
  #    targetCNAME: "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
  #  - ingressClass: traefik
  #    targetCNAME: "traefik.traefik.svc.cluster.local."

  # Legacy single-class config (used only when ingressClassMappings is empty).
  # Ingress class to watch for changes
  ingressClass: "nginx"

  # Target service for DNS resolution (where ingress hostnames should resolve)
  targetCNAME: "ingress-nginx-controller.ingress-nginx.svc.cluster.local."

  # Gateway API support (Gateway + HTTPRoute), additive to Ingress support
  # above. Same shape and tiebreak semantics as ingressClassMappings, but
  # keyed on GatewayClass. Leave empty ([]) to disable Gateway API entirely --
  # no RBAC, watches, or CRD access are added when unset.
  gatewayClassMappings: []
  #  - gatewayClass: traefik
  #    targetCNAME: "traefik.traefik.svc.cluster.local."

  # Traefik IngressRoute support (traefik.io/v1alpha1), additive to Ingress and
  # Gateway API support above. IngressRoute has no class-like field, so unlike
  # the mappings above this is a single target CNAME. Leave empty ("") to
  # disable IngressRoute support entirely -- no RBAC, watches, or CRD access
  # are added when unset.
  ingressRouteTargetCNAME: ""
  #  ingressRouteTargetCNAME: "traefik.traefik.svc.cluster.local."

  # Namespace filtering - controls which namespaces to monitor for ingresses
  # Empty string = watch all namespaces cluster-wide (default)
  # Comma-separated list = watch only specific namespaces
  watchNamespaces: ""
  # Exclusions (applied after watchNamespaces)
  # Exclude specific namespaces entirely
  excludeNamespaces: ""
  # Exclude specific ingresses by name or namespace/name
  excludeIngresses: ""
  # Exclude specific HTTPRoutes by name or namespace/name
  excludeHTTPRoutes: ""
  # Annotation-based exclusion: when set to a false-like value on an Ingress, it will be ignored
  # Examples of false-like values: "false", "0", "no", "off", "disabled"
  annotationEnabledKey: "coredns-ingress-sync-enabled"
  # Annotation key for per-ingress priority (integer, higher wins). When two
  # ingresses (e.g. nginx and traefik) share a hostname, the one with the
  # higher value wins the CoreDNS rewrite. No annotation = config order wins.
  annotationPriorityKey: "coredns-ingress-sync-priority"
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
| `INGRESS_CLASS_MAPPINGS` | JSON array of `{"ingressClass","targetCNAME"}` mappings (preferred). When set, takes precedence over `INGRESS_CLASS`/`TARGET_CNAME`. Array order is the default priority tiebreak (first listed wins). | `""` (uses single-class shim) |
| `INGRESS_CLASS` | IngressClass to watch (legacy single-class; used when `INGRESS_CLASS_MAPPINGS` is unset) | `nginx` |
| `TARGET_CNAME` | Target service for DNS resolution (legacy single-class; used when `INGRESS_CLASS_MAPPINGS` is unset) | `ingress-nginx-controller.ingress-nginx.svc.cluster.local.` |
| `GATEWAY_CLASS_MAPPINGS` | JSON array of `{"gatewayClass","targetCNAME"}` mappings, additive to Ingress support. Same tiebreak semantics as `INGRESS_CLASS_MAPPINGS`. When unset (and `GATEWAY_CLASS` is unset), Gateway API support is disabled entirely — no watches, RBAC, or CRD List/Get calls. | `""` (Gateway API disabled) |
| `GATEWAY_CLASS` | GatewayClass to watch (legacy single-class; used when `GATEWAY_CLASS_MAPPINGS` is unset). Setting this enables Gateway API support. | `""` (Gateway API disabled) |
| `GATEWAY_TARGET_CNAME` | Target service for DNS resolution for the legacy single-class Gateway config (used when `GATEWAY_CLASS_MAPPINGS` is unset) | `""` |
| `INGRESSROUTE_TARGET_CNAME` | Single target CNAME every processable Traefik `IngressRoute` (`traefik.io/v1alpha1`) resolves to, additive to Ingress and Gateway API support. Unlike the mapping lists above, `IngressRoute` has no class-like field, so this is one value, not an ordered list. When unset, IngressRoute support is disabled entirely — no watches, RBAC, or CRD List/Get calls. | `""` (IngressRoute support disabled) |
| `WATCH_NAMESPACES` | Namespaces to monitor (empty = all) | `""` |
| `EXCLUDE_NAMESPACES` | Namespaces to exclude (comma-separated) | `""` |
| `EXCLUDE_INGRESSES` | Ingresses to exclude (name or namespace/name, comma-separated) | `""` |
| `EXCLUDE_HTTPROUTES` | HTTPRoutes to exclude (name or namespace/name, comma-separated) | `""` |
| `EXCLUDE_INGRESSROUTES` | IngressRoutes to exclude (name or namespace/name, comma-separated) | `""` |
| `ANNOTATION_ENABLED_KEY` | Annotation key to control inclusion; false-like value disables | `coredns-ingress-sync-enabled` |
| `ANNOTATION_PRIORITY_KEY` | Annotation key for per-ingress priority (integer, higher wins) when multiple ingresses share a hostname | `coredns-ingress-sync-priority` |
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

# Disable syncing for a specific ingress using an annotation (default key)
# metadata:
#   annotations:
#     coredns-ingress-sync-enabled: "false"

# Exclude certain namespaces and ingresses
controller:
  excludeNamespaces: "dev,qa"
  excludeIngresses: "legacy,production/no-sync"
```

### Annotation-based exclusions

Exclude a specific Ingress from internal DNS syncing by setting the configured
annotation key to a false-like value.

- Default key: `coredns-ingress-sync-enabled`
- False-like values (case-insensitive, trimmed): `false`, `0`, `no`, `off`, `disabled`
- Configure a custom key via Helm: `controller.annotationEnabledKey`

Example (using the default key):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: production
  annotations:
    coredns-ingress-sync-enabled: "false"
spec:
  ingressClassName: nginx
  rules:
    - host: web.example.com
      http:
        paths: []
```

### Multi-class configuration and per-ingress priority

A single controller can watch multiple ingress classes and rewrite each
hostname to the correct target CNAME. This is the mechanism used during an
ingress-nginx → Traefik migration, where both controllers serve the same
hostnames simultaneously (one Ingress per class per service).

Configure the ordered class mappings via Helm:

```yaml
controller:
  ingressClassMappings:
    - ingressClass: nginx
      targetCNAME: "ingress-nginx-controller.ingress-nginx.svc.cluster.local."
    - ingressClass: traefik
      targetCNAME: "traefik.traefik.svc.cluster.local."
  annotationPriorityKey: "coredns-ingress-sync-priority"
```

When two Ingresses declare the same hostname, the winner of the CoreDNS rewrite
is resolved as follows:

1. **Priority annotation (higher wins).** Set
   `coredns-ingress-sync-priority` to an integer on an Ingress to give it an
   explicit priority. The highest value wins the hostname.
2. **Config order (tiebreak).** Without annotations, all Ingresses share a
   baseline priority and the **first-listed class wins** (nginx in the example
   above). This is the safe default during migration.

`INGRESS_CLASS_MAPPINGS` takes precedence over the legacy `INGRESS_CLASS` /
`TARGET_CNAME` pair. Single-class deployments that set only the legacy values
continue to work unchanged.

#### Migration workflow (nginx → Traefik)

```yaml
# Both Ingresses exist for the same host. With no priority annotation,
# nginx (first in config) wins the internal rewrite — the safe default.
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-nginx
spec:
  ingressClassName: nginx
  rules:
    - host: app.example.com
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-traefik
  annotations:
    # Promote Traefik to win the CoreDNS rewrite for this hostname only.
    # Remove or lower this value to roll back instantly — no other service
    # is affected.
    coredns-ingress-sync-priority: "20"
spec:
  ingressClassName: traefik
  rules:
    - host: app.example.com
```

Migrate service-by-service: deploy both Ingresses (nginx wins by default), test
Traefik externally, then annotate the Traefik Ingress to flip the internal
rewrite. Removing the annotation rolls that single hostname back to nginx.

To use a custom key, set it in values and annotate with that key:

```yaml
# values.yaml
controller:
  annotationEnabledKey: "example.com/dns-sync-enabled"
```

```yaml
# Ingress
metadata:
  annotations:
    example.com/dns-sync-enabled: "false"
```

Note: This only controls internal CoreDNS rewrite generation and does not affect any external-dns records.

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

### Gateway API configuration

Gateway API support (`Gateway` + `HTTPRoute`) is additive to Ingress support and off by default.
Configure it the same way as multi-class Ingress, keyed on `GatewayClass` instead:

```yaml
controller:
  gatewayClassMappings:
    - gatewayClass: traefik
      targetCNAME: "traefik.traefik.svc.cluster.local."
```

An HTTPRoute's class is resolved by looking up its `spec.parentRefs[].name` (and optional
`.namespace`, defaulting to the HTTPRoute's own namespace) against the referenced `Gateway`'s
`spec.gatewayClassName`. `sectionName`/`port` on parentRefs are ignored — matching is done at
Gateway granularity, not listener granularity.

Cross-source tiebreak, when an Ingress and an HTTPRoute claim the same hostname, uses the same
`annotationPriorityKey` annotation and config-order rules as multi-class Ingress (see above), with
**Ingress winning ties by default** — the incumbent stays authoritative during an Ingress → Gateway
API migration, and can be promoted per-host via the priority annotation on the HTTPRoute.

`excludeHTTPRoutes` / `EXCLUDE_HTTPROUTES` and `annotationEnabledKey` behave the same as their
Ingress equivalents.

When a HTTPRoute doesn't set `spec.hostnames`, it inherits the hostname(s) declared on its parent
`Gateway`'s listener(s), per the Gateway API spec. If the Gateway's listener also has no hostname
(matching all hosts), that HTTPRoute contributes no CoreDNS rewrite rule — there's nothing
host-specific to rewrite.

**Known limitation**: HTTPRoute acceptance status
(`status.parents[].conditions[type=Accepted]`) is not checked. A rejected or conflicting HTTPRoute
(e.g. one that lost a hostname conflict at the Gateway) still produces a CoreDNS rewrite rule — this
matches how Ingress spec is trusted without checking controller-side acceptance.

When `gatewayClassMappings` (and the legacy `GATEWAY_CLASS`) are both unset, no Gateway/HTTPRoute
watches, RBAC rules, or CRD List/Get calls are added — a preflight check will also fail fast with a
clear message if Gateway API is enabled but the CRDs aren't installed or RBAC is missing.

### Traefik IngressRoute configuration

Traefik's native `IngressRoute` CRD (`traefik.io/v1alpha1`) support is additive to Ingress and
Gateway API support and off by default. Unlike those, `IngressRoute` has no class-like field, so
it's configured with a single target CNAME rather than an ordered mapping list:

```yaml
controller:
  ingressRouteTargetCNAME: "traefik.traefik.svc.cluster.local."
```

`IngressRoute` doesn't carry its hostname in a structured field — it's embedded in each
`spec.routes[].match` entry, a router-rule DSL string such as
`` Host(`api.example.com`) && PathPrefix(`/api`) ``. Literal, backtick-quoted arguments to `Host()`
matchers are extracted via regex (including comma-separated multi-host `Host(\`a.com\`,\`b.com\`)`
OR syntax); hosts are deduplicated within one `IngressRoute` across its `spec.routes[]` entries.

Cross-source tiebreak, when multiple sources claim the same hostname, uses the same
`annotationPriorityKey` annotation and config-order rules as multi-class Ingress and Gateway API
(see above), with **Ingress winning ties by default, then Gateway API, then IngressRoute** — the
incumbent stays authoritative during a migration, and any source can be promoted per-host via the
priority annotation.

`excludeIngressRoutes` / `EXCLUDE_INGRESSROUTES` and `annotationEnabledKey` behave the same as their
Ingress/HTTPRoute equivalents.

**Known limitations**:
- Only literal, backtick-quoted `Host(...)` matcher arguments are parsed. `HostRegexp`, `HostSNI`,
  and other matchers are not evaluated and contribute no candidates.
- The match expression's boolean structure (`&&`, `||`, negation like `` !Host(`x`) ``) is not
  evaluated — every `Host()` matcher found contributes a candidate regardless of whether it's
  actually reachable per the rule's logic. This matches how Ingress/HTTPRoute specs are trusted
  without evaluating controller-side acceptance or matching semantics.
- `IngressRouteTCP` and `IngressRouteUDP` (separate CRDs, no HTTP host semantics) are out of scope.

When `ingressRouteTargetCNAME` is unset, no `IngressRoute` watches, RBAC rules, or CRD List/Get
calls are added — a preflight check will also fail fast with a clear message if IngressRoute support
is enabled but the CRD isn't installed or RBAC is missing.

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
