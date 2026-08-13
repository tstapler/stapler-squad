# Architecture: Permissions Extension (MDD)

**Status**: Implemented | **Phase**: 3 — Implementation
**Project**: permissions-extension-mdd

## Overview

The goal is to decouple the command classification and AST parsing logic from the main `stapler-squad` project to allow it to be shared with a standalone hooks binary and integrated into other CLI tools (Gemini, Open Code).

## Proposed Design Patterns

### 1. Shared Domain Package (`pkg/classifier`)
Extract all classification-related logic into a clean, dependency-light package. This package will be the "Source of Truth" for how commands are parsed and validated.

- **Strategy Pattern**: The `Classifier` interface defines the contract for classification. `RuleBasedClassifier` is the primary implementation, but others (e.g., `MLClassifier` or `ExternalBinaryClassifier`) could be added.
- **Data Transfer Objects (DTOs)**: Use structured types like `PermissionRequestPayload` and `ClassificationResult` to pass data between the host application and the classifier.
- **Recursive AST Walking**: Use `mvdan.cc/sh` to perform deep analysis of compound shell commands (pipes, subshells, logical operators), ensuring that every sub-command is checked against the rules.

### 2. Plugin-like Interface
Define a clear interface for the classifier that can be easily consumed by different entry points.

```go
type Classifier interface {
    // Classify evaluates a tool use request against the current rule set.
    Classify(payload PermissionRequestPayload, ctx ClassificationContext) ClassificationResult
    
    // BuildContext gathers environment information (CWD, Git state) for classification.
    BuildContext(cwd string) ClassificationContext
    
    // LoadRules loads or updates the rules used by the classifier.
    LoadRules(rules []Rule) error
}
```

## Integration Points

### 1. Main Application (`stapler-squad`)
- The `SessionService` will instantiate the `RuleBasedClassifier` from `pkg/classifier`.
- The `ApprovalHandler` (HTTP hook handler) will convert incoming JSON payloads into `PermissionRequestPayload` and call the classifier.
- Rules will be loaded from the existing `RulesStore` (SQLite) and passed to the classifier.

### 2. Standalone Hooks Binary (`ssq-hooks`)
- A lightweight Go binary that imports `pkg/classifier`.
- **CLI Mode**: Can be invoked as `ssq-hooks check "git push"`.
- **Server Mode**: Can run a minimal HTTP server to act as a drop-in replacement for the Claude Code `PermissionRequest` hook.
- **Configuration**: Will read rules from a shared configuration file (e.g., `~/.config/stapler-squad/rules.yaml`) or the main SQLite database.

### 3. Shell Interception (Gemini/Open Code)
- For tools that don't support native hooks, a shell-level interception can be used.
- **Zsh/Bash `preexec`**: A shell function that calls `ssq-hooks check` before executing any command.
- **Alias/Wrapper**: Wrapping the target CLI (e.g., `alias gemini='ssq-hooks wrap gemini'`).

## Tradeoffs

| Feature | Internal Logic (Shared Pkg) | External Logic (Sidecar) |
| :--- | :--- | :--- |
| **Performance** | High (In-process calls) | Lower (IPC/Process overhead) |
| **Consistency** | Guaranteed (Same code) | Risk of version mismatch |
| **Complexity** | Low (Standard Go imports) | High (RPC/IPC protocol) |
| **Portability** | Limited to Go projects | Language agnostic |

**Decision**: Use a **Shared Domain Package** (`pkg/classifier`) for the Go-based components (main app and hooks binary) to ensure maximum performance and consistency.

## Configuration Sharing Strategy

To ensure the standalone binary and the main app share the same rules:

1.  **Primary Store**: The main app's SQLite database remains the source of truth for user-defined rules.
2.  **Export/Sync**: The main app will export rules to `~/.config/stapler-squad/rules.json` whenever they are modified.
3.  **Fallback**: The standalone binary will read from the exported JSON file. If the file is missing, it will use the `SeedRules` (built-in defaults).

## AST Parsing & Performance

- **Overhead**: Parsing every command with `mvdan.cc/sh` adds a few milliseconds of latency. For interactive use, this is negligible (<10ms).
- **Complexity**: Compound commands (e.g., `git add . && git commit -m "feat" | pbcopy`) require recursive walking to ensure no "hidden" dangerous commands are executed.
- **False Positives**: The classifier must be conservative. If a command cannot be parsed or matched, it should default to `Escalate` (manual review).

## Next Steps

1.  Create `pkg/classifier` and move logic from `server/services/classifier.go` and `server/services/command_parser.go`.
2.  Update `stapler-squad` to use the new package.
3.  Implement the `ssq-hooks` binary.
4.  Design the rule export mechanism.
