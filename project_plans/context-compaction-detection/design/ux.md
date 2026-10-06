# UX Design: context-compaction-detection

SDD Phase 3 design artifact. Builds on `requirements.md` (acceptance criteria)
and `research/ux.md` (precedent analysis). This is a small, additive feature:
one new chip variant reusing the existing `SubStatusChip`/`StatusBadge`
family — no new screen, no new interaction, no new component shape.

Sources read directly for this design: `web-app/src/components/sessions/SubStatusChip.tsx`,
`SubStatusChip.css.ts`, `StatusBadge.tsx`, `SessionCard.tsx:500-569`,
`web-app/src/lib/utils/deriveWorkingState.ts`, `server/adapters/review_queue_adapter.go`,
and (via a research pass) `web-app/src/components/sessions/ReviewQueuePanel.tsx` /
`ReviewQueueBadge.tsx`.

## Design decision carried from research (stated, not re-litigated)

`research/ux.md` §1 flags an unresolved precedence conflict: `SessionCard.tsx`
only renders `StatusBadge` (the `DetectedStatus` consumer) when
`subStatus` is `UNSPECIFIED`/`IDLE`; if a compacting session's `subStatus`
is `PROCESSING` (the common case), a `DetectedStatus`-only compacting badge
would be silently suppressed by the existing "don't show both" rule. That's
an architecture/data-flow decision, not a UX one — this design specifies the
**visual contract** that must hold regardless of how planning resolves it:

> **Exactly one pill occupies the badge slot at a time.** Whichever
> component ends up owning "compacting" (a new `SubStatusChip` case, or a
> precedence carve-out for `StatusBadge`), the user must see a single
> "⟳ Compacting context" pill replace "⏳ Thinking…" for the duration —
> never both stacked, never neither.

This is written as UX acceptance criterion 1 below so it's checkable
independent of which implementation path is chosen.

## Visual spec (shared across surfaces)

New chip variant `chipCompacting`, modeled on `chipWaitingForAgent` (research
§1's recommended precedent — same "calm, transient, self-managing" register):

- Icon: `⟳` (matches the acceptance-criteria example text, distinct from
  `chipProcessing`'s bare spinner-arc-only icon and `chipWaitingForAgent`'s `⏳`)
- Label: `Compacting context`
- Tokens: `vars.color.accentBg` background / `vars.color.primary` border+text —
  **identical token pair to `chipProcessing`/`chipWaitingForAgent`**, not
  `warning`/`error` tokens (research §2's explicit anti-alarm recommendation)
- `role="status"`, `aria-label="Compacting context"`,
  `title="Claude is summarizing older conversation history to free up context space"`
- Icon wrapped in `aria-hidden="true"` (or, if reusing the animated-spinner
  treatment, the existing `spinner` class — copied verbatim, including its
  `prefers-reduced-motion` static-dot fallback)

```
┌─────────────────────────┐
│  ⟳  Compacting context  │   ← pill: border-radius full, padding 2px 8px,
└─────────────────────────┘      fontSize xs, fontWeight normal (matches
                                  chipWaitingForAgent, not chipProcessing's
                                  semibold — slightly quieter than "Thinking…")
```

---

## Surface (a): session card badge — normal transient state

Location: the badge row inside a session card, same horizontal slot
currently occupied by `SubStatusChip`/`StatusBadge` (`SessionCard.tsx:533-548`).

```
┌───────────────────────────────────────────────────────────┐
│ ● session-name-here                          [Active] ▾    │
│ ⟳ Compacting context     main → feature/foo   +42 / −7      │
│                                                              │
│ [terminal preview / last output line...]                    │
└───────────────────────────────────────────────────────────┘
        ▲
        └─ same slot as "⏳ Thinking…" / "🔒 Approve Tool Use" today —
           no new row, no layout shift, no resize of the card.
```

Interaction flow — this is a passive indicator, not a control:

1. User is watching the session board (list or kanban view) with several
   cards active.
2. Backend detects the compaction-in-progress string in PTY output → proto
   `DetectedStatus`/`SubStatus` update flows to the client over the existing
   session-update stream.
3. The badge slot's content swaps from "⏳ Thinking…" (or whatever the prior
   sub-state was) to "⟳ Compacting context" — no click, no dialog, no toast.
4. User can keep working the rest of the board; this card requires no
   action. Hovering/focusing the pill surfaces the `title` tooltip
   explaining *why* ("Claude is summarizing older conversation history...").
5. When compaction ends, the badge swaps back to whatever sub-state is next
   (typically back to "Thinking…"/"Executing" as the agent resumes) — again
   with no user action.

No dead end: there is no button, link, or expand affordance on this pill —
it cannot be a dead end because it is not an entry point to anything.

---

## Surface (b): transition in/out

Sequence view (single card, over time), no user input at any step:

```
t0        t1                t2                  t3
Thinking… → Compacting context → Thinking/Executing → (task continues)
 ⏳            ⟳                    ⏳/⚡
 |             |                    |
 |             |                    └─ badge reverts automatically;
 |             |                       no residual "just finished
 |             |                       compacting" state (matches
 |             |                       chipProcessing/chipWaitingForAgent's
 |             |                       existing no-progress-artifact pattern)
 |             └─ compaction-in-progress string detected; single pill swap
 └─ ordinary processing before compaction begins
```

Edge case — rapid flicker: if detection is noisy (the pattern match
oscillates on/off within a couple of polling cycles), the badge should not
visibly flicker Thinking→Compacting→Thinking→Compacting. This is the same
class of risk every existing chip transition already carries (Processing →
Needs Approval can flicker too) — no new debounce mechanism is proposed here
per the additive-only constraint, but it's called out as UX acceptance
criterion 7 so it's checked against real detector output during
implementation, not assumed safe.

---

## Surface (c): stuck / long-running — what the user sees today (no new timeout)

Per `requirements.md`'s Non-Goals and `research/ux.md` §4, this feature does
**not** add a compaction-specific timeout or "stuck" alarm. What a user
actually sees if compaction runs far longer than the reported 30-60s:

```
┌───────────────────────────────────────────────────────────┐
│ ● session-name-here                          [Active] ▾    │
│ ⟳ Compacting context                                        │
│   (unchanged — 2 min, 10 min, 45 min later: identical pill, │
│    no elapsed-time counter, no progress bar, no color        │
│    escalation)                                               │
└───────────────────────────────────────────────────────────┘
```

- The pill is static text, not a countdown — this matches `chipProcessing`/
  `chipWaitingForAgent`'s existing no-progress convention, and matches the
  Non-Goals section explicitly ("not building a general context-usage
  progress meter").
- If/when the sibling `stale-session-detection` project's staleness
  threshold fires independently (it keys off `lastMeaningfulOutput`/
  `lastTerminalUpdate`, not sub-state — see `research/ux.md` §4), it is the
  system of record for "this has been running too long," not this feature.
- **Cross-project precedence risk worth flagging for `stale-session-detection`'s
  own design** (found while tracing this feature's badge-row logic, not
  solved here): `StatusBadge` — which is how `AttentionReason.STALE` would
  render — is only shown when `subStatus` is `UNSPECIFIED`/`IDLE`
  (`SessionCard.tsx:533-537`). If `subStatus` is `COMPACTING` (non-idle,
  non-unspecified) when staleness is detected, a stuck-while-compacting
  session could have its "Stale" badge silently suppressed the same way
  research §1 already flags for "Thinking…" today. This is not this
  feature's problem to solve (staleness threshold doesn't exist yet), but
  it's the same failure shape and should be checked when that project lands.
- No error/alarm state is shown for long-running compaction in this
  feature's scope — the pill simply persists. This is a deliberate,
  research-backed choice (§4/§5: "not worried it's stuck or broken" is the
  job to be done), not an oversight.

---

## Surface (d): review-queue panel — current behavior (no visible change)

Traced `server/adapters/review_queue_adapter.go`'s `subStatusFromItem` (the
"second surface" flagged in the task) against the actual frontend consumer.
Finding: **`ReviewQueuePanel.tsx` does not render `SubStatus` at all today.**
Per-item status in the review queue is driven entirely by
`ReviewQueueBadge.tsx` → `StatusBadge` keyed off `AttentionReason` (why the
item is *in the queue* — approval pending, stale, uncommitted changes,
etc.), not by `SubStatus` (what the session is *doing right now*). The
`ReviewItem.subStatus` proto field is populated by the adapter but is
currently dead on the frontend — true for every existing `SubStatus` value,
not something new introduced by this feature.

```
┌──────────────────────────────────────────────┐
│ session-name-here                    [🔴 URG] │
│ [🔴 Urgent] [🔒 Approval Pending]              │  ← AttentionReason-driven,
│ waiting on tool use approval for `rm -rf ...` │     unaffected by compacting
│ Pattern: needs_approval                       │
│ Program: claude   Branch: feature/foo         │
│ Last Activity: 3m ago            +42 / −7     │
│ [✓ Approve] [✗ Deny] [✦ Create Rule]          │
└──────────────────────────────────────────────┘
   ▲
   └─ if this same session were also mid-compaction right now, nothing
      here changes — no "⟳ Compacting context" appears in this card,
      identical to how no SubStatus ever surfaces here today.
```

Interaction flow: none — this surface is unaffected by the feature as
scoped. Recommendation: **do not add `SubStatus` rendering to the
review-queue panel as part of this feature.** `requirements.md`'s
acceptance criteria (5, 6) scope the visible change to "session card UI" and
`deriveWorkingState.ts` only; adding a second rendering surface here would
be new frontend scope the requirements doc never asked for. The only
implementation-completeness note (not a UX concern, flagged for the plan):
`subStatusFromItem`'s switch has no explicit case for a new
`StatusCompacting` Go value, so an unhandled value falls through to the
function's final `return SUB_STATUS_UNSPECIFIED` — harmless today (the
field is unread) but worth a one-line case addition for wire completeness
consistent with the "additive only" acceptance criterion, so a future
review-queue consumer doesn't inherit a silent gap.

---

## UX Acceptance Criteria

Each is testable by a human clicking through the running app (or reading
the rendered DOM/CSS), not just by code review.

1. **Single-pill invariant.** With a session mid-compaction, exactly one
   status pill is visible in the badge row — never "⏳ Thinking…" and
   "⟳ Compacting context" stacked together, never neither. (Directly tests
   the precedence risk from `research/ux.md` §1, regardless of which
   component ends up owning the state.)
2. **Visual distinctness from "Thinking…"/generic Processing.** The
   compacting pill's icon (`⟳`) and label ("Compacting context") are
   visually distinguishable at a glance from `chipProcessing`'s spinner +
   "Thinking…" — different icon, different text — while intentionally
   sharing the same color/border tokens (`accentBg`/`primary`). A reviewer
   should be able to tell the two apart without reading the tooltip.
3. **No dead ends.** The pill has no click handler, no href, no focus
   affordance beyond the default tab order a `<span>` doesn't participate
   in — it never blocks, intercepts, or gates any other action on the
   session card (approve/deny/stop/open-terminal/etc. all remain usable
   exactly as when any other sub-status chip is showing).
4. **Keyboard navigation — N/A, by design.** The pill is a passive status
   announcement, not a control, so it is correctly *not* keyboard-focusable
   (no `tabIndex`, no button/link semantics). Confirm it does **not**
   receive focus when tabbing through the card — a reviewer finding it
   focusable would indicate it was miscoded as interactive.
5. **Screen-reader label present.** The pill has `role="status"`, an
   `aria-label="Compacting context"`, and its icon carries
   `aria-hidden="true"` so a screen reader announces only the label, not a
   decorative glyph. Verify via browser accessibility tree inspection (or
   axe/VoiceOver/NVDA spot check).
6. **Tooltip carries the "why."** Hovering or focusing the pill shows a
   native tooltip (`title` attribute) with a plain-language explanation
   (e.g. "Claude is summarizing older conversation history to free up
   context space") — the short label alone is not assumed sufficient,
   consistent with every other chip in the family.
7. **Color contrast ≥ 4.5:1.** The chip reuses the existing
   `vars.color.accentBg`/`vars.color.primary` token pair already used by
   `chipProcessing`/`chipWaitingForAgent`, which already pass the repo's
   CI-enforced Axe Core WCAG AA gate (`web-app/src/`'s PR CI, per root
   `CLAUDE.md`). No new token is introduced, so no new contrast risk — this
   criterion is satisfied by construction, and re-verified automatically by
   the existing Axe Core CI check rather than a new manual audit.
8. **Consistency of register.** Side-by-side with `chipWaitingForAgent` and
   `chipProcessing`, the compacting pill reads as a sibling — same shape,
   same weight of color, same "calm, self-managing" tone. It must **not**
   use `warning`/`error` tokens or any visual cue (color, icon shape) that
   reads as "something went wrong" or "needs your attention."
9. **Reduced motion respected.** If the compacting pill uses the animated
   spinner treatment, confirm via OS/browser `prefers-reduced-motion:
   reduce` emulation that it falls back to the existing static-dot
   treatment (`SubStatusChip.css.ts`'s `spinner` class), not a frozen
   half-arc that reads as broken.
10. **No new surface in the review queue.** Confirm the review-queue panel's
    per-item rendering is unchanged by this feature — no compacting pill
    appears there, and no other review-queue visual regresses. (Documents
    the explicit scope boundary from Surface (d) above so a reviewer
    doesn't flag its absence as a bug.)
11. **Stuck/long-running has no alarm escalation.** With compaction held
    active for several minutes (simulated via fixture/mock if needed), the
    pill remains visually identical to its just-started appearance — no
    color change, no counter, no "this is taking a while" warning — since
    that concern is explicitly deferred to `stale-session-detection`.
