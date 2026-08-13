#!/usr/bin/env bash
#
# install-gemini-hook.sh - Install Stapler Squad hook for Gemini CLI
#
# This script configures Gemini CLI to use Stapler Squad permissions check
# before executing tools.
#
# Usage:
#   install-gemini-hook.sh
#

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    echo -e "${BLUE}→${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

echo "Stapler Squad Gemini Hook Installer"
echo "===================================="
echo ""

HOOK_CMD='echo "$TOOL_INPUT" | ssq-hooks check --db ~/.config/stapler-squad/stapler-squad.db'

info "To enable Stapler Squad permissions check in Gemini CLI, add the following"
info "to your Gemini configuration (e.g., ~/.gemini/config.json or project .gemini.json):"
echo ""
echo "{"
echo "  \"hooks\": {"
echo "    \"BeforeTool\": \"$HOOK_CMD\""
echo "  }"
echo "}"
echo ""

# Attempt to find gemini config
CONFIG_FILES=(".gemini.json" "$HOME/.gemini/config.json" "$HOME/.gemini/settings.json")
FOUND=false

for f in "${CONFIG_FILES[@]}"; do
    if [[ -f "$f" ]]; then
        info "Found Gemini configuration at: $f"
        info "You can update it using jq (example):"
        echo "  jq '.hooks.BeforeTool = \"$HOOK_CMD\"' \"$f\" > \"${f}.tmp\" && mv \"${f}.tmp\" \"$f\""
        FOUND=true
    fi
done

if [[ "$FOUND" == "false" ]]; then
    info "No Gemini configuration file found. Please create one if needed."
fi

echo ""
success "Instructions generated."
