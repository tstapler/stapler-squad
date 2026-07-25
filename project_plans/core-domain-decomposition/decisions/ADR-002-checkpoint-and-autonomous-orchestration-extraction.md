# ADR-002: Extract `CheckpointService` and `AutonomousOrchestrationService`

**Status**: Accepted (Decision/Context scope-corrected 2026-07-01 after independent adversarial review — see Amendment)
**Date**: 2026-07-01
**Deciders**: Tyler Stapler
**Relates to**: `ADR-001` (the governance rule this decision applies)

## Context

Per `research/architecture.md` §2, direct body-line-count inspection of `session_service.go` found two clusters with genuine inline business logic (not yet following ADR-001's established delegate pattern): checkpoint operations (`CreateCheckpoint` 34 lines, `ListCheckpoints` 23 lines, `ClearConversationState`) and autonomous-driver orchestration (~250 lines across `registerDriver`/`stopAndDeregisterDriver`/`StopDriverForSession`/`buildTurnCallback`/`StartAutonomousDriverForInstance`/`StartAutonomousDriverWithTimeout`/`onAutonomousDriverComplete`/5 `wireXCallback` methods).

`GetProviderLimits` (76 lines) and `GetDetectionEvents` (36 lines) were also found long but are deliberately **not** included in this decision — each needs individual evaluation during implementation (per `requirements.md` R1.3) rather than being extracted merely for exceeding a line count, avoiding extraction-for-its-own-sake.

**Amendment (post-third-adversarial-review scope correction)**: The Decision below originally read "extract exactly two new services in this project." That was accurate when this ADR was written, but a later adversarial review pass expanded this project's scope (see `requirements.md` R1.9-R1.12 and `plan.md` Stories 1.4-1.6) to add two further new services this ADR never covered — `TerminalService` and `FeatureFlagService` — plus two RPC methods delegated to already-existing sibling services rather than extracted as new ones. The "exactly two" wording is corrected below to enumerate the project's actual current scope of four new services. This is the same class of fix `ADR-003` already applied to its own Context/Decision for the `Preview()` normalization scope.

## Decision

This project extracts **four** new services in total:

- `CheckpointService` (session/instance_checkpoint.go's RPC-facing wrapper) and `AutonomousOrchestrationService` (the driver-lifecycle cluster) — the original two clusters this ADR was written for, per the shapes sketched in `research/architecture.md` §3-4 (Stories 1.1-1.2).
- `TerminalService` (`GetTerminalSnapshot`/`WriteToSession`, Story 1.5) and `FeatureFlagService` (`GetFeatureFlags`/`UpdateFeatureFlag`, Story 1.6) — added later by the adversarial review's expanded inventory of undelegated clusters in `session_service.go`. Both are new services because, unlike `ListBranches`/`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` below, no existing sibling service is a natural home for them (per `plan.md` Stories 1.5/1.6's rationale).

Separately, `ListBranches`, `ArchiveWorkflowSessions`, and `DeleteWorkflowFailedSessions` (Story 1.4) are also being extracted from `session_service.go`, but **not** as new services — they delegate to the already-existing `WorkspaceService` (`ListBranches`) and `WorkflowService` (`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions`), reusing `ADR-001`'s already-formalized delegate pattern rather than requiring new-service governance under this ADR.

`AutonomousOrchestrationService`'s extraction is a **structural move only** — it does not fix the unguarded `Instance` field writes in `onAutonomousDriverComplete` (`AutonomousMode`/`AutonomousTurn`/`AutonomousMaxTurns`/`AutonomousOutcome`), which are `instance-actor-concurrency`'s Epic 5 scope. This project relocates the method; that project fixes its synchronization. Explicitly not claiming this ADR resolves that race.

## Consequences

### Positive
- Removes the largest remaining undelegated clusters from `session_service.go` — the original two (checkpoints, autonomous-driver orchestration) plus the two added by the scope correction (terminal I/O, feature flags) — following the now-formalized ADR-001 pattern.
- `AutonomousOrchestrationService` becomes the single, documented home for driver lifecycle — previously scattered across 8+ methods with no unifying type.
- `TerminalService`/`FeatureFlagService` give terminal I/O and feature-flag logic their own documented homes instead of staying inline in `session_service.go`; `ListBranches`/`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` join their already-established sibling services (`WorkspaceService`/`WorkflowService`) instead of needing new-service governance at all.

### Negative / Accepted tradeoffs
- Must land in a way that doesn't conflict with `instance-actor-concurrency`'s Epic 5 touching the same `onAutonomousDriverComplete` callback body — coordinate landing order (see `requirements.md` Sequencing) or expect a merge conflict, not a design conflict, to resolve.
- `GetProviderLimits`/`GetDetectionEvents` remain unresolved by this ADR; a follow-up decision may be needed if implementation-time investigation finds they warrant extraction too.
