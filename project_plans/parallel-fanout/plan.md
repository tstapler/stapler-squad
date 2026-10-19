# Implementation Plan: Parallel Fan-Out

## Overview

Five phases: proto + backend RPC → backend winner-selection RPC → UI comparison layout → UI pick-winner action → docs + guardrails. The two RPCs and the UI are mostly independent after the proto is generated.

---

## Phase 1 — Proto and data model (backend)

### 1.1 Add `FanoutGroup` message and `FanOutSession` RPC

File: `proto/session/v1/session.proto`

- Add `message FanoutGroup` with fields `group_id`, `variant`, `total`, `model_override` (field numbers starting at the next available slot, confirmed via the proto file).
- Add optional `FanoutGroup fanout_group = 21` to `CreateSessionRequest`.
- Add `rpc FanOutSession(FanOutSessionRequest) returns (FanOutSessionResponse)`.
- Add `rpc PickFanoutWinner(PickFanoutWinnerRequest) returns (PickFanoutWinnerResponse)`.
- Run `make proto-gen` to regenerate.

### 1.2 Persist `fanout_group` in the ent schema

File: `session/ent/schema/` (new or extended schema entity)

- Add `fanout_group_id`, `fanout_variant`, `fanout_total`, `fanout_model_override` fields to the session entity (or a new `FanoutMembership` edge entity if a separate table is preferred for query efficiency).
- Run the correct `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` command (per CLAUDE.md).
- Do not commit generated output; only commit the schema change.

### 1.3 Expose group membership in `ListSessions` / `GetSession`

- Extend `Session` proto message to surface `FanoutGroup` so the UI can group and filter sessions.
- Update `session_storage.go` filter logic to support `fanout_group_id` as a filter dimension.

---

## Phase 2 — `FanOutSession` RPC implementation (backend)

File: `server/services/session_service.go` (new method `FanOutSession`)

### 2.1 Validation

- Reject `count < 2` or `count > 5` with `connect.CodeInvalidArgument`.
- Validate `base_title`, repo path, and other session fields identically to `CreateSession`.
- Check `QuotaGate` state: if the gate is paused, reject with a descriptive error (do not silently enqueue).

### 2.2 Bulk session creation

- Generate a `group_id` UUID.
- Call `CreateSession` (the internal helper, not the RPC) in a loop for each variant (0 to count-1), passing:
  - `title = base_title + " [variant N]"` (unique per variant)
  - `fanout_group = { group_id, variant: i, total: count, model_override: model_overrides[i] }`
  - Same prompt, repo path, program.
- Sessions are created sequentially (not goroutine-parallel) to avoid the worktree naming race identified in research.
- Return `group_id` and all `session_ids`.

### 2.3 Cost estimate endpoint

- Add a `EstimateFanoutCost(count, program) → { estimated_cost_usd }` RPC (or reuse the feature-flags endpoint).
- Client calls this before showing the launch dialog to populate the warning.

---

## Phase 3 — `PickFanoutWinner` RPC implementation (backend)

File: `server/services/session_service.go` (new method `PickFanoutWinner`)

### 3.1 Validation

- Confirm `group_id` exists and `winner_session_id` belongs to it.
- Confirm winner session is in a terminal or review state (not still running — enforce this so we don't merge a partial diff).

### 3.2 Merge

- Use the existing git merge helper to merge the winner's branch into the group's base branch.
- Return a structured error if the merge fails due to conflicts (let the user resolve manually).

### 3.3 Archive losers

- For each non-winner session in the group, call `Archive(session_id)` (the existing archive path).
- Delete (or mark for cleanup) their worktrees. The worktree cleanup should follow the same path as `pause_session`'s worktree deletion.

---

## Phase 4 — UI: comparison layout (frontend)

### 4.1 `FanoutComparisonView` component

File: `web-app/src/components/sessions/FanoutComparisonView.tsx`

- Fetches all sessions with matching `fanout_group_id` via `ListSessions` (filter by group).
- Renders them in a horizontal CSS grid: one column per variant.
- Each column contains:
  - Session title + status badge.
  - `DiffViewer` for that session's diff.
  - "Pick as winner" button (disabled unless session is in a completed/review state).
- Mark with `// +feature: parallel-fanout` in the first 10 lines.

### 4.2 Route / navigation

- Add a `/fanout/:groupId` route that renders `FanoutComparisonView`.
- Link from the session card for any session that has a `fanout_group_id` (a "Compare variants" badge/button).

### 4.3 Omnibar fan-out action

Per `docs/reference/session-creation-registry.md` (7 touchpoints) and `docs/reference/feature-testing-registry.md`:

1. `types.ts` — add `FanOutAction` to `OmnibarAction` union.
2. `dispatch.ts` — handle `FanOutAction`, call `FanOutSession` RPC.
3. `dispatch.test.ts` — unit tests for fan-out dispatch.
4. `detector.ts` — pattern: `"run N variants:"` or similar prefix.
5. `detector.test.ts` — unit tests for fan-out detection.
6. Session creation form — add "Variants (1–5)" numeric input.
7. Feature registry marker — `// +api: session:fanout` on the RPC handler.

### 4.4 Pre-launch cost warning

- Before dispatching, call `EstimateFanoutCost`, then show a confirmation dialog: "This will launch N sessions, estimated cost $X. Continue?"
- Wire cancel path to abort.

---

## Phase 5 — Guardrails, tests, docs

### 5.1 Server-side N ≤ 5 enforcement

- `FanOutSession` handler enforces `count ≤ 5` unconditionally (not just client-side).

### 5.2 `CapacityMonitor` integration

- Fan-out sessions are regular sessions from the monitor's perspective — no special casing needed, as confirmed in research.
- Add a log line at fan-out creation time: `"creating fan-out group"` with `group_id`, `count`.

### 5.3 Tests

- Unit tests for `FanOutSession` validation (count < 2, count > 5, quota-paused rejection).
- Unit tests for `PickFanoutWinner` (winner not in group, merge conflict path, archive path).
- E2E test: `tests/e2e/fanout-session-lifecycle.spec.ts` — create 2-variant fan-out, compare view renders, pick winner archives the other. Must start with `// @feature session:fanout`.

### 5.4 Documentation

- Add `docs/features.md` entry (or create the file if absent) describing the parallel fan-out workflow.
- Update `docs/reference/session-creation-registry.md` with the fan-out mode's 7 touchpoints.

---

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Sequential vs parallel session creation | Sequential | Avoids worktree naming race; N ≤ 5 so latency is acceptable |
| Separate `FanOutSession` RPC vs batch `CreateSession` | Separate RPC | Cleaner proto contract; `CreateSession` stays single-session |
| Winner merge target | Base branch of the group | Consistent with the existing worktree model |
| Loser disposition | Archive + delete worktree | Mirrors pause_session's cleanup; recoverable via worktree reconciliation if needed |
| Group persistence | New ent fields on Session | Simpler than a separate edge entity for this query cardinality |

---

## Effort Estimate (cost-class, not hours)

| Phase | Complexity | Primary cost driver |
|---|---|---|
| 1. Proto + ent schema | Low | Proto-gen, ent-gen, schema migration |
| 2. FanOutSession RPC | Medium | Validation, bulk creation, quota check |
| 3. PickFanoutWinner RPC | Medium | Git merge integration, archive path |
| 4. UI comparison layout | Medium-High | New component, route, omnibar 7 touchpoints |
| 5. Guardrails, tests, docs | Medium | E2E test, unit tests, doc update |

Total: ~3–4 focused agent-sessions across the 5 phases.
