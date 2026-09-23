# Adversarial Review: worktree-selfheal-test-flake

**Date**: 2026-08-22
**Verdict**: CONCERNS

## Blockers
- [x] `plan.md`'s Pattern Decisions table cited `research/architecture.md` §3(c) as supporting
  evidence for the chosen Ground-Truth Re-Query fix. §3(c) actually recommends the opposite: a
  CI-scoped `runGitCommand` timeout override, explicitly stating the fix should **not** touch
  the self-heal string-matching logic, and §3(a) separately argues loosening that matching would
  be unsafe. Left unaddressed, a future reader comparing plan.md against its own cited research
  would find an unexplained, misattributed contradiction on the single most consequential design
  decision in the plan. — **Patched**: corrected the false citation and added a "Reconciliation
  with `research/architecture.md` §3(c)" section to `plan.md` (after the Pattern Decisions table)
  explaining why Ground-Truth Re-Query supersedes architecture.md's recommendation — it resolves
  §3(a)'s stated safety objection (no longer inferring outcome from error text) rather than
  triggering it, and architecture.md never evaluated Re-Query as an option (it only considered
  hardening the string match, locking the test, or widening the timeout).

## Concerns
- [ ] Task 1.1.1b (`setupNewWorktree`'s `branchRefExists` retry loop) reuses the single
  already-open `*git.Repository` handle across all retry attempts, unlike the `getHeadCommitSHA`
  precedent it's explicitly modeled on (`util.go:310-329`), which re-opens the repo fresh every
  attempt specifically because go-git was observed to unreliably resolve state immediately after
  a concurrent git-CLI operation. `branchRefExists`'s single loose-ref lookup is less likely to
  hit that same issue, but the plan doesn't state why reusing the handle is safe here despite
  citing the reopen-per-attempt precedent for the retry *idiom*. Recommendation: add a one-line
  justification in Task 1.1.1b, or switch to re-opening the repo fresh per attempt for parity
  with the cited precedent.
- [ ] Neither retry loop (Task 1.1.1b's `branchRefExists` loop, or Task 1.1.2a's now-added
  `worktree list --porcelain` loop) specifies behavior when the re-query call itself errors
  transiently — a `worktree list --porcelain` subprocess is bound by the same 30s
  `runGitCommand` timeout and could itself be killed under the same CI load this bug is about.
  Recommendation: state explicitly in both tasks whether a transient re-query error consumes a
  retry attempt (and the loop keeps going) or aborts the loop immediately, falling straight to
  the hard error.
- [ ] Pre-mortem Failure #2 (P2, `pre-mortem.md`) — Story 2.1.1/2.1.2's stress repro runs
  `stress` against the isolated `worktree_ops.test` binary with `-test.run=<single test>`, but
  the requirements doc's own root-cause framing says the flake fired under "full-suite CI load"
  (contention from ~84 other `t.Parallel()` sites in the package, per `pitfalls.md` §1), not
  amplification of the isolated test alone. This was already identified and correctly triaged as
  P2/non-blocking by the pre-mortem pass, but it still means a clean "did not reproduce" result
  from Task 2.1.1a/2.1.2a as currently specified is weaker evidence for AC-1 than the plan's
  framing ("documented either way") implies. Recommendation (non-blocking, can land as a
  follow-up or a same-session tweak): run the stress harness against the whole `session/git`
  package rather than a single `-test.run` filter, and describe an isolated-binary clean result
  as inconclusive rather than as AC-1 closure.

## Minors
- Per-task time estimates (`~3 min`, `~5 min`, `~8 min`) are optimistic for concurrency-sensitive
  code plus new deterministic and stress-based tests; unlikely to derail the work but worth the
  implementer not treating them as a rushing cue, especially given pre-mortem Failure #1's
  requirement to size the retry budget against empirical stress-repro data rather than copying a
  precedent unexamined.
- Task 1.1.1b's and Task 1.1.2a's hard-fail fallback paths silently discard the re-query's own
  error (e.g. a `branchRefExists` or `worktree list` failure) rather than wrapping/joining it
  alongside the original `worktree add` error. Minor debuggability loss for whoever triages a
  future hard failure here, not a correctness risk.

## Notes for the record

This plan already went through one rigorous pre-mortem pass (`pre-mortem.md`, 2026-08-22) that
independently found and patched the most significant structural gap a reviewer would otherwise
flag here: `setupFromExistingBranch`'s Ground-Truth Re-Query (Task 1.1.2a) originally had no
bounded retry, unlike `setupNewWorktree`'s (Task 1.1.1b), which would have left the second
self-heal layer exactly as timing-fragile as the string-matching it was meant to replace. That
finding (pre-mortem Failure #4, P1) and the retry-sizing finding (Failure #1, P1) were both
already patched into `plan.md` before this review started — confirmed by reading the current
`plan.md`, which contains "Pre-mortem addendum" callouts in Tasks 1.1.1a and 1.1.2a. This review
did not re-flag either as a fresh blocker; it instead surfaces the one gap the prior passes
missed (the architecture.md §3(c) citation/contradiction) and three lower-severity items for the
implementer's awareness.
