# Implementation Plan: monitor-waiting-indicator

**Feature**: Fix undercounting when Claude Code's turn-completion status line reports both
outstanding shells and monitors in one comma-joined phrase ("N shell, M monitor still running"),
so the existing "⏳ Waiting for N Tasks" indicator shows the correct combined count.
**Date**: 2026-09-09
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-extend-existing-detector-not-new-package.md) — extend the
existing detector pattern table instead of building a new `monitorwait/` package.

---

## Scope Note

This plan is a **targeted bug fix in an existing detector**, not a new feature build. The short
footer form of this indicator (`"⏵⏵ auto mode on · N shells, M monitors · ← for agents"`) already
works end-to-end — detection, count propagation, and UI chip — verified below by file:line. The
only confirmed gap is the mid-turn/turn-completion long form
(`"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"`), which silently drops
the shell count today. See ADR-001 for why this is fixed in place rather than as a new package.

## Verification of Current State (before planning further)

Read directly, not taken from research summaries verbatim:

- `session/detection/binaries/claude.go:361-371` — `shells_still_running` pattern:
  `` Pattern: `(\d+)\s+shells?\s+(?:still\s+)?running` ``. Requires "running" immediately after the
  shell count/word.
- `session/detection/binaries/claude.go:373-381` — `monitors_still_running` pattern:
  `` Pattern: `(\d+)\s+monitors?\s+still\s+running` ``.
- `session/detection/binaries/claude.go:342-348` — a comment block claims all three
  `WaitingForAgent` patterns are "confirmed dead against Claude Code's current CLI output." This is
  now contradicted for `shells_still_running`/`monitors_still_running` by the backlog's own captured
  real output; `waiting_for_background_agent` remains unverified.
- `session/detection/pattern_set.go:92-109` (`matchWaitingForAgent`) — for the real line `"✻
  Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"`: `shells_still_running`
  does not match (comma, not "running", follows "1 shell"), so the loop falls through to
  `monitors_still_running`, which matches "1 monitor still running" alone and returns `count=1`,
  discarding the shell. Confirmed by direct regex testing (Python `re`, equivalent syntax) during
  planning — `shells_still_running`'s current pattern captures `('1', None)` style groups only when
  no comma-joined monitor phrase follows; RE2/Go `regexp.FindStringSubmatch` behaves identically for
  this pattern shape.
- `session/detection/pattern_set.go:92-109` (`matchWaitingForAgent`) also only ever reads capture
  group `m[1]` — it has no mechanism today to sum a second capture group even if one existed.
- `session/detection/detector.go:19-76` — `autoModeFooterRegex` +`footerAgentCount` +
  `applyFooterIdleOverride` already parse the short footer form's comma-joined
  `"N shells, M monitors"` correctly by summing both capture groups — this is the pattern the fix
  below mirrors.
- `session/instance_status.go:22-24,137` — `InstanceStatusInfo.SubagentCount`, set from
  `ClaudeController.GetStatusAndIdleInfo` (`session/claude_controller.go:1105`), independent of
  control-mode vs. polling-mode: both read through the same `ptyAccess.GetRecentHash` /
  `statusCache` path (`claude_controller.go:1124-1138`), so this satisfies AC4 already — no
  mode-specific code exists to break or extend.
- `server/adapters/instance_adapter.go:252-256` — `SubagentCount` mapped onto the proto
  unconditionally.
- `web-app/src/components/sessions/SubStatusChip.tsx:21,34-61` — `SubStatusChip` already renders
  `"⏳ Waiting for {subagentCount} Tasks"` for `SubStatus.WAITING_FOR_AGENT`, singular/plural handled.
- `session/detection/bug_regression_test.go:605-654` (`TestBug_ShellsStillRunning`) and
  `:1131-1163` (`TestBug_AutoModeFooter_WaitingWhenIdleWithBackgroundShells`) — existing regression
  coverage for the two forms, neither of which exercises the comma-joined long form.
- Precedent: `git log --all --oneline --grep="auto mode on"` confirms commit `7335c2d76`
  (`fix(detection): recognize current auto-mode footer for WaitingForAgent`, PR #678) plus two
  follow-ups (`974ead4f0`, `13b600964`) fixed the short-form gap the same way this plan fixes the
  long-form gap: extending the existing pattern table, not adding a package.

**Conclusion**: AC2 and AC4 are already satisfied by existing code; AC1/AC5/AC6 require the
targeted regex + summation fix below plus new tests; AC3 requires no new production code, but only
for the realistic case — verified directly (not assumed) against the fixed detector during
adversarial review using a fixture that includes a real idle-screen marker line (`"  ? for
shortcuts"`) alongside the bare prompt. A bare `"❯ "` prompt line with no other content is
insufficient by itself: `MatchLines`' catch-all returns `StatusUnknown` for it
(`session/detection/pattern_set.go:172-174`), `detectFromLines` skips `StatusUnknown` lines
(`detector.go:617-619`), and the backward scan then matches the older "...still running" line
instead of clearing. See the Known Limitations section for the resulting residual risk. AC3 is
verified by a new test using the realistic fixture, not new production code.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `WaitingForAgent` status (`DetectedStatus`) | Existing `session/detection/detector.go:93` enum value meaning Claude's turn ended (or the footer bar is showing) but background shells/monitors are still running. | Reused unchanged — this plan adds no new status value. |
| `shells_still_running` pattern | Existing named `StatusPattern` (`binaries/claude.go:361`) matching the turn-completion line's "N shell(s), optionally followed by M monitor(s), still running" suffix. | Regex extended in this plan to optionally capture a comma-joined monitor count; name and position in the `WaitingForAgent` slice unchanged. |
| `monitors_still_running` pattern | Existing named `StatusPattern` (`binaries/claude.go:373`) matching a monitor-only "N monitor(s) still running" suffix. | Unchanged — remains the fallback for lines with no shell mention. |
| `matchWaitingForAgent` | Existing method (`pattern_set.go:92`) that scans `WaitingForAgent` regexes in order and extracts a subagent/shell/monitor count from the first match. | Generalized in this plan to sum every non-empty captured digit group, not just group 1 — mirrors `footerAgentCount`. |
| combined count | The sum of outstanding shells + outstanding monitors reported on one status line. | Same concept `footerAgentCount` already computes for the footer form; no new type — stays a plain `int`, consistent with `SubagentCount`. |
| `SubagentCount` | Existing field (`session/instance_status.go:24`) — the combined count, 0 unless `ClaudeStatus == StatusWaitingForAgent`. | Reused unchanged. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Long-form shell+monitor detection | Extend existing regex-based pattern table (`binaries/claude.go`) | This codebase's own prior art (PR #678) | New standalone `session/detection/monitorwait/` package mirroring `ratelimit/` (`research/architecture.md`) | No lifecycle/recovery/timer state to manage (unlike rate-limit cooldowns) — the signal is a pure, stateless function of the current PTY tail recomputed on every call; a new package would still funnel back into the same `SubagentCount` field for no structural benefit. See ADR-001. |
| Count extraction across multiple capture groups | Generalize `matchWaitingForAgent` to sum all non-empty captured digit groups (mirrors `footerAgentCount`'s existing summation) | This file's own `footerAgentCount` (`detector.go:40-61`) | Special-case a second `m[2]` read only for `shells_still_running` | The generalized form is one code path that already handles every current and future 1- or 2-group `WaitingForAgent` pattern, instead of a pattern-specific branch that would need its own future one-off edit. |
| Combined count representation | Reuse plain `int` (`SubagentCount`) | Existing field, `type-driven-design` — newtype only where it prevents cross-entity confusion | New `ShellMonitorCount` value type | The count is never compared against or mixed with a different kind of count anywhere in the codebase (no cross-entity confusion risk to guard against) — `SubagentCount` already plays this role for the footer form; introducing a wrapper type here and not there would itself be an inconsistency. |
| Zero-count guard on the long-form match | Defensive `count <= 0 → no match` guard, mirroring `footerAgentCount`'s `total <= 0 → ok=false` (`detector.go:55-57`) | `research/pitfalls.md` §2 | No guard (rely on `n > 0` per-group check alone, as today) | Today's per-group `n > 0` check already prevents a "0 shells" group from contributing, but a hypothetical future pattern edit producing an all-zero match would silently return `ok=true, count=0` (StatusWaitingForAgent with a 0 count) instead of falling through — cheap to guard against explicitly. |
| UI indicator | No change — reuse `SubStatusChip.tsx`'s existing `WAITING_FOR_AGENT` case | Existing component | New badge/chip component | Already renders a distinct "⏳ Waiting for N Tasks" chip driven entirely by `SubagentCount`; the bug is upstream of the UI, so no UI code needs to change for AC2 to hold once the count is correct. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `session/detection/binaries/claude.go:342-348` | Comment block asserts all three `WaitingForAgent` patterns are "confirmed dead against Claude Code's current CLI output" — now factually wrong for two of the three patterns, per the backlog's own captured real output. | Extend as-is (correct the comment as part of this change) | This is a stale-documentation issue in a stable, narrowly-scoped pattern table, not a structural violation — correcting the comment inline with the regex fix is a one-line edit, not a refactor. Leaving it uncorrected would actively mislead the next person touching this file into thinking these patterns are unreachable. |
| `session/detection/pattern_set.go`'s `matchWaitingForAgent` | Reads only capture group `m[1]`; not a violation today (all three existing patterns have ≤1 group before this change) but would silently ignore a second group if one were ever added — which this plan does. | Extend as-is (generalize the summation) | Small, localized change to an already-small, single-purpose function; no broader coupling or size problem to isolate behind a seam. |

No other hotspots from `research/architecture.md` are touched by this change.

---

## Observability Plan
- **Logs**: No new log lines needed for the fix itself — `ClaudeController`'s existing status-change
  debug log (`claude_controller.go:1090-1097`, fields `status`/`desc`/`subagent_count`) already
  includes the corrected count once the fix lands, with no code change required at that call site.
  Per `pre-mortem.md` #1/#2 (P2): add a debug-log canary mirroring the existing `compactingCanary`
  idiom (`detector.go:349-364`) that fires when a line matches a `WaitingForAgent` pattern but also
  contains additional unmatched `\d+\s+(?:shells?|monitors?)` tokens, or matches the turn-completion
  marker + "running" but no `WaitingForAgent` pattern at all — so the *next* Claude Code CLI wording
  drift (this project's fourth, per the PR #678 precedent chain) surfaces via logs instead of a
  silent undercount or a user bug report. See Task 1.1.1e below.
- **Metrics**: No new operation exceeds 100ms (this is a regex match on an already-in-memory string);
  no new metric required.
- **Alerts**: No new alerts required.

## Risk Control
- **Feature flag**: Not gated — this is a bug fix to an existing, always-on detector (mirrors how
  PR #678's footer fix shipped ungated).
- **Rollback procedure**: Standard revert via PR close + revert commit.
- **Staged rollout**: Full rollout on merge.

## Unresolved Questions

None. Both research disagreement points (architecture approach; whether AC2 needs new UI work) are
resolved above (ADR-001; Pattern Decisions table) with direct codebase verification.

## Dependency Visualization

```
1.1.1a (fix regex + comment)
        │
        ▼
1.1.1b (generalize matchWaitingForAgent summation + zero-count guard)
        │
        ▼
1.1.1c (regression tests: combined/singular/plural/monitor-only/AC3-clears)
        │
        ▼
1.1.1d (run session/detection + full session package tests — AC6)
        │
        ▼
1.1.1e (wording-drift canary log + reversed-order regression test — pre-mortem P2 hedge)
        │
        ▼
1.2.1a (verify AC2/AC4 already hold — no code change, verification only)
        │
        ▼
1.2.1b (verify control-mode/polling-mode parity — no code change, verification only)
        │
        ▼
1.2.1c (verify status-grouping bucket placement — no code change, verification only)
```

---

## Phase 1: Fix Comma-Joined Shell+Monitor Undercounting

### Epic 1.1: Correctly Detect and Sum "N shell, M monitor still running"
**Goal**: Claude Code's turn-completion status line reporting both outstanding shells and monitors
in one comma-joined phrase is detected as `StatusWaitingForAgent` with the correct combined count,
matching the precision the short footer form already has.

#### Story 1.1.1: Extend the `shells_still_running` pattern and its count extraction
**As a** stapler-squad user with a Claude Code session running background shells and monitors,
**I want** the session's status indicator to reflect the true combined count of outstanding work,
**so that** I don't mistake "still waiting on a monitor" for "session is idle/needs my input."

**Acceptance Criteria**:

- AC1: The long form (`"N shell, M monitor(s) still running"`) and the existing short form
  (footer bar) are both detected and parsed into a combined count.
  - *Given* the raw line `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still
    running"` passed to `PatternSet.MatchLines`, *When* `MatchLines` runs the `WaitingForAgent`
    group, *Then* it returns `(StatusWaitingForAgent, "shells_still_running", <desc>, 2)` — 1 shell
    + 1 monitor summed, not just the 1 monitor detected today.
- AC2: The web UI shows a distinct visual indicator when a session has outstanding
  shells/monitors (already satisfied — verification only, no new code).
  - *Given* `InstanceStatusInfo{ClaudeStatus: StatusWaitingForAgent, SubagentCount: 2}` computed
    from the fixed detector, *When* it flows through `instance_adapter.go:256` to
    `SubStatusChip.tsx` as `subagentCount={2}`, *Then* the chip renders
    `"⏳ Waiting for 2 Tasks"` (`SubStatusChip.tsx:61`) — distinguishable from the plain Idle or
    NeedsApproval chips.
- AC3: The indicator clears once the session output no longer reports outstanding work.
  - *Given* scrollback where the line `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor
    still running"` is followed by the two more recent lines `"❯ "` and `"  ? for shortcuts"` (the
    realistic idle-screen footer hint present on every real Claude Code idle screen — a bare
    `"❯ "` prompt line with no other content is *not* sufficient on its own; see the Known
    Limitations entry below), *When* `detectFromLines` backward-scans from the most recent line,
    *Then* it resolves to `StatusIdle` (not `StatusWaitingForAgent`) and the returned count is `0`.
    Verified directly against the post-fix detector during adversarial review
    (`implementation/adversarial-review.md`), not assumed.
- AC4: Detection works in both tmux control mode and legacy polling mode (already satisfied —
  verification only, no new code).
  - *Given* `ClaudeController.GetStatusAndIdleInfo` is invoked regardless of
    `STAPLER_SQUAD_USE_CONTROL_MODE`, *When* it reads via `ptyAccess.GetRecentHash`
    (`claude_controller.go:1128`), *Then* the same regex/summation logic runs against the same PTY
    tail bytes in both modes — no mode-conditional code path exists to diverge.
- AC5: New detection logic has unit test coverage using representative captured-scrollback
  fixtures for both forms.
  - *Given* the table-driven cases added to `bug_regression_test.go` in Task 1.1.1c, *When*
    `go test ./session/detection/...` runs, *Then* all new cases pass and cover: combined counts,
    singular phrasing, plural phrasing, monitor-only (no regression), the AC3 clearing case, the
    zero-count guard's fallthrough for a non-`shells_still_running` pattern (CONCERN #2), and the
    documented wrapped/split-line limitation (CONCERN #1).
- AC6: No regression to existing status detection/tag-organization behavior for sessions without
  this footer format.
  - *Given* the full existing `session/detection/` test suite (including
    `TestBug_ShellsStillRunning`, `TestBug_AutoModeFooter_*`, and `TestPatternSet_MatchLines_*`),
    *When* `go test ./session/... -count=1` runs after the fix, *Then* every previously-passing
    test still passes unchanged.

**Files**: `session/detection/binaries/claude.go`, `session/detection/pattern_set.go`,
`session/detection/bug_regression_test.go`, `session/detection/pattern_set_test.go` (Task
1.1.1c), `session/detection/detector.go` (Task 1.1.1e)

##### Task 1.1.1a: Fix the `shells_still_running` regex and correct the stale comment (~5 min)
- In `session/detection/binaries/claude.go`, change the `shells_still_running` pattern
  (currently line 368) from `` `(\d+)\s+shells?\s+(?:still\s+)?running` `` to
  `` `(\d+)\s+shells?(?:,\s*(\d+)\s+monitors?)?\s+(?:still\s+)?running` `` — adds an optional
  comma-joined monitor capture group, mirroring `autoModeFooterRegex`'s shape
  (`detector.go:35`). Update the pattern's `Description` to mention the optional monitor suffix.
- Update the doc comment on the `shells_still_running` pattern (lines 360-367) to note it now
  also captures a trailing comma-joined monitor count.
- Split the shared "confirmed dead" comment block (lines 342-348): keep the caveat only on
  `waiting_for_background_agent` (still unverified against a live modern capture); remove/correct
  it for `shells_still_running` and `monitors_still_running`, citing the backlog's captured real
  line as confirmation these are reachable.
- Files: `session/detection/binaries/claude.go`

##### Task 1.1.1b: Generalize `matchWaitingForAgent` to sum all captured groups (~5 min)
- In `session/detection/pattern_set.go`'s `matchWaitingForAgent` (lines 92-109), replace the
  single `m[1]`-only read with a loop over `m[1:]` that sums every non-empty group parsing to a
  positive integer — mirrors `footerAgentCount`'s loop (`detector.go:46-54`).
- Add the zero-count guard from the Pattern Decisions table: after summing, if `count <= 0`,
  treat as no match for this pattern (`continue` the outer loop) instead of returning `ok=true`
  with a zero count.
- Update the function's doc comment to describe the multi-group summation.
- Files: `session/detection/pattern_set.go`

##### Task 1.1.1c: Add regression tests for the combined form and its clearing (~5 min)
- In `session/detection/bug_regression_test.go`, extend `TestBug_ShellsStillRunning`'s table (or
  add a sibling `TestBug_ShellsAndMonitorsStillRunning`) with cases:
  - `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"` → `StatusWaitingForAgent`, count `2`.
  - `"✻ Baked for 3m · 2 shells, 3 monitors still running"` → `StatusWaitingForAgent`, count `5`.
  - `"✻ Churned for 52s · 1 monitor still running"` (monitor-only, no shell mention) → unchanged,
    `StatusWaitingForAgent`, count `1` (regression guard for `monitors_still_running`'s existing
    behavior).
- Add a new test verifying AC3's clearing behavior: a `[]string` scrollback slice with the
  combined "still running" line followed by the two more recent lines `"❯ "` and
  `"  ? for shortcuts"` (the realistic idle-screen footer hint, not a bare prompt alone — a bare
  `"❯ "` with nothing else does *not* clear the indicator; see Known Limitations), asserting
  `sd.DetectWithContextAndCountFromLines(lines)` returns `StatusIdle` and count `0` — mirrors the
  shape of `TestBug_AutoModeFooter_NoFooterLine_NotOverridden` (`bug_regression_test.go:1211-1228`).
- Add a case exercising the zero-count guard's fallthrough (Task 1.1.1b, CONCERN #2 in
  `adversarial-review.md`) for a pattern other than `shells_still_running` — e.g. construct a line
  matching `monitors_still_running` with a non-numeric or zero capture (or call
  `matchWaitingForAgent` directly with a crafted match) and assert the group falls through to the
  next pattern / exits `WaitingForAgent` entirely (`ok=false`) rather than returning `ok=true,
  count=0`, per the same fallback contract already tested for `shells_still_running`/footer-form
  via `TestBug_AutoModeFooter_ZeroShells_NotOverridden`.
- Add a case in `pattern_set_test.go` alongside
  `TestPatternSet_MatchLines_should_returnCount_When_shellsStillRunningMatches` (line 63) for the
  comma-joined form, asserting `MatchLines` returns count `2` directly (not just via the full
  detector).
- Add a documented-limitation test case (or a comment block, if a passing test can't cleanly
  express a known gap) for wrapped/split status lines (CONCERN #1 in `adversarial-review.md`): a
  `[]string` fixture where `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still
  running"` is split across two physical lines (e.g. break right before `"1 shell"`), asserting
  the current (accepted) behavior — neither `shells_still_running` nor `monitors_still_running`
  matches either fragment, so the count undercounts/misses — rather than leaving this case
  unexercised. See the Known Limitations section for the disposition (not fixed in this plan).
- Files: `session/detection/bug_regression_test.go`, `session/detection/pattern_set_test.go`

##### Task 1.1.1d: Run the full regression suite and confirm no drift (~3 min)
- Run `go test ./session/detection/... -run TestBug -v` and confirm all `TestBug_*` cases pass,
  including the new ones from 1.1.1c.
- Run `go test ./session/... -count=1` and confirm no pre-existing test regresses (AC6).
- Files: none (verification only)

##### Task 1.1.1e: Add the wording-drift canary log and reversed-order regression test (~8 min)
Per `pre-mortem.md` #1/#2 (P2) — this project's fourth documented brush with Claude Code CLI
wording drift on this exact status line (PR #678 + 2 follow-ups + this plan), cheap to hedge now:
- In `session/detection/detector.go`, add a debug-log canary near the existing `compactingCanary`
  idiom (`detector.go:349-364`) that fires when a line either (a) matches a `WaitingForAgent`
  pattern but still contains an unmatched `\d+\s+(?:shells?|monitors?)` token after the match, or
  (b) contains the turn-completion marker + "running" but matched no `WaitingForAgent` pattern at
  all — surfacing a future wording change (e.g. reversed order `"N monitor, M shell still
  running"`) via logs instead of a silent undercount.
- Add a regression test case documenting today's known-undercount behavior for the reversed-order
  form (`"✻ ... 1 monitor, 1 shell still running"` → currently undercounts to `1`, not `2`) to
  `session/detection/bug_regression_test.go`, per pre-mortem finding #1 — pins the gap explicitly
  rather than leaving it only in "Out of Scope" prose.
- Files: `session/detection/detector.go`, `session/detection/bug_regression_test.go`

---

### Epic 1.2: Verify Downstream UI/Mode-Agnosticism Needs No Change
**Goal**: Confirm AC2 and AC4 hold end-to-end with the fixed count, without adding redundant code
or tests for paths already covered.

#### Story 1.2.1: Confirm AC2/AC4 are satisfied by existing code
**As a** planner closing out this feature, **I want** documented confirmation that the UI and
mode-agnosticism acceptance criteria are met by existing code, **so that** no speculative UI or
control-mode-specific work is added.

**Acceptance Criteria**:
- AC2 (distinct UI indicator) is satisfied by `SubStatusChip.tsx`'s existing `WAITING_FOR_AGENT`
  case with no code change.
  - *Given* the fix from Epic 1.1 producing the correct `SubagentCount`, *When* a reviewer traces
    `SubagentCount` from `instance_status.go:137` → `instance_adapter.go:256` →
    `SubStatusChip.tsx:34-61`, *Then* no gap or missing prop is found.
- AC4 (control mode / legacy polling parity) is satisfied because detection is PTY-content-based,
  not mode-based.
  - *Given* `session/claude_controller.go:1105-1138`, *When* a reviewer confirms there is no
    `STAPLER_SQUAD_USE_CONTROL_MODE`-conditional branch in `GetStatusAndIdleInfo` or its PTY-read
    path, *Then* the same fix applies unconditionally to both modes.

##### Task 1.2.1a: Trace and document the SubagentCount → UI path (~3 min)
- Re-read `session/instance_status.go:119-147`, `server/adapters/instance_adapter.go:252-256`,
  `web-app/src/components/sessions/SubStatusChip.tsx:21,34-61`, and the two mount points
  (`web-app/src/components/sessions/SessionCard.tsx:787-792`,
  `web-app/src/components/sessions/SessionRow.tsx:349-361`) to confirm the count prop reaches the
  chip unconditionally once `ClaudeStatus == StatusWaitingForAgent` in both card and row/list view
  (per `design/ux.md` Surface 2's card/row visibility-guard comparison). No code change expected;
  if a gap is found, file it as a new task before closing this story.
- Files: none (verification only)

##### Task 1.2.1b: Confirm control-mode/polling-mode parity (~2 min)
- Grep `session/claude_controller.go` for `STAPLER_SQUAD_USE_CONTROL_MODE` / control-mode
  branching near `GetStatusAndIdleInfo` and `ptyAccess`; confirm none exists that would exclude
  the fixed detector from either mode. No code change expected.
- Files: none (verification only)

##### Task 1.2.1c: Confirm status-grouping bucket placement (`design/ux.md` Surface 4) (~2 min)
- Re-read `web-app/src/lib/utils/deriveWorkingState.ts:12,19,34,49` to confirm
  `SubStatus.WAITING_FOR_AGENT` → `WorkingState.PROCESSING` and
  `DetectedStatus.WAITING_FOR_AGENT` → `WorkingState.ACTIVE` (neither maps to `WorkingState.IDLE`),
  so a session with this chip never lands in a Status-grouped view's "Idle" bucket. This closes the
  loop `research/ux.md` §2 raised as an open question; `design/ux.md` Surface 4 already verified it
  during the UX pass — this task is the corresponding plan-side trace so the check isn't only
  recorded in the design doc. No code change expected.
- Files: none (verification only)

---

## Known Limitations

Identified during adversarial review (`implementation/adversarial-review.md`); documented here
per that review's recommendation rather than left unconsidered. Neither is fixed by this plan.

- **Tail-window edge case can leave the indicator stuck (BLOCKER follow-up, accepted pre-existing
  limitation)**: `detectFromLines`'s backward scan only sees the last `StatusDetectionTailBytes`
  (4096) bytes of PTY output (`session/detection/detector.go:314`). If the idle-hint/footer
  line(s) that would otherwise clear the indicator (e.g. `"  ? for shortcuts"`) ever scroll outside
  that window while an older "...still running" line remains inside it — long wrapped lines, a lot
  of intervening turn output, or a future Claude Code idle-screen redesign — the "⏳ Waiting for N
  Tasks" chip has no code path to clear and would get stuck indefinitely. **Verified this is not
  newly introduced or worsened by this plan's fix**: the reviewer confirmed the same bare-`"❯ "`
  AC3 fixture produces `StatusWaitingForAgent` both before and after the planned regex/summation
  change (count 1 pre-fix via the `monitors_still_running` fallback, 2 post-fix) — the fix changes
  the count, not the underlying tail-window susceptibility. Every other pattern in this file that
  relies on a most-recent-line override (e.g. `applyFooterIdleOverride`'s footer-form idle
  detection) shares the identical tail-window exposure — it is a property of
  `detectFromLines`'s general backward-scan design, not something specific to the shells/monitors
  patterns this plan touches. **Disposition**: accepted, pre-existing limitation, not fixed here —
  defer a general fix (e.g. widening the tail window, or a dedicated "was a terminal/idle line ever
  seen at all" fallback) to a future ticket rather than scope-creeping it into this bug fix.
  Per adversarial-review's concern that "defer to a future ticket" was prose-only: no dedicated
  tracking ticket is filed as part of this change (this plan's own backlog item tracks only the
  comma-joined undercount fix, not this pre-existing tail-window property) — recorded here as the
  authoritative pointer for whoever picks up a general fix to `detectFromLines`'s backward scan.
- **Wrapped/split status lines are not detected (CONCERN #1, pre-existing, shared by every
  single-line pattern in this file)**: `MatchLines` matches one physical line at a time
  (`session/detection/detector.go:616`). If a narrow terminal pane wraps
  `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"` across two physical
  lines (e.g. splitting right before `"1 shell"`), neither `shells_still_running` nor
  `monitors_still_running` matches either fragment, silently undercounting — the same failure mode
  this plan fixes for the comma-joined case, just triggered by pane width instead of phrasing.
  This is not unique to the patterns this plan touches — every existing single-line regex pattern
  in `binaries/claude.go` (footer form included) has the identical limitation, since none of them
  do any line-joining/reflow before matching. **Disposition**: accepted, pre-existing limitation,
  not fixed here — a general fix (e.g. joining/reflowing wrapped lines before matching) is
  out of scope for this bug fix and would need its own design (how to detect a wrap vs. a
  genuine newline). Exercised by an explicit test case in Task 1.1.1c documenting the current
  (accepted) undercounting behavior, rather than left unconsidered.

## Out of Scope (per requirements.md and research/ux.md)

- Staleness/"stuck forever" handling for a long-running background shell/monitor (borrowing
  `backlog-stuck/stuckReason.ts`'s pattern) — unstated need, not required by any AC.
- A nav-level aggregate `WaitingForAgentNavBadge` across all sessions — unstated need, not
  required by any AC.
- Reversed-order captures ("N monitor, M shell still running") — no live evidence this format
  exists; add as a follow-up regex variant if ever observed.
