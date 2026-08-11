# UX Research: OSC Title/Progress Sequences as High-Priority Status Signals

## Scope check

This is a backend detection-accuracy change. No new UI surface, component, or
interaction is being added. The only user-facing consequence is that
existing status badges/chips will update *more accurately* (and potentially
somewhat faster) than they do today. Findings below are scoped to that
existing surface only.

## 1. Where DetectedStatus is rendered

- `session/detection/proto_mapping.go` — `DetectedStatusToProto` /
  `DetectedStatusToSubStatus` are the single authoritative Go→proto mappings
  (`DetectedStatus` and derived `SubStatus`). Both are exposed to the
  frontend on the session object.
- `web-app/src/components/sessions/StatusBadge.tsx` — renders a badge from
  either `AttentionReason` (priority) or `DetectedStatus` (fallback).
  `getDetectedStatusInfo()` maps every `DetectedStatus` enum value
  (`READY`, `PROCESSING`, `NEEDS_APPROVAL`, `INPUT_REQUIRED`, `ERROR`,
  `TESTS_FAILING`, `IDLE`, `EXECUTING`, `SUCCESS`, `WAITING_FOR_AGENT`,
  `UNKNOWN`/`UNSPECIFIED` → no badge) to a label/icon/CSS variant, uses an
  exhaustive switch (`assertNever` on the default case — new enum values
  would be a compile error, so this feature does not need a new
  `DetectedStatus` value to render; existing mappings already cover
  Executing/Processing/Idle).
- `web-app/src/components/sessions/SubStatusChip.tsx` /
  `SubStatusChip.css.ts` — a second, higher-priority chip driven by
  `SubStatus` (finer-grained than `DetectedStatus`); `SessionCard.tsx`
  (`web-app/src/components/sessions/SessionCard.tsx:533-547`) only falls
  back to `StatusBadge` when `SubStatusChip` has nothing to show.
- `web-app/src/lib/utils/deriveWorkingState.ts` — derives a coarser
  `WorkingState` (ACTIVE/PROCESSING/IDLE/UNSPECIFIED) used for other UI
  (e.g. spinner/animation gating), with `SubStatus` primary and
  `DetectedStatus` as fallback when `SubStatus` is UNSPECIFIED.
- `web-app/src/components/sessions/SessionCard.tsx`,
  `SessionRow.tsx`, `SessionList.tsx`, `DetectionEventsPanel.tsx` all
  consume `detectedStatus`/`subStatus` from the session object fetched via
  `useSessionService.ts`.

No new frontend component work is implied: this feature only changes what
value flows into `detectedStatus` (and its priority) on the backend, not how
that value is displayed.

## 2. Update speed: debounce, flicker risk, existing smoothing

- **Delivery path**: sessions stream to the frontend over a WebSocket
  (`watchSessions` in `web-app/src/lib/hooks/useSessionService.ts`,
  `clientRef.current.watchSessions(...)`), not polling. Each
  `sessionUpdated` event is applied to the Redux store and re-rendered
  immediately — there is **no frontend-side debounce/throttle** on status
  badge updates (`grep` for `debounce`/`throttle` in
  `web-app/src/lib/hooks/` and `web-app/src/components/sessions/` turns up
  debounce hooks for unrelated things — path completions, worktree
  suggestions, rule validation, search — none of them wrap session status).
  A 30s "stale" backstop timer only guards the *connection*, not per-update
  smoothing.
- **No CSS transition** on `StatusBadge`/`SubStatusChip` — swaps are
  instantaneous (`StatusBadge.css.ts` has no `transition`/`@keyframes`
  except a `spin` keyframe used for a processing spinner icon, not for
  badge-to-badge crossfade). So there is nothing on the frontend that would
  "mask" a faster or flappier backend signal — whatever the backend emits,
  the badge will show immediately and exactly.
- **Existing backend-side debounce**: `session/detection/idle.go` already
  has a `DebounceDelay` (500ms, `IdleDetectorConfig.DebounceDelay`,
  `session/detection/idle.go:34`) specifically "to prevent flickering" when
  transitioning idle state. This is the natural place this feature's
  priority/override logic needs to interact with: if OSC-derived status is
  given priority and bypasses this debounce, a Braille spinner that
  redraws every ~80-100ms could in principle produce updates fast enough to
  defeat the existing anti-flicker guard and cause visible badge flapping.
  **This is a backend design concern, not a frontend one** — the fix belongs
  in how the Claude binary detector consumes/aggregates OSC updates (e.g.
  don't treat every individual spinner-glyph redraw as a new status event,
  only treat OSC-signaled *state transitions* — spinner→`✳`, or
  `\x1b]4;0`→complete — as one), not in adding new frontend smoothing.
  Flagging this so implementation/plan phases account for it, since the
  acceptance criteria explicitly says "no regression to existing status
  display."

## 3. Accessibility

- `StatusBadge` already conveys status via more than color: `role="status"`,
  `aria-label={info.label}` (text, not icon-only), a `title` tooltip, and a
  visible text label alongside the icon (`StatusBadge.tsx:94-104`). This
  pattern already satisfies the "not color/icon alone" bar.
  `SubStatusChip.tsx` should be checked for the same pattern if it's edited,
  but this feature does not add a new displayed status value — the existing
  `Executing`/`Processing`/`Idle` mappings in `getDetectedStatusInfo()`
  already have accessible labels. **No new accessible-label work is
  required** since no new enum value or UI state is introduced; OSC parsing
  only changes which existing `DetectedStatus` value is chosen and how
  quickly/confidently.

## 4. Job-to-be-done

The user's job here is implicit trust: *"I want the session card's status
badge to reflect what's actually happening in the terminal, without having
to open the pane myself to check."* Today a false-idle (blank prompt during
a slow tool call showing `Idle`/`Ready` while Claude Code's own OSC title
still shows a spinner) breaks that trust — the user either under-reacts
(assumes it's safe to interrupt/steer when it isn't) or has to manually
verify by opening the pane, which defeats the purpose of the status badge
existing at all. This feature's UX payoff is entirely about *closing that
trust gap*, not about adding new UI.

## Bottom line

No new UI surface, no new accessible-label work, and nothing on the
frontend needs to change or add debouncing — the existing WebSocket push +
immediate-render pipeline will just display better data. The one real risk
worth carrying into implementation is backend-side: OSC title updates can
arrive much faster than the existing 500ms `DebounceDelay` anti-flicker
window in `session/detection/idle.go` was designed for, so the
priority/override logic should coalesce OSC-derived *state transitions*
rather than forwarding every raw title redraw, or it will re-introduce the
flicker that debounce was added to prevent.
