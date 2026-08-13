# Adversarial Review: subagent-spawn-tracking
**Date**: 2026-08-01
**Verdict**: CONCERNS

## Summary

Read `plan.md`, `requirements.md`, `research/features.md`, and `ADR-001` in full, and
spot-verified the plan's factual claims directly against the current repo (proto field
numbers, `MatchLines`/`detectFromLines`/`detectFromText` line numbers, `TerminalDetector`
interface contents, `statusCacheEntry` struct, `SubStatusChip.tsx` current source, and
`snapshot_test.go` contents). Every specific line-number/field-number/interface claim I
checked held up exactly as stated — this plan did real verification work, not narrative
assertion, and it shows. No blocker-level defects found. The concerns below are real but
none should stop implementation from starting; they should be resolved during
implementation or explicitly accepted.

## Blockers

(none)

## Concerns

- [ ] **Unguarded `m[1]` access in the `MatchLines` count-extraction snippet (Task
  1.2.1.1)** — the example code is `if m := regex.FindStringSubmatch(text); m != nil {
  ... strconv.Atoi(m[1]) ... }`. This assumes every regex in `waitingForAgentRegexes` has
  exactly one capturing group. That's true for the three patterns this story edits, but
  nothing enforces it going forward — a future added `WaitingForAgent` pattern without a
  capture group, or a typo that drops the `(...)` during Epic 1.1, would make `m != nil`
  true while `len(m) < 2`, causing an index-out-of-range panic in the hot detection path
  instead of a graceful `count = 0`. Recommend `if m != nil && len(m) > 1` before indexing
  `m[1]`. Low real-world risk (Task 1.2.1.2's tests would very likely surface this
  immediately via a panic, well before Phase 6), but it's a defensive-coding gap in code
  the plan hands to the implementer verbatim.

- [ ] **Proto field number 72 is verified correct *right now*, but the plan has no
  re-verification step at implementation time.** I confirmed via `grep` that the `Session`
  message's fields run 56–71 with only 61 skipped, and 72 is genuinely free as of this
  review — the plan's claim is accurate today. But this exact field is the one the
  research doc already got wrong once (guessed 57, was stale by planning time), which is
  a demonstrated pattern of drift on this specific number. If implementation happens in a
  later session (per this repo's SDD workflow, planning and implementation are frequently
  separate sessions/branches), another PR could claim 72 in the interim. A collision would
  likely fail loudly at `make proto-gen` (protoc/buf reject duplicate field numbers in one
  message) rather than silently corrupting data, so this is not catastrophic — but add a
  one-line re-grep step to Task 3.1.1.1 ("re-run `grep -noE '= [0-9]+;' proto/session/v1/types.proto`
  scoped to `Session` immediately before adding the field") to close the gap cheaply.

- [ ] **AC #3's "no badge when count is 0" is not actually implementable in the chosen
  design, and this gap isn't flagged.** I read the current `SubStatusChip.tsx`: the
  `WAITING_FOR_AGENT` case already renders an unconditional `<span>` chip
  (`⏳ Waiting for Agents`) whenever `subStatus === WAITING_FOR_AGENT`, independent of any
  count — this is pre-existing behavior the plan doesn't touch. The plan's Task 5.1.1.2
  only changes the *text inside* that always-rendered chip; it never suppresses the chip
  itself at `subagentCount === 0`. So literally, "no badge when count is 0" (one of AC #3's
  two stated gate conditions) can't be demonstrated true by this design — the badge (the
  chip) was already showing before this feature and continues to. The plan's Unresolved
  Question #6 flags a *copy* deviation from requirements' illustrative "⊕ N tasks" glyph,
  but doesn't flag this deeper semantic deviation. In practice this is low-impact — all
  three `WaitingForAgent` regexes require `\d+`, so `subagentCount` should be ≥1 whenever
  `subStatus === WAITING_FOR_AGENT` under normal detection, making the "count is 0 but
  status is WAITING_FOR_AGENT" case rare/degenerate — but the plan should say this
  explicitly rather than silently reinterpreting the AC, so it can get a conscious
  product/requirements sign-off rather than being discovered later as "AC #3 not actually
  literally met."

## Minors

- Task 1.2.1.1's "~5 min" estimate is tight: I read the live `MatchLines` function and
  counted 13 total `return` statements (Error, TestsFailing, NeedsApproval, InputRequired,
  readline_typing, the target WaitingForAgent loop, Success, Active, Processing,
  screen_overwrite fallback, Idle, Ready catch-all, final fallback) — 12 non-target sites
  each need a trailing `, 0` appended, on top of the target-loop rewrite and the new
  `strconv` import. Not a correctness risk (Go's compiler enforces exact return arity, so
  any missed site fails the build immediately), just a task-sizing note.
- The "sibling method" drift risk (bug fixed in `DetectWithContextFromLines` but not
  `DetectWithContextAndCountFromLines`, or vice versa) is structurally close to zero by
  design — both public wrappers delegate to the same private `detectFromLines`, confirmed
  by reading the real function boundaries (`detectFromLines` at line 768, `DetectFromLines`
  at 854, `DetectWithContextFromLines` at 867, all in `session/detection/detector.go`) — but
  the plan never states this explicitly as a mitigation. Worth a one-line callout for future
  maintainers who might otherwise assume the two public methods are independent
  implementations that need to be kept in sync by hand.
- The cache-coherence test (Task 2.3.1.1) is sequential (call A, then call B, assert
  matching values), not a genuine concurrent-goroutine race test. This fully covers
  ADR-001's actual described failure mode (a paired-write omission, not a true data race),
  so it's adequate — just noting it doesn't add new coverage for the underlying
  `atomic.Pointer[statusCacheEntry]`'s pre-existing "last Store() wins" behavior under
  true concurrent callers, which predates this feature and isn't this plan's job to fix.
- ADR-001's "Consequences" section only names one rejected alternative (the naive
  single-writer implementation). A fuller ADR would also record why fully decoupling the
  two methods' caches (separate cache slots instead of shared coherence) was rejected, if
  it was considered — as written it's not clear whether that alternative was evaluated or
  simply not considered.
- I independently checked `session/detection/snapshot_test.go` on the hypothesis that it
  might be a full-message golden-file snapshot sensitive to the literal regex source
  strings (which change in Epic 1.1 by gaining capture groups) — it is not. It's a
  status-enum-only fixture test (`expected DetectedStatus` per captured terminal-output
  file), unaffected by adding non-matching-behavior-changing capture groups. The plan's
  claim that it "passes unmodified" is well-founded; flagging this only because I went in
  skeptical of it and it checked out.
