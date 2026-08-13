# Architecture Review: subagent-spawn-tracking
**Date**: 2026-08-01
**Verdict**: CONCERNS

## Constitution Violations
- No constitution file found (`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo).

## Blockers
None. The plan's core design decisions were verified directly against the current source
(`session/detection/detector.go`, `pattern_set.go`, `terminal_detector.go`, `session/claude_controller.go`,
`session/instance_status.go`, `proto/session/v1/types.proto`, `server/adapters/instance_adapter.go`,
`server/services/event_converter.go`, `web-app/src/components/sessions/SubStatusChip.tsx` and
`SessionRow.tsx`) and hold up:

- **Additive sibling method instead of interface change** (Pattern Decisions, Step 0.5 Option A):
  confirmed `TerminalDetector` (`session/detection/terminal_detector.go:10-11`) pins
  `DetectWithContextFromLines`/`DetectFromLines`, and `ClaudeController.statusDetector` is a concrete
  `atomic.Pointer[detection.StatusDetector]`, not the interface — so the new
  `DetectWithContextAndCountFromLines` method is callable directly with zero interface churn. Correct OCP
  application; avoids rippling into `review_queue_determiner.go` and the direct call sites in
  `asterism_test.go`, `detector_test.go`, `shared_detector_test.go`, `terminal_detector_test.go`, and
  `bug_regression_test.go` (5 files, not the plan's stated 4 — see Nitpicks).
- **Cache-coherence fix (ADR-001)**: verified `GetCurrentStatus` (`claude_controller.go:631-711`) and
  `GetStatusAndIdleInfo` (`claude_controller.go:955-1060`) share one `atomic.Pointer[statusCacheEntry]`
  keyed by tail hash, and confirmed the naive "only the method that needs it writes it" version would
  produce exactly the stale-zero-overwrites-real-count race the ADR describes. The paired-write fix
  (both methods populate `subagentCount` on every `Store()`) correctly closes that illegal state.
- **Direct positional return over new struct/interface**: consistent with the codebase's existing
  multi-value-tuple idiom throughout `session/detection` and with
  `.claude/rules/interface-pollution-checklist.md` item 5 (unjustified generic). No speculative
  abstraction introduced.
- **Proto field placement**: `subagent_count = 72` sits beside `detected_status = 68` /
  `detected_context = 69` directly on the top-level `Session` message — verified these existing peer
  fields also live on `Session` (not nested in `ClaudeSession`), so the new field's aggregate placement
  matches its closest existing analogs exactly. Field number `72` is genuinely free (`61` is the only
  gap below `71`/`workspace_key`, confirmed via `grep -noE "= [0-9]+;"`).
- **Build-vs-buy conformance**: Task 1.2.1.1's guarded `FindStringSubmatch` → `strconv.Atoi` idiom
  matches `research/build-vs-buy.md`'s recommendation and the precedent at
  `session/git/worktree_git.go:372-375` exactly.
- **No new concurrency structure**: `InstanceStatusInfo.SubagentCount` (Epic 2.4) is a field on a
  struct returned by value from `GetStatus()`, not shared mutable state — correctly does not
  reintroduce a mutex or touch the `xsync.Map` in `InstanceStatusManager`.

## Concerns

- [ ] **Story 1.3.1 / Task 1.3.1.3 — internal line-reference contradiction.** The task lists "the
  raw-bytes variant (line 298)" as one of "4 call sites of `detectFromText` that don't yet need the
  count" and should discard the new 4th return value with `_`. Verified via
  `grep -n "detectFromText(" session/detection/detector.go`: line 298 is inside
  `detectWithContextFromString`, which is the exact call site **Task 1.3.1.2** already modifies to
  *capture and propagate* count (not discard it) as part of extending that method's own signature to
  3 values. The two tasks give contradictory instructions for the same line. There are only 3
  remaining call sites that genuinely discard count: `Detect` (275), `DetectWithContext` (284), and
  `DetectForProgram`'s fallback (756) — not 4. An implementation subagent following Task 1.3.1.3
  literally could re-touch line 298 and break Task 1.3.1.2's edit, or stall on the mismatch.
  **Remediation:** edit Task 1.3.1.3 in `plan.md` to list exactly 3 call sites (275, 284, 756) and
  remove the line-298 reference before implementation starts.

- [ ] **Story 1.3.3 — count-propagation test coverage gap in the two most bug-prone branches of
  `detectFromLines`.** Risk Control item 2 correctly identifies threading `bestCount` through
  `detectFromLines` (`detector.go:768-843`) as "the highest-line-count, most bug-prone task in the
  plan," and mitigates it by requiring the *existing* status/desc tests to keep passing unmodified.
  But those existing tests only assert `status`/`desc` — never `count` — so a wrong `bestCount` thread
  through the two structurally distinct branches not covered by the two new tests would ship silently:
  (1) the CR-segment (`\r`-split) branch (`detector.go:775-806`), and (2) the
  "`bestStatus == StatusExecuting` candidate overridden by a later higher-urgency status" switch
  (`detector.go:830-839`). Task 1.3.3.1/1.3.3.2 only cover a single non-CR WaitingForAgent line and a
  two-line non-CR collision. **Remediation:** add at least one test with a `\r`-containing line whose
  final segment matches a WaitingForAgent pattern (asserting count from that segment), and one test
  where an earlier line yields a `StatusExecuting` candidate that gets overridden by a later
  (higher, in reverse-scan order) `StatusWaitingForAgent` line via the switch at line 832 — asserting
  the returned count comes from the overriding line, not left over from the candidate.

## Nitpicks

- Step 0.5 says `DetectWithContextFromLines` "is untouched" — its exported signature and observable
  behavior are unchanged, but its body does change (Task 1.3.2.3: from a direct passthrough of
  `detectFromLines`'s return to `s, desc, _ := sd.detectFromLines(lines); return s, desc`). Wording
  precision only; not a design defect.
- Plan states `DetectWithContextFromLines` is used by "`review_queue_determiner.go` plus four test
  files." Verified 5 test files have direct call sites (`asterism_test.go`, `detector_test.go`,
  `shared_detector_test.go`, `terminal_detector_test.go` (fake impl), `bug_regression_test.go`). Doesn't
  change the plan's conclusion — if anything it strengthens the case for not touching the interface —
  but the count is understated in the plan text.
- Long-term API surface: `StatusDetector` will carry two overlapping exported methods
  (`DetectWithContextFromLines` and `DetectWithContextAndCountFromLines`) indefinitely once this ships.
  Reasonable additive-API tradeoff for now (same shape as stdlib's `context.WithTimeout` /
  `WithDeadline`), but worth a follow-up note to migrate `review_queue_determiner.go` onto the
  count-aware method and retire the narrower one if a second caller ever needs the count.
