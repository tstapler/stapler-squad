# UX Design: slack-review-notifications

Scope: Phase 1 (outbound) is the primary design target; Phase 2 (inbound interactive buttons)
is designed only where it changes a Phase-1 surface (the settings toggle, the message's
`actions` block slot) so nothing here has to be redesigned when Phase 2 ships. Sources:
`requirements.md`, `research/ux.md`, `implementation/plan.md` (Epic 1.4, Epic 2.1).

## Surfaces designed

1. Slack Notification Settings panel (`SlackNotificationSettings.tsx`, on `/settings`)
2. Slack message — new review-queue item (`NotifyReviewQueueItem`)
3. Slack message — approval pending (`NotifyApprovalPending`, Phase 1 and Phase 2 variants)
4. Slack message — queue-depth digest (`MaybeNotifyQueueDepthThreshold`)

---

## Surface 1: Slack Notification Settings panel

### Wireframe

```
┌─ Slack Notifications ──────────────────────────────────────────────┐
│                                                                     │
│  Get pinged in Slack when an agent needs your review or approval.  │
│                                                                     │
│  Webhook URL                                                       │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │ •••• (configured)                                    [Edit]  │  │ ← masked; input is
│  └─────────────────────────────────────────────────────────────┘  │   empty by default,
│  ⓘ Paste your Slack Incoming Webhook URL                          │   "Edit" reveals a
│                                                                     │   blank input to type
│  [ Send test message ]                                             │   a replacement
│  ✓ Test message sent — check #your-channel        (role=status)   │
│                                                                     │
│  ☑ Notify on new review-queue item                                 │
│  Queue-depth digest threshold:  [  5  ]  (0 = off)                 │
│  ☐ Allow Approve/Deny from Slack (Beta — requires public           │
│    reachability; see docs)                          [disabled      │
│                                                        until Phase  │
│                                                        2 ships]     │
│                                                                     │
│  ─────────────────────────────────────────────────────────────    │
│  Last Slack delivery: 2 minutes ago — ✓ delivered   (role=status)  │
│  (or, on failure:)                                                 │
│  Last Slack delivery: 2 hours ago — ✗ failed: no_service            │
│  [ Send test message ]                              (role=alert)   │
└─────────────────────────────────────────────────────────────────────┘
```

Unconfigured (no webhook saved yet) state:

```
┌─ Slack Notifications ──────────────────────────────────────────────┐
│  Webhook URL                                                       │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │ https://hooks.slack.com/services/...                         │  │ ← empty input,
│  └─────────────────────────────────────────────────────────────┘  │   real placeholder
│  ⓘ Paste your Slack Incoming Webhook URL                          │   shape as hint
│                                                                     │
│  [ Send test message ]  (disabled — no URL yet)                    │
│                                                                     │
│  ☐ Notify on new review-queue item        (disabled, greyed)       │
│  Queue-depth digest threshold: [ 0 ] (disabled, greyed)            │
│  ☐ Allow Approve/Deny from Slack          (disabled, greyed)       │
└─────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Opens Settings, scrolls to "Slack Notifications" | `GetSlackConfig` fires on mount; panel renders in loading state briefly, then either the "configured" (masked) or "unconfigured" (empty, toggles disabled) layout above |
| 2 | Pastes a webhook URL into the input | On blur/submit, client-side regex checks the `https://hooks.slack.com/services/...` shape (per `research/ux.md` §4 row 1). Malformed → inline `role="alert"` field error, save blocked. Well-formed → error clears, "Send test message" becomes enabled |
| 3 | Clicks "Send test message" | Button enters `submitting` state (disabled, spinner/label change, mirrors `BacklogSourcesSettings.tsx:102`). Synchronous `TestSlackWebhook` RPC fires against the value currently in the form (not yet necessarily saved) |
| 4a | Test succeeds | `role="status"` region shows "Test message sent — check your Slack channel." User has now closed the trust loop before ever saving |
| 4b | Test fails | `role="alert"` region shows Slack's literal error text, e.g. "Test failed: slack returned 404: no_service" — not a generic "something went wrong" |
| 5 | Checks "Notify on new review-queue item" | Only reachable once `webhook_configured` is true (server) or a valid URL is currently entered (client) — checkbox is `disabled` otherwise, per Story 1.4.3 AC3 |
| 6 | Sets queue-depth threshold to a number | Plain number input, `0` = disabled (documented via the `(0 = off)` hint text next to the label, not a tooltip a mobile/keyboard user could miss) |
| 7 | Leaves the page and returns later | `GetSlackConfig` runs again on mount; the "Last Slack delivery" line reflects whatever the backend's `SlackDeliveryStatus` last recorded — even if that delivery happened while the settings page was closed (e.g. triggered by a real review-queue item). This is the passive trust-repair mechanism from `research/ux.md` §4 |

### Error and edge-case handling

| Case | Trigger | UX treatment | Exit path |
|---|---|---|---|
| Malformed webhook URL | Client-side shape check fails on blur/save | Inline `role="alert"` under the field: "This doesn't look like a Slack Incoming Webhook URL (expected `https://hooks.slack.com/services/...`)." Save blocked | User edits the field; error clears live as soon as the shape becomes valid — no re-submit required to see it clear |
| Test-send fails (well-formed but wrong/revoked URL, network error) | User clicks "Send test message" | `role="alert"` region adjacent to the button; body includes Slack's actual response text where available (`no_service`, `channel_not_found`, `invalid_token`) rather than a generic failure string | Button remains enabled immediately; user can fix the URL and retry with no page reload |
| Empty URL, toggle attempted | User tries to check "Notify on new review-queue item" with no webhook saved/entered | Checkbox is `disabled` — not merely validated-on-submit. There is nothing to "fail," so no error state is needed; the affordance itself communicates the precondition | N/A — nothing to escape from |
| Webhook silently revoked in Slack weeks later | Detected only server-side, at actual send time, invisible to any currently-open UI | Next time Settings is opened, the "Last Slack delivery" line reads: "Last Slack delivery: 2 hours ago — ✗ failed: `channel_not_found`" in `role="status"` (informational, not an interrupt — the user isn't being alerted live, they're being informed on next visit) with a "Send test message" button directly beneath it | User clicks "Send test message" to re-diagnose, or re-pastes a fresh webhook URL — both are already-present controls, no new UI needed |
| RPC/network failure loading the panel itself (`GetSlackConfig` fails) | Backend unreachable, auth issue, etc. | Panel renders a `role="alert"` in place of the form: "Couldn't load Slack settings." with a "Retry" action — never a silently blank/frozen panel | Retry re-fires `GetSlackConfig`; if the whole settings page has its own error boundary, this nests inside it rather than duplicating |

### Component/behavior notes (for implementation, informed by existing conventions)

- Reuse `InlineNotice` (`role="status"`, `aria-live="polite"`) for the success/passive-status cases and the existing `role="alert"` pattern (`CronScheduleInput.tsx:186-188`) for blocking errors — do not invent a third tier.
- The webhook field never re-displays the saved plaintext value (masking pattern from `BacklogSourcesSettings.tsx`'s token field) — this is a security property, not just a cosmetic one, since the URL is itself a bearer credential.
- The Phase 2 "Allow Approve/Deny from Slack" toggle is visible-but-explained in Phase 1's shipped UI (per Task 1.4.3c: "rendered but visibly marked ... requires public reachability") rather than hidden — this avoids a mystery-feature-appearing-later UX surprise and previews the tradeoff up front.

---

## Surface 2: Slack message — new review-queue item

### Wireframe (Slack Block Kit rendering)

```
┌ #your-channel ──────────────────────────────────────────────────┐
│ 🔔 stapler-squad                                          10:42  │
│                                                                   │
│ fix-login-bug needs review — tests failing                       │
│ 12 files changed, +340/−58                                       │
│                                                                   │
│ View fix-login-bug ↗                                             │
└────────────────────────────────────────────────────────────────┘
```

Truncated-diff variant (diff stats exceed the block text cap):

```
┌ #your-channel ──────────────────────────────────────────────────┐
│ 🔔 stapler-squad                                          10:42  │
│                                                                   │
│ fix-login-bug needs review — tests failing                       │
│ +line +line +line ... truncated, see dashboard                   │
│                                                                   │
│ View fix-login-bug ↗                                             │
└────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Agent session hits a review-blocking condition | `ReactiveQueueManager.OnItemAdded` fires; if `NotifyOnQueueItem` is true and the queue-depth digest didn't just fire for the same event, `NotifyReviewQueueItem` dispatches asynchronously (never blocks the queue operation) |
| 2 | Message arrives in the user's Slack (desktop or mobile) | One message, primary line = session name + reason (not prose), secondary line = compact metadata, exactly one link whose visible text is the session name (not "click here" — accessibility/scanning best practice per `research/ux.md` §1) |
| 3 | User taps/clicks the link | Opens `dashboardURL + /?session=<id>` — deep-links to the specific item, not the dashboard root, so the user lands directly on the thing they were pinged about |
| 4 | User resolves the item via the web UI | (Phase 1) No further Slack signal — the original message is not updated or annotated. This is a named, accepted gap (see Acceptance Criteria below) |

### Error and edge cases

| Case | Handling |
|---|---|
| Diff/context content exceeds Slack's ~3000-char block limit | Truncated at `maxSlackBlockTextLen` (2900 runes) with a `"... truncated, see dashboard"` suffix — the user always has a way to see the full content (the link), truncation never silently drops information without saying so |
| `dashboardURL` unset (no `DashboardBaseURL` configured, no reverse proxy) | Link falls back to `http://<listen-address>` — works only on the same LAN/machine. This is a named, accepted gap (`plan.md`'s "Unresolved Questions": whether to hard-require `DashboardBaseURL`) — from mobile off-LAN, the link will not resolve. **UX risk**: a Slack message whose only CTA is a dead link on mobile defeats the "get unblocked from my phone" job-to-be-done named in `research/ux.md` §5. See Acceptance Criteria UX-9 below. |
| Slack webhook itself down/rate-limited at send time | Invisible to this surface by design (fire-and-forget) — surfaced instead on the Settings panel's "Last delivery" line, not as a retry or error message the user sees in Slack (there is nothing to show in a channel that never received a message) |
| Two review-queue items arrive within the same second | Each gets its own message (Phase 1 does not batch per-item messages — only the digest threshold batches) — acceptable at single-user human-latency volume per requirements' appetite, but is the mechanism that can trip Slack's ~1msg/sec limit; see queue-depth digest below for the intended relief valve |

---

## Surface 3: Slack message — approval pending

### Wireframe — Phase 1 (no buttons)

```
┌ #your-channel ──────────────────────────────────────────────────┐
│ 🔔 stapler-squad                                          10:43  │
│                                                                   │
│ fix-login-bug wants to run Bash                                  │
│ rm -rf ./node_modules && npm install                             │
│                                                                   │
│ Review in dashboard ↗                                            │
└────────────────────────────────────────────────────────────────┘
```

### Wireframe — Phase 2 (`ApprovalEnabled: true`, buttons present)

```
┌ #your-channel ──────────────────────────────────────────────────┐
│ 🔔 stapler-squad                                          10:43  │
│                                                                   │
│ fix-login-bug wants to run Bash                                  │
│ rm -rf ./node_modules && npm install                             │
│                                                                   │
│ [ ✅ Approve ]   [ ❌ Deny ]        Review in dashboard ↗         │
└────────────────────────────────────────────────────────────────┘
```

After a click (Phase 2 — button replaced with a static outcome per the CircleCI/GitHub-Actions
pattern named in `research/ux.md` §1, preventing a second click from double-applying):

```
┌ #your-channel ──────────────────────────────────────────────────┐
│ 🔔 stapler-squad                                          10:43  │
│                                                                   │
│ fix-login-bug wants to run Bash                                  │
│ rm -rf ./node_modules && npm install                             │
│                                                                   │
│ ✅ Approved by you at 10:44          Review in dashboard ↗       │
└────────────────────────────────────────────────────────────────┘
```

### Interaction flow (Phase 1)

| Step | User action | System response |
|---|---|---|
| 1 | Agent session hits a permission-request gate | `ApprovalHandler.broadcastApprovalNotification` fires; `NotifyApprovalPending` dispatches asynchronously |
| 2 | Message arrives | Primary line: session name + tool name; secondary line: the specific command/action being requested (truncated per the same 2900-rune cap as Surface 2); one link to `/?session=<id>` |
| 3 | User taps the link | Deep-links to the session's approval UI in the dashboard, where Approve/Deny actually happens (Phase 1 has no in-Slack action) |

### Interaction flow (Phase 2, additive)

| Step | User action | System response |
|---|---|---|
| 4 | User taps "Approve" or "Deny" directly in Slack | Slack POSTs the interactive payload to `/api/hooks/slack-interactive`; server verifies the Slack signature (hard gate, rejects with a generic 401 on failure — no internal detail leaked, per plan's Story 2.1.2) then resolves the approval in-process |
| 5 | Someone else (or the user via the web UI) already resolved it first | Per the CI-bot idempotency lesson in `research/ux.md` §1: the second click must not double-apply the action or silently no-op with no signal. **This is a named gap in the current plan** — `implementation/plan.md`'s Epic 2.1 does not yet specify message-update-in-place (`chat.update`) or an idempotency check server-side. See Acceptance Criteria UX-10 below; flagging for planning before Phase 2 implementation, not blocking Phase 1 |

### Error and edge cases

| Case | Handling |
|---|---|
| Command/tool argument text is very long | Same truncation treatment as Surface 2's diff stats |
| Phase 2: signature verification fails | Request rejected with a generic 401; nothing in Slack changes (button stays clickable, appears to silently do nothing to the clicker) — **this is a genuine dead end from the end user's perspective if the signing secret drifts out of sync**, since Slack does not surface the 401 body to the clicking user, only to server logs. Documented as an accepted Phase-2 gap requiring the Settings panel's delivery-status-style surfacing to extend to inbound-verification failures too, not just outbound send failures — flag for Phase 2 planning |
| Phase 2: approval already resolved by the web UI before the Slack click | Named gap — see Interaction flow step 5 above |

---

## Surface 4: Slack message — queue-depth digest

### Wireframe

```
┌ #your-channel ──────────────────────────────────────────────────┐
│ 🔔 stapler-squad                                          10:50  │
│                                                                   │
│ 6 items pending in the review queue                              │
│                                                                   │
│ View review queue ↗                                              │
└────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Queue depth crosses the configured threshold (edge-triggered — fires once per crossing) | A single digest message fires instead of N per-item messages, and (per the plan's Unresolved-Questions default) suppresses the per-item message for that same triggering event |
| 2 | User taps the link | Goes to the review-queue list view (`/review-queue`), not a specific item — this message is intentionally a *summary*, so its link target is the list, matching the GitHub-app "3 new PRs opened → view list" pattern named in `research/ux.md` §1 |
| 3 | Queue depth drops back below threshold, then re-crosses later | Fires again — the latch resets, so the user isn't permanently silenced after the first burst |

### Error and edge cases

| Case | Handling |
|---|---|
| Depth stays above threshold for a long time (no re-crossing) | No repeat digest fires — this is by design (edge-triggered, not level-triggered) but is a **UX tradeoff worth naming explicitly**: a user who ignores the first digest gets no further nudge while N items pile up. Acceptable at this appetite per requirements, but should be called out in the Settings panel's hint text (e.g. "You'll get one digest per burst — dismissed items reset the counter") so the behavior isn't discovered by surprise. See Acceptance Criteria UX-11 |
| Both a threshold-crossing and a per-item trigger would fire on the same event | Digest wins, per-item is suppressed (plan's stated default) — from the user's perspective this means the *first* item in a burst that crosses the threshold gets folded into "6 items pending" rather than getting its own "fix-login-bug needs review" message. Slightly less specific, but avoids a redundant pair of messages in the same second |

---

## UX Acceptance Criteria

### Task efficiency

- **UX-1**: A user with a Slack Incoming Webhook URL already copied to their clipboard can configure and verify Slack notifications (paste URL → send test message → see success) in **≤ 3 actions** (paste, click test, done) with **zero page reloads**.
- **UX-2**: A user can identify, from the Settings panel alone and without needing to send a fresh test, whether their existing configuration is currently healthy — i.e. "Last Slack delivery" is visible on every page load once at least one real delivery attempt has occurred, requiring **0 additional clicks**.
- **UX-3**: From receiving a Slack notification to landing on the specific queue item in the dashboard is **exactly 1 tap** (the message link), on both mobile and desktop Slack clients.

### Error states

- **UX-4**: An invalid webhook URL shows a specific, actionable inline error ("This doesn't look like a Slack Incoming Webhook URL (expected `https://hooks.slack.com/services/...`)") — never a generic "invalid input" message — and blocks save until corrected.
- **UX-5**: A failed test-send surfaces Slack's own error text (e.g. `no_service`, `channel_not_found`, `invalid_token`) inline next to the "Send test message" button, not a generic "test failed."
- **UX-6**: A webhook that silently stopped working between settings-page visits is surfaced passively (no action required to discover it) the next time Settings is opened, via the "Last Slack delivery" status line, including Slack's error text.
- **UX-7**: **No dead ends** — every error state above (invalid URL, failed test, stale/broken webhook) has a same-screen, immediately-available exit path (edit the field, click test again) with no page reload, no navigation away, and no state where the only recovery is "figure it out yourself."
- **UX-8**: A toggle that requires a precondition not yet met (webhook configured) is `disabled` rather than clickable-then-erroring — error prevention over error messaging, per Nielsen heuristic 5.

### Named gaps (flagged, not blocking Phase 1 ship, but must be either resolved or explicitly accepted before Phase 1 ships)

- **UX-9** (RESOLVED — `implementation/plan.md` Task 1.4.3f): If `DashboardBaseURL` is left unset, the dashboard link in every Slack message falls back to a LAN-only address — defeating the "act from my phone off-LAN" job-to-be-done. Resolved as option (b): the Settings panel exposes a visible `dashboard_base_url` field (Task 1.4.3f) plus a persistent, dismissable `role="status"` warning shown when a notify toggle is on but the field is empty: "Your Slack links may not work outside your home network — set a Dashboard URL below." Option (a) (hard-require the field before enabling toggles) was considered and rejected in favor of the soft warning, to avoid blocking the Medium-appetite ship on a product decision with no wrong answer either way — see `implementation/plan.md`'s Unresolved Questions, item 1, marked resolved.
- **UX-10**: Phase 2's Approve/Deny buttons need an explicit double-click/already-resolved UX before shipping — either disabling/replacing the buttons in place (requires storing the Slack message timestamp, `research/ux.md`'s PagerDuty-pattern note) or, at minimum, having the second click's response text explicitly say "already resolved" rather than silently no-op-ing or erroring. Not required for Phase 1; flagged so Phase 2 planning doesn't ship the buttons without deciding this.
- **UX-11**: The edge-triggered (not level-triggered) digest behavior should be stated in the Settings panel's hint text next to the threshold input, so a user who sees one digest and then nothing further isn't surprised that a persistently-full queue goes silent.

### Accessibility

- **UX-12**: Every settings input has an explicit `<label htmlFor>` bound to the input's `id` — no placeholder-as-label, no wrapping-label-only pattern (matches existing `PushNotificationSettings.tsx`/`CronScheduleInput.tsx` convention).
- **UX-13**: The webhook URL field uses `aria-invalid` + `aria-describedby` pointing at both a persistent hint region and the conditional error region's `id`, exactly matching `CronScheduleInput.tsx:170-189`'s template.
- **UX-14**: Blocking errors (invalid URL, failed test) use `role="alert"`/implicit `aria-live="assertive"`; non-blocking informational state (test succeeded, last-delivery status) uses `role="status"`/`aria-live="polite"` — the two-tier convention already established by `InlineNotice.tsx` and used consistently, never conflated into one generic banner type.
- **UX-15**: The two notify-toggles use native `<input type="checkbox">` with a bound `<label>` — no hand-rolled `<div onClick>` switch (per `PushNotificationSettings.tsx`'s existing pattern and this repo's a11y precedent).
- **UX-16**: Every interactive element (URL input, checkboxes, threshold number input, "Send test message" button, "Edit" link) is reachable and operable via keyboard alone (Tab order follows visual order, Enter/Space activates buttons and checkboxes) — verified by tabbing through the panel without a mouse.
- **UX-17**: All text in the panel (labels, hints, status lines, error messages) meets **WCAG AA color contrast (≥ 4.5:1)** against its background, using only tokens already defined in `web-app/src/app/globals.css` (`--text-primary`, `--error-text`, `--success`, etc.) or the `vanilla-extract` theme contract (`web-app/src/styles/theme.css.ts`) — **no hardcoded hex/rgb values**, per this repo's `.claude/rules/css-architecture.md`. New styling for this component must live in `SlackNotificationSettings.css.ts` using `vars.*` token references, not a new `.module.css` file.
- **UX-18**: The "Send test message" button's pending/submitting state is conveyed both visually (disabled + label change, not just a spinner) and to assistive tech (the result region's `role="status"`/`role="alert"` update fires only once the request completes, not on click, so a screen reader doesn't announce a stale or premature result).
- **UX-19**: In the Slack message itself, the link's visible text is the session/item name (e.g. "View fix-login-bug"), never bare "click here" or a raw URL — screen-reader users navigating by link list must be able to distinguish multiple stapler-squad notifications in their channel history from the link text alone.

---

## Summary

4 user-facing surfaces designed (Settings panel; review-queue-item Slack message; approval-pending Slack message, Phase 1 + Phase 2 variants; queue-depth-digest Slack message), each with a wireframe, an interaction flow table, and an error/edge-case table.

19 UX acceptance criteria written (UX-1 through UX-19), spanning task efficiency (3), error-state handling (5), named/flagged gaps requiring a product decision before or during Phase 2 (3), and accessibility (8, including the repo's vanilla-extract/design-token constraint).
