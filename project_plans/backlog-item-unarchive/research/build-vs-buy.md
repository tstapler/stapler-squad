# Research: Build vs. Buy — Backlog Item Unarchive

## Buy / external library / SaaS

Not applicable. This is a single state-machine transition (`archived -> idea`) plus
one UI button inside a monolith that already owns every primitive involved:
ConnectRPC codegen (`make proto-gen`), the ent ORM repository layer
(`session/ent_repository_backlog.go`), the status-event audit trail
(`BacklogStatusEvent`), and the React action-dispatch pattern already used by every
other backlog action (`handleAction` in `BacklogItemDetail.tsx`,
`ActionsSection.tsx`). There is no third-party package, framework feature, or
hosted service that reaches inside a proprietary ent schema and ConnectRPC service
to flip one status field — "buy" doesn't parse as an option here. The real decision
is which of three in-house implementation shapes to use.

## Evidence gathered

- `session/domain/backlog.go:385-387` — `BacklogStatusArchived`'s only outbound
  transition is `{BacklogStatusIdea: true}`. The state machine already treats
  `archived -> idea` as *the* unarchive path; there is no separate "restore" state
  to model.
- `session/ent_repository_backlog.go:869-920` (`EntRepository.TransitionBacklogItemStatus`)
  — the CAS-based update sets `status` and `user_modified_status_at` but never
  touches `archived_at` on any transition, including this one. `ArchiveBacklogItem`
  (same file, lines 741-780) is the only method that ever writes `archived_at`
  (via `SetArchivedAt(now)`); nothing clears it.
- `server/services/backlog_service_lifecycle.go:486-611`
  (`BacklogService.TransitionBacklogItemStatus` RPC handler) already special-cases
  the backward-to-idea/refining direction at lines 593-606 ("reset planning
  approval so triage must re-run") — proof the handler already has a precedent
  block for "extra side effects when moving backward out of a terminal-ish state."
  Clearing `archived_at` for the `archived -> idea` case is the same shape of fix,
  not a new architectural pattern.
- `web-app/src/components/backlog/BacklogItemDetail.tsx:549-551` — the existing
  `send_back_idea` action is already wired end-to-end: `case "send_back_idea": await
  transitionStatus(item.id, "idea")`. `transitionStatus` is a thin wrapper over the
  generic `TransitionBacklogItemStatus` RPC (confirmed via the `case "reopen"` /
  `case "send_back_refining"` / `case "send_back_ready"` siblings, all calling the
  same `transitionStatus` helper with a different target string).
- `web-app/src/components/backlog/detail/ActionsSection.tsx:90-354` — every status
  has its own conditional render block (`item.status === "idea"`, `"ready"`,
  `"queued"`, `"in_progress"`, `"review"`, `"done"`) except `"archived"`, which has
  none. The `"done"` block (lines 320-354) already renders a "Return to Triage"
  button wired to `onAction("send_back_idea")` — the exact UI affordance the ask
  wants for `"archived"`, just missing a copy-pasted conditional branch.
- `server/services/session_service.go:4232-4250` (`SessionService.UnarchiveSession`)
  — the session pattern this ask is asked to "mirror" is trivial by comparison: no
  repository layer, no ent CAS update, no audit-event trail, no state-machine guard
  to satisfy. It's an in-memory `Instance.SetArchivedAt(nil)` + `SaveInstances`.
  Backlog items are ent-backed with a CAS precondition, a status-event history
  requirement (AC3), and an existing guarded state machine — the session RPC is not
  actually analogous in implementation weight, only in end-user intent ("undo an
  archive").

## Build-new-RPC vs. adapt-existing-RPC vs. fork-session-pattern-verbatim

### Option A — Build a new `UnarchiveBacklogItem` RPC (mirrors session's `UnarchiveSession`)

**Pros**
- Self-documenting endpoint name; intent is explicit in the API surface
  (`+api: backlog:unarchive` marker, discoverable in the feature registry without
  reading call sites).
- No `target_status` string threading required from the frontend — a single
  no-argument-besides-`item_id` call.
- Mirrors an established naming precedent (`UnarchiveSession`), so a reader who
  knows the session RPC recognizes the shape immediately.

**Cons**
- Requires new proto RPC + request/response messages (`proto/session/v1/backlog.proto`)
  and a `make proto-gen` regen touching both Go and TS bindings — for a state
  transition the generic RPC can already express and, per the requirements doc, can
  already execute today (the guard already permits `archived -> idea`).
- New handler in `backlog_service_lifecycle.go` would have to reimplement (or call
  into) the exact same guard check (`CanTransitionBacklog`), CAS-based repository
  update, and status-event recording that `TransitionBacklogItemStatus` already
  does — either duplicating that logic or making the new RPC a thin wrapper that
  calls the existing one internally, which raises the question of why it's a
  separate RPC at all.
- Adds a second backend code path that can independently drift from
  `TransitionBacklogItemStatus`'s behavior (e.g., the CAS precondition semantics,
  the `resolveStuckOnManualTransition` call, the work-session cleanup on terminal
  transitions) unless every one of those is deliberately re-derived for the
  archived case.
- Session's `UnarchiveSession` is a poor structural template: it has no ORM layer,
  no CAS, no state-machine guard, and no audit trail to satisfy — copying its
  *shape* (a dedicated single-purpose RPC) doesn't transfer any of its
  *simplicity*, because backlog items don't share that simplicity.

### Option B — Fix `TransitionBacklogItemStatus` to clear `archived_at`, reuse from a new UI button (adapt existing RPC)

**Pros**
- One-line-scale backend fix: add `.ClearArchivedAt()` (or `SetArchivedAt` to a
  zero/nil sentinel per the ent field's nillable setting) to the existing
  CAS update chain in `EntRepository.TransitionBacklogItemStatus`
  (`session/ent_repository_backlog.go:894-897`), gated on `toStatus !=
  BacklogStatusArchived` (or simply always clear it — archived is the only status
  that ever sets it, so clearing unconditionally on every transition is safe and
  arguably more correct: it fixes the stale-`archived_at` bug described in the
  requirements doc for *every* future path that reaches `archived -> X`, not just
  `-> idea`).
- No proto change, no `make proto-gen`, no new handler function — the existing
  `TransitionBacklogItemStatus` RPC handler already has the guard check, CAS
  precondition, and `recordStatusEvent`-equivalent history write (verify exact
  call site) that AC1 and AC3 require, for free.
- The frontend half is *already done* for a different status: `send_back_idea` /
  `transitionStatus(item.id, "idea")` is exactly the call an "Unarchive" button
  needs. Adding the button is copying the existing `"done"` block's "Return to
  Triage" button pattern in `ActionsSection.tsx` into a new `item.status ===
  "archived"` block — no new dispatch case is strictly required if the button
  reuses the `send_back_idea` action id, though a dedicated `"unarchive"` action id
  with its own confirm/success-message text (`ACTION_SUCCESS_MESSAGES`) reads
  better to a user than "Sent back to triage."
- Directly matches the codebase-correction framing in requirements.md: the ask
  itself concedes the state machine isn't missing a path, only the UI button and
  the `archived_at` clear are missing.

**Cons**
- The fix lives inside a shared, heavily-guarded method
  (`TransitionBacklogItemStatus` has a detailed doc comment about a
  previously-fixed TOCTOU race — BUG-026 — and is exercised by ~10 existing tests
  across `backlog_service_lifecycle_test.go`, `ent_repository_backlog_transition_test.go`,
  and `ent_repository_backlog_events_test.go`). Any change there carries slightly
  higher blast-radius risk than an isolated new function, and needs a test added
  that specifically asserts `archived_at` is cleared without perturbing existing
  transition tests (AC4, AC5).
- If a future transition *should* preserve `archived_at` for some reason (none
  identified today), an unconditional clear would need revisiting — worth gating
  the clear on `from == BacklogStatusArchived` rather than clearing on every call,
  to keep the change minimal and obviously scoped to this bug.

### Option C — Fork the session pattern verbatim (new RPC, but copy `UnarchiveSession`'s implementation style: no CAS, no guard, direct field clear)

**Pros**
- Fastest to write in isolation — a few lines, no proto message body needed beyond
  `item_id`.

**Cons**
- Actively wrong for this domain: bypassing `CanTransitionBacklog` and the CAS
  precondition would let an `UnarchiveBacklogItem` call succeed on an item that
  isn't actually `archived` (session's `UnarchiveSession` has no equivalent status
  state machine to violate, so its verbatim pattern has no guard to skip in the
  first place). Skipping `recordStatusEvent` would silently violate AC3 (audit
  history). This option only "looks" simple because it drops requirements the
  session RPC was never subject to.
- Still incurs all of Option A's proto/codegen/new-handler overhead while
  additionally sacrificing correctness guarantees Option A's authors would
  presumably want to keep (guard + CAS + audit event). Strictly dominated by
  Option A.

## Verdict

**Option B (adapt `TransitionBacklogItemStatus`) is the correct choice.** Option A
is not unreasonable in isolation — a named `UnarchiveBacklogItem` endpoint is
more discoverable — but it fails the "justified over reuse" bar the research
question sets: it would duplicate guard/CAS/audit logic that already exists and
works, purely for a naming/discoverability preference, on an RPC the requirements
doc already confirms can perform the transition today modulo the `archived_at`
bug. Option C is dominated by Option A on every axis and should not be considered.

Recommended shape for plan.md: gate the `archived_at` clear in
`EntRepository.TransitionBacklogItemStatus` on `from == BacklogStatusArchived`
(minimal, obviously-scoped diff, addresses the exact bug named in the requirements
doc without touching behavior for any other transition pair), add a matching
`ActionsSection.tsx` render block for `item.status === "archived"`, and decide
during planning whether the button reuses the `send_back_idea` action id or gets
its own `"unarchive"` id purely for clearer confirm/toast copy (functionally
identical either way — both call `transitionStatus(item.id, "idea")`).
