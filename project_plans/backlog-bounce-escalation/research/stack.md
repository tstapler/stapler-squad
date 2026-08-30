# Stack Research: backlog-bounce-escalation

## Summary

This is **pure reuse of the existing stack** — Go stdlib, ent ORM (SQLite), ConnectRPC,
React/vanilla-extract. No new dependencies are needed. The feature is an incremental
extension of the `backlog_stuck_states` durable-storage infrastructure built by the
`backlog-stuck-item-visibility` project, following the exact same architectural patterns
already established in that codebase.

## Existing Infrastructure to Build On

### 1. Storage layer: ent ORM + SQLite

- Schema: `session/ent/schema/backlog_stuck_state.go` — `BacklogStuckState` entity, one row
  per `(item_id, reason)` pair (resolve-in-place model, not append-only), enforced by a
  plain 2-column unique index (`index.Fields("item_id", "reason").Unique()`).
- Fields relevant to this feature already exist: `remediation_attempts` (int32, default 0),
  `next_remediation_at` (nillable time — NULL + `remediation_attempts >= cap` means
  "parked"), `resolved_at` (nillable — NULL means open), `notified_at` (nillable — NULL
  means "detected but not yet notified, fires exactly once").
- **go.mod versions** (verified via `go.mod`): `entgo.io/ent v0.14.5`,
  `connectrpc.com/connect v1.19.0`, `github.com/google/uuid v1.6.0`,
  `google.golang.org/protobuf v1.36.11`, Go `1.26.3`.
- Any schema change (e.g. adding a `severity`/`escalated_at` field to
  `BacklogStuckState`, or a new small schema for a multi-reason escalation marker) must be
  regenerated with the project's mandatory flag per `.claude/rules/ent-schema-generation.md`:
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
  Omitting `--feature sql/upsert` silently breaks `UpsertRule`-style upsert methods used
  elsewhere in this package (the resolve-in-place `MarkStuck` upsert pattern depends on it).

### 2. Pure decision-function layer (no DB, no I/O)

`session/stuck_decisions.go` is the established pattern for this kind of threshold/severity
logic: plain functions over primitive values (times, counts, bools) → bool, kept
exhaustively table-driven-testable, with detector thresholds as named `const`/`var` at the
top of the file (e.g. `bounceThreshold = 3`, `bounceLookback = 24 * time.Hour`,
`abandonedReviewGrace = 15 * time.Minute`). A new "N simultaneous open reasons" or
"capped-while-bouncing" predicate belongs here as another pure function (e.g.
`isMultiReasonEscalation(openReasonCount int) bool`, `isCappedWhileBouncing(attempts,
cap int32, bouncingOpen bool) bool`), called from the reconcile loop — not inlined.

### 3. Reconciliation / remediation gate

`session/backlog_remediation.go` — `evaluateRemediation()` is the shared pure
backoff-schedule function (cap → backoff-due → restart-grace, in that order) that every
remediation action already goes through. `MaxRemediationAttempts` (currently 5, derived from
`len(remediationBackoffSchedule)`) is the existing cap this feature's item 2
("capped while still bouncing") keys off directly — no new cap needed, just a new signal
computed from the existing `remediation_attempts >= MaxRemediationAttempts` condition
combined with "does this item still have an open `bouncing` row."

`FindOpenStuckStates(ctx)` (used pervasively across `session/backlog_lifecycle_*.go`) is
the existing query to enumerate all open rows for an item — the natural input to a
"count simultaneous open reasons per item" computation for escalation item 1.

### 4. RPC / proto layer

`server/services/backlog_service_stuck.go` — existing ConnectRPC handlers
(`ListStuckBacklogItems`, `SnoozeStuckItem`) with proto enum mapping helpers
(`toProtoStuckReason`/`fromProtoStuckReason`) between `domain.StuckReason` and
`sessionv1.StuckReason`. A new severity/escalation field on `StuckBacklogItem` (or a new
small RPC) follows this exact same mapping-function + read-projection pattern
(`OpenStuckStateData` → proto). Any new/modified RPC requires:
- `proto/session/v1/*.proto` edits → `make proto-gen` (regenerates both
  `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`)
- A feature-registry entry per `.claude/rules/feature-registry.md`
  (`docs/registry/features/backend/<feature>.json`), then `make registry-generate`

### 5. Notification mechanism

`session/backlog_lifecycle.go:459` — `(l *BacklogLifecycleListener) notify(itemID, title,
message string, notificationType, priority int32)`, backed by
`server/services/backlog_notifier.go`'s `EventBusNotifier.Notify` (an in-process event bus,
not a new external service). The requirements doc explicitly wants the new escalation signal
to be **durable/queryable**, not a one-time toast like the existing `notified_at`-gated
"use Reset" notification (`session/backlog_lifecycle_review.go:592`) — so the natural
approach is: (a) a durable field/row (ent-backed, same SQLite DB) that persists the
escalated state and is queryable via a `List`-style RPC (mirroring
`ListStuckBacklogItems`), plus (b) reuse of the existing one-time `notify()` call for the
*event* of first crossing the escalation threshold. No new notification library/service
needed — there is no separate "Notification" ent schema; notifications today are
fire-and-forget event-bus pushes, with durability coming from the stuck-state table itself
(exactly the model this feature should extend).

### 6. Frontend

React SPA in `web-app/`, vanilla-extract for any new component styling
(`.claude/rules/css-architecture.md`) — no new CSS modules. If a UI surface is added (e.g.
a severity badge or escalation indicator on the existing stuck-items view), it consumes the
new proto field(s) already generated into `web-app/src/gen/session/v1/*_pb.ts` — no new
frontend dependency. Package manager is pnpm only (`.claude/rules/package-manager.md`).

## Dependencies Needed

**None.** This is pure reuse of:
- Go stdlib (`time`, `context`, `errors`, `fmt`)
- `entgo.io/ent v0.14.5` (existing schema extension or new small schema, same package)
- `connectrpc.com/connect v1.19.0` (existing RPC pattern)
- `google.golang.org/protobuf v1.36.11` (existing codegen pipeline via `make proto-gen`)
- `github.com/google/uuid v1.6.0` (existing ID pattern, if a new schema/row is added)
- React + vanilla-extract (existing frontend stack, if UI surfacing is in scope)

## Patterns to Match (from existing code, not to be reinvented)

1. **Pure predicate functions** in `session/stuck_decisions.go`-style files — no DB/I/O,
   table-driven-testable, named constants for thresholds.
2. **Resolve-in-place** row semantics (reuse the same `BacklogStuckState` row or a similarly
   modeled new field) rather than append-only history — per ADR-001 "Durable Stuck-State
   Storage Model" (referenced in the schema's doc comment), a plain unique index can't be an
   `OnConflictColumns` target for append-only on SQLite since NULLs are distinct.
3. **One-time notification gate** via a nillable `*_at` timestamp field (like `notified_at`),
   not a boolean, to represent "fired once, don't refire."
4. **Read-projection → proto mapping function** pattern (`stuckBacklogItemToProto`) rather
   than exposing ent-generated types directly over RPC.
5. Detector/threshold constants belong in `session/stuck_decisions.go` or
   `session/backlog_remediation.go`, explicitly documented as independent of
   `config.Config` unless there's a stated reason to make them user-configurable (see the
   `bounceThreshold` doc comment explaining why it's *not* wired to config today).

## Open Question for Planning Phase

Whether "N simultaneous open reasons" and "capped-while-bouncing" warrant a genuinely new
`BacklogStuckState` field (e.g. `severity int32` computed and stored per-row) vs. a
purely computed/derived value at read time (in `stuckBacklogItemToProto` or a new
aggregation query) with no schema change at all. The requirements doc's "durable... rather
than a one-time toast" success criterion leans toward *some* persisted state, but the
existing `remediation_attempts`/`resolved_at` fields on the row already carry enough raw
data that severity could plausibly be derived at query time without a new column — this
is an architecture decision for `/sdd:3-plan`, not resolved by this stack research.
