# UX Design: Session Retry & Backoff

Source: `requirements.md`, `research/ux.md`, `implementation/plan.md` (Phase 4, Epic 2.5.3,
Epic 3.3). This document operationalizes the research doc's recommendations — which this
design does **not** relitigate (badge = new peer in the header row, `permanently_failed` as a
top-level `SessionStatus` per ADR-001, `CheckpointList`/`ReviewQueueBadge`/
`SessionActionsOverflow` as verbatim structural precedents, no `aria-live` ticking countdown)
— into concrete layouts, interaction flows, error states, and testable acceptance criteria for
every surface Phase 4 touches, plus one surface (Surface 6, retry-policy settings) that
Requirements FR1 calls for but Phase 4 does not currently design — see the Gaps section.

Vocabulary used throughout: **Attempt N/max** (retry-in-progress badge, neutral→warning
tiered), **Permanently failed** (top-level status, `error`/`errorBg` tokens, never shares
color/icon with routine `NeedsAttention` reasons).

---

## Surface 1 — `RetryBadge.tsx` (new shared component, header row + expanded body)

### Wireframe — compact variant (header badge row)

```
Header badge row, left → right (existing precedence + new peer):
[ status ]  [ rate-limit ]  [ StatusBadge / SubStatusChip ]  [ 512 MB RAM ]  [ 🔍 Review ]  [ 🔁 2/3 ]

                                                                                              ^^^^^^
                                                                                      new RetryBadge (compact)
                                                                                      position: after ReviewQueueBadge
                                                                                      per plan Task 4.1.1b
```

*(Corrected 2026-08-29 — cross-artifact-consistency pass found this wireframe row drew the badge
before `ReviewQueueBadge`/`🔍 Review`, contradicting both this caption and plan.md Task 4.1.1b's
"position after ReviewQueueBadge." The row above now matches the authoritative spec in
plan.md.)*

Three visual tiers (mirrors `memoryBadge`'s threshold pattern, `SessionCard.tsx:552-554`):

```
attempt 1, no history  →  (no badge rendered at all)
attempt 2+, headroom   →  [ 🔁 2/4 ]   neutral/info tokens
final attempt          →  [ 🔁 3/3 ]   warning tokens (warningBg/warningText/warning)
exhausted              →  handled by Surface 2 (top-level PERMANENTLY_FAILED badge, not this one)
```

### Wireframe — full variant (expanded card body, replacing/next to `reviewItem.context`)

```
┌ Expanded card body ──────────────────────────────────────────┐
│ ...                                                            │
│ 🔁 Attempt 2 of 3 — retrying at 3:42 PM (~45s)                 │
│    Last failure: tmux_exited                                   │
│ ...                                                            │
└──────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. Session crashes → driver increments `retryAttempt`, sets `nextRetryAt`, `lastFailureReason`.
2. Frontend receives the update (existing session poll/websocket push — no new transport).
   `RetryBadge` renders because `retryAttempt > 0`.
3. User hovers/reads the compact badge → glances the fraction, moves on (no click required —
   this is glance-only triage info, matching the memory badge's non-interactive precedent).
4. User expands the card → sees the full-text line with the point-in-time computed "retrying
   at HH:MM (~Ns)" string and the failure reason, satisfying AC3's "states the failure reason"
   at the UI layer as well as in the resumed session's first message.
5. On the next natural re-render (data poll, not a local ticking timer — per research/ux.md §3),
   the "~45s" recomputes from `nextRetryAt - now`. No `setInterval`, no `aria-live` region here.

### Edge cases

- `retryAttempt === 0` (never failed): no badge, no expanded-body line. This is the majority
  case for 5-10 healthy sessions and must add zero visual noise.
- `retryAttempt === retryMaxAttempts` but status is still mid-transition (driver about to mark
  `PermanentlyFailed`): badge shows the warning tier for at most one render cycle; if the poll
  interval is slow enough that this is visible, that's acceptable (matches CI systems' own
  "final attempt" transient state) — it is not a stuck/incorrect state.
- `retryMaxAttempts` changed via settings mid-flight (Surface 6) while a session is already
  retrying: badge shows the fraction as of the session's own resolved policy at retry start,
  not the currently-configured global default — a policy change is not retroactive to
  in-flight sessions. (Backend-owned; UI just renders what the session record says.)
- Very large `retryMaxAttempts` (e.g. a misconfigured policy of 50): badge text is a plain
  fraction and does not visually break — no truncation logic needed, but the frontend should
  not crash on values >99; use plain string interpolation, no fixed-width formatting.

---

## Surface 2 — `PermanentlyFailed` primary status badge (`SessionCard.tsx`)

### Wireframe

```
┌ Card header ───────────────────────────────────────────────┐
│ [ ⛔ Failed — gave up after 3 attempts ]   my-feature-branch │
│  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^                     │
│  leftmost slot — same position STOPPED/PAUSED use today     │
│  error/errorBg tokens — never warning, never shares icon    │
│  with routine NeedsAttention reasons (APPROVAL_PENDING,     │
│  IDLE, STALE, etc.)                                          │
└───────────────────────────────────────────────────────────┘
```

Contrast with a routine `NeedsAttention` card, side by side (the distinction this whole
feature exists to make legible at a glance, per research/ux.md §4):

```
[ 🟡 Needs Attention: Approval Pending ]   ← warning tokens, self-resolving-ish, "look soon"
[ ⛔ Failed — gave up after 3 attempts  ]   ← error tokens, terminal, "look now, I gave up"
```

### Interaction flow

1. Driver exhausts `max_attempts` → `markSessionPermanentlyFailed` transitions `Status` (Epic
   2.5.2) and fires exactly one notification (edge-triggered, not on every re-observation).
2. Card re-renders with the new top-level status in the primary badge slot — no separate
   `AttentionReason` lookup, no competing with `ReviewQueueBadge`'s slot.
3. A toast/notification (existing `NotificationContext` bus, per requirements FR5) surfaces
   the same event outside the card, so a user who isn't looking at the board still learns
   about it — this is the "confidence that they will be told" emotional job from research §5.
4. User clicks the card or the toast → lands on the card / detail view, where Surface 4's
   "Retry now" primary button and Surface 3's retry history are both available.

### Edge cases

- A session with `max_attempts=1` (today's preserved default per AC7) goes `crashed` →
  1 retry → `PermanentlyFailed` on the second failure with **no visible `RetryBadge` ever
  shown** (because `retryAttempt` only reaches 1, and the badge-suppression rule hides
  attempt-1). The *only* visible signal for such a session is the top-level status badge
  itself. This is correct per AC4's "once at least one retry has occurred" gate, but it means
  Surface 2 alone — not Surface 1 — must fully carry the "something happened" signal for the
  minimum-policy case. Call out in acceptance criteria below.
- Two triage-adjacent statuses must never render identically: verify `PermanentlyFailed` and
  `Stopped` (crashed with no retry policy at all, `enabled=false`) are visually distinguishable
  — a user with retries disabled globally still needs to tell "stopped, no retry attempted"
  apart from "stopped, retries exhausted." Recommend `Stopped`/no-policy keeps its existing
  neutral gray, `PermanentlyFailed` is the only status using error/errorBg red — this is
  already implied by plan Task 2.5.3a but is worth stating as an explicit acceptance criterion
  since it's easy to accidentally reuse `Stopped`'s existing color token by copy-paste.

---

## Surface 3 — `RetryHistoryList.tsx` (new component, `SessionDetailView.tsx`)

### Wireframe — populated

```
┌ Session Detail — Retry History ───────────────────────────────┐
│ Retry History                                                   │
│ ┌───────────────────────────────────────────────────────────┐ │
│ │ Attempt 2   tmux_exited      Aug 6, 10:05 AM                │ │
│ │ Attempt 1   crashed          Aug 6, 10:00 AM                │ │
│ └───────────────────────────────────────────────────────────┘ │
│                                              [ Show all (12) ] │
└─────────────────────────────────────────────────────────────┘
```

### Wireframe — empty state

```
┌ Session Detail — Retry History ───────────────────────────────┐
│ Retry History                                                   │
│         No retries yet                                          │
└─────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. User opens a session's detail view (existing navigation — no new route).
2. `RetryHistoryList` renders as a peer section near the existing checkpoints section (no new
   tab), newest attempt first, capped at `MAX_VISIBLE = 10` matching `CheckpointList`.
3. If more than 10 attempts exist (a high-`max_attempts` policy), a "Show all (N)" toggle
   expands the full list in place — identical interaction to `CheckpointList`'s own toggle, so
   a user who already knows that pattern needs no new learning (Nielsen: consistency and
   standards).
4. Each row shows `{ attempt number, reason (crashed/stalled/tmux_exited), relative + absolute
   timestamp }`. Hovering/tapping a row is not interactive (this is a read-only audit log, not
   a set of actionable items) — matches `CheckpointList`'s own read-mostly rows (its optional
   `onDelete` is not applicable here; retry history should not be user-deletable).

### Edge cases

- Zero attempts: "No retries yet" empty state, exact copy match to `CheckpointList`'s "No
  checkpoints yet" convention (same tone, same placement) so the two empty states read as
  siblings within the same detail view.
- A `permanently_failed` session where the user then clicks "Retry now" (Surface 4) and it
  succeeds: the list must show the new attempt appended *above* the prior exhausted run,
  and the top-level status badge (Surface 2) must flip back to `Active`/`Retrying` before the
  next poll — a stale "Permanently failed" badge sitting next to a freshly-succeeding retry
  attempt in the history list would be a confusing, self-contradicting card state.
- Very long-running sessions with dozens of historical attempts across *multiple* exhaustion
  cycles (crashed → exhausted → manual retry → crashed again → exhausted again): the list is a
  flat chronological log, not grouped by "episode." Acceptable for v1 (matches
  `CheckpointList`'s own flat-list precedent) but flag as a future enhancement if a user
  reports it's hard to tell where one give-up cycle ends and the next begins.

---

## Surface 4 — Manual "Retry now" (`SessionActionsOverflow.tsx`)

### Wireframe — primary button (session is `PermanentlyFailed`, card shows primary actions)

```
┌ Card actions row ─────────────────────────────┐
│ [ 🔁 Retry now ]     [ ⋮ ]                      │
│   ^^^^^^^^^^^^                                  │
│   same slot the "🔄 Restart" button uses today  │
│   for plain STOPPED sessions (line ~537)        │
└────────────────────────────────────────────────┘
```

### Wireframe — overflow menu item (session mid-backoff, not yet exhausted)

```
┌ Overflow menu (⋮) ───────────────┐
│ ...                                │
│ Group 4: ...                       │
│ ─────────────────────────────────  │
│ Group 5 (destructive-adjacent):    │
│   🗑️ Clear Conversation            │
│   🔁 Retry now         ← new       │
│   🔄 Restart                       │
│   🗑️ Delete                        │
└───────────────────────────────────┘
```

### Wireframe — confirm dialog, two copy variants

```
Entry point A — mid-backoff (session still auto-retrying):
┌ Retry Session ──────────────────────────────────────┐
│ Skip the wait and retry now?                          │
│ This session is waiting ~38s before its next automatic│
│ attempt (2 of 3). Retrying now skips that wait.        │
│                                                         │
│              [ Retry now ]   [ Cancel ]                │
└────────────────────────────────────────────────────┘

Entry point B — from PERMANENTLY_FAILED:
┌ Retry Session ──────────────────────────────────────┐
│ This session gave up after 3 attempts — retry anyway? │
│ This starts a fresh attempt in the same worktree,      │
│ resetting the attempt counter.                         │
│                                                         │
│              [ Retry anyway ]   [ Cancel ]             │
└────────────────────────────────────────────────────┘
```

### Wireframe — dialog error states

```
Case: concurrent automated retry won the race (CAS guard already claimed):
┌ Retry Session ──────────────────────────────────────┐
│ This session gave up after 3 attempts — retry anyway? │
│                                                         │
│ ⚠ A retry is already in progress for this session —    │
│   no action needed. This dialog will close             │
│   automatically once the status updates.                │
│                                                         │
│              [ Retry anyway ]   [ Cancel ]             │
└────────────────────────────────────────────────────┘
   (Retry button disabled while this message is showing;
    dialog auto-closes on the next status poll that shows
    retryAttempt advanced or status left PERMANENTLY_FAILED)

Case: worktree gone (retry cannot proceed at all):
┌ Retry Session ──────────────────────────────────────┐
│ This session gave up after 3 attempts — retry anyway? │
│                                                         │
│ ⚠ Retry failed: this session's worktree no longer      │
│   exists on disk. Retrying cannot continue.             │
│   [ Delete this session ]  to start fresh instead.      │
│                                                         │
│              [ Retry anyway ]   [ Cancel ]             │
└────────────────────────────────────────────────────┘
   (inline [ Delete this session ] link routes straight to
    the existing Delete confirm flow — the exit path)

Case: generic RPC/network failure:
┌ Retry Session ──────────────────────────────────────┐
│ This session gave up after 3 attempts — retry anyway? │
│                                                         │
│ ⚠ Couldn't reach the server. Check your connection      │
│   and try again.                                        │
│                                                         │
│              [ Retry anyway ]   [ Cancel ]             │
└────────────────────────────────────────────────────┘
   (Retry button re-enabled immediately — this is a
    transient, retryable error, unlike the worktree-gone case)
```

### Interaction flow

1. User sees either the primary `🔁 Retry now` button (`PermanentlyFailed`, primary-action
   slot) or opens the overflow menu and finds `🔁 Retry now` in Group 5 (mid-backoff case).
2. Click opens the confirm dialog (`createPortal` to `document.body`, `useFocusTrap`, `Escape`
   to cancel — cloned verbatim from the existing Restart dialog per `research/ux.md` §1c).
3. Dialog copy branches on entry point (A vs. B above) so the user's mental model matches what
   actually happens — "skip ahead" vs. "give it another chance."
4. Confirm → button shows a loading label (`"Retrying..."`, disabled, mirrors
   `isRestarting`/`"Restarting..."`) → RPC call (`RetrySession`) fires.
5. Success: dialog closes, card re-renders with the session back in an active/retrying state,
   `retryAttempt` reset (if it was the permanently-failed path) or advanced (if mid-backoff).
6. Failure: dialog stays open, inline error message appears above the action buttons (mirrors
   `restartError`'s existing placement), specific to the failure class (see wireframes above).
   The Cancel button always remains available — no failure mode traps the user in the dialog.

### Edge cases

- **Concurrent automated retry claims the CAS guard first** (the scenario named in the task
  brief): backend returns `connect.CodeFailedPrecondition` (plan Task 3.2.2c,
  `ErrRetryInFlight`). UI shows the "already in progress, no action needed" message, disables
  the confirm button (re-clicking would just race again), and auto-closes on the next poll
  once state visibly changed — the user is not left staring at a stale dialog wondering if
  anything happened.
- **Worktree gone** (session's git worktree was deleted externally, e.g. by a cleanup script):
  RPC returns a specific error the frontend maps to the "cannot continue" message with an
  inline exit path to Delete — this is the concrete "no dead ends" case: without the inline
  Delete link, a user hitting this would have a permanently-failed session that can never be
  retried and no obvious next step short of finding Delete buried in the overflow menu
  themselves. **This inline link is a new requirement this design adds beyond the plan's
  current task list — flag for Epic 4.3 as an explicit acceptance criterion (see below).**
- **Double-click / rapid re-click of the primary button** before the dialog finishes opening:
  standard React state guard (dialog open state is a boolean, not a counter) — no special
  handling needed beyond what `isRestartConfirmOpen` already does.
- **Retry succeeds but the session crashes again immediately** (e.g. a systemic problem, not a
  transient one): this is not a UI edge case — it flows through the normal automated-retry path
  again with its own fresh backoff/attempt count, exactly as if this had been an automated
  retry from the start. No special UI state; Surfaces 1-3 simply reflect the new attempt.

---

## Surface 5 — Notification / toast (`NotificationContext`)

### Wireframe

```
┌ Toast (bottom-right or existing toast slot) ─────────────┐
│ ⛔ "my-feature-branch" failed permanently                  │
│    Gave up after 3 attempts. Last failure: tmux_exited.    │
│                                          [ View session ]  │
└────────────────────────────────────────────────────────┘
```

### Interaction flow

1. Fires exactly once per exhaustion episode (edge-triggered per Story 2.5.2's acceptance
   criteria — not on every reconcile pass that re-observes the same `PermanentlyFailed` state).
2. `[ View session ]` scrolls/navigates to the card, consistent with how other existing
   notification types in this app route back to their originating session.
3. Dismissible like every other toast in the existing bus — no new dismiss pattern.

### Edge cases

- User has the tab backgrounded/closed when the notification fires: the toast is necessarily
  missed, but the persistent top-level status badge (Surface 2) means the information is not
  lost — it's simply discovered on next visit instead of in real time. This is why Surface 2
  (persistent, glanceable) and Surface 5 (real-time, ephemeral) are both required, not
  redundant — research/ux.md §5's "trust that they will be told" job needs the persistent
  fallback, not just the transient toast.
- Multiple sessions exhaust retries in a tight window (e.g. a shared infra outage takes out
  3 of 8 running sessions at once): toasts should stack/queue per the existing notification
  bus's behavior, not suppress or collapse into one generic "multiple sessions failed" message
  — each failure needs its own actionable `[ View session ]` target.

---

## Surface 6 — Retry Policy settings (`GlobalDefaultsForm.tsx`) — gap, not yet in Phase 4

Requirements FR1 calls for a **configurable** retry policy (`enabled`, `max_attempts`,
`backoff`, `initial_delay_seconds`, `max_delay_seconds`, `retry_on`), and plan Epic 3.3 wires
`RetryPolicyConfig` through to `sessionDefaultsToProto` plus a "settings-update path so the
frontend can persist a changed global RetryPolicy" (Task 3.3.1b) — but **no Phase 4 story adds
the actual form fields**. Without this surface, the policy is only configurable by hand-editing
the backend JSON config file, which contradicts FR1's own language ("Configurable retry
policy... global default"). This design closes that gap using the existing
`GlobalDefaultsForm.tsx` number-input/label pattern (`maxAutoReworkIterations`'s field is the
direct precedent, lines ~310-322).

### Wireframe

```
┌ Global Defaults — Retry Policy ──────────────────────────────┐
│ ☑ Enable automatic retry on session crash                      │
│                                                                  │
│ Max attempts            [ 3 ]     (1-10)                       │
│ Initial delay (seconds) [ 30 ]                                 │
│ Max delay (seconds)     [ 300 ]                                │
│                                                                  │
│ Retry on:                                                       │
│   ☑ Crashed        ☑ Stalled        ☑ Tmux exited              │
│                                                                  │
│ ☐ Also retry when a session is flagged stale                   │
│   (uses the stale-session-detection threshold)                  │
└────────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. User opens Settings → Global Defaults (existing navigation, existing form).
2. New "Retry Policy" section appears as a peer of the existing sections (CLI flags, max
   auto-rework iterations, etc.) — same `label`/`input` CSS classes, no new visual language.
3. Unchecking "Enable automatic retry" grays out (disables, does not hide) the fields below it
   — matching how other conditionally-relevant settings in this form already behave, and
   avoiding layout shift on toggle.
4. Save persists via the existing form-wide save action (no new save button needed).
5. Changed values apply to *newly started* retry episodes only — see Surface 1's edge case
   note that in-flight sessions keep the policy resolved at their own retry-episode start.

### Edge cases

- `max_attempts` input allows 0 or negative: clamp to minimum 1 client-side (mirrors
  `Math.max(1, Number(e.target.value) || 1)` already used for `maxAutoReworkIterations`,
  line 320) — this also protects AC7's "sane default preserves at least today's retry-once
  behavior" from being silently defeated by a fat-fingered 0.
- All three "Retry on" checkboxes unchecked while "Enable automatic retry" stays checked: this
  is a valid-but-useless configuration (retry enabled, but no failure mode triggers it) — show
  a non-blocking inline hint ("No failure types selected — automatic retry will never trigger")
  rather than blocking Save, since it's not actually invalid, just probably a mistake.
- Per-session override (`CreateSessionRequest.retry_policy_override`, plan Task 3.1.2c) has no
  corresponding UI in this design — the proto field exists for future/API use but Phase 4 does
  not surface it in the session-creation Omnibar. This is an intentional scope boundary
  (YAGNI — global default covers the single-user use case), not an oversight, but note it
  explicitly here so it isn't mistaken for a missed touchpoint of
  `.claude/rules/session-creation-registry.md` (no new session-creation *mode* is being added,
  so that registry's 7 touchpoints don't apply).

---

## Mobile / responsive notes

Per the standing mobile+desktop UX requirement: session cards already reflow to a
single-column, full-width layout on narrow viewports.

- **Badge row wraps, doesn't truncate.** `RetryBadge` and the `PermanentlyFailed` status badge
  must wrap onto a second line with the rest of the badge row on narrow viewports (existing
  flex-wrap behavior) rather than being clipped or hidden — a retry-count badge that silently
  disappears on mobile would defeat the "glance triage" job for a user checking sessions from
  their phone.
- **Primary "Retry now" button and overflow menu item are ≥44×44px touch targets**, matching
  the existing `actionButton`/`overflowMenuItem` sizing already used by Restart/Pause/Resume —
  no new sizing rule needed since this clones those components' classes directly.
- **Confirm dialog is full-width (not full-screen) on mobile**, matching the existing Restart/
  Delete dialogs' current responsive behavior (`createPortal` + `dialogContent`'s existing
  max-width/margin rules) — no new dialog breakpoint needed since Surface 4 reuses that
  component's CSS verbatim.
- **RetryHistoryList rows stack vertically** (attempt/reason/timestamp on separate lines rather
  than a wide row) below the existing `CheckpointList` responsive breakpoint, since that's the
  component this list clones structurally.

---

## UX Acceptance Criteria (human-testable)

### Task completion

1. A user scanning the session board can identify a `permanently_failed` session **in ≤1
   glance, 0 clicks** — the top-level badge alone (icon + "Failed — gave up after N attempts"
   text) is sufficient, with no need to expand the card or open the overflow menu.
2. A user can manually retry a `permanently_failed` session in **≤3 clicks**: (1) click
   primary "Retry now" button, (2) confirm in dialog, (3) [implicit] — no third click needed;
   count is 2 clicks for the primary-button path, 3 for the overflow-menu path (open menu →
   click item → confirm).
3. A user can view the full retry history for a session in **≤2 clicks** from the board:
   (1) open the session's detail view, (2) history is visible without further interaction
   (no accordion-to-expand required for the first 10 items).
4. A user can determine *why* the last retry happened (crashed vs. stalled vs. tmux_exited)
   without leaving the card list — the expanded card body's full-text `RetryBadge` line shows
   the reason inline, no drill-down to detail view required for the *most recent* failure
   (drill-down is only required for full history beyond the latest attempt).

### Error / edge-case states

5. Clicking "Retry now" when a concurrent automated retry has already claimed the CAS guard
   shows the exact message "A retry is already in progress for this session — no action
   needed," disables further retry attempts from that dialog, and the dialog auto-closes once
   the session's state visibly advances — the user is never left wondering if their click did
   anything.
6. Clicking "Retry now" when the session's worktree no longer exists shows a message stating
   retry cannot continue **and** offers an inline path to Delete the session — this is the
   required exit path; a user must never be left with a `permanently_failed` session that has
   no available next action.
7. Clicking "Retry now" during a generic network/RPC failure shows a message distinct from
   cases 5 and 6 (implies "try again," not "this is stuck" or "delete and start over"), and the
   confirm button remains enabled for an immediate re-attempt.
8. **No dead ends**: every error state reachable from the "Retry now" flow (in-progress,
   worktree-gone, generic failure) has a visible exit — Cancel (always), Delete (worktree-gone
   case), or immediate re-attempt (generic-failure case). Verified: no error state in Surface 4
   omits an action.
9. A session with the minimum default policy (`max_attempts=1`, preserving today's behavior
   per AC7) that exhausts its single retry shows **no** `RetryBadge` at any point (attempt-1
   suppression rule) but **does** show the `PermanentlyFailed` top-level badge — Surface 2 must
   independently carry the full signal in this case; this is a explicit test case, not just an
   inference, because it is the one configuration where Surface 1 contributes nothing.
10. An empty retry history ("No retries yet") is visually and textually distinct from a
    loading state — a session detail view that hasn't finished fetching retry history must not
    flash "No retries yet" before data arrives (loading skeleton or blank, not a false-empty
    state).

### Accessibility (extends research/ux.md §3 — no contradictions)

11. `RetryBadge` uses `role="img"` with `aria-label={"Retry attempt " + n + " of " + max}`,
    icon marked `aria-hidden="true"` — matches every existing sibling badge's convention
    verbatim (validated against `SessionCard.tsx:509-510`'s pattern; no deviation).
12. The `PermanentlyFailed` status badge's `aria-label` reads the full sentence ("Session
    status: Failed — gave up after 3 attempts"), not just "Failed" — a screen reader user must
    get the same "how many attempts" information a sighted user reads from the badge text.
13. No `aria-live` region ticks on every second of a visible countdown — confirmed no
    implementation introduces a per-second-updating `aria-live="polite"` container (per
    research/ux.md §3's explicit anti-pattern warning); if a state-transition announcement is
    added, it fires only on retry-started/retry-succeeded/permanently-failed transitions.
14. "Retry now" (both primary button and overflow menu item) is reachable via keyboard alone:
    Tab to the button/menu item, Enter/Space to activate, Tab within the confirm dialog is
    trapped (`useFocusTrap`), Escape cancels — identical to the existing Restart flow's
    keyboard behavior, verified by not introducing any `onClick`-only (non-`<button>`) handler.
15. Color is never the sole signal distinguishing `PermanentlyFailed` from routine
    `NeedsAttention` states — both must differ in icon and text label as well as color/token,
    satisfying WCAG 1.4.1 (verified: Surface 2's wireframe pairs ⛔+"Failed — gave up..." vs.
    🟡+"Needs Attention: ...").
16. All new interactive elements (Retry now button/menu item, Delete-session inline link in
    the worktree-gone error, Retry Policy form fields) meet ≥4.5:1 text contrast and ≥44×44px
    touch target size — inherited automatically by cloning existing components' CSS, but
    called out explicitly here as a regression check since it's easy to lose when a "clone"
    task is implemented by copy-pasting markup without copying the associated `.css.ts` file.

---

## Gaps found (for the coordinator)

1. **Missing surface, not a missing exit path**: Phase 4 of `implementation/plan.md` has no
   story for the retry-policy settings form (Surface 6 above), despite Requirements FR1
   explicitly requiring a configurable policy and Epic 3.3 wiring the backend persistence path
   for it. This is a plan gap, not a UX dead end — recommend adding an Epic 4.5 (or extending
   Epic 3.3) to implement Surface 6 before this feature is considered to satisfy FR1. Flagging
   for the coordinator to route back into planning; not blocking Phase 4's existing four epics.
2. **One new acceptance criterion beyond the current plan**: Task 4.3.1d covers confirm-dialog
   copy branching for entry point A vs. B, but the current plan does not have an explicit task
   for the worktree-gone error's inline "Delete this session" link (AC6 above). Recommend
   adding this as a sub-task under Story 4.3.1 so the "no dead ends" requirement has an
   explicit implementation home, not just an acceptance criterion with no owning task.
3. **No flow was found with a genuine, unaddressed dead end** once the worktree-gone case's
   inline Delete link (gap #2) is added — every other error/edge state identified above already
   has a clear exit (Cancel, immediate retry, or the persistent top-level status badge as a
   fallback when a toast is missed). No repair loop is needed for interaction design itself;
   the two items above are additive planning gaps, not usability defects in what's already
   planned.

---

## Summary for implementation

- 6 surfaces designed: (1) `RetryBadge` compact+full, (2) `PermanentlyFailed` top-level status
  badge, (3) `RetryHistoryList`, (4) manual "Retry now" (button + menu item + confirm dialog +
  3 error variants), (5) notification/toast, (6) retry-policy settings form (gap-filling, not
  in current Phase 4 scope).
- 16 UX acceptance criteria written, split across task-completion (4), error/edge-case (6), and
  accessibility (6).
- 2 planning gaps flagged (missing settings-form epic; missing task for the worktree-gone
  inline Delete link) — both additive, neither is an unaddressed dead end in what Phase 4
  already scopes.
