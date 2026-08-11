# Implementation Plan: Permissions Extension (MDD)

Modularize the command classification logic and provide standalone hooks for Gemini, Open Code, and Claude.

---

## Epic Overview

**User Value**: Enable consistent, secure, and automated permission checks across multiple AI coding agents (Gemini, Open Code, Claude) using the same proven classification engine. This reduces context-switching and provides a unified security layer for all LLM-driven command executions.

**Success Metrics**:
- Core classifier logic extracted to `pkg/classifier` with 100% test parity.
- Standalone `ssq-hooks` binary correctly intercepts and classifies `gemini` and `open-code` calls.
- Permissions are synced with `stapler-squad` rules when available, or fall back to safe defaults.
- Latency added by the hook is < 50ms for classification.

**Scope**:
- Refactor `stapler-squad` to use the new `pkg/classifier`.
- Implement `ssq-hooks` binary in Go.
- Create installation scripts for prefixing `$PATH` and setting up wrappers.
- Support `gemini` and `open-code` as primary targets.

**Constraints**:
- Must be compatible with Linux (primary target) and macOS.
- Must not require a running `stapler-squad` server to function (must fall back).
- Must avoid infinite recursion when calling the real underlying tool.

---

## Architecture Decisions

<adr_reference number="001">
    <file>project_plans/permissions-extension-mdd/decisions/ADR-001-package-structure-reusable-classifier.md</file>
    <summary>Extract classifier logic into a new top-level `pkg/classifier` to enable reuse between the server and standalone binaries.</summary>
</adr_reference>

<adr_reference number="002">
    <file>project_plans/permissions-extension-mdd/decisions/ADR-002-hook-interception-strategy-wrapper.md</file>
    <summary>Use a wrapper binary approach by prefixing the user's `$PATH` with a directory containing `ssq-hooks` symlinks.</summary>
</adr_reference>

---

## Story Breakdown

### Story 1: Core Package Refactoring [1 week]
**User Value**: Foundation for reuse. Ensures that improvements to the classifier benefit all tools simultaneously.

**Acceptance Criteria**:
- `pkg/classifier` contains all logic previously in `server/services/classifier.go` and `server/services/command_parser.go`.
- `stapler-squad` server passes all tests using the new package.
- No dependencies on `server/` packages within `pkg/classifier`.

#### Tasks:
1.1 **Task: Initialize pkg/classifier and Move Logic [2h]**
- **Objective**: Relocate the classifier and command parser to the new package.
- **Context Boundary**: `pkg/classifier/classifier.go`, `pkg/classifier/command_parser.go`, `server/services/classifier.go`.
- **Implementation**: Move files, update package names, and fix imports in `stapler-squad`.
- **Validation**: `go build ./pkg/classifier/...` passes.

1.2 **Task: Refactor Server Services to Use pkg/classifier [2h]**
- **Objective**: Update the server to use the extracted logic.
- **Context Boundary**: `server/services/approval_handler.go`, `server/services/session_service.go`, `server/services/rules_store.go`.
- **Implementation**: Replace internal calls with `pkg/classifier` calls. Ensure analytics and rules remain correctly wired.
- **Validation**: `go test ./server/services/...` passes.

---

### Story 2: Standalone Hooks Binary (`ssq-hooks`) [1 week]
**User Value**: Allows users to run `gemini` or `open-code` with the same safety guarantees as `stapler-squad`.

**Acceptance Criteria**:
- `ssq-hooks` binary can classify a command provided in `argv`.
- Correctly identifies the target tool based on its own filename (e.g., when symlinked as `gemini`).
- Communicates with a running `stapler-squad` server if available, otherwise uses local rules.

#### Tasks:
2.1 **Task: Implement ssq-hooks Main Entry Point [3h]**
- **Objective**: Create the base binary structure and argv parsing.
- **Context Boundary**: `cmd/ssq-hooks/main.go`, `pkg/classifier/classifier.go`.
- **Implementation**: Parse `argv[0]` to determine target tool. Load rules from `~/.stapler-squad/auto_approve_rules.json`.
- **Validation**: Running `ssq-hooks` prints classification result to stderr.

2.2 **Task: Implement Binary Proxying and Signal Handling [4h]**
- **Objective**: Execute the real tool after successful classification.
- **Context Boundary**: `cmd/ssq-hooks/proxy.go`.
- **Implementation**: Find the "real" binary using `PATH` (excluding the current directory). Use `exec.Command` and pipe `stdin/stdout/stderr`. Forward signals (SIGINT, SIGTERM).
- **Validation**: `ssq-hooks ls` behaves exactly like `ls`.

---

### Story 3: Integration Wrappers (Gemini, Open Code) [2h]
**User Value**: Easy setup for targeted tools.

**Acceptance Criteria**:
- Scripts or symlinks available for `gemini` and `open-code`.
- Installation is idempotent.

#### Tasks:
3.1 **Task: Create Installation and Wrapper Scripts [2h]**
- **Objective**: Provide a way to install the hooks.
- **Context Boundary**: `scripts/ssq-hooks-install.sh`, `scripts/hooks/gemini`, `scripts/hooks/open-code`.
- **Implementation**: Create symlinks in `~/.stapler-squad/bin/`. Update shell profiles.
- **Validation**: `which gemini` points to the `stapler-squad` bin directory.

---

## Known Issues

<bug number="001">
    <title>🐛 Recursion: Infinite Loop if PATH not managed correctly [SEVERITY: Critical]</title>
    <description>If `ssq-hooks` is in the `PATH` and calls `exec.LookPath("tool")`, it might find itself, leading to infinite recursion.</description>
    <mitigation>
        <strategy>Explicitly filter out the `stapler-squad/bin` directory when searching for the real tool.</strategy>
        <strategy>Use an environment variable like `SSQ_HOOKS_ACTIVE=1` to detect and break loops.</strategy>
    </mitigation>
    <files_likely_affected>
        <file>cmd/ssq-hooks/proxy.go - Responsible for tool discovery</file>
    </files_likely_affected>
    <related_tasks>2.2</related_tasks>
</bug>

<bug number="002">
    <title>🐛 TTY: Terminal State Corruption [SEVERITY: Medium]</title>
    <description>Proxying `stdin/stdout/stderr` might interfere with raw terminal modes required by some interactive tools.</description>
    <mitigation>
        <strategy>Use `os.StartProcess` or `exec.Command` with direct assignment to `os.Stdin` etc., rather than manual piping.</strategy>
    </mitigation>
    <files_likely_affected>
        <file>cmd/ssq-hooks/proxy.go - Handles I/O piping</file>
    </files_likely_affected>
    <related_tasks>2.2</related_tasks>
</bug>

---

## Dependency Visualization

```
Refactor pkg/classifier (1.1, 1.2)
       │
       ▼
Implement ssq-hooks binary (2.1)
       │
       ▼
Signal & Proxy Handling (2.2) ───┐
       │                         │
       ▼                         ▼
Installation Scripts (3.1) <── Integration (Story 3)
```

---

## Success Criteria
- [x] `pkg/classifier` exists and is used by `stapler-squad`.
- [x] `ssq-hooks` binary is built and functional.
- [x] `gemini` call is intercepted and blocked if it violates rules.
- [x] Real `gemini` call executes correctly if approved.
- [x] Zero regressions in existing `stapler-squad` tests.
