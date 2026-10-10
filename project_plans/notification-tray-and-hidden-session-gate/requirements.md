# Requirements: notification-tray-and-hidden-session-gate

**Date**: 2026-10-07
**Type**: feature addition (backend delivery gating + frontend notification UX)
**Complexity**: 3 — multiple epics (backend gate, hidden-session access, toast stack, tray), mobile + desktop
**Operator decisions O1-O8**: status is recorded in `implementation/plan.md` "Operator decisions" (all DECIDED; O7 confirmed and O8, the extended appetite with the full scope kept, decided on 2026-10-09). Any change to a decision happens there first.

Follow-up to merged PR #738 (`project_plans/notification-revamp/`). That work fixed
signal quality (dedup, idle queue, rule reconciliation, grouped Notifications page);
it did not address either problem below. Branch from `origin/main`.

## Problem Statement

1. **Background (hidden) sessions still notify the operator, and the notification is a dead end.**
   `review:*`, triage and diagnose sessions run with `Hidden: true`. Their completion
   fires a "Session Completed" web push (screenshot: `Session 'review:ee1b4be0:18dc7…'`)
   from `server/push/subscriber.go` `buildStatusChangeNotification`, which has no
   Hidden check (`grep -i hidden server/push` → no matches on `origin/main` 013856269).
   PRs #227/#854 gated only the hook-notification path in
   `NotificationService.SendNotification`; #864 added a status-transition check but no
   Hidden check. "View Session" then fails because `ListSessions` drops hidden sessions
   unless `IncludeHidden` is set (`server/services/session_service_crud.go:61`) and the
   web UI never sets it (`web-app/src/app/page.tsx:215`).
2. **Toasts can't be managed in bulk.** Toasts stack bottom-right with no cap or
   grouping (`web-app/src/lib/contexts/NotificationContext.tsx` ~L480-500), covering the
   terminal on mobile (both screenshots). `clearAll()` exists in the context but no UI
   calls it. The existing `NotificationPanel` sidebar (header bell) is a history list;
   toasts don't collapse into it and it is hard to reach on mobile.

## Baseline

- Routine completions from hidden sessions reach the phone as pushes and the tray/history,
  with an unopenable "View Session".
- N simultaneous toasts occupy the bottom of the screen with only per-toast dismiss.
- Operator must leave the current view (Notifications page) to triage, reloading terminals.
- (Resolved in Phase 4, see Open Question 2: a visible work session, not a hidden leak.) Originally unverified: origin of the "Claude Notification … via tmux"
  toast for `staplersquad_stapler-squad-background-llm-model-pinning-r2` — may be a
  visible work session, a hook path, or an ungated channel.

## Users / Consumers

Single operator (Tyler), web UI on desktop and mobile (Chrome on Android over Tailscale/LAN,
see screenshots), plus web push. Both form factors are required (`feedback_mobile_desktop_ux`).

## Success Metrics

- A hidden-session routine completion produces **zero** push, toast, or history row on
  every delivery channel, **in steady state** (Baseline: ≥1 push per review session.)
  Stated bound (plan "Accepted residual leak", ADR-002; Phase 3 iteration 3): zero for
  sessions created or restored by the running server; non-zero only through enumerated,
  individually counted stragglers (events after a restart for a hidden session deleted
  before the restart, the first event of a session created by a path the index feed
  missed, a renamed session before its rename is fed). Fail-open on an unresolvable
  session is deliberate (a lost failure is worse than a stray routine event); each
  occurrence increments `notification_delivery_unresolved_total{class}`.
- A hidden-session failure or needs-human event still notifies, and its "View Session"
  opens the session (read-only) 100% of the time. (Baseline: 0%.) A hidden session that
  crashes or permanently fails is a failure (a Crashed transition gets a FAILURE
  notification; `PermanentlyFailed` already does). A pending "Claude has a question"
  can be answered from the read-only view through one audited Reply (see Scope 2).
  - **Phase 3 iteration 3 note**: the Reply is one of two explicitly allowlisted UI writes
    into a hidden session (operator decision O2; plan Story 5.6; ADR-010).
- With N≥10 simultaneous notifications, at most 3 toasts are visible, with a "+N more"
  affordance and one one-tap bulk control, "Move all to tray", that clears the deck without
  deleting anything (triad iteration 1: the earlier "Dismiss all" was a near-dead path because
  routine types never toast and nearly every toast is pinned). (Baseline: unbounded, no bulk control.)
- Opening/closing the tray never reloads the page or remounts terminal components
  (verified by e2e: terminal DOM node identity and scrollback preserved).
- No bulk action clears an item still needing a decision (carried over from #738).
- Toasts never cover the terminal input area or soft keyboard on mobile.

### Outcome measurement (triad iteration 1: the metrics above are acceptance criteria; these are the outcomes they must move)

Baseline is taken **before PR 2a-1 ships** and re-measured **after the soak** with the same procedure (plan Story 1.5 and Task 2.9e). Source: the live notification history file (7-day retention, `server/notifications/store.go:19`, so one window is the whole store), the sessions DB and, after PR 2a-2, `GetDeliveryGateStats`. Method limit: pushes are not recorded in history, so the push figure is INFERRED (history rows that pass `shouldNotify` plus hidden Stopped transitions found in the sessions DB; UNVERIFIED method).

| Outcome (trailing 7 days) | Baseline | After soak | Target |
|---|---|---|---|
| Hidden-session history rows (all types) | NOT YET MEASURED (Story 1.5) | not yet | only failure/needs-human types, plus counted stragglers |
| Hidden-session toast-eligible rows | NOT YET MEASURED (Story 1.5) | not yet | 0 routine |
| Hidden-session pushes (INFERRED) | NOT YET MEASURED (Story 1.5) | not yet | 0 routine |
| Hidden-session failure and needs-human events (the dead-end exposure) | NOT YET MEASURED (Story 1.5) | not yet | unchanged volume, each one opens read-only in <= 1 tap |
| Largest burst of rows inside any 60 seconds (proxy for simultaneous toasts) | NOT YET MEASURED (Story 1.5) | not yet | deck still capped at 3 (1 on a phone) whatever the burst |
| Pinned toasts per operator-active hour (alert-fatigue check; an auto-remediating WARNING must not count; an "active hour" is an hour with at least one history row, INFERRED proxy) | NOT YET MEASURED (Story 1.5) | not yet | **median <= 2 and busiest hour <= 6** (planner default, DECISION; the figures are a judgment about how many pending decisions one operator can take per hour, not a measured threshold, UNVERIFIED). Re-derived once the baseline exists: if the baseline median is below 4, the target becomes half the baseline median, so the number can only get stricter |
| **Interruptions per day** (operator-experience; every history row that toasts plus the INFERRED push estimate, hidden and visible sessions, trailing 7 days) | NOT YET MEASURED (Story 1.5) | not yet | falls by at least the hidden-session routine share the baseline reports (the removable part); the remainder (visible sessions, hidden failure and needs-human) is recorded, not targeted |
| **Taps to triage a burst** (scripted walkthrough on a manual instance: a seeded burst of 10 notifications, 6 informational, 3 pending approvals, 1 failure; taps counted by the script from the burst arriving to an empty deck with every pending decision acted on or left pinned in the tray) and **time to triage** (the operator, with a stopwatch on a phone, median of 3 runs; INFERRED, manual) | NOT YET MEASURED (Story 1.5, run on the unchanged `origin/main` build: dismiss each toast, then the Notifications page) | not yet | taps <= 2 + 1 per pending decision (derived from TB-8: one tap to an empty deck, one to open the tray, one per decision); time recorded, and once a baseline exists the target is half of it (planner default, DECISION) |
| **Taps to reach a failing hidden session** (scripted: a hidden-session failure notification, taps from the notification to the read-only output view, from the toast and from the tray) | not reachable on `origin/main` (Baseline; Spike 1.1 confirms and records the four repros) | not yet | <= 1 tap from the toast or push (TC-6), <= 2 from the closed tray (open tray, tap "View output") |

The numbers go in this table when Story 1.5 runs; until then the cells say so rather than guess. **Measurement owner**: plan Story 1.5 (Tasks 1.5a and 1.5b) takes the baseline for every row above, including the scripted tap counts and the operator-timed runs on the unchanged build; Task 2.9e re-runs the history rows after the soak (R1) and the committed walkthrough spec of Task 3.10b re-runs the scripted taps after R2 and again after R4; the operator-timed runs repeat on the same phone after the device checks DV-1 to DV-9. A figure that cannot be measured by the stated method stays labelled INFERRED or NOT YET MEASURED.

### Riskiest product hypothesis (stated so it can be checked, not assumed)

**H1: capping toasts (3 on desktop, 1 on a phone) and moving the overflow into a tray fixes the operator's burst fatigue beyond what the hidden-session gate already removes.** It is the riskiest assumption because the gate alone may remove most bursts (the screenshots show hidden-session routine pushes and toasts; the burst rate from visible sessions and hidden failures has not been measured), and because a tray is a second place to look that can cost more attention than it saves.

**The check, pre-registered before any toast or tray code is written.** Story 1.5 computes from the 7-day history, as part of the baseline, the **residual bursts**: the number of 60-second windows holding at least 4 toast-eligible rows after every hidden-session routine row is removed (the counterfactual "gate only"), per week, plus the largest burst and the pinned-per-hour median. Task 2.9e re-computes the same figures from the real soak window, where the gate really is on. H1 is judged on those residual figures, not on the raw baseline, because the raw baseline still contains the hidden-session flood the gate removes.

### Post-soak decision rule (what the result changes)

Applied once, from the Task 2.9e figures, before Epic 3 merges; the operator records which row applied in the PR 3 description. O8 stands: **this rule cuts no scope and withholds no story**; it changes parameters and ordering, and it names the one thing that stops.

| Post-soak result | What it changes |
|---|---|
| Residual bursts >= 3 per week, or a 60-second burst of >= 4 on at least 2 days | H1 supported. Epics 3 and 4 proceed as planned; SM-3 keeps its "fixes burst fatigue" wording. |
| Residual bursts 1 to 2 per week | H1 partly supported. Epics 3 and 4 proceed as planned; the cap values (3 and 1) and `PINNED_COLLAPSE_MS` are the tunable parameters, set from DV-1 and the operator's use rather than changed in advance. |
| Residual bursts 0 (the gate alone removed the bursts) | H1 not supported by data. The toast stack and tray still ship (O8), but SM-3 is reworded from "fixes burst fatigue" to "bounds the worst case" and **no further toast-stack or tray features are added beyond the planned stories** on the strength of burst fatigue (the one thing that stops: additions, not planned work). |
| Pinned toasts per active hour above the target (median > 2 or busiest hour > 6) in any row above | Before R2 ships, the pinned set is reviewed: which warning and error producers are pinned and whether more should be stamped `auto_remediating` (plan Task 3.2d); the cap and tray are unaffected. |
| After R2 or R4, taps to triage the seeded burst are at or above the baseline's (the tray costs more than it saves) | The late-sequenced Epic 4 items (Quiet mode, hotkey, `top-sheet`, `landscape-panel`, Pin tray) pause until the triage path meets the taps target; they are resumed, not cut. |

### Roadmap and cost of delay

Ordered by what each day of delay costs, then by what can start now (plan "Sequencing and PR order"). No human-time figure is given or implied.
- **R1 (gate, first).** Each day without it costs the baseline's hidden-session interruptions per day (NOT YET MEASURED; Story 1.5 turns this into a number). It needs no phone, has the smallest blast radius behind a flag, and the soak figures feed the decision rule above, so it also buys the evidence for R2.
- **R2 (toast stack).** Costs delay only when a burst happens: the residual bursts per week of H1's check, times the taps per burst. Startable now after the contract PR; the phone placement fixes the covered input line in the screenshots on its own.
- **R3 (hidden-session reachability).** Rare events with high severity: a hidden failure is currently a notification that cannot be opened (0%). It is third because it is blocked (Spike 1.1, the re-review PASS, and Spike 1.3g for Reply), not because it matters least; the Story 1.5 count of hidden failure and needs-human events checks this order against data before R2 starts.
- **R4 (tray and late-sequenced items).** The convenience layer; lowest cost of delay, and Reply is last within Epic 5 because its benefit (answering a hidden question from the phone) is the smallest of the group and its risk is the largest.

## Appetite

**Extended: full scope, no fixed time box** (operator decision O8, 2026-10-09). The earlier fixed "Large" time box and its cut order are withdrawn: the operator is comfortable with a large project and asked for the full scope, including swipe-to-dismiss, per-session grouping, a Background activity view, the per-kind flag override, the audited Reply, the prune RPC, and the former Stretch items (Quiet mode, the tray hotkey, the `top-sheet` and `landscape-panel` tray variants, the push service-worker handoff), sequenced last within their epic. There is no cut list and no appetite question; scope items 2, 4 and 7 are fully in scope with no amendment pending.

**Size and effort** are tracked in `implementation/plan.md` "Effort" as price-weighted cost classes, agent waves and named wall-clock blockers (method: `~/.claude/skills/sdd/skills/ESTIMATION.md`), not as human time. The partial-ship points R1-R4 in the plan's "Sequencing" are coherent merge and release boundaries, not cut lines.

## Constraints

- Hidden-session policy (decided): only **failures** and **needs-human/decision** events
  notify; routine completions never do. Applies uniformly to all delivery channels.
- Bulk actions must never silently clear needs-a-decision items (#738 requirement,
  `feedback_document_ai_decisions_in_edge_cases`).
- Rollout flags must be live-settable via the feature-flag RPC/panel, not env vars
  (`feedback_rollout_flags_live_settable_no_env_vars`).
- Frontend: pnpm only in `web-app/`; vanilla-extract styling; jscpd gate (threshold 0.14 in `web-app/.jscpd.json:5`, VERIFIED; the repo `CLAUDE.md` still says 0.1%, a doc-drift collateral-debt item owned by plan Task 3.10f, not a requirement of this project).
- E2E conventions (`e2e-test-conventions`): feature header, no `waitForTimeout`,
  `data-testid`/ARIA locators.
- Go: read `*Instance` fields via `Snapshot()`, not raw fields
  (`.claude/rules/instance-lock-free-reads.md`).
- Do not use `make install-service` to try changes on the live instance; use the manual
  instance recipe in CLAUDE.md (ports 62871+, private tmux socket).

## Non-functional Requirements

- **Performance SLO**: not specified in milliseconds; tray open must not block terminal
  rendering. Measurable form (Phase 4, validation G-6): opening the tray issues **no
  synchronous full-history fetch** (0 `GetNotificationHistory` calls beyond the hydrated
  slice; the list is virtualized or paginated) and **adds no long task (> 50 ms) attributable
  to the open** in the Playwright performance trace, with the terminal root not remounted
  and `cols x rows` unchanged. No numeric latency target is invented beyond these.
- **Scalability**: hundreds of history rows; tray list virtualized or paginated.
- **Accessibility**: tray is a focus-managed dialog/landmark; WCAG AA (Axe CI gate);
  touch targets ≥44px; honors safe-area insets and open soft keyboard.
- **Security classification**: internal.
- **Data residency**: no special requirements.

## Scope

### In Scope

1. **Single hidden-session delivery gate** applied once in front of every channel
   (web push, toast event, history store, Slack/other notifiers that share the
   EventBus), replacing per-path checks. Policy: hidden → only failure / needs-human.
   Audit every `EventNotification` / `EventSessionUpdated` producer for ungated paths.
2. **Hidden-session reachability**: a notification that does fire for a hidden session
   opens it (read-only terminal/output view, no terminal input) from the toast, tray, push
   click. **One exception, by operator decision O2**: while a "Claude has a question" is
   pending **and on screen**, an audited, rate-limited, single-line Reply control answers
   that one question once (a newer question, or any other dialog-producing hook such as a
   permission request, supersedes the older one; a send that fails
   after the first byte is reported as indeterminate and is never retried; plan Story 5.6,
   ADR-010). **Reply stays in scope by operator decision, sequenced last in Epic 5 and hard-gated on Spike 1.3g and a passing re-review of the Reply design; it ships behind a kill switch, **works only for sessions started after its hook proof ships (an accepted limit, default applied in review-repair iteration 4)** and its residual risk is stated plainly in plan Risk Control and ADR-010: a typed answer can land in the wrong dialog, worst case approving a permission dialog, and the kill switch cannot undo a keystroke already sent.** **A second narrow exception keeps the shipped backlog "Steer"
   composer working on hidden review sessions linked to a live backlog item** (operator
   decision O7, confirmed by the operator on 2026-10-09: plan Story 5.2, ADR-005). Everything
   else stays blocked by typed capabilities, an RPC-descriptor classification and a short
   pinned-caller table of the pane-typing primitives, with two UI write paths (plus one
   token-gated steer writer that a handler may call only with a token built by the
   access-decision block, and the internal non-RPC writers pinned as `acquirer` rows);
   review-repair iteration 5 replaced the earlier whole-tree scan with these checks, and
   what they do not catch is stated in ADR-005 decision 3. The guard is a safety rail, not a security boundary. Plus a "Background activity" view listing hidden sessions and their recent state.
3. **Toast stack**: cap visible toasts (~3) with "+N more" chip opening the tray;
   one bulk control, "Move all to tray" (the existing `clearAll()` stays pinned-safe for the flag-off
   legacy list); collapse/minimize; routine toasts
   demote straight to the tray; approval/decision toasts stay pinned; mobile positioning
   clears bottom nav, pane tab strip, soft keyboard.
4. **Side tray**: evolve `NotificationPanel` into the toast home — persistent edge handle
   with unread count, overlay (no navigation/remount), bulk actions ("Mark activity read",
   the shipped label, which excludes unread decisions; clear informational; collapse per session), swipe-to-dismiss on touch, grouping by
   session, "Background activity" section.
5. Investigate and resolve the "via tmux" Claude Notification toast origin.
   **Outcome (Phase 4): resolved, no code change.** It is a legitimate hook notification from
   a visible Backlog work session (`source_app=tmux` from `scripts/ssq-hook-handler:231-235`,
   rendered as `via tmux`); that session has `hidden=0`. See Open Question 2, plan
   "Closed during plan repair" and Task 4.5c (docs note).
6. Docs (`docs/` Diataxis), feature registry (`make registry-generate`), e2e specs.
7. **Operator-triggered history prune** (plan Story 2.7, `PruneHiddenSessionNotifications`):
   a dry-run-first admin RPC that removes already-stored hidden-session routine rows without
   ever deleting an unread pending decision. It replaced the earlier restart-time one-shot
   cleanup and is the plan's answer to the Baseline's existing stale rows (existing rows also
   age out under the 7-day retention). In scope; it ships after the gate (plan PR 2c).
8. **Items beyond the original requirements text, kept in scope by operator decision O8** (appetite extended, full scope): Quiet mode (plan Story 4.5), the tray hotkey (Task 4.2f; the chord is spiked first and the hotkey is not shipped if no safe chord exists, which is a finding), the `top-sheet` and `landscape-panel` tray variants (Task 4.2e) and the push service-worker handoff (Story 5.5). They are sequenced last within their epic and keep their dependencies and gating spikes. None is required by Success Metrics or Scope 1-7, but none is cuttable. Triad repair 1 adds, in the same spirit and also in scope: the opt-in **Pin tray** (dock) mode that removes the overlay's hidden-columns cost for a user who wants it (Task 4.2e, `design/ux.md` D12), the one-time "What changed" card (TM-13), the phone approval card layout (D11) and the single stable phone tray entry (D10).

### Out of Scope

- Notification dedup, review-queue idle logic, rule reconciliation (shipped in #738).
- Native OS notification auto-dismiss / health-alert dedup (`smart-notification-dedup`).
- Slack/Jules channel redesign beyond applying the shared gate.
- Full activity-feed product (saved filters, advanced search) — keep existing search/type filter.
- Changing what hidden sessions *do*, only what they surface.

## Rabbit Holes

- "Make hidden sessions reachable" can balloon into un-hiding them everywhere
  (session list, search, board). Cap at a read-only view + Background activity section.
- Gate placement: per-channel checks regrow the bug class. Plan must pick one choke point
  and prove no channel bypasses it (EventBus subscriber vs. producer-side).
- Swipe gestures vs. existing pane-swipe/terminal touch handling on mobile — conflict risk.
- Tray overlay vs. terminal focus/keyboard: stealing focus from xterm or resizing the
  terminal (resize votes were recently reworked, #728/#731) could cause reflow/redraw.
- Toast and tray state split across `NotificationContext` (client) and server history;
  keep one source of truth for read/dismissed state across tabs (cross-tab sync exists).

## Alternatives Considered

- Per-channel Hidden checks (status quo pattern): rejected — already regressed once;
  the push path was simply missed.
- New standalone tray component: rejected — `NotificationPanel` already exists, has
  history, search, type filter, auto-handled section; evolve it.
- Un-hide hidden sessions in the main list: rejected — floods the list the hiding exists
  to keep clean.
- Pure CSS toast shrink: rejected — doesn't give bulk control.

## Feasibility Risks

- `event.Session` in the push subscriber may be a stale/partial snapshot; Hidden must be
  read via `Snapshot()`; hidden sessions deleted before delivery can't be resolved.
- Read-only hidden-session view needs a server-side guard so `WriteToSession`-style paths
  can't be reached from it (mirror the MCP-handler gate lesson,
  `instinct_mcp_dispatched_agent_tool_surface`).
- Mobile soft-keyboard viewport handling already exists (`--viewport-height`); tray must
  reuse it.

## Observability Requirements

- Structured log (slog) + counter whenever the gate suppresses a delivery: channel,
  session id, event type, reason. Suppressed counts visible in logs for after-the-fact
  verification that nothing needed was dropped.
- Log (not suppress) any hidden-session failure delivery with its channel.
- Client: no new telemetry beyond existing audit log hooks (`useAuditLog`).

## Risk Control

- Gate behind a live-settable feature flag (global + per-scope override via the
  feature-flag panel) so the previous behavior can be restored without a deploy;
  default on after verification. **Built as specified (operator decision O1)**: the scope
  is the hidden-session kind (`review`, `triage`, `diagnose`, `other`), precedence kind
  override > explicit global > default (plan Story 2.11, ADR-004). No deviation is
  recorded. Caveat: an explicit persisted `false` survives the default flip and is
  surfaced with a startup WARN and the flag's status line, not overridden.
- Tray/toast changes behind a second live-settable flag for the new stack/tray UI
  during rollout; old toast list remains the fallback.
- Rollback: flip flags; code revert is clean (no schema/migration; notification history
  is JSON-backed).

## Open Questions

1. Which producers emit notifications outside `NotificationService.SendNotification`
   and the push subscriber (hook handler, review queue, stale notifier, fork-pressure,
   backlog)? Full channel inventory needed before choosing the gate's choke point.
2. What is the "Claude Notification … via tmux" toast source for the
   `background-llm-model-pinning-r2` session, and is that session Hidden?
   **RESOLVED (Phase 4, VERIFIED by the research agent; see `research/features.md` section
   2 and `research/pitfalls.md` section 0).** The toast is the hook default title "Claude
   Notification" with subtitle `<project> via tmux`, where `source_app=tmux` is set by
   `scripts/ssq-hook-handler:231-235` for any stapler-squad-managed tmux session and
   rendered by `NotificationToast.tsx:158` / `NotificationItem.tsx:83`. The session has
   `hidden=0` (the "background" in its name is the backlog slug), so it is **not** a
   hidden-session leak and not an ungated channel; the hook's default (medium) priority is
   covered by the type-keyed policy. Recorded in plan.md "Closed during plan repair" and
   Task 4.5c.
3. Does a read-only hidden-session view already exist (e.g. via `IncludeHidden` or the
   backlog item detail's review-session link)? Reuse before building.
4. What is "needs-human" for a hidden session concretely (approval request, guidance
   request, error state, stuck)? Map to existing `NotificationType`/priority values.
5. Swipe-to-dismiss vs. existing pane/terminal gestures — which regions are safe?
6. Tray on desktop: right edge overlay vs. docked push-aside; the operator asked for
   "a tray that comes out from one side" without changing the view — confirm overlay.
