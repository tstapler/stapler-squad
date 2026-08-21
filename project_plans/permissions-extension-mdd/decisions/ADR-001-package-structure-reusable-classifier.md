# ADR-001: Package Structure for Reusable Classifier

**Status**: Accepted
**Date**: 2026-04-06
**Project**: permissions-extension-mdd

## Context
The command classification and AST parsing logic currently reside in `server/services/classifier.go` and `server/services/command_parser.go`. These files are tightly coupled to the `stapler-squad` server infrastructure. To enable reuse in a standalone hooks binary and other CLI proxies (Gemini, Open Code), this logic must be moved to a decoupled package.

## Decision
We will extract the classifier and parser into a new top-level package `pkg/classifier`.

## Rationale
- `pkg/` is the Go convention for code that is intended to be reused by external projects or other internal binaries.
- Moving logic out of `server/services` removes dependencies on `approval_store`, `analytics_store`, and other server-side components.
- Standardizes the interface for command classification across all `stapler-squad` related tools.

## Consequences
- `stapler-squad` server will depend on `pkg/classifier`.
- `ssq-hooks` binary will depend on `pkg/classifier`.
- Requires refactoring of existing tests to point to the new package.
- Any server-specific logic (like analytics recording) must be handled by callers, not the classifier itself.

## Patterns Applied
- **Modular Monolith**: Separating core logic from delivery mechanisms (Server, CLI).
- **Dependency Inversion**: High-level components (Server) depend on abstractions in `pkg/`.
