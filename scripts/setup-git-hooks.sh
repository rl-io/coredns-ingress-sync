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

# Check for Node.js and offer to install commitlint
if command -v npm >/dev/null 2>&1; then
    echo -e "${YELLOW}Node.js detected. Do you want to install commitlint for better validation? (y/n)${NC}"
    read -r response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        echo -e "${BLUE}Installing commitlint...${NC}"
        npm install
        echo -e "${GREEN}✓ Commitlint installed!${NC}"
        echo -e "${YELLOW}You can now use: npm run commit (interactive) or git commit (with validation)${NC}"
    fi
else
    echo -e "${YELLOW}Node.js not found. Using custom validation script as fallback.${NC}"
    echo -e "${YELLOW}For better experience, install Node.js and run: npm install${NC}"
fi

echo
echo -e "${YELLOW}What this does:${NC}"
echo "  • Validates commit messages against Conventional Commits format"
echo "  • Uses commitlint if available, custom validator as fallback"
echo "  • Provides helpful error messages and examples"
echo "  • Allows merge commits and reverts to pass through"
echo
echo -e "${YELLOW}Valid commit formats:${NC}"
echo "  feat: add new feature"
echo "  fix: resolve bug"
echo "  docs: update documentation"
echo "  feat(scope): add scoped feature"
echo "  fix!: breaking change fix"
echo
echo -e "${YELLOW}Interactive commits (if commitlint installed):${NC}"
echo "  npm run commit  # Interactive commit builder"
echo
echo -e "${YELLOW}To test the hook:${NC}"
echo "  git commit -m 'invalid message'  # Should fail"
echo "  git commit -m 'feat: add something'  # Should pass"
echo
echo -e "${YELLOW}To bypass validation (not recommended):${NC}"
echo "  git commit --no-verify -m 'message'"
