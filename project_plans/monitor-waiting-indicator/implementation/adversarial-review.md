# Adversarial Review: monitor-waiting-indicator

**Date**: 2026-09-09
**Verdict**: RESOLVED (plan.md patched — see resolution notes below; original findings preserved for record)

## Blockers

- [x] **RESOLVED** — AC3's given/when/then (`plan.md:187-195`) and Task 1.1.1c (`plan.md:251-256`) now use the realistic idle-screen fixture (`"❯ "` + `"  ? for shortcuts"`) instead of a bare prompt line, matching the behavior actually verified below, and a `Known Limitations` section (`plan.md:334+`) documents the residual bare-prompt-only gap as an accepted limitation rather than leaving it unaddressed. Original finding preserved below.

- **AC3's "no new production code needed, falls out of existing behavior" claim is empirically false for the literal scenario the plan specifies** (`plan.md:66-69`, AC3 given/when/then at `plan.md:175-180`, Task 1.1.1c at `plan.md:234-238`) — **Recommendation**: fix AC3's given/when/then and Task 1.1.1c's test fixture, and decide whether the underlying gap needs a small production fix or just documentation, before implementation starts.

  Verified by applying exactly the two production edits the plan specifies (Task 1.1.1a's regex change and Task 1.1.1b's summation generalization) in this worktree and running the real detector against the plan's own AC3 fixture:

  ```
  lines := []string{
      "✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running",
      "❯ ",
  }
  sd.DetectWithContextAndCountFromLines(lines)
  // → status=Waiting for Agent, count=2   (plan claims this should resolve to idle, count=0)
  ```

  This holds both **before and after** the planned fix (count is 1 pre-fix via the `monitors_still_running` fallback, 2 post-fix) — the fix does not change this outcome. Root cause: `MatchLines`' `.*` Ready catch-all now returns `StatusUnknown` for any line that matches nothing more specific (`session/detection/pattern_set.go:172-174`; confirmed dead-`StatusReady` behavior noted in `session/detection/detector_test.go:42`), and `detectFromLines` simply `continue`s past `StatusUnknown` lines (`session/detection/detector.go:617-619`) rather than treating a bare prompt as a terminal "idle" result. The backward scan keeps walking past the contentless `"❯ "` line and returns the *older* `"...still running"` line's `StatusWaitingForAgent` match immediately (`detector.go:648`, "specific match wins immediately").

  The everyday case is fine: adding a realistic idle marker line (`"  ? for shortcuts"`, present in every real Claude Code idle screen and in the existing precedent test `TestBug_AutoModeFooter_NoFooterLine_NotOverridden`, `bug_regression_test.go:1211-1228`) alongside the bare prompt **does** correctly resolve to `StatusIdle`, count `0` — verified with the same fixed code. But that is not what the plan's AC3 example describes ("a plain `'❯ '` prompt line as the most recent line," `plan.md:236`), and the precedent test it cites as a "mirror" is a weaker case (no earlier WaitingForAgent line exists at all in that fixture, so it can't exercise the override-masking behavior this AC actually needs).

  Practical risk if left as-is: this is a second, independent path to the exact "sticky indicator" failure mode `research/pitfalls.md` §4 warns about (that section only covers the caching variant) — if the idle-hint/footer line(s) ever fall outside the 4096-byte tail window (`StatusDetectionTailBytes`, `session/detection/detector.go:314`) relative to the older "still running" line (long wrapped lines, extra turn output, a future Claude Code idle-screen redesign), the "⏳ Waiting for N Tasks" chip would get stuck indefinitely with no code path to clear it — directly contradicting the feature's stated goal ("so a user doesn't mistake 'waiting on a background monitor' for 'session is idle/needs input'").

## Concerns

- [x] **RESOLVED** — Task 1.1.1c (`plan.md:268-274`) now adds an explicit documented-limitation test case for wrapped/split status lines, per the Known Limitations section. Original finding preserved below.
- [ ] **No regression test for wrapped/split status lines** — `detectWithContextFromString`/`MatchLines` matches one physical line at a time (`detector.go:616`). A narrow terminal pane could wrap `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"` across two physical lines (e.g. split right before `"1 shell"`), and neither `shells_still_running` nor `monitors_still_running` would match either fragment, silently reproducing the same undercounting bug this plan fixes, just triggered by pane width instead of comma-joining. Not mentioned in the plan or `research/pitfalls.md`. **Recommendation**: add a wrapped-line test case (even if the answer is "known limitation, no fix" — document it explicitly rather than leaving it unconsidered) alongside Task 1.1.1c's other cases.
- [x] **RESOLVED** — Task 1.1.1c (`plan.md:257-263`) now adds a case exercising the zero-count guard's fallthrough for a pattern other than `shells_still_running`. Original finding preserved below.
- **The zero-count guard's generalization in `matchWaitingForAgent` changes fallback behavior for all three `WaitingForAgent` patterns, not just `shells_still_running`, with no dedicated test for the other two** — Task 1.1.1b's `continue`-on-`total<=0` (mirroring `footerAgentCount`) means that if `waiting_for_background_agent` or `monitors_still_running` ever produced an unparseable/zero capture, the code now falls through to try the remaining patterns and, if none match, exits the `WaitingForAgent` group entirely (letting Success/Compacting/Active/Idle win instead) — previously it returned `ok=true, count=0` immediately. This is the correct, intended fix (matches the Pattern Decisions table, `plan.md:93`), but Task 1.1.1c's test list (`plan.md:227-233`) only covers the combined/singular/plural/monitor-only shapes, not a zero/malformed-capture case for the other two patterns. **Recommendation**: add one small case exercising this fallthrough for a pattern other than `shells_still_running`, rather than relying on symmetry with the already-tested footer-form zero-count behavior (`TestBug_AutoModeFooter_ZeroShells_NotOverridden`), which is a separate code path (`applyFooterIdleOverride`/`footerAgentCount` in `detector.go`, not `matchWaitingForAgent` in `pattern_set.go`).

## Minors

- Plan's "Verification of Current State" leans on a Python `re` reproduction (`plan.md:38-42`) as evidence the current regex fails, deferring the actual Go/RE2 run to the final task (1.1.1d). It happened to be right here, and RE2 vs. PCRE semantic differences don't bite this particular pattern shape, but a direct Go probe during planning (as done in this review) would have surfaced the AC3 gap above earlier, before task breakdown/time estimates were finalized.
- All cited precedent commits (`7335c2d76`, `974ead4f0`, `13b600964`, PR #678) and file:line citations spot-checked in this review (`session/detection/binaries/claude.go:342-381`, `pattern_set.go:92-109`, `detector.go:19-76,596-661`, `instance_status.go:22-24,137`, `instance_adapter.go:252-256`, `SubStatusChip.tsx:21,34-61`) were accurate — no fabricated references found.
- Scope is otherwise tightly bounded to the stated bug (no drift found); ADR-001's rejection of a new `monitorwait/` package is well-supported by the codebase's existing recompute-on-read design and checks out against `research/pitfalls.md` §4's caching-staleness warning.
