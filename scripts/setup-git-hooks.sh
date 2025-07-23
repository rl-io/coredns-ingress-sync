#!/bin/bash

# Setup script to configure git hooks for conventional commits

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Setting up conventional commits validation...${NC}"

# Get the project root directory
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Check if we're in a git repository
if [ ! -d "$PROJECT_ROOT/.git" ]; then
    echo "Error: Not in a git repository"
    exit 1
fi

echo -e "${YELLOW}Configuring git hooks directory...${NC}"
cd "$PROJECT_ROOT"
git config core.hooksPath .githooks

# Make sure hooks are executable
chmod +x .githooks/*

echo -e "${GREEN}✓ Git hooks configured successfully!${NC}"
echo
echo -e "${YELLOW}What this does:${NC}"
echo "• Validates commit messages follow conventional commit format"
echo "• Requires format: type(scope): description"
echo "• Valid types: feat, fix, docs, style, refactor, test, chore, perf, ci, build"
echo ""
echo -e "${YELLOW}Examples of valid commits:${NC}"
echo "• feat: add user authentication"
echo "• fix(api): resolve timeout issues"
echo "• docs: update installation guide"
echo "• chore(ci): update workflow"
echo ""
echo -e "${YELLOW}To test the hook:${NC}"
echo "• git commit -m 'invalid message'  # Should fail"
echo "• git commit -m 'feat: add something'  # Should pass"
echo ""
echo -e "${YELLOW}To bypass validation (not recommended):${NC}"
echo "• git commit --no-verify -m 'message'"
echo ""
echo -e "${GREEN}✓ Setup complete! Your commits will now be validated.${NC}"
