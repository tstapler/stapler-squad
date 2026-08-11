# ADR-001: `server/services/`-One-File-Per-Responsibility-Cluster is the Standing Convention

**Status**: Accepted
**Date**: 2026-07-01
**Deciders**: Tyler Stapler

## Context

`session_service.go` (4,542 lines) is this repo's #1 architectural hotspot by every measure (temporal-coupling audit: 70 revisions in ~2 months, hotspot score 3x the runner-up, 49 distinct `project_plans/*` mentions) and has never had a dedicated ADR despite this. Investigation found the pattern for fixing this — extract a focused `XService` type with its own constructor, inject it into `SessionService`, reduce the RPC method to a delegate — already exists de facto across 26 files (`project_service.go`, `github_service.go`, `workflow_service.go`, etc.), but was never written down as a rule. New features have no documented signal to follow this pattern rather than adding another method directly to `session_service.go`.

## Decision

Formalize the existing convention as a standing rule: **any new RPC-handler logic that forms a cohesive cluster of 3+ related operations, or any single operation exceeding ~30 lines of business logic, gets its own `server/services/<cluster>_service.go` file** with:
- A `New<Cluster>Service(deps...) *<Cluster>Service` constructor taking only the dependencies that cluster actually needs (not the full dependency bag).
- `SessionService` (or whichever RPC-facing type owns the proto service interface) holds a `<cluster>Svc *<Cluster>Service` field, wired at construction, with RPC methods reduced to single-line delegates.
- A colocated `<cluster>_service_test.go`.

This extends, rather than replaces, the pattern already visible in every existing `*_service.go` file in the directory.

## Consequences

### Positive
- Codifies a pattern that was already working — this ADR adds zero new mechanism, only a documented rule.
- Gives future contributors (and Claude Code sessions) an explicit answer to "where does this new RPC logic go," closing the governance gap the temporal-coupling audit flagged.
- `SessionService` itself stays as the session-lifecycle aggregate root plus composition root — its own remaining size is bounded by (a) irreducible ConnectRPC interface boilerplate and (b) legitimately-owned lifecycle logic, not indefinite accretion of unrelated concerns.

### Negative / Accepted tradeoffs
- ConnectRPC's single-interface-per-proto-service constraint means `SessionService` will always need one method per RPC the proto defines, even when every one delegates in one line — this ADR does not and cannot eliminate that boilerplate, only ensure it stays boilerplate rather than accreting real logic.
- Judgment is still required on "cohesive cluster of 3+" — this is a heuristic, not a mechanical rule; `code-hotspot-analysis`/`architecture-review` remain the tools for identifying when a threshold is crossed, not a hard line count.
