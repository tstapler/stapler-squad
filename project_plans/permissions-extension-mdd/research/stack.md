# Research: Stack — Permissions Extension (MDD)

**Status**: Completed | **Phase**: 2 — Research
**Created**: 2026-04-03

## 1. Integration Points for Gemini and Open Code

### Gemini CLI Hooks
Gemini provides a native `hooks` system configured in `settings.json`. The most relevant integration point is the **`BeforeTool`** hook with a matcher for **`run_shell_command`**. This allows the permissions classifier to intercept, analyze, and potentially block or modify shell commands before the agent executes them.

### Open Code / General CLI
For CLIs without native hook systems, the **Proxy Pattern** (similar to the existing `rtk` tool) is the most effective. This involves aliasing the target CLI to a wrapper binary that performs the classification before passing the command to the actual CLI.

### Shell Aliases
While useful for launching the CLI with specific flags, they are insufficient for deep command classification *inside* the agent's loop. Native hooks or proxies are required for "pre-exec" logic within the AI session.

## 2. Binary Delivery Methods

### `go install`
- **Pros**: Easy for developers, ensures the binary is built for the local environment.
- **Cons**: Requires the Go toolchain to be installed on the user's machine.

### Pre-built Binaries (Recommended)
- **Pros**: No dependencies, faster installation, and better user experience for non-Go users.
- **Implementation**: Leverage the existing `.goreleaser.yaml` in the repository to automate the creation of cross-platform binaries (Linux, macOS, Windows).
- **Recommendation**: Provide a simple `curl | sh` installation script that fetches the appropriate pre-built binary for the user's system.

## 3. Technical Stack Recommendation

- **Language**: Continue using **Go** to maintain full reuse of the existing domain logic (e.g., `cmd/permissions.go`) and AST parsing capabilities.
- **Architecture**: Design the classifier as a standalone, lightweight binary that can be invoked both as a Gemini hook and as a standalone CLI proxy.
