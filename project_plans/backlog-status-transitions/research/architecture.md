# Architecture Research: backlog-status-transitions

Builds on `research/stack.md` (BacklogStatus enum shape, MCP tool registration pattern,
ent schema/no-migration finding, reconciliation-loop line inventory) — not re-derived
here, cited by file:line. Answers: applicable patterns, integration points, data-flow/
consistency requirements around the CAS mechanism, an Event-Command-Policy table, and
the enum-vs-`close_reason` decision flagged in requirements.md's "Notes for Downstream
Phases."

## 0. Critical finding: a parallel SDD project already exists for this exact problem, with a materially different design

`project_plans/backlog-self-resolve/` (requirements.md dated 2026-08-01 — today) targets
**the same underlying pain** (work session discovers its item is redundant, has no
self-service closure tool) but its `research/architecture.md` reaches a **different
architectural conclusion**: it deliberately does *not* add a new terminal `BacklogStatus`
at all. Instead its proposed `report_duplicate` tool routes the item into the *existing*
`review` state (same edge `request_review` uses) with the duplicate evidence attached,
and lets the existing review-verdict pipeline (`handleReviewSessionExited`,
`session/backlog_lifecycle.go:954`) make the actual terminal decision. Its stated reason,
quoting its own research:

> "`submit_review_verdict`'s comment (`tools_backlog.go:548-556`) establishes the actual
> governing pattern for this feature: *only one code path may ever drive a given terminal
> transition*... `report_duplicate` must respect this discipline exactly: it is a
> *producer of evidence and a trigger into `review`*, never a second writer of
> `review → done`/`review → duplicate`."

I independently confirmed the cited comment — `server/mcp/tools_backlog.go:548-556`
(`submit_review_verdict` handler) states verbatim: *"Deliberately no status transition
here: `BacklogLifecycleListener.handleReviewSessionExited`... is the sole place that
decides what happens next once this review session exits... Transitioning straight to
done here would race that handler."* This is a real, load-bearing, pre-existing
single-writer-per-terminal-state invariant in this codebase, not a stylistic preference.

**This directly conflicts with `backlog-status-transitions`'s requirements.md AC1**,
which calls for the work session to self-certify straight into a *new terminal* status
in one CAS call — i.e., a **second writer of a terminal transition**, bypassing the
review gate entirely. `backlog-self-resolve`'s research also invokes a second relevant
established pattern — "skeptical-by-default self-report handling"
(`AggregateOutcome`'s empty-verdict → FAIL default, `session/domain/backlog.go:132-137`)
— to argue an agent's *unverified* claim of "this is moot" should never itself be
sufficient to reach a terminal state; it should only ever be *evidence* fed to whichever
mechanism (human or reviewer) still makes the call. (Note: `backlog-self-resolve`'s doc
cites this at lines 132-137; independently re-checked this pass — `AggregateOutcome`'s
actual empty-verdict → FAIL default is at `session/domain/backlog.go:294-299` as of this
reading, not 132-137. The behavior and the argument built on it both check out; only the
line number in the other doc is stale — flag it if that doc is revised.)

**This is a design decision the plan phase (`sdd:3-plan`), not this research doc, must
resolve** — it changes whether this feature needs a new `BacklogStatus` value at all.
Rest of this document evaluates both directions per the assigned research question
(which explicitly asks about "a new terminal BacklogStatus"), but plan-phase should
read `project_plans/backlog-self-resolve/research/architecture.md` and
`.../research/features.md` in full before finalizing, and someone should decide whether
these are one feature or two competing proposals for the same backlog item before both
proceed independently into implementation.

Two ways this can resolve, both legitimate, with different tradeoffs:

| | New terminal status, self-certified (this project's premise) | Route into `review`, reviewer decides (backlog-self-resolve's premise) |
|---|---|---|
| Terminal-writer discipline | Violates it — work session becomes 2nd writer of a terminal edge | Preserves it — `review`→terminal stays single-writer |
| Self-report trust | Work session's own claim is authoritative | Work session's claim is evidence only, reviewer verifies |
| Latency to closure | Immediate (one CAS call) | One more review cycle (spawns a review session) |
| Matches "duplicate is not really reviewable work" framing | Yes — there's no diff to review, so routing through `review` is arguably a category error | No — but avoids ever trusting a work session's own "nothing to see here" claim |
| Requirements doc alignment | Matches AC1/AC2 as written | Requires reworking AC1 to "routes to review with evidence" |

If planning keeps the self-certified-terminal-status design, the compensating control
this codebase's own conventions suggest is a narrow, mechanically-verified precondition
(e.g. `reference_url` must resolve to a real, merged/closed GitHub PR/issue — mirroring
`reportPRCreated`'s `h.verifyPR` GitHub cross-check at `tools_backlog.go:707`) rather than
accepting a bare free-text claim — this is the same "mechanical verification proves
relevance, not correctness" line backlog-self-resolve draws, applied to a
self-certified-terminal design instead of a routed-through-review one.

## 1. Applicable architectural patterns

### 1.1 Guarded State Machine extension (not a new domain concept)

`session/domain/backlog.go` is a textbook Guarded State Machine: enum (`BacklogStatus`,
lines 16-24), transition table (`validTransitions`, lines 331-388), and a separate guard
layer (`TransitionGuard` — sentinel errors at line 415 onward) for preconditions that
aren't simple from→to booleans. A new terminal status is a **new node + a new set of
inbound edges** in this existing machine, not a new pattern. Per requirements AC2, the
inbound edge set is `{in_progress, review, pr_pending} → closed`; the outbound edge set
should be empty or, mirroring `BacklogStatusArchived`'s only edge (`archived → idea`,
line 385-387), a single manual "reopen to idea" escape hatch for operator correction —
recommend copying that exact shape (`closed → idea`) rather than inventing new reopen
semantics.

### 1.2 Command Pattern with an authority-scoped write model (role-gated CQRS-lite)

Every MCP tool in `tools_backlog.go` is a role-scoped command: `who` (role gate) may
invoke `what` (transition), one handler owns one precondition. `requestReview` (line
337) and `reportPRCreated` (line 623) are the two closest templates — stack.md's §2
already lays out the 9-step handler skeleton (feature-flag gate → auth → arg parsing →
link check → role check → CAS transition → audit note → log → result). The new tool
(`mark_duplicate` or similar) is additive to this vocabulary, not a modification of an
existing command's permission scope — consistent with requirements.md's explicit
Out-of-Scope rejection of a generic `update_backlog_item_status` escape hatch.

### 1.3 Ent-layer CAS does NOT enforce state-machine validity — must be enforced in the handler

Confirmed by reading `TransitionBacklogItemStatus` in full
(`session/ent_repository_backlog.go:869-943`): it performs a genuine SQL-level
compare-and-swap (`UPDATE ... WHERE status = ? [AND updated_at = ?]`, folded into the
same statement as the write — the fix for BUG-026, see §3 below) and unconditionally
appends a `BacklogStatusEvent` via `recordStatusEvent` (line 938). **It does not call
`CanTransitionBacklog` or `TransitionGuard` at all.** `backlog-self-resolve`'s research
(`research/architecture.md` §2.2) independently confirmed the same thing and traced why:
`TransitionGuard`/`CanTransitionBacklog` are invoked only from `WorkflowEngine`
(`session/workflow_engine.go`), which only `BacklogService` holds a reference to
(`server/services/backlog_service.go:114`) — `backlogHandlers` (the MCP tool struct,
`tools_backlog.go:91-110`) wraps `*session.Storage` directly and has no `WorkflowEngine`.
**Every existing MCP tool handler already bypasses state-machine validation, relying
solely on its own hardcoded precondition** (e.g. `requestReview`'s
`ExpectedStatus: string(session.BacklogStatusInProgress)` at line 414). This means the
new `mark_duplicate` handler must call `session.CanTransitionBacklog(from, to)` itself
and treat `false` as a hard error — it cannot rely on the ent layer, on
`TransitionGuard`, or on `WorkflowEngine` to reject an invalid edge (e.g. calling it on
an already-`done` item, if `done` is deliberately excluded from the inbound set).

### 1.4 Optimistic CAS against currently-observed status, not a hardcoded expectation

AC2 requires the tool work from three different source statuses. `requestReview`/
`reportPRCreated` both hardcode a single `ExpectedStatus`; this tool cannot. Per
stack.md §2 step 6 and confirmed by reading the CAS implementation: the correct pattern
is fetch-current-status-then-CAS-against-it —
`item := h.storage.GetBacklogItem(...)`, `precondition := &BacklogItemPrecondition{ExpectedStatus: item.Status, Note: reason}`,
then `CanTransitionBacklog(BacklogStatus(item.Status), targetStatus)` before issuing
`TransitionBacklogItemStatus`. This is the same shape `backlog-self-resolve`'s
architecture research independently derived for its own (different-target) transition
(§3.1/4.2 of that doc) — both projects converge on the same mechanical answer to "how do
you CAS-transition from a caller-supplied set of acceptable source statuses instead of
one hardcoded one," which is reassuring cross-validation even though the *destination*
of the transition differs between the two designs.

## 2. Integration points

### 2.1 MCP tool registration — `server/mcp/tools_backlog.go:920` (`registerBacklogTools`)

Mechanical, no surprises: `s.AddTool(mcpgo.NewTool("mark_duplicate", ...), h.markDuplicate)`
added to the existing flat list. Handler method added to `backlogHandlers`
(struct at `tools_backlog.go:91-110`). No new registration mechanism needed.

### 2.2 `session/domain/backlog.go` — enum + transition table + (optionally) a guard

- New `const BacklogStatusClosed BacklogStatus = "closed"` (or whatever name planning
  picks — see §5) at line ~23.
- New entries in `validTransitions` (line 331): add `BacklogStatusClosed: true` to the
  target maps of `BacklogStatusInProgress` (line 355), `BacklogStatusReview` (line 361),
  and `BacklogStatusPRPending` (line 369); add a new top-level key
  `BacklogStatusClosed: {BacklogStatusIdea: true}` mirroring `BacklogStatusArchived`'s
  entry (line 385-387) if a reopen escape hatch is wanted.
- If planning wants a mechanically-verified precondition (per §0's compensating-control
  recommendation), that check lives in the MCP handler itself, not `TransitionGuard` —
  per §1.3, `TransitionGuard` is not wired into the MCP-tool call path at all today, so
  adding a case there would be dead code unless `backlogHandlers` is also given a
  `WorkflowEngine` reference (a larger, out-of-scope architectural change).

### 2.3 Reconciliation sweeps — `session/backlog_lifecycle.go` (extends stack.md §5's list)

Re-grepped this pass; stack.md's inventory (lines 1264, 2063, 2545, 2618, 2799, 3432,
2997-3011) is confirmed current. **One additional site stack.md's grep missed**, found by
reading `selfHealStuck` in full:

- **`session/backlog_lifecycle.go:2987`** — `selfHealStuck`'s blanket terminal rule:
  ```go
  if row.ItemStatus == BacklogStatusDone || row.ItemStatus == BacklogStatusArchived {
      // Blanket terminal rule — an item that has truly finished has nothing
      // left needing operator attention, regardless of which reason its
      // stuck row is for.
      l.resolveStuckLogged(ctx, er, row.ItemID, row.Reason, "selfHealStuck/terminal")
      continue
  }
  ```
  This is the single highest-value site to update: it's the codebase's own existing
  concept of "terminal, stop caring about this item's stuck-rows" — the new `closed`
  status belongs in this `||` chain alongside `Done`/`Archived`, otherwise a
  closed-as-duplicate item with an open stuck row (e.g. it was stuck on
  `StuckReasonPRReadyUnmerged` moments before being marked duplicate) never
  self-resolves and lingers as a false-positive operator alert forever — a direct,
  concrete violation of AC4 ("never re-polled, flagged stuck, or auto-reopened") if
  missed.

Plan-phase should still do its own final grep of
`BacklogStatusInProgress\|BacklogStatusReview\|BacklogStatusPRPending\|BacklogStatusDone\|BacklogStatusArchived`
across the full 4000+-line file before implementation — two independent research passes
(stack.md and this one) have each found sites the other's single grep pass missed, which
is itself evidence the exhaustive-list-scattered-across-a-4000-line-file shape of this
code is the actual architectural risk here (see §4.2).

### 2.4 Frontend — a single authoritative mapping function, not six independent files

stack.md §6 lists six files with `pr_pending`-shaped exhaustive handling as candidates
needing updates, flagged as "not deeply explored." Read the two board/tracker files in
full this pass — the actual picture is more concentrated than six independent sites:

- **`web-app/src/components/backlog/detail/StageTracker.tsx:32-59`**
  (`deriveStageDisplay`) is the **single authoritative status→display mapping**. Both
  `BacklogBoard.tsx` (via `stageOf()`, `BacklogBoard.tsx:20-23`, itself a thin wrapper
  calling `deriveStageDisplay`) and `StageTracker.tsx` itself consume this one function.
  This is good news architecturally: fixing AC5's "renders meaningfully instead of
  unknown-status fallback" for both the board and the item-detail lifecycle tracker is
  **one switch-case addition in one function**, not six.
- Its existing `default:` branch (line 54-57) already has defensive behavior for
  unknown/future statuses: `return { activeStage: "idea", archived: true }` — i.e. an
  unhandled new status **does not crash**, but silently renders as a dimmed tracker with
  an **"Archived" ribbon label** (`StageTracker.tsx:107-108`,
  `aria-label="Lifecycle stage: Archived"` at line 74) — which is actively *misleading*
  for a `closed`/`duplicate` item (it was never archived), not just a generic "unknown"
  fallback. This is a concrete UX bug AC5 must close, not just a cosmetic gap: add an
  explicit `case "closed":` branch with its own (non-"Archived"-labeled) dimmed/ribbon
  treatment before this ships, or the closed-as-duplicate item's detail page will
  actively lie about why it stopped moving.
- `BacklogBoard.tsx`'s `COLUMNS` (line 38) is derived from `StageTracker.STAGE_ORDER`
  (5 stages) — `deriveStageDisplay` returning `archived: true` for a status already
  causes the board to render no column/card for it (same as today's `archived` items) —
  confirm during planning whether "closed" items should be fully hidden from the default
  board view (matching `archived`'s current behavior, simplest) or need their own
  visible column/filter (larger UI scope, likely unnecessary for a first version per the
  "prefer zero UI changes" bias `backlog-self-resolve`'s research also recommends for its
  own evidence-field question, §3.3 of that doc).
- The other four files stack.md flagged (`page.tsx`, `BacklogItemDetail.tsx`,
  `ActionsSection.tsx`, `PullRequestSection.tsx`/`VersionControlSection.tsx`) were not
  read in full this pass — still flagged as needing a planning-phase check, but they are
  very likely *consumers* of `item.status` for things like "which action buttons to show"
  rather than independent exhaustive-status-display logic; `ActionsSection.tsx` in
  particular should be checked for whether it needs to *hide* the `mark_duplicate`-driven
  actions (request review / report PR) once an item is `closed`, mirroring how it must
  already handle `done`/`archived`.

### 2.5 `get_backlog_item` role-specific guidance — `server/mcp/tools_backlog.go:190-217`

AC5 also calls out `get_backlog_item`'s guidance text. The `case "work":` block
(lines 205-216) is the one that needs updating — it currently walks the work-role agent
through AC completion → `request_review` → PASS/FAIL handling, with no mention that
"this item turned out to be moot" is also a valid outcome. Add a line here pointing the
agent at the new tool (mirroring how line 211 already documents `request_review` and
line 215 documents `report_pr_created`) — same file, same function, no new integration
point.

### 2.6 Feature registry — `docs/registry/features/backend/backlog/`

Per `.claude/rules/feature-registry.md`: every existing backlog MCP tool has its own
per-feature JSON at `docs/registry/features/backend/backlog/<kebab-case-tool-name>.json`
(confirmed directory listing — `archive-item.json`, `attach-session.json`, etc. follow
this convention; e.g. `report-pr-created` would be `report-pr-created.json`). A new
`mark-duplicate.json` (or matching whatever tool name planning picks) is required, plus
`make registry-generate`. requirements.md's own Notes-for-Downstream-Phases already
flags this — confirming here that the naming convention is kebab-case-of-the-tool-name
and the directory is `docs/registry/features/backend/backlog/`, not the flat top-level
`backend/` stack.md's phrasing might otherwise suggest.

## 3. Data flow

```
work session (linked to item, role=work)
  → MCP mark_duplicate(item_id, reason, reference_url?)
    → featureDisabledResult / callerSessionUUID auth gate      [boilerplate, copy from reportPRCreated]
    → arg parse + length caps (reason <= 2000, mirrors `message` cap in requestReview:360-362)
    → GetItemSessionBySessionAndItem(caller, item_id)          [link check — AC3 "unlinked" rejection]
      → session.ErrNotFound → ErrPermissionDenied
    → itemSession.Role != session.SessionRoleWork               [role check — AC3 "wrong role" rejection]
      → ErrPermissionDenied
    → item := GetBacklogItem(item_id)                           [read current status — NOT hardcoded]
    → idempotency check: item.Status == closed already? → success no-op
      (mirrors reportPRCreated:681-686's "already pr_pending with same PR" no-op)
    → session.CanTransitionBacklog(BacklogStatus(item.Status), closed)  [explicit — ent layer won't check this, §1.3]
      → false → ErrInvalidArgument ("cannot close item from status %q")
    → (optional, if planning keeps self-certified design + wants a compensating control:
       mechanically verify reference_url resolves to a real, merged/closed PR/issue,
       mirroring h.verifyPR at tools_backlog.go:707 — hard error if unverifiable, no soft-degrade)
    → precondition := &BacklogItemPrecondition{ExpectedStatus: item.Status, Note: reason (+ reference_url)}
    → h.storage.TransitionBacklogItemStatus(item_id, closed, precondition, session.TriggeredBySystem)
      → ent CAS: UPDATE ... WHERE id=? AND status=item.Status   [atomic — see §4]
        → affected==0 → ErrPreconditionFailed ("concurrent modification detected")
          → surfaced to caller as ErrInternalError with the actual current status;
            caller should re-fetch via get_backlog_item and decide, NOT blind-retry
            (see §4.3 — a blind retry loop here can livelock against the reconciler)
        → affected==1 → recordStatusEvent(from=item.Status, to=closed, note=reason)  [audit, automatic]
    → log.InfoLog "[mcp:mark_duplicate] ..." (matches [mcp:request_review] tag convention)
    → mcpgo.NewToolResultText("Item marked closed: <reason>")
```

If planning instead adopts `backlog-self-resolve`'s routed-through-`review` design, this
data flow collapses into that project's §3.2 (`report_duplicate` → `review` →
`submit_review_verdict` → `handleReviewSessionExited`) and no new terminal status exists
at all — this project's §2.2/§2.3/§2.4 frontend and reconciliation touch points would
then largely evaporate (nothing new to exclude from "active" sweeps, because nothing new
is terminal). This bifurcation is exactly why §0's conflict must be resolved before
either data flow is implemented.

## 4. Consistency requirements around the CAS mechanism

### 4.1 The CAS is already atomic — BUG-026 is fixed, not a live risk for a new caller

`TransitionBacklogItemStatus`'s doc comment (`ent_repository_backlog.go:847-868`) and
`docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md` (read in full)
confirm: prior to a 2026-07-20 fix, the precondition was checked via a separate `Get()`
before an unconditional `UpdateOneID().Save()` — a genuine TOCTOU race (root cause #1 of
a live incident that silently reopened a shipped item). **This is already fixed**: the
precondition is now folded directly into the `UPDATE ... WHERE` clause
(`ent_repository_backlog.go:883-897`), so the check-and-write is a single atomic SQL
statement — no gap for a concurrent writer to land in. A new `mark_duplicate` caller
inherits this fix for free; it does not need to (and should not attempt to) re-implement
any additional locking. `ErrPreconditionFailed` (`session/repository.go:12`, message
"concurrent modification detected") is the correct, expected, and already-safe signal
for "someone else moved this item since you last read it" — not a bug class this feature
needs to guard against further, just an error the new handler must handle *gracefully*
(§4.3), not treat as a system failure.

### 4.2 State-machine validity is a separate concern from concurrency safety — both required

Per §1.3/§2.2: the ent-level CAS only proves "the row was in status X when this UPDATE
committed" — it says nothing about whether `X → closed` was ever a *legal* edge. Both
checks (`CanTransitionBacklog` explicit call + CAS precondition) are required together;
neither alone is sufficient. This is the direct architectural lesson both this project's
research and `backlog-self-resolve`'s independently converged on (their §4.2, my §1.3) —
worth stating as a hard requirement for the plan phase rather than an implementation
detail, since it's easy to write a handler that "looks done" after wiring only the CAS
call (which is the part every existing tool already demonstrates) and miss that the
state-machine check has no existing call site to copy from in the MCP layer at all.

### 4.3 Failure handling: surface, don't blind-retry

If `TransitionBacklogItemStatus` returns `ErrPreconditionFailed` (item moved since the
handler's own `GetBacklogItem` read — e.g. the reconciler's `PRFixSpawner` or
`recoverDriftedPRItem` legitimately reopened/advanced the item in the small window
between this handler's read and its CAS write), the handler must **not** loop-retry
against a freshly re-read status automatically. Two reasons: (a) it risks a livelock
against a reconciliation sweep that runs on its own tick and could keep winning the race
indefinitely under contention, and (b) more importantly, if the item moved because a
*legitimate* concurrent process (e.g. `report_pr_created` from the very same session, or
a system reopen) changed what "closing this item" even means, blindly retrying with the
new status as the new `ExpectedStatus` could close an item out from under state the
caller never actually observed. Correct behavior: return a clear, actionable
`ErrInternalError`/`ErrConflict`-shaped message telling the agent to call
`get_backlog_item` again and decide whether closing is still appropriate — the same
"fail closed, let the caller re-decide" posture `requestReview`'s existing error message
already models (`tools_backlog.go:416-417`: `"transition to %s failed: %v"`, surfaced
verbatim to the caller today).

### 4.4 Audit trail consistency

`recordStatusEvent` (`ent_repository_backlog.go:938`) fires unconditionally and
atomically as part of the same `TransitionBacklogItemStatus` call — no separate audit
write needed or should be added (a second write reintroduces exactly the kind of
partial-failure gap BUG-026's fix eliminated for the primary transition). The only
requirement is *content*: `reason`/`reference_url` must be packed into
`BacklogItemPrecondition.Note` **before** the transition call (as the data flow in §3
shows), not written in a follow-up call, so a crash between transition and evidence-write
can never leave a `closed` item with no audit-visible reason — directly satisfying AC1's
"recording the reason/reference so operators can see why an item closed without a merge."

### 4.5 `TriggeredBy` provenance — unresolved, matches both prior research docs

Only `TriggeredByUser`/`TriggeredBySystem` exist (`session/backlog.go:91-94` per
stack.md/backlog-self-resolve's independent confirmation). Whether a work-session-
initiated `mark_duplicate` call should be recorded as `TriggeredBySystem` (current
convention for all MCP-tool-driven transitions, including `requestReview`) or a new
`TriggeredByAgent` value is a plan-phase decision with no architectural blocker either
way.

## 5. Event-Command-Policy table (EventStorming grammar)

Actors: **Work Session** (agent process with role=work, linked to the item), **Review
Session** (role=review), **Reconciler** (`BacklogLifecycleListener`'s periodic sweeps,
`session/backlog_lifecycle.go`), **Human Operator** (via web UI or direct DB/API access).

| # | Actor | Command | Policy (when this fires) | Event (past tense) |
|---|---|---|---|---|
| 1 | Work Session | — (internal reasoning, not a system command) | Agent finds a superseding merged PR/issue during its own work | **Duplicate Discovered** |
| 2 | Work Session | `MarkDuplicate(item_id, reason, reference_url)` | Policy: *"When Duplicate Discovered, if caller is linked to item AND caller role = work AND `CanTransitionBacklog(current_status, closed)` is true, issue `MarkDuplicate` with CAS precondition = currently-observed status"* | **Item Marked Closed** (success) *or* **Mark-Duplicate Rejected** (permission/link/transition-invalid failure) |
| 3 | *(system, inside `TransitionBacklogItemStatus`)* | `TransitionBacklogItemStatus(item_id, closed, precondition)` | Policy: *"When Item Marked Closed command is accepted, the CAS UPDATE and the audit-event append happen atomically — no partial state is observable externally"* | **Item Closed** (row updated) → **Closure Audited** (`BacklogStatusEvent` row appended, same transaction path) |
| 3a | *(system)* | — | Policy: *"When the CAS UPDATE affects 0 rows (another writer won the race since this handler's read), fail closed"* | **Mark-Duplicate Rejected: Concurrent Modification** (`ErrPreconditionFailed`, "concurrent modification detected") — surfaced to the work session, not silently retried (§4.3) |
| 4 | Reconciler | *(read-only sweep — `FindOpenStuckStates`, `FindDriftedPRItems`, `FindReviewItemsWithUnprocessedVerdict`, etc.)* | Policy: *"When any stuck-detector/reconciliation sweep runs (`backlog_lifecycle.go` lines 1264/2063/2545/2618/2799/2987/3432, per stack.md + §2.3 of this doc), treat `closed` as terminal — same as `done`/`archived` — and exclude it from every 'active work' query"* | **Reconciler Skipped Closed Item** (no action taken, item never re-polled/flagged — this is the *correct*, silent, expected outcome AC4 requires) |
| 4a | Reconciler | `ResolveStuck(item_id, reason)` | Policy: *"When `selfHealStuck` (`backlog_lifecycle.go:2980-2987`) observes an item with an open stuck row has reached a terminal status (`done`/`archived`/**`closed`**), auto-resolve the stuck row regardless of which reason it was open for"* | **Stuck Row Auto-Resolved** — closes the loop on any stuck-row an item accumulated *before* being marked duplicate, preventing a false-positive operator alert (§2.3's flagged gap — this row is what breaks if `closed` is missed from the `\|\|` chain) |
| 5 | Reconciler | *(none — explicit non-action)* | Policy: *"`PRFixSpawner`/`recoverDriftedPRItem`-style reopen/CAS-transition sweeps must never target a `closed` item — their own preconditions (`ExpectedStatus: pr_pending`, etc.) already exclude it by construction once the item's status is no longer `pr_pending`, but this is worth an explicit regression test, not just an implicit consequence of the CAS precondition"* | **Reconciler Correctly Ignored Closed Item** (no-op by construction, verified by test not just code-reading) |
| 6 | Human Operator | `ReopenClosedItem(item_id)` *(only if planning adds the `closed → idea` escape hatch, §2.2)* | Policy: *"When an operator determines a `closed` item was mis-closed (bad duplicate call, reference turned out to be wrong), allow a manual reopen to `idea`, mirroring `archived`'s only outbound edge — this is a deliberately narrow, human-only escape hatch, not exposed to any MCP tool a work/review session can call"* | **Item Reopened** |
| 7 | Review Session *(only under `backlog-self-resolve`'s design, §0)* | `SubmitReviewVerdict(item_id, verdict, evidence)` | Policy: *"If planning adopts the routed-through-review design instead, the reviewer — not the work session — is the actual authority that converts 'Duplicate Discovered' evidence into a terminal outcome; `handleReviewSessionExited` (`backlog_lifecycle.go:954`) remains the sole terminal-transition writer, unchanged"* | **Duplicate Claim Verified** (PASS-equivalent) *or* **Duplicate Claim Rejected** (FAIL-equivalent, triggers existing `AutoReopenAfterFailedReview` rework path) |

Row 7 is included to make the two competing designs (§0) concretely comparable in
EventStorming terms: the self-certified design collapses events 1→2→3 into a single
work-session-driven command with no reviewer in the loop at all; the routed-through-
review design instead makes event 1 ("Duplicate Discovered") feed a *different* command
(`report_duplicate` → transitions to `review`, not `closed`) and defers the actual
terminal event to row 7, driven by a different actor entirely. This is the clearest way
to see that these are not two implementations of the same event model — they are two
different event models for the same real-world trigger.

## 6. Enum value vs. generic `closed` + `close_reason` field

requirements.md's Notes-for-Downstream-Phases flags this as open. Weighing against the
codebase's existing pattern and this feature's actual shape:

**The existing pattern is enum-only, with no precedent for a reason-as-separate-field
design anywhere in `BacklogStatus`.** Every other "why did this transition happen"
signal in this codebase already has a durable home that is *not* a new column on
`BacklogItem`:
- `BacklogStatusEvent.note` — exactly the free-text "why" field every other transition in
  this file uses (`precondition.Note`, e.g. `AutoReopenAfterFailedReview`'s rollback note,
  `backlog_lifecycle.go:3333`). It is per-transition, timestamped, and already queryable.
- `StuckReason` (`session/domain/backlog.go:153` area, per stack.md) is the closest
  existing analog to a "reason enum" — but it lives on a *separate* `stuck_states` table,
  not as a field on `BacklogItem` itself, because a stuck reason is a fact about *why an
  item is stuck right now*, not a durable property of the item's identity.

Given that shape, **`close_reason` as a durable `BacklogItem` column is over-engineering
for what this feature actually needs**: the reason/reference is a one-time fact about
*how the item reached its terminal state*, structurally identical to every other
transition's `Note`, not a queryable-forever attribute like `priority` or `category` that
callers filter/sort on repeatedly. Recommend:

- **A single new terminal `BacklogStatus` value** (e.g. `BacklogStatusClosed = "closed"`)
  — not one enum value per closure reason (`duplicate`/`superseded`/`obsolete`/`wont_fix`
  as four separate `BacklogStatus` values would each need their own entries in
  `validTransitions`, their own case in `deriveStageDisplay`, their own reconciler
  exclusion — quadrupling every touch point in §2 for a distinction that has no
  behavioral consequence anywhere in the reconciler/UI, only in reporting).
- **The reason/reference carried in `BacklogItemPrecondition.Note`** (already durable,
  already the established mechanism, zero schema change — confirmed zero-migration per
  stack.md §3) for v1, with an *optional* future `close_reason` enum column only if a
  concrete need emerges to filter/report on closure reason at the `BacklogItem` level
  across many items (e.g. a dashboard of "how many items closed as duplicate vs.
  wont_fix this month") — that's a reporting/analytics requirement, not something this
  feature's stated acceptance criteria (AC1-AC6) actually call for. This mirrors
  `backlog-self-resolve`'s own §3.3 recommendation for its evidence field ("prefer the
  option that requires zero UI changes for v1... over introducing a wholly new render
  path") — independent convergence on the same minimal-footprint bias from a different
  angle of the same problem.
- If a `close_reason` *category* (not full free-text) turns out to matter for UI badge
  styling (e.g. a duplicate gets a different badge color than wont_fix), that's still
  cheaply achievable by parsing a short structured prefix out of `Note`
  (e.g. `"[duplicate] superseded by PR #281"`) rather than adding a schema column — worth
  flagging as a planning-phase option before reaching for a migration.

**Recommendation: single `BacklogStatusClosed` enum value + reason/reference in the
existing `Note` field. No new `BacklogItem` column, no migration.** This is the option
requiring the fewest new touch points across §2's integration-point inventory and is
consistent with every existing "why did this transition happen" mechanism already in the
codebase.

## Summary of answers to the assigned research questions

1. **Patterns**: Guarded State Machine extension (§1.1); Command Pattern / role-scoped
   CQRS-lite (§1.2); ent-CAS-does-not-validate-state-machine (§1.3, load-bearing —
   `TransitionGuard` is not wired into the MCP layer at all); optimistic-CAS-against-
   current-status rather than a hardcoded expectation (§1.4).
2. **Integration points**: MCP registration (§2.1, mechanical); domain enum + transition
   table (§2.2); reconciliation sweeps — stack.md's list plus one additional site found
   this pass, `selfHealStuck`'s blanket terminal check at `backlog_lifecycle.go:2987`
   (§2.3); frontend — one authoritative function, `StageTracker.tsx`'s
   `deriveStageDisplay` (§2.4), whose `default` branch currently mislabels any new status
   as "Archived," a real AC5 bug to close explicitly; `get_backlog_item`'s work-role
   guidance block at `tools_backlog.go:205-216` (§2.5); feature registry at
   `docs/registry/features/backend/backlog/<kebab-case>.json` (§2.6).
3. **Data flow / CAS consistency**: the CAS mechanism itself is already atomic and safe
   (BUG-026 fixed, §4.1) — the "concurrent modification detected" error is an expected,
   correctly-functioning signal, not a live defect this feature needs to work around;
   what the new handler must add is (a) an explicit `CanTransitionBacklog` call the ent
   layer will never perform for it (§1.3/§4.2), and (b) fail-closed, no-auto-retry error
   handling on `ErrPreconditionFailed` to avoid livelocking against the reconciler
   (§4.3).
4. **Event-Command-Policy table**: §5, covering both the self-certified-terminal design
   this project's requirements.md specifies and the routed-through-review design
   `backlog-self-resolve` independently proposes for the same underlying trigger (row 7).
5. **Enum vs. `close_reason` field**: single new terminal enum value, reason/reference
   carried in the existing `BacklogItemPrecondition.Note` mechanism — no new column, no
   migration (§6).
6. **Unassigned but load-bearing**: §0's finding that `project_plans/backlog-self-resolve/`
   is independently researching the same real-world problem with a conflicting
   architectural premise (self-certified terminal status vs. routed-through-review) needs
   resolution before planning proceeds on either project in isolation.
