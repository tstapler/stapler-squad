# UX Design: github-call-optimization

Scope, per `requirements.md` and `research/ux.md`: this project is backend/infra
(rate limiting, caching, OTel). Its only user-facing surface is Phase 6, Epic
6.1 of `implementation/plan.md` — friendlier copy for GitHub rate-limit
failures in two *existing* error-display locations. No new component, panel,
or interaction model is introduced, so this is designed as two condensed
surfaces (same box, new text) rather than full wireframes for a new screen.

Grounding, verified directly (not trusted from the plan's citations):

- `web-app/src/components/shared/VcsWidget.tsx:70-79` really does contain
  `<div role="status" aria-live="polite" className={styles.liveRegion}>`
  wrapping the mergeability pill / blocking-reasons list — confirmed by
  reading the file. This is the pattern Task 6.1.1d says to match.
- `web-app/src/components/sessions/VcsPanel.tsx:43-55` today renders
  `error.message` verbatim (no reason coding, no `aria-live`) next to a
  bare "Retry" button.
- `web-app/src/components/shared/vcs-widget/VcsWidgetComments.tsx:59-63,107-109`
  today discards the real error in its `catch` and always renders the fixed
  string `"Failed to load comments"` with no retry action at all — a dead
  end for the account-wide-exhausted case.

## 1. Surfaces identified

| # | Surface | Type | Location |
|---|---|---|---|
| 1 | VCS status error box | Condensed (existing error slot, copy-only change) | `VcsPanel.tsx`'s error box, rendered in the session detail's VCS panel |
| 2 | PR comments load failure | Condensed (existing error slot, copy-only change) | `VcsWidgetComments.tsx`'s collapsed "Comments" section body |

Both surfaces carry the same three error states (two rate-limit buckets +
the pre-existing generic-error fallback), so they're designed together below
rather than duplicated per surface.

Out of scope for this design (per `research/ux.md` §1): `MergePR`, `ClosePR`,
and `PostPRComment` have no frontend call site today — there is no button to
add error copy to. If/when one is built, it should reuse the same
`getGitHubRateLimitMessage` classification designed here rather than
inventing new copy.

## 2. The three error states (condensed treatment)

Each state is defined by: what triggered it, the before/after copy, whether
an exit path (Retry) is offered, and the accessibility wrapper. All three
render inside the *same* DOM slot each surface already has — no new markup
structure, only content and (for VcsWidgetComments) a data flow.

### State A — Transient (self-inflicted, will likely clear on its own)

- **Trigger**: `rateLimitTransport`'s secondary-rate-limit fail-fast, or an
  `AdmitOrigin` rejection classified `reason="transient"` (short reset
  window) — per Task 6.1.1b.
- **Before** (today, `VcsPanel.tsx`): `github: rate limited until 2026-09-08T15:23:00Z: secondary rate limit`
- **After** (`VcsPanel.tsx`): `GitHub is temporarily rate-limited — this should clear up in about 40s, retrying automatically.`
  Verified true for this surface: `useSessionVcs.ts:158-171`'s `useEffect` starts a 60s fallback
  `setInterval` (line 162) that calls `fetchStatus()` unconditionally on every tick — not gated on
  the previous call having succeeded — so a transient rate-limit error genuinely does get retried
  without the user clicking anything.
- **Exit path** (`VcsPanel.tsx`): none required — self-resolving via the 60s fallback poller
  above. Retry button stays visible and functional (per plan, no state suppresses it) but the
  copy sets the expectation that the user doesn't need to click it.
- **Comments surface after-copy** (`VcsWidgetComments.tsx`, shorter space):
  `GitHub is rate-limited right now — tap Retry to try again.`
  **Correction**: the original copy here (`"Rate-limited — retrying automatically…"`) was false
  for this surface. `VcsWidgetComments.tsx` fetches comments exactly once per mount, guarded by
  `fetchedRef` (`VcsWidgetComments.tsx:36,48-49`) — there is no timer, subscription, or any other
  auto-retry mechanism here, unlike `VcsPanel.tsx`'s real 60s poller above. The only way this
  surface ever retries is the manual Retry button Task 6.1.1e adds, so its copy must state what's
  wrong and point at that action instead of implying the system will act on its own.
- **Comments surface exit path**: the same manual Retry button as State B/C (Task 6.1.1e) — unlike
  `VcsPanel.tsx`, there is no self-resolving path for this surface, so Retry is required here, not
  merely available.

### State B — Exhausted (account-wide, needs the user to wait)

- **Trigger**: primary rate limit exhausted, `reason="exhausted"`.
- **Before**: `github: rate limited until 2026-09-08T15:23:00Z: primary rate limit`
- **After**: `GitHub rate limit reached — try again in ~4 minutes.` (the "~4
  minutes" computed via the existing `formatRelativeTime` utility already
  imported in both target files — no new date-formatting code.)
- **Exit path**: **Retry** button (VcsPanel already has one; VcsWidgetComments
  needs one added, since today's fixed string offers none — see Acceptance
  Criteria).
- **Comments surface after-copy**: `GitHub rate limit reached — try again in ~4 minutes.`

### State C — Generic, non-rate-limit failure (existing fallback, unchanged behavior)

- **Trigger**: any error without a `reason` marker (network error, auth
  failure, etc.) — `getGitHubRateLimitMessage` must pass these through
  unclassified rather than mis-labeling them as a rate limit.
- **VcsPanel copy**: unchanged — raw `error.message`, as today.
- **VcsWidgetComments copy**: unchanged — `"Failed to load comments"`, as today.
- **Exit path**: Retry button in VcsPanel (unchanged); VcsWidgetComments
  gains the same Retry affordance it needs for State B, so this state also
  benefits.
- **Why unchanged**: Phase 6's scope is specifically rate-limit copy
  (`research/ux.md` §4 explicitly rejects inventing a third state beyond the
  two rate-limit buckets); a non-rate-limit error isn't part of this
  project's classification and shouldn't silently get relabeled.

## 3. Layout (unchanged) — condensed wireframe

```
VcsPanel error box (VcsPanel.tsx:44-53)         VcsWidgetComments (collapsed → expanded)
┌───────────────────────────────────┐          ┌─────────────────────────────┐
│ ⚠️  <message>            [Retry]   │          │ ▼ Comments                  │
└───────────────────────────────────┘          │   <message>        [Retry]  │
        role="status" aria-live="polite"        └─────────────────────────────┘
        (NEW — not present today)                role="status" aria-live="polite"
                                                  (NEW — Retry button also NEW)
```

No pixel/layout change: same icon-message-button row in VcsPanel; same
single status line in VcsWidgetComments, with one addition (a Retry
button/link) needed to give State B and State C an exit path they currently
lack in that surface.

## 4. Interaction flow

**VcsPanel (surface 1)** — unchanged flow, changed copy source:
1. `useSessionVcsContext()` returns an `error` (from `useSessionVcs.ts:87-88`).
2. `VcsPanel` calls `getGitHubRateLimitMessage(error)` instead of reading
   `error.message` directly.
3. If the error carries a `reason` marker, the friendly copy (State A or B)
   renders; otherwise the raw message renders unchanged (State C).
4. User may click **Retry** at any time in any state → `refresh()` re-fires
   the underlying RPC; on success the error box unmounts and the normal
   `VcsWidget` renders.

**VcsWidgetComments (surface 2)** — new: error is now captured, not discarded:
1. `fetchComments()`'s `.catch` (currently `console.error` + `setLoadState("error")`,
   discarding `err`) instead stores the classified error object in state.
2. The body renders `getGitHubRateLimitMessage(err)` (State A/B) or the
   existing fixed string (State C, when `err` carries no reason marker).
3. A **Retry** action (button or clickable text) resets `fetchedRef.current`
   and re-invokes `fetchComments()` — today there is no way to retry a
   failed comments load short of collapsing/re-expanding the section, which
   is a hidden, undocumented retry path this change makes explicit.

## 5. Acceptance criteria (UX)

1. **No dead ends**: every error state in both surfaces offers a visible,
   labeled Retry action reachable in ≤ 1 click from the error's rendered
   state. (Today, `VcsWidgetComments.tsx`'s error state has zero — this
   criterion is not yet met and Epic 6.1 must close it.)
2. **Copy correctness — State A**: given a `reason="transient"` error,
   `VcsPanel.tsx`'s rendered text matches the pattern `"GitHub is temporarily
   rate-limited — this should clear up in about <relative time>, retrying
   automatically."` (true for this surface — see §2's verified 60s fallback
   poller) and contains no raw ISO-8601 timestamp; `VcsWidgetComments.tsx`'s
   rendered text instead matches `"GitHub is rate-limited right now — tap
   Retry to try again."` and likewise contains no raw ISO-8601 timestamp and
   no claim of automatic retry, since this surface has no auto-retry
   mechanism to back that claim.
3. **Copy correctness — State B**: given a `reason="exhausted"` error, the
   rendered text matches the pattern `"GitHub rate limit reached — try again
   in ~<relative time>."` and contains no raw ISO-8601 timestamp.
4. **Fallback preserved — State C**: given an error with no reason marker,
   `VcsPanel` renders `error.message` verbatim (unchanged from today) and
   `VcsWidgetComments` renders `"Failed to load comments"` (unchanged from
   today) — `getGitHubRateLimitMessage` must not misclassify a non-rate-limit
   error into State A/B copy.
5. **Screen-reader announcement**: both surfaces' error containers carry
   `role="status" aria-live="polite"` (verified live pattern:
   `VcsWidget.tsx:71`), so a state transition into or between A/B/C is
   announced without the user needing to refocus. Testable by inspecting the
   rendered DOM for the attribute pair on the containing element in all
   three states.
6. **Color is not the only signal**: each state keeps the existing
   icon+text pairing (`⚠️` + message in VcsPanel; plain text in
   VcsWidgetComments) — no state relies on color alone to convey severity.
7. **No new components**: `getGitHubRateLimitMessage`/`GitHubRateLimitReason`
   (`web-app/src/lib/vcs/githubRateLimit.ts`, per Task 6.1.1a) is the only
   new file; both target components keep their existing structure, per
   `research/ux.md`'s explicit "no new UI panel" constraint.
8. **Keyboard navigable**: the Retry action in both surfaces is a real
   `<button>` (not a `<div onClick>`), reachable via Tab and activatable via
   Enter/Space — required for the new `VcsWidgetComments` Retry control,
   which doesn't exist yet, and re-confirmed unchanged for `VcsPanel`'s
   existing one.
9. **Contrast**: error text/icon against its container background meets
   ≥ 4.5:1 (WCAG AA) — no new colors are introduced by this change, so this
   is a regression check against the existing `VcsPanel.css.ts`/
   `VcsWidgetComments.css.ts` tokens, not new design work.

## 6. Non-goals (explicit, from research)

- No toast/notification for these errors — this is the durable-record
  (persistent box) side of the toast-vs-card split `research/ux.md` §1
  documents (`getFailureReasonToastMessage` vs. `getFailureMessage`); Epic
  6.1 only touches the card/box path.
- No mention of "poller," "background," or "priority admission control" in
  any user-facing copy — that's implementation detail for the logs, per
  `research/ux.md` §3.
- No third "ambiguous" state between transient and exhausted.
- No UI changes for `MergePR`/`ClosePR`/`PostPRComment` — they have no
  frontend call site to attach copy to (see §1).

## Summary

Surfaces designed: **2** (`VcsPanel.tsx` error box, `VcsWidgetComments.tsx`
failed-to-load state), covering **3 error-state variants** each (transient,
exhausted, generic fallback).
UX acceptance criteria written: **9**.
