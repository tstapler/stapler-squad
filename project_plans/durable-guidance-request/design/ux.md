# UX Design: Durable Guidance Request

SDD Phase 3 (Design). Builds directly on `research/ux.md` (comparable
patterns, mental model, accessibility, error/edge table, JTBD) and
`implementation/plan.md`'s Phase 6/7 (component names/files are load-bearing
and used verbatim below: `GuidanceRequestCard.tsx`,
`GuidanceRequestCard.css.ts`, `GuidanceRequestCard.test.tsx`,
`GuidanceRequestNavBadge.tsx`). This document does not re-derive the
research — it wireframes and sequences it.

Grounding read for embedding-context shapes (confirmed by reading the actual
files, not assumed):
- `web-app/src/components/backlog/BacklogItemDetail.tsx` — a scrollable,
  section-based page (`styles.header` / `bannerBar` / `scrollArea`), each
  section independently collapsible with a per-item `localStorage` key
  (`backlog-detail-section-${itemId}-${sectionKey}`). Roomy.
- `web-app/src/components/backlog/TriageReviewPanel.tsx` — a dense review
  panel (`aria-live="polite"` on the whole panel) hosting
  `TriageDiffSection.tsx`'s diff + question-answer flow alongside
  Apply/Skip/Refine actions. Compact, action-dense.
- `web-app/src/components/sessions/SessionDetailView.tsx` — a tabbed view
  (`terminal` is the default/primary tab, plus `diff`/`vcs`/`files`/
  `browser`/`artifacts`/logs/shell tabs) with a fixed header. The terminal
  tab is the noisiest, most focus-sensitive surface in the app.
- `web-app/src/components/sessions/ApprovalNavBadge.tsx` — the exact badge
  shape to clone: `useApprovalsContext()` → `NavBadge` primitive, full-
  sentence `aria-label`, hidden at zero. Its backing context
  (`ApprovalsContext.tsx`) uses RTK Query with a 5s `pollingInterval` — this
  is the **existing, pre-approved** client-side refresh convention (not a
  new backend poll loop; ssq#428/the Constraints ban is about server-side
  `ScheduleWakeup`-style polling, which this doesn't touch).

---

## 1. Surface Inventory

| # | Surface | Component | Density context |
|---|---|---|---|
| a | `GuidanceRequestCard` — pending, 3 sub-variants (yes-no, multiple-choice, short-answer) | `GuidanceRequestCard.tsx` | varies by host (§7) |
| b | `GuidanceRequestCard` — answered | `GuidanceRequestCard.tsx` | varies by host (§7) |
| c | `GuidanceRequestCard` — cancelled/archived (`readOnly`) | `GuidanceRequestCard.tsx` | varies by host (§7) |
| d | Proactive "N/cap pending" summary | `GuidanceRequestCard.tsx` (Story 6.1.2) | backlog item detail + triage panel |
| e | `GuidanceRequestNavBadge` — zero-state (hidden) and N-state | `GuidanceRequestNavBadge.tsx` | `Header.tsx` / `BottomNav.tsx` |
| f | 3 embedding contexts | `BacklogItemDetail.tsx`, `TriageReviewPanel.tsx`, `SessionDetailView.tsx` | §7 |

---

## 2. `GuidanceRequestCard` — Pending State

All three variants share one shell: a collapsed one-line summary with an
`Answer ▸` toggle (exact clone of `TriageDiffSection.tsx`'s existing
pattern), expanding in place to the type-specific control + Submit/Cancel.
The whole card is `role="form"` with `aria-label={"Answer: " + questionText}`,
wrapped in `aria-live="polite"` (never `assertive` — research/ux.md §3).

### 2.1 Yes/No

```
Collapsed:
┌──────────────────────────────────────────────────────────┐
│ ❓ Should triage merge PR #780 into this item's branch?   │
│                                              [ Answer ▸ ] │
└──────────────────────────────────────────────────────────┘

Expanded (role="radiogroup", aria-labelledby=question text):
┌──────────────────────────────────────────────────────────┐
│ ❓ Should triage merge PR #780 into this item's branch?   │
│                                                            │
│   ( ) Yes        ( ) No                                   │
│                                                            │
│                          [ Cancel ]   [ Submit (disabled) ]│
└──────────────────────────────────────────────────────────┘
```

**Flow**:
1. Operator sees the collapsed summary (one line, always visible — never
   hidden behind a second click just to know a question exists).
2. Click/tap `Answer ▸` → `aria-expanded` flips true, radiogroup renders in
   place (no navigation, no modal).
3. Arrow keys move the roving-tabindex selection between Yes/No; Tab enters
   once, exits once (research/ux.md §3 — native `<input type="radio">`, not
   `<div onClick>`).
4. Selecting either option enables Submit (`aria-disabled`/`disabled` both
   flip false together).
5. Submit → `aria-busy="true"`, button disabled during the request, **and**
   its label changes to "Submitting…" — a disabled-invalid button (no
   selection yet) and a disabled-in-flight button (request pending) must not
   look identical, since `disabled` alone doesn't tell the operator whether
   the click did anything. The label swap is the cheap, sufficient signal;
   no spinner asset is needed.
6. System response — see §8 (Error/Edge Table) for the three outcomes
   (success, stale-answered-elsewhere, transient failure).
7. `Cancel` while expanded collapses back to the pending summary with no
   network call — always available, no dead end.

### 2.2 Multiple-choice

```
≤6-8 options — radiogroup (same shape as 2.1, N options):
┌──────────────────────────────────────────────────────────┐
│ ❓ Which environment should this deploy target?           │
│                                                            │
│   ( ) staging   ( ) canary   ( ) production                │
│                                                            │
│                          [ Cancel ]   [ Submit (disabled) ]│
└──────────────────────────────────────────────────────────┘

>8 options — <select>/listbox instead of a long radio list:
┌──────────────────────────────────────────────────────────┐
│ ❓ Which service owns this regression?                    │
│                                                            │
│   [ Select a service            ▾ ]                       │
│                                                            │
│                          [ Cancel ]   [ Submit (disabled) ]│
└──────────────────────────────────────────────────────────┘
```

**Flow**: identical to 2.1 except the control; the ≤6-8 vs. >8 threshold
switch is a rendering decision inside `GuidanceRequestCard.tsx` keyed on
`options.length` (Task 6.1.1b), not a prop the caller sets. `<select>`
variant gets Home/End/typeahead for free from the native element — no
custom keyboard handling needed.

### 2.3 Short-answer

```
Collapsed: identical shape to 2.1/2.2.

Expanded:
┌──────────────────────────────────────────────────────────┐
│ ❓ Should this item include the mobile client changes,    │
│    or just backend?                                       │
│                                                            │
│   ┌────────────────────────────────────────────────────┐ │
│   │ (type your answer)                                  │ │
│   │                                                      │ │
│   └────────────────────────────────────────────────────┘ │
│                          [ Cancel ]   [ Submit (disabled) ]│
└──────────────────────────────────────────────────────────┘
```

**Flow**: `<textarea>` + matching `<label>`/`aria-label`, `Escape` cancels
(clone of `TriageDiffSection.tsx`'s existing `onKeyDown`), Submit stays
disabled until the trimmed value is non-empty.

---

## 3. `GuidanceRequestCard` — Answered State

```
┌──────────────────────────────────────────────────────────┐
│ ✓ Answered: "yes"                                         │
│   Should triage merge PR #780 into this item's branch?    │
└──────────────────────────────────────────────────────────┘
```
`role="status" aria-live="polite"` — clone of `TriageDiffSection.tsx`'s
`answeredMarker`. **No interactive controls in the DOM** (not `disabled` —
actually absent), same convention as `TriageReviewPanel`'s `readOnly` prop.

### 3.1 The end-to-end answered flow (the flagged emotional risk)

research/ux.md's biggest flagged risk: the card's own `✓ Answered` echo is
*local* proof the click registered, but it is **not** proof the (possibly
different, possibly headless) asking session actually resumed. The flow
below shows all three layers the operator can observe, in sequence, not
just the card:

```
 Layer 1: THE CARD (local, immediate)              Layer 2: THE BADGE (system-wide, ~5s)        Layer 3: THE NOTIFICATION (durable, resumption-confirmed)
 ─────────────────────────────────────              ──────────────────────────────────────       ──────────────────────────────────────────────────────

 t0  Operator clicks Submit on q3
     ("yes") in the Triage Panel for
     item b608ab1e.
        │
        ▼
 t0+ε  Card transitions in place to
     "✓ Answered: yes"                     ── AnswerGuidanceRequest RPC succeeds ──▶
     (proves: my click did something)               │
                                                       ▼
                                            t0+ε  RPC publishes
                                            GuidanceRequestEventPayload on the
                                            bus (same request, before HTTP
                                            response returns — plan.md
                                            Story 3.1.1)
                                                       │
                                                       ▼
                                            t0+ε  GuidanceRequestDeliveryService
                                            consumes it: backlog-item scope →
                                            headless.Pool respawns a fresh
                                            triage session for b608ab1e, seeded
                                            with "The human answered: yes"
                                                       │
                                                       ▼
                                            t0+≤5s  Header/BottomNav's
                                            GuidanceRequestNavBadge (RTK Query,
                                            5s poll, same convention as
                                            ApprovalNavBadge) refetches
                                            ListAllPendingGuidanceRequests →
                                            count drops 1→0, badge hides
                                            (proves: the system-wide pending
                                            state actually changed, not just
                                            my local DOM)
                                                                                          │
                                                                                          ▼
                                                                              t0+respawn  Once the
                                                                              respawned triage session
                                                                              completes its resumed
                                                                              run (Story 3.2.3), a
                                                                              notification posts to the
                                                                              existing pipeline:
                                                                              "Item b608ab1e's triage
                                                                              resumed after your
                                                                              answer to q3"
                                                                                          │
                                                                                          ▼
                                                                              Operator sees it in
                                                                              NotificationsNavBadge /
                                                                              NotificationPanel
                                                                              (proves: the ANSWER was
                                                                              actually consumed
                                                                              downstream, not just
                                                                              queued)
```

**Why all three layers matter, stated as one sentence each**: the card
proves the click landed; the badge count dropping proves the backend
actually cleared the pending row (not just this browser tab's cache); the
notification proves a *specific* downstream consumer used the answer to do
something. Any one alone leaves a gap the JTBD research explicitly flags —
shipping only the card's local echo (the tempting minimal implementation)
reproduces the exact "did that even do anything?" feeling research/ux.md
calls out as the single biggest UX risk in this feature.

**Distinguishing "my own submit succeeded" from "answered elsewhere, auto-
transitioned"**: both end at the same "✓ Answered: {value}" render (§3), but
they must not feel identical in the moment, or the operator can't tell
whether their own click landed. The self-submitted case briefly shows the
Submit button's own success feedback (the "Submitting…" label from §2.1
resolving directly into the answered card in the same interaction, no
intervening blank/loading gap) before settling into the shared final state.
The answered-elsewhere case (a second tab, or a different operator device,
answered the same question first) has no such transition to show — the card
simply arrives already in the answered state on next render/refetch, with no
local "my click did this" moment to echo. No new visual state is needed
beyond what §2.1/§3 already specify — the distinction lives entirely in
*which path* got the operator to the answered card, not in a new copy or
component variant.

**Edge case — respawn fails silently**: if `GuidanceRequestDeliveryService`'s
resume attempt fails (e.g. `headless.Pool` respawn errors), the badge count
still drops to 0 (the question itself IS answered — that fact doesn't
revert), but no "resumed" notification appears. This is a real, currently
undetectable-from-the-UI gap: the operator has no signal that "answered" and
"consumed" diverged. Flagged in §9 as an acceptance criterion the backend
must support (a failure-to-resume path should also notify, not just the
success path) — this is a plan.md Story 3.2.3 scope question, not purely
frontend, and is called out explicitly rather than smoothed over.

---

## 4. `GuidanceRequestCard` — Cancelled/Archived (`readOnly`) State

```
┌──────────────────────────────────────────────────────────┐
│ ⊘ No longer needed                                        │
│   Should triage merge PR #780 into this item's branch?    │
│   This item was archived before this question was         │
│   answered — no longer actionable.                        │
└──────────────────────────────────────────────────────────┘
```
Clone of `TriageReviewPanelReadOnlyProps`: interactive controls are **absent
from the DOM**, not disabled — "a historical record shouldn't be dismissible
in the first place" (verbatim rationale already established for
`TriageReviewPanel`). Renders whenever `cancelled_at` is set — whether by the
self-heal sweep (Phase 8, scope archived while pending) or a future manual
cancel path. Copy is specific to *why* it's inert, not a generic "cancelled."

---

## 5. Proactive "N/cap Pending" Summary

```
Below the cap (no summary shown — the ux.md-recommended default: only
render once close to the cap, not on every card, to avoid numeric noise
the operator has to read on every single question):

┌──────────────────────────────────────────────────────────┐
│ ❓ Should this item include mobile changes?  [ Answer ▸ ] │
└──────────────────────────────────────────────────────────┘

At or near the cap (pendingCount >= cap - 1):

┌──────────────────────────────────────────────────────────┐
│  3/4 pending                                               │  ← chip, same visual
├──────────────────────────────────────────────────────────┤     language as the nav
│ ❓ Should this item include mobile changes?  [ Answer ▸ ] │     badge count (small,
│ ❓ Merge PR #780 first?                       [ Answer ▸ ] │     scannable, not prose)
│ ❓ Which environment?                         [ Answer ▸ ] │
└──────────────────────────────────────────────────────────┘
```

**Flow**: rendered purely from fields already on `ListGuidanceRequests`'s
response (`pending_count`, `cap`) — zero extra round trip (ux.md's explicit
API contract ask, honored in plan.md's proto design, Task 2.1.1a). Appears
in backlog item detail and the triage panel (both are creation points for
new item-scoped questions); does not need to appear in the session view,
since a session only ever sees its own single-scope queue, not a
create-time decision point an operator is weighing.

**Edge case — cap already exceeded** (an agent's `create_guidance_request`
call was rejected server-side, AC7): the UI never shows a "rejected" state
for a request that never got created — there is nothing to render. The
signal instead lives in the asker's own MCP-tool error return (agent-facing,
not UI-facing) and in structured logs (`cap-rejected` per Observability
Plan). The UI's only job is the *proactive* "3/4 pending" chip above, shown
*before* the agent would hit the wall.

---

## 6. `GuidanceRequestNavBadge` — Zero-State and N-State

```
Zero pending (hidden entirely — not rendered with a "0", matching
ApprovalNavBadge's hide-at-zero convention):

┌─────────────────────────────────────────────────┐
│  [Backlog] [Sessions] [Approvals] [Reviews]      │   ← no guidance badge present
└─────────────────────────────────────────────────┘

N pending (visible, badge count, full-sentence aria-label):

┌─────────────────────────────────────────────────┐
│  [Backlog] [Sessions] [Approvals] [Reviews] [❓2]│
└─────────────────────────────────────────────────┘
         aria-label="2 pending guidance requests. Click to review."
```

**Flow**:
1. `useGuidanceRequestsContext()` polls `ListAllPendingGuidanceRequests` on
   the same RTK Query 5s-interval convention as `useApprovalsContext()` —
   badge count is eventually-consistent within ~5s of a create/answer, never
   push-instant (honest about ssq#428's no-push-subscription constraint;
   this is the accepted tradeoff, not a bug).
2. Click behavior (destination is **not specified in plan.md** — flagged as
   a design decision below, not invented silently):
   - **Exactly 1 pending request**: navigate directly to its owning
     scope's existing view (the item's backlog detail page for
     `backlog-item` scope, or the session's `SessionDetailView` for
     `session` scope) — mirrors `ApprovalNavBadge`'s parent-supplied
     `onClick` pattern, reusing a view that already exists rather than
     building a new page (no new page is in plan.md's scope).
   - **>1 pending request, possibly across scopes**: open a lightweight
     popover (reusing `NotificationPanel.tsx`'s existing sidebar-overlay
     precedent, not `ReviewQueueNavBadge`'s dedicated-page precedent, since
     no `/guidance-requests` page exists in scope) listing each pending
     question's text + a "Go to →" link per row, built entirely from the
     data already fetched for the badge count (no second round trip).
   - **Design decision flagged, not silently assumed**: plan.md's
     Unresolved Questions section doesn't cover badge-click destination.
     The above is this design's recommended default (cheapest to build,
     reuses two existing precedents already in the codebase) — confirm with
     product before Epic 7 implementation, but it isn't a blocker to start.

---

## 7. Three Embedding Contexts — Same Component, Three Crops

The same `GuidanceRequestCard.tsx` file renders in three host layouts with
different density and focus rules. The differences below are host-side
(spacing, default-expanded state, focus policy) — **never a prop that
changes the component's own markup/behavior**, since AC4 requires byte-
identical rendering logic across hosts.

### 7.1 Backlog Item Detail — roomy, section-based

```
┌ b608ab1e — Fix login redirect loop ───────────────────── [Archive] ┐
│ status: in_progress                    2/2 WIP slots in use        │
├──────────────────────────────────────────────────────────────────┤
│ ▾ Guidance Requests                                    3/4 pending │  ← new collapsible
│                                                                     │     section, same
│   ┌────────────────────────────────────────────────────────────┐  │     pattern as every
│   │ ❓ Should triage merge PR #780 first?         [ Answer ▸ ]  │  │     other section on
│   └────────────────────────────────────────────────────────────┘  │     this page
│   ┌────────────────────────────────────────────────────────────┐  │
│   │ ✓ Answered: "backend only"                                  │  │
│   │   Should this include mobile client changes?                │  │
│   └────────────────────────────────────────────────────────────┘  │
│                                                                     │
│ ▸ Sessions (2)                                                     │
│ ▸ Triage History                                                   │
└──────────────────────────────────────────────────────────────────┘
```
- New top-level collapsible section (matches every other section's
  expand/collapse + `localStorage` persistence convention already in
  `BacklogItemDetail.tsx`).
- Full-width cards, generous spacing, all sub-fields (question text, type)
  legible without truncation — this is the "roomy" host.
- Fetched for `scope="backlog-item"`, refetched on focus/interaction
  (Task 6.1.3a — no push subscription).
- **Zero-pending empty state**: matches this page's own existing sibling-
  section convention exactly — `SessionsSection.tsx`'s `<div role="status"
  aria-live="polite" className={sectionStyles.emptyState}>No sessions yet
  for this item.</div>` — so the new section renders `<div role="status"
  aria-live="polite" className={sectionStyles.emptyState}>No guidance
  requests yet for this item.</div>` when the item has zero requests of any
  status (pending, answered, or cancelled). This is a genuinely-empty state,
  distinct from "all answered" (§3's answered cards still render, they just
  aren't pending).

### 7.2 Triage Panel — dense, review-flow-embedded

```
┌ Triage Review — b608ab1e ─────────────────────── aria-live=polite ┐
│ Diff: +3 files, -1 file                                            │
│  [existing diff sections...]                                       │
│                                                                     │
│ ┌ Questions ──────────────────────────────────────────────────┐   │
│ │ ❓ Should triage merge PR #780 first?         [ Answer ▸ ]   │   │  ← same card,
│ └────────────────────────────────────────────────────────────┘   │     tighter margins,
│                                                                     │     sits alongside
│  [ Skip ]  [ Refine ]  [ Apply ]                                   │     TriageDiffSection's
└──────────────────────────────────────────────────────────────────┘     own question UI —
                                                                            visually a sibling,
                                                                            not a fork
```
- Collapsed by default (never pre-expanded) — this panel already has
  Apply/Skip/Refine competing for attention; an auto-expanded form here
  would be the exact "surprise focus steal in a busy panel" research/ux.md
  warns against.
- Tighter vertical rhythm than 7.1 (smaller card padding via `.css.ts`
  variant selectors keyed on a host-supplied density class, NOT a
  component-behavior prop — purely `className`/CSS, per AC4's "byte-
  identical markup" bar).
- Fetched scoped to the panel's own item context (Task 6.1.3b).

### 7.3 Session View — must not steal focus from the terminal

```
┌ Session s1 ── [Terminal] [Diff] [VCS] [Files] [Browser] [Logs] ───┐
│ ┌────────────────────────────────────────────────────────────┐   │
│ │ ❓ 1 question pending — Use approach A or B?    [ Answer ▸ ]│   │  ← slim, collapsed
│ └────────────────────────────────────────────────────────────┘   │     banner strip
│ ┌────────────────────────────────────────────────────────────┐   │     ABOVE the
│ │ $ npm test                                                   │   │     terminal, inside
│ │ ...                                                           │   │     the terminal tab
│ │ _                                                              │   │     only — never
│ └────────────────────────────────────────────────────────────┘   │     covering it
└──────────────────────────────────────────────────────────────────┘
```
- **Collapsed and non-intrusive by default** — a single-line strip, not a
  modal or an expanded form. The terminal keeps 100% of vertical space
  minus this one line.
- **Never auto-focused on mount**: mounting this banner while the operator
  is mid-keystroke in the terminal must not move focus — announced only via
  the `aria-live="polite"` region (research/ux.md §3's explicit requirement,
  directly motivated by this exact host). Focus moves into the expanded
  form **only** on an explicit click on `Answer ▸`, exactly like the other
  two hosts — no host-specific exception, just a host-specific *starting*
  density.
- Fetched for `scope="session"` keyed by the session's own UUID
  (Task 6.1.3c).
- If the session is currently on a non-terminal tab (e.g. `Diff`), the
  banner still renders at the top of whichever tab is active — it's part of
  the shared page chrome above the tab content, not terminal-tab-specific
  markup, so a pending question is never hidden behind a tab switch.

---

## 8. Error / Edge-State Table (per surface, exact copy)

| Scenario | What the operator sees | Exit path |
|---|---|---|
| Submit succeeds | Card transitions in place to "✓ Answered: {value}" (§3) | N/A — terminal success state |
| Answered elsewhere, this card is stale | Auto-transition to "✓ Answered: {value}" using the answer returned inline by the API (no reload needed) — falls back to "This question was already answered — reload to see the answer." **only** if the payload genuinely lacks the answer value | Reload button on the fallback text; auto-resolves otherwise |
| Scope archived/deleted while pending | Cancelled/`readOnly` render (§4): "⊘ No longer needed — this item was archived before this question was answered." | None needed — this is itself the terminal, non-actionable state; nothing to retry |
| Cap exceeded (agent-side, at creation) | Nothing new renders (no card was created) — proactive "N/cap pending" chip (§5) is the only UI signal, shown *before* this happens | N/A — prevention, not recovery |
| Transient network/server error on submit | `InlineError type="transient"` — headline + Retry + Dismiss (existing convention, verbatim reuse) | Retry re-submits the same answer; Dismiss returns to the pending collapsed state, answer text preserved in the control |
| Respawn/resume fails after answer (delivery-service-side) | Badge count still drops (question is answered); **no** "resumed" notification appears — currently invisible to the operator (flagged in §3.1 as a real gap, not hidden) | Backend self-heal sweep (Phase 8) is the only current backstop; no operator-facing retry UI exists for this in v1 — recorded as a follow-up, not silently accepted as fine |
| Session view: question arrives while operator is typing in terminal | `aria-live="polite"` announcement only, banner appears above terminal without stealing focus (§7.3) | Operator finishes their terminal input, then clicks the banner when ready — no forced interruption |

Every row above has a non-dead-end exit except "cap exceeded" (nothing to
exit from — prevented, not encountered) and "respawn fails silently" (a
named, tracked gap rather than a fabricated recovery path).

---

## 9. UX Acceptance Criteria (human-testable)

**Task completion**
1. From any of the 3 host views, a pending yes/no or multiple-choice
   (≤8 options) question can be answered in **≤3 interactions**: click
   `Answer ▸` → click/select an option → click Submit.
2. A pending short-answer question can be answered in **≤3 interactions**:
   click `Answer ▸` → type → click Submit (Escape counts as the cancel
   path, not part of this count).
3. From the global nav, a single pending guidance request is reachable in
   **≤2 clicks**: click the `GuidanceRequestNavBadge` → arrive at (or be one
   click from, via the popover's "Go to →") the card itself.
4. Discovering that *some* guidance request is pending requires **0
   navigation** — the badge is visible from every page in the app the
   moment `count > 0` (matches `ApprovalNavBadge`'s existing standing bar).

**Error states**
5. A stale-answered-elsewhere submit shows either the auto-resolved
   "✓ Answered: {value}" state or the exact fallback copy "This question
   was already answered — reload to see the answer." — never a generic/
   unlabeled error.
6. A cap-adjacent view (pendingCount ≥ cap − 1) shows the "N/cap pending"
   chip **before** any creation attempt fails — an operator should never
   learn about the cap only from a rejection they can't see (rejections are
   agent-facing/logged, not surfaced as a UI error at all, since nothing
   was created to render).
7. A transient submit failure shows `InlineError` with both a Retry and a
   Dismiss action — never a bare error with no recovery affordance.
8. **No dead ends**: every error/edge state in §8 has a named exit path
   except the two explicitly called out as "nothing to exit from" (cap
   prevention) or "named gap, not fabricated" (silent respawn failure) —
   both are documented above, not silently omitted.

**Cross-session resumption visibility (the JTBD risk, §3.1)**
9. Within 5 seconds of answering a `backlog-item`- or `session`-scoped
   question, the `GuidanceRequestNavBadge` count reflects the answer (drops
   by 1) on every open tab/view — independently verifiable evidence beyond
   the local card echo.
10. When the asking session/triage run successfully resumes, a
    notification entry naming the specific session/item appears in the
    existing notification pipeline (`get_notification_history`) — not a
    generic "something happened" message.

**Accessibility** (grounded in research/ux.md §3's already-specified roles —
no new roles invented here)
11. The entire card is reachable and operable via keyboard alone: Tab
    reaches the `Answer ▸` toggle, Enter/Space activates it, arrow keys move
    within a `radiogroup`, Tab reaches Submit/Cancel, Enter activates them.
12. Every card exposes `role="form"` with an `aria-label` naming the
    specific question text — verifiable via any screen reader or the
    accessibility tree inspector, not just visually.
13. New-question and answered-state changes are announced via
    `aria-live="polite"` — verified by confirming no `aria-live="assertive"`
    exists anywhere in `GuidanceRequestCard.tsx` except (if product opts in
    later) the nav badge's own count-change announcement, which is a
    separate, explicit decision, not a default.
14. Text/background contrast in both the pending-summary chip and the
    cancelled-state copy meets **4.5:1** (WCAG AA) in both light and dark
    theme — verified with the same contrast-checking pass already applied
    to sibling components (`GateVerdictBox`, `TriageDiffSection`), not a new
    color introduced for this feature.
15. In the session view specifically, mounting a new pending-question
    banner never moves keyboard focus away from an in-progress terminal
    input — verified by typing in the terminal while a new question arrives
    and confirming focus/cursor position is unchanged.
16. The answered/cancelled states render **zero** interactive elements in
    the DOM (checked via `container.querySelectorAll('button, input,
    select, textarea')` returning empty in `GuidanceRequestCard.test.tsx`)
    — not merely `disabled` attributes, matching `TriageReviewPanel`'s
    `readOnly` bar.

**Mobile/touch** (§11)
17. Every tappable row — each yes/no and multiple-choice radio option, the
    closed `<select>` trigger, and the `Answer ▸`/Submit/Cancel buttons in
    all 3 hosts — measures at least `44×44px` (verified via computed
    `getBoundingClientRect()` in a component test or a manual device-width
    check), matching this repo's existing `--min-touch-target: 44px`
    convention (`globals.css`, `DrawerNav.css.ts`, `BoardCard.css.ts`) —
    never a smaller target introduced for this feature's denser hosts
    (§7.2's triage panel).
18. At a `<900px` viewport width (this repo's confirmed mobile breakpoint,
    `BottomNav.css.ts`), the yes/no and multiple-choice option rows in all 3
    host contexts wrap to a vertical stack instead of horizontally
    scrolling, truncating, or overlapping — verified by a resize/viewport
    test or manual check at a ~375px width.
19. `GuidanceRequestNavBadge` in `BottomNav.tsx` is tappable at standard
    mobile widths via its surrounding nav item's existing `64px`
    `minHeight` (`BottomNav.css.ts`) — the badge itself stays the small,
    non-interactive (`pointerEvents: none`) count pill, matching
    `ReviewQueueNavBadge`'s existing shape, never a standalone small tap
    target of its own.

---

## 10. Traceability

| Design section | plan.md story/AC |
|---|---|
| §2 (pending, 3 variants) | Stories 6.1.1a/b, AC4 |
| §3 (answered + cross-session flow) | Stories 3.1.1, 3.2.1, 3.2.3, 6.1.1c, AC2/AC4 |
| §4 (cancelled/readOnly) | Story 8.1.1, 6.1.1c, AC5 |
| §5 (N/cap summary) | Story 6.1.2, AC7 |
| §6 (nav badge) | Story 7.1.1, AC2 |
| §7 (3 host contexts) | Story 6.1.3, AC4 |
| §8 (error/edge table) | research/ux.md §4, Stories 2.1.2b, 6.1.1c |
| §9 (acceptance criteria) | AC1–AC7 (cross-cutting) |
| §11 (mobile/touch) | Stories 6.1.1d, 6.1.3a/b/c, 7.1.1a/b, AC1/AC2/AC4 |

---

## 11. Mobile/Touch Considerations

Grounded in the same "read the actual file" discipline as §0's file survey, not
invented conventions — every number below is copied from an existing file, not
picked fresh for this feature. A prior real mobile/desktop-parity miss is on
record for this repo (memory: `feedback_mobile_desktop_ux.md`), so this section
is not optional polish.

### 11.1 Touch target sizing (yes/no and multiple-choice radio options)

This repo already has one documented touch-target convention:
`--min-touch-target: 44px` (`web-app/src/app/globals.css:91`, labeled "WCAG 2.1
AA"), applied verbatim as `44px`/`44px` in `DrawerNav.css.ts:60,142-143` and as
literal `44px`×`44px` padding-driven sizing in `BoardCard.css.ts:33-41`
("44x44px meets the minimum touch target"). `GuidanceRequestCard`'s radio
options (§2.1/§2.2) and `<select>` fallback (§2.2) must hit this same `44px`
minimum on the tappable row (label + input together, not just the bare
`<input type="radio">` control) — implemented the same way `BoardCard.css.ts`
does it: padding does the sizing work, the visible radio glyph itself stays at
its native small size. This is a `.css.ts` change to the row wrapper, not a
new token — reuse `--min-touch-target` (or the `44px` literal, matching
whichever of the two existing call sites' style `GuidanceRequestCard.css.ts`
ends up closer to) rather than introducing a third spelling of the same
number.

The `>8 options` `<select>`/listbox variant (§2.2) inherits native
OS-rendered touch targets for its open dropdown (no custom listbox markup),
so only the closed `<select>` trigger itself needs the `44px` minimum height
applied.

**Spacing between stacked rows**: the `44px` figure covers each row's own
target size, not the gap between adjacent rows when the yes/no or multiple-
choice options wrap to a vertical stack (§11.3's narrow-viewport reflow).
Checked `BoardCard.css.ts`: its own row spacing uses the existing space-scale
token `vars.space["2"]` (`8px`), not a bespoke value. `GuidanceRequestCard.css.ts`
should use the same `vars.space["2"]` (`8px`) as the minimum gap between
stacked option rows, so adjacent 44px targets don't sit edge-to-edge — one
more spelling of an existing token, not a new number introduced for this
feature.

### 11.2 Mobile keyboard behavior (short-answer `<textarea>`)

Checked `TriageDiffSection.tsx`'s own existing free-text answer `<textarea>`
(lines 211-229) — the pattern §2.3 explicitly clones — and it has **neither**
an `inputMode` attribute nor any on-screen-keyboard-obscures-Submit handling
(no `scrollIntoView`, no `visualViewport` listener). This is a real,
pre-existing gap in the pattern being cloned, not something already solved
that this feature can inherit for free:

- **`inputMode`**: not needed here — `inputMode="text"` is the default for a
  `<textarea>`, and the short-answer question type is free-form prose, not a
  numeric/email/url case where a non-default `inputMode` would help. No
  change from the cloned pattern.
- **Font-size floor (iOS Safari auto-zoom)**: checked `TriageDiffSection.css.ts`'s
  `answerTextarea` rule directly — it sets `fontSize: vars.fontSize.sm`,
  which resolves to `14px` (`theme.css.ts`'s `fontSize` scale). This is
  **below** the 16px floor iOS Safari uses to decide whether to auto-zoom on
  input focus, so the pattern being cloned does not already solve this; it's
  a real, inherited gap, not a global convention already in place.
  `GuidanceRequestCard.css.ts`'s short-answer `<textarea>` must set its own
  `font-size: 16px` (or `vars.fontSize.lg`, which is `16px`) rather than
  reusing `vars.fontSize.sm`, so focusing it on an iOS device doesn't trigger
  a jarring viewport zoom. File a follow-up to backport the same fix to
  `TriageDiffSection.tsx`'s textarea, matching §11.2's existing "shared fix,
  not a duplicated bug" treatment of the keyboard-obscures-Submit issue
  below.
- **Keyboard-obscures-Submit**: on a narrow viewport, focusing the
  `<textarea>` inside an already-scrolled-down card (particularly in the
  roomy §7.1 backlog-detail host, where the card can sit low in a long
  scrollable page) risks the on-screen keyboard covering the Submit button
  below it — a common mobile form bug, and one `TriageDiffSection.tsx`
  itself has not solved. Since this is a pre-existing gap in the cloned
  pattern rather than a new regression this feature introduces, the fix
  (a `scrollIntoView({block: "nearest"})` on textarea focus, or a
  `position: sticky` action bar) is scoped as a **shared fix** — apply it in
  `GuidanceRequestCard.tsx` and file a follow-up to backport it to
  `TriageDiffSection.tsx`, rather than silently letting the new component
  duplicate the old component's unfixed bug. Task 6.1.1a (short-answer
  control) should call `textareaRef.current?.scrollIntoView({block:
  "nearest", behavior: "smooth"})` on focus.

### 11.3 Responsive reflow of the 3 card variants

Checked `TriageReviewPanel.css.ts` and `BacklogItemDetail.css.ts` directly:
**neither file contains a single `@media` query today** — both hosts have no
existing responsive-reflow convention for this feature to match. The
project's one confirmed mobile/desktop breakpoint is `900px`
(`BottomNav.css.ts:15-20`, "Only show below 900px (mobile + foldable
range)"), used consistently across `EscapeAnalyticsPage.css.ts` and others —
`GuidanceRequestCard.css.ts` should key any of its own reflow rules off this
same `900px` value rather than inventing a new breakpoint.

- **7.1 Backlog Item Detail (roomy)**: the host page has no responsive CSS
  today, so the card's own layout carries the full burden. At narrow widths,
  the yes/no and multiple-choice radio rows (currently laid out
  horizontally, §2.1/§2.2's ASCII wireframes) must wrap via `flex-wrap: wrap`
  rather than horizontally scroll or truncate — each option's `44px`-tall row
  (§11.1) stacks vertically below ~2-3 options on a ~360-400px-wide phone
  viewport. This is a straightforward flex-wrap addition to
  `GuidanceRequestCard.css.ts`, not a structural change to §7.1's section
  layout.
- **7.2 Triage Panel (dense) — the tightest constraint**: this is genuinely
  the highest-risk surface, as flagged in the task brief. The panel already
  has Apply/Skip/Refine competing for space (§7.2's wireframe) with zero
  existing responsive handling. At `<900px`, the card's tighter
  `.css.ts`-variant padding (§7.2's existing "tighter vertical rhythm"
  design) must not compress the `44px` touch targets from §11.1 — density
  reduction is spacing/margin only, never a smaller tap target. The
  Apply/Skip/Refine action row and the card's own Submit/Cancel row should
  each independently wrap to stacked full-width buttons below `900px`
  (matching no existing convention, since none exists yet for this panel —
  this is new ground, called out explicitly rather than assumed solved).
- **7.3 Session View**: see §11.4 below — this surface's mobile relevance is
  answered there first.

### 11.4 Does the session/terminal view render on mobile at all?

Checked `SessionDetailView.tsx` directly: it contains no mobile-breakpoint
logic of its own — the file's only mobile-specific reference is a comment
noting a Radix Dialog action sheet gets bottom-sheet behavior "on mobile via
globals.css" (an unrelated dialog, not the terminal tab). There is no
`@media`/`useMediaQuery`/viewport check gating the terminal tab itself.

**Explicit call: yes, the session view is a realistic mobile surface today**
— nothing in the codebase prevents it from rendering on a phone, and
`BottomNav.tsx`'s own primary nav includes session-related destinations
reachable at `<900px`. This means §7.3's design (slim collapsed banner strip
above the terminal, never auto-focusing, `aria-live="polite"` only) is not
over-designing for a nonexistent context — it is designing for the
narrowest realistic width this view already supports, just without today's
codebase giving it any special mobile handling to build on. The one
mobile-specific addition needed beyond §7.3's existing design: the banner
strip's own collapsed height must also clear the `44px` touch target
(§11.1) for its `Answer ▸` toggle, since on a phone-width terminal tab every
pixel of vertical space is contested and there's a real temptation to shrink
this strip below that floor.

### 11.5 `GuidanceRequestNavBadge` tappability in `BottomNav.tsx`

Checked `BottomNav.tsx` and `BottomNav.css.ts` directly, not assumed: the
mobile nav's existing badge convention (`ReviewQueueNavBadge`, used
`inline={true}` inside a primary `AppLink` nav item) renders the count as a
small (`1.25rem`/20px, `NavBadge.css.ts:13-14`) decorative pill with
`pointerEvents: "none"` — it is *not itself* the tap target. The actual tap
target is the surrounding `navItem`/`AppLink`, styled at `minHeight: "64px"`
(`BottomNav.css.ts:36`) — already well above the `44px` floor. Per Task
7.1.1b ("matching their exact placement convention"),
`GuidanceRequestNavBadge` must follow this same shape: an inline, non-
interactive count badge nested inside an existing (or new) 64px-tall primary
nav item, never a standalone small tappable badge of its own. If Epic 7's
implementation instead needs a *dedicated* nav entry (rather than
piggybacking on an existing one — e.g. `BOTTOM_NAV_MORE` if it doesn't
justify a primary slot), that new entry inherits `navItem`'s `64px`
`minHeight` automatically, with no new sizing decision required.

For the desktop `Header.tsx` `.actions` context, `ApprovalNavBadge` is
rendered as a real standalone `<button>` (not nested in a bigger touch
target) — but desktop pointer/mouse interaction has no touch-target minimum,
so this asymmetry between the two hosts is pre-existing and out of scope for
this feature to fix.
