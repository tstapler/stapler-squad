# UX research: "background work in progress" indicator

## 0. Important finding first: this already exists, partially

`SubStatus.WAITING_FOR_AGENT` (`web-app/src/components/sessions/SubStatusChip.tsx:38-64`) already
renders exactly this signal, driven by `session.subagentCount`
(`web-app/src/components/sessions/SessionCard.tsx:792`, `SessionRow.tsx:359`):

```tsx
⏳ Waiting for {hasCount ? `${subagentCount} ${isSingular ? "Task" : "Tasks"}` : "Agents"}
```

The component's own comment says the count is "three distinct WaitingForAgent sources
(background agents, shells still running, monitors still running) collapsed into one int" —
i.e., today's Claude Code footer line ("1 shell, 1 monitor still running") is the intended source
of this exact chip. Any new work here is refinement (splitting the collapsed count back into
shell/monitor/agent sub-counts, fixing the mis-classification-as-idle problem the task describes,
adding aggregation/edge-state handling), not building the concept from scratch. Confirm with
whoever filed this task whether the "no way to show this" framing is stale, or whether the gap is
specifically that a session showing `WAITING_FOR_AGENT` still risks being *misread* as idle
elsewhere in the UI (e.g., in a Status-grouped board column, or the moment before the chip
resolves).

## 1. Comparable UX patterns

| Tool | Pattern | Visual language |
|---|---|---|
| VS Code | Status-bar background task items (e.g. TS server, extensions) show a spinning codicon + short label; long tasks add a progress ring | Spinner (indeterminate) or ring (determinate), never just a static icon |
| GitHub Actions | Yellow/amber filled circle with a spinner animation for in-progress; solid check/x once resolved | Color + motion + shape, motion carries "still running" |
| CI badges (CircleCI, Jenkins) | "N running" chip on a dashboard aggregate view, separate per-job dot on detail view | Count badge at aggregate level, per-item dot at leaf level — two altitudes, not one |
| tmux/screen status line | Persistent text segment (e.g. `1 shell, 1 monitor`) that only appears when non-zero | Text-count, no icon — relies on brevity and permanence, appropriate for a single-user terminal but not for a scannable list of many sessions |
| Claude Code CLI itself | `⏵⏵ auto mode on · 1 shell, 1 monitor · ← for agents` | Persistent footer, disappears fully when count is 0 — precedent already followed by this repo's `StuckNavBadge`/`UnfinishedNavBadge` ("hidden when count is 0") |

Takeaway: an icon + count reads faster than a bare dot at a glance across a list of many
sessions (the CI-dashboard "N running" pattern), but the count must disappear entirely at zero
rather than show "0" — this repo already enforces that convention in every nav badge it has
(`StuckNavBadge.tsx:32`, referencing `UnfinishedNavBadge.tsx`). A spinner alone (no count) is
the CI per-job/VS Code pattern and fits best on a single detail view where there's only one
thing to track; a list of session cards needs the count for scannability.

## 2. Minimal signal for the user's mental model

The question a user asks glancing at the session list is binary and consequential: "can I touch
this session's devstack right now, or would I step on something?" That maps to *existing*
`SubStatus.WAITING_FOR_AGENT` far more cleanly than to `PROCESSING` ("Thinking…", foreground,
spinner) — the repo already treats these as two distinct sub-statuses with two distinct chips,
which matches the CLI's own distinction (foreground turn vs. footer-line background jobs). The
minimal fix for the "could be misread as idle" risk described in the task is likely at the
*aggregation* layer (Status-based grouping, `docs/reference/tag-organization.md`'s eight grouping
strategies) rather than the chip itself: verify a session in `WAITING_FOR_AGENT` sorts into an
"active/busy" bucket, not an "idle" one, wherever Status grouping buckets sessions. That's a
one-line categorization check, not a new component.

No extra click should be required — precedent (`SubStatusChip`, `RemoteConnectionIndicator`) is
that the badge itself carries the full answer via its visible label + `title` tooltip; a click
target is for taking action (e.g. jumping to the session), not for revealing the state.

## 3. Accessibility — established pattern to match

Every status indicator in `web-app/src/components/sessions/` and `layout/ConnectionIndicator.tsx`
follows the same recipe; a new indicator should too:

- `role="status"` (or `role="img"` for `RemoteConnectionIndicator`, which pairs it with a separate
  `aria-live` region) on the visible badge element.
- `aria-label` carries the full semantic meaning ("Claude is waiting for N background tasks to
  finish"), independent of the icon glyph or color.
- Icon glyph (`⏳`, `⚠`, `✓`, etc.) is wrapped in its own `<span aria-hidden="true">` — decorative,
  never the sole carrier of meaning.
- `title` attribute duplicates/extends the `aria-label` for mouse-hover users — see
  `StatusBadge.tsx:94-101` and every `SubStatusChip` case.
- Color is never the only differentiator: each state pairs a distinct icon *and* a distinct label
  text *and* a CSS variant class (`SubStatusChip.css.ts`'s `chipWaitingForAgent` etc.), satisfying
  WCAG 1.4.1.
- State transitions that need to be *announced* (not just displayed) use a dedicated
  `aria-live="polite"` region separate from the visual badge, with `role="alert"` reserved for
  terminal/attention-worthy transitions (`RemoteConnectionIndicator.tsx:92-101,117-122`). If a
  "stuck forever" edge state (below) needs an announcement, follow this split rather than
  overloading the visible badge's own `aria-live`.
- Nothing in this family is keyboard-focusable/clickable today (they're passive `role="status"`
  spans) — if a new indicator becomes clickable (e.g. to jump straight to the shell/monitor
  output), it needs a real interactive role (`button` or a focusable link), not a `span` with a
  click handler, to stay keyboard-operable.

## 4. Error / edge states

- **Stuck forever (monitor never resolves):** the repo already has a designed answer for "this
  polled state might be lying to you" — `backlog-stuck/stuckReason.ts:91-115`'s staleness
  threshold pattern: track a last-updated timestamp per background task and, once it exceeds a
  threshold, stop trusting the "still running" label and surface a distinct "unknown/stalled"
  variant instead of leaving the spinner spinning indefinitely. Apply the same idea here: if a
  shell/monitor's "still running" signal hasn't refreshed within some bound, the chip should shift
  from `⏳ Waiting for N Tasks` to something like `⚠ Task status unknown` rather than spin forever.
- **Flickering count:** rapid count changes (tasks completing/starting in quick succession) are a
  classic layout-thrash and distraction risk. No existing component in this repo debounces a
  count, so this would be new — worth a short debounce/coalesce (e.g. 1-2s) on the displayed
  number, matching the spirit of `useDropEpisodeCoalescer.ts` (an existing coalescing hook in
  `sessions/`) rather than re-rendering on every raw update.
- **Aggregation across sessions:** strong precedent for a global nav-level summary badge exists —
  `StuckNavBadge`, `ReviewQueueNavBadge`, `ApprovalNavBadge`, `MemoryNavBadge` (all in
  `web-app/src/components/`) each show "count across all sessions" in the nav, hidden at zero,
  with a neutral skeleton before the first fetch resolves rather than a misleading "0"
  (`StuckNavBadge.tsx:17-29`). A `WaitingForAgentNavBadge` following the identical shape (same
  hook pattern as `useStuckBacklogItems`) would be the natural aggregate view, and should be
  considered part of this feature's scope, not a follow-up — the whole point of the "job to be
  done" below is surfacing this at a glance without opening each session.

## 5. Job-to-be-done

The emotional job is loss-avoidance, not information delivery: prevent "I killed a session while a
build was running." This repo's own CLAUDE.md documents the exact real-world version of this
mistake at the *service* level — `make install-service` restarts the live systemd unit and kills
every live tmux session including in-flight work, unless `--tmux-keep-server` is passed
(`docs/explanation/tmux-keep-server-on-restart.md`). The per-session indicator is the same
mistake at a smaller blast radius: a user reaching for "stop/kill this session" or "restart this
session" (`ConfirmKillDialog.tsx`, `RestartWithSummaryButton.tsx` both already exist) on a session
that looks idle but has a shell or monitor in flight. The design implication: this indicator's
highest-value placement is wherever those destructive actions are triggered from (the session
card's kill/restart affordances), not just the passive status list — consider whether
`ConfirmKillDialog` should surface "this session has N background tasks still running" as an
explicit warning line when `WAITING_FOR_AGENT` is true, mirroring how the dialog already exists
specifically to add friction before a destructive action.
