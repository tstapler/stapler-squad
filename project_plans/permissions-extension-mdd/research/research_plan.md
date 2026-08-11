# Research Plan: Permissions Extension (MDD)

**Status**: Complete | **Phase**: 3 — Implementation
**Project**: permissions-extension-mdd

## Objectives
Investigate the feasibility and design for modularizing the permissions classifier and extending it to Gemini, Open Code, and a standalone hooks binary.

## Dimensions

### 1. Stack
- **Focus**: Integration points for Gemini and Open Code hooks.
- **Questions**:
    - How do Gemini and Open Code CLI hooks work (e.g., pre-exec hooks)?
    - What are the best methods for binary delivery (e.g., `go install`, pre-built binaries)?
    - Can we use shell-level interception if native hooks aren't available?
- **Output**: `project_plans/permissions-extension-mdd/research/stack.md`

### 2. Features
- **Focus**: Comparable tools and security approaches.
- **Questions**:
    - How do `direnv`, `pre-commit`, or `boundary` handle command-level security?
    - Are there existing "permission-aware" shell proxies?
    - What are common command classification patterns in the industry?
- **Output**: `project_plans/permissions-extension-mdd/research/features.md`

### 3. Architecture
- **Focus**: Domain logic abstraction and sharing.
- **Questions**:
    - How to decouple `cmd/permissions.go` from `stapler-squad`'s TUI and server logic?
    - What's the interface for a "Classifier Plugin"?
    - How to handle configuration sharing between the main app and the standalone binary?
- **Output**: `project_plans/permissions-extension-mdd/research/architecture.md`

### 4. Pitfalls
- **Focus**: Known failure modes and risks.
- **Questions**:
    - Performance overhead of AST parsing on every command execution.
    - False positives/negatives in command classification across different shells (bash/zsh/fish).
    - Security risks of the "hooks" binary being bypassed.
- **Output**: `project_plans/permissions-extension-mdd/research/pitfalls.md`

## Parallel Execution Strategy
- Use `research-workflow` subagents for each dimension.
- Each agent reads `requirements.md` and writes its respective finding file.
