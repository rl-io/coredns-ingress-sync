#!/bin/bash
# Conventional commit helper script

set -e

echo "🚀 Conventional Commit Helper"
echo "=============================="

# Get commit type
echo ""
echo "Select commit type:"
echo "1) feat     - New feature (minor version bump)"
echo "2) fix      - Bug fix (patch version bump)" 
echo "3) docs     - Documentation only"
echo "4) style    - Code style changes"
echo "5) refactor - Code refactoring"
echo "6) test     - Adding tests"
echo "7) chore    - Maintenance tasks"
echo "8) feat!    - Breaking change (major version bump)"
echo "9) fix!     - Breaking fix (major version bump)"

read -p "Enter choice (1-9): " choice

case $choice in
    1) type="feat" ;;
    2) type="fix" ;;
    3) type="docs" ;;
    4) type="style" ;;
    5) type="refactor" ;;
    6) type="test" ;;
    7) type="chore" ;;
    8) type="feat!" ;;
    9) type="fix!" ;;
    *) echo "Invalid choice"; exit 1 ;;
esac

# Get scope (optional)
read -p "Enter scope (optional, e.g., 'helm', 'controller', 'docs'): " scope
if [ -n "$scope" ]; then
    scope="($scope)"
fi

# Get description
read -p "Enter commit description: " description

# Check for breaking change details
breaking_change=""
if [[ "$type" == *"!" ]]; then
    echo ""
    echo "⚠️  This is a breaking change!"
    read -p "Enter breaking change description: " breaking_desc
    breaking_change=$'\n\nBREAKING CHANGE: '"$breaking_desc"
fi

# Get optional body
echo ""
read -p "Enter optional commit body (press Enter to skip): " body
if [ -n "$body" ]; then
    body=$'\n\n'"$body"
fi

# Construct commit message
commit_msg="${type}${scope}: ${description}${body}${breaking_change}"

# Show preview
echo ""
echo "📝 Commit message preview:"
echo "=========================="
echo "$commit_msg"
echo "=========================="

# Confirm and commit
echo ""
read -p "Commit with this message? (y/N): " confirm
if [[ $confirm == [yY] || $confirm == [yY][eE][sS] ]]; then
    git add .
    git commit -m "$commit_msg"
    echo "✅ Committed successfully!"
    
    # Show next steps
    echo ""
    echo "📋 Next steps:"
    echo "1. Push to your branch: git push origin $(git branch --show-current)"
    echo "2. Create a PR to main branch"
    echo "3. Once merged, Release Please will handle versioning automatically"
else
    echo "❌ Commit cancelled"
fi
