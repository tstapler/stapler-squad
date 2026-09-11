# Feature-landscape research: monitor-waiting-indicator

Companion to `architecture.md` (detector-package recommendation) and `ux.md` (UI treatment) —
this file focuses on (1) inventorying existing detector prior art, (2) edge cases, (3) unstated
needs. See those two files for the "this already partially exists" finding's implications for
design; this file adds the gap analysis for the combined comma-form line and the ANSI/wrap edge
cases they don't cover in depth.

## 1. Existing detector landscape

stapler-squad already has a mature "parse Claude Code CLI status text → structured signal →
UI indicator" pipeline, used as prior art by multiple prior backlog items:

- **`session/detection/`** — the general PTY-output-status-detector package
  (`detector.go`, `pattern_set.go`, `binaries/claude.go`'s `dtypes.StatusPattern` tables,
  `plugins.go`/`plugin_watcher.go` for hot-reloadable YAML pattern overrides). Produces a
  `DetectedStatus` enum (`StatusIdle`, `StatusExecuting`, `StatusNeedsApproval`,
  `StatusWaitingForAgent`, `StatusCompacting`, etc.) consumed by
  `server/adapters/instance_adapter.go` and mapped to proto (`proto_mapping.go`) for the
  frontend.
- **`session/detection/ratelimit/`** (`detector.go`, `manager.go`, `scheduler.go`,
  `integration.go`, `recovery.go`) — a *separate*, narrower detector for the specific
  "Claude usage limit reached" banner, with its own state machine and a scheduled recovery
  check (relevant precedent for "does staleness/timeout matter" — see §3).
- **`project_plans/detect-rate-limit/`** and **`project_plans/detect-and-address-rate-limits/`**
  — prior SDD projects that built the ratelimit detector above.
- **`project_plans/review-queue-state-detection/`** → `session/review_queue_determiner.go` +
  `server/adapters/review_queue_adapter.go` — a *derived-status* consumer pattern: it maps
  `DetectedStatus` values (including `StatusWaitingForAgent`, see
  `review_queue_adapter.go:14`) into a higher-level "is this session in the review queue"
  decision. Directly relevant: whatever new detection this feature adds must flow through the
  same `DetectedStatus`/count plumbing so `review_queue_determiner.go` doesn't need a second
  parallel signal.
- **`project_plans/stale-session-detection/`** — a different kind of staleness (session
  abandoned, not "monitor still running"); useful as precedent for how the repo phrases a
  timeout policy, but the mechanism (looking at wall-clock gaps in activity) doesn't
  transfer directly here since a *legitimately* long-running shell/monitor is expected to
  report no new scrollback for extended periods.
- **`project_plans/detector-plugins/`**, **`project_plans/remote-detection-manifests/`**,
  **`project_plans/detection-architecture-refactors/`** — architecture-level projects that
  generalized the detector plugin/pattern-table mechanism itself (YAML-driven
  `StatusPattern` tables, hot-reload). New patterns for this feature should be added to that
  existing table-driven mechanism, not a bespoke regex bolted onto a different file.

**Most directly relevant finding (confirmed by reading the code, not just docs):**
`session/detection/detector.go`'s `autoModeFooterRegex` / `footerAgentCount` /
`applyFooterIdleOverride` (added by a *prior*, apparently very recent, investigation —
comment references "the auto-mode-footer investigation") **already detects the short
persistent status-bar form** from the backlog's example:
```
⏵⏵ auto mode on · 1 shell, 1 monitor · ← for agents
```
via regex `` `auto mode on\s*·\s*(\d+)\s+shells?(?:,\s*(\d+)\s+monitors?)?` ``, and already
maps it to `StatusWaitingForAgent` with a combined shell+monitor count, applied as a
post-hoc override only when the per-line scan otherwise concluded idle/unknown (never
overriding Error/NeedsApproval/etc. — see `detector.go:63-76`). The frontend chip
(`web-app/src/components/sessions/SubStatusChip.tsx`) already renders a
"⏳ Waiting for N Tasks" badge for `StatusWaitingForAgent` with a `subagentCount` prop, and
`GetRateLimitState`-style getters plumb this through `instance_adapter.go`.

**Confirmed gap:** the *long-form* combined line from the backlog's own example —
```
✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running
```
is **not** correctly parsed by the existing `WaitingForAgent` patterns in
`binaries/claude.go` (`waiting_for_background_agent`, `shells_still_running`,
`monitors_still_running`). Those three patterns are explicitly commented as
"confirmed dead against Claude Code's current CLI... kept for older CLI versions" — that
comment is now contradicted by the requirements doc's captured real output, OR the capture
is from a still-current-but-different code path than what was tested. Either way:
`shells_still_running`'s regex (`` `(\d+)\s+shells?\s+(?:still\s+)?running` ``) requires
"running" immediately after "N shell(s)", but in the real line the shell count is followed by
`, 1 monitor still running` — so it never matches, and the shell count is silently dropped.
Only `monitors_still_running` fires (on "1 monitor still running"), giving an undercounted
total of 1 instead of 2. **This is the concrete bug the new work needs to fix**: a new
pattern (or a rewrite of the existing "dead" comment + regex) needs to match the combined
`N shell(s), N monitor(s) still running` clause as one unit, the same way `autoModeFooterRegex`
already does for the footer's `N shells, M monitors` clause. Reusing/adapting
`autoModeFooterRegex`'s two-capture-group approach for a new `stillRunningRegex` anchored on
`still running` instead of `auto mode on` is the natural fix, rather than patching the two
separate dead per-type patterns.

## 2. Edge cases

- **Combined vs. separate counts, changing over turns**: the footer form always shows the
  *current* total (it's live/persistent), so no accumulation logic is needed — each new
  scrollback snapshot simply reports the latest counts. The long-form line is a one-shot
  turn-completion summary; if a *later* turn also finishes with outstanding work, a new
  line is appended below (never mutated in place) — the backward/most-recent-line scan
  pattern already used by `footerAgentCount` (iterates `len(lines)-1` down to 0) is correct
  and must be preserved for the long form too, so an earlier turn's stale count doesn't win
  over a later one.
- **Count reaching zero without the line disappearing**: `footerAgentCount` already treats
  `total <= 0` as `ok=false` (i.e., "nothing to override with") rather than treating "0
  shells, 0 monitors" as a positive match — needed because the footer bar's shell/monitor
  clause disappears from the bar entirely once counts hit zero (per the real example, the
  bar becomes bare `⏵⏵ auto mode on · ← for agents` with no comma clause), so this case may
  not be reachable in practice for the footer. It **is** reachable for the long form if a
  future CLI version ever emits `0 shell, 0 monitor still running` as an artifact — the new
  pattern should mirror the `total <= 0 → no match` guard defensively.
- **ANSI color codes / box-drawing chars around the status bar**: the example shows the
  status bar sandwiched between `─` (U+2500) box-drawing separator lines, and the real
  terminal render includes ANSI SGR codes for the dim/highlighted bar styling. Existing
  patterns in `binaries/claude.go` already assume `pkg/ansi` strips SGR codes upstream
  before matching (confirmed by `detector.go`'s import of `github.com/tstapler/stapler-squad/pkg/ansi`
  and other patterns like the spinner-glyph regexes not needing to account for color escapes)
  — the new pattern should be added to the same post-ANSI-strip text, not re-solve stripping.
  Box-drawing separator lines are already excluded implicitly since the regex is anchored to
  literal text, not full-line matching.
- **Line wrapping at different terminal widths**: the footer bar and long-form line are both
  single logical lines that tmux/the PTY may hard-wrap at narrow widths, splitting e.g.
  `1 shell, 1 monitor still running` across two captured lines. None of the existing
  `WaitingForAgent`/footer regexes handle a mid-pattern line break — this is a real,
  currently-unaddressed gap shared with *all* existing single-line patterns in this file (not
  new to this feature), so the fix should follow whatever convention (if any) the rest of
  `pattern_set.go`/`detector.go` uses for reassembling wrapped lines before matching, rather
  than inventing a one-off unwrap step just for this pattern. Grep of `pattern_set.go` shows
  no existing unwrap logic — worth flagging to the planning phase as a possible pre-existing
  limitation rather than solving it net-new here (scope risk).
- **Line appearing mid-scrollback vs. only at the tail**: the long form appears once per
  turn completion and then scrolls up as new output is appended — a session actively
  producing lots of subsequent output could push it out of the most-recent-N-lines window
  used for detection. Need to confirm (in the planning/architecture phase) how many trailing
  lines `detectFromLines`/`DetectWithContextAndCountFromLines` scans, and whether that window
  is sufficient given that the *footer* bar (always at the true tail) is the more reliable
  steady-state signal — the long form is best treated as a same-turn confirmation/backstop,
  not the primary detection path.
- **False positives from literal text in agent-authored content**: since Claude Code sessions
  in this repo are frequently used to *write code that talks about Claude Code itself* (as
  this very research task demonstrates), a session could have the literal string
  `1 shell, 1 monitor still running` appear inside a diff, quoted log, or comment being
  authored — e.g. exactly the requirements.md file this research produced. The existing
  `autoModeFooterRegex` mitigates this somewhat by requiring the `auto mode on ·` prefix
  (unlikely to appear in ordinary quoted text), but the long-form `N shell, N monitor still
  running` pattern has no comparably distinctive anchor — it's a plausible substring in any
  document *about* this feature (including this repo's own `project_plans/` tree and test
  fixtures). Mitigation options for the planning phase: anchor to the `✻`/`◉`/`✦` spinner
  glyph prefix + turn-duration text (`Cogitated for`, `Churned for`, etc.) the same way the
  existing dead patterns did, and/or require the match be within the last N lines of *raw
  PTY output* specifically (not e.g. a file-read tool result rendered into the transcript,
  if those are distinguishable in the capture pipeline).

## 3. Unstated user needs

- **Click-through to see which shell/monitor is running**: the backlog item and requirements
  doc only ask for a *count*, not identity — Claude Code's own status line doesn't expose
  *which* shell/monitor command is outstanding either (it's just a number), so
  stapler-squad has no data to click through *to* even if the UI wanted this. Confirmed by
  checking `SubStatusChip.tsx`: its existing "⏳ Waiting for N Tasks" tooltip
  (`SubStatusChip.tsx:52-57`) already just describes count, not identity — consistent with
  what the CLI can express. Recommend explicitly marking this out-of-scope in the plan
  (matching acceptance criteria, which never mention identity) rather than silently
  under-delivering against an implied expectation.
- **Should this force a session out of "needs attention"?**: yes, functionally already the
  intent of `StatusWaitingForAgent` — `review_queue_adapter.go:14` and
  `idle.go:227`/`osc_priority.go` already treat `StatusWaitingForAgent` as an active/non-idle
  state, distinct from "needs approval" or "needs input", which is exactly the backlog's
  stated goal ("shouldn't mistake waiting-on-monitor for idle/needs-input"). The gap is
  purely in *detecting* the long-form line correctly (§1), not in how the resulting status
  is consumed downstream — no new status-mapping logic is needed once detection is fixed.
- **Timeout/staleness rule if a monitor never resolves**: not covered by any existing
  detector for `StatusWaitingForAgent` (contrast with `ratelimit/scheduler.go`'s explicit
  recovery-check scheduling, or `stale-session-detection`'s general abandonment heuristic).
  Worth raising as an open question for the planning phase rather than assuming
  in-or-out-of-scope: a genuinely stuck background shell (e.g. a hung build) would show
  "1 shell still running" indefinitely with no distinguishing signal from a slow-but-healthy
  one. The acceptance criteria as written don't ask for this, so the safe default is to
  ship without a staleness rule and let `stale-session-detection`'s existing general
  mechanism (if it looks at scrollback-quiescence rather than specific status) catch the
  pathological case — confirm this in planning rather than building a second timeout system.

## Cross-references

- `project_plans/monitor-waiting-indicator/research/architecture.md` — recommends whether to
  extend `session/detection/` (Pattern A) vs. build a `ratelimit`-style standalone package
  (Pattern B); this file's finding that the footer form is *already* Pattern A code is
  relevant input to that decision (the fix for the long-form gap should almost certainly go
  in the same file/table as the already-working footer code, i.e. Pattern A, rather than
  fragmenting the "waiting on background work" signal across two different detector
  architectures).
- `project_plans/monitor-waiting-indicator/research/ux.md` §0 — corroborates "this already
  exists, partially" from the UI side (`SubStatusChip` already renders it for
  `StatusWaitingForAgent`); this file's §1 supplies the specific regex-level reason the
  long-form capture from the backlog doesn't yet reach that UI correctly.
