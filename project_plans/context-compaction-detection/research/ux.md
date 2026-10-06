# UX Research: context-compaction-detection

SDD Phase 2. Scope per `requirements.md`: a distinct "compacting context"
badge/spinner on the session card when Claude Code is auto-compacting.
Additive only — no new page/flow, no change to existing state precedence.

## 1. Comparable UX patterns already in the codebase — reuse, don't invent

Two components render session working-state today, in a fixed precedence
order (`web-app/src/components/sessions/SessionCard.tsx:533-548`):

- **`SubStatusChip`** (`web-app/src/components/sessions/SubStatusChip.tsx`)
  is the *primary* renderer, driven by the proto `SubStatus` field. It is
  shown whenever `session.status === ACTIVE` and `subStatus` is not
  `UNSPECIFIED`/`IDLE`.
- **`StatusBadge`** (`web-app/src/components/sessions/StatusBadge.tsx`) is
  the *fallback*, driven by `DetectedStatus`, and is only rendered when
  `subStatus` is `UNSPECIFIED` or `IDLE` — the code comment at
  `SessionCard.tsx:533-534` states the rule explicitly: "showing both is
  duplication."

**This precedence rule is the single most important finding for planning.**
`requirements.md` scopes the new detection state to the Go `DetectedStatus`
enum and `deriveWorkingState.ts` (which also reads `DetectedStatus` only as
a fallback when `SubStatus` is `UNSPECIFIED`), not to the `SubStatus` proto
field. But `SessionCard.tsx` only renders `StatusBadge` (the `DetectedStatus`
consumer) when `subStatus` is `UNSPECIFIED`/`IDLE`. If a session is
mid-compaction while its `SubStatus` is `PROCESSING` (the common case — the
backlog item's own framing is that today compaction is invisible *because*
the card stays pinned in `PROCESSING`), `SubStatusChip` renders "⏳
Thinking…" and the new `DetectedStatus.COMPACTING`-driven `StatusBadge`
would be suppressed by the existing precedence rule — defeating the
feature. Planning needs to explicitly decide one of:
(a) give `SubStatusChip` a `COMPACTING` case too (requires a `SubStatus`
proto/wire change, currently out of the stated acceptance criteria), or
(b) special-case the badge-row precedence so the compacting `StatusBadge`
is shown even when `subStatus === PROCESSING`. This research flags it as an
open question; it is not resolved here (out of UX-research scope) — it's an
architecture/data-flow decision for `research/architecture.md` and
`implementation/plan.md`.

- **Visual vocabulary already established**, from `SubStatusChip.css.ts`:
  icon/spinner + short label pill, `border-radius: full`, `padding: 2px 8px`,
  one of two shapes:
  - *Static glyph* pill (⚠, ⌨, ✖, ⏱, ●, ✓) for discrete/blocked states.
  - *Animated spinner + text* pill — used **only** for `chipProcessing`
    ("Thinking…", spinning border-arc built from `currentColor`,
    `0.8s linear infinite`). This is the closest existing analog to
    "compacting" (an actively-running-not-blocked state) and the strongest
    candidate to imitate rather than a static icon.
  - `chipWaitingForAgent` carries the exact emotional register wanted here,
    per its own source comment: *"Neutral/transient — agent is doing work
    autonomously; no user action needed."* Its styling (`vars.color.accentBg`
    / `vars.color.primary` border, `fontWeight.normal`, `opacity: 0.85`) is
    the recommended starting point for a `chipCompacting` variant — same
    "calm, non-alarming, doesn't need your attention" register, distinguished
    only by icon + label text (per the codebase-wide rule below), not a
    louder color.
  - Codebase-wide accessibility rule (mirrored from the sibling
    `stale-session-detection` UX research,
    `project_plans/stale-session-detection/research/ux.md:14-24`, itself
    citing `stuckReason.ts:33`): **icon is always decorative
    (`aria-hidden="true"`), text label always accompanies it** — never
    color/icon alone as the signal.
- Reduced-motion handling is already solved and should be copied verbatim:
  `SubStatusChip.css.ts:130-143`'s `spinner` style swaps the spinning arc for
  a static filled dot (`opacity: 0.6`) under `(prefers-reduced-motion:
  reduce)`, rather than just freezing the half-arc (which would look like a
  broken/errored spinner, not a static indicator).

**Do not invent a new visual language.** A `chipCompacting` variant that
pairs the `chipProcessing`/`chipWaitingForAgent` spinner-pill shape with a
new icon (e.g. 🗜 or ⟳) and the label "Compacting context" is consistent
with the existing system; a bespoke shape or color would stand out for the
wrong reason.

## 2. User mental model

"Compacting context" is **not** self-explanatory to a user who hasn't used
Claude Code's CLI directly and seen the "N% until auto-compact" status line
themselves — "compacting" is internal Claude Code jargon (summarizing/
trimming conversation history to free context window) that doesn't map to
any visible on-screen action, unlike "Waiting for Agents" or "Tests
Failing," which describe an observable activity.

- Every existing `SubStatusChip` case already carries a `title` tooltip
  attribute with a one-sentence plain-language explanation (e.g.
  `chipWaitingForAgent`: `"Claude is waiting for background agents to
  finish"`). The new state should follow the same convention without
  deviation — e.g. `title="Claude is summarizing older conversation history
  to free up context space"` — rather than assuming the short label alone
  is enough. This is a `title` attribute (native browser tooltip, shown on
  hover/focus), not a new tooltip component — zero new UI surface.
- **Risk of alarm vs. reassurance**: the requirements doc explicitly quotes
  the original issue's framing — "session is not blocked, not idle — it's
  self-managing." The single biggest UX risk is choosing an icon/color that
  reads as an interruption or degradation (e.g. a warning-amber tone, or an
  icon resembling compression/squeezing under pressure) when the correct
  read is "normal, expected, transient, no action needed" — the same
  register as `chipWaitingForAgent`. Recommend explicitly avoiding
  `vars.color.warning*`/`vars.color.error*` tokens for this state; reuse
  `vars.color.accentBg`/`vars.color.primary` (the same tokens
  `chipProcessing`/`chipWaitingForAgent` already use) so it reads as a
  sibling of "actively working," not a problem.
- Label wording: "Compacting context" (from the acceptance criteria) is
  reasonable as the visible badge text, since it's short and scannable, but
  should not be the *only* explanation available — the `title` tooltip
  carries the "why," consistent with every other chip.

## 3. Accessibility

- Every existing status chip/badge (`SubStatusChip`, `StatusBadge`) uses
  `role="status"` on the pill `<span>`, which has *implicit* ARIA semantics
  of `aria-live="polite"` + `aria-atomic="true"` per the ARIA spec — no
  component in this family sets an explicit `aria-live` attribute. The one
  place in `SessionCard.tsx` that *does* set `aria-live="polite"` explicitly
  is the visually-hidden creation-progress announcement
  (`SessionCard.tsx:792`), which pairs a screen-reader-only text node with a
  separate `aria-hidden="true"` visual spinner — a stronger, more
  deliberate pattern than the implicit-`role=status` chips use.
- Practical implication for a **transient** state like compacting (per the
  requirements, this state should self-clear back to normal
  processing/executing once compaction finishes, typically 30-60s later):
  because React will likely swap the chip's DOM subtree (different icon,
  different text, potentially a different element if `SubStatusChip`'s
  switch statement produces a new `<span>`) rather than mutating text in
  place, an implicit `role="status"` region does *not` reliably guarantee
  an announcement on every swap in all screen readers — some only announce
  on text-content mutation of a stable node, not full node replacement.
  This is a pre-existing characteristic of every state transition in this
  chip family already (e.g. Processing → Needs Approval), not something
  unique to compacting, so it's consistent to follow the same pattern
  rather than solve it net-new here. Recommend: match the existing
  convention exactly (`role="status"`, `aria-label`, `aria-hidden` icon) for
  consistency, and flag "verify chip-swap announcements across the existing
  family" as a pre-existing, not new, accessibility follow-up if it's ever
  audited — out of scope to fix here per the additive-only requirement.
- Follow the "announce without being chatty" guidance already embedded in
  the codebase's copy conventions: a single `aria-label` on entry (e.g.
  `aria-label="Compacting context"`) is sufficient; do not add a live
  percentage-progress announcement (compaction has no observable progress
  fraction the UI can show, and Claude Code's own "N% until auto-compact"
  line is the *approaching* indicator, not an in-progress one — see
  requirements.md's Open Question).

## 4. Error/edge-case UX — stuck detection

The requirements doc reports compaction "reportedly" runs 30-60s, but this
is not a verified hard bound (it's user-reported, not measured or sourced
from Claude Code's own documentation). Two related but distinct concerns:

1. **Normal-duration UX**: no countdown or progress bar — matches the
   Non-Goals section ("not building a general context-usage progress
   meter"). A static "Compacting context" pill for the duration is
   sufficient; this mirrors how `chipWaitingForAgent` and `chipProcessing`
   already work (no progress indication, just a persistent state pill).
2. **Stuck/longer-than-expected duration**: this repo already has a
   direct precedent for exactly this class of problem —
   `project_plans/stale-session-detection/` — which built a dedicated,
   config-driven staleness threshold specifically because "the badge's job
   is negative-space scanning" (catching the one card that silently died
   among many healthy ones). Two options, in order of recommendation:
   - **(Preferred) Do nothing new for this feature.** If a compacting
     session genuinely hangs, the *existing* stale-session detection
     (`project_plans/stale-session-detection/`, already shipped/in-flight
     per its own plan docs) is the correct owner of "this session hasn't
     made progress in N minutes" — it operates on `lastMeaningfulOutput`/
     `lastTerminalUpdate` independent of which specific sub-state the
     session is in, so a stuck-while-compacting session is already covered
     as a stale session once it crosses that threshold. Building a second,
     compacting-specific timeout would duplicate that mechanism and
     introduce a second, harder-to-reason-about threshold (see that
     project's own ADR-001 for why introducing a *third* distinct
     sensitivity threshold needs explicit justification, not silent reuse
     or silent invention).
   - **(If planning decides otherwise) Revert-to-generic on a timeout**,
     not a hard error state: if a maximum compacting duration is added, on
     expiry the UI should fall back to the pre-existing generic
     `chipProcessing`/`StatusBadge` "Processing" treatment (the state
     before this feature existed), not a new error/alarm badge — the
     backend session is not actually erroring, only the *detection* has
     lost confidence in which specific sub-state it's in. Do not invent a
     "stuck compacting" alarm state as part of this feature.
   Recommend research/planning treat this as an **open question resolved by
   pointing at `stale-session-detection`**, not a new mechanism — flagging
   per this research question's own framing rather than deciding
   unilaterally here, since it's a cross-project sequencing decision
   (does `context-compaction-detection` ship before or after
   `stale-session-detection` lands its threshold config?).

## 5. Jobs-to-be-done

- **Functional** — "know what's happening": distinguish "still generating/
  executing a task" from "housekeeping its own context window" without
  needing to open the session's terminal output to check. Directly serves
  the problem statement: a user watching 5-10 parallel cards currently
  can't tell these apart because both render as generic
  Processing/Executing.
- **Emotional** — "not worried it's stuck or broken": per requirements.md's
  own framing of the source issue ("it's self-managing"), the job is
  explicitly reassurance, not alarm. This drives the color/icon
  recommendation in §2 (reuse the calm `accentBg`/`primary` register used
  by `chipWaitingForAgent`, not `warning`/`error` tokens) and the
  no-progress-bar recommendation in §4 (a progress meter implies something
  worth tracking/worrying about; a static "in progress, this is normal"
  pill does not).
- **Social** — not applicable; this is a single-user, single-card
  indicator with no sharing/handoff dimension (matches requirements.md's
  Non-Goals, which scope this to detection/UI only, no lifecycle/control
  behavior).
