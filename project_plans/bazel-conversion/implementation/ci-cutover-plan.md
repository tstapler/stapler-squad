# CI cutover to Bazel: staged plan (2026-11-11)

Output of the `ci-bazel-cutover-design` workflow (64 agents: three analysts, two review lenses per proposal, one synthesizer). Owner decisions that bound it (2026-11-11): move CI onto the Bazel build as the authoritative blocking signal; waive the Story 7.6 soak gate (the ledger had one row) with a one-line rollback; cut the number of discrete checks (a code PR touching every filtered path had 31); make the Bazel-selected e2e one PR check and move the full 5-shard run to push-to-main and nightly. `main` has no branch protection or ruleset, so "blocking" means a job without `continue-on-error` plus the merge-on-green convention. Claims are the agents' and were not all reproduced.

## Target PR checks

- bazel (job renamed from 'bazel (non-blocking)'; unfiltered, heavy steps skipped on docs-only; blocking unless vars.BAZEL_ADVISORY == 'true')
- pr-guards (backlog scaffolding, generated protos, PR title, banned terms / media / docs coverage, bazel guards; unfiltered)
- autofix (Bazel autofix; paths-ignore docs/project_plans/website/web-app/tests/e2e; never gating)
- gates (lint.yml: lint, web build checks, registry, ci-gates, remaining Jest; path-filtered to code paths)
- e2e (bazel-e2e.yml single job; path-filtered; blocking unless vars.BAZEL_ADVISORY == 'true')
- Conditional on their own paths only: goreleaser check + build smoke test (release config, go.mod, go.sum), ghostty-wasm, css-differential, website-ci, bazel-cache-proxy-trial x3
- Label-only or off PRs: ux-analysis (label run-ux-analysis, plus nightly and push), benchmark, e2e-video
- Push-to-main and nightly only: build.yml (full 5-shard e2e with comparator, full -race test-affected, go-guards incl. PTY stress), mcp-integration.yml, fallback-canary weekly, main-ci.yml, bazel-nightly-flake, bazel-main-repair, bazel-parity (weekly)

## Before and after

By tools/ci/count-pr-checks.py (which counts a fromJSON matrix as 1), a code PR touching every filtered path goes from 31 checks to 12, and a docs-only PR from 5 to 2. Real runtime checks per PR were higher (PR #1583 had 25, #1585 had 24, because the 5 e2e shards expand). A typical Go PR goes to 5 (bazel, pr-guards, autofix, gates, e2e), plus goreleaser when go.mod or release config changes. A web-only PR goes to 4. The 12 worst case is those 5 plus goreleaser, ghostty-wasm, css-differential, website-ci and 3 bazel-cache-proxy-trial. Stage-by-stage counts, to re-run with the script at each stage: S0-S4b 31/5, S5 29/3, S6 25/2, S7 25/2, S8a 22/2, S9 12/2. Bazel becomes the authoritative blocking signal at S3 and absorbs the legacy PR-only coverage by S4b (race, sinkguard, gitoracle, MCP integration, scanner, release smoke). Nothing is deleted from the repo. Retired jobs sit behind a `ci-legacy-off` branch sentinel, so one command (`tools/ci/legacy-ci on`) restores them. `gh variable set BAZEL_ADVISORY --body true` makes Bazel advisory again.

## Stages

### S0: Record the dated owner waiver of the Story 7.6 soak gate and the D-PROTECT-MERGE decision. Paperwork only, no CI behaviour change.

Checks before/after: Code PR 31 -> 31. Docs-only 5 -> 5. No workflow edits.

Prerequisites:
- Owner commits this change themselves. bazel-parity.yml fails unless the last author of owner-attestations.yaml matches tools/bzl/owner.txt, and CODEOWNERS pins the file.
- Owner confirms the commit email for tools/bzl/owner.txt (the review saw tystapler@gmail.com as the last author). The file does not exist today, so bazel-parity.yml currently exits 1 at its attestation step.

Edits:
- tools/bzl/owner.txt: create with the owner commit email.
- project_plans/bazel-conversion/implementation/owner-attestations.yaml: add block `ci_cutover_2026_11_11` in the existing waiver style (the `economic_gate_waivers` shape), not a new Go-parsed schema. Fields: `decision` (Bazel job is the authoritative blocking signal; fewer discrete checks; one PR e2e check), `waives` (Story 7.6 N=30 / K=10 / PARITY: PASS, the P10 cold row, and the e2e_escapes=0 and T8 precondition of Task 12.6a), `reason` (>=20 chars, quoting the owner decision), `granted: 2026-11-11`, `expires: 2027-02-09` (90-day policy), `soak_ledger_rows_at_waiver: 1`, `not_covered` (no 30-run evidence, no parity verdict, e2e escapes unmeasured), `rollback: tools/ci/legacy-ci on (S1)`.
- Same file: keep `adr_009_e2e.status: recorded` (check-spike-results.py keys on it). Add `superseded_by: ci_cutover_2026_11_11` beside it.
- project_plans/bazel-conversion/decisions/ADR-009-selective-e2e-and-expensive-tests-in-bazel.md: append 'Update (2026-11-11)'. S14 stays Fail. Owner decision 3 supersedes 'Full e2e run'. The interim PR e2e is legacy rule selection plus rotation (S8a), not slice selection. Epic 12 stories 12.1-12.5 stay as the path to real selection.
- project_plans/bazel-conversion/implementation/plan.md: add a dated 'waived by owner 2026-11-11, see owner-attestations.yaml' line to Story 7.6, Task 7.6a-c and Task 12.6a. Close the Unresolved Questions (N/K, waiver policy, ADR-009, D-PROTECT-MERGE option b) with the same date. Note that Task 10.2c must not delete any legacy job or the legacy-ci switch while the switch exists.
- docs/how-to/build-with-bazel.md: add section 'D-PROTECT-MERGE' (`grep -c D-PROTECT-MERGE` must be >=1 for Task 7.6d). Text: main has no branch protection or ruleset (verified: protected=false, rulesets=0). 'Blocking' means the job has no continue-on-error, and merge happens only when `gh pr checks <n> --repo tstapler/stapler-squad` shows no failing, pending or cancelled check. Skipped is accepted, but a PR touching e2e paths must show `e2e` present and passing. Merge with `gh pr merge <n> --repo tstapler/stapler-squad --merge`. Bot PRs stuck in action_required (release-please) are approved by the owner first. Add a ruleset-later note: never require path-filtered checks. Mark any `bazel`/`make` command lines with `<!-- proposal -->` unless they are in tools/bzl/verified-commands.txt.
- Leave soak-ledger.yaml untouched. bazel-nightly-flake.yml keeps appending rows.

Coverage mapping: No check changes. The waiver replaces an evidence gate, not a test. PARITY: PASS and the non-waivable P10 rows stay red by design. The attestation text, not parity-run-all, is the gate for the cutover stages. UNCOVERED by design: 30-run soak evidence, parity verdict, e2e_escapes=0. Compensating controls: nightly x3 flake run keeps writing ledger rows, full e2e stays on push-to-main and nightly (S9), and the legacy-ci rollback (S1) exists.

Rollback: Revert the docs commit. Nothing reads the waiver.

Verify:
- `go run ./tools/bzl/soak/soak-check -validate` and `python3 tools/bzl/check-spike-results.py`: both pass with the edited file. Paste the output in the PR.
- `grep -c D-PROTECT-MERGE docs/how-to/build-with-bazel.md` returns >=1.
- `bash tools/bzl/check-skill-proposals.sh --docs` on the five docs listed in bazel.yml passes.
- PR CI: bazel-guards (docs guard) green. Expect bazel-parity (weekly) to stay red on P10 rows. That is documented, not a regression.

### S1: Build the rollback machinery before anything depends on it: the legacy-ci toggle, the sentinel convention, the BAZEL_ADVISORY variable lever and a drift test. No behaviour change yet.

Checks before/after: Code 31 -> 31. Docs-only 5 -> 5.

Prerequisites:
- S0 merged.

Edits:
- New tools/ci/legacy-ci (Go, with BUILD.bazel and a test, per the repo rule that new scripts are Go). `legacy-ci on|off|status` rewrites the single token `ci-legacy-off` <-> `main` in the `pull_request: branches:` list of the registered legacy workflows. Initial registry is empty. S6 adds mcp-integration.yml and fallback-canary.yml, and S9 adds build.yml.
- Convention, applied from S6 onward. A retired legacy workflow keeps its whole `pull_request:` block, including its `paths:` / `paths-ignore:` filters, but with `branches: [ci-legacy-off]`. While that branch does not exist the workflow creates zero checks, and flipping the token back to `main` restores the exact old checks (names, filters, concurrency, permissions), with no reusable-workflow or workflow_call indirection.
- Test in tools/ci (Go): every workflow that contains the token is in the registry, every registry entry still has its `pull_request:` and `paths` block, and no branch named `ci-legacy-off` exists on origin (`git ls-remote`, skipped offline).
- Second lever for the Bazel side, a repository variable and no file edit: S3 and S9 will use `continue-on-error: ${{ vars.BAZEL_ADVISORY == 'true' }}`. Document `gh variable set BAZEL_ADVISORY --body true --repo tstapler/stapler-squad` (Bazel advisory again) and `gh variable delete BAZEL_ADVISORY` (blocking).
- tools/ci/count-pr-checks.py: add `--fromjson-width N` (default read from E2E_SHARD_TOTAL in build.yml) so a fromJSON matrix counts as N, print a 'real worst case' line, and add a unit test.
- docs/how-to/build-with-bazel.md: new section 'CI rollback switch' with the exact commands. State that a rerun may reuse the original run's context, so re-trigger with a new push or workflow_dispatch after changing the variable or the token. Add one CLAUDE.md pointer line (Reference Documents Index) once the section exists.

Coverage mapping: None dropped.

Rollback: Revert the PR. Nothing depends on the tool yet.

Verify:
- `go test ./tools/ci/legacy-ci/...` and `bazel test //tools/ci/legacy-ci/...`.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` is unchanged at 31 / 5. The new flag prints the real worst case.
- `actionlint` (make lint-workflows) clean.
- Local dry run on a scratch copy: `legacy-ci off` then `legacy-ci on` round-trips with `git diff` empty.

### S2: Harden the still non-blocking Bazel job so that blocking it is survivable: concurrency, timeouts, blobless checkout, a docs-only fast path, budget markers, and the repair-flow fix.

Checks before/after: Code 31 -> 31. Docs-only 5 -> 5. The Bazel job still shows as 'bazel (non-blocking)'.

Prerequisites:
- One completed post-#1591 `bazel` job on main with job conclusion success. Check `gh run view <id> --json jobs`, not the run conclusion, because continue-on-error masks failures (13 of 23 sampled runs failed at 'Build the full graph' on the dialogpresence visibility error).
- Local proof that tools/ci/bazel-gates, tools/ci/affected-targets and tools/bzl/check-gazelle-directives.sh pass in `git clone --filter=blob:none` with a pinned base and head. Paste the output. If it fails, drop the blobless edit.

Edits:
- .github/workflows/bazel.yml top level: add `concurrency: {group: bazel-${{ github.event.pull_request.number || github.run_id }}, cancel-in-progress: ${{ github.event_name == 'pull_request' }}}`. The run_id fallback means main pushes are never cancelled or dropped.
- .github/workflows/bazel.yml job `bazel`: `timeout-minutes: 60` -> `75` (observed p90 49 min, a cold run hit 67 min). Add `timeout-minutes: 5` to the 'Upload BEP' and 'Upload profile' steps (keep `if: always()`) because artifact-API outages caused retry stalls.
- Same job, checkout step: add `filter: blob:none` beside `fetch-depth: 0` (measured checkout 126-449 s, and lint.yml already does this).
- Same job: add a first step id `scope`. It lists the PR files via `gh api repos/${{ github.repository }}/pulls/$PR/files --paginate` and sets `code=false` when every file matches docs or planning paths (docs/**, project_plans/**, website/**, root *.md except CHANGELOG.md). It always sets `code=true` on push and workflow_dispatch. Gate the heavy steps (freshness, build, test, cross-compile, otel, affected) with `if: steps.scope.outputs.code == 'true'`. The job still reports green on docs-only PRs. Set `GH_TOKEN: ${{ github.token }}` and `pull-requests: read`.
- Same job: wrap 'Build the full graph' and 'Test the full graph' in a soft-timeout wrapper. On timeout it emits `::error::CI-BUDGET-EXCEEDED: bazel <step> exceeded <N>m`, using the exact prefix that ciBudgetExceededMarker in server/services/github_webhook_pr_fix.go reads. Budgets are derived from measured p95, for example build 40 and test 20.
- Same job: add a 'Failure signature' step (`if: failure() || cancelled()`) that greps the logs for 'The runner has received a shutdown signal', 'Terminated', 'Temporary failure in name resolution' and 'Unexpected token <'. It writes `infra-suspect` or `bazel-real` to $GITHUB_STEP_SUMMARY. There is no automatic test retry, which would hide real failures. Document `gh run rerun <id> --failed` as the rerun path.
- .github/workflows/bazel-main-repair.yml 'Regenerate': replace `make sync` (BUILD_SYSTEM defaults to make, which needs asdf/go/buf/bun; the log shows 'go/buf/bun not found') with `bazel run --config=ci //:sync-gen`. Handle the go.mod drift that `bazel mod tidy` reported. Before merge, dispatch it on a scratch branch and confirm the gazelle `embed/tmux` / `embed/tymuxd` errors do not rewrite BUILD files.
- Update the bazel.yml header comment (drop PROVISIONAL), and add the concurrency and timeout rules to docs/reference/ci-job-budget.md.

Coverage mapping: Nothing dropped. Docs-only PRs skip the heavy steps, and all their inputs are docs. The five docs read by check-skill-proposals.sh --docs are still checked, because bazel-guards still exists and is unfiltered.

Rollback: Revert the PR. Each edit is independent: concurrency, timeouts, checkout, scope, wrapper, repair.

Verify:
- `actionlint` and the workflow cache-write guard (the real tool is tools/bzl/wfcachecheck; run `go test ./tools/bzl/wfcachecheck/...`).
- PR's own CI: the job runs end to end on the blobless checkout. Record wall time versus the 2410 s median.
- A docs-only scratch PR shows `bazel` green in under 3 minutes.
- Dispatch bazel-main-repair on a scratch branch and record the result.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` is still 31 / 5.

### S3: Make the Bazel job the blocking signal. Legacy jobs stay on PRs alongside it, so this stage only adds a gate.

Checks before/after: Code 31 -> 31. Docs-only 5 -> 5. Same checks, but `bazel` can now block.

Prerequisites:
- S0, S1 and S2 merged.
- Evidence gate: at least 5 consecutive PR-event runs of the S2 job with job conclusion success (via `gh run view --json jobs`), plus one main push success after S2. Record the run IDs in the PR.
- Owner accepts the time-to-green cost of about +10 min until cache or checkout improves (legacy critical path is about 28 min, Bazel median is 40 min).

Edits:
- .github/workflows/bazel.yml job `bazel`: `name: bazel (non-blocking)` -> `name: bazel`. Replace `continue-on-error: true` with `continue-on-error: ${{ vars.BAZEL_ADVISORY == 'true' }}`. Add the step `echo "::notice title=CI mode::BAZEL_ADVISORY=${{ vars.BAZEL_ADVISORY || 'unset (blocking)' }}"`.
- Update every reference to the old check name: ci-job-budget.md, bazel.yml comments, plan.md, count-pr-checks.py expectations, bazel-main-repair.yml.
- Leave bazel-guards, bazel-autofix and all legacy jobs alone.

Coverage mapping: Adds a blocking signal and drops nothing. Bazel catches the new-package visibility class that no legacy job catches. Legacy jobs still carry -race, sinkguard, gitoracle, integration, registry, web-checks and lint until S4 and S7.

Rollback: `gh variable set BAZEL_ADVISORY --body true` (immediate, no file edit; re-trigger the run). Second layer: revert the PR.

Verify:
- `actionlint`.
- The PR's own CI shows `bazel` running as a non-continue-on-error job. Prove the lever with a scratch PR and the variable set to true: a forced failure leaves the PR check green-with-annotation.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py`: 31 / 5, with the new name in the list.

### S4a: Add the Bazel-side equivalents of the legacy-only coverage as measured, advisory steps inside the `bazel` job. No new check.

Checks before/after: Code 31 -> 31. Docs-only 5 -> 5.

Prerequisites:
- S3 merged.
- tmux, socat and Bun are available on ubuntu-latest, or are installed by the steps.

Edits:
- .github/workflows/bazel.yml job `bazel`, after 'Test the full graph', each step `if: ${{ !cancelled() && steps.scope.outputs.code == 'true' }}`, `continue-on-error: true`, with its own step `timeout-minutes` and the CI-BUDGET-EXCEEDED wrapper:
- (1) 'Tagged guard suites': `make test-delivery-guards test-oracle BUILD_SYSTEM=bazel` (sinkguard per directory, plus gitoracle with race on the five oracle package globs; recipes exist in make/bazel.mk). Replaces go-guards sinkguard and git oracle.
- (2) 'Race (affected)': `bazel build --config=ci //tools/ci/affected-targets`, then `sel=$(bazel-bin/tools/ci/affected-targets/affected-targets -base "$BASE_SHA")`, guard `[ -n "$sel" ] || exit 0`, then `bazel test --config=ci --config=race --test_arg=-test.short $sel`. Same HOME and test_env flags as the main test step. On main pushes nothing extra is needed, because main-ci.yml and build.yml already run the full -race suite. Replaces test-affected -race -short.
- (3) 'MCP integration': `bazel test --config=ci --config=integration --config=race //server/mcp/...`, then `bazel test --config=ci --config=integration //server/services/...` (no race, matching mcp-integration.yml). Record with `--test_output=summary` that TestMCPHandshakeSubprocess (server/mcp/server_integration_test.go, //go:build integration) actually ran and passed. Replaces mcp-integration.yml.
- (4) 'Scanner tests': `cd tools/scanner && go test ./...` (separate go.mod, not in the Bazel graph).
- (5) 'Release-shaped smoke': build //:stapler-squad, then run `./stapler-squad version` and `./stapler-squad list`, plus the `--mcp` handshake if cheap. This is the PR-time guard for the CGO_ENABLED=0 DB-driver regression. Note modernc.org/sqlite is the direct driver and mattn is indirect-only (go.mod, .bazelrc:102-103), so the risk is small.
- (6) Optional 'Pinned tmux / oracle': `bazel build //tools/bzl/tmux:tmux_bin`, export it through the test_wrapper PATH hook, install socat, and only then set FIDELITY_REQUIRE_ORACLE=1. Keep it only if a run shows zero SKIP in the fidelity corpus output. Otherwise delete the step and list it as the coverage change below.
- Drop nothing from legacy yet. Record measurements (cold and warm wall time, flake rate) for 10 PR runs in docs/reference/ci-job-budget.md.

Coverage mapping: -race -short on affected packages (test-affected, MCP): step 2 and step 3. sinkguard and gitoracle (go-guards): step 1. MCP integration tag: step 3. Scanner tests: step 4. Install check and `version`: step 5. EXPLICIT COVERAGE CHANGES: (a) the PTY-triple go-stress run (go-guards) has no Bazel port and moves to push-to-main and nightly via build.yml, so a ptmx race is caught after merge; (b) pinned tmux 3.8 and the xterm oracle on PRs, unless step 6 is proven, are caught on push via build.yml test-affected and main-ci test-tmux; (c) the test-affected coverage comment, registry-generate drift and RPC checks move to `gates` in S7 and are not dropped until then.

Rollback: Delete the added steps. The legacy jobs are untouched.

Verify:
- `actionlint`.
- The PR's own CI runs each step. Paste wall times and the test counts (non-vacuous) from `bazel test` output, and the TestMCPHandshakeSubprocess line.
- Compare the affected-targets selection with the tools/ci/test-affected.py selection on 3 recent PRs, and record any difference.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` is still 31 / 5.

### S4b: Promote the measured coverage steps from advisory to blocking, or consciously demote them to push-to-main.

Checks before/after: Code 31 -> 31. Docs-only 5 -> 5.

Prerequisites:
- S4a merged and at least 10 PR runs recorded: 0 infra-free flakes, p95 job wall time well under the 75 min cap. Owner reviews the numbers.
- Attach the run list to the PR.

Edits:
- bazel.yml: remove `continue-on-error: true` from each S4a step that met the threshold. A step that did not meet it is deleted, and its legacy equivalent stays on push-to-main only, as an accepted post-merge coverage change in ci-job-budget.md.
- Set job `timeout-minutes` from the measured p95 (default 75).
- If the affected-race step is over about 15 min warm, run it on PRs only for server/, session/ and mcp globs and leave the full -race to main (see risks).

Coverage mapping: Same table as S4a. After this stage every legacy PR-only guard has a blocking Bazel equivalent, or is listed as an accepted coverage change.

Rollback: Re-add `continue-on-error: true` to the steps (one edit), or set BAZEL_ADVISORY=true.

Verify:
- `actionlint`.
- The PR shows the steps passing blocking.
- Run the same PR once with a deliberately failing tagged test on a scratch branch and confirm red.
- count-pr-checks unchanged.

### S5: Consolidate the three PR guard jobs (pr-diff-guards, promo-guards, bazel-guards) into one unfiltered `pr-guards` check.

Checks before/after: Code 31 -> 29 (pr-diff-guards, promo-guards and bazel-guards become pr-guards). Docs-only 5 -> 3 (pr-guards, bazel, autofix).

Prerequisites:
- S3 merged (check names stable).

Edits:
- .github/workflows/backlog-scaffolding-guard.yml: delete job `guard`. Rename job `promo-guards` to `pr-guards`. Keep the `scope` step first (website/tests/unit/workflow-yaml.test.ts asserts steps[0].id === 'scope'). Directly after it add the two cheap gh-api steps ('Check PR diff for backlog scaffolding files' and 'Check PR diff for committed generated protobuf files') with `if: ${{ !cancelled() && github.event.action != 'labeled' && github.event.action != 'unlabeled' }}`. Add `if: ${{ !cancelled() && ... }}` to every existing promo step (they currently lack it and would silently skip after an earlier failure). Keep the job-level `if` that lets the allow-media label event through.
- Same file: add an unconditional checkout (blobless, fetch-depth 0), Setup Bazelisk, and the steps moved verbatim from bazel.yml `bazel-guards`: 'Guard scripts', 'Skill, docs and reviewability guards', 'Gazelle directive changes need owner review', 'Dependency changes' (annotation only, written to $GITHUB_STEP_SUMMARY and ::notice; the sticky PR comment is dropped so the job stays contents: read and pull-requests: read), 'First-error legibility', and 'PR title is a Conventional Commit'. Add `edited` to pull_request.types so a fixed title re-runs the check. `timeout-minutes: 12 -> 20`. The job has no `paths:` filter, so it reports on every PR including docs-only.
- Fix the stale baseline comments ('avg 4m / max 10.1m'). Measured median is about 4 s, from `gh run` data on the last 38 runs.
- .github/workflows/bazel.yml: delete job `bazel-guards`.
- Update website/tests/unit/workflow-yaml.test.ts: drop `guard` from the permissions loop and move the label-event assertion onto the moved steps' `if`.
- Grep and update every reference to the old names: docs/reference/ci-job-budget.md (lines 19/43/45), .gitignore:35 comment, build.yml:1013 comment, tools/ci/count-pr-checks.py expectations, plan.md, bazel-main-repair.yml.

Coverage mapping: All guard scripts run unchanged. The title check now re-runs on edit. Coverage change: the sticky dependency-change PR comment becomes a job-summary annotation (visibility only). The guards keep running on every PR, because there is no path filter.

Rollback: Revert the single commit (restores the three jobs).

Verify:
- `actionlint`; the website unit tests for website/tests/unit/workflow-yaml.test.ts; `bash tools/bzl/check-skill-proposals.sh --docs <5 docs>`.
- Scratch docs-only PR: `pr-guards` reports. Scratch PR with a bad title: it fails, then passes after the title edit.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` prints 29 / 3. Paste it.

### S6: Retire PR triggers that Bazel now covers or that are low-value, and narrow autofix and goreleaser. Each is its own small commit in one PR.

Checks before/after: Code 29 -> 25 (MCP Integration Tests -1, Fallback canary -3; goreleaser and autofix stay in the counter because go.mod and Go-adjacent paths still match). Real Go-only PR: also loses goreleaser and its 29-min median. Docs-only 3 -> 2 (autofix gone).

Prerequisites:
- S4b merged (MCP integration, race and the release smoke are blocking in Bazel).
- S1 legacy-ci tool exists.

Edits:
- .github/workflows/mcp-integration.yml: change `pull_request: branches: [main]` to `branches: [ci-legacy-off]`. Keep the paths block and the push trigger. Register the file in tools/ci/legacy-ci. Also check tools/ci/compare-affected/main.go (it reads this file's pull_request paths; still works because the block is kept), tools/ci/duration-trend.sh:42 and docs/reference/ci-job-budget.md:22; run `go test ./tools/ci/compare-affected/...`.
- .github/workflows/fallback-canary.yml: same sentinel on `pull_request`. Schedule and workflow_dispatch stay. Register it. Record a dated owner waiver of the PR-time make-path parity (Story 8.5 AC, plan.md:2117) in owner-attestations.yaml. The test-set-diff 'no test missing from Bazel' check was dormant anyway (BAZEL_FLIPPED unset).
- .github/workflows/goreleaser-check.yml: remove '**.go' from pull_request.paths (keep .goreleaser.yaml, Formula/**, tools/bzl/checkformula/**, go.mod, go.sum and itself). Push paths unchanged. Add a weekly `schedule` (cron '41 5 * * 1') as a cgo-regression net. Do not add workflow_call.
- .github/workflows/bazel-autofix.yml: add `paths-ignore: [docs/**, project_plans/**, website/**, web-app/**, tests/e2e/**]` under pull_request (types unchanged). paths-ignore is used instead of an allowlist so embed inputs and root files like CHANGELOG.md still trigger it. State in docs that autofix is never a gating check; the `bazel` freshness steps are the backstop for an unlisted input.
- Update docs/reference/ci-job-budget.md and tools/ci/duration-trend.sh trigger lists.

Coverage mapping: MCP Integration Tests -> bazel step 3 (integration, race on server/mcp) in S4b; still runs on push to main. Fallback canary make-only and pass-set-equality -> weekly canary plus build.yml on push; ACCEPTED coverage change: PR-time make-path rot detection and test-set-diff parity (rot can persist up to a week; Bazel is the build of record). GoReleaser: the config schema and formula dry run still run on release-config and dependency PRs. The `list`/`--mcp` smoke on Go-only PRs moves to Bazel step 5 (S4b). ACCEPTED coverage change: the otelc-woven CGO_ENABLED=0 release build is now post-merge for Go-only PRs. Autofix: web-only and docs PRs get no regenerate commit (none is needed).

Rollback: `tools/ci/legacy-ci on` restores the MCP and canary PR checks. For goreleaser, re-add '**.go' to the PR paths (one line). For autofix, remove paths-ignore.

Verify:
- `actionlint`; `go test ./tools/ci/...`; `go test ./tools/ci/compare-affected/...`.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` prints 25 / 2.
- Scratch PR touching only session/*.go: only bazel, pr-guards, autofix, lint, e2e and the legacy build jobs appear. No MCP, canary or goreleaser check.
- Scratch PR touching go.mod: goreleaser still runs.
- Roll the toggle on/off locally and confirm `git diff` is limited to the token.

### S7: Fold the non-Bazel gates into the one lint workflow, renamed `gates`: web build checks, registry, ci-gates and the remaining Jest suites. This is prep for dropping build.yml from PRs in S9.

Checks before/after: Code 25 -> 25 (build.yml web-checks still runs on PRs until S9, so work is temporarily duplicated). Docs-only 2 -> 2.

Prerequisites:
- S6 merged.

Edits:
- .github/workflows/lint.yml: rename job `lint` to `name: gates`. Keep all current steps (buf breaking, ShellCheck, golangci-lint with the 16-min soft cap and CI-BUDGET-EXCEEDED marker, dupl/complexity --new-from-rev, `make lint-custom`, gofmt, codemod tests, jscpd, ESLint, stylelint, tsc + validate-catalog).
- Remove 'Import cycle check (go build ./...)' (Bazel compile fails on cycles). Replace the full `npx jest --ci` with a step still named 'npx jest' that runs only the suites //web-app:jest ignores: `--testPathPatterns 'e2e_isolation_guard|e2e_notification_conventions|notification_docs_exist|notification_plan_docs|files/analytics/__tests__/events|terminal/__tests__/conformance'`. Add a small test (in tools/ci-gates) asserting this list equals web-app/BUILD.bazel ignore_patterns.
- Add steps (each `if: ${{ !cancelled() }}` with its own timeout-minutes), copied from build.yml `web-checks` and `test-affected`: 'make web-build'; 'Minified size build and budget check', 'Download main stats' and 'Bundle growth vs main' (job `permissions: {contents: read, actions: read}`); 'Build Storybook'; 'Walk every built file against the auth exemption' (`REQUIRE_BUILT_OUT=1 go test ./server/middleware -run TestBuiltAssets`); 'Lockfile is stable across repeated installs'; 'Dependency screen'; `cd tools/ci-gates && bun install && bun run test`; 'Check new RPCs have tests' (make registry-generate, `git diff --exit-code docs/registry/features/`, new-RPC tested check, validate-catalog).
- Widen lint.yml `pull_request.paths` and `push.paths`: tools/ci-gates/**, tools/scanner/**, docs/registry/**, web-app/public/docs/**, web-app/build/**, web-app/patches/**, web-app/bunfig.toml, web-app/package.json, web-app/bun.lock, Makefile, .github/workflows/lint.yml. Derive the final list mechanically from the build.yml detect-changes regex (line ~102) and the web-checks filters, so no path that triggered web-checks or the registry check now triggers nothing.
- Job `timeout-minutes`: set from a measured serial run (about 940 s + web checks 153 s + registry and ci-gates; the earlier estimate was 1200-1300 s, so confirm). Keep 60 only if the measured total is under about 35 min. Otherwise 75.
- Update the slow-suites row `lint.yml#lint~npx jest` in docs/reference/bazel-slow-suites.md if the step name changes. Run `bazel run //tools/bzl/e2e/slowsuites` (the checker fails on a missing named step).
- Update the ci-job-budget.md table, the tools/ci/count-pr-checks.py expectations, and the references to `Lint (Go, web, shell, proto)`.

Coverage mapping: Web build checks (smoke, size and growth, Storybook, TestBuiltAssets, lockfile, dep screen) -> gates steps. Registry drift and RPC test check, validate-catalog, ci-gates tests -> gates steps. Full Jest -> //web-app:jest in `bazel` plus the six ignored suites here. Import cycle -> bazel build. Custom analyzers -> nogo in `bazel build` and `make lint-custom` here. Test-affected coverage comment and the feature-coverage report: ACCEPTED coverage change (visibility only; registry-generate and validate-catalog remain gating). A rerun of `gates` now repeats about 20 min of unrelated work (accepted).

Rollback: Revert lint.yml. build.yml jobs are still on PRs, so nothing was removed.

Verify:
- `actionlint`; the tools/ci-gates tests; `bazel run //tools/bzl/e2e/slowsuites`.
- The PR's own CI: gates green, with wall time recorded against the timeout.
- Scratch PR touching only docs/registry/**: `gates` runs. Scratch PR touching only tools/ci-gates: runs.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` prints 25 / 2.

### S8a: Replace the four shadow jobs with a single advisory `e2e` job that exercises the exact design before it replaces anything. Legacy 5-shard e2e still runs on PRs.

Checks before/after: Code 25 -> 22 (bazel-e2e plan/bin/shard/report -> one advisory `e2e`; build.yml's 9 jobs, including the 5 shards, still run). Docs-only 2 -> 2 (the e2e workflow is path-filtered, so docs-only PRs get no e2e status; this is a deliberate choice recorded in ci-job-budget.md).

Prerequisites:
- S3 merged. S7 merged (not strictly required).
- Dry-run is the evidence for S9: one PR-event run on a mode=all diff and one on a mode=none diff. The workflow file must exist on main with a workflow_dispatch trigger before `gh workflow run` can target a branch, so the dispatch evidence comes from PR-event runs of this stage and a second dispatch after it merges.
- Fact: tests/e2e/slice-manifest.json and durations.json do not exist (Task 12.1a not done), so 'Bazel-selected' cannot be real yet. The interim design is legacy select-specs.ts rule selection plus a rotating fifth of the suite, with the binary built by Bazel. S0 records this.

Edits:
- .github/workflows/bazel-e2e.yml: delete jobs plan, bin, shard, report. Add ONE job `e2e` (`name: e2e`), runs-on ubuntu-latest, `timeout-minutes: 60`, `continue-on-error: true` for now, `permissions: {contents: read, pull-requests: read}`, concurrency kept (cancel-in-progress on PRs). Add `workflow_dispatch` with input `pr_number`. Remove the `push` trigger (main is covered by build.yml). pull_request `paths:` widened to the full legacy detect-changes set: **.go, go.mod, go.sum, proto/**, web-app/**, tests/e2e/**, Makefile, install.sh, .bun-version, .gitmodules, third_party/**, CHANGELOG.md, server/middleware/testdata/**, scripts/e2e-*.ts, .github/actions/**, .github/workflows/_prepare.yml, build.yml, bazel-e2e.yml.
- Steps (SHA pins unchanged from the file): checkout (submodules, blobless); ./.github/actions/setup-bun; setup-go; setup-bazel (bazelisk-cache, repository-cache, disk-cache bazel-e2e; no cache-write credential, `--remote_upload_local_results=false`); 'Read pinned Playwright version'; 'Select specs' (id plan): gh api file list -> `bun tests/e2e/scripts/select-specs.ts --specs specs.txt --changed-file changed.txt`, failing open to `all` with a `::warning::`. For mode none, output `mode=none` and skip the rest. Otherwise `k=$(( ${PR_NUMBER:-0} % 5 + 1 ))`, `rot=$(cd tests/e2e && bun scripts/shard-assign.ts assign --raw --shard $k --total 5 < ../../specs.txt)`, union = selected specs + SAFETY_NET (smoke.spec.ts, route-smoke.spec.ts) + rot, deduped and filtered with `grep -E '^[A-Za-z0-9._/-]+$'`. If the estimated spec-seconds / 3 workers from shard-timings.json exceeds 35 min, trim the rotation fifth to fit. When PR_NUMBER is unset (dispatch), k=1 with a warning.
- If the Axe-relevant paths from ux-analysis.yml (web-app/src/**, web-app/build/**, bun.lock, server/web/**, server/middleware/static.go, Makefile) changed, force-include accessibility.spec.ts.
- Build steps (each `if: steps.plan.outputs.mode != 'none'`): prepare-stub; `bazel build --config=ci --stamp --workspace_status_command=tools/bzl/workspace_status.sh --lockfile_mode=error --remote_upload_local_results=false //:stapler-squad` and copy to out/; `go build` of seed-demo, seed-hidden and e2e-sshd (missing from the old shadow); e2eguard revision check; a step that fails if the embedded index.html is the prepare stub (port build.yml's 'Verify the real web UI was embedded'). Retry-once loops (`for i in 1 2; do ... && break; done`) around bazel build, `bun install --frozen-lockfile`, and `bunx playwright install --with-deps chromium` plus apt-get of tmux/git/unzip.
- Run: env TEST_SERVER_BINARY, TEST_SEED_DEMO, TEST_SEED_HIDDEN, TEST_SSHD_BINARY, E2E_WORKERS=3, E2E_SPEC_SELECTION; `cd tests/e2e && timeout 40m bash scripts/run-ci-shard.sh 1 1`; copy the legacy tolerance block (exit 124 -> `::error::CI-BUDGET-EXCEEDED: e2e exceeded 40m`; other nonzero exit tolerated if blob-report exists); the two assert-e2e-report.mjs calls (executed>0, shard-pass1-empty, shard-stress-skipped); upload blob-report and shard-report*.json (`if: always()`, retention 7); write mode, k, spec count and estimated seconds to $GITHUB_STEP_SUMMARY.
- If the dry run shows rendering differences versus the pinned mcr.microsoft.com/playwright image, run the Playwright step through `docker run --rm --init --ipc=host` of that image, with the Bazel-built binary mounted. Do not make host `--with-deps` a blocking step without that proof.
- Code edits: tests/e2e/scripts/shard-assign.ts add a `--raw` flag (print the path unchanged) with a case in tests/e2e/baseline/shard-assign.test.ts. tests/e2e/scripts/select-specs.ts: move .github/workflows/bazel-e2e.yml (and build.yml) from NONE_RULES to ALL_RULES, with test coverage. Add a baseline test that accessibility.spec.ts is in the selection for every web-app/src path in the ux-analysis filter.
- Retire the dormant machinery without deleting it: keep tools/bzl/e2eslice, e2erun, e2eguard, e2e-global-inputs.txt, quarantine.json and escapes.json. Update tools/bzl/wfcachecheck/main_test.go so its glob matches the new file and fails on zero matches. Update docs/reference/bazel-slow-suites.md:21.

Coverage mapping: Nothing dropped yet. Report-only semantics of raw test failures on PRs are retained (the legacy comparator, confirm-rerun and baseline steps never ran on PRs). The Bazel-built binary is exercised by Playwright for the first time, which is the point of this stage.

Rollback: Revert the PR (the old shadow jobs return; they gave no signal anyway).

Verify:
- `actionlint .github/workflows/bazel-e2e.yml` exits 0.
- `bun test` for the shard-assign and select-specs baseline tests.
- PR-event run on a mode=all diff and one on a docs-adjacent diff. Paste run URLs, wall time and spec counts. Wall time must fit 60 min. If it needs more than about 50 min, lower the fifth.
- Compare the Bazel-built binary run against the legacy `make stapler-squad` e2e on the same SHA, on a few specs: same pass set, or list the differences.
- `go test ./tools/bzl/wfcachecheck/...`.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` prints 22.

### S8b: Collateral prerequisite: make the main/nightly full-run verdict meaningful before it becomes the only full signal.

Checks before/after: Code 22 -> 22. Docs-only 2 -> 2.

Prerequisites:
- Owner reviews the baseline diff. This is the owner decision point on whether regressions are fixed or blessed.

Edits:
- Triage the comparator red on main (run 43109172728). Extract the real comparator verdict, not a raw per-shard sum (the claimed 281 'unexpected' is a raw Playwright sum across shards and stress passes; per-shard lines add up to about 259 plus 22). Use the comparator step's own output (the 'would fail the gate' count).
- For each failing group, classify it as real regression, stale expected-diff or environment. Fix the specs first. Only then add the remainder to tests/e2e/baseline/bun-expected-diff.json (currently 0 entries, and its note says the list may only shrink; keep a ratchet) or next-e2e-baseline.json, with per-entry owner review.
- Refresh tests/e2e/shard-timings.json from a green run (shard 1 ran 2293 s of a 2280 s budget; the artifact expires in 7 days).
- Exit gate: 3 consecutive green `e2e (Bun build)` runs on main (last 60 push runs: 0 success). Ship as its own collateral PR.

Coverage mapping: None dropped. This restores the meaning of the full run that S9 moves to main and nightly.

Rollback: git revert the baseline commit.

Verify:
- `bun scripts/e2e-baseline-compare.ts` on the failing run's blob reports passes with the new baselines.
- Three consecutive green main runs of e2e (Bun build).

### S9: The e2e swap: make `e2e` blocking and take the legacy per-PR build.yml jobs off PRs. The full 5-shard run moves to push-to-main and a new nightly. Includes UX analysis moving to label-only.

Checks before/after: Code 22 -> 12 by the counter method (bazel, pr-guards, autofix, gates, e2e, goreleaser, ghostty-wasm, css-differential, website-ci, bazel-cache-proxy-trial x3). build.yml's 9 PR jobs and ux-analysis drop out. Typical Go PR 5 (+goreleaser on go.mod); web-only PR 4; docs-only PR 2 (bazel, pr-guards). Overall 31 -> 12 worst case, 5 typical, 2 docs-only (from 5).

Prerequisites:
- S4b, S6, S7, S8a merged. S8b green (or the owner explicitly accepts a red nightly).
- S8a dry-run evidence for both modes is attached. If the dry run shows the host-install Playwright renders differ, the docker variant is in place.
- Owner confirms the coverage trade: a mode-all PR runs targeted specs plus a rotating fifth (about 20% of 206 specs / 21,200 spec-seconds) instead of 100%, and regressions in the other four fifths are found on the next global-input PR, main push or nightly.

Edits:
- .github/workflows/bazel-e2e.yml job `e2e`: replace `continue-on-error: true` with `continue-on-error: ${{ vars.BAZEL_ADVISORY == 'true' }}`. Also fail when selected=0 for a non-none mode, so a missing manifest or selector failure cannot pass silently.
- .github/workflows/build.yml: change `pull_request: branches: [main]` to `branches: [ci-legacy-off]`. Keep the whole block, `push`, `workflow_dispatch`, and add `workflow_call:`. No job is deleted, which keeps the rollback and Task 10.2c ordering. Register build.yml in tools/ci/legacy-ci. Confirm that build.yml's job conditions of the form `always() && (event_name != 'pull_request' || ...)` run everything on push and workflow_call.
- .github/workflows/nightly.yml: add job `e2e-full-chromium` with `uses: ./.github/workflows/build.yml`, its own concurrency group `e2e-full-nightly` with cancel-in-progress false, so main pushes cannot evict the nightly. Do not add a schedule to build.yml. Dry-run through workflow_dispatch of nightly.yml first.
- .github/workflows/ux-analysis.yml: `pull_request.types: [labeled]` with job `if: github.event_name != 'pull_request' || github.event.label.name == 'run-ux-analysis'`; add `schedule` (cron '53 6 * * *') and `push: main` with the existing web paths; change the concurrency group to `ux-analysis-${{ github.event.pull_request.number || github.run_id }}`; keep the PR comment step guarded by event_name == 'pull_request'; write Lighthouse and Claude output to $GITHUB_STEP_SUMMARY on non-PR events. Axe moves into `e2e` (accessibility.spec.ts forced include from S8a).
- Update tools/ci-gates/__tests__/workflow-config-invariants.test.js and any reference to moved check names (utilization-report, e2eavoided T8 baseline, targets.yaml).
- docs/reference/ci-job-budget.md: replace the 'Where each check runs' table with the final set, record before/after counts (paste the script output), state that skipped and path-filtered workflows give no status, and update lines 47, 51 and 55 (e2e rows and the 'require e2e (Bun build)' advice becomes: if protection is ever added, require `bazel`, `pr-guards` and `gates` only; `e2e` is path-filtered and must not be required). Add the rollback and merge convention pointer to CLAUDE.md.
- Record in owner-attestations.yaml the accepted coverage changes: Lighthouse and the Claude screenshot analysis are label/nightly only; PTY stress is push+nightly; PR e2e is targeted plus rotation.

Coverage mapping: build.yml detect-changes, prepare, e2e-env, e2e-binary, 5 shards + e2e-bun: PR coverage replaced by the single `e2e` check (S8a design); full 5-shard run with comparator and confirm-rerun stays on push-to-main and now nightly. build.yml web-checks -> `gates` (S7). test-affected -> `bazel` S4b steps + `gates` registry/ci-gates; the full -race suite stays on push. go-guards -> S4b steps, except the PTY stress, which moves to push+nightly (ACCEPTED). UX Analysis: Axe (blocking WCAG AA) -> accessibility.spec.ts inside `e2e` for web paths; Lighthouse (already continue-on-error) and Claude vision (advisory) -> label/nightly (ACCEPTED). ACCEPTED: per-PR e2e coverage falls to targeted plus rotating fifth; PR test failures were already report-only on the legacy PR path, so the verdict strictness is unchanged; a crash, empty report or timeout fails `e2e`. UNCOVERED and not justified away: spec-level escapes are unattributed until Task 12.5b (e2e-escape.py) exists; this is tracked as a follow-up, not a blocker.

Rollback: Two commands. `tools/ci/legacy-ci on` (one token flipped in build.yml, mcp-integration.yml and fallback-canary.yml) restores the legacy PR jobs with original paths, names and concurrency. `gh variable set BAZEL_ADVISORY --body true` makes `bazel` and `e2e` advisory. UX analysis and goreleaser are restored by reverting their hunks (listed in docs). Nothing is deleted from the repo. Use a push or workflow_dispatch to re-trigger after flipping.

Verify:
- `actionlint` on all touched workflows; `go test ./tools/ci/...`; the tools/ci-gates workflow-config-invariants test.
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` prints 12 / 2 (and `--fromjson-width` real worst case). Paste before and after in the PR.
- The PR's own CI: only the final check list appears. build.yml does not run on the PR.
- Dispatch nightly.yml once: `e2e-full-chromium` runs the full 5-shard suite and comparator. Record the result.
- Scratch PR run with `legacy-ci on` applied: the legacy checks reappear with their original names (proves the rollback). Revert it.
- Merge-gate test: touch an Axe-relevant web path and confirm `e2e` runs accessibility.spec.ts.

## Remaining risks

- Time-to-green gets worse. The Bazel PR job has a median of 40 min (p90 49, one cold run 67) against a legacy critical path of about 28 min. The cap is 75 min. A cold cache or a runner stall now blocks a PR with only a manual `gh run rerun --failed`. Remote cache is deferred by the owner. Blobless checkout (S2) is the main mitigation and is only valid if the local proof passes.
- The Bazel job was NOT green on main when examined. 13 of 23 sampled runs failed at 'Build the full graph' (dialogpresence visibility, fixed by #1591), hidden by continue-on-error. S3 is gated on fresh post-fix evidence. A new Go package that breaks Bazel can now block merges. That is intended, but expect early noise.
- Race, sinkguard, gitoracle, integration and the pinned-tmux oracle have never run under Bazel in CI. Their cost and flake rate are UNVERIFIED until S4a measures them. If the affected-race step is too slow it falls back to push-to-main. Parity of affected-targets versus test-affected.py selection is checked on only 3 PRs.
- Accepted post-merge coverage changes: PTY-triple stress (push+nightly), otelc-woven release build on Go-only PRs, PR-time make-path canary and test-set-diff parity (weekly), Lighthouse and Claude vision analysis (label/nightly), test-affected coverage comment. A ptmx race or cgo regression can reach main before it is caught.
- PR e2e coverage falls to targeted specs plus a rotating fifth on mode-all PRs (about 80% of PRs under the legacy selector). Regressions in the other four fifths surface on the next global-input PR, main push or nightly, after merge. There is no escape detector (Task 12.5b is not built; escapes.json is []). 'Bazel-selected' is not literally true until the slice manifest (Task 12.1a, 205 specs) exists, and spike S14 predicts the manifest will select almost everything even then.
- The full run on main is not a working safety net today: 0 of the last 60 build.yml push runs succeeded (comparator red; 26 were queue-cancelled). S8b addresses this, but if the baseline is blessed wholesale it hides real regressions. The nightly will be red until then, and no automatic issue is opened (a main-only issue reporter was left out for lack of a proven design).
- Single-job e2e depends on unproven items: Playwright host install matching the pinned container (fallback docker run), a Bazel-built binary under Playwright (never exercised), and about 50 min wall time against a 60 min cap. S8a's dry run is the proof.
- The rollback is one command but not a repository variable. A repo variable cannot remove checks (skipped jobs still count) and `paths` filters cannot read variables, so the sentinel is a one-token edit across 3 workflow files via a tool. The ux-analysis and goreleaser changes need their hunks reverted. Name-keyed tooling is unaffected because names are preserved. Rerunning after changing a variable or token may reuse old context.
- Gates absorbs about 20 minutes of previously parallel work into one serial job, and a rerun repeats all of it. It also takes on the widened path filters, whose completeness is checked by derivation, not by test.
- `git add`-style hazard: the owner-attested file must be committed by the owner. bazel-parity.yml stays red on P10 rows by design, and any external expectation of `PARITY: PASS` is overridden by convention only.
- No branch protection exists. 'Blocking' is a convention (merge only on green `gh pr checks`), and a path-filtered `e2e` that does not run on docs-only PRs is not a signal. Do not add it to a ruleset. Bot PRs stuck in action_required (release-please) still need the owner's approval, and bazel-main-repair PR checks depend on its token.

## Owner questions

- Rollback form: is `tools/ci/legacy-ci on` (one command, one commit, one token across 3 workflow files) an acceptable 'one-line rollback', or do you want a literal single-line edit? The single-line alternative is a caller workflow with a branches filter, which was reviewed as riskier (check-name changes, concurrency and permission inheritance, lost per-workflow path filters).
- Confirm the commit email for tools/bzl/owner.txt (tystapler@gmail.com?) and that you will commit S0 yourself. Without it bazel-parity.yml keeps failing.
- Interim PR e2e: do you accept 'legacy rule selection plus a rotating fifth, binary built by Bazel' as the meaning of 'Bazel-selected' until Task 12.1a produces a manifest? Or should S9 wait for the manifest (which spike S14 says will select almost everything)?
- Should the nightly full e2e open an issue when red (needs a main-only reporter and the S8b baseline triage first), or is a red nightly visible only in the Actions tab acceptable for now?
- S8b baseline policy: fix the failing specs first and bless only a reviewed remainder, or accept a one-time bulk baseline refresh?
- Bazel job timeout 75 min and the +10 min median time-to-green until cache or checkout improve: acceptable, or should remote cache be un-deferred before S3?
- Accepted post-merge coverage list (PTY stress, release-shaped smoke on Go-only PRs, PR-time make-path canary, Lighthouse and Claude vision, coverage comment, sticky dependency comment): approve as written, or name any that must stay on PRs?
- Should the weekly goreleaser schedule and a main-only 'ci-red-main' issue reporter be added in this effort, or deferred?
