# UX Research: session-retry-backoff

Scope per `requirements.md`: retry-count badge on `SessionCard.tsx`, retry history in the
session history view, manual "Retry now" action, and a `permanently_failed` terminal state
distinct from generic `NeedsAttention`. This doc covers comparable in-repo patterns to reuse,
external comparables (CI/job-queue retry UX), accessibility, edge cases, and jobs-to-be-done.
Architecture/data-flow (proto fields, backend wiring) is out of scope here.

## 1. Reuse, don't invent — three precedents already answer most of this feature's UI

**This is the third feature in a row touching `SessionCard.tsx`'s badge row** (after
`review-gate-stale-session-rework` and the in-flight `stale-session-detection`,
whose UX research at `project_plans/stale-session-detection/research/ux.md` this doc aligns
with directly). All three should look like siblings, not three different visual languages.

### 1a. The badge row and its precedence order (`SessionCard.tsx:493-548`)

The header badge row already has an established left-to-right precedence: primary `status`
badge → `rateLimitState` badge → `StatusBadge` (only when `SubStatusChip` has nothing to show,
per the explicit code comment at line 533: "showing both is duplication") → `SubStatusChip` →
a memory-usage badge (`memoryBadge`/`memoryBadgeWarning`/`memoryBadgeHigh` severity-tiered by
threshold, lines 549-566) → `ReviewQueueBadge` (compact, line 484). The new "Attempt N/max"
badge is a **new peer in this same row**, following the stale-session-detection research's own
conclusion: one scan-line per card, not a second place to look. Suggested position: immediately
after `ReviewQueueBadge`/before or after the memory badge — it's low-urgency-glance info like
memory, not a "look now" flag like `StatusBadge`/`SubStatusChip`.

The `memoryBadge` severity-tiering pattern (three CSS classes gated on a numeric threshold,
`SessionCard.tsx:552-554`) is also the direct precedent for how the retry badge should escalate
visually as attempts climb toward `max_attempts` (see Section 4).

### 1b. `ReviewQueueBadge.tsx` — compact vs. full badge variants

`ReviewQueueBadge` (`web-app/src/components/sessions/ReviewQueueBadge.tsx`) already implements
exactly the two rendering modes this feature needs: a `compact` prop that renders an
emoji-only-plus-abbreviation pill for the header row (lines 85-96), and a full variant with
icon + full text label for the expanded body (lines 99-109, rendered at `SessionCard.tsx:667`
inside `reviewInfo`). The retry badge should follow the same `compact` boolean split:
`compact` in the header row (e.g. "🔁 2/3"), full text ("Attempt 2 of 3 — retrying in 45s") in
the expanded card body, next to or replacing where `reviewItem.context` is shown
(`SessionCard.tsx:684-686`).

### 1c. Restart confirmation dialog — the direct precedent for "Retry now"

`SessionActionsOverflow.tsx` already has a manual, user-triggered "Restart" action with the
exact interaction shape "Retry now" should mirror:
- **Primary-button shortcut** for the relevant state: `showPrimaryAction && isStopped &&
  onRestart` renders a `🔄 Restart` button directly on the card (`SessionActionsOverflow.tsx:536-544`)
  — the equivalent surfacing for a `permanently_failed` session would be a `🔁 Retry now`
  primary button in the same slot, since `permanently_failed` is itself a stopped/terminal
  status.
- **Overflow menu item** in "Group 5" (the destructive-actions group, before Delete, per the
  `UX-009` comment at line 746) for the non-primary case — i.e. retrying a session that's
  currently mid-backoff-wait (not yet exhausted) rather than a primary button, since bypassing
  an active backoff is a less-common, slightly more deliberate action than restarting an
  already-stopped session.
- **Confirmation dialog** (`isRestartConfirmOpen`, lines 288-311) with a `dangerButton`-styled
  confirm action and inline `restartError` display — reuse this shape verbatim for "Retry now":
  a lightweight confirm ("Retry session now? This restarts immediately, skipping the remaining
  backoff wait.") is warranted because it's an out-of-band manual override of an automated
  process, and existing sibling actions (Restart, Delete, Clear Conversation) all confirm before
  acting. `focus-trap` via `useFocusTrap` (line 154) is already wired for this dialog shape and
  should be reused, not reimplemented.
- One difference from plain Restart: "Retry now" needs two entry contexts — (a) skip the
  current backoff countdown on a session that's still automatically retrying, and (b) force a
  restart from `permanently_failed` after retries are exhausted. Both route to the same backend
  action per requirements.md AC6, but the confirm-dialog copy should say which case it is
  ("skip the wait and retry now" vs. "this session gave up after N attempts — retry anyway?").

### 1d. `CheckpointList.tsx` — the direct precedent for retry history

`web-app/src/components/sessions/CheckpointList.tsx` is structurally identical to what "retry
history" needs: a `sessionId`-scoped list of `{ timestamp, label/reason, ...metadata }` items,
sorted newest-first (lines 31-35), capped at `MAX_VISIBLE = 10` with a "show all" expand toggle,
an explicit empty state ("No checkpoints yet"), and a relative-time formatter
(`formatRelativeTime`, lines 15-25) matching the `formatTimeAgo` already used elsewhere in
`SessionCard.tsx`. A `RetryHistoryList` component should copy this shape almost verbatim:
`{ attemptNumber, reason (crashed/stalled/tmux_exited), timestamp, outcome (retried/gave up) }`
per item, same "show all" pattern once retries exceed the visible cap (relevant once a policy
allows higher `max_attempts`), same `aria-label="Session retry history"` on the `<ul>`. Where it
should live: `SessionDetailView.tsx` doesn't currently have a dedicated "history" tab — the
closest analog is the checkpoints section pattern. Add retry history as a peer section there
(not a new top-level tab) unless the implementation plan finds checkpoints already live inside
a tab worth sharing.

### 1e. `StatusBadge.tsx` / `getAttentionReasonInfo` — the precedent for a new terminal reason

`AttentionReason` (proto enum consumed by `StatusBadge.tsx` and `ReviewQueueBadge.tsx`) already
has a `switch` mapping each reason to icon/color/label (`STALE`, `ERROR_STATE`, `IDLE_TIMEOUT`,
etc., `StatusBadge.tsx:17-32`). A new `permanently_failed` state should NOT be shoehorned into
`AttentionReason.ERROR_STATE` (which today reads as "generic error, glance and reassess") —
per requirements.md's own framing, it needs to be visually distinct so a user scanning cards
doesn't confuse "quick look needed" with "gave up after N attempts." Two implementation options,
both consistent with existing patterns:
  1. Add `AttentionReason.RETRIES_EXHAUSTED` (or similar) to the existing enum + switch — cheapest,
     reuses `ReviewQueueBadge`'s existing rendering path automatically. **Requires updating both
     `StatusBadge.tsx`'s switch and any other exhaustive consumer** — grep for `AttentionReason.`
     before implementing to enumerate all switch sites (unlike `stuckReason.ts`'s compile-enforced
     `Record` maps per the stale-session UX doc, this is a plain `switch`, so a forgotten case is a
     silent fallthrough, not a compile error — flag as an implementation checklist item).
  2. Give `permanently_failed` its own top-level `SessionStatus` value (parallel to how `STOPPED`,
     `PAUSED`, `HIBERNATED` are distinct statuses today) rather than treating it as an
     `AttentionReason` flavor of `NeedsAttention`. This matches requirements.md's framing more
     literally ("`permanently_failed` terminal state" — language, singular, describing session
     *status*, not *attention reason*) and would render via the existing top-level `status`/
     `getStatusColor`/`getStatusText` badge machinery (`SessionCard.tsx:516-522`) rather than
     nested inside `ReviewQueueBadge`. This is the recommended option: it reads correctly at the
     highest-precedence badge slot (leftmost, same slot as STOPPED/PAUSED) rather than competing
     for attention with the same-priority slot as "approval pending" or "idle" — a card that has
     given up should out-rank routine review-queue reasons in scan priority, and putting it in the
     primary status slot achieves that without new precedence logic.

## 2. External comparables

- **GitHub Actions job re-run + attempt history** — the closest 1:1 analog to this feature's
  three asks combined (badge, history, manual retry). GHA surfaces "Attempt 2" as a small
  dropdown/tab selector at the top of a job's log view (not a persistent badge on a list card —
  GHA's list view just shows the final/latest attempt's status), and a single "Re-run jobs"
  button that is *always* available regardless of current status (not gated to "only when
  failed"). Applicable takeaway: keep "Retry now" available whenever a session is in a
  retry-eligible or exhausted state, not only exactly at the moment backoff is waiting — a user
  who wants to force an earlier retry attempt (before backoff naturally elapses) shouldn't be
  blocked by the UI even though it's a less common trigger point.
- **Sidekiq retry UI** — shows a literal countdown ("Retry in 2h14m") per job in its Retries
  tab, plus a manual "Retry Now" button and a "Kill" button (permanently give up, the direct
  analog to this feature's manual "abandon and mark permanently_failed" — not explicitly
  requested here, but worth flagging as a natural companion action if scope allows: acceptance
  criteria 6 covers "retry now" but not "give up now"). Applicable takeaway: a live countdown
  ("Retrying in 45s") is more informative than a static "waiting" label, but see Section 4 for
  why it should NOT tick every second via re-render.
- **CI systems generally (CircleCI, Buildkite)** treat "attempt N of M" as inherently
  informational, never color-alone — always paired with a numeric fraction, which validates
  requirements.md's own "Attempt 2/3" phrasing as the right compact-badge text, not just an
  icon.

## 3. Accessibility

- **No `aria-live` region for the backoff countdown.** A "Retrying in 45s" display that
  re-renders every second and is wrapped in `aria-live="polite"` would announce every tick to
  screen reader users — a spam anti-pattern this repo's own review-queue/stale-session badges
  avoid by being static text re-evaluated only on data refresh, not a client-side ticking timer.
  Two acceptable approaches, in order of preference:
  1. **Static, non-ticking display**: show the countdown as static text computed at render time
     from `nextRetryAt` (e.g. "Retrying at 3:42 PM" or "Retrying in ~45s" using the same
     `formatTimeAgo`-style helper, re-computed only when the component naturally re-renders from
     a data poll/websocket update — not a local `setInterval`). This matches how
     `lastActivityRow`'s "Active 47m ago" already works (`SessionCard.tsx:677-696`) — it's a
     point-in-time render, not a live ticker.
  2. If a visibly-ticking countdown is wanted for polish, keep it **visual-only** (a plain
     `<span>` updated via local state/interval) and do NOT mark the container `aria-live` — the
     one-time backoff-started/backoff-elapsed *transitions* are the meaningful events for a
     screen reader user, not every intermediate second. If an announcement is wanted for those
     transitions specifically, use a separate, low-frequency `aria-live="polite"` region that
     only updates on state transition (retry started / retry succeeded / permanently failed),
     mirroring how toast/notification systems in this repo already debounce to state transitions
     (`NotificationContext.tsx`'s dedup-key pattern, cited in the stale-session UX doc).
- **Retry badge follows the same `role="img"` + full-sentence `aria-label` pattern** every
  sibling badge in the row already uses (`SessionCard.tsx:509-510` is the canonical example):
  `aria-label={`Retry attempt ${n} of ${max}`}`, icon marked `aria-hidden="true"`.
- **"Retry now" button is a real `<button>`**, keyboard-operable by default — follow
  `SessionActionsOverflow.tsx`'s existing `🔄 Restart` button/menu-item exactly (native
  `<button>`, `aria-label={`Retry session ${session.title} now`}`, focus-trapped confirm
  dialog via the existing `useFocusTrap` hook). No new keyboard pattern needed.
- **Color is never the only signal** for `permanently_failed`, same rule the stale-session UX
  doc already established for the whole badge row (WCAG 1.4.1) — icon + explicit text label
  ("Permanently failed" or similar), not a bare red border/dot.

## 4. Mid-backoff and `permanently_failed` — distinct states, distinct visual weight

- **While waiting to retry** (session crashed, backoff timer running, attempts remaining): show
  a distinct, lower-urgency badge — e.g. "🔁 Retrying in ~45s (attempt 2/3)" — using a neutral
  or info-toned color (not the same `warning`/`error` tokens `NeedsAttention` uses), since this
  is *expected, automated, self-healing* behavior, not a signal the user needs to act on. This
  mirrors the `memoryBadge` three-tier severity precedent (`SessionCard.tsx:552-554`): normal
  attempt count = neutral/no badge at all (attempt 1, no history yet — don't show "Attempt 1/3"
  per AC4's "once at least one retry has occurred" gate), attempt count approaching max = a
  warning-toned badge (same `warningBg`/`warningText`/`warning` token triplet the
  stale-session-detection research already established as this repo's shared "elevated concern"
  language), exhausted = `permanently_failed`'s own distinct treatment (see below). Concretely:
  attempt 1 of N → no badge; attempt 2+ of N while N > 2 → neutral/info badge; final attempt in
  progress → warning-toned badge; exhausted → `permanently_failed` badge (its own, most severe
  tier, likely reusing `error`/`errorBg` tokens since `warning` is already claimed by the
  "elevated but recoverable" tier).
- **`permanently_failed` must not be confused with generic `NeedsAttention`.** This is the
  sharpest UX risk named in the task brief, and Section 1e's recommendation (a distinct
  top-level `SessionStatus`, not an `AttentionReason` flavor) directly addresses it: it renders
  in the same primary-badge slot as `STOPPED`/`PAUSED` (leftmost, highest scan-precedence) with
  its own color (`error`/`errorBg`, not `warning`/`warningBg` — reserve amber/warning for "will
  self-heal," reserve red/error for "gave up, needs you") and its own label text ("Failed — gave
  up after 3 attempts" or similar), never sharing a color/icon with routine review-queue
  reasons like `APPROVAL_PENDING` or `IDLE`. A user scanning 5-10 cards should be able to
  distinguish "this one wants a quick approval click" from "this one is dead and needs real
  intervention" by color/icon alone (with text as the accessible confirmation, per Section 3).
- **What "Retry now" should do from each state**: from mid-backoff, it should short-circuit the
  wait and retry immediately (AC6's "ignoring the current backoff delay"); from
  `permanently_failed`, it should reset the attempt counter and restart fresh (AC6's "including
  from a `permanently_failed` state") — the confirm-dialog copy should say which of these two
  is happening (Section 1c) since they have different mental models for the user ("skip ahead"
  vs. "give it another chance after giving up").

## 5. Jobs-to-be-done

- **Functional job**: for someone running 5-10 parallel agent sessions, the job is triage
  prioritization at a glance — "which of these dead/failing sessions is self-healing on its own
  (ignore it) vs. which one truly needs me to look now (permanently_failed)." The badge answers
  this without opening any card; the confirm-gated "Retry now" answers "I don't want to wait for
  the backoff, do it now" without requiring the user to understand backoff math; retry history
  answers "was this session actually stable, or has it been quietly crash-looping the whole
  time" — a trust-but-verify check the user runs occasionally, not on every glance.
- **Emotional job** (directly from requirements.md's Users/Consumers framing and echoed by the
  sibling stale-session-detection research): **trust that automation is handling transient
  failures so the user doesn't have to babysit every session, paired with confidence that they
  will be told, unambiguously, the moment automation truly can't recover.** The self-healing
  retry-in-progress badge serves the first half (quiet, low-urgency, "don't worry about this
  one"); the visually distinct `permanently_failed` state serves the second half (loud, "this
  one needs you specifically," never confusable with routine review-queue noise). Getting the
  two states visually *un*-confusable is the single highest-leverage UX decision in this
  feature — a `permanently_failed` session that reads as "just another NeedsAttention card" is
  exactly the silent-failure-hiding-in-plain-sight failure mode requirements.md's problem
  statement (and the whole reason this feature exists) is trying to eliminate.
- **Social/self job** (same framing as the stale-session sibling doc): fewer moments of
  discovering, hours later via an external channel, that a session had been failing on repeat
  the whole time — retry history's "reason + timestamp per attempt" list is the artifact that
  answers "how long has this actually been broken" after the fact, distinct from the real-time
  badge which only answers "what's happening right now."

## Summary for plan.md to carry forward

1. New "Attempt N/max" badge = new peer in the existing `SessionCard.tsx` header badge row
   (`SessionCard.tsx:493-548`), `compact`/full split like `ReviewQueueBadge`, same
   `role="img"`/`aria-label` convention as every sibling badge. Suppressed until attempt 1 has
   actually failed once (AC4).
2. "Retry now" = clone of `SessionActionsOverflow.tsx`'s existing Restart pattern verbatim:
   primary-button shortcut when the session is in a retry-eligible/exhausted terminal-ish state,
   overflow-menu item (Group 5, destructive-adjacent) otherwise, confirm dialog with
   `useFocusTrap`, `dangerButton` styling, and copy that names which of the two "skip the wait"
   vs. "reset from permanently_failed" cases is happening.
3. Retry history = clone of `CheckpointList.tsx`'s shape verbatim: newest-first, capped list
   with show-more, per-item `{ reason, timestamp }`, empty state, `aria-label` on the `<ul>`.
   Lives as a peer section near checkpoints in `SessionDetailView.tsx` (no dedicated "history"
   view exists yet to slot into).
4. `permanently_failed` should be a new top-level `SessionStatus`, not a new `AttentionReason` —
   renders in the primary (leftmost) badge slot with its own `error`/`errorBg` tokens, never
   sharing color/icon with routine `NeedsAttention` reasons. This is the single most important
   correctness requirement in this doc: distinguishability from generic NeedsAttention is the
   whole point of the feature.
5. No `aria-live` ticking countdown — static point-in-time text recomputed on natural re-render,
   matching the existing `lastActivityRow` pattern; if a visual ticker is wanted, keep it
   non-`aria-live` and reserve live-region announcements for state transitions only.
6. Color tiering for in-progress retries: no badge (attempt 1) → neutral/info (attempt 2+,
   headroom remaining) → `warning` tokens (final attempt) → `error` tokens
   (`permanently_failed`) — reuses the `memoryBadge` three-tier precedent and the
   `warning`/`error` token vocabulary already established by `stale-session-detection` and
   `review-gate-stale-session-rework`, so this feature's badges read as siblings of both rather
   than a fourth, unrelated visual language.
