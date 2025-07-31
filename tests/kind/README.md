# KIND Multi-Version Testing

This directory contains scripts for testing the CoreDNS Ingress Sync controller against multiple Kubernetes versions using KIND (Kubernetes in Docker).

## Overview

The KIND test suite validates our controller against all supported Kubernetes versions (1.25+) by:

- Creating KIND clusters with specific K8s versions
- Installing ingress-nginx in each cluster
- Building and deploying our controller
- Running integration tests to verify functionality
- Cleaning up all resources

## Prerequisites

Before running the tests, ensure you have:

- **Docker**: For running KIND clusters
- **KIND**: Kubernetes in Docker (`brew install kind` or see [installation guide](https://kind.sigs.k8s.io/docs/user/quick-start/))
- **kubectl**: Kubernetes CLI tool
- **helm**: Helm package manager
- **Go**: For building the controller (if not using Docker build)

## Usage

### Test All Supported Versions

```bash
# Run tests against all supported Kubernetes versions
./tests/kind/test-k8s-versions.sh
```

This will test each version in sequence:

- 1.29.14   # Previous stable
- 1.30.13   # 1.30 stable
- 1.31.9    # 1.31 stable
- 1.32.5    # 1.32 stable
- 1.33.1    # Latest stable

### Test Specific Version

```bash
# Test a specific Kubernetes version
./tests/kind/test-k8s-versions.sh --version 1.29.4
```

### List Supported Versions

```bash
# Show all supported versions
./tests/kind/test-k8s-versions.sh --list
```

### Cleanup

```bash
# Clean up all test clusters
./tests/kind/test-k8s-versions.sh --cleanup
```

### Help

```bash
# Show usage information
./tests/kind/test-k8s-versions.sh --help
```

## What Gets Tested

For each Kubernetes version, the test:

1. **Cluster Setup**:
   - Creates a 3-node KIND cluster (1 control-plane, 2 workers)
   - Configures port mappings for ingress-nginx
   - Waits for cluster to be ready

2. **Ingress Installation**:
   - Installs ingress-nginx controller
   - Waits for controller to be ready

3. **Controller Deployment**:
   - Builds controller Docker image
   - Loads image into KIND cluster
   - Deploys controller via Helm chart
   - Configures auto-configuration for CoreDNS

4. **Integration Testing**:
   - Creates test ingress with `test.example.com`
   - Verifies dynamic ConfigMap creation
   - Checks CoreDNS import statement addition
   - Validates rewrite rule generation
   - Tests DNS resolution within cluster

5. **Cleanup**:
   - Removes KIND cluster
   - Cleans up temporary files

## Test Results

The script provides detailed output for each step and a final summary:

```text
==========================================
TEST SUMMARY
==========================================
✅ PASSED (6):
  ✅ Kubernetes 1.25.16
  ✅ Kubernetes 1.26.15
  ✅ Kubernetes 1.27.13
  ✅ Kubernetes 1.28.9
  ✅ Kubernetes 1.29.4
  ✅ Kubernetes 1.30.0

All tests passed! 🎉
Controller is compatible with Kubernetes 1.25.16 through 1.30.0
```

## Troubleshooting

### Common Issues

1. **Docker not running**:

   ```text
   ❌ docker is not installed. Please install it.
   ```

   Start Docker Desktop or Docker daemon.

2. **KIND cluster creation fails**:

   ```text
   ❌ Failed to create KIND cluster
   ```

   Check if ports 80/443 are available, or if you have resource constraints.

3. **Image pull failures**:

   ```text
   ❌ Failed to build/load image
   ```

   Ensure Docker has enough disk space and memory.

4. **Integration test failures**:

   ```text
   ❌ Dynamic ConfigMap not found
   ```

   Check controller logs: `kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync`

### Resource Requirements

Each KIND cluster requires:

- **CPU**: ~1 core per cluster
- **Memory**: ~2GB per cluster
- **Disk**: ~1GB per cluster
- **Ports**: 80, 443 (for ingress-nginx)

Since tests run sequentially, only one cluster exists at a time.

### Timeouts

Default timeouts are:

- Cluster creation: 300s
- Component readiness: 300s
- Overall test timeout: 10m

Adjust these in the script if needed for slower environments.

## CI Integration

**Note**: These tests are designed for local development only, not CI/CD pipelines, because:

- KIND requires Docker-in-Docker or privileged containers
- Each test takes 5-10 minutes
- Resource requirements are significant
- Tests are comprehensive but slow

For CI, we use the existing fast unit and integration tests in `/tests/`.

## Customization

### Adding New Kubernetes Versions

Edit the `K8S_VERSIONS` array in `test-k8s-versions.sh`:

```bash
K8S_VERSIONS=(
    "1.25.16"
    "1.26.15" 
    # ... existing versions
    "1.31.0"    # Add new version
)
```

### Modifying Test Scenarios

The integration test function `run_integration_tests()` can be extended with additional test cases:

```bash
run_integration_tests() {
    # ... existing tests
    
    # Add custom test
    test_custom_scenario
}
```

### Changing Cluster Configuration

Modify the KIND config in `create_kind_cluster()` to adjust:

- Node count
- Port mappings  
- Kubernetes feature gates
- Container runtime settings

## Development Workflow

Use this test suite when:

1. **Upgrading dependencies**: Verify compatibility across K8s versions
2. **Adding new features**: Ensure they work on all supported versions
3. **Before releases**: Validate the entire compatibility matrix
4. **Debugging version-specific issues**: Test isolated versions

Example workflow:

```bash
# Quick test on latest version
./tests/kind/test-k8s-versions.sh --version 1.30.0

# If successful, test all versions
./tests/kind/test-k8s-versions.sh

# Clean up when done
./tests/kind/test-k8s-versions.sh --cleanup
```
