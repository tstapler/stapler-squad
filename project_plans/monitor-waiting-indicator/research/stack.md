# Research: monitor-waiting-indicator stack

## Headline finding: this feature already exists end to end

Backlog item ae9ea980 describes a gap that appears to be **already closed** by two prior
PRs. Before implementing anything new, verify against a live/representative capture whether
a real gap remains (see "Possible remaining gaps" below) rather than re-building what's there.

- `feat(session): track and surface subagent count in WAITING_FOR_AGENT status (#312)`
- `fix(detection): recognize current auto-mode footer for WaitingForAgent (#678)` — commit
  `7335c2d7629ed8a06cf30c52956745ef159b5a89`

## 1. Existing Claude Code footer/status-line parsing

`session/detection/detector.go`:
- `autoModeFooterRegex` (line 35): `` `auto mode on\s*·\s*(\d+)\s+shells?(?:,\s*(\d+)\s+monitors?)?` `` —
  matches the persistent bottom-of-pane bar, e.g. `⏵⏵ auto mode on · 2 shells, 1 monitor · ← for agents`.
  This is exactly AC1's "shorter status-bar variant."
- `footerAgentCount(lines []string)` (line 40) scans backward for that footer, sums shell+monitor
  counts, returns `ok=false` if the footer reports zero of either.
- `applyFooterIdleOverride` (line 68) is a **post-hoc override**, deliberately kept out of the
  per-line `PatternSet`/priority-chain matching (`MatchLines`) — the footer is present at all
  times (active or idle), so folding it into the normal priority scan would let it mask
  Thinking/Active status for the entire duration of any background shell/monitor. It only fires
  once the rest of the scan already concluded `StatusIdle`/`StatusUnknown`, never overriding a
  genuinely active turn (`TestBug_AutoModeFooter_DoesNotOverrideActiveTurn`).
- `DetectedStatus` gets a dedicated `StatusWaitingForAgent` enum value (line 93).

`session/detection/binaries/claude.go` (`ClaudeDetector.Patterns()`, the single source of truth
for Claude's default pattern set — `getDefaultPatterns()` in detector.go delegates to it):
- A `WaitingForAgent` pattern group (lines 349–382) covers the **older**, mid-turn spinner-line
  format: `✻ Waiting for N background agent(s) to finish`, and the "N shell(s) still running" /
  "N monitor(s) still running" variants matching AC1's longer form almost verbatim
  (`shells_still_running`: `` `(\d+)\s+shells?\s+(?:still\s+)?running` ``; `monitors_still_running`:
  `` `(\d+)\s+monitors?\s+still\s+running` ``).
- These three patterns are explicitly commented as **dead against the current CLI** (verified via
  live tmux capture-pane during the auto-mode-footer investigation) — the modern CLI never emits
  that text, using the persistent footer (`autoModeFooterRegex`) instead. Kept only for older CLI
  versions / historical documentation, not treated as reachable in tests.

Regex/parsing style used throughout: raw Go `regexp` (RE2), no third-party regex lib. Heavy use of
named pattern structs (`dtypes.StatusPattern{Name, Pattern, Description, Priority}`) aggregated
into `dtypes.StatusPatterns` and compiled/ranked by `session/detection/pattern_set.go`'s
`PatternSet`/`MatchLines`. ANSI stripping lives in `pkg/ansi` (see `ExtractLastOSC`, referenced
from `claude.go`'s `ClassifyOSCTitle`) — not re-implemented per-detector.

## 2. Count propagation through the backend

- `InstanceStatusInfo.SubagentCount` (`session/instance_status.go:24`): "count of background
  agents/shells/monitors from the WaitingForAgent detector; 0 unless `ClaudeStatus ==
  detection.StatusWaitingForAgent`." Three distinct sources (background agents, shells, monitors)
  are deliberately collapsed into one int.
- Consumed in `session/review_queue_determiner.go` (treats `StatusWaitingForAgent` as evidence of
  real background activity, gated to avoid false idle-queue entries) and `session/status_mapping.go`
  (maps `StatusWaitingForAgent` alongside `StatusExecuting`/`StatusProcessing` for higher-level
  status rollup).
- Single detection entry point regardless of tmux mode: `session/claude_controller.go`
  (`GetCurrentStatus`, `GetStatusAndIdleInfo`, `resolveStatusFromTail`) wraps
  `detection.StatusDetector` and is fed PTY/scrollback bytes the same way whether the underlying
  session uses tmux control mode or legacy polling — both paths converge on `ClaudeController`
  before status ever reaches `server/services/session_service.go`. This satisfies AC4 already;
  no separate control-mode-vs-polling branching exists for this detector.

## 3. Frontend badge/indicator pattern

- `web-app/src/components/sessions/SubStatusChip.tsx` already renders a
  `SubStatus.WAITING_FOR_AGENT` chip: `⏳ Waiting for {N} Task(s)` (falls back to unnumbered
  "⏳ Waiting for Agents" when `subagentCount` is undefined/0/negative/NaN), with
  `role="status"`/`aria-label`/`title` set. Comment explains why "task" (not "agent") is used —
  the count merges background-agent, shell, and monitor sources.
- Styling: `web-app/src/components/sessions/SubStatusChip.css.ts` — `chipWaitingForAgent` is a
  `vanilla-extract` `style([chip, {...}])` composed from the shared `chip` base plus
  `vars.color.accentBg`/`vars.color.primary` tokens (same token pair as `chipProcessing`, but
  `fontWeight: normal` to read quieter than "Thinking…"). This is the reusable pattern for any
  new chip variant: extend the shared `chip` base, don't hand-roll new CSS.
- The chip is driven by a proto `SubStatus` enum (`@/gen/session/v1/types_pb`) with an exhaustive
  switch (`_exhaustive: never` guard) — any new sub-status requires a proto enum value plus a case
  here, not ad hoc string status.
- `SessionCard.test.tsx` already has coverage referencing this chip.

## 4. Detector registry plumbing (docs/reference/feature-testing-registry.md)

This feature isn't itself a new *Omnibar* action/detector (that registry covers
`OmnibarAction`/`DetectorRegistry` for user-input auto-detection in the composer, a different
subsystem from Claude-CLI-output status detection). The relevant registry entry point instead is:
- `docs/registry/features/` — per-feature JSON files; run `make registry-generate` after adding
  any new `// +api:`/`// +feature:` marker. If a genuinely new gap requires a new RPC field or
  React component, add the corresponding markers and regenerate.

## Existing test coverage (AC5 already substantially met)

`session/detection/bug_regression_test.go`:
- `TestBug_AutoModeFooter_WaitingWhenIdleWithBackgroundShells` — idle + footer → `StatusWaitingForAgent`,
  count = shells + monitors (e.g. 2 shells + 1 monitor = 3).
- `TestBug_AutoModeFooter_DoesNotOverrideActiveTurn` — footer present but turn genuinely active →
  must stay `StatusExecuting`, footer must not override.
- `TestBug_AutoModeFooter_SingularShell` — singular "1 shell" phrasing.

`web-app/src/components/sessions/SessionCard.test.tsx` references the `WAITING_FOR_AGENT` chip.

## Possible remaining gaps (to confirm before writing new requirements/plan)

1. **Scope check against the exact backlog wording.** The backlog title says "claude waiting on
   shell/agent format" — worth confirming with a fresh live tmux capture-pane (per the CLAUDE.md
   footer-verification precedent already used for PR #678) whether current Claude Code CLI output
   still matches `autoModeFooterRegex`, or whether the CLI has since changed the separator/wording
   again (e.g. em-dash vs `·`, reordering "monitor, shell").
   `autoModeFooterRegex` currently requires shells to appear before monitors in the given order
   (`shells?(?:,\s*(\d+)\s+monitors?)?`) — a footer with monitors only (no shells) or reordered
   would not match; worth a targeted regression test if not already covered.
2. **UI regression scope (AC3, AC6):** confirm the chip disappears once
   `subagentCount`/`ClaudeStatus` clears — likely already handled since the chip is driven purely
   by the current `SubStatus` value with no separate "sticky" state, but worth an explicit test if
   one doesn't already exist for the clearing transition.
3. If the backlog item is indeed fully resolved by #312 + #678, the SDD next step is likely a
   quick verification pass (fresh CLI capture + targeted test) rather than a new implementation
   plan.
