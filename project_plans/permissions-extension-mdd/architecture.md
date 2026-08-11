# Architecture: Permissions Extension (MDD)

**Status**: Draft | **Phase**: 3 — Architecture complete
**Created**: 2026-04-03

## Overview

The Permissions Extension project aims to modularize the command classification and AST parsing logic currently residing in `stapler-squad`'s server services. This logic will be moved to a shared package, `pkg/classifier`, and exposed via a standalone binary, `ssq-hooks`, to support multiple LLM CLIs (Claude, Gemini, Open Code) and independent execution.

## Component Design

### 1. Shared Package: `pkg/classifier`
This package will be the core domain for command analysis. It must have zero dependencies on `server` or `session` packages to ensure portability.

- **Data Models**: `ClassificationResult`, `RiskLevel`, `ParsedCommand`, `CommandCriteria`.
- **Parser**: `ExtractAllCommands` (using `mvdan.cc/sh`) to decompose complex shell strings into structured `ParsedCommand` objects.
- **Engine**: `RuleBasedClassifier` that evaluates `PermissionRequestPayload` against a set of rules.
- **Payload**: `PermissionRequestPayload` (standardized across hook types).

### 2. Standalone Binary: `ssq-hooks`
A lightweight Go CLI that acts as the entry point for non-`stapler-squad` integrations.

- **`ssq-hooks check`**:
    - Input: JSON payload via stdin or flags.
    - Logic: Invokes `pkg/classifier`.
    - Output: JSON decision (`allow`, `deny`, `escalate`).
    - Usage: Gemini `BeforeTool` hook, pre-exec shell hooks.
- **`ssq-hooks serve`**:
    - Purpose: Runs a lightweight HTTP server for Claude Code's HTTP hook system.
    - Benefit: Faster than spawning a process for every request.
- **`ssq-hooks proxy`**:
    - Usage: `alias open-code='ssq-hooks proxy -- open-code'`.
    - Logic: Intercepts the command, classifies it, and either executes the target or blocks it.

### 3. Integration Strategies

#### Claude Code
- Continues to use the HTTP hook system.
- Can point to either the main `stapler-squad` server or the standalone `ssq-hooks serve`.

#### Gemini
- **Native Hook**: Configure Gemini to use a shell command hook:
  `hooks.BeforeTool = "ssq-hooks check --tool bash --input-json $TOOL_INPUT"`
- **Response Handling**: Map `ssq-hooks` output to Gemini's expected return values to block/allow tool execution.

#### Open Code / General CLI
- **Wrapper Pattern**: Use the `ssq-hooks proxy` command.
- **Mechanism**: The proxy analyzes the proposed command before passing it through to the underlying CLI.

## Data Flow & Configuration

1. **Rule Management**: Rules are authored in the `stapler-squad` UI and stored in SQLite.
2. **Export**: The main app exports rules to `~/.config/stapler-squad/rules.json` whenever they change.
3. **Consumption**: `ssq-hooks` reads `rules.json` to perform its classification.
4. **Standalone Usage**: For users without `stapler-squad`, `rules.json` can be managed manually or via a separate config command.

## Infrastructure Changes

- Move `server/services/classifier.go` -> `pkg/classifier/classifier.go`.
- Move `server/services/command_parser.go` -> `pkg/classifier/parser.go`.
- Decouple `RuleBasedClassifier` from `AnalyticsStore` and `ReviewQueue` (use interfaces or callbacks).
- Update `ApprovalHandler` in `server/services/` to use the new `pkg/classifier` package.

## Implementation Roadmap

1. **Sprint 1: Refactor & Shared Package**
    - Create `pkg/classifier`.
    - Migrate and clean up core classification logic.
    - Ensure 100% test coverage for the new package.

2. **Sprint 2: Standalone CLI**
    - Implement `cmd/ssq-hooks`.
    - Add `check` and `serve` commands.
    - Implement the rule export mechanism in the main app.

3. **Sprint 3: Multi-CLI Integration**
    - Create installation scripts for Gemini and Open Code wrappers.
    - Document hook configurations for each platform.
    - Perform deep security analysis expansion (AST parsing enhancements).
