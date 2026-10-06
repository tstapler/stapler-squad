# Adversarial Review: backlog-link-error-consistency

**Date**: 2026-08-16
**Verdict**: CONCERNS

## Blockers

None. Line numbers, error-sentinel wrapping (`session.ErrNotFound`), and the root-cause
claim were all re-verified directly against `server/mcp/tools_backlog.go` and
`session/storage_backlog.go` (see notes below) and hold up. Scope is tight — no
non-goal creep, no new storage primitives, no new dependencies, no schema change. The
TOCTOU and information-disclosure risks are explicitly analyzed and the "accept, don't
transact" call is reasonable given `SetMaxOpenConns(1)` and this being a single-user,
low-QPS tool surface.

## Concerns

- [ ] **Plan overclaims existing regression coverage for the role-mismatch branch on 2 of
  5 handlers, and the new test doesn't close the gap either.** `pitfalls.md` (lines 79-84)
  asserts "each handler already has its own `Test<Handler>_RejectsWhenSessionNotLinked`-style
  test," citing `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork` as one example
  — but that test covers the *role* check, not the *link* check, as the doc's own parenthetical
  admits. Verified directly: `grep -n "^func Test" server/mcp/tools_backlog_test.go` shows
  **zero** tests for `submitReviewVerdict` beyond the `FEATURE_DISABLED` case in
  `feature_flag_test.go` (no happy path, no not-linked case, no role-mismatch case anywhere in
  the repo — confirmed via a second grep across all `*_test.go` files), and **zero**
  not-linked/role-mismatch tests for `submitTriageResult`. Only `reportProgress` and
  `requestReview` have real pre-existing not-linked tests; only `reportPRCreated` has a real
  pre-existing role-mismatch test. Despite this, Story 1.2.3's and 1.2.5's acceptance criteria
  say the role-mismatch behavior is "unchanged from today" / must keep passing
  "existing...tests," and Story 2.1.1's AC claims "Existing role-mismatch tests are confirmed
  unaffected" — there is no baseline for `submitReviewVerdict`/`submitTriageResult` to be
  "confirmed unaffected" against. The new Epic 2.1 table-driven test (Task 2.1.1a/b) only
  covers the not-linked and item-not-found cases, not role-mismatch, so after this PR ships
  `submitReviewVerdict`'s and `submitTriageResult`'s role-mismatch `PERMISSION_DENIED` paths
  remain permanently untested — exactly the kind of silent-regression risk `resolveItemLink`'s
  neighboring refactor (Tasks 1.2.3a, 1.2.5a) could introduce without anyone noticing.
  — **Recommendation**: correct the plan's "unchanged from today" language (it's an
  aspiration, not a verified baseline) and add one minimal role-mismatch case per tool to
  Epic 2.1's table (mirroring `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork`)
  for `submitReviewVerdict` and `submitTriageResult` — low incremental cost, and directly on
  point for a fix whose whole premise is "self-diagnosis via correct, trustworthy error codes."

- [ ] **The pitfalls.md-recommended structural guard against a "fix 4 of 5" miss isn't in the
  plan's task list.** `pitfalls.md` §3 explicitly recommends pairing the table-driven test with
  "a grep-based structural guard... asserting no remaining call site matches the pattern
  `errors.Is(linkErr, session.ErrNotFound)` immediately followed by an unconditional
  `ErrPermissionDenied`... this catches the '5th call site missed' case even if the
  hand-written table test itself has a gap." The plan (Tasks 2.1.1a-c, 4.1.1a) never adds this
  — it relies solely on Stories 1.2.1–1.2.5 being individually done correctly and the
  table-driven test happening to enumerate all 5. If a 6th mutating tool is added later (or one
  of the 5 stories is silently skipped during implementation), nothing catches it structurally.
  — **Recommendation**: add a one-line grep check (e.g. `! grep -n
  'errors.Is(linkErr, session.ErrNotFound)' server/mcp/tools_backlog.go` returns no matches) to
  Task 2.1.1c or 4.1.1a's verification steps — cheap insurance the research already flagged.

## Minors

- Task 2.1.1a's illustrative table code mixes role literals inconsistently
  (`session.SessionRoleWork` constant for `report_pr_created` vs. raw strings `"work"`,
  `"review"`, `"triage"` for the other four) — cosmetic, won't affect correctness since Go
  string constants and literals compare equal, but worth normalizing for readability when
  transcribed into the real test file.
- AC4's satisfaction path (Task 3.1.1a) is a manual "paste an excerpt into the PR description"
  step with no gate ensuring the three open items in "Unresolved Questions" (the
  `DeleteBacklogItem` guard, a real `report_blocked` tool, and filing the timeout follow-up)
  actually get triaged by a human rather than silently dropped once the PR merges — acceptable
  given requirements.md's own "even if the fix is out of scope... the finding must be written
  up" bar is met, but worth a reminder in the PR body to explicitly ping an operator rather than
  relying on the artifact being read later.
- The improved `PERMISSION_DENIED` remediation text says "wait a few seconds and retry" for the
  startup-race window (Domain Glossary) without a concrete duration or backoff guidance — fine
  as a strict improvement over today's message, but an LLM agent session may not reliably
  interpret "a few seconds" consistently; a specific number (e.g. "5-10 seconds") would be more
  actionable if this message is revisited later.
