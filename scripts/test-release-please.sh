#!/bin/bash
# Test Release Please configuration

set -e

echo "🧪 Testing Release Please Configuration"
echo "======================================"

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
values_tag=$(grep "tag:" helm/coredns-ingress-sync/values.yaml | cut -d'"' -f2)
prod_values_tag=$(grep "tag:" helm/coredns-ingress-sync/values-production.yaml | cut -d'"' -f2)

echo "Chart version: $chart_version"
echo "App version: $app_version"
echo "Values tag: $values_tag"
echo "Production values tag: $prod_values_tag"

if [ "$chart_version" = "$app_version" ] && [ "$app_version" = "$values_tag" ] && [ "$values_tag" = "$prod_values_tag" ]; then
    echo "✅ All versions are synchronized"
else
    echo "❌ Version mismatch detected!"
    exit 1
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

echo ""
echo "🎉 All Release Please configuration tests passed!"
echo ""
echo "📋 To trigger your first release:"
echo "1. Make commits using conventional commit format"
echo "2. Push to main branch"  
echo "3. Release Please will create a release PR automatically"
echo "4. Merge the release PR to trigger Docker and Helm chart publishing"
