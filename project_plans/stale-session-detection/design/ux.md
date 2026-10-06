# UX Design: stale-session-detection

**Phase**: SDD Phase 4.5 (pre-implementation UX design), for the plan at
`project_plans/stale-session-detection/implementation/plan.md`.
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md`.

This doc turns the UX research's recommendations into concrete wireframes, interaction
flows, edge-case handling, and testable acceptance criteria for the five user-facing
surfaces this feature touches. It does not re-litigate architecture/component names —
those are locked in `plan.md`; this doc is the visual/interaction spec for what `plan.md`
builds.

---

## Surfaces identified

| # | Surface | Component (per plan.md) | New/Extended |
|---|---------|--------------------------|---------------|
| 1 | Stale badge on session cards | `SessionCard.tsx` header badge row | New peer badge |
| 2 | "Stale" grouping/filter | `web-app/src/lib/grouping/strategies.ts` + `SessionList.tsx`'s native `<select>` | New enum + case |
| 3 | Stale-session notification | Toast (`NotificationToast.tsx`) + persistent panel (`NotificationPanel.tsx`) via `NotificationContext.tsx` | New event source, existing UI |
| 4 | Threshold/notify config | `GlobalDefaultsForm.tsx` (Settings, full page) | New fields |
| 5 | `min_session_idle_minutes` rule condition | `RuleBuilderForm.tsx` (modal, opened from `ApprovalRulesPanel.tsx`) | New field |

---

## Surface 1 — SessionCard Stale Badge

### Wireframe

Badge row ordering per `SessionCard.tsx:505-548` (status → rate-limit → StatusBadge/
SubStatusChip → **Stale [new]** → memory → autonomous → workflow → pending-program),
followed by the existing "Last Activity" row unchanged:

```
┌────────────────────────────────────────────────────────────────────┐
│ [● Active] [SubStatus: Idle] [🟠 Stale]  [⚙ nightly-sync]           │  ← header badge row
│ fix-flaky-auth-test                                                 │
│ backend  api                                            [Edit Tags] │
│ Active 47m ago                                                      │  ← lastActivityRow, unchanged
└────────────────────────────────────────────────────────────────────┘

Non-stale card (fresh, or non-ACTIVE — no badge, ever):
┌────────────────────────────────────────────────────────────────────┐
│ [● Active] [SubStatus: Working]                                     │
│ implement-oauth-refresh                                              │
│                                                          [Edit Tags] │
│ Active 2m ago                                                        │
└────────────────────────────────────────────────────────────────────┘

Paused session, idle 6h — badge suppressed even though timestamps are old:
┌────────────────────────────────────────────────────────────────────┐
│ [⏸ Paused]                                                           │
│ deferred-refactor-task                                                │
│                                                          [Edit Tags] │
│ Active 6h ago                                                        │
└────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. User opens the session list (default view, no action required).
2. Every rendered `SessionCard` computes `isSessionStale(session, staleThresholdMinutes)`
   at render time.
3. If `true`: an amber pill `🟠 Stale` renders inline in the header row, `role="img"`,
   `aria-label="Stale — no output for 47m"` (mirrors the existing badge pattern's
   full-sentence label style at `SessionCard.tsx:509-510`).
4. User hovers/focuses the badge → no tooltip needed (the adjacent "Active 47m ago" row
   already carries the exact duration — badge and timestamp are the same signal at two
   granularities per UX research §2, so no redundant tooltip).
5. Session produces new output *or* wall-clock crosses the 60s tick
   (`SessionList.tsx` `forceStaleRecompute`, Epic 2.3) → badge disappears/appears on the
   next recompute, no manual refresh needed, no page reload.

### Edge cases

- **Paused/Stopped/Hibernated/Creating session**: badge never renders, regardless of how
  old the last-activity timestamp is (`isSessionStale` gates on `SessionStatus.ACTIVE`
  first, plan Task 2.1.1b). This is a correctness requirement, not a display nuance — a
  paused session showing "Stale" would actively mistrain the user to ignore the badge.
- **Session recovers after going stale**: on its next output, `lastMeaningfulOutput`
  updates, `isSessionStale` immediately returns `false` on the next render/tick — no
  stuck badge, no manual dismiss needed.
- **Session with no associated backlog item** (manual/one-off session): unaffected. The
  staleness signal is `ReviewState.LastMeaningfulOutput`/`LastTerminalUpdate`, tracked
  per `session.Instance` independent of any backlog item — the badge works identically
  whether or not a backlog item exists.
- **Brand-new session, no output yet**: both timestamps unset (`seconds: 0n`) →
  `getLastActivityTimestamp` returns `undefined` → `isSessionStale` returns `false`
  (never flag a session that hasn't had a chance to produce output yet, plan Task
  2.1.1b / `features.md` §3 precedent).
- **Config not yet loaded** (`useStaleSessionConfig` fetch in flight): defaults to
  `{thresholdMinutes: 30, notifyEnabled: true}` — badge computation never blocks on the
  network round trip; worst case, a card briefly uses the fallback default threshold
  instead of a saved 45-minute override for one render.

### Acceptance criteria

- **AC1.1**: User can identify the one stale session among 10 rendered cards in 0
  additional clicks/steps beyond the default session-list view (no menu, no toggle) —
  the badge is always-on for stale `ACTIVE` sessions.
- **AC1.2**: Badge text reads "Stale" and is never the only signal — an
  `aria-label` starting with `"Stale — no output for"` is present whenever the visible
  pill is present (WCAG 1.4.1 — color is never the sole indicator).
- **AC1.3**: No PAUSED, STOPPED, HIBERNATED, or CREATING session ever shows the Stale
  badge, verified by `SessionCard.test.tsx`'s Given-When-Then for a 6-hour-idle PAUSED
  session (plan Story 2.2.1, second AC).
- **AC1.4**: Badge visually reuses `vars.color.warningBg` / `vars.color.warningText` /
  `vars.color.warning` (the exact triplet `chipStaleWork` uses in
  `web-app/src/components/backlog-stuck/StuckItem.css.ts:44-51`) — no new hex value
  introduced. Contrast is inherited from that already-audited triplet (≥ 4.5:1), so no
  new contrast check is required as long as the token names are reused verbatim.
- **AC1.5**: No dead end — the badge is purely informational (not a button/link in this
  Complexity-2 scope); clicking the card still opens the session as normal, so there is
  no trap state introduced by the badge's presence.

---

## Surface 2 — "Stale" Grouping/Filter Strategy

### Wireframe

Native `<select>` at `SessionList.tsx:1117-1130`, `GroupingStrategyLabels` entries as
options (existing 9 + new "Stale"):

```
Group by (Keyboard: G): [ Stale        ▾ ]   Sort by: [ Last Activity ▾ ]  [↓]
                            Status
                            Category
                            Tag
                            Branch
                            Path
                            Program
                            Session Type
                            Project
                            Workflow
                          → Stale        ← new entry, appended
```

Resulting grouped view once "Stale" is selected:

```
▾ Stale (2)
  ┌──────────────────────────────┐  ┌──────────────────────────────┐
  │ [🟠 Stale] fix-flaky-auth-test│  │ [🟠 Stale] migrate-db-schema  │
  └──────────────────────────────┘  └──────────────────────────────┘

▾ Active (8)
  ┌──────────────────────────────┐  ┌──────────────────────────────┐
  │ implement-oauth-refresh       │  │ [⏸ Paused] deferred-refactor  │
  └──────────────────────────────┘  └──────────────────────────────┘
  ... (6 more)
```

Empty state (zero stale sessions right now):

```
Group by: [ Stale ▾ ]

              Nothing here — no sessions are currently stale.
     (Every active session has produced output within the last 30m.)

              [ Group by: Status ▾ ]  ← quick exit back to a useful view
```

### Interaction flow

1. User opens the "Group by" `<select>` (mouse click, or keyboard shortcut `G` per the
   existing `title="Group by (Keyboard: G)"` hint).
2. User selects "Stale".
3. `groupSessions(sortedSessions, GroupingStrategy.Stale, { thresholdMinutes })`
   re-buckets the already-in-memory session list client-side — no RPC round trip, no
   loading spinner (pure client computation, plan Pattern Decisions table row 6).
4. Two groups render: "Stale" (only `ACTIVE` sessions past threshold) and everything
   else, under whatever label the non-stale bucket key resolves to.
5. Selecting the tick (60s `SessionList` re-render, Epic 2.3) re-buckets live if a
   session crosses the threshold while the user has this grouping open — a card can
   visibly move from the non-stale group into "Stale" without user action.

### Edge cases

- **Zero stale sessions**: show the standard empty-group state (per existing grouping
  empty-state pattern) rather than an error — "No stale sessions" is a *good* outcome
  message, not a failure. Provide a one-click path back to a useful grouping (e.g. a
  persistent "Group by" selector is always visible, so no dedicated "reset" button is
  needed — changing the dropdown is the exit path).
- **Fallback-bucket mislabeling risk (flagged for implementation)**: `plan.md` Task
  3.1.1b buckets every non-stale session (including PAUSED/STOPPED sessions, which
  `isSessionStale` also returns `false` for) into a bucket literally named `"Active"`.
  A PAUSED session appearing inside a group header labeled "Active" is misleading —
  the grouping's own semantics ("is this session's output current") get conflated with
  the unrelated `SessionStatus.ACTIVE` enum value in the label text. **Recommendation**:
  label the fallback bucket `"Not Stale"` instead of `"Active"` (the enum member name
  `GroupingStrategy.Stale`'s two buckets don't need to match `SessionStatus` naming) —
  a one-string change in Task 3.1.1b with no other impact. Flagging here since it's a
  copy/wording decision naturally owned by UX design, not by the architecture/plan
  layer.
- **All sessions stale**: no special case — a "Stale (10)" group containing the entire
  workspace is itself the useful signal (something environment-wide broke, e.g. a
  shared credential expired), not an error state to special-case away.
- **User has `staleThresholdMinutes` misconfigured to something absurd** (e.g. `1`
  minute): every recently-touched session floods into "Stale" — this is a Settings
  (Surface 4) input-validation concern, not something Surface 2 needs to guard against
  independently; the grouping strategy is a faithful mirror of whatever threshold is
  configured.

### Acceptance criteria

- **AC2.1**: User can view all stale sessions grouped together in ≤ 2 steps (open
  dropdown, select "Stale") from the default session list.
- **AC2.2**: The "Stale" option has a non-blank, keyboard-reachable label — selectable
  via `Tab`/arrow keys/`Enter` like every existing native `<select>` option, with no new
  ARIA pattern required (inherits the existing `aria-label="Group sessions by (keyboard: G)"`
  from the `<select>` itself).
- **AC2.3**: A PAUSED session with a 6-hour-old timestamp never appears in the "Stale"
  group (mirrors AC1.3's gate, applied to grouping membership instead of the badge).
- **AC2.4**: Zero stale sessions shows a clear "no stale sessions" empty state with an
  always-visible exit path (the same "Group by" selector) — no dead end, per plan
  Story 3.1.1's acceptance criteria pattern.

---

## Surface 3 — Stale-Session Notification (toast + persistent panel)

Two connected UI pieces, both already-existing infrastructure per UX research §1 and
confirmed via code read: `NotificationToast.tsx` (ephemeral, auto-minimizing) mounted by
`NotificationContext.tsx:454-471`, and `NotificationPanel.tsx` (persistent history,
`role="dialog"`, opened from `NotificationsNavBadge.tsx`'s unread-count pill).

### Wireframe

```
Moment the session crosses the threshold (StaleSessionNotifier ticks, publishes event):

                                                    ┌───────────────────────────────┐
                                                    │ ⚠  Session went stale          │
                                                    │ fix-flaky-auth-test            │
                                                    │ No output for 30m               │
                                                    │                    [Focus]  [✕] │
                                                    └───────────────────────────────┘
                                                       bottom-right toast, fixed position

~5s later — auto-minimizes to a compact pill (per notification-policy.ts, warning
type auto-minimizes at 5000ms), does NOT auto-close (stays available, unlike lower-
priority toasts):

                                                                  ┌───────────────┐
                                                                  │ ⚠ 1 warning  ▾ │
                                                                  └───────────────┘

Nav badge always reflects unread count regardless of toast state:

  [🔔 3]  ← NotificationsNavBadge, unread count includes this stale notification

Opening the panel (click nav badge) shows it persists in history:

┌─ Notifications ─────────────────────────────────────────── ✕ ─┐
│ ● ⚠  Session went stale                                        │
│      fix-flaky-auth-test                                       │
│      No output for 30m                              3m ago  ✕  │
│      [Focus session]                                            │
│ ──────────────────────────────────────────────────────────────│
│ ○  PR ready for review                                          │
│      migrate-db-schema                              1h ago  ✕  │
└──────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. `StaleSessionNotifier` ticks every 60s server-side (plan Task 4.1.1b); a session
   crosses `ThresholdMinutesOrDefault()` for the first time in its current stale episode.
2. Event bus publishes `NOTIFICATION_TYPE_WARNING` → frontend `NotificationContext`
   receives it → `addNotification()` pushes into both `notifications` (drives the toast)
   and `notificationHistory` (drives the panel) simultaneously.
3. Toast appears bottom-right, fully visible, with a "Focus" action (jump to the
   session) and a manual "✕" dismiss.
4. After ~5s (warning-type auto-minimize timing in `notification-policy.ts`), the toast
   collapses to a compact pill rather than disappearing entirely — user doesn't have to
   catch it in a 5-second window to act on it later; it stays reachable.
5. Independent of toast state, the nav badge's unread count increments and the entry is
   durably available in `NotificationPanel` until the user manually dismisses it there
   — satisfying the "confidence nothing was missed" emotional job named in UX research
   §5 (a badge with no manual-dismiss requirement would risk the exact anxiety this
   feature exists to reduce).
6. User clicks "Focus" (toast) or "Focus session" (panel) → navigates to that session.
7. If the session recovers (produces output, idle time drops under threshold) and later
   goes stale again, a **second, new** notification fires — re-arm behavior (plan Story
   4.1.1, "Re-arms after recovery" AC).

### Edge cases

- **`stale_notify: false` in config**: `StaleSessionNotifier.checkAll()` never calls
  `notify()` (plan Task 4.1.1c) — no toast, no panel entry, no nav-badge increment. The
  badge (Surface 1) and grouping (Surface 2) remain fully functional and unaffected —
  notification is one of three independent delivery mechanisms for the same signal
  (UX research §5), not a prerequisite for the others.
- **Session pauses in the same tick it crosses the threshold**: status is checked at
  *emission* time inside `checkAll()`, not just at threshold-crossing time — a user who
  deliberately pauses a session never gets a spurious "went stale" notification for an
  action they just took on purpose (plan Story 4.1.1, third AC; UX research §4's
  flapping-transition edge case).
- **Flapping active/stale transitions**: the in-memory `notifiedSessions` dedup map
  (edge-triggered: notify once on cross, clear on recovery, plan Task 4.1.1c) prevents
  a session bouncing near the threshold boundary from spamming duplicate toasts —
  exactly one notification per continuous stale episode.
- **Event bus publish fails / server restarts mid-episode**: `notifiedSessions` is
  in-memory only (not persisted, plan's Domain Glossary) — a server restart re-arms
  every currently-stale session, so the user gets at most one redundant notification
  per restart rather than silence. This is the documented, accepted tradeoff (no
  migration/durable-state cost for a Complexity-2 feature) — acceptable because a
  redundant notification is a much smaller UX cost than a missed one.
- **Many sessions go stale simultaneously** (e.g. shared credential expiry): each gets
  its own toast, stacking bottom-right per the existing multi-toast stacking behavior
  already used by other notification types — no new stacking logic needed for this
  feature.

### Acceptance criteria

- **AC3.1**: A session crossing the threshold produces exactly one toast and one panel
  entry — never zero (if `notify_enabled: true`) and never more than one per continuous
  stale episode.
- **AC3.2**: Toast auto-minimizes but does not auto-close for warning-severity — user
  can always still find and act on it via the persistent minimized pill or the panel,
  satisfying "no dead ends" even after the 5-second full-visibility window elapses.
- **AC3.3**: Every notification instance offers an action to reach the affected session
  (toast: "Focus"; panel: "Focus session") — the notification is never a dead end that
  only informs without offering a next step.
- **AC3.4**: Disabling `stale_notify` in Settings produces zero toasts/panel
  entries/nav-badge increments for stale sessions on the very next `StaleSessionNotifier`
  tick, verified without requiring a server restart (config hot-reload per
  `config.LoadConfig()`, plan Risk Control section).
- **AC3.5**: A deliberately-paused session never produces a stale notification, even if
  paused in the exact tick its idle time crosses the threshold (regression guard for the
  "self-inflicted spurious notification" edge case above).

---

## Surface 4 — Settings: Stale Session Threshold + Notify Toggle

### Wireframe

`GlobalDefaultsForm.tsx` is a full settings page (`web-app/src/app/settings/page.tsx`),
not a modal. New fields sit alongside `maxAutoReworkIterations` using the existing
labeled-above-input + helper-text convention:

```
Settings › Session Defaults
──────────────────────────────────────────────────────────────

  Max auto-rework iterations
  [ 3                    ]
  Number of automatic rework attempts before escalating to review.

  Stale session threshold (minutes)                          ← new
  [ 30                   ]
  How long a session can produce no output before it's flagged
  "Stale" on its card and in the "Stale" grouping filter.

  ☑ Notify when a session goes stale                          ← new
  Sends a notification the first time a session crosses the
  threshold above. Turn off to rely on the card badge only.

  [ Save ]
```

Validation error state (user enters `0` or a negative number):

```
  Stale session threshold (minutes)
  [ 0                    ]  ⚠ Must be a positive number — 0 disables nothing here;
                              use the toggle above to disable notifications instead.
                              (Saved value falls back to the default: 30)
```

### Interaction flow

1. User navigates to Settings (existing nav entry, no new touchpoint).
2. `GlobalDefaultsForm` mounts, calls `getSessionDefaults()`; the numeric input
   pre-fills with `staleSessionThresholdMinutes` (defaults to `30` before the fetch
   resolves) and the checkbox pre-fills with `staleSessionNotifyEnabled` (defaults
   `true`).
3. User edits the threshold and/or toggles the checkbox.
4. User clicks the existing "Save" button (no new save action) → `updateGlobalDefaults`
   is called with both new fields alongside existing ones.
5. Server resolves and echoes back the applied (never-zero) value; form re-displays the
   resolved value, confirming the save took effect.

### Edge cases

- **User enters `0` or a negative number**: per `ThresholdMinutesOrDefault()`'s
  server-side contract (plan Story 1.1.1), this is *not* rejected as a hard validation
  error that blocks Save — it silently resolves to the default (30) server-side. The
  form should still surface an inline hint (as sketched above) so the user understands
  *why* their explicit `0` didn't "stick" as `0`, rather than silently reverting behind
  their back — this prevents the confusing outcome of "I saved 0, but the badge still
  fires at 30 minutes" with no visible explanation.
- **Save RPC fails** (network error, server unavailable): follow whatever existing
  error-surfacing convention `GlobalDefaultsForm.tsx` already uses for its other fields
  (e.g. `maxAutoReworkIterations`) — an inline error message near the Save button, form
  values remain as the user typed them (not silently reverted), and Save remains
  clickable to retry. No new error pattern needed; this is inherited behavior, called
  out here only to confirm the new fields don't bypass it.
- **User navigates away with unsaved changes**: inherits whatever existing
  unsaved-changes handling (or lack thereof) the rest of `GlobalDefaultsForm` already
  has — this feature does not introduce a new save-guard requirement beyond parity with
  sibling fields.

### Acceptance criteria

- **AC4.1**: User can change the stale threshold and notify flag in ≤ 3 steps (navigate
  to Settings, edit field(s), click Save) — no new page, no modal.
- **AC4.2**: Loading the form always shows a resolved (non-blank, non-zero) threshold
  value, even before the network fetch completes (client-side default `30`).
- **AC4.3**: Entering `0` shows an inline explanatory hint rather than silently
  reverting with no feedback — "0 falls back to the default (30)" or equivalent,
  co-located with the input.
- **AC4.4**: A failed save leaves the user's typed values on screen (not reverted) and
  offers a visible retry path (Save button remains actionable) — no dead end.

---

## Surface 5 — Approval Rule Builder: `min_session_idle_minutes` Condition

### Wireframe

`RuleBuilderForm.tsx`, rendered inside a modal (`role="dialog"`, opened from
`ApprovalRulesPanel.tsx`). New numeric field sits as a sibling to the existing
`require-ci-passing-checkbox`, following that region's layout convention:

```
┌─ Edit Approval Rule ────────────────────────────────────── ✕ ─┐
│                                                                  │
│  Rule name:  [ Deny long-idle auto-approvals            ]      │
│  Decision:   ( ) Approve  (•) Deny  ( ) Escalate                │
│  Tool:       [ Bash                                    ▾]      │
│                                                                  │
│  ☑ Require CI passing on this branch                            │
│                                                                  │
│  Minimum session idle time (minutes)                    ← new  │
│  [ 60                    ]                                      │
│  Only match if the requesting session has produced no output    │
│  for at least this many minutes. 0 = not applied.                │
│                                                                  │
│                                        [ Cancel ]   [ Save Rule ]│
└──────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. User opens "Approval Rules" panel, clicks "New rule" or edits an existing one →
   `RuleBuilderForm` opens as a modal.
2. Existing rule: numeric input pre-fills with `editRule.minSessionIdleMinutes` (0 if
   unset).
3. User sets a value (e.g. `60`) alongside any other conditions (tool, CI-passing,
   etc.) — the idle-minutes condition ANDs with every other condition on the rule, it
   never OR's or overrides them (plan Epic 5.3).
4. User clicks "Save Rule" → `upsertApprovalRule` payload includes
   `minSessionIdleMinutes: 60`.
5. Rule now participates in live classification: a permission request from a session
   idle ≥ 60 minutes matches this condition; a session idle < 60 minutes, or a session
   whose idle time is unknown (no live instance found), does not match — fails closed
   (plan Epic 5.3/5.4).

### Edge cases

- **`0` (default/unset)**: condition is not applied at all — same idiom as
  `require_ci_passing`'s boolean off-state; explicitly documented in the field's helper
  text ("0 = not applied") so a user doesn't mistake `0` for "match only brand-new
  sessions."
- **Combined with other conditions**: the form does not need special UI to communicate
  AND-only composition — every condition in this form already ANDs together (existing
  `matchesRule` behavior, plan Pattern Decisions row 4), so no new interaction pattern
  is introduced; this field is visually just another row in the same flat list.
- **Unknown idle time at evaluation time (fail-closed)**: this is invisible at rule
  *authoring* time — the form has no way to preview "what if idle time is unknown," and
  per plan Epic 5.3/5.4 that case always fails the condition (never accidentally
  matches). No additional UI is warranted for a Complexity-2 scope; this is a
  documented behavioral contract, not a surface a user configures. Recommendation: the
  helper text under the field should make the fail-closed contract explicit for rule
  authors writing DENY rules with this condition, since an unexpectedly-lenient
  DENY-on-idle rule (that silently never fires because idle time couldn't be
  determined) is a much worse silent failure than an unexpectedly-strict one. Suggested
  copy: *"Only match if the requesting session has produced no output for at least this
  many minutes. 0 = not applied. If the session's idle time can't be determined, this
  condition does not match."*
- **Save fails**: modal stays open, user's entered values remain, existing
  `RuleBuilderForm` error-surfacing convention applies (no new pattern needed) — same
  no-dead-end requirement as Surface 4.

### Acceptance criteria

- **AC5.1**: User can add an idle-minutes condition to a rule in ≤ 1 additional step
  beyond an existing rule edit (fill in one numeric field, no new modal/page).
- **AC5.2**: The field's helper text explicitly states both the "0 = not applied" idiom
  and the fail-closed behavior for unknown idle time — a rule author should never be
  surprised that a DENY-on-idle rule silently never fired.
- **AC5.3**: Saving a rule with this field set round-trips correctly — editing the same
  rule again shows the previously-saved value, not a reverted `0`.
- **AC5.4**: A failed save leaves the modal open with the user's input intact and a
  visible retry path (Save Rule button remains actionable) — no dead end.

---

## Cross-cutting UX Acceptance Criteria

These apply across all five surfaces.

### Task efficiency

- **X1**: Finding the one stale session among 5–10 running sessions takes 0 clicks
  (badge, always visible) or ≤ 2 clicks (grouping filter) — never requires opening each
  session individually, directly satisfying the problem statement in
  `requirements.md:10-11`.
- **X2**: Configuring the feature end to end (threshold + notify flag) takes ≤ 3 steps
  from Settings; adding the approval-rule condition to an existing rule takes ≤ 1
  additional step within the rule editor already open for other reasons.

### Error / no-dead-ends

- **X3**: Every error state identified above (Settings save failure, rule-builder save
  failure, invalid threshold input) leaves the user's in-progress input on screen and
  offers a specific, visible retry action — never a blank form, a silent revert with no
  explanation, or a state requiring page reload to recover from.
- **X4**: Every notification (toast or panel entry) offers a concrete next action
  ("Focus"/"Focus session") — informational-only notifications with no path to the
  affected session do not exist in this feature.

### Accessibility

- **X5 — Color is never the sole signal (WCAG 1.4.1)**: the Stale badge (Surface 1)
  always pairs the icon with the visible text "Stale"; the toast/panel entries
  (Surface 3) always pair the warning icon with title/message text — consistent with
  the codebase-enforced rule already documented at
  `web-app/src/components/backlog-stuck/stuckReason.ts:33`.
- **X6 — `role="img"` + full-sentence `aria-label`**: the new Stale badge follows the
  exact pattern every sibling badge in `SessionCard.tsx`'s header row already uses
  (e.g. `SessionCard.tsx:509-510`) — `aria-label="Stale — no output for {duration}"`,
  icon marked `aria-hidden="true"`.
- **X7 — Keyboard navigation**: the "Stale" grouping option is reachable via the
  existing native `<select>`'s standard keyboard interaction (`Tab`, arrow keys,
  `Enter`/`Space`) with no new focus trap or custom widget introduced. The Settings
  numeric input/checkbox and the rule-builder numeric input use native `<input>`
  elements, inheriting standard tab-order and label association (`htmlFor`/wrapping
  `<label>`) from their respective forms' existing conventions.
- **X8 — Contrast ≥ 4.5:1**: satisfied by reusing existing, already-audited
  vanilla-extract tokens verbatim — do not introduce new colors for this feature.
  Per `.claude/rules/css-architecture.md` and the token triplet already used by
  `chipStaleWork` in `web-app/src/components/backlog-stuck/StuckItem.css.ts:44-51`:
  - `vars.color.warningBg` — badge/chip background
  - `vars.color.warningText` — badge/chip text color
  - `vars.color.warning` — badge/chip 1px border
  No new token should be added to `web-app/src/styles/theme.css.ts` for this feature;
  any implementation that introduces a new hex value or a new token name for the Stale
  badge is a plan deviation and should be flagged in review.
- **X9 — Labels map is not compile-enforced (implementation checklist item)**: unlike
  `stuckReason.ts`'s `Record<StuckReason, T>` maps, `GroupingStrategyLabels` is a
  `Partial<Record<...>>` — adding `GroupingStrategy.Stale` to the enum without also
  adding its label entry compiles cleanly but renders a blank/undefined option in the
  dropdown, which is both a functional bug and an accessibility regression (a
  screen-reader user hears an unlabeled option). This must be verified by a test
  asserting `GroupingStrategyLabels[GroupingStrategy.Stale] === "Stale"` (already
  specified in plan Task 3.1.1d), not just by code review.

---

## Summary

- **5 user-facing surfaces designed**: SessionCard stale badge, "Stale"
  grouping/filter strategy, stale-session notification (toast + persistent panel),
  Settings threshold/notify config, and the approval-rule idle-minutes condition.
- **27 UX acceptance criteria written** across the five surfaces (AC1.1–AC1.5,
  AC2.1–AC2.4, AC3.1–AC3.5, AC4.1–AC4.4, AC5.1–AC5.4) plus 9 cross-cutting criteria
  (X1–X9) covering task efficiency, error/no-dead-ends, and accessibility — 36 total
  testable criteria.
- One implementation-facing UX recommendation surfaced during this pass, not yet in
  `plan.md`: label the "Stale" grouping strategy's non-stale bucket `"Not Stale"`
  rather than `"Active"` (Surface 2 edge cases) to avoid a PAUSED/STOPPED session
  appearing under a group header that implies it's actively running.
