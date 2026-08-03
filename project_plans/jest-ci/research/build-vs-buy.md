# Build vs. Buy: AC5 (Jest summary in GitHub Actions Summary tab)

Research for: wiring the web-app Jest suite into CI. Scope of this doc is narrow —
AC5 only: "Test Suites/Tests/Time summary must show up in the GitHub Actions Summary
tab (not buried in raw log), on both pass and fail runs."

## 1. Existing OSS options

| Candidate | Writes to `$GITHUB_STEP_SUMMARY`? | New devDependency? | New Action? | Maintenance (as of Aug 2026) | License |
|---|---|---|---|---|---|
| `dorny/test-reporter` | Yes — publishes via Check Runs API **and** can render into the job summary (recent releases added a step-summary size bump to 1 MiB, confirming active summary-tab support) | Yes — needs a machine-readable input; typically `jest-junit` (JUnit XML) or Jest's own `--json`/`--ctrf` output | Yes — marketplace action | Active: issues/PRs from Feb/Mar/May 2026, ongoing dependency bumps (e.g. `actions/upload-artifact` v5→v6) | MIT |
| `mikepenz/action-junit-report` | Primarily a PR-check/annotation action (Check Run with summary + inline annotations on the PR); job-summary is a secondary surface, not its primary design target | Yes — `jest-junit` reporter to produce JUnit XML | Yes — marketplace action | Active: latest release v6.4.2, June 2026 | Apache-2.0 |
| `ctrf-io/github-test-reporter` (+ `ctrf-io/jest-ctrf-json-reporter`) | Yes — explicitly supports posting to job summaries, PRs, checks, issues, annotations (`github-report: true`) | Yes — `jest-ctrf-json-reporter` (new Jest reporter, CTRF-schema JSON) | Yes — marketplace action | Active, actively promoted by the maintainer's own blog posts through 2026 | MIT |
| Bespoke: `jest --json` + shell/`jq` parsing → `$GITHUB_STEP_SUMMARY` | Yes, directly | No | No | N/A — it's a few lines of workflow YAML, nothing to maintain externally | N/A |
| Bespoke: capture Jest's own printed summary block → `$GITHUB_STEP_SUMMARY` | Yes, directly | No | No | N/A | N/A |

All three marketplace actions are current and reasonably maintained — none is a dead
project. But all three require **both** a new npm devDependency (a Jest reporter that
emits JUnit XML or CTRF JSON) **and** a new marketplace Action, pulling in third-party
supply-chain surface (an external action executing in CI with repo context) for a
requirement that boils down to "put text already produced by Jest into a job summary."

## 2. Bespoke summary vs. adopting a reporter

Jest already prints exactly the block AC5 asks for — `Test Suites: X passed, Y total`,
`Tests: ...`, `Time: ...` — as part of its default reporter output. The catch: **Jest's
default reporter writes this summary to stderr, not stdout** (long-standing,
well-documented Jest behavior — the default reporter avoids stdout specifically so it
doesn't collide with test `console.log`/piped output). Any bespoke capture must merge
stderr into the captured stream (`2>&1`) or read stderr explicitly — `npx jest > out.log`
alone will silently produce an empty summary. This is the one sharp edge in the "few
lines of shell" approach and must be handled explicitly (`npx jest --ci 2>&1 | tee
jest-output.log`), not assumed.

Two bespoke variants:

- **(a) Capture stdout+stderr, tail the human-readable summary block, append to
  `$GITHUB_STEP_SUMMARY`.** Simple, but string-scrapes free-form CLI text — a Jest
  version bump that reformats the summary (color codes, wording) could silently break
  the tail/grep. Needs to wrap the block in a fenced code block (```` ``` ````) in the
  summary so ANSI codes don't corrupt Markdown rendering, and should strip ANSI escapes
  (`sed -E 's/\x1b\[[0-9;]*m//g'` or run with `--colors=false`/`FORCE_COLOR=0`).
- **(c) `jest --json --outputFile=jest-results.json`, parse with `jq`, format Markdown
  by hand.** Structured and robust to cosmetic stdout changes, but Jest's `--json`
  flag suppresses the normal human-readable reporter output — the workflow then needs
  *two* Jest invocations (one for the real run, one for JSON) or must reconstruct the
  summary counts from the JSON schema (`numTotalTests`, `numPassedTests`, `numFailedTests`,
  `numTotalTestSuites`, etc. are all present) rather than getting Jest's own formatted
  text for free. `jq` is preinstalled on GitHub-hosted `ubuntu-latest` runners, so this
  adds no dependency either, but it is meaningfully more code than (a) for a
  Complexity-1 task.

Given AC5's actual bar — "the standard Jest summary visible without opening the raw
log," not structured per-test reporting, trend graphs, or flaky-test detection — a
bespoke script is proportionate. It avoids a new npm devDependency, avoids a new
third-party Action with its own maintenance/supply-chain lifecycle, and directly
satisfies the literal AC. The `ponytail` philosophy (avoid new dependencies where a few
lines of shell suffice) applies cleanly here.

## 3. Precedent in this repo

`grep -rn "GITHUB_STEP_SUMMARY" .github/workflows/` finds exactly one existing user:

- [`.github/workflows/benchmark.yml:100-101`](../../.github/workflows/benchmark.yml)
  (job: benchmark comparison) —
  ```yaml
  echo "## Go Benchmarks (Tier 1)" >> $GITHUB_STEP_SUMMARY
  cat benchstat-tier1.txt >> $GITHUB_STEP_SUMMARY
  ```
  This is exactly the bespoke pattern: run a tool, capture its plain-text output to a
  file, append a Markdown heading + the raw text to the summary file. No marketplace
  action, no new dependency. This is a direct, in-repo precedent for approach (a)/(c)
  and should be copied rather than re-researched. No other workflow in
  `.github/workflows/` (`backlog-scaffolding-guard.yml`, `build.yml`, `demo-publish.yml`,
  `deploy-pages.yml`, `e2e-video.yml`, `goreleaser-check.yml`, `lint.yml`,
  `mcp-integration.yml`, `registry-validation.yml`, `release-please.yml`, `release.yml`,
  `ux-analysis.yml`) writes to `$GITHUB_STEP_SUMMARY`, and none references `jest-junit`,
  `dorny`, `test-reporter`, or `ctrf` — confirmed via
  `grep -rln "jest-junit\|dorny\|test-reporter\|ctrf" .github/workflows/ web-app/package.json`
  returning no matches.

## 4. Recommendation

| Option | Verdict |
|---|---|
| (a) Bespoke: capture Jest's own stdout+stderr summary block, strip ANSI, append to `$GITHUB_STEP_SUMMARY` in a fenced code block | **Recommended** — matches the existing `benchmark.yml` precedent, zero new dependencies, satisfies AC5's literal bar (Jest's familiar summary block visible in the Summary tab on pass and fail). Must explicitly capture stderr (`2>&1`) and neutralize ANSI color codes; wrap output in `always()` step condition so it posts on failure too. |
| (b) `jest-junit` + `dorny/test-reporter` | **Viable, not recommended for this task** — well-maintained (MIT, active in 2026) and would give richer per-test/PR-check output, but adds a new devDependency and a third-party marketplace Action for a requirement that is "show the summary block," not full JUnit-grade reporting. Worth reconsidering only if a future AC needs per-test annotations, trend history, or flaky-test tracking. |
| (c) `jest --json` + custom parsing script | **Viable fallback** — no new dependency, more robust to Jest CLI text-formatting changes than (a) since it reads structured fields, but requires either a second Jest invocation or reconstructing summary text Jest already formats for free. More code than (a) for the same AC5 outcome; use only if (a)'s ANSI/text-scraping proves fragile in practice. |

**Bottom line: build, don't buy.** Use (a) — a bespoke shell append to
`$GITHUB_STEP_SUMMARY`, following the exact pattern already in
`.github/workflows/benchmark.yml` — with `always()` so it runs on both pass and fail,
`2>&1` to capture Jest's stderr-based summary, and ANSI-stripping so the fenced block
renders cleanly. No new npm devDependency, no new marketplace Action required for AC5.
