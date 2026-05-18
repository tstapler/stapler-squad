# UX Design: Backlog Management Layer

**Feature**: backlog-management  
**Date**: 2026-05-10  
**Status**: Complete  
**Based on**: requirements.md, implementation/plan.md, triad review 2026-05-10

---

## Scope

This document covers the three UX problem areas identified in the product triad review as blockers to implementation. It is the authoritative design specification for Epic 4 (Frontend).

---

## Problem 1: Review-Status Action Buttons

### User Flow

```
[Session exits] → [System runs LLM gate] → [Item moves to review status]
     ↓
[User opens BacklogItemDetail]
     ↓
[User reads gate verdict: PASS / PARTIAL / FAIL / PENDING]
     ↓
  PASS:         [Approve (Mark Done)] — primary CTA, one click, no confirmation
  PARTIAL/FAIL: [Override form: textarea + confirm button] — friction intentional
  Any:          [Reopen for Revision] — secondary action, no confirmation needed
  Any:          [Skip gate] — tertiary destructive link, inline confirmation required
```

### Core Semantic Distinction

"Approve" = the gate passed and I agree with it. "Bypass" = I am choosing to ignore the gate's evaluation entirely. These are meaningfully different decisions and must read differently at a glance — not just differently labeled buttons at the same visual level.

### UI Pattern: Progressive Disclosure with a Destructive Tertiary Link

**Gate verdict: PASS**

```
┌─────────────────────────────────────────────────────────────────┐
│  Gate Verdict                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  ✓  PASSED  ·  Reviewed 3 of 3 acceptance criteria        │  │
│  │     All criteria met. Session output aligns with spec.    │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  [  Approve — Mark Done  ]    [ Reopen for Revision ]          │
│   ← primary (filled)              ← secondary (outlined)       │
│                                                                 │
│  ─────────────────────────────────────────────────────────     │
│  Skip gate and mark done without review ↗                      │
│  ← destructive tertiary link, small text, not a button         │
└─────────────────────────────────────────────────────────────────┘
```

**Gate verdict: PARTIAL or FAIL**

Primary CTA becomes "Reopen for Revision" (the recommended action). Marking done requires explicit justification via the override form.

```
┌─────────────────────────────────────────────────────────────────┐
│  Gate Verdict                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  ✗  PARTIAL  ·  2 of 3 acceptance criteria met            │  │
│  │     "AC-3: Error handling spec was not addressed."        │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  [ Reopen for Revision ]                                       │
│   ← primary (filled)                                           │
│                                                                 │
│  Override: Mark done anyway                                    │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Reason for override (required)                          │  │
│  │  ┌────────────────────────────────────────────────────┐  │  │
│  │  │                                                    │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  │  [ Cancel ]   [ Mark Done — Override ]                   │  │
│  │               ← danger intent, disabled until ≥ 5 chars  │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ─────────────────────────────────────────────────────────     │
│  Skip gate and mark done without review ↗                      │
└─────────────────────────────────────────────────────────────────┘
```

**Gate verdict: PENDING (still running)**

```
┌─────────────────────────────────────────────────────────────────┐
│  Gate Verdict                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  ◌  PENDING  ·  Review in progress (12s)                  │  │
│  │     The LLM gate is evaluating acceptance criteria.       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  [ Approve — Mark Done ]  ← disabled, tooltip: "Wait for gate  │
│  [ Reopen for Revision ]  ← disabled                 or skip"  │
│                                                                 │
│  Skip gate and mark done without review ↗  ← always active     │
└─────────────────────────────────────────────────────────────────┘
```

### Bypass Confirmation (Inline, Not Modal)

On click, the skip gate link expands an inline confirmation — not a modal, to avoid modal-in-panel nesting problems.

```
┌─────────────────────────────────────────────────────────────────┐
│  Skip gate and mark done without review ↗                      │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  The acceptance criteria will not be evaluated.          │  │
│  │  This cannot be undone.                                  │  │
│  │                                                          │  │
│  │  [ Cancel ]   [ Confirm — Skip Gate ]                    │  │
│  │               ← danger intent, no textarea required      │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

One confirmation step, no text entry. The bypass case is legitimate and common enough (gate is wrong) that a textarea would be punitive. The confirmation dialog itself is the friction — it breaks the "click fast and accidentally bypass" failure mode.

### Key States

| State | Primary CTA | Secondary CTA | Override Form | Skip Link |
|---|---|---|---|---|
| PASS | Approve | Reopen | Hidden | Visible |
| PARTIAL | Reopen | — | Visible (collapsed, expand on click) | Visible |
| FAIL | Reopen | — | Visible (collapsed) | Visible |
| PENDING | Approve (disabled) | Reopen (disabled) | Hidden | Visible |
| Override in progress | — | — | Textarea focused | Visible |

### Visual Implementation Notes

- Approve: `intent: primary`, filled button
- Reopen: `intent: secondary`, outlined
- Override — Mark Done: `intent: danger`, filled, disabled until ≥ 5 chars in textarea
- Skip gate link: `font-size: sm`, `color: var(--text-muted)`, underline on hover, no button affordance
- Gate verdict left border: `--success` (PASS), `--warning` (PARTIAL), `--error` (FAIL), `--text-muted` (PENDING)
- The separator line between action buttons and skip link is mandatory — it groups "within the gate system" actions above and the escape hatch below (Gestalt proximity)

### Accessibility — Problem 1

- Tab order: Gate verdict region → Approve/Reopen → Override toggle → Skip link
- Skip link confirmation: focus moves to Cancel on open; Escape closes and returns focus to skip link
- Gate verdict box: `role="status"`, `aria-live="polite"` — updates when gate result arrives
- Approve (disabled): `aria-disabled="true"`, `title` explains why
- Override submit (disabled): `aria-disabled="true"`, `aria-describedby="override-hint"` where hint reads "Enter a reason of at least 5 characters to continue"
- Inline confirmation: `role="alertdialog"`, `aria-labelledby` points to "This cannot be undone"
- Focus traps within confirmation (Cancel + Confirm Skip Gate only); on cancel, focus returns to skip link
- Gate status is communicated via icon + text + color — never color alone (WCAG 1.4.1)

---

## Problem 2: Empty State for /backlog First Run

### User Flow

```
[User navigates to /backlog for the first time]
     ↓
[No items exist → First-run empty state renders]
     ↓
[User clicks "+ New Backlog Item" — the only CTA]
     ↓
[Inline creation form expands within the empty state]
     ↓
[User fills title → submits → first item created]
     ↓
[Empty state dismisses → normal list with one item renders]
     ↓
[Partially-populated state: item visible, no in_progress items yet]
```

### First-Run Empty State

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Backlog                                                           [Board View] │
│─────────────────────────────────────────────────────────────────────────────────│
│                                                                                 │
│         Your backlog is empty.                                                  │
│                                                                                 │
│         Create a work item, define what "done" looks like,                     │
│         spawn an agent — the system reviews output automatically.               │
│                                                                                 │
│         idea → ready → in progress → review → done                             │
│          ◉      ○          ○            ○        ○                              │
│         (you start here)                                                        │
│                                                                                 │
│                     [ + New Backlog Item ]                                      │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

One sentence: what the user does + what the system does automatically. One diagram: lifecycle with "you start here" marker. One CTA. No second sentence, no additional links.

### Inline Form Expansion

On click of "+ New Backlog Item," the empty state does not navigate away. The CTA morphs into a creation form in place — the workflow diagram stays visible for context.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Backlog                                                           [Board View] │
│─────────────────────────────────────────────────────────────────────────────────│
│                                                                                 │
│         Your backlog is empty.                                                  │
│         idea → ready → in progress → review → done                             │
│                                                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  Title                                                                    │  │
│  │  ┌────────────────────────────────────────────────────────────────────┐  │  │
│  │  │  ▋                                                                 │  │  │
│  │  └────────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                           │  │
│  │  Priority   [ Low ▾ ]          Labels   [ + Add label ]                  │  │
│  │                                                                           │  │
│  │  [ Cancel ]                                          [ Create Item ]     │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

AC field is intentionally omitted from the minimal creation form — items start in `idea` and AC is prominent on the detail view. Avoids front-loading friction.

### Partially-Populated State (1+ items, 0 in_progress)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Backlog                          [ + New Item ]   [ Suggest what to work on ] │
│─────────────────────────────────────────────────────────────────────────────────│
│  Status ▾    Priority ▾    Sort: Updated ▾                                      │
│─────────────────────────────────────────────────────────────────────────────────│
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  Refactor auth module  ·  IDEA  ·  Medium  ·  2m ago          [Detail] │   │
│  │  No acceptance criteria yet — add some to mark ready            [+ AC ] │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                 │
│  No items are currently in progress.                                           │
│  Mark an item ready and spawn a session to start working.                      │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

The "no items in progress" notice is below the list, not a banner — it does not compete with actual items. Disappears once any item reaches `in_progress`.

### Key States

| State | What renders |
|---|---|
| Zero items, first visit | Full empty state: workflow diagram + centered CTA |
| Zero items, creation form open | Workflow diagram remains; form replaces CTA |
| 1+ items, all `idea`/`ready` | Normal list + "no items in progress" footer nudge |
| 1+ items, at least one `in_progress` | Normal list, no nudge |
| Filter applied with zero results | "No items match these filters. [Clear filters]" — NOT the first-run empty state |

The filter-zero state and first-run state must be visually distinct. Filter-zero: no workflow diagram, no CTA — just the clear-filters link.

### Accessibility — Problem 2

- Empty state region: `role="region"`, `aria-label="Backlog — empty"`
- "+ New Backlog Item" button receives focus automatically on page load (only focusable element)
- Lifecycle diagram (`idea → ready → ...`): `aria-hidden="true"` — meaning is conveyed by the description sentence above it
- On form expansion: focus moves to Title input
- Form element: `aria-label="Create new backlog item"`
- Cancel: collapses form, returns focus to the trigger button
- Escape also collapses form and returns focus
- "No items in progress" nudge: `role="status"`, `aria-live="polite"`

---

## Problem 3: LLM Timeout and Error States

### User Flows

**"Help me flesh this out" (item-level triage):**

```
[User on BacklogItemDetail, AC count < 2]
     ↓
["Help me flesh this out" link visible below AC list]
     ↓
[User clicks → triage session spawns → inline loading state in AC section]
     ↓
[User can navigate away — session continues server-side]
     ↓
  SUCCESS (≤ 180s):  Suggested AC items appear with [Suggested] badge
  TIMEOUT (> 180s):  Inline error in AC section
  ERROR:             Inline error with logs link
  CANCEL:            SIGTERM sent; loading box collapses; link re-appears
```

**"Suggest what to work on next" (list-level triage):**

```
[User on /backlog list]
     ↓
[User clicks "Suggest what to work on next" in header]
     ↓
[Button disabled; ambient loading indicator in header area]
     ↓
[List remains fully interactive — user is NOT trapped]
     ↓
  SUCCESS:  Dismissible recommendation banner at top of list
  TIMEOUT:  Inline error replaces indicator
  ERROR:    Inline error replaces indicator
  CANCEL:   Indicator gone; button re-enables immediately
```

### Critical Principle: Non-Trapping Loading States

Loading indicators are ambient — visible without dominating. The user's ability to filter, scroll, and interact is uninterrupted. This is a background-process indicator (like a browser tab spinner), not a full-page spinner.

### List-Level Triage: Loading

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Backlog                          [ + New Item ]   [ ◌  Thinking...  41s  × ] │
│─────────────────────────────────────────────────────────────────────────────────│
│  [ list items render normally, fully interactive ]                              │
└─────────────────────────────────────────────────────────────────────────────────┘
```

Elapsed time counter gives the user concrete information to decide whether to continue waiting. `×` cancels immediately — no confirmation needed (cancel is always safe).

At 60s, the indicator text changes to: `◌  Still working — up to 3 min  41s  ×`

### List-Level Triage: Success

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Backlog                          [ + New Item ]   [ Suggest what to work on ] │
│─────────────────────────────────────────────────────────────────────────────────│
│  ┌──────────────────────────────────────────────────────────────────────── × ─┐ │
│  │  Suggestion  →  Refactor auth module                                       │ │
│  │  "This item has the most complete AC and unblocks two downstream items."   │ │
│  │  [ Start working on this → ]                                               │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│  Status ▾  ...                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

Left-border accent (`--primary`), not full background fill. "Start working on this →" links to item detail — not an automatic spawn. Dismissible with `×`.

### Item-Level Triage: Loading and Success

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Acceptance Criteria                                        [+ Add criterion]   │
│─────────────────────────────────────────────────────────────────────────────────│
│  [ ]  Auth flow handles expired tokens                                          │
│                                                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  ◌  Thinking about acceptance criteria...  23s                    [Stop] │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

On success:

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Acceptance Criteria                                        [+ Add criterion]   │
│─────────────────────────────────────────────────────────────────────────────────│
│  [ ]  Auth flow handles expired tokens                                          │
│  [ ]  Session refresh is silent (no re-login prompt)           [Suggested] [×] │
│  [ ]  Logout clears all session storage                        [Suggested] [×] │
│                                                                                 │
│  Tap to accept — dismiss with × to reject                                      │
│  [Accept all suggestions]                                                       │
└─────────────────────────────────────────────────────────────────────────────────┘
```

Individual tap promotes one suggestion. `[×]` rejects it. "Accept all suggestions" promotes all at once. This gives full control without forced en-masse decisions.

### Error State Hierarchy

**Type 1 — Transient error (network blip, session startup failure):**
```
[ ✕  Triage failed — network error.  [Retry ↺]   × ]
```
Inline, recoverable with one click.

**Type 2 — Timeout (> 180s without result):**
```
[ ✕  Triage timed out after 3 min.   [Retry ↺]   × ]
```
Same inline pattern; copy differs so user understands why (timeout vs crash).

**Type 3 — Permanent failure (session exited with error code):**
```
┌────────────────────────────────────────────────────────────────────────┐
│  ✕  Triage failed                                             [  ×  ] │
│     The triage session exited unexpectedly (exit code 1).              │
│     Check session logs for details.                                    │
│     [ View session logs ]   [ Retry ↺ ]                               │
└────────────────────────────────────────────────────────────────────────┘
```

"View session logs" opens the session terminal view for the failed triage session — zero additional infrastructure. Satisfies Nielsen heuristic 9 (help users recover from errors).

### Cancel Behavior

| Cancel context | Session | UI |
|---|---|---|
| `×` on list loading indicator | SIGTERM sent | Indicator gone; button re-enables immediately |
| `[Stop]` on item-level triage | SIGTERM sent | Loading box collapses; "Help me flesh this out" link re-appears |
| Navigate away during list triage | Session continues running | On return: show result if available, else error/retry |
| Navigate away during item triage | Session continues running | On return: show result if available, else error/retry |

The navigate-away case is critical: the triage session is tied to the backlog item's ID, not the page mount. On return, the UI checks for output and renders accordingly. Users who accidentally navigate away do not lose results.

### Key States

| State | Duration | Render |
|---|---|---|
| Loading: in flight | 0–60s | Ambient indicator with elapsed timer + cancel |
| Loading: still running | 60–180s | Indicator text changes to "Still working — up to 3 min" |
| Success | On result | Banner (list) or suggested AC badges (item) |
| Timeout | > 180s | Inline error: "Timed out. [Retry]" |
| Transient error | On failure | Inline error: "Network error. [Retry]" |
| Permanent failure | On diagnosis | Expanded error with logs link + retry |
| User cancel | Immediate | Indicator gone, button re-enables |
| Return after navigate-away (result ready) | — | Banner/badges render immediately |
| Return after navigate-away (still loading) | — | Ambient indicator still visible |
| Return after navigate-away (failed during away) | — | Error state renders |

### Accessibility — Problem 3

- Loading region: `role="status"`, `aria-live="polite"`, `aria-label="Triage in progress"` — announces without interrupting current focus
- Elapsed timer: update `aria-label` every 30 seconds, not every second (announcing every second is intolerable for screen readers)
- Cancel button: `aria-label="Cancel triage"` (not just "×")
- Recommendation banner: `role="status"`, `aria-live="polite"`, `aria-label="Triage recommendation: [item title]"`
- Suggested AC badges: on appear, `aria-live="polite"` region announces "N acceptance criteria suggestions added. Review and accept or dismiss."
- Focus does not move automatically when suggestions appear — live region announcement is sufficient
- Error containers: `role="alert"`, `aria-live="assertive"` (errors require user action)
- Retry: `aria-label="Retry triage"` in all error variants
- "View session logs": opens in same tab; if new tab required, `aria-label="View session logs (opens in new tab)"`
- Escape on recommendation banner dismisses it; focus returns to "Suggest what to work on next" button

---

## Shared Component Candidates

These three components should be built once and shared across both triage paths:

| Component | Props | Used by |
|---|---|---|
| `<TriageLoadingIndicator>` | `elapsed: number`, `onCancel: fn`, `label: string` | List header + item AC section |
| `<InlineError>` | `type: "transient\|timeout\|permanent"`, `onRetry: fn`, `logsSessionId?: string` | Both triage paths; generalizes to other long-running ops |
| `<GateVerdictBox>` | `verdict: "PASS\|PARTIAL\|FAIL\|PENDING"`, `summary: string`, `elapsed?: number` | BacklogItemDetail |

All new components use vanilla-extract `.css.ts` files per ADR-009. Token dependencies: `--success`, `--warning`, `--error`, `--text-muted`, `--primary` from `globals.css`. Add to `globals.css` first if new tokens are needed.

---

## Usability Validation Checklist

| Heuristic | P1 (Review) | P2 (Empty State) | P3 (LLM States) |
|---|---|---|---|
| 1. System status visibility | ✓ Gate verdict box with real-time update | ✓ Workflow diagram shows lifecycle position | ✓ Elapsed timer; navigate-away persistence |
| 2. Real-world match | ✓ Approve/Reopen/Skip map to natural language | ✓ Status labels match system names exactly | ✓ "Timed out" and "failed" are plain language |
| 3. User control | ✓ Cancel on bypass; Reopen available after Approve | ✓ Cancel on creation form | ✓ Cancel on all loading states; navigate away safely |
| 4. Consistency | ✓ Button intent variants match design system | ✓ Inline form matches other creation forms | ✓ Error pattern matches app-wide inline error |
| 5. Error prevention | ✓ Bypass confirmation; Override textarea threshold | ✓ Items start in `idea`, reducing invalid-state risk | ✓ Cancel before commit; no destructive actions in triage |
| 6. Recognition over recall | ✓ Gate verdict visible before actions | ✓ Workflow diagram is inline context | ✓ Error type labeled ("timeout" vs "failed") |
| 7. Flexibility | ~ No keyboard shortcut for Approve (consider `Ctrl+Enter`) | ✓ Inline form; no navigation required | ~ No keyboard shortcut for retry |
| 8. Minimalist design | ✓ Skip gate below separator; three clear visual weight levels | ✓ One sentence + one diagram + one button | ✓ Ambient indicator; minimal space |
| 9. Error recovery | ✓ Override form provides path when gate fails | N/A | ✓ Logs link for permanent failures |
| 10. Help + docs | ✓ Verdict excerpt + button tooltips | ✓ Workflow diagram is contextual help | ✓ "View session logs" for diagnosis |

### Friction Points to Address in Implementation

**Problem 1:**
- "Reopen" on a PASS verdict may feel counterintuitive; add tooltip: "Move back to in_progress for further work"
- PENDING state with no ETA is frustrating; surface estimated gate time if available from the session

**Problem 2:**
- Inline form validation errors must appear within the form, not as a page-level toast
- Filter-zero state must NOT display the workflow diagram — new users could confuse it with first-run empty state

**Problem 3:**
- At 150s elapsed, users may feel the system is broken; the 60s message change ("Still working — up to 3 min") addresses this
- Multiple simultaneous triage sessions (item + list) need distinct loading indicators with distinct labels; ensure they don't visually merge
- "Accept all suggestions" is irreversible in one click; consider a brief undo toast: "3 AC items added — Undo"

---

## Epic 4 Implementation Priority Order

1. **Problem 1 (Review action buttons)** — Implements the most consequential irreversible action in the system. Get the bypass confirmation behavior reviewed before shipping.
2. **Problem 3 (LLM states)** — The ambient loading indicator and navigate-away persistence are infrastructure patterns other features will reuse. Build these as shared hooks/components early.
3. **Problem 2 (Empty state)** — Simplest to implement; users see it exactly once. Time saved here goes into making filter-zero and partially-populated states clear.

---

## UX Readiness Gate

- [x] User flow mapped — All three problem areas have step-by-step flows covering primary path and all failure modes including navigate-away persistence and cancel behavior
- [x] Key states identified — Each area has a complete state table covering: empty/pending/loading, success, transient error, permanent error, timeout, cancel, and edge cases
- [x] Accessibility requirements noted — ARIA roles, live region strategy, focus management on open/close/cancel, color-independent status communication, and keyboard navigation specified for all three areas
