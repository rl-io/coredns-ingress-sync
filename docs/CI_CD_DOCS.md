# CI/CD Pipeline Documentation

This document describes the comprehensive, modular CI/CD pipeline setup for the coredns-ingress-sync controller project.

## Overview

The project uses GitHub Actions for continuous integration and deployment, with a **modular approach** using reusable actions and multiple specialized workflows for different aspects of the development lifecycle.

## Modular Architecture

### Reusable Actions

The pipeline is built on four core reusable actions located in `.github/actions/`:

#### 1. Docker Build Action (`.github/actions/docker-build/action.yml`)
**Purpose**: Builds Docker images with consistent tagging, caching, and multi-platform support

**Features**:
- ✅ Consistent tagging strategy across workflows
- 📦 Multi-platform builds (AMD64/ARM64)
- 💾 GitHub Actions cache optimization
- 🔧 Configurable push/export options
- 📤 Artifact export for downstream jobs

**Usage**:
```yaml
- uses: ./.github/actions/docker-build
  with:
    image_name: coredns-ingress-sync
    push: false
    platforms: linux/amd64
    export_artifact: true
```

#### 2. Security Scan Action (`.github/actions/security-scan/action.yml`)
**Purpose**: Trivy-based security scanning for containers and filesystem

**Features**:
- 🔒 Container vulnerability scanning
- 📁 Filesystem security analysis
- 📊 SARIF uploads to GitHub Security tab
- 🎯 Configurable scan targets
- 📋 Artifact retention for scan results

**Usage**:
```yaml
- uses: ./.github/actions/security-scan
  with:
    image_name: coredns-ingress-sync:test
    image_artifact_path: /tmp
    scan_filesystem: true
    upload_sarif: true
```

#### 3. Test Runner Action (`.github/actions/test-runner/action.yml`)
**Purpose**: Comprehensive Go testing with Kubernetes Kind clusters

**Features**:
- 🧪 Unit, integration, and E2E tests
- ☸️ Kind cluster provisioning
- 📈 Codecov integration
- 🔄 Configurable test suites
- 📊 Coverage reporting

**Usage**:
```yaml
- uses: ./.github/actions/test-runner
  with:
    go_version: '1.24'
    run_integration_tests: true
    run_e2e_tests: true
    codecov_token: ${{ secrets.CODECOV_TOKEN }}
```

#### 4. PR Status Update Action (`.github/actions/update-pr-status/action.yml`)
**Purpose**: Updates PR status checks for release-please workflows

**Features**:
- ✅ Automated status check updates
- 🔗 PR integration with repository dispatch
- � Configurable status messages
- 🎯 Targeted PR status management

**Usage**:
```yaml
- uses: ./.github/actions/update-pr-status
  with:
    context: "CI/CD Pipeline / build"
    state: "success"
    description: "Build completed successfully"
    pr_number: ${{ github.event.client_payload.pr_number }}
```

## Workflows

### 1. Pull Request Tests (`.github/workflows/pr-tests.yml`)

**Triggers:**
- Pull request events (opened, synchronize, reopened)

**Jobs:**
- **Detect Changes**: Smart change detection for targeted testing
- **Build Docker Image**: Uses reusable docker-build action
- **Run Tests**: Uses reusable test-runner action
- **Security Scan**: Uses reusable security-scan action
- **Documentation Check**: Markdown validation

**Features:**
- ⚡ Fast feedback loop for PRs
- 🎯 Only runs tests for changed components
- 🔄 Parallel execution where possible
- 📊 Artifact-based workflow (build once, test multiple times)

### 2. Main CI/CD Pipeline (`.github/workflows/ci-cd.yml`)

**Triggers:**
- Repository dispatch events from release-please
- Manual workflow dispatch

**Jobs:**
- **Debug Information**: Workflow context logging
- **Trigger Build and Test**: Dispatches to build-test workflow

**Features:**
- 🔗 Integration with release-please
- 📋 Centralized orchestration
- 🚀 Event-driven automation

### 3. Build and Test Workflow (`.github/workflows/build-test.yml`)

**Triggers:**
- Repository dispatch from CI/CD pipeline
- Manual workflow dispatch

**Jobs:**
- **Setup PR Status**: Initializes status checks for release-please PRs
- **Build**: Uses reusable docker-build action
- **Test**: Uses reusable test-runner action  
- **Security Scan**: Uses reusable security-scan action

**Features:**
- � Status check management for release-please
- 📦 Artifact passing between jobs
- ✅ Comprehensive validation pipeline

### 4. Build and Push (`.github/workflows/build-push.yml`)

**Triggers:**
- Push to `main` branch
- Version tags (`v*`)
- Manual workflow dispatch

**Jobs:**
- **Build and Push**: Production Docker builds with registry push

**Features:**
- 🏗️ Multi-platform production builds
- 📦 Container registry publishing
- 🔧 Uses reusable docker-build action

### 5. Security Scanning (`.github/workflows/security.yml`)

**Triggers:**
- Schedule (daily)
- Push to main
- Manual workflow dispatch

**Jobs:**
- **CodeQL Analysis**: GitHub's semantic code analysis
- **Dependency Review**: Automated dependency security checks

**Features:**
- 🔒 Comprehensive security analysis
- 📊 SARIF integration with GitHub Security tab
- ⏰ Scheduled security monitoring

### 3. Security and Maintenance (`.github/workflows/security.yml`)

**Triggers:**
- Daily schedule (2 AM UTC)
- Manual trigger

**Jobs:**
- **Dependency Update**: Automated dependency updates
- **Security Scan**: Daily vulnerability scanning
- **Image Cleanup**: Removes old container images

**Features:**
- 🔄 Automated dependency updates with PR creation
- 🛡️ Daily security scanning
- 🧹 Automatic cleanup of old container images
- 📈 Security audit with govulncheck and gosec

## Setup Instructions

### 1. Repository Setup

Enable the following in your GitHub repository:

```yaml
# Repository settings
Settings > Actions > General:
  - Actions permissions: Allow all actions and reusable workflows
  - Workflow permissions: Read and write permissions
  - Allow GitHub Actions to create and approve pull requests: ✅

Settings > Security > Code security and analysis:
  - Dependency graph: ✅
  - Dependabot alerts: ✅  
  - Dependabot security updates: ✅
  - Code scanning: ✅
```

### 2. Required Secrets

The workflows use the following secrets (most are automatically provided):

- `GITHUB_TOKEN`: Automatically provided by GitHub
- `CODECOV_TOKEN`: (Optional) For coverage reporting

### 3. Container Registry

The pipeline publishes to GitHub Container Registry (ghcr.io):

```bash
# Images are available at:
ghcr.io/YOUR_USERNAME/coredns-ingress-sync:latest
ghcr.io/YOUR_USERNAME/coredns-ingress-sync:v1.0.0
```

### 4. Branch Protection

Recommended branch protection rules for `main`:

```yaml
Branch protection rules:
- Require pull request reviews before merging
- Require status checks to pass before merging:
  - CI/CD Pipeline / build
  - CI/CD Pipeline / test
  - CI/CD Pipeline / security-scan
  - CI/CD Pipeline / build-and-push
- Require branches to be up to date before merging
- Require linear history
- Do not allow force pushes
- Do not allow deletions
```

## Local Development

### Prerequisites

Install development tools:

```bash
# Setup development environment
make dev-setup

# Or manually:
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
go install github.com/aquasecurity/trivy/cmd/trivy@latest
```

### Development Workflow

```bash
# 1. Run tests locally (mimics CI)
make ci-test

# 2. Build and test Docker image
make docker-build
make docker-scan

# 3. Test with local Kind cluster
make kind-setup
make k8s-deploy

# 4. Clean up
make kind-delete
make clean
```

### Testing

```bash
# Run all tests
make test-all

# Run specific test types
make test           # Unit tests
make test-integration
make test-e2e
make benchmark

# Check test safety
make test-safety
```

## Release Process

### Automated Release

1. **Create a tag:**
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. **GitHub Actions will automatically:**
   - Run full test suite
   - Build multi-architecture images
   - Sign container images
   - Generate SBOM
   - Create GitHub release with artifacts

### Manual Release

```bash
# Test release process locally
make release-dry-run

# Full release (builds, tests, pushes)
make release
```

## Security Features

### Image Security
- 🔒 Non-root user (65534)
- 📦 Scratch base image (minimal attack surface)
- 🔐 Signed container images with Cosign
- 🛡️ Daily vulnerability scanning
- 📋 SBOM generation for compliance

### Code Security
- 🔍 Static analysis with golangci-lint
- 🛡️ Security scanning with gosec
- 🔒 Dependency vulnerability scanning
- 📊 Code coverage reporting

### Supply Chain Security
- 🔐 Container image signing
- 📋 Software Bill of Materials (SBOM)
- 🔒 Reproducible builds
- 📦 Multi-architecture support

## Monitoring and Alerts

### GitHub Actions Notifications
- Failed build notifications
- Security vulnerability alerts
- Dependency update notifications

### Metrics and Reporting
- Test coverage reporting to Codecov
- Security scan results in GitHub Security tab
- Performance benchmark results

## Troubleshooting

### Common Issues

1. **Test failures in PR:**
   - Check the kubecontext safety warnings
   - Ensure Kind cluster is properly configured
   - Verify Go version compatibility

2. **Docker build failures:**
   - Check Dockerfile syntax
   - Verify .dockerignore patterns
   - Ensure static binary compilation

3. **Security scan failures:**
   - Review Trivy vulnerability reports
   - Update dependencies if needed
   - Check container image configuration

### Debug Commands

```bash
# Check workflow status
gh workflow list
gh run list

# Debug specific workflow
gh run view <run-id>

# Local testing
make ci-test
make docker-scan
```

## Best Practices

1. **Pull Requests:**
   - Keep PRs small and focused
   - Write descriptive commit messages
   - Wait for all checks to pass

2. **Testing:**
   - Run `make test-safety` before pushing
   - Test locally with Kind clusters
   - Verify security scans pass

3. **Releases:**
   - Use semantic versioning
   - Include changelog in release notes
   - Test releases in staging environment

4. **Security:**
   - Regularly update dependencies
   - Monitor security alerts
   - Review security scan results

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes and add tests
4. Run `make ci-test` locally
5. Submit a pull request
6. Wait for CI/CD checks to pass
7. Address any review feedback

The CI/CD pipeline will automatically handle testing, building, and deployment once your changes are merged!
