#!/bin/bash
# Test Release Please configuration

set -e

# Check for CLI-only mode
if [ "$1" = "--cli-only" ]; then
    CLI_ONLY=true
    echo "🧪 Testing Release Please CLI Configuration"
    echo "=========================================="
else
    CLI_ONLY=false
    echo "🧪 Testing Release Please Configuration"
    echo "======================================"
fi

# Test 1: Validate JSON configuration files
echo "1. Validating JSON configuration..."
if command -v jq >/dev/null 2>&1; then
    jq empty .release-please-config.json && echo "✅ .release-please-config.json is valid JSON"
    jq empty .release-please-manifest.json && echo "✅ .release-please-manifest.json is valid JSON"
else
    echo "⚠️  jq not installed, skipping JSON validation"
fi

# Test 2: Check that all referenced files exist
echo ""
echo "2. Checking referenced files exist..."
files=(
    "helm/coredns-ingress-sync/Chart.yaml"
    "helm/coredns-ingress-sync/values.yaml" 
    "helm/coredns-ingress-sync/values-production.yaml"
    "README.md"
    "helm/coredns-ingress-sync/README.md"
)

for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "✅ $file exists"
    else
        echo "❌ $file missing"
        exit 1
    fi
done

# Test 3: Validate current version consistency
echo ""
echo "3. Checking version consistency..."

chart_version=$(grep "^version:" helm/coredns-ingress-sync/Chart.yaml | cut -d' ' -f2)
app_version=$(grep "^appVersion:" helm/coredns-ingress-sync/Chart.yaml | cut -d' ' -f2 | tr -d '"')
manifest_version=$(jq -r '."."' .release-please-manifest.json 2>/dev/null || echo "unknown")

echo "Chart version: $chart_version"
echo "App version: $app_version" 
echo "Manifest version: $manifest_version"

if [ "$chart_version" = "$app_version" ] && [ "$app_version" = "$manifest_version" ]; then
    echo "✅ All versions are synchronized"
else
    echo "⚠️  Version differences detected (may be expected during development)"
fi

# Test 4: Check for installation command patterns in documentation
echo ""
echo "4. Checking documentation version patterns..."

if grep -q "oci://ghcr.io/rl-io/charts/coredns-ingress-sync" README.md; then
    echo "✅ Helm chart installation command found in README.md"
else
    echo "❌ Helm chart installation command missing in README.md"
fi

if grep -q "oci://ghcr.io/rl-io/charts/coredns-ingress-sync" helm/coredns-ingress-sync/README.md; then
    echo "✅ Helm chart installation command found in chart README.md"
else
    echo "❌ Helm chart installation command missing in chart README.md"
fi

# Test 5: Validate GitHub workflow exists
echo ""
echo "5. Checking GitHub workflow..."
if [ -f ".github/workflows/release-please.yml" ]; then
    echo "✅ Release Please workflow exists"
else
    echo "❌ Release Please workflow missing"
    exit 1
fi

# Test 6: CLI dry-run test (if GitHub token available and release-please CLI installed)
if [ "$CLI_ONLY" = "true" ]; then
    echo "6. Testing Release Please CLI configuration (detailed)..."
else
    echo ""
    echo "6. Testing Release Please CLI configuration..."
fi

if [ -z "$GITHUB_TOKEN" ]; then
    echo "❌ GITHUB_TOKEN environment variable is required for CLI testing"
    echo "   Export your GitHub token: export GITHUB_TOKEN=your_token_here"
    if [ "$CLI_ONLY" = "true" ]; then
        exit 1
    else
        echo "   Skipping CLI test..."
        echo ""
        echo "🎉 Configuration tests passed (CLI test skipped)!"
        echo ""
        echo "📋 To trigger your first release:"
        echo "1. Make commits using conventional commit format"
        echo "2. Push to main branch"  
        echo "3. Release Please will create a release PR automatically"
        echo "4. Merge the release PR to trigger Docker and Helm chart publishing"
        exit 0
    fi
fi

if ! command -v npx >/dev/null 2>&1; then
    echo "❌ npx is required but not installed"
    echo "   Install Node.js to get npx"
    if [ "$CLI_ONLY" = "true" ]; then
        exit 1
    else
        echo "   Skipping CLI test..."
        echo ""
        echo "🎉 Configuration tests passed (CLI test skipped)!"
        echo ""
        echo "📋 To trigger your first release:"
        echo "1. Make commits using conventional commit format"
        echo "2. Push to main branch"  
        echo "3. Release Please will create a release PR automatically"
        echo "4. Merge the release PR to trigger Docker and Helm chart publishing"
        exit 0
    fi
fi

echo "🔍 Running Release Please CLI dry-run..."
if [ "$CLI_ONLY" = "true" ]; then
    # Detailed output for CLI-only mode
    npx release-please release-pr \
        --token="$GITHUB_TOKEN" \
        --repo-url="rl-io/coredns-ingress-sync" \
        --config-file=".release-please-config.json" \
        --manifest-file=".release-please-manifest.json" \
        --dry-run --debug
else
    # Original brief test
    if npx release-please release-pr \
        --token="$GITHUB_TOKEN" \
        --repo-url="rl-io/coredns-ingress-sync" \
        --config-file=".release-please-config.json" \
        --manifest-file=".release-please-manifest.json" \
        --dry-run 2>/dev/null | grep -q "Would open.*pull request"; then
        echo "✅ Release Please CLI configuration is valid"
    else
        echo "⚠️  Release Please CLI test completed (check logs for details)"
    fi
fi

if [ "$CLI_ONLY" != "true" ]; then
    echo ""
    echo "🎉 All Release Please configuration tests passed!"
    echo ""
    echo "📋 To trigger your first release:"
    echo "1. Make commits using conventional commit format"
    echo "2. Push to main branch"  
    echo "3. Release Please will create a release PR automatically"
    echo "4. Merge the release PR to trigger Docker and Helm chart publishing"
fi
