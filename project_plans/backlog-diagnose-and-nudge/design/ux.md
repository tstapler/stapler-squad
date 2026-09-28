# UX Design: Backlog Diagnose & Nudge

Companion to `requirements.md`, `research/ux.md`, and `implementation/plan.md`'s
Phase 8. This document is the UX design artifact for the frontend surfaces of the
"Diagnose" action — wireframes, interaction flows, error/edge-case handling, and
testable acceptance criteria.

Everything here builds directly on `research/ux.md`'s findings rather than
re-deriving them: the seven-outcome-state table (§4), the house-style conventions
already shipped in `backlog-stuck-item-visibility` (§0, §2, and its own
`design/ux.md`, read in full for this doc), and the components `plan.md`'s Phase 8
already scoped (`onDiagnose` prop, `DiagnoseOutcomeDisplay.tsx`,
`DiagnoseHistoryList.tsx`, reuse of `GateVerdictBox`/`TriageReviewPanel`
`readOnly` mode, `ActivityLogSection`, and `sessionKind.ts`'s
`headless_diagnostic` classification via a `headless-diagnose-*` id prefix).

Conventions inherited and matched exactly (verified by reading the cited files
directly, not inferred):
- Three-state button pattern (`idle`/`pending`/`error`) — `StuckItemDetail.tsx`'s
  `overrideState`, lines 126, 197, 217-219 (rework-cap override form) and its
  identical `onApprovePlan` pending/error handling (lines 138-146, 261-263).
- `data-testid`-tagged `actionCopy` `<p>` per reason variant — never bare prose
  with no test hook (`StuckItemDetail.tsx` lines 160, 166, 173, 228, 236, 245).
- `role="alert"` reserved for the actual error string on failure, never for a
  routine settle (`StuckItemDetail.tsx` lines 218, 261; sibling doc's AC 17).
- Color+text-paired chips, never color-only (sibling `design/ux.md` AC 18,
  `STATUS_CLASS`/`getStatusLabel` / `STUCK_REASON_CLASS` precedent).
- Non-exclusive accordions, focus-returns-to-toggle-on-collapse (sibling AC 29-30).
- `aria-live="polite"` for routine settles, `role="alert"`/assertive reserved for
  urgent/actionable-only signals (sibling AC 17).
- No state may silently upgrade toward "healthy" on stale data (sibling's
  `pr_status_unknown` rule) — the direct analog here: a nudge/bug-filed/skipped
  outcome must never render as if the underlying stuck condition is resolved.
- `GateVerdictBox`'s `readOnly` variant (verified: `GateVerdictBoxReadOnlyProps`,
  `GateVerdictBox.tsx` lines 27-38) already renders a verdict card + summary +
  per-criterion list with **zero** action-button/form DOM — exactly the shape
  needed for a historical outcome record with no write affordance.
- `ActivityLogSection`'s `role="list"`/`role="listitem"` free-form note feed
  (`ActivityLogSection.tsx` lines 39-46), visually distinct from a formal
  verdict — the existing home for the `InconclusiveNoteFiled` outcome's note.
- `classifySessionKind`'s existing `sessionId.startsWith("headless-")` branch
  (`sessionKind.ts` line 30) needs zero new code for a `headless-diagnose-*`
  session to register as `"headless_diagnostic"` (non-steerable, read-only card)
  in the existing Sessions list.

---

## Surface Inventory

| # | Surface | Type | New/Extends |
|---|---|---|---|
| 1 | Diagnose button — `StuckItemDetail.tsx` | Action button, 3-state | New (`onDiagnose` prop) |
| 2 | Diagnose button — `BacklogItemDetail.tsx` | Action button, 3-state | New (`onDiagnose` prop) |
| 3 | Diagnosing (in-flight) state | Busy state | New, shared by 1 & 2 |
| 4 | `DiagnoseOutcomeDisplay` — Nudged | Outcome record | New component |
| 5 | `DiagnoseOutcomeDisplay` — Skipped (safety gate) | Outcome record | New component |
| 6 | `DiagnoseOutcomeDisplay` — Skipped (cap/cooldown) | Outcome record | New component (variant of 5) |
| 7 | `DiagnoseOutcomeDisplay` — Bug filed | Outcome record | New component |
| 8 | `DiagnoseOutcomeDisplay` — Inconclusive | Outcome record | New component |
| 9 | `DiagnoseOutcomeDisplay` — Dispatch failed | Error record | New component |
| 10 | Nudging-disabled (feature flag off) banner | Standing notice | New, proactive |
| 11 | `DiagnoseHistoryList` — populated | Chronological list | New component |
| 12 | `DiagnoseHistoryList` — empty (never diagnosed) | Empty state | New |
| 13 | Diagnostic session as `headless_diagnostic` Synthetic Session | Sessions-list row | Zero new classification code |
| 14 | Duplicate/racing dispatch | Edge-case interaction | Cross-cutting behavior |
| 15 | Diagnostic session itself stalls after dispatch | Edge-case transition | Cross-cutting behavior |
| 16 | Result viewed after navigating away and back | Edge-case / persistence | Cross-cutting behavior |

16 surfaces total (7 outcome-record variants sharing one component, 2 button
call sites sharing one state machine, 1 history list in 2 states, 1 zero-new-code
classification confirmation, and 3 cross-cutting edge-case behaviors).

---

## Surface 1 & 2: Diagnose button (`StuckItemDetail.tsx` / `BacklogItemDetail.tsx`)

**Purpose**: trigger a diagnostic dispatch for this item without leaving the page —
same interaction shape as the existing "Approve Plan" / rework-cap-override
controls, applied to a new action (per `plan.md` Epic 8.1: `onDiagnose` mirrors
`onApprovePlan`'s contract exactly).

```
┌─ StuckItemDetail (expanded) ───────────────────────────────────────────┐
│ 🔴 Rework cap hit       fix: diff auto-repair loop            stuck 2h │
│ item 96cc9eaa                                                [Snooze ▾]│
├──────────────────────────────────────────────────────────────────────────┤
│  Why:        Auto-rework stopped after 3 failed review cycles          │
│  Work sessions used:  3 / 3                                             │
│  ...(existing detail fields, unchanged)...                              │
│                                                                          │
│  [Diagnose]                             ← new button, idle state       │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

┌─ BacklogItemDetail (any item, not only stuck ones) ────────────────────┐
│  ...(existing item detail, unchanged)...                                │
│                                                                          │
│  [Diagnose]                             ← same button, same contract   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Button renders wherever `onDiagnose` is bound (both call sites) — omitted
   entirely if the parent doesn't pass the prop (matches `onApprovePlan`'s
   optional-prop convention: the control does not render as disabled-with-no-
   explanation, it simply isn't in the DOM, per `StuckItemDetail.tsx`'s existing
   pattern of conditionally rendering `onReworkCapOverride && (...)`).
2. User activates via click, Enter, or Space (real `<button>`, Tab-reachable).
3. On activation: button immediately becomes `disabled`, gets `aria-busy="true"`,
   and its visible label changes to **"Diagnosing…"** (Surface 3) — not a spinner
   icon alone, mirroring the `overrideState === "pending"` disabled-button pattern
   already in `StuckItemDetail.tsx` line 197/211.
4. `onDiagnose(itemId)` resolves → button reverts to idle **"Diagnose"** label, and
   a new `DiagnoseOutcomeDisplay` (Surfaces 4-9) plus a refreshed
   `DiagnoseHistoryList` entry appear below the button.
5. `onDiagnose(itemId)` rejects → button reverts to idle, and a `role="alert"`
   inline error renders directly below the button (Surface 9's "dispatch failed"
   shape), exactly matching `overrideState === "error"`'s existing
   `styles.overrideStatus` treatment (`StuckItemDetail.tsx` lines 217-219).

### Accessibility
- `aria-label="Diagnose this stuck item"` on `StuckItemDetail`'s button;
  `aria-label="Diagnose this item"` on `BacklogItemDetail`'s (the bare visible
  label "Diagnose" is ambiguous out of context for a screen-reader user tabbing
  through a dense card — per `research/ux.md` §3).
- Button is never rendered clickable-but-effectively-broken: if nudge capability
  is globally off (feature flag), the button still dispatches (diagnosis itself
  is always available — only the *nudge* branch is gated), but Surface 10's
  standing notice is shown proactively above/near the button so the user knows
  the ceiling of what will happen before clicking.

### Error/edge cases
- See Surface 9 (dispatch failed) and Surface 14 (duplicate/racing dispatch).

---

## Surface 3: Diagnosing (in-flight) state

```
[Diagnosing…]   ← disabled, aria-busy="true"
```

### Interaction flow
1. Entered immediately on button activation (Surface 1/2, step 3).
2. No polling UI is needed beyond the button's own busy state — the eventual
   settle (Surfaces 4-9) is what the user is waiting for. If the user navigates
   away and back before it settles, Surface 16 governs what they see on return.
3. Exit: `onDiagnose` resolves or rejects (Surface 1/2, steps 4-5).

### Error/edge cases
- **Dispatch takes unusually long**: no fixed client timeout is specified here
  (mirrors the sibling's stance on RPC timeouts belonging to the transport
  layer); the button remains in this state until the promise settles. If the
  underlying RPC has its own ConnectRPC timeout, that surfaces as a normal
  rejection → Surface 9.

---

## Surfaces 4-9: `DiagnoseOutcomeDisplay` — the seven outcome states

**Purpose**: render exactly one of `research/ux.md` §4's outcome states with its
own copy, icon+color pairing, and `data-testid`, reusing `GateVerdictBox`/
`TriageReviewPanel` (`readOnly`) for the structured-verdict-shaped outcomes and a
plain banner for the others — per `plan.md` Task 8.2.1a. This is the load-bearing
surface for the feature's central trust job (`research/ux.md` §5): since there is
no approval gate before a nudge, this display is the *only* moment the user
learns what the autonomous action did, so every state must be as visually loud
and specific as a successful one.

```
┌─ Diagnose outcome (below the button, same card) ───────────────────────┐
│  <icon> <state-specific copy>                          <link, if any>  │
└──────────────────────────────────────────────────────────────────────────┘
```

### Surface 4 — Nudged

```
┌────────────────────────────────────────────────────────────────────────┐
│ 🟢 Diagnosed 2m ago — nudged the session.        🔗 View diagnosis      │
└────────────────────────────────────────────────────────────────────────┘
```
Copy (literal): **"Diagnosed \<time\> ago — nudged the session."** Link opens the
diagnostic session's record (its `headless-diagnose-*` Sessions-list row,
Surface 13). Rendered via `GateVerdictBox` `readOnly` with `verdict: "PASS"`-
equivalent styling repurposed as an "acted" color family — **not** literally
green-as-"resolved"; the icon/color communicates "the agent took the nudge
action," never "the underlying stuck condition is now fixed" (`research/ux.md`
§2's explicit warning against implying resolution).

### Surface 5 — Skipped (safety gate: not-idle or identity-mismatch)

```
┌────────────────────────────────────────────────────────────────────────┐
│ 🟡 Diagnosed 5m ago — nudge skipped (session wasn't idle).              │
└────────────────────────────────────────────────────────────────────────┘
┌────────────────────────────────────────────────────────────────────────┐
│ 🟡 Diagnosed 1m ago — nudge skipped (identity check failed).            │
└────────────────────────────────────────────────────────────────────────┘
```
Copy names the **specific** `SafetyGateReason` — never a generic "skipped" (per
`plan.md` AC: "names the specific `SafetyGateReason`"). 🟡 neutral color,
explicitly **not** styled as a failure — declining to act unsafely is the gate
working correctly, matching `research/ux.md` §4's framing.

### Surface 6 — Skipped (nudge cap / cooldown hit)

```
┌────────────────────────────────────────────────────────────────────────┐
│ 🟡 Diagnosed just now — nudge skipped (nudge cap reached: 2/2 this      │
│    window).                                                             │
└────────────────────────────────────────────────────────────────────────┘
┌────────────────────────────────────────────────────────────────────────┐
│ 🟡 Diagnosed just now — nudge skipped (cooldown active, next eligible   │
│    14:32 UTC).                                                          │
└────────────────────────────────────────────────────────────────────────┘
```
A sixth flavor of "skipped" per `research/ux.md` §4's edge cases — names which
specific limit fired, using the "N / cap" numeric convention already
established by `formatReworkCapOverride` (`StuckItemDetail.tsx`). This is the
direct implementation of the "no silent auto-retry with no visible counter"
anti-pattern warning (`research/ux.md` §1) — the running "N nudges so far"
count must be visible here, not only in logs.

### Surface 7 — Bug filed instead

```
┌────────────────────────────────────────────────────────────────────────┐
│ 🐛 Diagnosed 10m ago — filed bug #412 instead of nudging.  🔗 View bug  │
└────────────────────────────────────────────────────────────────────────┘
```
Direct link to the filed backlog item/bug (whatever `create_backlog_item`
returned). 🐛/info color — informational, not urgent.

### Surface 8 — Inconclusive

```
┌────────────────────────────────────────────────────────────────────────┐
│ ⚪ Diagnosed 3m ago — inconclusive.               🔗 View diagnostic note│
└────────────────────────────────────────────────────────────────────────┘
```
⚪ muted, same "couldn't determine" family as the sibling's `pr_status_unknown`
chip. Link scrolls to (or opens) the note in `ActivityLogSection` (Surface 8's
note is posted there, per `plan.md` Task 8.2.2b — no new note-type UI).

### Surface 9 — Dispatch failed

```
┌────────────────────────────────────────────────────────────────────────┐
│ ⚠ Couldn't start diagnosis — MCP server unreachable. Try again.        │
│                                                          [Retry]        │
└────────────────────────────────────────────────────────────────────────┘
```
Copy (literal): **"Couldn't start diagnosis — \<reason if known\>. Try again."**
`role="alert"` (the one urgent state in this set — see Accessibility below).
**Retry** re-invokes `onDiagnose` — the required exit path (no dead end).
Distinct from "ran and found nothing" (Surface 8): this fires *before* a
diagnostic session ever existed.

### Interaction flow (shared across Surfaces 4-9)
1. Rendered as soon as `onDiagnose` resolves with a settled outcome, replacing
   the (now-idle) button's busy state.
2. Persists on the item's detail view indefinitely as the "most recent outcome"
   — it is also always available as the newest entry in `DiagnoseHistoryList`
   (Surface 11), so nothing here is lost once a second Diagnose runs.
3. A second Diagnose dispatch replaces this display with the new outcome (the
   old one remains visible in history — see Surface 11).

### Accessibility
- Surfaces 4-8 (routine settles): `aria-live="polite"` — informational, not
  urgent, per `research/ux.md` §3 and the sibling's `role="alert"`-is-urgent-only
  rule. A "nudged"/"skipped"/"bug filed"/"inconclusive" outcome is exactly the
  kind of routine background settle the sibling's count-region convention
  already covers.
- Surface 9 (dispatch failed) **and** any nudge-attempted-but-errored variant:
  `role="alert"` / `aria-live="assertive"` — matches `StuckItemDetail.tsx`'s
  existing `overrideState === "error"` treatment exactly.
- Every state pairs an icon with a text label; removing all color must still
  leave the seven states distinguishable by text alone (sibling AC 18's test,
  applied here).
- All new chip/badge text-on-background pairs meet WCAG AA 4.5:1, consistent
  with the existing Axe Core CI gate.

### Error/edge cases
- **Diagnostic session crashes/stalls after successful dispatch** — see
  Surface 15 (distinct from Surface 9, which fails *before* a session exists).
- **Result viewed after navigating away** — see Surface 16 (must source from
  persisted state, not client-only memory).

---

## Surface 10: Nudging-disabled (feature flag off) banner

```
┌─ StuckItemDetail / BacklogItemDetail (nudge flag OFF) ─────────────────┐
│  ⚙ Nudging is currently disabled — diagnosis will file a bug or post a │
│     note, but won't act on the session directly.                       │
│                                                                          │
│  [Diagnose]                                                             │
└──────────────────────────────────────────────────────────────────────────┘
```
Copy (literal, required per `research/ux.md` §3/§4): **"Nudging is currently
disabled — diagnosis will file a bug or post a note, but won't act on the
session directly."**

### Interaction flow
1. Shown **proactively**, before any dispatch — as soon as the parent knows the
   `DiagnoseNudgeExecutionFeatureFlag` value (per `plan.md` AC: "When
   `DiagnoseNudgeExecutionFeatureFlag` is off, `StuckItemDetail` renders
   proactively... before any dispatch").
2. The Diagnose button remains fully functional — clicking it still runs
   diagnosis; only the nudge *branch* of the outcome space is foreclosed
   (Nudged/Skipped-safety-gate/Skipped-cap-cooldown outcomes cannot occur while
   the flag is off; Bug filed/Inconclusive/Dispatch failed remain possible).
3. This is an ⚙ informational, standing notice — never styled as a warning or
   error; it states the current mode plainly (Dependabot's "auto-merge enabled
   for patch-level only" precedent, `research/ux.md` §1).

### Error/edge cases
- The banner must never be a disabled-looking control with no explanation — the
  literal on-screen text is required, matching the sibling's `pr_status_unknown`
  "no action available" convention (`research/ux.md` §3).
- If the flag flips ON mid-session (live-settable per repo convention), the
  banner disappears on the next render that observes the new value — no stale
  "disabled" notice lingering after the flag is actually on.

---

## Surface 11: `DiagnoseHistoryList` — populated

**Purpose**: item-scoped, chronological, multi-event history — the Kubernetes-
Events precedent (`research/ux.md` §1) — so a pattern of repeated
ineffective/skipped nudges is visible, not just the latest outcome.

```
┌─ Diagnose History ───────────────────────────────────────────────────────┐
│  role="list", chronological (newest first), each entry role="listitem"   │
│                                                                            │
│  🟢 Nudged the session                          2026-09-27 14:02 UTC     │
│     🔗 View diagnosis session                                            │
│  ──────────────────────────────────────────────────────────────────      │
│  🟡 Nudge skipped (session wasn't idle)         2026-09-27 13:41 UTC     │
│  ──────────────────────────────────────────────────────────────────      │
│  🐛 Filed bug #412 instead of nudging           2026-09-27 12:55 UTC     │
│     🔗 View bug                                                          │
└────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Fetched via `ListDiagnoseDispatches` (not derived from client-only state) —
   renders below `DiagnoseOutcomeDisplay` on the item's detail view.
2. Each entry is independently keyboard-navigable (`role="list"`/`role="listitem"`,
   matching `ActivityLogSection`'s existing pattern exactly); an entry with a
   linked session/bug/note exposes that link as a focusable child.
3. Survives a page refresh — a fresh `ListDiagnoseDispatches` call on remount
   must render the same order as before refresh (per `plan.md` AC).
4. New dispatches prepend to this list (newest-first) as they settle — the
   currently-showing `DiagnoseOutcomeDisplay` (Surfaces 4-9) is always
   identical to this list's first/newest entry, never a divergent client-only
   copy.

### Error/edge cases
- **`ListDiagnoseDispatches` fails to load**: the history section shows its own
  small inline "Couldn't load diagnosis history — \<Retry\>" rather than
  silently rendering empty (which would misread as "never diagnosed" — the
  same class of false-negative the sibling's fetch-error banner exists to
  prevent). This is additive to — not a replacement for — Surface 9, which
  covers a *new* dispatch failing, not a *history read* failing.
- **`InconclusiveNoteFiled` entries**: these route into `ActivityLogSection`'s
  existing feed instead of duplicating note text here (per `plan.md` Task
  8.2.2b) — the history-list entry for such a dispatch still appears (as a
  timestamped "Inconclusive" record with a link into `ActivityLogSection`), it
  just doesn't re-render the note body inline a second time.

---

## Surface 12: `DiagnoseHistoryList` — empty (never diagnosed)

```
┌─ Diagnose History ───────────────────────────────────────────────────────┐
│              This item hasn't been diagnosed yet.                        │
└────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
- Terminal, non-error state — the item is simply new to this feature. No action
  required; the Diagnose button above remains the entry point.

### Error/edge cases
- N/A — distinguishing this from Surface 11's fetch-error variant (above) is
  the important boundary: "confirmed zero" vs. "couldn't confirm" must never be
  conflated, per the sibling's `pr_status_unknown` precedent.

---

## Surface 13: Diagnostic session as `headless_diagnostic` Synthetic Session

**Purpose**: zero-new-UI verification — a dispatched `headless-diagnose-*`
session appears in the existing Sessions list automatically via
`classifySessionKind`'s existing `sessionId.startsWith("headless-")` branch
(`sessionKind.ts` line 30), rendered read-only/non-steerable, exactly like the
existing `headless-re-review-*` diagnostic sessions.

```
┌─ Sessions list (existing, unchanged UI) ───────────────────────────────┐
│  ...                                                                     │
│  🔍 Diagnosis: fix: diff auto-repair loop        headless · read-only   │
│     "Nudged the session — see item detail for outcome"                  │
│  ...                                                                     │
└──────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. On dispatch, the new session registers with a `headless-diagnose-<itemId>-
   <uuid>` id — no new classification code needed (per `plan.md` Epic 8.3, a
   pinning regression test only).
2. It renders in the existing Sessions list with the existing
   `headless_diagnostic` icon/label treatment, non-steerable (`isSteerable`
   returns false — no PTY, nothing to attach to), plus its one-line
   `role="status"` summary (already-shipped mechanism, per `research/ux.md`
   §0).
3. Clicking through from `DiagnoseOutcomeDisplay`'s "View diagnosis" link
   (Surface 4) navigates here — this *is* the "full detail" one click away
   from the item's glance-level outcome chip, per the "triage, not
   investigation" mental model (`research/ux.md` §2).

### Error/edge cases
- See Surface 15 (the diagnostic session itself stalling).

---

## Surface 14: Duplicate/racing dispatch

```
User clicks Diagnose twice quickly (or an automated re-check races a manual click):

[Diagnosing…]  ← button already disabled + aria-busy after first click;
                 second click is a no-op at the DOM level (disabled button).

Server-side duplicate (e.g. a second tab, or a race with a reconciler-triggered
call in a later iteration): the second dispatch's response is treated as
"already diagnosing" rather than a second, contradictory outcome —

┌────────────────────────────────────────────────────────────────────────┐
│ ⟳ Diagnosing… (already in progress)                                     │
└────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Client-side: the disabled+`aria-busy` button (Surface 3) already prevents a
   same-tab double-click from firing a second request.
2. Cross-tab/server race: if a second dispatch is attempted anyway and the
   server reports "already diagnosing" for this item, the UI reuses the
   in-flight state (Surface 3's display) rather than surfacing a second outcome
   that could contradict the first one still resolving.

### Error/edge cases
- This is the direct implementation of `research/ux.md` §4's "two Diagnose
  dispatches racing" edge case — no dead end, no contradictory simultaneous
  outcomes for the same item.

---

## Surface 15: Diagnostic session itself stalls after dispatch succeeded

```
┌─ Diagnose outcome ───────────────────────────────────────────────────────┐
│ ⚠ Diagnosis stopped without a completion signal. Open the session to see │
│   what it found, then diagnose again if needed.        🔗 Open session   │
└────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Distinct from Surface 9 (dispatch failed before a session existed) — here, a
   `headless_diagnostic` Synthetic Session was created and then ended without
   a completion signal.
2. Copy directly adapts the existing `AUTONOMOUS_STUCK` precedent
   (`stuck-item-autonomous-stuck-copy`, `StuckItemDetail.tsx` line 228): name
   the mechanism, point at where to look, name the recovery lever ("diagnose
   again" — re-triggering the Diagnose button, not the stalled session itself).
3. Per `research/ux.md` §4, this is intentionally **not** a new bespoke
   handling path — the existing stuck-item machinery is the recommended home
   for surfacing a diagnostic session that itself gets stuck (re-diagnosing a
   Diagnose session is explicitly out of scope; the exit path here is simply
   "try Diagnose again," which dispatches a fresh session).

### Error/edge cases
- No dead end: "Open session" always works (it's a real, if stalled, session
  record); re-clicking Diagnose from the item detail always dispatches anew.

---

## Surface 16: Result viewed after navigating away and back

```
Tyler opens the item detail page well after a nudge already happened
(no approval step existed to notify him in the moment):

┌─ StuckItemDetail (revisited later) ────────────────────────────────────┐
│  [Diagnose]                                                             │
│  🟢 Diagnosed 47m ago — nudged the session.        🔗 View diagnosis    │
└──────────────────────────────────────────────────────────────────────────┘
        ▲ identical to what would have rendered live — sourced from
          persisted DiagnoseDispatch state, not client-only memory
```

### Interaction flow
1. On mount, `DiagnoseOutcomeDisplay` and `DiagnoseHistoryList` both fetch from
   the server (`ListDiagnoseDispatches` / the item's latest dispatch), never
   from a client-side store that only existed during the original session.
2. A page refresh must show the same outcome as was live moments earlier — this
   is the single most important precedent carried over from the sibling
   feature's "duration/timestamps always sourced from persisted fields, never
   process-uptime" rule, applied here to the entire outcome record, not just a
   timestamp.

### Error/edge cases
- This is the central trust mechanism for the no-approval-gate design
  (`research/ux.md` §5's "trust-building for an action he didn't approve in the
  moment") — if this surface were client-state-only, a nudge that happened
  while Tyler wasn't looking would be effectively invisible after a refresh,
  reintroducing exactly the "did the automation even try?" anxiety the feature
  exists to resolve.

---

## UX Acceptance Criteria

### Task completion
1. User can trigger a diagnosis from either `StuckItemDetail` or
   `BacklogItemDetail` in **1 click** (Diagnose button) — no navigation, no
   confirmation dialog (Surfaces 1, 2).
2. User can reach the diagnostic session's full record from the item's glance-
   level outcome chip in **1 click** ("View diagnosis" link — Surfaces 4, 11,
   13).
3. User can retry a failed dispatch in **1 click** (Retry button on the
   "dispatch failed" state — Surface 9) with no need to reload the page.
4. User can view an item's full diagnose history (not just the latest outcome)
   without leaving the item's detail view — **0 additional clicks** beyond
   expanding the item itself (`DiagnoseHistoryList` renders inline — Surface
   11).
5. User can tell, before clicking Diagnose, whether nudging is currently
   enabled — **0 clicks** (Surface 10's proactive banner renders unconditionally
   when the flag is off, no hover/expand required).

### Error states
6. A failed dispatch shows the literal string **"Couldn't start diagnosis —
   \<reason if known\>. Try again."** and offers a **Retry** button (Surface 9).
7. A safety-gate skip names the **specific** gate that fired — "session wasn't
   idle," "identity check failed," "nudge cap reached: N/cap," or "cooldown
   active, next eligible \<time\>" — never a generic "skipped" (Surfaces 5, 6).
8. A bug-filed outcome links directly to the filed bug; an inconclusive outcome
   links directly to the posted diagnostic note (Surfaces 7, 8).
9. A diagnostic session that stalls after dispatch shows distinct copy from a
   dispatch that failed outright, adapted from the existing `AUTONOMOUS_STUCK`
   copy, and names the recovery lever ("diagnose again") (Surface 15).
10. A failed `ListDiagnoseDispatches` history fetch shows its own "Couldn't load
    diagnosis history" inline message with Retry — never a silently empty list
    that could be misread as "never diagnosed" (Surface 11).

### No dead ends
11. Every error/degraded state above (6, 9, 10) has a visible next action
    (Retry, "diagnose again," or a working link) — none require a page reload.
12. A duplicate/racing dispatch never produces two contradictory simultaneous
    outcomes for the same item — the second attempt reuses the in-flight state
    (Surface 14).
13. The nudging-disabled banner (Surface 10) is never a dead end masquerading as
    a limitation — the Diagnose button remains fully clickable and functional
    while it's shown; only the nudge-shaped subset of outcomes is unreachable.

### Accessibility
14. The Diagnose button is a real `<button>`, Tab-reachable, activated via
    Enter/Space, with `aria-label="Diagnose this stuck item"` (`StuckItemDetail`)
    or `aria-label="Diagnose this item"` (`BacklogItemDetail`) (Surfaces 1, 2).
15. While in flight, the button exposes `aria-busy="true"`, is `disabled`, and
    its visible label reads "Diagnosing…" — never a spinner icon alone
    (Surface 3).
16. Routine outcome settles (Nudged, Skipped, Bug filed, Inconclusive) use
    `aria-live="polite"`; dispatch-failed and any nudge-attempted-but-errored
    state use `role="alert"`/`aria-live="assertive"` — matching
    `StuckItemDetail.tsx`'s existing `overrideState === "error"` convention
    exactly (Surfaces 4-9).
17. Each of the seven outcome states pairs an icon with a distinct text label —
    verified by the "remove all color, state is still legible" test (sibling AC
    18, applied here).
18. `DiagnoseHistoryList` entries use `role="list"`/`role="listitem"` and are
    individually keyboard-focusable, matching `ActivityLogSection`'s existing
    pattern exactly (Surface 11).
19. All new chip/badge text-on-background color pairs meet WCAG AA 4.5:1
    contrast, consistent with the existing Axe Core CI gate on `web-app/src/`.
20. Keyboard-only full flow: Tab to Diagnose → Enter/Space to trigger → focus
    remains on the button (which now shows "Diagnosing…") → on settle, the
    outcome display and history list are reachable by continuing to Tab, with
    no focus loss to `<body>` at any transition.

### Persistence / no false confidence
21. A page refresh after a nudge shows the identical outcome as was live
    moments before the refresh — sourced from persisted `DiagnoseDispatch`
    state via `ListDiagnoseDispatches`, never client-only memory (Surface 16).
22. No outcome state ever implies the underlying stuck condition is *resolved*
    — "Nudged" means "the agent took the nudge action," not "the item is now
    fixed"; color/icon choices avoid a green "all good" implication (Surfaces
    4, `research/ux.md` §2's explicit constraint).
23. The nudge-cap/cooldown counter (Surface 6) is visible on the item itself
    (via the outcome display and history list), not only derivable from
    digging through structured logs — directly implements the "no silent
    auto-retry with no visible counter" anti-pattern fix (`research/ux.md` §1).

---

## Traceability to requirements.md and research/ux.md

| Requirement / decision | Surface(s) |
|---|---|
| Diagnose action on `StuckItemDetail.tsx` and `BacklogItemDetail.tsx` | 1, 2 |
| Three-way outcome split (nudged / skipped-unsafe / bug-filed) each equally visible | 4, 5, 7 |
| "Dispatch failed" distinct from "ran and found nothing" | 9 vs. 4-8 |
| Nudge cap/cooldown configurable, UI names which fired | 6 |
| Nudging disabled (flag OFF) is its own distinct, proactive state | 10 |
| Structured logging *is* the audit trail (no approval gate) → UI must be equally durable | 11, 16 |
| `headless_diagnostic` Synthetic Session reuse (zero new classification code) | 13 |
| `ActivityLogSection` reuse for inconclusive notes | 8, 11 |
| `GateVerdictBox`/`TriageReviewPanel` `readOnly` reuse for structured outcomes | 4, 5, 6, 7 |
| Diagnostic session stalling (adapts `AUTONOMOUS_STUCK` copy) | 15 |
| Duplicate/racing dispatch handling | 14 |
| Never implies resolution on a best-effort nudge | 4, AC 22 |
| Match sibling feature's visual/interaction conventions | Header note, throughout |
