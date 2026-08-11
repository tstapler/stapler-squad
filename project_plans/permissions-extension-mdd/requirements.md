# Requirements: Permissions Extension (MDD)

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-03

## Problem Statement

When using `claude-squad` on personal machines, users jump between different models (Gemini, Open Code) to leverage free tiers. Separately, there's interest in the permissions classifier (the command classification and AST parsing logic) independent of the full `stapler-squad` workflow system.

## Success Criteria

- Rollout the same command classifier to all targeted CLIs (Claude, Gemini, Open Code).
- Expand coverage for commands and AST parsing to perform deep security analysis.
- The classifier logic is usable as a standalone hooks binary outside of `stapler-squad`.

## Scope

### Must Have (MoSCoW)
- Support for Gemini and Open Code.
- Independent hooks binary.
- Full reuse of domain logic between the standalone binary and the main project.

### Out of Scope
- (Currently focused on the classifier and hooks)

## Constraints

- **Tech stack**: Match the existing Go project and maintain logic portability.
- **Dependencies**: Must be able to run independently of the full `stapler-squad` workflow system.

## Context

### Existing Work
- Core logic exists within the `stapler-squad` project (e.g., `cmd/permissions.go`). No specific implementation decisions for the new hooks have been made yet.

### Stakeholders
- User (Self)

## Research Dimensions Needed

- [ ] Stack — Evaluate integration points for Gemini and Open Code hooks (e.g., shell aliases vs. native hook systems).
- [ ] Features — Survey how other security-focused CLIs or hook systems (like `direnv` or `pre-commit`) handle command classification.
- [ ] Architecture — Design the abstraction layer to share domain logic between the main project and the new standalone binary.
- [ ] Pitfalls — Identify risks in AST parsing performance or false positives in command classification across different shells.
