# Contributing to coredns-ingress-sync

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Docker
- Kubernetes cluster (kind, minikube, or cloud provider)
- kubectl configured
- Helm 3.x

### Development Setup

1. **Fork and clone the repository**:

   ```bash
   git clone https://github.com/your-username/coredns-ingress-sync.git
   cd coredns-ingress-sync
   ```

2. **Setup development environment**:

   ```bash
   # Install git hooks for conventional commits
   ./scripts/setup-git-hooks.sh
   
   # Optional: Install commitlint for better commit experience
   make install-commitlint
   
   # Install dependencies
   go mod download
   
   # Run tests to verify setup
   ./tests/run_tests.sh
   ```

## Commit Message Guidelines

This project follows [Conventional Commits](https://www.conventionalcommits.org/) specification.

### Format

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Valid Types

- **feat**: A new feature
- **fix**: A bug fix
- **docs**: Documentation only changes
- **style**: Changes that do not affect the meaning of the code
- **refactor**: A code change that neither fixes a bug nor adds a feature
- **perf**: A code change that improves performance
- **test**: Adding missing tests or correcting existing tests
- **build**: Changes that affect the build system or external dependencies
- **ci**: Changes to CI configuration files and scripts
- **chore**: Other changes that don't modify src or test files
- **revert**: Reverts a previous commit

### Examples

```text
feat: add support for custom ingress classes
fix: resolve ConfigMap update race condition
docs: update installation instructions
feat(helm): add production values template
fix!: change API response format (breaking change)
```

### Validation

Commit messages are automatically validated by:

- **Git hooks**: Local validation when committing
- **GitHub Actions**: CI validation on pull requests

## Pull Request Process

### 1. Create Feature Branch

```bash
git checkout -b feat/your-feature-name
# or
git checkout -b fix/issue-description
```

### 2. Make Changes

- Write code following our coding standards
- Add tests for new functionality
- Update documentation as needed
- Ensure all tests pass

### 3. Test Your Changes

```bash
# Run full test suite
./tests/run_tests.sh

# Run specific test types
./tests/run_tests.sh --integration
./tests/run_tests.sh --e2e

# Test locally with kind/minikube
helm install test-release ./helm/coredns-ingress-sync \
  --namespace test-ns --create-namespace
```

### 4. Update Documentation

- Update relevant documentation in `docs/`
- Add examples if introducing new features
- Update the README if necessary

### 5. Commit Changes

```bash
# Make sure to follow conventional commits
git add .
git commit -m "feat: add your feature description"
```

### 6. Push and Create PR

```bash
git push origin feat/your-feature-name
```

Then create a pull request through GitHub.

## Code Standards

### Go Code Guidelines

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Add comments for exported functions and types
- Write tests for new functionality

### Helm Chart Guidelines

- Follow [Helm Best Practices](https://helm.sh/docs/chart_best_practices/)
- Update `Chart.yaml` version for chart changes
- Test with multiple Kubernetes versions
- Document new values in `values.yaml` comments

### Documentation Guidelines

- Write clear, concise documentation
- Include examples for complex configurations
- Update the documentation index if adding new guides
- Test documentation examples

## Testing

### Test Types

- **Unit Tests**: `go test -v .`
- **Integration Tests**: `./tests/integration_test.sh`
- **End-to-End Tests**: `./tests/e2e_test.sh`
- **Performance Tests**: `./tests/benchmark_test.sh`

### Test Requirements

- All new features must include tests
- Maintain or improve test coverage
- Tests must pass in CI/CD pipeline
- Use the test safety checks for cluster validation

## Security

### Reporting Security Issues

Please report security vulnerabilities by emailing [security contact]. Do not create public GitHub issues for security problems.

### Security Guidelines

- Follow principle of least privilege for RBAC
- Validate all user inputs
- Use secure defaults in configuration
- Document security implications of features

## Release Process

### Versioning

We use [Semantic Versioning](https://semver.org/):

- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible functionality additions
- **PATCH**: Backwards-compatible bug fixes

### Release Notes

Release notes are automatically generated from conventional commits:

- `feat:` commits become "Features"
- `fix:` commits become "Bug Fixes"
- `perf:` commits become "Performance Improvements"
- Breaking changes are highlighted

## Getting Help

### Questions and Discussions

- **General questions**: [GitHub Issues](https://github.com/rl-io/coredns-ingress-sync/issues) (use the "question" label)
- **Bug reports**: [GitHub Issues](https://github.com/rl-io/coredns-ingress-sync/issues)
- **Feature requests**: [GitHub Issues](https://github.com/rl-io/coredns-ingress-sync/issues)

### Documentation

- **Architecture**: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- **Configuration**: [docs/CONFIGURATION.md](docs/CONFIGURATION.md)
- **Development**: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)
- **Troubleshooting**: [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you are expected to uphold this code.

## License

By contributing to this project, you agree that your contributions will be licensed under the MIT License.

## Recognition

Contributors are recognized in:

- Release notes for their contributions
- The project's contributors list
- Special recognition for significant contributions

Thank you for contributing to coredns-ingress-sync! 🎉
