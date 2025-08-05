# Security and Dependency Management

This project uses several automated tools to maintain security and keep dependencies up to date.

## 🛡️ Security Features

### Dependabot

- **Automated dependency updates** across all ecosystems (Go, Node.js, Docker, GitHub Actions)
- **Security-focused**: Prioritizes security patches over feature updates
- **Smart grouping**: Minor and patch updates are grouped to reduce PR noise
- **Conservative approach**: Major version updates require manual review

### Trivy Security Scanning

- **Container image scanning** for vulnerabilities
- **Filesystem scanning** for secrets and misconfigurations
- **SARIF reporting** integrated with GitHub Security tab
- **Automated scanning** on every push and PR

### Signed Container Images

- **Keyless signing** with Cosign and Sigstore
- **Verification available** for all published images
- **Supply chain security** through signed artifacts

## 📋 Setup Checklist

### Required GitHub Secrets

- [ ] `CODECOV_TOKEN` - For authenticated coverage uploads (optional but recommended)

### Recommended GitHub Settings

- [ ] Enable Dependabot security updates in repository settings
- [ ] Enable vulnerability alerts
- [ ] Configure branch protection rules
- [ ] Review security advisories regularly

## 🔧 Dependabot Configuration

### Update Schedule

- **Go modules**: Weekly on Monday 09:00 UTC
- **Node.js packages**: Weekly on Monday 09:30 UTC
- **Docker images**: Weekly on Tuesday 09:00 UTC
- **GitHub Actions**: Weekly on Tuesday 09:30 UTC

### Update Strategy

- **Security updates**: Always applied automatically
- **Minor/patch updates**: Grouped and applied weekly
- **Major updates**: Require manual review and approval

### Conventional Commits

All Dependabot PRs follow conventional commit standards:

- `chore(deps): update golang.org/x/crypto to v0.15.0`
- `chore(ci): bump actions/checkout from 3 to 4`
- `chore(docker): update golang docker tag to v1.21`

## 🚀 Using the Security Features

### Viewing Security Reports

1. Go to repository → Security tab
2. View vulnerability alerts and advisories
3. Check code scanning results from Trivy
4. Review dependency updates from Dependabot

### Verifying Container Images

```bash
# Verify image signature (requires cosign)
cosign verify ghcr.io/rl-io/coredns-ingress-sync:latest

# Check for vulnerabilities
trivy image ghcr.io/rl-io/coredns-ingress-sync:latest
```

### Manual Security Updates

```bash
# Update Go dependencies
go get -u ./...
go mod tidy

# Update Node.js dependencies
npm audit fix
npm update

# Rebuild and test
make ci-test
```

## 📚 Documentation

- [Dependabot Documentation](https://docs.github.com/en/code-security/dependabot)
- [Trivy Scanner](https://trivy.dev/)
- [Cosign Signatures](https://docs.sigstore.dev/cosign/signing/signing_with_containers/)
- [GitHub Security Features](https://docs.github.com/en/code-security)
