# ADR-001: Extend `session/detection/binaries/claude.go`'s Pattern Table Instead of a New `monitorwait/` Package

**Status**: Accepted
**Date**: 2026-09-09

## Context

`research/architecture.md` proposes a standalone `session/detection/monitorwait/` package mirroring
`session/detection/ratelimit/`'s independent Detector/Manager/PTYConsumer structure, plus new proto
fields, on the theory that "N shell, N monitor still running" is a transient, independently-clearing
signal like rate-limiting. `research/build-vs-buy.md`, `research/pitfalls.md`, `research/stack.md`,
and `research/features.md` (4 of 6 research files) instead recommend fixing/extending the existing
regex-based pattern table in `session/detection/binaries/claude.go`.

Verifying the codebase directly (not just trusting the research summaries) confirms the smaller
option is not just cheaper but structurally correct:

- The short footer form of this exact problem ("⏵⏵ auto mode on · N shells, M monitors") was already
  solved this way, not with a new package — `autoModeFooterRegex` +`footerAgentCount` +
  `applyFooterIdleOverride` in `session/detection/detector.go:19-76`, landed in PR #678
  (`fix(detection): recognize current auto-mode footer for WaitingForAgent`,
  commit 7335c2d76) and its two follow-ups (974ead4f0, 13b600964).
- The pipeline this fix needs to feed already exists and is generic, not footer-specific:
  `PatternSet.matchWaitingForAgent` (`session/detection/pattern_set.go:92-109`) extracts a count from
  whichever `WaitingForAgent` pattern wins, `InstanceStatusInfo.SubagentCount`
  (`session/instance_status.go:22-24,137`) carries it through `ClaudeController.GetStatusAndIdleInfo`
  (`session/claude_controller.go:1105`), `instance_adapter.go:256` maps it onto the proto
  unconditionally, and `SubStatusChip.tsx`'s `WAITING_FOR_AGENT` case already renders "⏳ Waiting for N
  Tasks" from that count. Nothing about the long-form line's shape (same two counts, same "shell(s)"/
  "monitor(s)" vocabulary) requires a new representation.
- The actual bug is narrowly mechanical: `shells_still_running`'s regex
  (`` `(\d+)\s+shells?\s+(?:still\s+)?running` ``, `binaries/claude.go:368`) requires "running"
  immediately after the shell count, but the real captured line
  (`"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"`, from the backlog
  item itself) has a comma-joined monitor phrase in between, so it never matches; only
  `monitors_still_running` fires, silently dropping the shell count. This is a regex-and-summation
  gap in an existing pattern table, not a missing lifecycle/state machine — there is no
  recovery/scheduler need here the way `ratelimit/` has (rate limits have a cooldown timer to track;
  this signal is read fresh from PTY content on every call and has no timing state at all).

## Decision

Fix the undercounting by (1) extending `shells_still_running`'s regex to optionally capture a
comma-joined monitor count, mirroring `autoModeFooterRegex`'s comma-joined shape, and
(2) generalizing `PatternSet.matchWaitingForAgent` to sum all captured digit groups instead of only
`m[1]`, mirroring `footerAgentCount`'s summation. No new package, no new proto fields, no new UI
component. `InstanceStatusInfo.SubagentCount` and `SubStatusChip.tsx` are reused unchanged.

## Reasoning

- Matches this codebase's own prior art for the identical problem shape (PR #678), rather than
  introducing a second, differently-structured mechanism for what a session card renders as the same
  "⏳ Waiting for N Tasks" chip either way.
- The counts are explicitly derived/ephemeral per both `research/architecture.md` and
  `research/build-vs-buy.md` (neither recommends storing them in `InstanceSnapshot`) — a new package
  would still end up recomputing from PTY content on every call, exactly like the existing
  `footerAgentCount`/`matchWaitingForAgent` do today, so a new package buys no lifecycle benefit while
  adding an independent Detector/Manager/PTYConsumer surface, a second count-plumbing path alongside
  `SubagentCount`, and a second place for the two representations of "shells+monitors waiting" to
  drift apart.
- `ratelimit/`'s independent-package shape earns its cost from real state to manage (a cooldown
  window, recovery scheduling). This signal has none of that — it is a pure function of the current
  PTY tail, recomputed fresh every call (see Pitfall guidance against caching it on `*Instance`,
  `plan.md`'s Pattern Decisions row on this point) — so the pattern that shape exists to serve does
  not apply here.

## Consequences

- `session/detection/binaries/claude.go`'s `shells_still_running` pattern and
  `session/detection/pattern_set.go`'s `matchWaitingForAgent` are the only production code touched.
- The stale "confirmed dead against current CLI" comment on `binaries/claude.go`'s `WaitingForAgent`
  block (`claude.go:342-348`) is corrected for `shells_still_running`/`monitors_still_running`, which
  are now confirmed live via the backlog's own captured output — `waiting_for_background_agent`
  remains unverified/legacy and keeps its caveat.
- No new proto fields, no new package, no `InstanceSnapshot` changes, no new UI component — see
  `plan.md`'s Tech Debt Disposition (Extend as-is) and Pattern Decisions for the corresponding
  per-component choices.
- If a future capture ever shows Claude Code emitting the reverse order ("N monitor, M shell still
  running") or a three-way combination, that is a new regex variant to add to the same pattern table,
  not a reason to revisit this decision.
