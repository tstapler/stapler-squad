# UX Design: Escalation Reasoning on Review Queue Items

Phase "design" artifact for `project_plans/escalation-reasoning/`. Wireframes and flows for
exactly what `implementation/plan.md` specifies — grounded by re-reading the current
`ReviewQueuePanel.tsx` (lines 683-853, 1344-1424) and `ApprovalAnalyticsPanel.tsx` (lines
90-350) directly, not just `research/ux.md`'s prior pass. This doc does not re-litigate plan
decisions (taxonomy, storage, gating logic); it visualizes them and flags anything left
UX-incomplete.

## Surface inventory

| # | Surface | File | Status |
|---|---|---|---|
| A | Review-queue card — reason line + Create Rule button | `web-app/src/components/sessions/ReviewQueuePanel.tsx` | New render branch + gating change |
| B | `SuggestedRuleCard` modal flow | `web-app/src/components/sessions/ReviewQueuePanel.tsx` (modal, lines 1344-1424) | Unchanged internals, newly gated trigger |
| C | "Escalation Reasons" breakdown table | `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | New table section |

---

## Surface A: Review-queue card

### A1. Wireframe — no-match escalation (Create Rule button visible)

```
┌─────────────────────────────────────────────────────────────────┐
│ my-session-branch                              [🔴 Needs Review]│ ← itemHeader (compact badge)
│                                                                   │
│ [🔴 High Priority · Approval Pending]                            │ ← ReviewQueueBadge (full)
│                                                                   │
│ ❓ No auto-approval rule matched this command — review it, then   │ ← itemContext (NEW, id=
│    optionally create a rule so similar requests don't need you   │   escalation-reason-<id>)
│    next time.                                                    │
│                                                                   │
│ ┌───────────────────────────────────────────────────────────┐  │
│ │ rm -rf /tmp/foo                                             │  │ ← commandPreview <pre>
│ └───────────────────────────────────────────────────────────┘  │
│ Directory: /home/user/worktrees/my-session                       │ ← detailRow (cwd)
│                                                                   │
│ Program: claude          Branch: feature/foo                     │ ← sessionDetails
│ Path: /home/user/worktrees/my-session                             │
│                                                                   │
│ Last Activity: 2m ago                              +12 -3        │ ← itemFooter
├─────────────────────────────────────────────────────────────────┤
│  [✓ Approve]   [✗ Deny]   [✦ Create Rule]                        │ ← itemActions (sibling,
│   primary       danger      secondary (was ghost)                │   outside role="button" div)
└─────────────────────────────────────────────────────────────────┘
```

### A2. Wireframe — explicit-rule escalation (Create Rule button absent)

```
┌─────────────────────────────────────────────────────────────────┐
│ my-session-branch                              [🔴 Needs Review]│
│ [🔴 High Priority · Approval Pending]                            │
│                                                                   │
│ 🛑 Branch deletion modifies repository structure and should be   │ ← rule's own Reason text,
│    reviewed.                                                     │   verbatim, no re-wrapping
│                                                                   │
│ ┌───────────────────────────────────────────────────────────┐  │
│ │ git branch -d feature/foo                                    │  │
│ └───────────────────────────────────────────────────────────┘  │
│ ...                                                              │
├─────────────────────────────────────────────────────────────────┤
│  [✓ Approve]   [✗ Deny]                                          │ ← no Create Rule — a rule
│   primary       danger                                          │   already exists and fired
└─────────────────────────────────────────────────────────────────┘
```

### A3. Wireframe — domain-age escalation (Create Rule button absent)

```
│ 🌐 This request targets a domain that was registered very        │
│    recently — a common signal for risky or unfamiliar            │
│    destinations.                                                 │
```
Same card shape as A2 — Approve/Deny only, no Create Rule (per plan Story 3.2.1: "no
'prevent this next time' story a pattern rule can express" for domain-age).

### A4. Wireframe — unclassifiable escalation (Create Rule button absent)

```
│ ⚙️ This command couldn't be automatically classified (shell       │
│    expansion/variable substitution) — needs a human read.        │
```

### A5. Wireframe — permanently-absent reason (orphaned, pre-feature approval)

```
│ Reason not recorded — this request predates escalation-reason    │ ← no emoji (no category
│    tracking.                                                     │   known), plain itemContext
```
Still shown for every `pending_approval_id` card — never silently omitted (an unexplained
gap next to cards that *do* show a reason would read as broken UI).

### A6. Interaction flow

```
Poller enriches ReviewItem.Metadata (synchronous with store.Create — no loading state)
        │
        ▼
Reviewer opens /review-queue
        │
        ▼
Card renders: badge → reason line (NEW) → command preview → cwd → session details → footer
        │
        ├─ reason category == "no-match" ─────────────► Create Rule button rendered
        │                                                        │
        │                                                        ▼
        │                                          Reviewer clicks "✦ Create Rule"
        │                                                        │
        │                                                        ▼
        │                                     Modal opens (Surface B) — see below
        │
        └─ reason category != "no-match" ─────────────► Only Approve / Deny shown
                                                                   │
                    Reviewer clicks Approve or Deny ◄─────────────┘
                                    │
                                    ▼
                    approveRequest()/denyRequest() → acknowledgeSession()
                                    │
                                    ▼
                    Card removed from queue / advances to next item
```

Reason line placement is deliberate: it sits **above** `commandPreview`, so the reviewer's
read order is "why am I looking at this" → "what exactly does it want to do" → "what
environment is it in" — priming the read of the raw command rather than following it.

### A7. Error / edge-case handling

| Case | Handling | Source |
|---|---|---|
| Reason present, known category | Emoji + verbatim backend text | Plan Story 3.1.1 |
| `pending_approval_id` present, `escalation_reason` key absent (orphaned, pre-feature) | Fallback sentence, no emoji | Plan Story 3.1.1, confirmed only edge case in scope |
| Reason present but category unrecognized (future taxonomy drift) | `ESCALATION_REASON_EMOJI[cat] ?? ""` — text renders with **no emoji prefix**, not a blank line or crash | Task 3.1.1b's `?? ""` fallback — **not explicitly tested in the plan; flagged below** |
| `escalation_reason_category` present but not `"no-match"` and `tool_input_command` also present | Create Rule button correctly absent (AND-gated) | Plan Story 3.2.1 |
| Extremely long `escalation_reason` text (e.g. a verbose rule `Reason`/description) | Wraps via normal block flow — **no `maxHeight`/`overflowY` bound**, unlike `commandPreview` which explicitly caps at `6em` with scroll. **This is a real gap, not a hypothetical** — confirmed by reading `ReviewQueuePanel.css.ts:202-207`: `itemContext` has no `wordBreak`, `maxHeight`, or `overflowY`, while the sibling `commandPreview` block (`:209-223`) explicitly has all three. A long rule-authored `Reason` string can grow the card without bound and push the action row far down, unlike the command preview which self-limits. **Flagged as UX-incomplete — see "Gaps for plan/implementation" below.** |
| Async "reason not yet computed" transient state | Confirmed not reachable — poller enrichment is synchronous with `store.Create` (plan Story 3.1.1, 4th AC) — no loading-state UI needed, and none is built |
| Keyboard/screen-reader navigation | Reason `<p>` is non-interactive (no `tabIndex`), referenced via `aria-describedby` from the card's `role="button"` wrapper — extends the card's accessible description without adding a tab stop | Plan Task 3.1.1c |

---

## Surface B: `SuggestedRuleCard` modal (existing flow, newly gated trigger)

### B1. Wireframe (unchanged internals — trigger condition is what's new)

```
┌──────────────────────────────────────────────────┐
│  Create Auto-Approval Rule                    [✕]│ ← createPortal → document.body
├──────────────────────────────────────────────────┤   role="dialog" aria-modal="true"
│  ⏳ Generating suggestion…                         │   aria-label="Create Auto-Approval Rule"
│      — or, once loaded —                          │
│  ┌──────────────────────────────────────────┐    │
│  │  SuggestedRuleCard                         │    │
│  │  Pattern: rm -rf /tmp/*                    │    │
│  │  [Accept]  [Discard]                       │    │
│  └──────────────────────────────────────────┘    │
│      — or, on accept —                             │
│  ✓ Rule saved                                      │
│      — or, on generation failure —                 │
│  ✗ <ruleError.message>                             │
└──────────────────────────────────────────────────┘
```

### B2. Interaction flow

```
Reviewer clicks "✦ Create Rule" (only visible on no-match cards, Surface A)
        │
        ▼
generateRule({source: COMMAND_SAMPLE, commandSample: metadata["tool_input_command"], toolNameFilter})
        │
        ▼
Modal opens via createPortal — "⏳ Generating suggestion…"
        │
        ├─ success ──► SuggestedRuleCard renders suggestion
        │                    │
        │        ┌───────────┴───────────┐
        │        ▼                       ▼
        │   Accept ──► "✓ Rule saved" ─► modal auto-closes (setActiveRuleItemId(null))
        │        Discard ──► modal closes, no rule created
        │
        └─ failure ──► "✗ <error message>" shown inline, modal stays open
                              │
                              ▼
                    Reviewer closes via [✕] or overlay click (both call
                    setActiveRuleItemId(null) + clearRule() — no dead end)
```

### B3. Error / edge-case handling

| Case | Handling |
|---|---|
| `GenerateSuggestedRule` RPC fails | `ruleError.message` rendered inline (`role="alert"` implied by existing `color: var(--error)` styling — **not literally `role="alert"`, see gap below**), modal stays open, `[✕]` still closes it |
| Loading takes a long time | Static "⏳ Generating suggestion…" text — no explicit timeout/cancel affordance beyond the always-present `[✕]` close |
| Reviewer clicks overlay or `[✕]` mid-generation | Modal closes immediately (`setActiveRuleItemId(null)`); the in-flight RPC is not explicitly aborted, but its result is simply not rendered — no dead end, no error surfaced to the reviewer for a request they no longer care about |

AC3's scope boundary holds: this flow is **entirely pre-existing** (`commandSample` sourcing,
modal chrome, `SuggestedRuleCard` internals). The only change is *when the trigger button
appears* (Surface A gating) — nothing here needs new wireframing beyond confirming the trigger
condition change doesn't orphan any state.

---

## Surface C: "Escalation Reasons" analytics table

### C1. Wireframe — normal case (non-zero counts across categories)

```
┌──────────────────────────────────────────────────────────────────┐
│ Approval Analytics                          [7d][14d][30d*][90d] ↻│ ← existing windowSelector
├──────────────────────────────────────────────────────────────────┤
│  [Total: 143]  [Auto-allow 68%]  [Auto-deny 12%]  [Manual 20%]    │ ← existing summary cards
├──────────────────────────────────────────────────────────────────┤
│ Daily Breakdown  |  Top Tools  |  Top Triggered Rules  |  ...      │ ← existing sections
├──────────────────────────────────────────────────────────────────┤
│ Escalation Reasons                                        (NEW)   │ ← sectionTitle
│ ┌────────────────────────────────────────┬───────┬─────────────┐│
│ │ Reason                                   │ Count │ Frequency    ││
│ ├────────────────────────────────────────┼───────┼─────────────┤│
│ │ No auto-approval rule matched            │  12   │ ████████████││ ← Bar, same idiom as
│ │ Rule explicitly flagged for review       │   5   │ █████        ││   Top Tools/Top Rules
│ │ Newly-registered domain                  │   2   │ ██           ││
│ │ Plaintext secret detected                │   1   │ █            ││
│ └────────────────────────────────────────┴───────┴─────────────┘│
└──────────────────────────────────────────────────────────────────┘
```
(`unclassifiable` row omitted here since its count is 0 — plan Story 4.2.1: "one row per
non-zero-count category, sorted descending by count".)

### C2. Wireframe — zero-escalations edge case (flagged gap, see below)

```
│ Escalation Reasons                                                │
│ ┌────────────────────────────────────────┬───────┬─────────────┐│
│ │ Reason                                   │ Count │ Frequency    ││
│ ├────────────────────────────────────────┼───────┼─────────────┤│
│ │                    (no rows — every category count is 0)        ││
│ └────────────────────────────────────────┴───────┴─────────────┘│
└──────────────────────────────────────────────────────────────────┘
```
This is a real, reachable state — e.g. a 7-day window where every decision was auto-allow/
auto-deny and nothing escalated. The plan's Story 4.2.1 AC only specifies the non-empty-row
rendering rule ("one row per non-zero-count category"); it does not specify what renders when
*all* counts are zero. As currently specced this produces a header-only table with no rows and
no explanation — visually different from every other section in this panel, which all have an
explicit empty-state message (`empty`/`emptyHint` classes, "No data for the last N days...",
used by the Daily Breakdown section at `ApprovalAnalyticsPanel.tsx:189-194`).

### C3. Interaction flow

```
Reviewer opens Approval Analytics tab
        │
        ▼
useApprovalAnalytics({windowDays}) fetches summary
        │
        ├─ loading ──► existing loadingClass ("Loading analytics…") for the whole panel
        │               (Escalation Reasons section not independently loading-gated — it
        │               renders/hides together with the rest of `summary`)
        │
        ├─ error ──► existing panel-level error banner + Retry button (errorClass, role="alert")
        │             (Escalation Reasons section does not get its own error state — it's
        │             part of the same `summary` fetch as every other section)
        │
        └─ success ──► Escalation Reasons table renders (or — per C2 — nothing, if the
                        section's own "count > 0" filter is applied but no wrapping empty-state
                        check exists per the plan as currently specced)
        │
        ▼
Reviewer clicks a different window (7d → 30d)
        │
        ▼
useApprovalAnalytics re-fetches, counts (and rows) update
```

### C4. Error / edge-case handling

| Case | Handling |
|---|---|
| Fetch fails | Panel-level `errorClass` banner + Retry (existing, shared across all sections) |
| Fetch loading | Panel-level `loadingClass` (existing, shared) |
| Non-zero counts, some categories at 0 | Zero-count categories omitted (plan spec) |
| **All categories at 0 (no escalations in window)** | **Not specified by the plan — flagged gap, see below.** |
| `secret-scan` row present | Renders with its label like any other row — it's analytics-only (never a `ReviewItem`), so this table is the *only* surface where a reviewer sees the secret-scan bucket at all |
| Very long category label | Labels are fixed, short, developer-authored strings from `ESCALATION_CATEGORY_LABELS` (not user input) — no overflow risk, unlike Surface A's `escalation_reason` free text |

---

## UX Acceptance Criteria

Each is independently testable by a human reviewer clicking through the running app.

### Surface A — Review-queue card

1. **Task completion**: A reviewer can identify *why* a no-match card was escalated and open
   the rule-creation flow in **≤ 2 clicks** from landing on `/review-queue` (1: land on page and
   read the always-visible reason line with zero clicks; 2: click "✦ Create Rule").
2. **No dead ends**: every card with `pending_approval_id` set shows a non-empty reason line —
   either the real reason or the "not recorded" fallback — never a blank gap. Verify by loading
   an orphaned pre-feature approval fixture and confirming fallback text renders.
3. **Category-correct copy**: for each of no-match / explicit-rule / domain-age /
   unclassifiable, the rendered `<p id="escalation-reason-...">` text starts with the specified
   emoji and contains the exact backend-supplied sentence, verbatim (no frontend re-wrapping).
4. **Create Rule button never appears for non-no-match categories**: given an explicit-rule,
   domain-age, or unclassifiable card with `tool_input_command` present, `create-rule-<id>` is
   absent from the DOM (`queryByTestId` returns null) — verified by Jest (plan Story 3.2.2) and
   spot-checked manually against a live domain-age escalation.
5. **No misclick risk between unequal-consequence actions**: "✓ Approve" (`intent="primary"`)
   and "✦ Create Rule" (`intent="secondary"`) are visually distinguishable at a glance —
   Approve is solid-filled, Create Rule has a border/fill but is not solid-color-filled.
   Confirm by screenshot comparison, not just class-name inspection.
6. **Reading order**: the reason line renders visually *above* the command preview on every
   pending-approval card (never below, never interleaved) — confirms the "why, then what"
   reading order the research recommends.
7. **Accessibility — screen reader**: with a screen reader active, tabbing to a review-queue
   card announces the card's accessible name/role *and* the reason text as a description (via
   `aria-describedby`) without requiring an extra tab stop.
8. **Accessibility — keyboard only**: a keyboard-only user can reach Approve/Deny/Create Rule
   via Tab without the new reason paragraph inserting an unexpected stop in the tab sequence.
9. **Accessibility — contrast**: the reason text (`itemContext`, `color: vars.color.textSecondary`,
   `italic`) meets ≥ 4.5:1 contrast against the card background in both light and dark theme —
   verify with a contrast-checker tool against the live rendered page, not just the token name.
10. **Accessibility — emoji glyph legibility**: each of ❓ 🛑 🌐 ⚙️ renders as a recognizable,
    distinct glyph (not a missing-character box/tofu) in the actual browser/font stack this app
    ships with, at the `itemContext` font size — verify by screenshot, not just confirming the
    text-color contrast of the *surrounding* text (contrast math doesn't apply to emoji glyphs,
    which are rendered by the system emoji font, not colored via CSS `color`).

### Surface B — SuggestedRuleCard modal

11. **No dead ends**: every terminal state of the modal (accept, discard, error, mid-generation
    close) has a working exit path back to the review queue with no orphaned open modal and no
    lost focus trap. Verify by triggering an RPC failure (e.g. throttle network) and confirming
    `[✕]` still closes the modal.
12. **Trigger-gating correctness**: the modal can only be opened from a no-match card's Create
    Rule button — confirm no other card exposes a code path to `generateRule(...)`.
13. **Focus management**: opening the modal moves focus into the dialog; closing it (via `[✕]`,
    overlay click, or Accept/Discard) returns focus to a sensible location (ideally the
    triggering Create Rule button) rather than dropping focus to `<body>`.

### Surface C — Escalation Reasons analytics table

14. **Task completion**: a user can identify the dominant escalation category for the selected
    window in **0 clicks** beyond selecting the window itself (table is always visible when data
    exists, sorted descending — top row is the answer).
15. **Error state**: on fetch failure, the panel shows "Failed to load analytics: `<message>`"
    and a "Retry" button that re-fetches — same as every other section in this panel (shared
    error boundary, not a per-section one).
16. **No dead ends**: the Retry button always re-attempts the fetch; there is no state where the
    panel is stuck showing a stale error with no recovery action.
17. **Zero-escalation window is explained, not blank**: when every category count is 0 for the
    selected window, the section shows an explicit message (e.g. "No escalations in this
    window.") rather than rendering a header with zero rows. **This criterion currently fails
    against the plan as specced — see "Gaps for plan/implementation" below; must be resolved
    before Surface C ships.**
18. **Window-switch consistency**: switching the time window (7d/14d/30d/90d) updates the table's
    counts and row set without a full-page reload or a flash of the *previous* window's data
    rendered under the *new* window's label.
19. **Long-tail category still visible when present**: `secret-scan` — a category this panel is
    the *only* surface for — renders correctly with its label when its count is non-zero (not
    silently dropped for being the "smallest"/analytics-only bucket).

### Cross-surface

20. **Copy never leaks internals**: no rendered string on Surface A or C contains a raw `RuleID`
    value, the literal word `"Escalate"`, or the sentinel strings `"new-domain-check"` /
    `"shell-expansion-program"` / `"secret-scan"` — every category is represented by its mapped
    human label, in both `ESCALATION_REASON_EMOJI`-prefixed card text and
    `ESCALATION_CATEGORY_LABELS`-mapped table rows.
21. **Color is never the sole differentiator**: verify by viewing both surfaces with a grayscale/
    color-blindness simulation filter — categories remain distinguishable via emoji glyph (Surface
    A) or text label (Surface C), not via color alone.

---

## Gaps for plan/implementation phase

Two items surfaced by this design pass that the plan does not currently resolve. Both are small
and additive — flagging rather than blocking, per this skill's brief ("flag anything UX-incomplete," not "re-litigate").

1. **Unbounded `escalation_reason` text on Surface A** (see A7). `itemContext`
   (`ReviewQueuePanel.css.ts:202-207`) has no `maxHeight`/`overflowY`/`wordBreak`, unlike its
   sibling `commandPreview` (`:209-223`) which explicitly bounds long content. An
   explicit-rule's `Reason` is free text set by whoever authored the `ApprovalRule` — nothing in
   the plan caps its length before it reaches this `<p>`. Recommend either (a) truncate at
   render time with a fixed character cap + `title` attribute for the full text (cheapest,
   matches "no new UI dependency" per AC6), or (b) apply the same `maxHeight`/`overflowY` pattern
   `commandPreview` already uses. Does not block AC1/AC6 compliance for the common case (backend
   `Reason` strings in the plan's own examples are one sentence), but is a real risk for
   user-authored rule descriptions.
2. **No explicit empty-state for "zero escalations in window" on Surface C** (see C2, UX-AC17).
   Every other section in `ApprovalAnalyticsPanel` (Daily Breakdown) has an explicit "No data for
   the last N days" message when its data is empty; the plan's Story 4.2.1 spec for the
   Escalation Reasons table only describes row-filtering ("one row per non-zero-count category")
   and doesn't specify a wrapping empty-state check for "all categories are zero." Recommend
   adding a one-line conditional (`Object.values(counts).every(c => c === 0)` → render
   `empty`-styled message instead of an empty `<table>`) — small, consistent with the panel's
   existing pattern, and closes UX-AC17.

Neither gap changes any AC1-AC8 numbering or scope — both are refinements inside AC4/AC6's
existing boundaries, sized to fit inside Epic 3.1/4.2's existing tasks rather than as new epics.
