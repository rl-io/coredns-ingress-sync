# Development Guide

This guide covers development setup, building, testing, and contributing to the coredns-ingress-sync project.

## Development Environment Setup

### Prerequisites

- **asdf**: Version manager for Go and Node.js ([install guide](https://asdf-vm.com/guide/getting-started.html))
- **Docker**: For building container images
- **Kubernetes cluster**: For testing (kind, minikube, k3s, etc.)
- **Helm 3.x**: For chart development and testing
- **kubectl**: Configured for your test cluster

> **Note**: The project uses asdf to manage Go and Node.js versions automatically via `.tool-versions`

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/rl-io/coredns-ingress-sync.git
cd coredns-ingress-sync

# Install required tool versions (Go + Node.js)
asdf install

# Setup complete development environment (includes git hooks)
make setup-dev

# Or setup manually:
go mod download         # Install Go dependencies  
npm install            # Install commit tools (optional but recommended)

# Verify setup
asdf current            # Verify tool versions
go version
docker --version
kubectl version --client
helm version
```

### Local Development Cluster

Set up a local Kubernetes cluster for development:

#### Using kind (Recommended)

```bash
# Create kind cluster with CoreDNS
cat > kind-config.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: coredns-test
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
  - containerPort: 443
    hostPort: 8443
EOF

kind create cluster --config kind-config.yaml

# Install ingress-nginx
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml

# Wait for ingress-nginx to be ready
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=90s
```

#### Using minikube

```bash
# Start minikube
minikube start --driver=docker

# Enable ingress addon
minikube addons enable ingress

# Set up DNS
minikube addons enable coredns
```

## Building the Project

### Local Binary

```bash
# Build for local platform
make build

# Or manually
go build -o controller ./cmd/coredns-ingress-sync

# Test the binary
./controller --help
```

### Docker Image

```bash
# Build Docker image
make docker-build

# Or manually
docker build -t coredns-ingress-sync:latest .

# Tag for development
docker tag coredns-ingress-sync:latest coredns-ingress-sync:dev
```

### Cross-platform Builds

```bash
# Build for multiple platforms
make build-cross-platform

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o controller-linux-amd64 ./cmd/coredns-ingress-sync
```

## Testing

The project includes comprehensive testing with safety checks to prevent testing against production clusters.

### Safety Checks

The test suite validates the kubecontext to ensure you're testing against a safe environment:

**Safe contexts:**

- `kind-*` (kind clusters)
- `minikube`
- `docker-desktop`
- `k3s-default`
- `localhost`
- `orbstack`
- Contexts containing `test`, `dev`, or `development`

### Running Tests

```bash
# Run all tests
./tests/run_tests.sh

# Run specific test suites
./tests/run_tests.sh --unit           # Unit tests only
./tests/run_tests.sh --integration    # Integration tests only
./tests/run_tests.sh --e2e           # End-to-end tests only

# Run unit tests directly
go test -v .

# Run with coverage
go test -v -cover .
```

### Test Categories

#### Unit Tests

Located in `main_test.go`, these test individual functions:

```bash
# Run unit tests
go test -v .

# With coverage report
go test -v -cover . -coverprofile=coverage.out
go tool cover -html=coverage.out
```

#### Integration Tests

Located in `tests/integration_test.sh`, these test the controller against a real Kubernetes cluster:

```bash
# Run integration tests
./tests/integration_test.sh

# Run specific integration scenario
TEST_SCENARIO="basic_ingress" ./tests/integration_test.sh
```

#### End-to-End Tests

Located in `tests/e2e_test.sh`, these test complete workflows:

```bash
# Run e2e tests
./tests/e2e_test.sh

# Run with custom domain
TEST_DOMAIN="mytest.local" ./tests/e2e_test.sh
```

#### Performance Tests

Located in `tests/benchmark_test.sh`, these measure performance characteristics:

```bash
# Run performance benchmarks
./tests/benchmark_test.sh

# Results are saved to tests/test_results/
```

### Test Configuration

Tests use environment variables for configuration:

```bash
# Set test domain
export TEST_DOMAIN="k8s.example.com"

# Set test namespace
export NAMESPACE="coredns-ingress-sync"

# Enable verbose logging
export TEST_VERBOSE="true"
```

## Local Development Workflow

### 1. Code Development

```bash
# Make your changes to the modular codebase
vim cmd/coredns-ingress-sync/main.go  # Entry point
vim internal/controller/reconciler.go  # Core logic
vim internal/ingress/filter.go         # Ingress filtering

# Test changes
go test -v ./...

# Build locally
make build
```

### 2. Local Testing

```bash
# Build and load image into kind
make docker-build
kind load docker-image coredns-ingress-sync:latest --name coredns-test

# Deploy with local image
helm install coredns-ingress-sync ./helm/coredns-ingress-sync \
  --namespace coredns-ingress-sync \
  --create-namespace \
  --set coreDNS.autoConfigure=true \
  --set image.tag=latest \
  --set image.pullPolicy=Never \
  --set controller.logLevel=debug

# Watch logs
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync -f
```

### 3. Testing Changes

```bash
# Create test ingress
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ingress
  namespace: default
spec:
  ingressClassName: nginx
  rules:
  - host: test.k8s.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: test-service
            port:
              number: 80
EOF

# Check if DNS configuration was generated
kubectl get configmap coredns-custom -n kube-system -o yaml

# Test DNS resolution
kubectl run test-pod --rm -i --tty --image=busybox -- nslookup test.k8s.example.com
```

### 4. Cleanup

```bash
# Remove test resources
kubectl delete ingress test-ingress
helm uninstall coredns-ingress-sync -n coredns-ingress-sync

# Clean up kind cluster (if done)
kind delete cluster --name coredns-test
```

## Helm Chart Development

### Chart Structure

```text
helm/coredns-ingress-sync/
├── Chart.yaml              # Chart metadata
├── values.yaml             # Default values
├── values-dev.yaml         # Development values
├── values-production.yaml  # Production values
├── templates/
│   ├── deployment.yaml     # Controller deployment
│   ├── rbac.yaml          # RBAC configuration
│   ├── serviceaccount.yaml # Service account
│   └── cleanup-job.yaml   # Cleanup job
└── scripts/
    └── cleanup.sh         # Cleanup script
```

### Chart Testing

```bash
# Lint the chart
helm lint ./helm/coredns-ingress-sync

# Template rendering test
helm template test ./helm/coredns-ingress-sync --debug

# Install locally
helm install test ./helm/coredns-ingress-sync \
  --namespace test \
  --create-namespace \
  --dry-run

# Test with different values
helm install test ./helm/coredns-ingress-sync \
  --values ./helm/coredns-ingress-sync/values-dev.yaml \
  --dry-run
```

### Chart Validation

```bash
# Validate chart with helm-test
./helm/validate-chart.sh

# Check security policies
./tests/test_safety_check.sh
```

## Debugging and Troubleshooting

For comprehensive debugging and troubleshooting information, see the **[Troubleshooting Guide](TROUBLESHOOTING.md)**.

### Quick Development Debugging

```bash
# Enable debug logging for development
helm upgrade coredns-ingress-sync ./helm/coredns-ingress-sync \
  --set controller.logLevel=debug \
  --namespace coredns-ingress-sync

# Follow logs
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync -f
```

### Common Development Issues

For detailed solutions to common issues including:

- Controller startup problems
- DNS resolution issues  
- ConfigMap update failures
- Performance problems

See the [Troubleshooting Guide > Common Issues](TROUBLESHOOTING.md#common-issues-and-solutions).

## Code Style and Standards

### Go Code Standards

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` for formatting
- Add comments for exported functions
- Write tests for new functionality

```bash
# Format code
gofmt -w .

# Vet code
go vet .

# Run linter (if available)
golangci-lint run
```

### Documentation Standards

- Update README for user-facing changes
- Add technical details to `docs/ARCHITECTURE.md`
- Document configuration in `docs/CONFIGURATION.md`
- Include examples in documentation

### Commit Standards

Follow conventional commit format:

```text
feat: add new feature
fix: bug fix
docs: documentation changes
test: add or update tests
refactor: code refactoring
```

### Format

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Types

- **feat**: A new feature
- **fix**: A bug fix
- **docs**: Documentation only changes
- **style**: Changes that do not affect the meaning of the code (white-space, formatting, etc)
- **refactor**: A code change that neither fixes a bug nor adds a feature
- **perf**: A code change that improves performance
- **test**: Adding missing tests or correcting existing tests
- **build**: Changes that affect the build system or external dependencies
- **ci**: Changes to CI configuration files and scripts
- **chore**: Other changes that don't modify src or test files
- **revert**: Reverts a previous commit

### Examples

```text
feat: add leader election support
fix: resolve ConfigMap update race condition
docs: update installation guide
feat(controller): add support for custom ingress classes
fix!: change API response format (breaking change)
test: add integration test for cleanup
```

### Automatic Git Hooks Setup

Git hooks are **automatically installed** when you run the development setup:

```bash
# Git hooks are installed automatically with either:
make setup-dev          # Complete development setup
npm install             # Just install commit tools

# Verify hooks are working
git commit -m "invalid message"     # Should fail validation
git commit -m "feat: add feature"   # Should pass validation
```

### Manual Git Hooks Setup (if needed)

If automatic setup didn't work, you can set up manually:

```bash
# Manual setup (fallback)
npm run setup-hooks

# Interactive commit builder
npm run commit           # Guided commit creation
```

### Validation Tools

This project supports multiple validation approaches:

1. **commitlint** (Recommended) - Industry standard tool
   - Interactive commit builder: `npm run commit`
   - Automatic validation via git hooks
   - CI/CD integration

2. **Custom validator** (Fallback) - Works without Node.js
   - Basic validation with helpful messages
   - Used when commitlint is not available

3. **pre-commit** (Optional) - Additional validation
   - Install with: `pip install pre-commit && pre-commit install`

### Validation

- **Pre-commit**: Git hooks validate commits before they're created (primary enforcement)
- **Interactive**: Use `npm run commit` for guided commit creation
- **Manual**: Use `npx commitlint --from=HEAD~1` to validate the last commit

### Breaking Changes

For breaking changes, add `!` after the type/scope:

```text
feat!: change API response format
fix(api)!: remove deprecated endpoint
```

Or include `BREAKING CHANGE:` in the footer:

```text
feat: add new configuration option

BREAKING CHANGE: Default configuration format has changed
```

## Contributing

### Setup Development Environment

1. **Fork and clone the repository**
2. **Setup development environment**: `make setup-dev` (includes automatic git hooks)
3. **Install dependencies**: `go mod download`
4. **Run tests**: `./tests/run_tests.sh`

### Pull Request Process

1. **Fork the repository**
2. **Create feature branch**: `git checkout -b feature/amazing-feature`
3. **Make changes** with tests
4. **Run full test suite**: `./tests/run_tests.sh`
5. **Update documentation** as needed
6. **Commit changes**: Follow conventional commit format
7. **Push to branch**: `git push origin feature/amazing-feature`
8. **Open Pull Request**

### Review Checklist

- [ ] Tests pass locally
- [ ] New functionality has tests
- [ ] Documentation updated
- [ ] Backward compatibility maintained
- [ ] Security considerations addressed
- [ ] Performance impact considered

### Release Process

1. **Version bumping**: Update Chart.yaml and relevant files
2. **Testing**: Full test suite on multiple Kubernetes versions
3. **Documentation**: Update CHANGELOG and README
4. **Tagging**: Create git tag following semantic versioning
5. **Packaging**: Build and publish Helm chart
6. **Announcement**: Update GitHub releases

## Useful Development Commands

```bash
# Quick development cycle
make build && make docker-build && kind load docker-image coredns-ingress-sync:latest --name coredns-test

# Watch controller logs
kubectl logs -n coredns-ingress-sync deployment/coredns-ingress-sync -f | grep -v "Reconciling"

# Check all resources
kubectl get all,configmaps,secrets -n coredns-ingress-sync

# Reset test environment
helm uninstall coredns-ingress-sync -n coredns-ingress-sync && \
kubectl delete namespace coredns-ingress-sync && \
kubectl delete configmap coredns-custom -n kube-system
```
