# Release Management with Release Please

This project uses [Release Please](https://github.com/googleapis/release-please) to automate version management and releases.

## How it Works

Release Please uses [Conventional Commits](https://www.conventionalcommits.org/) to determine version bumps and automatically:

- 📈 **Bumps versions** in Chart.yaml, values files, and documentation
- 📝 **Generates CHANGELOG.md** based on commit messages
- 🏷️ **Creates Git tags** and GitHub releases
- 🚀 **Triggers CI/CD** to build and push Docker images and Helm charts

## Commit Message Format

Use this format for your commit messages:

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Commit Types

- **`feat:`** New feature → **Minor version bump** (0.1.0 → 0.2.0)
- **`fix:`** Bug fix → **Patch version bump** (0.1.0 → 0.1.1)  
- **`docs:`** Documentation only → **Patch version bump**
- **`style:`** Code style changes → **Patch version bump**
- **`refactor:`** Code refactoring → **Patch version bump**
- **`test:`** Adding tests → **Patch version bump**
- **`chore:`** Maintenance tasks → **Patch version bump**

### Breaking Changes

Add `BREAKING CHANGE:` in the footer or `!` after type → **Major version bump** (0.1.0 → 1.0.0)

```bash
feat!: redesign configuration API

BREAKING CHANGE: The configuration format has changed from environment variables to structured Helm values.
```

## Example Commits

```bash
# Patch release (0.1.0 → 0.1.1)
git commit -m "fix: resolve CoreDNS volume mount race condition"

# Minor release (0.1.0 → 0.2.0)  
git commit -m "feat: add support for multiple ingress classes"

# Major release (0.1.0 → 1.0.0)
git commit -m "feat!: migrate to v2 API structure

BREAKING CHANGE: Configuration moved from environment variables to Helm values structure."

# Documentation update (patch)
git commit -m "docs: update installation instructions for v0.2.0"
```

## Release Process

1. **Create PR** with conventional commits
2. **Merge to main** branch
3. **Release Please bot** automatically:
   - Creates a release PR with version bumps
   - Updates `Chart.yaml`, values files, and documentation
   - Generates changelog entries
4. **Merge the release PR** to trigger:
   - Docker image build and push
   - Helm chart package and push
   - GitHub release creation

## Version Synchronization

Release Please keeps these versions in sync:

- `helm/coredns-ingress-sync/Chart.yaml` → `version` and `appVersion`
- `helm/coredns-ingress-sync/values.yaml` → `image.tag`
- `helm/coredns-ingress-sync/values-production.yaml` → `image.tag`
- `README.md` → Helm installation commands
- `helm/coredns-ingress-sync/README.md` → Helm installation commands

## Manual Release (if needed)

```bash
# Trigger release workflow manually
gh workflow run release-please.yml

# Or create a release manually with proper version tags
git tag v0.2.0
git push origin v0.2.0
```

## Files Managed by Release Please

- `.release-please-config.json` - Configuration
- `.release-please-manifest.json` - Current versions
- `CHANGELOG.md` - Auto-generated changelog
- All version references in documentation and values files
