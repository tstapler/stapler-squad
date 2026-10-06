# UX Design: context-compression — "Restart with Compressed Handoff Summary"

Phase 3.5 design artifact, built on `research/ux.md` (Phase 2) and
`implementation/plan.md` (Phase 3, "Ready for implementation"). Scope is the
reframed feature per ADR-001: **a session-detail event/action** — generate an
async handoff summary of a degrading session, then restart into a fresh
session seeded with that summary as its opening prompt. This is **not** a
live in-conversation compression indicator; that direction was rejected
(`research/build-vs-buy.md` Option 1 — the controller cannot splice/remove
turns from a CLI subprocess it only supervises).

Read first, treated as settled precedent and not re-derived:
- `project_plans/context-compression/research/ux.md` — establishes: no
  second card-row badge; no reusable session-timeline component exists yet;
  build on `WorkflowHistorySection.tsx` (list pattern) + `CheckpointList.tsx`
  (row shape); `role="listitem"` not `role="status"` for historical rows.
- `project_plans/context-health-monitoring/design/ux.md` — owns the
  `ContextHealthBadge` (session-card ⚠/✖ pill) that will (once
  `context-health-monitoring` ships and Phase 4 of this plan unblocks)
  auto-dispatch generation on a RED transition. This document does not
  redesign that badge — see Surface 1.

## Surfaces identified

| # | Surface | Plan reference | Sub-states covered |
|---|---|---|---|
| 1 | Trigger context: relationship to the (future) `ContextHealth` RED badge | Phase 4 (blocked); today, manual only | n/a — pointer only, no new UI |
| 2 | `HandoffSummarySection` — Info tab collapsible history list | Epic 3.3 | empty, generating, ready, error, feature-disabled |
| 3 | `RestartWithSummaryButton` — embedded action | Epic 3.2 | idle/trigger, generating, ready-to-restart (+ preview), creating-session, restart-failed |
| 4 | New session creation — prompt pre-fill + lineage indicator | Epic 2.3, Domain Glossary `restarted_from_session_id` | silent prefill (no user-visible step), "Restarted from" link in the new session's Info tab |

Four surfaces are in scope, matching the task brief's five bullet points —
the RED-badge/notification bullet collapses into Surface 1 as a pointer
(no redesign, since `context-health-monitoring/design/ux.md` already fully
specs the badge itself) and "loading/generating state" + "error state" are
covered as sub-states of Surfaces 2 and 3 rather than standalone surfaces,
since they only exist attached to the button/list, never independently.

---

## Surface 1: Trigger context — relationship to the RED badge

No new UI. Per the plan's Unresolved Questions and Phase 4 gating, **today
(Phase 3) there is no automatic link between the `ContextHealthBadge` and
this feature at all** — a user who sees a red "Context Needs Attention" pill
on a session card must independently navigate to that session's **Info**
tab to find "Restart with summary." Phase 4 (blocked on
`context-health-monitoring` shipping) will auto-dispatch generation on the
RED transition so the summary is pre-warmed by the time the user looks, but
even then the plan's own Pattern Decisions table rejects a fully-automatic
restart ("Auto-restarting a session out from under a user with no
confirmation is a destructive, no-rollback action") — the badge will never
itself gain an `onClick` (confirmed against `context-health-monitoring`'s
own plan: `ContextHealthBadge`'s only props are `health`, `reason`,
`isPaused`, no navigation).

**Gap found**: nothing in either plan currently threads a "this session has
a handoff summary ready" affordance back onto the badge or the session card
— a user has to remember, on their own, that seeing red means "go check the
Info tab." Flagging for Phase 4 planning, not a blocker for Phase 3:
Phase 4's pre-warmed `READY` row is wasted UX value if the only way to
discover it is still "open Info tab and check." A lightweight fix (not
scoped to this plan, worth a follow-up story) would be the badge's own
tooltip text gaining a trailing clause once a `READY` handoff row exists,
e.g. `"Context health: Context Needs Attention — <reason> · handoff summary ready"`.

---

## Surface 2: `HandoffSummarySection` — Info tab collapsible history list

### Wireframe — mount point (Info tab, below the existing key/value grid)

```
┌─ Info tab ──────────────────────────────────────────────────────────┐
│ Instance ID: a1b2c3...            📋   Status: Active                │
│ Session Type: Directory           Created: ...    Updated: ...       │
│ Branch: main                      Working Directory: /home/...       │
│ ...(existing key/value grid, unchanged)...                           │
│                                                                        │
│ ▾ Handoff Summary                                    (CollapsibleSection,
│ ┌────────────────────────────────────────────────────────────────┐  │  default
│ │ role="list" aria-label="Handoff summary history"                │  │  collapsed)
│ │  ...row per state, see below...                                 │  │
│ └────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

### Row wireframes, one per `HandoffSummaryStatus`

```
EMPTY (no row exists yet — status has no ent row, common case)
  "No handoff summary generated for this session."
  [Restart with summary]                          ← RestartWithSummaryButton, idle state

GENERATING
  role="listitem"
  ⟳ Generating handoff summary…                    relative time: "started 4s ago"
  (button area shows the same in-progress button — see Surface 3)

READY
  role="listitem"
  ✓ Ready to restart              12 turns summarized   ready 2m ago  ← pill, CheckpointList-style
  "Active task: Fix the flaky TestFoo assertion and re-run make test"  ← denormalized `active_task`
  ▸ Preview full handoff text                                          ← <details>/<summary>, collapsed
  [Start new session from this summary]            ← RestartWithSummaryButton, ready state

ERROR
  role="listitem"
  ⚠ Failed to generate handoff summary              last attempt: 2m ago
  "Failed while generating the handoff summary."     ← plain-language stage sentence
  ▸ Details                                          ← <details>, raw error_message
  [Try again]

FEATURE DISABLED (config.handoff_summary.enabled == false)
  "Restart-with-summary is disabled for this workspace."   ← distinct from the EMPTY string,
                                                              see Gap below
  (no button rendered)
```

Icon choice: `⟳` (in-progress), `✓` (success), `⚠` (failure) — deliberately
**reused**, not novel, from the repo's own existing "async job status"
vocabulary already used in this exact structural role by
`SessionSummaryPanel.tsx` (`"⟳ Generating…"`, `"⟳ Regenerating…"`,
`"✓ Ready · generated ..."`, `"⚠ Couldn't finish generating this
summary."`). This is a different semantic slot than
`context-health-monitoring`'s `⚠`/`✖` badge (a *session-card* live-state
pill) — no confusion risk because the two never appear in the same visual
context (card row vs. Info-tab list), and `research/ux.md` §3's "shape, not
just color" concern was written for two badges competing in the *same* slot,
which doesn't apply here.

### Interaction flow — expanding the section, opening a row's preview

```
User action                              System response
──────────────────────────────────────   ─────────────────────────────────────
1. User opens session detail, clicks     CollapsibleSection expands (collapsed
   Info tab, clicks "Handoff Summary"    by default per plan Task 3.3.1a,
                                          matching WorkflowHistorySection).
2. (READY row only) user clicks          Native <details> toggles open —
   "Preview full handoff text"           no JS state needed, matches
                                          SessionSummaryPanel's errorDetails
                                          pattern exactly. Full REFERENCE-ONLY-
                                          prefixed text renders in a
                                          monospace/pre block.
3. User clicks elsewhere / collapses     No side effect on session or summary
                                          state — pure disclosure.
```

### Error / edge-case handling

| Case | Rendered state | Recovery action |
|---|---|---|
| No row yet (common case) | Empty-state text + idle button | Click "Restart with summary" |
| Row `GENERATING`, tab reopened later (browser refresh) | Polling resumes via `useHandoffSummary`'s mount-time fetch; row shows `⟳ Generating…` again, not stuck | None needed — self-resolves within one poll interval |
| Row `ERROR`, stage `transcript` (conversation file unreadable) | `⚠` row, "Couldn't read this session's conversation history." | "Try again" — re-dispatches `trigger()` |
| Row `ERROR`, stage `generation` (pool/LLM call failed) | `⚠` row, "Failed while generating the handoff summary." | "Try again" |
| Row `ERROR`, unrecognized/future stage string | `⚠` row, generic fallback: "Something went wrong while generating this summary." (mirrors `SessionSummaryPanel`'s `ERROR_STAGE_COPY` fallback exactly — never a raw stage string or stack trace as the primary line) | "Try again" |
| Feature disabled, no row exists | Distinct disabled-state text (see Gap below), no button | None — informational only |
| Feature disabled, but a `READY`/`ERROR` row already exists from before it was disabled | Row still renders (read-only) — `GetHandoffSummary` is not gated by `Enabled`, only `Trigger` is (plan Task 2.2.1a) — but the row's action button is suppressed (`RestartWithSummaryButton` returns `null` per its 3rd AC) | None — the row becomes informational-only; "Preview full handoff text" still works since that's local disclosure, not an RPC |
| Zero-length/degenerate transcript (session had almost no turns before restart was requested) | Generation still proceeds per plan Task 1.4.1c (`"(nothing to summarize — conversation was short)"` placeholder) — row still reaches `READY`, not blocked | None — informational, not an error path |

**Gap found — the disabled-state string is unspecified by the plan.**
Task 3.3.1a's acceptance criteria only define the empty-state string for "no
row exists," and Story 3.2.1's 3rd AC only says the button "renders `null`"
when disabled — it does not say what (if anything) the *section* should say
instead. Rendering the same "No handoff summary generated for this session."
text in both the ordinary-empty and disabled cases is a **silent dead end**:
a user who has never seen this feature work has no way to distinguish "you
haven't tried yet" from "this is turned off and clicking anything won't
help." Recommend `HandoffSummarySection` accept an `enabled: boolean` prop
(sourced the same way `RestartWithSummaryButton` gets it — plan flags this
as itself unresolved: "confirm during implementation whether
`HandoffSummaryConfig.Enabled` needs its own read RPC, or can ride on an
existing config-exposure surface") and branch its empty-state text on it.

---

## Surface 3: `RestartWithSummaryButton` — embedded action states

### Wireframe — button label per state (single evolving button, not three components)

```
IDLE (no row / PENDING)         [ Restart with summary            ]
GENERATING                      [ ⟳ Generating summary…            ]  disabled
READY                           [ Start new session from this summary ]
CREATING (session RPC in flight)[ ⟳ Starting session…               ]  disabled
RESTART FAILED (createSession    [ Start new session from this summary ]  ← same
  RPC errored)                     label, NOT disabled — see flow below
FEATURE DISABLED                (renders null — no button at all)
```

### Interaction flow — full happy path

```
User action                              System response
──────────────────────────────────────   ─────────────────────────────────────
1. Click "Restart with summary"          useHandoffSummary.trigger() fires
   (IDLE state)                          TriggerHandoffSummaryRPC; button
                                          immediately shows "⟳ Generating
                                          summary…" (disabled) — interim
                                          GENERATING row returned synchronously
                                          per plan Story 2.2.1's 2nd AC ("returns
                                          within milliseconds").
2. (no action — wait)                    useHandoffSummary polls
                                          GetHandoffSummary. On READY, button
                                          re-enables as "Start new session
                                          from this summary."
3. Click "Start new session from this    createSession({ path: undefined,
   summary"                              sessionType: DIRECTORY,
                                          prompt: data.summaryText,
                                          restartFromSessionId: sessionId })
                                          fires; button shows "⟳ Starting
                                          session…" (disabled).
4. (no action — wait)                    On success, UI navigates to the
                                          newly created session's detail view
                                          (its Info tab shows "Restarted
                                          from: <source>" — Surface 4).
```

Task-completion count: **2 user clicks** total from a healthy/idle session
to a running restarted session (step 1 and step 3) — steps 2 and 4 are
system waits, not user actions. This satisfies a "≤3 steps" completion bar
comfortably.

### Interaction flow — restart-session-creation failure (gap the plan doesn't cover)

The plan's Story 2.3.1 explicitly specs a `CodeNotFound` RPC error when
`restart_from_session_id` points at a source session that no longer exists
(e.g. the user archived/deleted it in the time between generating the
summary and clicking restart) — but neither Epic 3.2's acceptance criteria
nor its task list describe what the button does when `createSession`
itself rejects. Designing this state explicitly:

```
User action                              System response
──────────────────────────────────────   ─────────────────────────────────────
3. Click "Start new session from this    createSession RPC returns an error
   summary" (READY state)                (CodeNotFound, or any other failure).
4. (system)                              Button returns to its READY label
                                          (not stuck on "⟳ Starting
                                          session…" forever), and an inline
                                          error line appears directly below
                                          the button:
                                          "Couldn't start the new session.
                                          <plain-language reason, e.g. 'The
                                          original session no longer exists.'
                                          for CodeNotFound, else 'Something
                                          went wrong — try again.'>"
5. Click "Start new session from this    Retries the same createSession call
   summary" again                        — no re-generation needed, since
                                          the READY summary text is unchanged
                                          and still valid.
```

This mirrors `SessionSummaryPanel`'s existing `transport-error` retry
convention (`"Couldn't load this summary." <message> [↻ Retry]`) rather than
inventing new error-handling vocabulary. **No dead end**: the button always
returns to an actionable state (READY, re-clickable) after any failure —
never left permanently disabled or stuck in a spinner.

### Accessibility

- Button `disabled` only while an RPC is genuinely in flight
  (`GENERATING`/`CREATING`), never as a permanent "feature unavailable"
  signal (that's `null`-render instead, per plan's 3rd AC) — a disabled
  button with no explanation is itself a dead end for a screen-reader user
  who lands on it and gets no context for why it doesn't respond.
- State-transition announcements via a single shared `aria-live="polite"`
  region (mirroring `SessionSummaryPanel.tsx`'s pattern verbatim — one
  persistent DOM node, not a fresh one per branch, so screen readers
  reliably announce the update): `"Generating handoff summary…"` →
  `"Handoff summary ready."` → (on restart click) `"Starting new
  session…"` → (on nav) nothing further needed, since navigation itself is
  the confirmation. On failure: `"Couldn't start the new session: <reason>"`.
- `aria-label` on the button always matches its visible text exactly (no
  icon-only state) — the `⟳` glyphs are `aria-hidden="true"` decorations
  alongside real text, per `research/ux.md` §3's rule, applied consistently
  here even though this is a button, not a status chip.

---

## Surface 4: New session creation — prompt pre-fill + lineage indicator

### Pre-fill (silent, no user-visible step)

There is deliberately **no intermediate "review before sending" screen** in
the plan as specced — `createSession` is called directly with
`prompt: data.summaryText` on the restart click (Surface 3, step 3). The
only user-visible preview opportunity is the `▸ Preview full handoff text`
disclosure in the `READY` row (Surface 2) **before** clicking restart — that
is the review step, not a separate modal. This keeps the flow to 2 clicks
(Surface 3) while still giving an inspectable, non-blocking way to check the
text first, satisfying "no dead ends" without adding a third click for
everyone who doesn't need to review.

### Wireframe — the new session's Info tab, lineage row

```
┌─ Info tab (new session) ──────────────────────────────────────────────┐
│ Instance ID: d4e5f6...             Status: Active                     │
│ Session Type: Directory            Created: just now                 │
│ Restarted from: ↗ "Fix flaky auth test" (a1b2c3...)                   │  ← new row
│ ...(existing key/value grid, unchanged otherwise)...                  │
└─────────────────────────────────────────────────────────────────────┘
```

**Gap found — no task currently builds this row.** The plan's Domain
Glossary explicitly motivates the `restarted_from_session_id` proto field
with "so the UI can render 'Restarted from: <link>'" (line 46), but no task
in Phase 2 (`server/adapters/instance_adapter.go` threading, Task 2.3.1b) or
Phase 3 (frontend) actually renders it — Task 2.3.1b only wires the field
through the adapter; `SessionDetailView.tsx`'s Info-tab grid is never told
to add this row. Without it, the field is persisted but **invisible** — a
user looking at a restarted session has no way to tell it *was* a restart,
or to click back to the source session for comparison. This is a real UX
gap in an otherwise-complete plan, not a nitpick: Surface 4 is one of the
five surfaces explicitly named in this design task's brief, and it
currently has no implementing task. Recommend adding a Task 3.3.1d (or a new
Epic) to `SessionDetailView.tsx`'s Info-tab grid: render `Restarted from:`
only when `session.restartedFromSessionId` is non-empty; link text is the
source session's title if `FindLiveInstance`/storage lookup still resolves
it, else the raw ID with `(no longer available)` and no link (mirrors
Story 2.3.1's own "missing source" handling — the read side needs the same
graceful-degradation the write side already specs).

### Interaction flow — following the lineage link

```
User action                              System response
──────────────────────────────────────   ─────────────────────────────────────
1. Click "↗ Fix flaky auth test"         Navigates to the source session's
   in the new session's Info tab         detail view (same-tab navigation,
                                          matching the rest of this app's
                                          session-to-session link convention
                                          — verify against whatever pattern
                                          `SessionCard`'s own internal links
                                          use before implementing, so this
                                          doesn't introduce a second nav
                                          idiom).
2. (source session was archived/         Text renders as plain, non-clickable:
   deleted since the restart)            "Restarted from: auth-test-sess
                                          (no longer available)" — no dead
                                          click, no 404 navigation.
```

---

## UX acceptance criteria (testable)

1. **Task completion, cold start to running restart: ≤3 user actions.**
   Given a session with no existing `HandoffSummary` row, a user reaches a
   newly running, summary-seeded session in 2 clicks ("Restart with
   summary," then "Start new session from this summary") plus, only if
   desired, 1 optional click to expand the preview disclosure first.
2. **No dead ends on any failure path.** Every terminal failure state
   (`ERROR` generation row, failed `createSession` on restart) leaves an
   enabled, re-clickable action in place — never a permanently disabled
   button or a state with no visible next step. Verified: generation
   failure → "Try again" re-enabled; restart-creation failure → button
   returns to READY, re-clickable, no re-generation required.
3. **Every error state has a plain-language message, not a raw stage
   string or stack trace as the primary line.** `stage == "transcript"` →
   `"Couldn't read this session's conversation history."`; `stage ==
   "generation"` → `"Failed while generating the handoff summary."`;
   unrecognized/future stage → generic fallback, never the enum string
   itself. Raw `error_message` is available only behind an explicit
   `▸ Details` disclosure, never in the primary line (mirrors
   `SessionSummaryPanel`'s `ERROR_STAGE_COPY` convention exactly).
4. **Explicit empty state, always rendered, never hidden.** A session with
   no `HandoffSummary` row shows `"No handoff summary generated for this
   session."` rather than omitting the `Handoff Summary` section entirely
   (matches `WorkflowHistorySection`'s issue-#198-motivated convention) —
   and this state is textually distinct from the feature-disabled state
   (see Gap in Surface 2), so a user can tell "not tried yet" from "not
   available here."
5. **Historical/generating rows use `role="list"`/`role="listitem"`, never
   `role="status"`.** A `GENERATING` row does not cause a screen reader to
   re-announce on every poll tick — only the shared `aria-live="polite"`
   region (Surface 3) announces state *transitions*, not the list's static
   content, avoiding the "chatty" anti-pattern `context-compaction-detection`'s
   own research explicitly warns against.
6. **Icon is always decorative, text label always present.** Every `⟳`/`✓`/`⚠`
   glyph in `HandoffSummarySection` carries `aria-hidden="true"` with a
   real adjacent text label — never an icon-only signal (verified against
   every row wireframe in Surface 2).
7. **Keyboard reachability.** The section's `CollapsibleSection` header,
   the `▸ Preview full handoff text` / `▸ Details` `<summary>` elements, and
   the action button are all native interactive elements (`<button>`,
   `<summary>`) — reachable via `Tab`, actionable via `Enter`/`Space`, with
   no custom `<span onClick>` pattern that would require an extra
   `tabIndex` fix (the exact gap `context-health-monitoring/design/ux.md`
   found and flagged in its own badge — this design avoids repeating it by
   using only native focusable elements throughout).
8. **Color contrast ≥ 4.5:1 for all body/label text.**
   - Light theme `successText` `#065f46` on `successBg` `#d1fae5` = **6.78:1** (pass) — used for the READY row's status text.
   - Light theme `errorText` `#991b1b` on `errorBg` `#fee2e2` = **6.8:1** (pass) — used for the ERROR row's status text.
   - Light theme `primaryText` `#ffffff` on `primary` `#0070f3` = **4.55:1** (pass, narrowly) — the primary action button (`primaryButton`, reused from `SessionSummaryPanel.css.ts`).
   - **Flagged, not introduced by this design**: dark theme `primaryText` `#ffffff` on `primary` `#2d9cdb` = **3.05:1**, which fails the 4.5:1 body-text floor (though it clears the 3:1 large-text/UI-component floor). This is a pre-existing token pairing already shipped in `SessionSummaryPanel`'s own "Regenerate"/"Retry" buttons — `RestartWithSummaryButton` inherits it by reusing `primaryButton`, not a new defect this feature introduces. Worth a follow-up token fix independent of this plan.
9. **No second live badge introduced.** Confirmed against `research/ux.md`
   §1's resolution (moot after ADR-001, since there is no live compression
   event anymore) — `HandoffSummarySection` never renders in the session
   card's badge row, only in the Info tab.
10. **Lineage is inspectable or gracefully absent, never a broken link.**
    A restarted session's "Restarted from" row either links to a resolvable
    source session or renders plain text with "(no longer available)" —
    never a link that 404s or throws.

---

## Summary of gaps flagged for implementation (not blocking Phase 3, but should be triaged)

1. **Restart-session-creation failure has no specced UI** (Surface 3) — Epic
   3.2's acceptance criteria cover only the happy path (idle → generating →
   ready → createSession) and the feature-disabled null-render; a failed
   `createSession` call (e.g. `CodeNotFound` per Story 2.3.1's own 3rd AC)
   has no designed recovery state. This document proposes one (inline error
   line + button reverts to READY, re-clickable) — recommend folding into
   Task 3.2.1a before implementation, not after.
2. **Feature-disabled empty-state text is unspecified** (Surface 2) — Task
   3.3.1a's empty-state AC only covers "no row exists," conflating it with
   "feature turned off." Recommend `HandoffSummarySection` take an
   `enabled` prop and branch its text.
3. **`restarted_from_session_id` is persisted but never rendered anywhere**
   (Surface 4) — the Domain Glossary names the intended UI ("Restarted
   from: <link>") but no task in Phase 2 or Phase 3 implements it. This is
   the most significant gap found: one of the plan's own stated purposes
   for adding the field has no corresponding frontend task.
4. **No affordance links a RED `ContextHealthBadge` to a ready handoff
   summary** (Surface 1) — today and even after Phase 4 unblocks, a user
   must remember to check the Info tab; nothing on the card signals "a
   summary is already waiting." Out of scope for this plan's Phase 3/4, but
   worth a fast-follow story once `context-health-monitoring` ships.
