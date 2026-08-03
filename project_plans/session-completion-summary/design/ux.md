# UX Design: Session Completion Summary

Concrete wireframes/flows for the architecture already chosen in
`implementation/plan.md` (Epic 3.1 `SessionSummaryPanel` + `useSessionSummary`,
Epic 3.2 tab integration, Epic 3.3 standalone route). This document does not
re-decide component boundaries, RPCs, or data shape — it specifies layout,
copy, and interaction detail for what plan.md already architected, resolving
the three items plan.md left underspecified (skeleton treatment, `aria-live`
strings, READY section ordering) and flagging one apparent coverage gap
(stale-document-plus-banner) for the coordinator.

Grounded in `requirements.md` (8 ACs) and `research/ux.md` (comparable
patterns, mental model, accessibility findings, empty-state copy, JTBD).

---

## Surface index

| # | Surface | Plan.md source |
|---|---|---|
| a | Summary tab — disabled (session running) | Story 3.2.1 |
| b | Summary tab — enabled, GENERATING | Story 3.1.2 |
| c | Summary tab — enabled, READY (main document) | Story 3.1.2 |
| d | Summary tab — enabled, ERROR | Story 3.1.2 |
| d2 | ERROR with a stale prior READY document (**gap — see §Gap**) | not addressed |
| e | Copy-to-clipboard (success / failure) | Story 3.1.2 |
| f | Regenerate (in-flight) | Story 3.1.2 / 2.2.2 |
| g | Standalone post-deletion route | Story 3.3.1 |

---

## (a) Summary tab — disabled (session still running)

```
┌─ Session: fix-login-redirect ──────────────────────────────────────┐
│ [Info] [Diff] [VCS] [Files] [Logs] [Summary▽]                      │
│                                     ▲                               │
│                        aria-disabled="true"                        │
│                        title="Summary is generated                 │
│                                after the session ends."             │
└──────────────────────────────────────────────────────────────────┘
```

**Flow**: User is viewing a live/running session. The Summary tab renders
in the strip (same position as other tabs, per the existing Browser-tab
`disabled` precedent) but is visually muted (reduced opacity, no hover
state) and not focusable via click. Hovering or focusing it (keyboard) shows
the native tooltip from `title`. It is *not* reachable via `Tab`/arrow-key
roving focus in the same way an enabled tab is — the existing tablist
roving-focus code already skips disabled tabs (no new work, per
`research/ux.md` §2/§3).

**Edge cases**:
- Session transitions from running → terminal while the user has the tab
  strip open: the tab flips from disabled → enabled in place (no page
  reload). If the user is already focused elsewhere in the tab strip, no
  focus is stolen.
- Session is terminal but `EventExited` reason was
  `reconcile-session-missing` (excluded per AC-1): the tab must **not**
  enable — it stays disabled indefinitely for that session, since no
  `SessionSummary` row will ever be created. Tooltip text remains
  unchanged; there is no user-facing distinction between "still running"
  and "will never generate" — see UX-AC-12 below, flagged as an accepted
  minor ambiguity (out of scope to fix here; the row simply never appears
  and the tab never enables, matching FR-1's exclusion).

---

## (b) Summary tab — enabled, GENERATING

```
┌─ Session: fix-login-redirect ──────────────────────────────────────┐
│ [Info] [Diff] [VCS] [Files] [Logs] [Summary●]                      │
├──────────────────────────────────────────────────────────────────┤
│  Session Summary: fix-login-redirect      ⟳ Generating…            │
│  ────────────────────────────────────────────────────────────     │
│  [Copy as Markdown] (disabled)      [Regenerate] (hidden)          │
│                                                                      │
│  ▓▓▓▓▓ ▓▓▓▓▓ ▓▓▓▓▓ ▓▓▓▓▓ ▓▓▓▓▓        ← Decisions placeholder:      │
│                                          heading bar (▓▓▓▓▓▓▓▓▓▓▓)  │
│                                          + 5 pill blocks            │
│                                                                      │
│  ▓▓▓▓▓▓▓▓▓▓▓▓                          ← What Was Done placeholder:│
│  ████████████████████████████████        heading bar               │
│  ████████████████████████████            + 3 text-line bars        │
│  ███████████████████                       (last one 60% width)    │
│                                                                      │
│  ▓▓▓▓▓▓▓▓▓                              ← Changes placeholder:      │
│  ████████████████████                     heading bar               │
│  ██████████                                + stat-line bar          │
│                                             + link-line bar (40%)   │
│                                                                      │
│  ▓▓▓▓▓▓▓▓                                ← Timeline placeholder:    │
│  ████████████████                           heading + 1 line bar   │
│                                                                      │
│  ▓▓▓▓▓▓▓▓▓▓▓▓                            ← Token Usage placeholder: │
│  ████████████████                           heading + 1 line bar   │
│                                                                      │
│  role="region" aria-busy="true"                                    │
│  <span aria-live="polite">Generating summary…</span>  (visually     │
│  hidden or shown next to the ⟳ icon — text must exist either way)  │
└──────────────────────────────────────────────────────────────────┘
```

### Exact skeleton spec (resolves plan.md's underspecified GENERATING treatment)

Five section placeholders, each a heading-shaped bar plus content bars —
**17 skeleton blocks total**, testable via `data-testid="summary-skeleton-block"`
count:

| Placeholder | Heading bar | Content bars | Shape notes |
|---|---|---|---|
| Decisions | 1 (≈140px × 16px) | 5 pill blocks (≈60px × 24px), one per decision category, laid out in a row | Mimics the "at a glance" stat strip (§READY ordering below) so the transition to READY doesn't reflow |
| What Was Done | 1 | 3 full-width text-line bars, last at 60% width | Mimics a 3-sentence paragraph |
| Changes | 1 | 1 stat-line bar (full width) + 1 link-line bar (40% width) | Stat line + diff link shape |
| Timeline | 1 | 1 text-line bar | Single "started/stopped/duration" line |
| Token Usage | 1 | 1 text-line bar | Single "tokens/cost" line |

- 5 heading bars + (5 + 3 + 2 + 1 + 1) = 12 content bars = **17 blocks**.
- All blocks use a shimmer animation (CSS `background-position` keyframe)
  **unless** `prefers-reduced-motion: reduce`, in which case blocks render
  static (no animation) — still visually present as gray blocks.
- Copy button is rendered but `disabled` (no markdown to copy yet).
  Regenerate button is not rendered at all while GENERATING (there is
  nothing to regenerate *from* yet in this state — Regenerate only appears
  in ERROR, and is disabled-in-place when clicked from READY; see (f)).
- The panel's outer container carries `role="region" aria-busy="true"
  aria-label="Session summary"`; a visually-present status line reads
  "Generating summary…" and is wrapped in `aria-live="polite"` so a screen
  reader announces it once, and again on transition to READY/ERROR.

**Flow**: user clicks the newly-enabled Summary tab moments after the
session stops. `useSessionSummary` has already started polling
(`GetSessionSummary` every 2s) as soon as the hook mounted — no click-to-load
delay. Skeleton shows immediately if `status` is `PENDING` or `GENERATING`.
No user action is available besides navigating away; there is intentionally
no "cancel generation" affordance (not in any AC).

**Edge cases**:
- If the row doesn't exist yet at all (listener dispatched `go
  GenerateAndPersist` but the first upsert to `status: "generating"` hasn't
  landed yet — a few-millisecond window), `GetSessionSummary` returns
  `Summary: nil`. The panel treats `nil` identically to `PENDING`/`GENERATING`
  (same skeleton) rather than a separate "not found" state — there is no
  user-visible distinction between "not started yet" and "in progress."
- Polling continues indefinitely while `GENERATING`; there is no client-side
  timeout in the current plan. If generation genuinely hangs past the
  server's own 5-minute staleness window (`staleGenerationTimeout`), the
  *next* `GetSessionSummary` read flips it to `ERROR` server-side (Task
  1.5.3b) and the poll loop picks that up on its next 2s tick — from the
  user's perspective, the skeleton simply changes to the ERROR state after
  up to 5 minutes with no distinct "still waiting" messaging change in
  between. This is inherent to the plan's design, not something this doc
  proposes changing, but is worth the coordinator knowing: a session that
  hangs mid-generation shows an unchanging "Generating…" skeleton for up to
  5 minutes with no visible countdown or escalating message.

---

## (c) Summary tab — enabled, READY (main document)

```
┌─ Session: fix-login-redirect ──────────────────────────────────────┐
│ [Info] [Diff] [VCS] [Files] [Logs] [Summary✓]                      │
├──────────────────────────────────────────────────────────────────┤
│  Session Summary: fix-login-redirect     ✓ Ready · generated 2m ago│
│  ────────────────────────────────────────────────────────────     │
│  [📋 Copy as Markdown]                          [↻ Regenerate]     │
│  aria-label="Copy summary as Markdown"                              │
│                                                                      │
│  ┌─ Decisions at a glance ─────────────────────────────────────┐  │
│  │  ✓ 5 auto-approved   ✓ 1 manually approved   ✕ 0 denied      │  │
│  │  ◔ 1 review-resolved   ● 0 still open                        │  │
│  │  (icon + text pairing — never color alone, per a11y note)    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ## What Was Done                                                   │
│  Fixed the login redirect loop by correcting the callback URL       │
│  comparison in AuthGuard.tsx and adding a regression test...        │
│                                                                      │
│  ## Changes                                                         │
│  3 files changed, +42 −7 lines · [View full diff →]                 │
│                                                                      │
│  ## Decisions                                                       │
│  5 auto-approved (62.5%) · 1 manually approved (12.5%) ·            │
│  0 denied (0%) · 1 review-queue-resolved (12.5%) ·                  │
│  1 still open (12.5%)                                               │
│                                                                      │
│  ## Timeline                                                        │
│  Started 10:04:12 · Stopped 10:19:47 · Duration 15m 35s             │
│                                                                      │
│  ## Token Usage                                                     │
│  128,000 tokens · Est. cost $1.92                                   │
└──────────────────────────────────────────────────────────────────┘
```

### Exact READY section ordering (resolves plan.md's underspecified ordering)

Two orderings exist for two different jobs, and they are **intentionally
different**:

1. **Exported/copied markdown** (`data.markdown`, what `RenderSessionSummaryMarkdown`
   produces, what gets pasted as a PR body) keeps plan.md's Task 1.5.1a
   order unchanged: **What Was Done → Changes → Decisions → Timeline →
   Token Usage.** This reads naturally as a PR description (context, then
   diff, then approvals-as-provenance, then metadata) and this document
   does not propose changing it — changing export order is out of scope
   for a UX pass and would need re-validation against AC-4's "reusable as
   PR body" wording.
2. **In-app rendered panel** adds one visual element *above* the flowing
   markdown body that the exported document does not have: a **"Decisions
   at a glance" card**, built from the structured `data.decisions` proto
   fields (not parsed from markdown), pinned directly under the toolbar and
   above the narrative. This satisfies `research/ux.md` §5's "unmissable,
   not buried" requirement for the emotional/accountability job (scanning
   for denied/still-open items) without changing what gets copied. The
   flowing markdown body below it still contains its own "## Decisions"
   section, in its original position — the glance-card is additive, not a
   replacement, and a user who copies the doc still gets the single
   canonical section order from item 1.

Final in-app visual order top to bottom: **Header/toolbar → Decisions-at-a-glance
card → rendered markdown (What Was Done → Changes → Decisions → Timeline →
Token Usage).**

**Flow**: transition from GENERATING happens in place — no page reload, no
scroll-position reset. The `aria-live="polite"` region announces `"Summary
ready."` once. The skeleton blocks are replaced by their corresponding real
content in the same DOM position (per §b's shape-matching design, this
avoids a layout jump).

**Empty-state rendering** (FR-6, minimal-activity sessions) — every section
still renders in its normal slot with this exact copy, never omitted or
grayed out (verbatim from `research/ux.md` §4, confirmed unchanged by
plan.md):

| Section | Empty-state text |
|---|---|
| What Was Done | "This session ended before any work was recorded." |
| Changes | "No files were changed." |
| Decisions | "No approval requests occurred during this session." |
| Timeline | Always populated; if duration rounds to zero, show "Duration: <1s" |
| Token Usage | "No tokens were used." (zero-and-available) or "Cost data unavailable." (`DataUnavailable: true`) — distinct copy, never conflated |

The Decisions-at-a-glance card also has an empty variant: instead of five
stat pills, it renders a single row: "No approval requests occurred during
this session." — same slot, same visual weight as the populated card, not
a smaller/muted variant.

---

## (d) Summary tab — enabled, ERROR (no prior document)

```
┌─ Session: fix-login-redirect ──────────────────────────────────────┐
│ [Info] [Diff] [VCS] [Files] [Logs] [Summary!]                      │
├──────────────────────────────────────────────────────────────────┤
│  Session Summary: fix-login-redirect          ⚠ Failed             │
│  ────────────────────────────────────────────────────────────     │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  ⚠  Couldn't finish generating this summary.                 │  │
│  │                                                                │  │
│  │  Failed while computing approval decisions.                  │  │
│  │  Last attempt: Aug 3, 2026, 10:20 AM                          │  │
│  │                                                                │  │
│  │  [ ↻ Regenerate ]                                             │  │
│  │                                                                │  │
│  │  ▸ Details                        (collapsed disclosure)      │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  <span aria-live="polite">Summary generation failed: Failed        │
│   while computing approval decisions.</span>                       │
└──────────────────────────────────────────────────────────────────┘
```

**Flow**: `status: ERROR` maps `error_stage` to a plain-language lead
sentence via a small stage→copy table (not the raw `error_stage` enum
string):

| `error_stage` | Displayed sentence |
|---|---|
| `"decisions"` | "Failed while computing approval decisions." |
| `"diff"` | "Failed while computing the diff summary." |
| `"restart-interrupted"` | "Generation was interrupted, possibly by a server restart." |
| *(anything else / unmapped)* | "Something went wrong while generating this summary." |

`error_message` (the raw backend string) is never shown inline in the
primary text — it's placed inside the collapsed "▸ Details" disclosure
(`<details>`/`<summary>`), per `research/ux.md` §4's "no raw stack
traces in the primary error text" guidance. The Regenerate button is
prominent (primary-styled), not a text link, matching "primary action" per
research §4.

**No dead end**: Regenerate is always present and always actionable from
this state (see (f) for its own in-flight sub-state) — this is the state's
one and only recovery path, and it is never hidden or disabled except while
a regenerate attempt is itself in flight.

---

## (d2) ERROR with a stale prior READY document — GAP, flagged explicitly

**This sub-state is not addressed anywhere in plan.md's Story 3.1.2.**
`research/ux.md` §4 explicitly proposes it:

> "if a *prior* successful generation exists (e.g. Regenerate itself failed
> but an earlier READY doc is still stored), show the stale document with a
> banner ... rather than replacing a working doc with a bare error."

Plan.md's Story 3.1.2 AC only branches on `status: ERROR` → show
error/Regenerate (surface (d) above), with no conditional on whether
`data.markdown`/`data.narrative` are non-empty at the time of that error.
Task 1.5.2b (step 3, backend) upserts `status: "error"` on a
`BuildDecisionsSnapshot` failure but doesn't specify whether the *previous*
row's `markdown`/`narrative` field values survive that upsert untouched —
ent's `Upsert...` API only overwrites the columns explicitly set in the
update, so the previous `markdown` value would survive at the **data**
layer by default ent semantics, but nothing in Story 1.5.2/2.2.1/3.1.2
specifies exposing it or rendering it. As written, the plan's frontend
branch is binary (`status === "READY"` vs `status === "ERROR"`) and would
show the bare ERROR card even when `data.markdown` from a prior successful
generation is sitting right there in the same response payload.

**Recommended design** (for the coordinator/planning phase to accept or
reject — not something this UX pass is authorized to silently fold into
plan.md):

```
┌─ Session: fix-login-redirect ──────────────────────────────────────┐
│ [Info] [Diff] [VCS] [Files] [Logs] [Summary!]                      │
├──────────────────────────────────────────────────────────────────┤
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ ⚠ Showing the summary from the last successful generation,  │  │
│  │   dated Aug 3, 2026, 10:04 AM. A regeneration attempt at     │  │
│  │   10:20 AM failed — see Details.        ▸ Details            │  │
│  │                                          [ ↻ Try again ]     │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌─ Decisions at a glance ─────────────────────────────────────┐  │
│  │  ✓ 5 auto-approved   ✓ 1 manually approved   ✕ 0 denied ...  │  │
│  └────────────────────────────────────────────────────────────┘  │
│  ## What Was Done  ...   (the stale but still-valid document,     │
│  ## Changes ...            rendered exactly as it would be in     │
│  ## Decisions ...          the READY state — surface (c) reused)  │
│  ## Timeline ...                                                   │
│  ## Token Usage ...                                                │
└──────────────────────────────────────────────────────────────────┘
```

- Banner sits where the ERROR card sat in surface (d), but the rest of the
  page reuses surface (c)'s READY rendering unchanged (same
  Decisions-at-a-glance card, same markdown body) — this state is
  literally "(c) plus a dismissible-feeling but persistent banner," not a
  new document layout.
  the [Regenerate] button is now [↻ Try again] inside the banner, not a
  second button in the toolbar, to avoid two competing regenerate
  affordances on screen at once.
- `aria-live` announcement on entering this sub-state: `"Showing the
  previous summary. Regeneration failed — see the banner for details."`
- This sub-state requires: (1) the frontend to branch on `status ===
  "ERROR" && data.markdown` (non-empty) rather than only `status`, and (2)
  confirmation that the backend upsert in Task 1.5.2b genuinely preserves
  the prior row's `markdown`/`narrative`/`diff`/`decisions`/etc. fields
  when only `status`/`error_stage`/`error_message` are written on failure
  (i.e., the upsert must be field-scoped, not a full-row replace that
  would null out the stale content). **Neither of these is currently
  specified as a task in plan.md** — implementing this sub-state would
  need a new task in Epic 1.5 (backend: confirm/guarantee field-scoped
  upsert-on-error) and a task in Story 3.1.2 (frontend: stale-doc-plus-banner
  branch), or an explicit decision to accept the gap and always show the
  bare error card even when a usable stale document exists.

---

## (e) Copy-to-clipboard — success and failure

```
Success:
┌────────────────────────────────────────────┐
│  [📋 Copy as Markdown]  →  [✓ Copied]        │   (glyph/label swaps back
│                                                │    to default after 1.5s)
└────────────────────────────────────────────┘
<span aria-live="polite">Summary copied to clipboard.</span>

Failure:
┌────────────────────────────────────────────┐
│  [📋 Copy as Markdown]  (no visual swap)      │
│  ⚠ Copy failed — select the text below       │
│    and copy manually.                        │
└────────────────────────────────────────────┘
<span aria-live="polite">Copy failed. Select the text and
copy manually.</span>
```

### Exact `aria-live` strings (resolves plan.md's unspecified literal text)

| Event | Literal `aria-live="polite"` announcement text |
|---|---|
| Copy succeeds | `"Summary copied to clipboard."` |
| Copy fails | `"Copy failed. Select the text and copy manually."` |
| Generation starts / in progress | `"Generating summary…"` |
| Generation completes (→ READY) | `"Summary ready."` |
| Generation fails (→ ERROR, no stale doc) | `"Summary generation failed: {stage sentence}"` e.g. `"Summary generation failed: Failed while computing approval decisions."` |
| Regenerate clicked, request accepted | `"Regenerating summary…"` |
| Regenerate succeeds | `"Summary regenerated."` |
| Regenerate fails, stale doc shown (d2, if implemented) | `"Showing the previous summary. Regeneration failed — see the banner for details."` |

All of these share **one** `aria-live="polite"` status node per panel
(not a new node per event) so a screen reader user only ever has one live
region to track, matching the existing `SessionDetailView.tsx:495,697`
precedent of a single reusable live region rather than one per feature.

**Flow — success**: user clicks "Copy as Markdown" (`aria-label="Copy
summary as Markdown"`) → `navigator.clipboard.writeText(data.markdown)`
resolves → button glyph/label swaps to "✓ Copied" for 1.5s (existing visual
pattern, kept) → **and**, new vs. the existing pattern, the shared
`aria-live` region's text is set to `"Summary copied to clipboard."` at the
same moment, then cleared (or left, next event overwrites it) after the
visual swap reverts.

**Flow — failure** (permission denied, insecure context, etc.): the
`writeText` promise rejects → **unlike the existing
`SessionDetailView.tsx:320-329` pattern** (which only does
`console.warn`), the button does *not* silently no-op. It shows a
user-visible inline message "Copy failed — select the text below and copy
manually." beneath the toolbar, and the `aria-live` region announces
`"Copy failed. Select the text and copy manually."` The failure message
persists (does not auto-dismiss on a timer, unlike the success glyph swap)
since there's no successful outcome to time out to; it clears on the next
successful copy attempt or on navigating away from the tab.

**No dead end**: the failure message's "select the text below" is
actionable because the rendered markdown body (surface (c)) is always
selectable plain-rendered text — there is no case where copy fails *and*
the underlying content is inaccessible to manual selection.

---

## (f) Regenerate — in-flight state

```
Before click (ERROR state, surface (d)):
  [ ↻ Regenerate ]                     ← enabled, primary-styled

Immediately after click:
  [ ⟳ Regenerating… ]                  ← disabled, spinner replaces icon
  <span aria-live="polite">Regenerating summary…</span>

  (tab content area: stays on the ERROR card layout — does NOT flash to
   the GENERATING skeleton from surface (b); the button's own state is
   the only in-flight indicator, since the panel already has content to
   show and doesn't need to blank itself)

On success:
  → transitions to surface (c), READY, aria-live: "Summary regenerated."

On failure (no stale doc — same as first-ever failure):
  → returns to surface (d), ERROR, updated timestamp + error_stage,
    aria-live: "Summary generation failed: {stage sentence}"

On failure (stale doc exists — surface (d2), if implemented):
  → returns to surface (d2)'s banner-over-stale-doc layout,
    aria-live: "Showing the previous summary. Regeneration failed —
    see the banner for details."
```

**Flow**: click → `regenerate()` (hook) calls `RegenerateSessionSummary` →
button immediately flips to `disabled` + "Regenerating…" + spinner glyph,
*before* the RPC response returns (optimistic disable, not waiting on the
network round-trip) → hook resumes 2s polling (mirroring surface (b)'s poll
loop) → on the next poll tick where `status` is no longer `GENERATING`, the
button re-enables and the panel re-renders per whichever terminal state was
reached.

**Idempotency UI (AC-8)**: if the user double-clicks Regenerate (or clicks
it, then a duplicate `EventExited` also fires server-side for the same
session), the *button* is already disabled after the first click, so a
second click is inert client-side. Server-side, the dedup guard (Task
1.5.3a) independently rejects the second trigger regardless of what the UI
does — the UI-side disable is a UX affordance, not the sole idempotency
mechanism, so there is no risk of a race between "the button looked
disabled" and "a second pipeline actually ran."

**No dead end**: if the poll loop never observes a terminal status (server
hang), the button remains "Regenerating…" indefinitely with no client-side
timeout specified in the current plan — this mirrors the same unbounded-wait
gap noted in surface (b)'s edge cases, inherited here rather than newly
introduced by Regenerate specifically.

---

## (g) Standalone route — `/sessions/[sessionId]/summary`

```
┌──────────────────────────────────────────────────────────────────┐
│  ← Back                                                             │
│                                                                      │
│  Session Summary: fix-login-redirect                                │
│  (no Info/Diff/VCS/Files/Logs/Browser tab strip — this route only   │
│   ever renders the summary content, per plan.md's Epic 3.3          │
│   "minimal page shell" design)                                      │
│  ────────────────────────────────────────────────────────────      │
│  [Same SessionSummaryPanel content as surface (c)/(b)/(d)/(d2),     │
│   reused verbatim — this route is a thin wrapper, not a new         │
│   layout]                                                            │
└──────────────────────────────────────────────────────────────────┘
```

**Flow**: user navigates directly to `/sessions/<uuid>/summary` — e.g. from
a bookmarked link, a link shared before the session was deleted, or typed
manually. The page reads `sessionId` from the route param and renders
`SessionSummaryPanel` directly; it does not attempt to look up a live
`Session` object first (per the plan's explicit design goal — no dependency
on `useAppSelector(selectAllSessions)`). All of surfaces (b)/(c)/(d)/(d2)/(e)/(f)
apply identically inside this shell — the only difference from the tab
context is the absence of sibling tabs and the presence of a "← Back" link.

**Edge cases**:
- `sessionId` does not correspond to any `SessionSummary` row at all (typo'd
  URL, or a session that ended via an excluded reason like
  `reconcile-session-missing` and therefore never got a row, or a session
  that never reached a terminal state and was deleted by some other path
  entirely): `GetSessionSummary` returns `Summary: nil` and this route's
  wrapper must render a distinct **"No summary available for this session"**
  message with the "← Back" link as the only affordance — this is *not*
  the same as the GENERATING skeleton (surface (b)), since on this route
  there is no session-still-running context to justify an indefinite
  polling skeleton. **This 404-shaped case is not explicitly covered by
  plan.md's Story 3.3.1** (which only specs the found-and-READY path) —
  flagged here as a smaller, secondary gap alongside the primary (d2) gap:
  without a distinct empty-result message, this route would otherwise show
  an indefinitely-polling GENERATING skeleton for a session that will never
  produce a document, which is a dead end (violates the "no dead ends"
  UX-AC below).
- Session still exists and is still running (user found/bookmarked the URL
  early): the route still works — `GetSessionSummary` returns `Summary:
  nil` the same way, which is indistinguishable at the RPC layer from "will
  never exist." The recommended fix for the immediately-prior bullet
  (a distinct empty-result message) would need to avoid misleading a user
  in *this* sub-case, e.g. by phrasing it as "No summary yet — this session
  may still be running, or may not have generated one." rather than a
  flat "not found," since the RPC alone can't distinguish the two without
  an additional live-session check this route was explicitly designed to
  avoid (Pattern Decisions table, "Retrieval route independent of live
  Session").

---

## UX Acceptance Criteria (human-testable)

**Task completion**
1. From a terminal session's detail view, a user can open the Summary tab
   and see either a READY document or a GENERATING skeleton in **1 click**
   (click the tab; no additional "load" step).
2. A user can copy the full markdown document to the clipboard in **1
   click** from the READY state (click "Copy as Markdown"; no menu, no
   confirmation dialog).
3. A user can trigger regeneration from the ERROR state in **1 click**
   (click "Regenerate"; no confirmation dialog required, since ACs treat
   this as a low-cost recovery action, not a destructive one).
4. A user can reach a session's summary after the session has been deleted
   in **1 navigation** (visiting `/sessions/<id>/summary` directly — no
   requirement to find it via a live session list, since by definition
   there isn't one).

**Error states**
5. The ERROR state (no stale doc) shows a plain-language failure sentence
   (from the `error_stage` → copy table above — never the raw
   `error_stage` enum value or a stack trace in the primary text) and
   offers a prominently-styled **Regenerate** button as the sole recovery
   action.
6. Copy failure shows the literal text "Copy failed. Select the text and
   copy manually." and the underlying document text remains manually
   selectable — the failure message itself names the fallback action.
7. **No dead ends**: every error/edge state in this document (ERROR,
   copy-failure, stale-doc-plus-banner if implemented, standalone-route
   not-found) has at least one visible, actionable exit path (Regenerate,
   manual text selection, or "← Back") — none renders a message with no
   follow-up affordance.

**Empty / minimal-activity states**
8. A trivial session's READY document shows the five section headings with
   their exact empty-state strings (verbatim table above), rendered in the
   same visual slot as populated content — never blank, never omitted,
   never visually de-emphasized relative to a populated section.

**Idempotency / in-flight**
9. Clicking Regenerate disables the button immediately (before the network
   response returns) and re-labels it "Regenerating…"; a second click
   while disabled has no additional effect (no second network request
   observable, no duplicate `aria-live` announcement).

**Accessibility**
10. Every interactive control in the panel (Copy, Regenerate, tab itself,
    "▸ Details" disclosure, "← Back" link) is reachable and operable via
    keyboard alone (`Tab` to focus, `Enter`/`Space` to activate), with no
    mouse-only affordance.
11. The Copy button has `aria-label="Copy summary as Markdown"` (not a bare
    icon with only a `title` attribute); state transitions (GENERATING →
    READY/ERROR, copy success/failure, regenerate start/success/failure)
    are announced through a single shared `aria-live="polite"` region using
    the exact strings in the table above.
12. Every place status is conveyed by color (Decisions-at-a-glance icons,
    ✓/✕/◔/● glyphs, the tab-strip status dot) pairs color with a text
    label or distinct icon shape — never color alone conveys
    approved/denied/pending.
13. Text and icon contrast in both the READY document and the ERROR card
    meets **4.5:1** against their background in both light and dark theme
    (per this repo's existing token contract, `web-app/src/styles/theme.css.ts`
    — no new hardcoded colors introduced by this feature, consistent with
    `.claude/rules/css-architecture.md`).
14. Skeleton-block shimmer animation respects `prefers-reduced-motion:
    reduce` (renders static gray blocks, no animation, when the user's
    system setting requests it).

---

## Gap summary for the coordinator

**Confirmed gap**: plan.md's Story 3.1.2 (`SessionSummaryPanel`) does not
address the "stale document with a banner" case from `research/ux.md` §4.
The plan's frontend branches only on `status` (READY vs. ERROR), with no
path for "ERROR, but a usable prior document exists in the same response."
As written, a failed Regenerate attempt on a session that already had a
good READY document would replace that document with a bare error card,
discarding a working summary from the user's view (the underlying DB row
likely still has the old `markdown` depending on whether the error-path
upsert in Task 1.5.2b is field-scoped — that itself is unconfirmed in the
plan). See surface (d2) above for a proposed design and the two concrete
task additions (one backend upsert-scoping confirmation, one frontend
branch) it would require if accepted. A secondary, smaller gap in the same
family is noted in surface (g): the standalone route has no explicit
"no summary exists / never will" empty-result design, which risks an
indefinite GENERATING-style dead end for a URL that will never resolve.
