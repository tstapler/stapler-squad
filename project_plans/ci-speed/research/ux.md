# UX Research: ci-speed

Scope note: this project has no product UI surface. This is a short pass on
developer-experience (DX) for the two consumer classes named in
`requirements.md`'s Users/Consumers section — human contributors watching PR
checks, and stapler-squad's own backlog/session-worker automation that opens
PRs and waits on CI. WCAG/ARIA/keyboard-navigation is out of scope (not
applicable to CI tooling) per the task brief.

## 1. Good DX for CI status/timing visibility

GitHub's native Checks UI already covers the baseline well — no need to build
a custom dashboard. The lightweight wins available without leaving that UI:

- **`::error::` / `::warning::` workflow commands** create inline annotations
  that surface directly on the PR "Files changed" tab and the checks summary,
  not buried in a log — `::error file=...,line=...::message` or the
  file/line-less `::error::message` form. This is the right mechanism for a
  budget failure (see §3): one line, visible without opening the run.
- **`$GITHUB_STEP_SUMMARY`** (append Markdown to the file at that path) renders
  as a persistent "Summary" panel on the workflow run page — the natural
  place to print a "this job took Xm34s, budget is Ym" line per job, and to
  render a small before/after or trend table (e.g. from Phase 2's proposed
  duration-history recording) without a separate viewer. Upload failures here
  don't fail the step, so it's safe to write generously.
- Both are official, zero-dependency `echo` commands — no marketplace action,
  no extra job, no new infra. This directly answers the brief's question:
  yes, there's a lightweight way to surface budget status in the Checks UI
  itself rather than requiring someone to open Actions logs.
- `gh pr checks <PR>` already gives contributors a terminal-native view
  (name, status, duration, URL per check) — worth calling out in any
  CONTRIBUTING-style doc this project produces, but it needs no new code.

Source: [Workflow commands for GitHub Actions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands).

## 2. Does this repo's own automation already poll CI, and what does that imply?

Yes. `session/backlog_plugin_github_prs.go`'s `fetchCILabel` (called from
`computeLabels`, [backlog_plugin_github_prs.go:147-163](../../../session/backlog_plugin_github_prs.go#L147-L163))
hits `GET /repos/{owner}/{repo}/commits/{sha}/check-runs` once per open PR,
on every backlog sync tick. That tick runs every
`defaultSyncInterval` = 15 minutes
([backlog_sync.go:16](../../../session/backlog_sync.go#L16)), or on-demand via
a manual `TriggerSync` RPC — not a tight poll loop, so it isn't measurably
wasteful in raw API-call volume.

Two real gaps, though, both relevant to this project's "automation waits on
CI" consumer:

- **Binary signal, no in-flight state.** `fetchCILabel` only looks at
  `conclusion` (`failure`/`timed_out` → `pr:ci-failing`); an `in_progress` or
  `queued` check run produces no label at all
  ([backlog_plugin_github_prs.go:191-196](../../../session/backlog_plugin_github_prs.go#L191-L196)).
  A PR sitting mid-run on `demo-publish.yml` (136m avg per the baseline table)
  looks identical to one that's fully green. Any automation that gates on
  this label (e.g. deciding a PR is ready to merge or ready for a fix-loop)
  can't currently tell "still running" from "passed." If this project adds
  a duration-budget gate as its own check run, it should not assume
  downstream consumers already have "pending" visibility — they don't today.
- **Same fixed 15-minute cadence regardless of expected workflow length.**
  The poll interval isn't tuned to any specific workflow's typical duration —
  it's a flat constant. Not costly at today's volume, but worth naming as a
  known simplification if a future duration-budget dashboard wants tighter
  freedom to shorten this interval for fast workflows without over-polling
  slow ones (e.g. `demo-publish.yml`).

## 3. Error-state UX for a CI-duration budget/gate failure

The clearest pattern, matching what's already idiomatic in this repo's own
webhook logic: make a budget failure *distinguishable from a correctness
failure* at both the human-glance and the automation-triage layer.

- **Human-facing**: the budget-gate step should end with
  `echo "::error::CI-BUDGET-EXCEEDED: job '<job>' took ${DURATION}, budget is ${BUDGET}" `
  before `exit 1`, plus a `$GITHUB_STEP_SUMMARY` line with the same numbers.
  That reads immediately in the PR checks list and the annotations panel as
  "too slow," not a generic red X requiring a log dive.
- **Automation-facing — this is the sharper finding.**
  `server/services/github_webhook_pr_fix.go`'s `failureShapedConclusions`
  ([github_webhook_pr_fix.go:45-50](../../../server/services/github_webhook_pr_fix.go#L45-L50))
  already auto-triggers a fix session on *any* `check_run`/`workflow_run`
  completing with `failure`, `timed_out`, `cancelled`, or `action_required` —
  with no distinction of *why*. GitHub's own `timed_out` conclusion is
  reserved for the platform's own `timeout-minutes` enforcement, not a custom
  script's `exit 1`; a budget-gate step that fails by exiting non-zero will
  report as a plain `failure`, indistinguishable from a real test/build
  break. Left as-is, the budget gate would cause the same auto-fix session
  to fire for "this job got slower" as for "this job is broken" — not
  useful, since a slow job usually isn't fixable by a code patch in one
  session. **Recommendation for the Phase 3 gate design**: prefix the
  check-run/job name or the `::error::` message with a stable, greppable
  token (e.g. `CI-BUDGET-EXCEEDED:`), and either (a) have
  `extractCheckOrWorkflowRunEvent` skip auto-fix triggering when the run name
  matches that pattern, or (b) route it to a distinct backlog label/category
  instead of the generic fix-loop. Either way, this needs a one-line addition
  to that extractor, not a new subsystem.

## Bottom line

No new UX surface, no dashboard, no design doc needed. Three concrete,
cheap asks for the implementation phase:
1. Use `::error::` + `$GITHUB_STEP_SUMMARY` for budget-gate output (§1, §3).
2. Give the budget-gate's failure a distinguishable, greppable marker so the
   existing auto-fix webhook (`github_webhook_pr_fix.go`) doesn't mistake
   "too slow" for "broken" (§3).
3. If Phase 3's observability work touches `backlog_plugin_github_prs.go`,
   note the existing binary in-flight/failing gap (§2) — not blocking, but
   don't assume "no `pr:ci-failing` label" already means "CI passed" for
   any new automation built on top of it.
