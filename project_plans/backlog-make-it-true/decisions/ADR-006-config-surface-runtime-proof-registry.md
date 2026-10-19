# ADR-006: Config surfaces are proven by a registry-driven reflection test, not an analyzer

**Status**: Accepted (planning; amended 2026-10-17 for the ratcheted pending list)
**Date**: 2026-10-17
**Project**: backlog-make-it-true, workstream E

## Context

Success Metric 5: a repo check must fail when a stage-config or quota setting is added to the UI/proto without a test that exercises its runtime effect. `tools/lint` analyzers are lexical and name-matched and "do not follow calls across packages" (`.claude/rules/noarchivedrevival.md`); proto types are generated and gitignored. A grep over the proto diff is gameable and is the "self-testing guard" anti-pattern (memory: omnibar review). Today's bug is exactly a test passing against the engine in isolation while `dependencies.go:678` wired the wrong one.

## Decision

- New test `server/services/config_surface_proof_test.go` with a registry `configSurfaceProofs map[surfaceID]proofEntry{testName, kind}`.
- **Enumeration by reflection**: JSON tags of `config.QuotaConfig` (plus the new `DefaultCostBudgetThresholdUSD`), the `knownFeatureFlags` names, and the field descriptors of the stage-config CRUD request messages and `UpdateQuotaConfig` via `protoreflect` (available after `make build`). A field with no registry entry fails with its exact name.
- **Anti-gaming rules enforced by the same test**: the named test function must exist (parsed with `go/parser` over the package's `_test.go` files), must call the shared helper `assertConfigChangesBehavior(t, ...)`, and that helper requires an entry-point (`TransitionBacklogItemStatus`, `QuotaGate.Reconcile`, `GetStuckReport`, etc.) closure plus a before/after observable that differs. A proof that constructs the engine directly instead of `BacklogService` is rejected by an `entrypoint` field in the entry. Wiring is separately pinned by the wiring test (Story 1.1.4).
- **Narrow companion, deferred**: a `runtime_test:"TestName"` struct tag on `QuotaConfig` fields is reflected by the same test (no new analyzer). A `tools/lint` analyzer for tag presence is explicitly deferred; the reflection test already fails on a tag-less new field.
- Surfaces exempt by design (display-only fields such as `status_detail`) are listed in the registry with `exempt: "<reason>"`; an exemption without a reason fails.

## Alternatives rejected

- Analyzer as the sole mechanism (E2), proto-diff CI script (E3).

## Consequences

- Story 5.1.1 ships the test green from day one with a ratcheted `pending` list that names EVERY existing and planned surface: all existing `QuotaConfig` fields, all existing `knownFeatureFlags` names, the four engine mode flags (`backlog:workflow-engine-shadow`, `-enforce`, `-enforce-agent`, `-enforce-system`), `DefaultCostBudgetThresholdUSD`, the auto-merge status field and the stage-config CRUD request fields. The list can only shrink: a story that registers a real entry must delete the matching pending line in the same change, and a pending line whose surface already has a real entry fails the test. A surface that is neither registered nor pending fails (this is what catches a new knob). Stories register their entries as they land (2.1.1b, 1.5.2b, 1.5.9c, 5.1.2); Story 5.1.3 backfills the remainder and the list must be empty at 5.1.3. This replaces the earlier "same story" wording that left the test red for waves W1-W11.
- Test avoids `BuildDependencies()` (real network calls); uses narrow `ServerDependencies` (memory: CI hermetic gotchas).
