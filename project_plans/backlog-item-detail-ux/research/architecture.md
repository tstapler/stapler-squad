# Architecture Research: backlog-item-detail-ux

Agent 3 (Architecture). Findings for the 5 research questions plus an
Event-Command-Policy table for the item lifecycle.

## 1. Backend session output access — does the read-only viewer need a new RPC?

**No new RPC is needed for the sessions named in scope (triage, headless-review,
review-blocked).** They are not backed by a `session.Instance`/tmux/PTY at all (see
§2), so there is no "output stream" to read for them — the concept doesn't apply.
`ReadSessionOutput`-equivalent RPCs in this codebase (`StreamTerminal`,
`server/services/session_service.go:2094`; `GetLogs`, line 2591) operate on a live
`session.Instance` looked up via `reviewQueuePoller.FindInstance` / storage — they
have no special guard excluding `Hidden` sessions (`GetSession`, line 1121, does not
check `Hidden` at all; `ListSessions`, line 1059-1100, only filters `Hidden` out of
*list* results unless `IncludeHidden` is set — a single-session fetch by ID is never
filtered). So *if* a real hidden `Instance` ever needed a read-only viewer, `GetSession`
+ `GetLogs`/scrollback already work today without new plumbing — but grepping every
backlog spawn call site (`server/services/backlog_service_triage.go`,
`backlog_service_lifecycle.go`) turned up **zero** call sites that pass `hidden: true`
to `CreateDirectorySession`/`CreateWorktreeSession`. Every real Instance-backed backlog
session (work sessions, interactive review sessions) is a normal, non-hidden, already-
clickable session. The "hidden=true guard" rabbit hole flagged in requirements.md does
not materialize in this codebase — there is no real hidden session type in play here.

**What the read-only viewer actually needs:** the diagnostic content (triage
result / review verdict summary) already arrives over the wire today in
`BacklogItem.item_sessions[].{triage_result, review_verdict}`
(`proto/session/v1/backlog.proto:66-84`) — see §2/§3. Building the viewer is a
**frontend-only change**: render fields already present in the `GetBacklogItem`
response that are currently fetched but never displayed for these rows.

## 2. The "headless-"/"review-blocked-" synthetic session ID question

Confirmed synthetic — **none of these represent a real session in the session store.**
They are DB-only rows in the `item_sessions` table (Go type `session.ItemSessionData` /
`ItemSessionSummary`, `session/storage_backlog.go:21`, `session/repository.go:283`),
created directly via `storage.CreateItemSession(...)` with a manufactured
`SessionUUID`, never via `sessionCreator.Create{Directory,Worktree}Session` (which is
what mints a real `session.Instance` with a tmux pane/PTY).

| Prefix | Created at | Real Instance? | What it represents |
|---|---|---|---|
| `headless-triage-` | `server/services/backlog_service_triage.go:1566` (const `headlessTriageUUIDPrefix`, line 204) | No | A headless LLM call via `session.headless.Pool` (`Call`/`CallBlocking`, `session/headless/caller.go`) — no tmux/PTY ever exists. Result lands in `ItemSession.TriageResult` (raw JSON) + `TriageResultSummary`. |
| `headless-re-review-` | `backlog_service_triage.go:1995`, also via `session.RecordDegradedReviewVerdict` (lines 1848/1932/1965) | No | Same headless-pool mechanism, for automated re-review. Result lands in `ItemSession.ReviewVerdict.Summary`. |
| `review-blocked-` | `session/review_gate.go:229` | No | A **pre-flight guardrail** verdict recorded directly (not even a headless LLM call) when `RunPreGateSecurityCheck` blocks a review before it starts. Terminal FAIL verdict with an explanatory `summary`. |
| `diff-error-` | `session/review_gate.go:191` | No | Same guardrail pattern, for "diff could not be computed against the recorded base commit." |
| `manual-review-` | `server/services/backlog_service_lifecycle.go:857` | No | Synthetic verdict for a user-submitted manual review. |

All five are "no session ever actually ran (or a session ran headlessly with no
terminal)" markers, not placeholders for a real session that could be opened. **The
"viewing" affordance the requirements ask for is a diagnostic/summary display, not a
terminal/output stream** — and the data for that display (`TriageResult.summary` +
`.suggestions`/`.tasks`, `ReviewVerdict.summary` + `.per_criterion`) is *already*
present on every `ItemSession` the frontend receives
(`proto/session/v1/backlog.proto:66-84`, fields 11-12: `review_verdict`,
`triage_result`). Confirmed present in the frontend response type consumed by
`BacklogItemDetail.tsx` (`item.linkedSessions[]`, gated at line 1333).

Cross-check: `role === "triage"` is *always* synthetic/headless — grepping every
`SessionRole: session.SessionRoleTriage` assignment across the backend finds exactly
one creation site (`backlog_service_triage.go:1574`, headless-only). `role === "review"`
is **not** always synthetic — `backlog_service_triage.go:2088` creates a review
`ItemSession` with a real `inst.UUID` for interactive re-review (already clickable
today, correctly excluded by the current `s.role === "triage" || sessionId.startsWith("headless-") || sessionId.startsWith("review-blocked-")`
guard at line 1333 since it has neither prefix).

**Architecture implication:** no backend work is required to make these sessions
"inspectable." The fix is entirely in `BacklogItemDetail.tsx`'s Sessions section:
route synthetic-ID rows to a new read-only summary display (reusing `TriageResult`/
`ReviewVerdict` rendering — see §5) instead of an inert `<span>`, and route real-UUID
rows (unchanged) to the existing `/?session=` terminal link.

## 3. Lifecycle/status data flow

`BacklogItemData` (`session/repository.go:343-429`) → proto `BacklogItem`
(`proto/session/v1/backlog.proto:112+`) → frontend `BacklogItem` type consumed by
`BacklogItemDetail.tsx`. Status itself (`idea/refining/ready/queued/in_progress/
review/pr_pending/done/archived`) is a single `status` string field with no
"derived stuck state" computed as part of this response.

**`useStuckBacklogItems.ts` is not reusable in-place for the detail view** — it's a
polled hook (`web-app/src/lib/hooks/useStuckBacklogItems.ts`) wrapping
`ListStuckBacklogItems`, an RPC with **no per-item filter** (`ListStuckBacklogItemsRequest`
is empty `{}` — fetches every open stuck row globally, intended for the `/unfinished`
page, explicitly out of scope for this project). Reuse pattern for the detail view's
"what's it waiting on" summary: call this same hook/RPC (cheap — single-user tool,
small N) and do `items.find(i => i.itemId === item.id)` client-side to check if *this*
item currently has an open stuck row, surfacing `StuckBacklogItem.reason` +
`.context` (`proto/session/v1/backlog.proto:876-897`) as the at-a-glance blocking
reason when present. This avoids duplicating the backend's stuck-detection logic in
the frontend (per requirements' explicit "avoid inventing a UI-only status model"
constraint) — the canonical `StuckReason` enum and detection already exist server-side
in `session/backlog_lifecycle.go`'s `reconcile*` family (e.g. `reconcileStaleWorkSessions`,
line 1777). When no open stuck row exists, the summary falls back to plain
`status` + `getStatusLabel()` (see §4) — no "waiting on" text.

**Status duplication confirmed** — `item.status` (or values derived from it) is
rendered independently at: header badge (`BacklogItemDetail.tsx:710-713`), ~15
separate `item.status === "..."` conditionals gating which of the 12+ sections render
(lines 647-1252), the Sessions list's per-row `isOrphan` inference (line 1324), and
the Workflow/status-history timeline (line 1475+, from `item.statusEvents`). These are
not all "the same info shown twice" — most are legitimate section-visibility gates —
but the header badge and the Workflow timeline's *latest* entry are redundant with
each other and are the consolidation target the requirements describe.

## 4. Board card integration point

Two card-rendering components already share a status-label utility:

- `web-app/src/components/backlog/BacklogItemCard.tsx` (151 lines) — used by
  `BacklogBoard.tsx` (114 lines) for `/backlog/board`. Derives a per-status
  `ActionSpec` (label/action/disabled) via a `switch (item.status)` (lines 24-51),
  independent of the shared label map.
- `web-app/src/components/backlog/BacklogItemBadge.tsx` (54 lines) — a small
  presentational badge (`STATUS_CLASS` map + `getStatusLabel`), already imports the
  **shared** `web-app/src/lib/backlog/status.ts` (`STATUS_LABELS` / `getStatusLabel`),
  which `BacklogItemDetail.tsx`'s header badge also already consumes (line 711-713).

**Status vocabulary is already consistent** between detail and cards via
`lib/backlog/status.ts` — that part of the requirement is largely satisfied today.
What's *missing* on cards is the "waiting on X" signal: `BacklogItemCard.tsx` has no
equivalent of the stuck-reason lookup proposed in §3. Scoping "board consistency"
therefore means: (a) confirm `BacklogItemBadge`/`BacklogItemCard` and the new detail
lifecycle summary literally share the same label-mapping and (ideally) the same
stuck-badge component, and (b) surface a compact stuck indicator on
`BacklogItemCard.tsx` sourced from the same `useStuckBacklogItems` data the detail
view now also reads — a shared small component (e.g. `StuckIndicatorBadge`) consumed
by both, not two independent implementations.

## 5. Component decomposition approach

The `backlog/` directory already establishes the target pattern — `BacklogItemDetail.tsx`
is the outlier (1577 lines), not the norm. Existing precedent for splitting a large
panel into focused sub-components already used *inside* `BacklogItemDetail.tsx`:

- `GateVerdictBox.tsx` (491 lines) — self-contained review-verdict display +
  override actions, already renders `ReviewVerdict` (summary, per-criterion
  outcomes). **Directly reusable** (in a read-only prop-gated mode, or by extracting
  its pure display half) for the new synthetic-review-session viewer in §2.
- `TriageReviewPanel.tsx` (326 lines) — self-contained triage-result display +
  accept/undo actions. **Directly reusable** for the synthetic-triage-session viewer.
- `SessionMonitor.tsx` (219 lines), `TriageDiffSection.tsx` (100 lines),
  `AcCriteriaList.tsx` (79 lines) — smaller focused sections already extracted from
  what would otherwise be inline JSX in the detail component.
- `BacklogItemBadge.tsx` (54 lines) — proof this codebase already extracts a
  single-purpose status-badge component rather than inlining badge markup per call
  site (relevant precedent for the header status summary + card consistency in §4).

**No collapsible/accordion/disclosure primitive exists anywhere in `web-app/src/`
today** (`grep -rl "Collapsible|Accordion|Disclosure|<details"` across
`web-app/src/components/` returns nothing). This must be built new as a shared
primitive (e.g. `web-app/src/components/ui/CollapsibleSection.tsx` +
`.css.ts`, per `.claude/rules/css-architecture.md` — no CSS Modules) rather than
inlined per-section, since progressive disclosure needs the *same* expand/collapse
behavior (persisted? per-section default state?) across ~8+ secondary sections.

**Recommended decomposition:** keep `BacklogItemDetail.tsx` as the orchestrating
container (data loading, mutation handlers — it already has these, e.g. `loadItem`,
the manual-review verdict form state) but extract each of its 12+ sections into
sibling components under a new `web-app/src/components/backlog/detail/` subdirectory,
each wrapped by the new shared `CollapsibleSection` primitive, mirroring how
`GateVerdictBox`/`TriageReviewPanel`/`SessionMonitor` are already extracted and
passed callback props rather than owning their own data-fetching. `BacklogItemPanel.tsx`
(154 lines, the drawer/panel host that already polls `getBacklogItem` and owns
open/close persisted-in-localStorage state) is the existing panel/drawer host and
should NOT be touched structurally — requirements confirm the panel mechanism itself
is out of scope, only its internal content's IA changes. Per the CSS architecture
rule's Page Scroll Convention, the panel's scrolling root already needs `height: 100%`
+ `overflowY: auto` — collapsing sections changes content height dynamically, so this
convention must be re-verified, not assumed, once sections collapse/expand.

## Event-Command-Policy table (EventStorming grammar)

Scoped to lifecycle events the redesigned detail view and board cards need to reflect
— not a full backend re-model (workflow logic changes are out of scope).

| Event | Triggering Command | Policy (what reacts) | UI-relevant data produced |
|---|---|---|---|
| `TriageRequested` | `TriggerTriage` RPC (user or auto) | Backend enqueues headless triage call, creates `headless-triage-*` `ItemSession` | `item.triageStatus = "running"` |
| `TriageCompleted` | (headless pool callback, internal) | Persists `TriageResult` on the `ItemSession`; advances `idea → ready` on success | `ItemSession.triage_result.{summary,suggestions,tasks}` — **currently rendered nowhere for the synthetic session row itself** (only inline in the live `TriageReviewPanel` at trigger time) |
| `TriagePersistFailed` | (internal) | `notifyTriagePersistFailure` — item stays `idea`, notification fired | Notification only; no item-session field change |
| `WorkSessionSpawned` | `SpawnSessionFromItem` RPC | Creates real `Instance` + `ItemSession{role: work}`; item → `in_progress` | Real session, already clickable |
| `ReviewGateTriggered` | Internal, on work-session completion signal | `spawnReviewGate` — either spawns a **real** interactive review `Instance` or, if a pre-flight guardrail fires, records a **synthetic terminal verdict** (`review-blocked-*`/`diff-error-*`) without spawning anything | Real session (clickable today) OR synthetic `ItemSession.review_verdict.summary` (**currently invisible to the user** — this is the core gap) |
| `ReviewVerdictRecorded` | (review session completion, headless re-review, or guardrail) | PASS → item advances toward `done`/PR flow; FAIL → `AutoReopenAfterFailedReview` (bounded by rework cap) | `ItemSession.review_verdict.{overall_outcome, summary, per_criterion}` |
| `ReworkCapHit` | (internal, Nth consecutive FAIL) | `notifyReworkCapHit` — item stays in `review` for human inspection, auto-reopen loop halts | Notification + item stuck in `review` with no further automated action — this is exactly the "why is it stuck" case the read-only viewer must explain |
| `ItemMarkedStuck` | Reconcile tick (`reconcileStaleWorkSessions` et al., `session/backlog_lifecycle.go`) | Durable `BacklogStuckState` row created; notify-once; `/unfinished` page + remediation loop pick it up | `StuckBacklogItem{reason, context, first_detected_at}` via `ListStuckBacklogItems` — the data source for the detail view's proposed "waiting on X" summary (§3) |
| `ItemArchived`/`ItemShipped` | User action / auto-PR flow | Terminal states | `status = done`/`archived` + `ShippedCheckConclusion` snapshot fields already on `BacklogItemData` |

**Key takeaway for the UI redesign:** the single biggest "why is this stuck"
explanatory event — `ReviewGateTriggered` resolving to a synthetic guardrail verdict,
or `ReworkCapHit` — already produces a human-readable `summary` string server-side.
The detail-view redesign's job is almost entirely *surfacing already-computed
backend text* (verdict summaries, triage summaries, stuck reasons) that today is
fetched over the wire and then dropped on the floor by the frontend, not computing
new derived state.
