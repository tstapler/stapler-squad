# UX Research: Webhook / Event-Driven Trigger and Callback Configuration

Agent 5 (UX Research), SDD phase 2 for `webhook-triggers`. Scope per
`project_plans/webhook-triggers/requirements.md`: trigger config (github_push /
cron / webhook), outbound callbacks, pipeline chaining, human oversight,
source attribution (FR5/AC6), non-silent failure surfacing (FR9/AC8).

## 1. Comparable UX patterns

### GitHub Actions (workflow triggers + run history)
- Trigger config (`on:` block) is declarative and versioned in-repo, but the
  **execution surface is separate**: the Actions tab shows a run list per
  workflow with status icon (✓/✗/●-in-progress/⊘-skipped), trigger event,
  branch, actor, and duration — one row per *attempt*, not just successes.
- Each run is clickable into full logs, including a **"why did this run"**
  breadcrumb (event payload summary at the top of the log).
- No explicit "test my trigger" button — the analogous experience is
  `workflow_dispatch` (manual trigger button) which doubles as a live test
  path without needing a real event.
- What works: the run list is the single source of truth for "is this alive,"
  and non-matching pushes to unrelated branches simply don't appear as noise
  — filtering happens at the trigger-eval layer, not the display layer.

### Zapier / n8n (trigger setup + execution log)
- Both show a **per-trigger "Task History" / "Executions" log** distinct from
  workflow *definition*, with a status per row: Success, Error, Filtered
  (n8n's term for "matched trigger but didn't pass a filter/condition
  node" — the direct analogue of this feature's `label_filter`/`event`
  mismatch), and Waiting/Retrying for outbound webhook deliveries.
- Zapier explicitly surfaces a **"Test trigger"** button before the Zap can
  be turned on — it fetches one real recent sample event and shows the
  parsed fields, letting the user confirm field-mapping (e.g. their
  `{{issue.key}}`-style template) resolves correctly *before* saving live.
  This is the single highest-leverage pattern to borrow: a **dry-run
  preview** of the rendered prompt against a sample/most-recent payload,
  shown inline in the trigger-creation form, before the user commits to
  "enabled."
- n8n additionally shows a **relative "last triggered" timestamp with a
  colored status dot** directly on the trigger/workflow list view (not
  buried in a sub-page) — this is the minimum-viable trust signal (see §2).
- Both distinguish "trigger fired, downstream step failed" from "trigger
  never fired" — critical because this repo's FR pairs a trigger firing
  with *session creation*, which can itself fail (path resolution, worktree
  creation, resource limits) independent of the trigger match succeeding.

### This repo's existing Rules UI (`ApprovalRulesPanel.tsx`,
`web-app/src/components/sessions/ApprovalRulesPanel.tsx`, and
`web-app/src/components/rules/RuleBuilderForm.tsx`)
This is the closest in-repo precedent and should be the template to extend,
not a new pattern to invent:
- **Hits (7d) column** (`ApprovalRulesPanel.tsx:521-529,566-574`) — a
  per-rule fire count with a "—" placeholder when zero, sourced from
  `useApprovalAnalytics`. Direct precedent for a per-trigger "fires (7d)" /
  "last fired" column on a new Triggers panel.
- **Source badges** (`sourceLabel`/`sourceBadge`/`configFileBadge`,
  lines 61-69, 550-565) distinguish where a rule came from (`user`, `seed`,
  `claude-settings`, `config`) with a `title` tooltip explaining each — same
  shape needed for trigger *type* (github_push/cron/webhook) and for
  session-origin attribution (FR5/AC6: "created by trigger X").
- **Enable/disable toggle** per-row (`toggle`/`toggleOn`/`toggleOff`,
  lines 576-588) with built-in rules shown as an un-toggleable "Always on"
  badge — directly reusable for FR6/AC7 (enable/disable without redeploy).
- **Empty state per filter tab** (lines 471-503) gives contextual guidance
  rather than a bare "no data" — worth replicating for a trigger list
  filtered by type with zero configured triggers of that type.
- **Mobile FAB + `headerButtonsHiddenOnMobile`** (lines 311-348, 628-635) —
  the established mobile pattern: primary create action becomes a
  bottom-corner FAB, secondary header actions (export/import/generate) are
  hidden entirely on small viewports rather than horizontally scrolled.
- **`aria-live="polite"` region for async state changes** (lines 366-372) —
  used for "Generating rule suggestions…" / "Rule suggestions ready."; the
  direct analogue for trigger UI is announcing "Trigger test succeeded" /
  "Webhook rejected: invalid signature" without requiring the screen-reader
  user to be focused on the result area.
- Gap: nothing in the current Rules UI shows an **execution/attempt log** —
  hits are aggregate counts only, no per-attempt drill-down. A Triggers
  panel needs a new list/table view this repo doesn't have a precedent for
  yet (see §2 for what it must contain).

## 2. User mental models: what does "this trigger is live" require?

A user configuring a trigger is really asking three questions, in order of
urgency: **(a) is it saved and enabled, (b) has it ever fired, (c) when it
fires, does it do the right thing.** Minimum trust bar, informed by n8n/Zapier
above and by this repo's existing hits-column precedent:

- **On the list view itself** (no drill-down required): enabled/disabled
  state, last-fired relative timestamp ("3m ago" / "Never fired"), and a
  status-colored dot for the *most recent* attempt (success / failed /
  rejected). This is the `hits`-column pattern generalized with a status
  color, matching n8n's list-level status dot.
- **On demand (expand row / detail panel), not by default**: a log of
  individual attempts. Users do **not** default-expect to see every
  non-matching event (e.g. every push to every branch) — that would be
  noise for `github_push`/`webhook` triggers with tight filters, mirroring
  GitHub Actions' choice not to show runs for un-configured branches.
  However, non-matches SHOULD be available on demand as a **"N events
  received / M matched"** counter, because it's the only way to
  distinguish "trigger is correctly filtering out noise" from "trigger is
  dead and receiving nothing" — a webhook receiving zero HTTP requests at
  all looks identical to a correctly-filtering trigger unless the received
  count is shown separately from the matched count.
- **A pre-save dry-run / test action** (Zapier's "Test trigger", borrowed
  here as a "Send test event" / "Preview rendered prompt" button) is the
  single biggest trust-builder before the user commits — this maps directly
  to FR4's Go-template rendering: show the rendered prompt against a
  sample payload *before* the trigger goes live, catching template typos
  (`{{issue.key}}` vs `{{ .Issue.Key }}` mismatches) at config time instead
  of at 2am when a real event silently produces a garbage prompt.
- Users expect **rejections (bad signature, malformed payload) to be
  visible** but do not expect them mixed into "normal" activity — these
  need a distinct visual treatment (see §4), not just another row that
  looks like a routine non-match.

## 3. Accessibility (vanilla-extract convention, WCAG/ARIA/keyboard)

Per `.claude/rules/css-architecture.md`, new components use `.css.ts` +
`vars` tokens, not raw hex/`var(--undefined)`. For a trigger-config form +
execution-history table specifically:

- **Form**: every trigger-type-specific field set (github_push's repo/branch,
  cron's schedule string, webhook's event/label_filter/prompt_template) must
  be a proper `<fieldset>`/`<legend>` or have `aria-labelledby` per group —
  screen-reader users switching trigger `type` via a radio/select need the
  newly-revealed fields announced, matching the existing rule builder's
  approach of conditionally rendering fields by rule shape
  (`web-app/src/components/rules/RuleBuilderForm.tsx`).
- **Live regions**: reuse the `aria-live="polite"` visually-hidden-span
  pattern from `ApprovalRulesPanel.tsx:366-372` for: "Test event sent,"
  "Trigger enabled," "Webhook rejected: invalid signature" — anything that
  changes state without a page navigation.
- **Status is never color-only**: success/failed/rejected/disabled status
  dots need a text label or icon+text pairing (not just a colored dot),
  per WCAG 1.4.1 (Use of Color) — this repo's `decisionBadge` pattern
  (text label inside a colored badge, `ApprovalRulesPanel.tsx:546-548`)
  already satisfies this; the trigger status indicator should follow the
  same badge-with-text shape rather than introducing a bare dot.
- **Keyboard**: the existing rule builder closes on `Escape`
  (`ApprovalRulesPanel.tsx:137-144`) and rows have `aria-label`s on
  icon-only buttons (toggle, delete) — same requirements apply: every
  icon-only action (test/disable/view-log) needs an `aria-label`, and any
  new modal/dialog (e.g. a "test trigger" panel) needs focus trap + Escape
  to close, matching the existing `role="dialog"` on `#rule-builder`
  (line 638).
- **Sortable table headers**: existing `thSortable` pattern
  (lines 508-529) uses `onClick` on a `<th>` with a text sort-direction
  arrow appended — should be upgraded to `<button>`-in-`<th>` or
  `aria-sort` attribute if this pattern is reused for a trigger list, since
  a bare `onClick` on a `<th>` is not keyboard-operable as-is (worth flagging
  as a pre-existing gap to fix if the pattern is copied forward, not just
  replicated).

## 4. Error states: what must be visually distinguishable

Per `feedback_document_ai_decisions_in_edge_cases` (automated actions must
post a visible comment/notify, not act silently) and FR9/AC8, these are
**five distinct states**, not a binary success/fail, and each needs its own
badge/color, matching the `decisionAllow`/`decisionDeny`/`decisionEscalate`
three-way badge precedent (`ApprovalRulesPanel.css.ts`):

| State | Meaning | Visual treatment | Where surfaced |
|---|---|---|---|
| **Fired successfully** | Trigger matched, session/backlog item created | Green badge, link to the created session | List row + session's own "created by trigger" attribution badge (AC6) |
| **Fired, session creation failed** | Trigger matched but downstream `create_session` path errored (worktree failure, resource limit, etc.) | Amber/red badge distinct from "did not match" — this is the state most likely to be silently swallowed if not designed for explicitly | List row, with error detail on expand; should also surface via existing notification path, not just buried in a log |
| **Did not match** | Event received, filter/branch/label criteria not satisfied | Neutral/gray badge, collapsed by default (not noise) | Only in the "N received / M matched" on-demand log, not the default list |
| **Rejected (bad signature / malformed)** | HMAC signature invalid or payload malformed — request never reached match evaluation | Distinct red "Rejected" badge, separate from "did not match" — conflating these would hide a security-relevant signal (someone is sending unauthenticated requests to this endpoint) | Should be visible without expanding — a rejected-count creeping up is the signal an attacker/misconfigured sender is probing the endpoint; treat like a failed-login counter, not routine noise |
| **Disabled** | Trigger exists but is toggled off | Grayed row, matching existing `rowDisabled` class (`ApprovalRulesPanel.tsx:536`) | List view, same toggle pattern as approval rules |

The "rejected" and "session creation failed" states are the two most likely
to get silently dropped if the UI only tracks "fired" vs "not fired" as a
boolean — both need their own first-class status value in whatever backend
event-log schema Phase 3 designs, not a derived/inferred state from HTTP
status codes alone.

## 5. Jobs-to-be-done

- **Functional job**: "Get work started without me babysitting a queue of
  GitHub pushes/tickets." Success criteria: a trigger fires within the
  expected latency (webhook: near-instant; cron: within the schedule
  tolerance) and the resulting session appears in the normal session list
  with zero extra manual steps.
- **Emotional job**: "Trust that automation won't run wild or silently drop
  events." This repo already has a concrete cautionary precedent — the
  2026-07-12 OOM incident (`feedback_backlog_wip_limit.md`) from uncapped
  concurrent session creation. A trigger system is a *new, unattended*
  session-creation path, which raises the stakes on the same failure mode:
  a misconfigured cron trigger or a replayed/duplicated webhook delivery
  could create sessions far faster than a human ever would by hand. The UX
  implication: the trigger config UI should show (or link to) the same
  WIP-limit guardrail this repo already enforces manually for backlog
  work, and should make **rate/volume of trigger-created sessions
  visible** (e.g., "12 sessions created by this trigger in the last hour")
  so a runaway trigger is caught by the human, not just by a backend cap.
  This is also why "fired, session creation failed" (§4) must never be
  silently retried into a loop without surfacing.
- **Social job**: "When a teammate asks 'why does this session exist,' I
  can answer without archaeology." This is FR5/AC6 directly — trigger
  attribution must be visible on the session itself (not just in a
  separate trigger admin log), the same way `sourceBadge`/`configFileBadge`
  makes a rule's origin visible inline rather than requiring a lookup.
  Recommend: a small badge on the session card/detail view reading
  "Triggered by: <trigger name> (github_push)" with a link back to the
  trigger's config and its execution-log entry for that specific firing —
  symmetric navigation (session → trigger, trigger-log-row → session) is
  what makes this auditable rather than one-directional.

## Mobile + desktop requirement

Per this repo's standing convention (memory note
`feedback_mobile_desktop_ux.md`: always consider both form factors) and the
concrete precedent in `docs/tasks/mobile-ux-improvements.md` (44px minimum
touch target for interactive controls, confirmed at
`docs/tasks/mobile-ux-improvements.md:296-297,396-397`):
- Any new trigger-list row actions (test/edit/toggle/delete) must meet the
  44px touch target minimum already enforced elsewhere in this codebase.
- Follow the existing `mobileAddFab` + `headerButtonsHiddenOnMobile` split
  (`ApprovalRulesPanel.tsx`): "Add Trigger" becomes a FAB on mobile; lower-
  priority actions (e.g. "Test all," "Export config") hide on small
  viewports rather than cramming into a scrolling toolbar.
- The execution-history table needs a mobile-friendly stacked/card layout
  (not a horizontally-scrolling wide table) if it carries more than 3-4
  columns (status, trigger name, timestamp, matched-summary) — consistent
  with this repo's general avoidance of horizontal scroll on mobile per the
  mobile-ux-improvements plan's scope (toolbar overflow, safe-area insets).

## Key files referenced

- `web-app/src/components/sessions/ApprovalRulesPanel.tsx` — closest
  existing precedent (hits column, source badges, toggle, mobile FAB,
  aria-live, empty states); extend this shape for a Triggers panel rather
  than inventing a new one.
- `web-app/src/components/sessions/ApprovalRulesPanel.css.ts` — badge/toggle
  token usage to mirror.
- `web-app/src/components/rules/RuleBuilderForm.tsx` — conditional
  field-set-by-type form precedent, relevant to github_push/cron/webhook
  trigger-type-specific fields.
- `web-app/src/components/sessions/HookStatusPanel.tsx` — simple
  install/status toggle pattern, secondary precedent for an
  enabled/disabled callback config surface.
- `docs/tasks/mobile-ux-improvements.md` — 44px touch target convention,
  toolbar overflow handling.
- `.claude/rules/css-architecture.md` — vanilla-extract token usage rules.
- `project_plans/webhook-triggers/requirements.md` — FR5/FR6/FR9/AC6/AC7/AC8
  are the requirements this research most directly informs.
