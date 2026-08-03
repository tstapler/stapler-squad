# Architecture Review: jest-ci
**Date**: 2026-08-02
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo
(verified: `ls docs/adr/` lists ADR-001 through ADR-026 plus numbered non-ADR
files, no ADR-000). No constitution to check against — N/A.

## Lens Summary

- **Lens 1 (structural integrity)**: SOLID/DDD are N/A for a CI-config task, correctly
  noted as such by the plan. Testability has one real gap — see Concern 4.
- **Lens 2 (type-level design)**: N/A, correctly not force-fit by the plan.
- **Lens 3 (pattern selection)**: The two real decisions (AC5 bespoke summary capture,
  AC4 in-file skip vs. flag) are both well-justified and consistent with
  `research/build-vs-buy.md` and `research/pitfalls.md`. The bespoke bash script in
  Task 2.2.1 is functionally correct where it matters most (exit-code propagation
  through the `tee`/`PIPESTATUS` pipe — verified empirically below) but has two
  smaller robustness gaps (Concerns 1–2) and one documentation overclaim (Concern 3).

## Verification performed

- Confirmed `docs/adr/ADR-000-architecture-constitution.md` does not exist.
- Read `.github/workflows/benchmark.yml:85-108` and `.github/workflows/lint.yml:1-70`
  directly to check the plan's precedent claims.
- Reproduced the exact `set +e` / `set -o pipefail` / `PIPESTATUS[0]` / `set -e` /
  `exit "$JEST_EXIT"` pattern from Task 2.2.1 in a standalone script run under
  GitHub Actions' documented default invocation
  (`bash --noprofile --norc -eo pipefail {0}`) with a deliberately failing command
  piped through `tee`. Result: exit code correctly propagated (captured `1`, script
  exited `1`) — **the core AC2/AC5 exit-code-preservation mechanism is sound.**
- Verified line numbers cited in Tasks 1.1.1/1.2.1 against the actual files:
  `BacklogEmptyState.test.tsx:154` (`it("component stays rendered when
  onCreateItem rejects"`) and `SessionDetail.embedded.test.tsx:106`/`:150`
  (the two `describe(...)` blocks) — all three match exactly.
- Counted tests: `SessionDetail.embedded.test.tsx`'s two describe blocks contain 5 +
  3 = 8 tests (matches Unresolved Questions §3's "8 across... two describe blocks").
  `BacklogEmptyState.test.tsx` contains **12** total `it(...)` tests, not 14 — see
  Nitpick below.

## Blockers

(none)

## Concerns

- [ ] **Task 2.2.1** — The `set -e` re-enabled after capturing `JEST_EXIT` covers the
  summary-writing block (`{ echo ...; tail ...; sed ...; } >> "$GITHUB_STEP_SUMMARY"`).
  If any command in that block fails for an unrelated reason (e.g. a transient
  runner issue writing to `$GITHUB_STEP_SUMMARY`, or `sed`/`tail` erroring on an
  unexpected `jest-output.log` state), the script aborts on *that* command's exit
  code before it ever reaches `exit "$JEST_EXIT"` — a passing Jest run (`JEST_EXIT=0`)
  could then report red for a reason that has nothing to do with test results, or a
  real Jest failure's exit code could get silently swapped for an unrelated one. The
  plan already spent real care getting the Jest-pipe exit code right; the same care
  wasn't applied to the block that follows it. **Remediation**: wrap the summary
  block so it can't override `JEST_EXIT`, e.g. `{ ...; } >> "$GITHUB_STEP_SUMMARY" ||
  true` before the final `exit "$JEST_EXIT"`, so the *only* thing that determines the
  step's exit code is the captured Jest result.

- [ ] **Task 2.2.1** — The ANSI-stripping regex `sed -E 's/\x1b\[[0-9;]*m//g'` only
  strips SGR (color) escape sequences, which end in `m`. It will not remove other CSI
  sequences some terminal-aware tooling can still emit even in `--ci` mode (e.g.
  cursor-visibility toggles `\x1b[?25l`/`\x1b[?25h`, erase-line `\x1b[2K`, which end in
  `l`/`h`/`K` respectively). If any of those appear in the captured Jest output, they
  render as raw escape bytes inside the Step Summary's fenced code block instead of
  being invisible. **Remediation**: broaden the pattern to strip all CSI sequences,
  e.g. `sed -E 's/\x1b\[[0-9;?]*[a-zA-Z]//g'`.

- [ ] **Pattern Decisions section** — The claim "Matches existing `benchmark.yml:100-101`
  precedent exactly" overstates the match. Read directly: `benchmark.yml:95-104`'s
  comparison step uses `benchstat ... > benchstat-tier1.txt 2>&1 || true` (the command's
  exit code is explicitly discarded and never gates the job) and then unconditionally
  appends the file to `$GITHUB_STEP_SUMMARY` — there is no `tee`/`PIPESTATUS`/`set -e`
  exit-code-preservation dance anywhere in that precedent, because that step was never
  designed to fail the job. Task 2.2.1's pipe-exit-code-preservation logic is a genuinely
  new pattern for this repo, not a reuse of an existing one. Only the "append plain text
  under a Markdown heading to `$GITHUB_STEP_SUMMARY`" mechanic actually matches.
  **Remediation**: reword to "reuses `benchmark.yml`'s `$GITHUB_STEP_SUMMARY`-append
  mechanic; the exit-code-preserving pipe pattern is new to this workflow" — a factual
  correction to the plan document, not a code change.

- [ ] **Story 2.3 / testability (Lens 1 item 4)** — Local verification (Tasks
  2.3.1/2.3.2) thoroughly validates Jest's own exit-code behavior, but cannot validate
  two of the six ACs before the workflow file is actually pushed and exercised by a
  real GitHub Actions runner: **AC1** (does the `paths:` filter actually trigger the
  workflow on e.g. a `package.json`-only diff — this is GitHub-side trigger evaluation,
  untestable locally) and **AC5** (does `$GITHUB_STEP_SUMMARY` actually render as
  expected in the real Summary tab — this env var and its UI rendering only exist on
  real GHA runners). The plan's Risk Control section prepares a rollback story ("revert
  the workflow file") for *after* it's live, but there is no task that requires actually
  confirming AC1/AC5 against a real run before calling the story done — the loop is left
  open rather than closed. **Remediation**: add an explicit task (could live in Story
  2.3 or as a Story 2.4) requiring a real PR that touches only a broadened-filter file
  (e.g. `web-app/package.json`) to confirm the workflow triggers and the Summary tab
  renders correctly, on both a green and an intentionally-broken run, before the story
  is considered complete.

- [ ] **Task 1.3.1** (`gh issue create`) — This is a non-idempotent external side
  effect with no duplicate guard. If the task is retried (partial failure, an SDD
  worker agent re-running a task, a human re-running the command), it creates a second
  GitHub issue with the same title rather than reusing the first. **Remediation**: have
  Task 1.3.1 check for an existing open issue with a matching title first (e.g. `gh
  issue list --search "in:title Fix 2 Jest suites quarantined..."`) before creating, or
  explicitly flag in the task text that this step must not be blindly re-run.

- [ ] **Task 1.3.2** — No task verifies that all 3 `<ISSUE_NUMBER>` placeholder
  occurrences (1 in `BacklogEmptyState.test.tsx`, 2 in
  `SessionDetail.embedded.test.tsx`) were actually replaced before the PR ships. A
  partially-completed find-and-replace would leave a literal `<ISSUE_NUMBER>` string in
  a shipped comment with nothing catching it. **Remediation**: add a one-line check —
  `grep -rn '<ISSUE_NUMBER>' web-app/src` must return zero matches — to Task 1.3.2 or
  Story 2.3's local verification, before the PR is opened.

## Nitpicks

- AC4's Given-When-Then text (plan.md, AC4 section) states "the other 13 tests in
  `BacklogEmptyState.test.tsx` keep running and passing" — the file actually contains
  12 total `it(...)` tests (verified by grep), so 11 remain after quarantining the one
  at line 154, not 13. This contradicts the file's real content, though it doesn't
  affect correctness: Unresolved Questions §3's separate arithmetic ("9 affected tests")
  is internally consistent, and Task 2.3.1 already requires empirically confirming real
  pass/skip counts rather than trusting any arithmetic in the plan, so this will
  self-correct during implementation. Worth fixing the GWT text directly so whoever
  executes Task 1.1.1 isn't confused by the mismatch.
- The dependency graph serializes Epic 2 (workflow file creation, Stories 2.1/2.2)
  behind Epic 1's placeholder-replacement (`T1_3b --> T2_1`), even though creating
  `jest.yml` has no functional dependency on the TODO comment text in the test files.
  Immaterial for a Complexity-1 single-session task, but if executed by parallel SDD
  worker agents, only Story 2.3 (verification, which needs the quarantine already
  applied) genuinely needs to wait on Epic 1 — Stories 2.1/2.2 could run concurrently
  with Epic 1.
