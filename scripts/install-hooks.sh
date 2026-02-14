#!/bin/bash
# Install git hooks for ekaya-engine project

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$PROJECT_ROOT/.githooks"
GIT_HOOKS_DIR="$PROJECT_ROOT/.git/hooks"

echo "🔧 Installing git hooks for ekaya-engine..."

# Check if we're in a git repository
if [ ! -d "$PROJECT_ROOT/.git" ]; then
    echo "❌ Error: Not in a git repository"
    exit 1
fi

# Check if .githooks directory exists
if [ ! -d "$HOOKS_DIR" ]; then
    echo "❌ Error: .githooks directory not found"
    exit 1
fi

# Create hooks directory if it doesn't exist
mkdir -p "$GIT_HOOKS_DIR"

# Install each hook
for hook in pre-commit pre-push; do
    if [ -f "$HOOKS_DIR/$hook" ]; then
        echo "📝 Installing $hook hook..."
        cp "$HOOKS_DIR/$hook" "$GIT_HOOKS_DIR/$hook"
        chmod +x "$GIT_HOOKS_DIR/$hook"
        echo "✅ $hook hook installed"
    fi
done

echo ""
echo "✅ Git hooks installed successfully!"
echo ""
echo "The following hooks are now active:"
echo "  • pre-commit: Runs 'make check' (formatting, linting, all tests) - takes ~30-60s"
echo "  • pre-push: Runs comprehensive checks before pushing (stricter for main/prod)"
echo ""
echo "Benefits:"
echo "  ✅ Catches issues before they're committed"
echo "  ✅ Prevents broken code from reaching code review"
echo "  ✅ Ensures all tests pass (including OAuth auth tests)"
echo ""
echo "To skip hooks temporarily (not recommended), use --no-verify flag:"
echo "  git commit --no-verify"
echo "  git push --no-verify"
echo ""
echo "For more info, see docs/git-hooks.md"

# Handle uninstall
if [ "$1" == "--uninstall" ]; then
    echo "🗑️  Uninstalling git hooks..."
    for hook in pre-commit pre-push; do
        if [ -f "$GIT_HOOKS_DIR/$hook" ]; then
            rm "$GIT_HOOKS_DIR/$hook"
            echo "✅ Removed $hook hook"
        fi
    done
    echo "✅ Git hooks uninstalled"
fi