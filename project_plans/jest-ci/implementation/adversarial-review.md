# Adversarial Review: jest-ci
**Date**: 2026-08-02
**Verdict**: CONCERNS

## Verification performed (this pass)

- Read `project_plans/jest-ci/implementation/plan.md` in full (500 lines).
- Confirmed the Mermaid graph (lines 149–181) draws no edge from `T1_3a`/`T1_3b`
  into `T2_1`/`T2_2`; the only edges out of Epic 1's issue-tracking sub-chain are
  `T1_1/T1_2 --> T1_3a --> T1_3b --> T3_1` (docs) and `T1_1/T1_2 --> T2_3a`
  (Epic 2's local-verification baseline, which only needs the `.skip()` edits, not
  the issue number). Epic 2's workflow creation (`T2_1`/`T2_2`) has zero inbound
  edges from Epic 1.
- Confirmed Story 1.3 (lines 300–327), the Risk Control bullet (lines 90–99), AC4
  (lines 223–237), AC6 (lines 249–260), and Unresolved Questions §2 (lines
  113–121) all consistently describe graceful degradation (`gh` failure →
  `TODO(#TBD)`, PR description / `jest-ci.md` notes a human must file the issue
  manually) rather than "stop and block." No residual place in the plan makes a
  real issue number a prerequisite for Epic 2 or Epic 3's CI-facing work.
- Spot-checked the 5 previously-noted concerns against actual current text:
  - **Exit-code masking (Task 2.2.1)**: confirmed fixed — `set -e` is deliberately
    never re-enabled after the pipe (lines 402–411), so the summary-writing block
    can't override `JEST_EXIT` before `exit "$JEST_EXIT"` runs.
  - **ANSI regex**: confirmed fixed — `sed -E 's/\x1b\[[0-9;?]*[a-zA-Z]//g'`
    (line 393) now strips all CSI sequences, not just SGR/color codes.
  - **benchmark.yml precedent overstatement**: confirmed fixed and verified
    against the actual file (`.github/workflows/benchmark.yml:95-101`, read
    directly) — that step does discard its exit code via `|| true`. Pattern
    Decisions (line 39) now correctly scopes the precedent to "the
    `$GITHUB_STEP_SUMMARY`-append mechanic," not the exit-code-preservation logic.
  - **`gh issue create` idempotency**: confirmed fixed — Task 1.3.1 (lines
    308–318) now checks `gh issue list --search ...` for an existing matching
    issue before creating one.
  - **Test-count GWT error**: confirmed fixed — AC4 (line 235) now correctly
    says "11 tests ... (12 total, 1 quarantined)," matching Unresolved Questions
    §3's verified count.
  - Also confirmed the related **Task 1.3.2 placeholder-grep** remediation is
    present: `grep -rn '<ISSUE_NUMBER>' web-app/src` must return zero matches
    (lines 324–327).

## Blockers

(none — the prior blocker is resolved; see Verification performed)

## Concerns

- [ ] **Story 2.3's local verification never exercises the actual `jest.yml` script — only bare `npx jest`.** Tasks 2.3.1/2.3.2 (lines 427–442) run `npx jest --selectProjects web-app --maxWorkers=2 --ci` directly in a shell, never the `set +e` / `set -o pipefail` / `tee` / `PIPESTATUS` / `$GITHUB_STEP_SUMMARY`-append script Task 2.2.1 actually ships. The plan now explicitly acknowledges this as accepted (lines 420–425: "the real script gets its first genuine exercise on the first real CI run either way... isn't worth a new task here"), which is a legitimate scope call for a Complexity-1 task, but the gap itself is unchanged from the prior review. A cheap mitigation (`export GITHUB_STEP_SUMMARY=/tmp/summary.txt` and literally pasting Task 2.2.1's script body into Task 2.3.1/2.3.2) was suggested previously and still isn't in the plan. Recommend adding it if the marginal 2–3 minutes is acceptable; otherwise the "accepted gap" framing is a reasonable enough call to ship as-is.

- [ ] **AC2's "zero suites collected" failure branch is asserted in the GWT but has no verification task.** AC2 (lines 207–214) explicitly names two distinct red-state triggers — a real test failure, and zero suites matched (e.g. all test files deleted) — but Story 2.3 only exercises the first (Task 2.3.2 flips one assertion). No task renames/empties `web-app/src`'s test files to confirm Jest still exits non-zero with 0 suites collected and no `--passWithNoTests` anywhere. This is well-established Jest behavior (low risk of being wrong), but unlike the AC1/AC5 gap above, this one isn't called out anywhere in the plan as a known, accepted limitation — it's just silently uncovered. Recommend either adding a third break/revert sub-task mirroring 2.3.2, or explicitly folding it into the same "accepted gap" language used for AC1/AC5 so it doesn't read as an oversight during a future readback.

- [ ] **AC1 (path-filter triggering) and AC5 (Summary tab rendering) still have no verification path before the real PR, by explicit design choice.** Unresolved Questions §4 (lines 134–143) and the Story 2.3 note (lines 444–451) now openly document this as an accepted limitation rather than silently leaving it uncovered — a real improvement over the prior version. But the underlying gap itself (no task confirms `jest.yml` actually triggers on a `package.json`-only diff, or that `$GITHUB_STEP_SUMMARY` actually renders in the real Summary tab, before or via a cheap throwaway-branch check) is unchanged from both the prior adversarial review and the architecture review, which both recommended adding a low-cost real-PR check. Downgraded from "silent gap" to "documented, accepted risk" — worth one more look before shipping, but not blocking.

## Minors

- **New**: The Risk Control section's "Quarantine permanence risk" mitigation (lines 79–84) states the mitigation is "every skip carries a `// TODO(#N)` referencing a tracked GitHub issue" — but that mitigation doesn't actually hold on the `TODO(#TBD)` fallback path (no real GitHub issue exists to track). The plan does document the `TODO(#TBD)` fallback elsewhere (Unresolved Questions §2, Risk Control's `gh issue create` bullet, AC4, AC6) and notes a human must file the issue manually, so this isn't a silent gap — but the Risk Control bullet itself isn't cross-referenced to acknowledge that its own stated mitigation is weaker on the degraded path. Low severity (a comment-only staleness risk in committed test files, not a functional one), but worth a one-line cross-reference so a future reader doesn't assume the mitigation always holds.
- `tail -n 40 jest-output.log` in Task 2.2.1 (line 393, unchanged from prior review) will always show the final Test Suites/Tests/Time block, but on a red run with several failures, 40 lines may not be enough to also show *which* suites failed. Consider `tail -n 100` or grepping specifically for the summary lines instead of a fixed line-count tail.
- The explicit `set -o pipefail` in Task 2.2.1's script (unchanged from prior review) is redundant given GitHub's own default `shell: bash` invocation already includes `-o pipefail`. Harmless defensive redundancy, not a bug — worth knowing it's belt-and-suspenders in case a future edit removes it thinking it's dead weight.
