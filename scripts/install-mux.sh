#!/bin/sh
# install-mux.sh - Install claude-mux PTY multiplexer
#
# This script builds and installs claude-mux, enabling seamless
# Claude session monitoring from external terminals (IntelliJ, VS Code, etc.)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY_NAME="claude-mux"
BUILD_CMD="go build -o $BINARY_NAME ./cmd/claude-mux"

# Helper functions
log_info() {
    printf "${BLUE}==>${NC} %s\n" "$1"
}

log_success() {
    printf "${GREEN}✓${NC} %s\n" "$1"
}

log_warning() {
    printf "${YELLOW}!${NC} %s\n" "$1"
}

log_error() {
    printf "${RED}✗${NC} %s\n" "$1"
}

# Check if running from project root
check_project_root() {
    if [ ! -f "go.mod" ] || [ ! -d "cmd/claude-mux" ]; then
        log_error "Must run from stapler-squad project root directory"
        log_info "Current directory: $(pwd)"
        exit 1
    fi
}

# Build claude-mux
build_mux() {
    log_info "Building claude-mux..."
    if ! $BUILD_CMD; then
        log_error "Build failed"
        exit 1
    fi
    log_success "Build completed"
}

# Install binary
install_binary() {
    log_info "Installing to $INSTALL_DIR..."

    # Create directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        log_info "Creating directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi

    # Install binary
    if ! mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"; then
        log_error "Failed to install binary"
        exit 1
    fi

    # Make executable
    chmod +x "$INSTALL_DIR/$BINARY_NAME"

    log_success "Installed to $INSTALL_DIR/$BINARY_NAME"
}

# Check if directory is in PATH
check_path() {
    if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
        log_warning "$INSTALL_DIR is not in your PATH"
        echo ""
        echo "Add this line to your shell configuration (~/.bashrc, ~/.zshrc, etc.):"
        echo ""
        echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
        echo ""
        return 1
    fi
    return 0
}

# Print shell alias setup instructions
print_alias_setup() {
    log_info "Shell Alias Setup (Optional but Recommended)"
    echo ""
    echo "To automatically wrap Claude commands, add this alias to your shell config:"
    echo ""
    echo "  ${GREEN}alias claude='claude-mux claude'${NC}"
    echo ""
    echo "Add to:"
    echo "  - Bash: ~/.bashrc"
    echo "  - Zsh: ~/.zshrc"
    echo "  - Fish: ~/.config/fish/config.fish (use: alias claude 'claude-mux claude')"
    echo ""
    echo "After adding the alias, run: ${BLUE}source ~/.zshrc${NC} (or your shell config)"
    echo ""
}

# Print IDE setup instructions
print_ide_setup() {
    log_info "IDE Terminal Configuration"
    echo ""
    echo "${YELLOW}IntelliJ IDEA / PyCharm / WebStorm:${NC}"
    echo "  1. Open Settings → Tools → Terminal"
    echo "  2. Set 'Shell path' to: ${GREEN}$INSTALL_DIR/claude-mux${NC}"
    echo "  3. Set 'Shell arguments' to: ${GREEN}claude${NC}"
    echo "  4. Restart IDE terminal"
    echo ""
    echo "${YELLOW}VS Code:${NC}"
    echo "  1. Open Settings (Cmd+, or Ctrl+,)"
    echo "  2. Search for 'terminal.integrated.shell'"
    echo "  3. For your OS, set:"
    echo "     \"terminal.integrated.profiles.osx\": {"
    echo "       \"claude-mux\": {"
    echo "         \"path\": \"$INSTALL_DIR/claude-mux\","
    echo "         \"args\": [\"claude\"]"
    echo "       }"
    echo "     }"
    echo "  4. Set as default profile"
    echo ""
}

# Print verification instructions
print_verification() {
    log_info "Verification Steps"
    echo ""
    echo "1. ${BLUE}Check installation:${NC}"
    echo "     which claude-mux"
    echo "     Expected: $INSTALL_DIR/claude-mux"
    echo ""
    echo "2. ${BLUE}Test wrapper:${NC}"
    echo "     claude-mux echo \"Hello from claude-mux\""
    echo "     Expected: Hello message + socket created at /tmp/claude-mux-*.sock"
    echo ""
    echo "3. ${BLUE}Run Claude (if installed):${NC}"
    echo "     claude-mux claude"
    echo "     Expected: Claude session starts + discoverable by claude-squad"
    echo ""
    echo "4. ${BLUE}Check discovery:${NC}"
    echo "     ls /tmp/claude-mux-*.sock"
    echo "     Expected: Socket files for active sessions"
    echo ""
}

# Print troubleshooting section
print_troubleshooting() {
    log_info "Troubleshooting"
    echo ""
    echo "${YELLOW}Issue: 'claude-mux: command not found'${NC}"
    echo "  Solution: Ensure $INSTALL_DIR is in your PATH (see above)"
    echo ""
    echo "${YELLOW}Issue: 'stdin is not a terminal'${NC}"
    echo "  Solution: claude-mux requires a TTY. Use from terminal, not scripts."
    echo ""
    echo "${YELLOW}Issue: Sessions not discovered by claude-squad${NC}"
    echo "  1. Check socket exists: ls /tmp/claude-mux-*.sock"
    echo "  2. Verify permissions: ls -l /tmp/claude-mux-*.sock"
    echo "  3. Enable discovery in claude-squad (auto-enabled for external sessions)"
    echo ""
    echo "${YELLOW}Issue: Multiple sessions conflict${NC}"
    echo "  Solution: Each session gets unique socket. Use 'ps aux | grep claude-mux' to see all."
    echo ""
}

# Print success summary
print_success_summary() {
    echo ""
    echo "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    log_success "claude-mux installed successfully!"
    echo "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Add $INSTALL_DIR to your PATH (if not already)"
    echo "  2. Add shell alias (recommended): alias claude='claude-mux claude'"
    echo "  3. Configure your IDE terminal (see instructions above)"
    echo "  4. Verify installation with: which claude-mux"
    echo ""
    echo "Documentation: docs/external-sessions.md"
    echo "Help: claude-mux --help"
    echo ""
}

# Main installation flow
main() {
    echo ""
    log_info "Claude-Mux Installation"
    echo ""

    # Check prerequisites
    check_project_root

    # Build
    build_mux

    # Install
    install_binary

    # Check PATH
    path_ok=0
    check_path || path_ok=$?

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Print setup instructions
    print_alias_setup
    print_ide_setup
    print_verification
    print_troubleshooting

    # Success
    print_success_summary

    exit 0
}

# Run main
main "$@"
