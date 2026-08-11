# UX Design: quota-aware-backlog-gating

Source inputs: `requirements.md`, `research/ux.md`, `implementation/plan.md` Epic 3.1
(pause/resume notifications) and Epic 3.2 (`status_detail` Settings UI surface). Per
`requirements.md`'s Out of Scope and `research/ux.md`'s recommendation, this feature adds
**zero new screens**. All five surfaces below are extensions of existing pipelines/components.

## Surfaces inventory

| # | Surface | Type | Treatment |
|---|---------|------|-----------|
| a | Pause notification toast | Interactive (dismissible, read) | Full (wireframe + flow + errors) |
| b | Resume notification toast | Interactive (dismissible, read) | Full (wireframe + flow + errors) |
| c | Notification history/panel entries (pause + resume) | Interactive (bell icon, click-to-open) | Full (wireframe + flow) |
| d | `status_detail` second line, Settings → Feature Flags "backlog" row | Interactive context (read-only text inside an interactive row) | Full (wireframe + flow + errors) |
| e | Manual toggle × auto-pause interaction (the footgun) | Interactive flow spanning (a)+(d) | Full (flow + error-state table — this is the core risk this doc must close out) |
| f | Structured log lines (`QuotaGate: pausing backlog`, etc.) | Non-interactive (operator/CLI-only) | Condensed |

No config-file-only or headless-flag surface exists in this feature beyond (f) — `QuotaConfig`
is edited via `config.json` today with no CLI/UI editor (out of scope per requirements.md), so
it is treated as non-interactive alongside the logs.

---

## Surface (f): structured logs — condensed entry

Representative sample (from Observability Plan in `implementation/plan.md`):

```
INFO  QuotaGate: pausing backlog  reason=soft_threshold pct_remaining=15.3 threshold=20.0
INFO  QuotaGate: resuming backlog  reason=soft_threshold pct_remaining=38.1 threshold=20.0 margin=15.0
WARN  QuotaGate: pausing backlog  reason=hard_override last_rate_limit_event="2026-08-10T14:02:11Z"
INFO  QuotaGate: detected external change to backlog enabled state  enabled=true
INFO  QuotaGate: foreground throttle active  until="2026-08-10T14:10:00Z"
```

Acceptance criteria:
- Every gate transition (pause, resume, foreground-throttle-active, foreground-throttle-cleared,
  detected-external-change) emits exactly one structured log line with a `reason` field.
- Log lines never substitute for the notification — a human must not need `journalctl`/log
  access to learn a transition happened (notification is the primary channel; log is the
  secondary/debugging channel).
- No PII or full token contents in log fields — only aggregate counts/percentages/timestamps.
- Log line wording is stable enough to `grep` (matches the exact strings named in Observability
  Plan) so a future SRE-style incident doesn't need to reverse-engineer format changes.

---

## Surface (a)+(b): Pause / Resume notification toast

### Wireframe

```
┌──────────────────────────────────────────────────────┐
│ ⚠  Backlog Automation Paused              2s ago   ✕ │
│ ─────────────────────────────────────────────────────│
│ Backlog paused: session-quota headroom below          │
│ threshold (15% remaining, threshold 20%). Resumes      │
│ automatically once headroom recovers.                  │
└──────────────────────────────────────────────────────┘
        (top-right corner stack, per NotificationToast.tsx;
         auto-closes ~12s per notification-policy.ts WARNING tier)

┌──────────────────────────────────────────────────────┐
│ ℹ  Backlog Automation Resumed              2s ago   ✕ │
│ ─────────────────────────────────────────────────────│
│ Backlog automation resumed: session-quota headroom     │
│ recovered to ~42% (threshold 20%). Re-evaluated every   │
│ reconcile cycle.                                        │
└──────────────────────────────────────────────────────┘
        (auto-closes ~8s per notification-policy.ts INFO/STATUS_CHANGE tier)
```

Both reuse the existing `NotificationToast.tsx` component unmodified — icon/color derive from
`notificationTypeIcon`/`priorityColor` in `notificationMapping.ts` keyed off
`NotificationType.WARNING` (pause) / `STATUS_CHANGE` (resume). No new component code.

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | (none — system-initiated) | `QuotaGate.Reconcile` crosses a pause/resume condition → publishes `events.NewNotificationEvent(...)` with `itemID="backlog-quota-gate"` |
| 2 | (none) | `NotificationContext` receives the event, renders a toast in the top-right stack; entry is also persisted to notification history (surface c) |
| 3a | User does nothing | Toast auto-closes after ~12s (pause) / ~8s (resume); entry remains in history panel (durable record) |
| 3b | User clicks ✕ | Toast dismisses immediately; entry remains in history panel |
| 3c | User clicks the toast body (if `NotificationToast` treats it as focusable — no navigation target exists for this event type, since there's no dedicated quota screen) | No navigation occurs (not `isActionable()` — matches `research/ux.md` §1); dismiss behaves like ✕ |

### Error / edge-case handling

| Case | Behavior | Source |
|---|---|---|
| Repeated pause while already paused, same tick cadence | Suppressed for 5 minutes (`lastPauseNotifyAt` cooldown) — **unless** within `ManualOverrideGraceMinutes` (10 min default) of a detected manual override, in which case cooldown is bypassed and a distinguishing message is shown (see Surface (e) below) | Story 3.1.1, Task 3.1.1a |
| Event bus / notification pipeline down | Out of scope for this feature to detect — inherits whatever behavior `capacity_monitor.go`'s existing alerts have today (no new failure mode introduced; Story 3.1.3 only verifies wiring reaches the *same* pipeline, it does not add new resilience) | Story 3.1.3 |
| User has notifications muted/panel closed | Notification still lands in persistent history (surface c); `status_detail` (surface d) is the fallback durable channel if the user never opens the panel at all | `research/ux.md` §2 |
| Resume fires with no prior pause notification ever shown to this client (e.g. browser tab opened mid-pause) | Resume toast still fires and is self-explanatory ("headroom recovered to ~42%") — does not assume the pause toast was seen; `status_detail` clearing to `""` is the confirming secondary signal | Design inference — resume message stands alone, verified against Task 3.1.2a wording, which contains no forward reference to "the earlier pause" |

---

## Surface (c): Notification history/panel entries

### Wireframe

```
🔔 (3)                                    ← bell icon, badge count
  ▼
┌──────────────────────────────────────────┐
│ Notifications                        Clear│
├──────────────────────────────────────────┤
│ ⚠  Backlog Automation Paused              │
│    15% remaining, threshold 20%...  2m ago│
├──────────────────────────────────────────┤
│ ℹ  Backlog Automation Resumed             │
│    ~42% remaining...               18m ago│
├──────────────────────────────────────────┤
│ ⚠  Backlog Automation Paused              │
│    Manually re-enabled but quota   32m ago│
│    still critical — pausing again.        │
└──────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | User clicks bell icon | `NotificationPanel` opens, shows persisted history (coalesced by `sessionID:notificationType` = `"backlog-quota-gate":quota_gate_paused"` / `"...resumed"` — repeat pause events within the cooldown window do not create duplicate rows) |
| 2 | User reads entries | Each entry shows title, truncated message, relative timestamp — same rendering as every other notification type, no special-casing needed |
| 3 | User closes panel or clicks elsewhere | Panel closes; history persists for next open |

### Error / edge-case handling

- Coalescing key collision with other backlog-item notifications: not possible — `"backlog-quota-gate"` is a synthetic ID distinct from any real backlog item UUID (Domain Glossary entry), so this feature's entries never merge with per-item backlog notifications.
- History unbounded growth: out of scope for this feature — inherits whatever retention/pagination the existing `NotificationPanel` already has for all notification types (no new requirement introduced).

---

## Surface (d): `status_detail` line on Settings → Feature Flags "backlog" row

### Wireframe (current state → three variants)

```
Healthy (no pause active):
┌────────────────────────────────────────────────────┐
│ Backlog                                    [On] ●══ │
│ Automates backlog item pickup and triage.            │
└────────────────────────────────────────────────────┘
  (no second line — StatusDetail() returns "", Story 3.2.2 GWT #2)

Paused by quota (soft threshold):
┌────────────────────────────────────────────────────┐
│ Backlog                                   [Off] ══● │
│ Automates backlog item pickup and triage.             │
│ Paused: session-quota headroom below threshold        │
│ (15% remaining, threshold 20%).                        │
└────────────────────────────────────────────────────┘
  (second line renders — flagDescription style reused, Task 3.2.3b)

Throttled (foreground session active, not a hard pause):
┌────────────────────────────────────────────────────┐
│ Backlog                                    [On] ●══ │
│ Automates backlog item pickup and triage.             │
│ Throttled — foreground session active, dispatch        │
│ resumes automatically once idle.                        │
└────────────────────────────────────────────────────┘
  (toggle still shows On — throttle doesn't flip enabled state,
   per Pattern Decisions' "Foreground-throttle enforcement point" row)
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | User navigates to Settings → Feature Flags | `FeatureFlagsContext` calls `GetFeatureFlags`; response includes `statusDetail` per flag (empty for all but "backlog" when relevant) |
| 2 | (none — passive read) | `FeaturesPage` renders the second line conditionally: `{statusDetail && <div className={flagDescription}>{statusDetail}</div>}` |
| 3 | User clicks the toggle while `statusDetail` is showing a pause reason | See Surface (e) below — this is the footgun path |

### Error / edge-case handling

| Case | Behavior | Source |
|---|---|---|
| `statusDetail == ""` | No extra `<div>` rendered — no empty paragraph, no layout shift | Story 3.2.3, Task 3.2.3b |
| Flag has no wired status-detail provider (every flag except "backlog") | `statusDetailProviders[name]` lookup misses → treated as `""` | Task 3.2.2b |
| `status_detail` present but stale (e.g. `QuotaGate.Reconcile` hasn't ticked since a state change) | Not separately handled — `GetFeatureFlags` always calls `StatusDetail()` live on each fetch (no caching layer introduced), so staleness is bounded by the frontend's own poll/fetch cadence, not by this field | Task 3.2.2a reads `g.mu`-guarded live state, not a cached string |

---

## Surface (e): Manual override × auto-pause interaction — the footgun

This is the flagged risk from `research/ux.md` §3 ("user manually re-enables while quota is
still low, it silently flips back to Off") and the plan's mitigation (Task 2.1.2c, Task
2.1.2d, Task 3.1.1a, Story 2.1.3's second GWT). This section confirms the mitigation closes
the UX gap and specifies exactly what the user sees at each step.

### Flow diagram

```
[Backlog paused by quota]
   status_detail: "Paused: headroom below threshold (15%, threshold 20%)"
   toggle: Off
        │
        │  user clicks toggle → On
        ▼
[Backlog manually re-enabled]
   toggle: On                                    ← UpdateFeatureFlag calls ctrl.Enable()
   status_detail: "" (StatusDetail() checks          unconditionally; existing behavior,
                       pausedByQuota, which was        no new code (research/ux.md §3)
                       just cleared by Task 2.1.2c)
        │
        │  next Reconcile tick (≤60s later): provenance detection (Task 2.1.2c)
        │  sees IsEnabled() flipped true since last tick → sets manualOverrideAt=now,
        │  clears pausedByQuota=false. This tick does NOT re-pause yet — only the
        │  *next* threshold evaluation decides that (Task 2.1.2c's own comment).
        ▼
[Headroom re-evaluated on a subsequent tick — still below threshold]
   consecutiveBelow counter climbs toward ConsecutiveTicksToPause (2 ticks default)
        │
        │  ConsecutiveTicksToPause reached (≤2 more reconcile ticks, ~120s worst case)
        ▼
[Backlog re-paused by quota — manual override was within its 10-minute grace window]
   toggle: Off                                   ← flips back, but NOT silently:
   status_detail: "Paused: headroom below            5-min cooldown is BYPASSED
                    threshold (15%, threshold 20%)"    because time.Since(manualOverrideAt)
   TOAST FIRES: "Backlog Automation Paused —          < ManualOverrideGraceMinutes (10m)
                 Backlog was manually re-enabled       (Task 3.1.1a)
                 at 14:02 but quota is still
                 critical — pausing again."
```

### Why this closes the footgun

The original risk (`research/ux.md` §3) was: toggle flips back to Off with **no visible
explanation**, looking like the click "didn't take." The plan's mitigation makes three things
true simultaneously, all confirmed present in the tasks read above:

1. **The re-pause is not silent** — Task 3.1.1a's cooldown-bypass logic
   (`time.Since(manualOverrideAt) < ManualOverrideGraceMinutes`) means the very re-pause that
   would otherwise be swallowed by the 5-minute cooldown *always* fires a toast, worded
   differently ("manually re-enabled ... but quota is still critical — pausing again") from a
   fresh pause.
2. **`status_detail` corroborates the toast after it times out** — the second line on the
   Settings row shows the same pause reason persistently, so even a user who misses the toast
   (e.g. switched tabs) sees *why* the toggle is Off again on next glance, not just *that* it is.
3. **The window isn't instant** — because re-pause still requires `ConsecutiveTicksToPause`
   (2) consecutive bad ticks, not an immediate single-tick reversal, the toggle does not flip
   back within the same reconcile cycle the user clicked it — there is a visible "it worked for
   ~1-2 minutes" window before the explained re-pause, which further avoids the "the click did
   nothing" perception the original footgun described.

**Confirmation, not refinement needed**: the plan's design (grace-window cooldown bypass +
persistent `status_detail` + non-instant re-pause) is sufficient to resolve the footgun as
specified. One residual gap worth naming explicitly (not a blocker, since it's explicitly
out of scope per requirements.md's "no new UI dashboard"): the user has no way to see the
`ConsecutiveTicksToPause` countdown in progress — between the manual re-enable and the
re-pause, the UI shows a plain "On" toggle with no `status_detail` (headroom is still bad but
not yet re-paused), which looks identical to genuinely healthy state for that 1-2 minute
window. This is an acceptable trade per the feature's explicit "no dashboard" scope, but is
worth a one-line callout in case a future iteration wants a third status_detail variant like
"Re-enabled — re-evaluating quota, may pause again shortly" during that gap.

### Error-state table

| Failure mode | User-visible signal | Exit path |
|---|---|---|
| User re-enables, quota is actually fine now | No re-pause occurs (consecutiveBelow never reaches threshold on healthy ticks) — toggle stays On, no toast, no status_detail. Correct behavior, not an error. | N/A — success path |
| User re-enables, quota still critical, grace window active (< 10 min since re-enable) | Toast fires immediately on re-pause, wording distinguishes "manually re-enabled but quota still critical." `status_detail` updates in lockstep. | User can read the toast/status_detail and choose to wait, or investigate why headroom is low (log surface f) |
| User re-enables, quota still critical, grace window has elapsed (> 10 min since re-enable, e.g. user re-enabled then walked away for an hour before quota re-crossed the threshold again) | Cooldown reverts to normal 5-minute suppression — if this is the *first* re-pause since the override, `manualOverrideAt` is still the most recent override event, so `ManualOverrideGraceMinutes` is evaluated against elapsed time, not tick count: **after 10 minutes, a later re-pause is subject to the normal 5-min cooldown same as any pause**, i.e. it does still notify (cooldown only suppresses *repeats*, not the first occurrence) | Same as above — status_detail is still authoritative and persistent regardless of toast cooldown state |
| User repeatedly clicks toggle On/Off manually while quota is low (rapid manual flapping, not a system bug) | Each manual change is independently detected by provenance logic (Task 2.1.2c runs every tick); toast cooldown still applies to *quota-initiated* pauses, not to manual clicks — manual clicks are not gated by anything (no debounce on the toggle itself) | User's own actions are always instantaneous and never blocked — no dead end possible here since the toggle always responds to the click itself |

---

## UX acceptance criteria

### Task completion
1. User can determine backlog automation is currently paused, and why, in **1 glance** at
   Settings → Feature Flags — no navigation required (status_detail is inline on the existing
   row).
2. User can learn of a pause/resume transition **without** visiting Settings, via the toast, in
   **0 clicks** (system-pushed).
3. User can manually override a quota-driven pause in **1 click** (existing toggle,
   unmodified interaction).
4. User can see full notification history (including superseded/cleared pause-resume pairs) in
   **1 click** (bell icon).

### Error states
5. Every pause notification names both the observed value and the threshold (never a bare
   "paused") — verified by Story 3.1.1's GWT.
6. Every resume notification names the recovered value and never states a specific ETA/countdown
   — verified by Story 3.1.2's GWT.
7. A re-pause following a manual override within the grace window **always** shows a toast
   (bypasses the standard cooldown) — verified by Story 2.1.3's second GWT — closing the
   "silent flip" footgun.
8. No dead ends: every notification (toast or panel entry) is dismissible without requiring the
   user to take any corrective action; every paused state has a working exit path (the existing
   manual toggle), which remains clickable regardless of pause reason or grace-window state.
9. No layout shift: a flag with an empty `status_detail` renders identically to today's row
   (verified by Story 3.2.3's second GWT — no empty `<div>`).

### Accessibility
10. **Confirmed, not challenged**: `research/ux.md` §4's assessment holds. Read directly from
    `web-app/src/app/settings/features/page.tsx` (current code, lines ~62-72): the toggle
    `<button>` already carries `aria-label={`${enabled ? "Disable" : "Enable"} ${label}`}` and
    `aria-pressed={enabled}`. The new `status_detail` element is a plain `<div>` of descriptive
    text with no interactive affordance and no state to announce beyond its own text content —
    screen readers will read it in document order immediately after the existing description
    `<div>`, which is sufficient (it's prose, not a control). No new `aria-live` region is
    warranted: this is a page the user actively navigates to and reads, not a background status
    that must interrupt — the toast (which *does* need to announce proactively) is already
    handled by the existing `NotificationToast` component's own accessibility treatment,
    unmodified by this feature.
11. Color contrast: both new text surfaces (`status_detail` line, toast message body) reuse
    existing `flagDescription`/`NotificationToast` message styles verbatim — no new colors
    introduced, so no new contrast audit is required (inherits whatever the existing
    components already pass).
12. Keyboard navigation: no new focusable elements are introduced by this feature (the
    `status_detail` line is non-interactive; the toggle button, toast dismiss button, and bell
    icon are all pre-existing, already-keyboard-reachable controls) — nothing to verify beyond
    the existing components' current behavior.

---

## Summary for implementers

- 6 surfaces designed (5 interactive with full wireframe/flow/error treatment, 1 condensed
  non-interactive).
- 12 UX acceptance criteria written (4 task-completion, 5 error-state, 3 accessibility).
- **No missing exit path and no missing error state found** — every flow above terminates in
  either a no-op (dismiss) or the pre-existing manual toggle, which is always clickable
  regardless of gate state. The one residual gap (no visible "counting down to re-pause"
  indicator during the `ConsecutiveTicksToPause` window after a manual override) is named
  explicitly above but is **not a blocker**: it degrades to "looks briefly healthy," not to a
  dead end, a silent failure, or an un-exitable state, and is explicitly in tension with the
  feature's own "no new dashboard" scope constraint rather than an oversight.
