# ADR-002: Push ContextHealth onto `Instance` rather than pulling it in `InstanceToProto`

**Date**: 2026-08-02
**Status**: Accepted
**Project**: context-health-monitoring
**Refines**: `research/architecture.md` §2

## Context

`research/architecture.md` §2 proposes populating the new proto fields inside `InstanceToProto`
by querying the analyzer store: `contextHealthStore.GetByUUID(inst's conversation UUID)`, mirroring
how `DetectedStatus` is computed at lines 157-171 of `server/adapters/instance_adapter.go`.

That comparison does not hold on inspection. `DetectedStatus` is reachable from the adapter because
it hangs off the `Instance` itself (`inst.GetStatusManager().GetStatus(inst)`) — the adapter needs no
extra dependency. A `TokenStore` handle is different: the adapter has none, and

```
$ grep -rn "InstanceToProto(" --include="*.go" . | wc -l
24
```

24 call sites across `server/services/session_service.go` (13), `server/services/event_converter.go`
(3), `server/services/workspace_service.go` (1), and `server/adapters/instance_adapter_test.go` (6).
Threading a store parameter through all of them — or introducing a package-level mutable global in
`server/adapters` — is a large, high-churn change for a single derived field.

A directly analogous field already solves this problem in the opposite direction. `Session.artifacts`
(proto field 70) is also derived from Claude JSONL by a background scanner, and it is **pushed**:
`artifactExtractor.OnScanComplete` calls `inst.SetArtifacts(blob)` and publishes a session-updated
event (`server/dependencies.go:1114-1138`); the adapter then just reads `snap.Artifacts`
(`server/adapters/instance_adapter.go:76-78`).

## Decision

Deliver `ContextHealth` by **push**, following the `Artifacts` precedent exactly:

```
TokenStore.Subscribe()  →  publishContextHealth (server/dependencies.go)
                        →  Instance.SetContextHealth(verdict)
                        →  InstanceSnapshot.ContextHealth
                        →  InstanceToProto reads snap.ContextHealth  (no signature change)
                        →  existing WatchSessions stream
```

`InstanceToProto`'s signature is unchanged. Computation still lives in `session/tokens`, exactly as
`research/architecture.md` §1 recommends — only the *delivery* mechanism differs from its §2 sketch.

## Consequences

**Positive**
- Zero changes to 24 call sites and 6 existing adapter tests.
- The level-*transition* comparison the Observability Requirements call for ("log green→amber,
  amber→red") falls out naturally: the publisher already holds both the previous verdict
  (`inst.Snapshot().ContextHealth`) and the new one. A pull design has no natural place to observe a
  transition, since it recomputes statelessly per conversion.
- Gives condition-change gating for free (`research/pitfalls.md` §5): an unchanged **level** publishes
  no event and logs nothing (the `Reason` string is still updated on `Instance` so the tooltip stays
  current, but a count-only change with the same level does not itself trigger a log/event — see
  plan.md Task 2.2.3a, corrected after an adversarial-review BLOCKER caught this doc originally saying
  "verdict," which would have made the log fire on every count increment, not just level transitions).
- "Freeze while paused" (`research/ux.md` §4) needs no special case: a paused session writes no new
  JSONL, so no re-parse fires, so the last verdict simply persists.

**Negative**
- Adds one field to `Instance`/`InstanceSnapshot`, which `research/architecture.md` §2 advised
  against ("not a field mutated in place on `Instance`"). The `Artifacts` precedent shows this is an
  accepted shape in this codebase for exactly this class of asynchronously-derived JSONL data, and
  `SetContextHealth` uses the same actor-serialised `sendSyncErr` path as `SetArtifacts`, so it adds
  no new locking surface.
- The proto value is eventually consistent — it reflects the last publish rather than being
  recomputed per conversion. Latency is bounded by `TokenStore`'s fsnotify → worker-pool cycle, the
  same bound `Artifacts` and `InsightsService.WatchInsights` already live with.

**Rejected alternative**
Package-level mutable `TokenStoreReader` global in `server/adapters`, set at startup. Rejected: hidden
global state, untestable in parallel, and it would make `InstanceToProto` — a pure conversion
function today — order-dependent on server bootstrap.
