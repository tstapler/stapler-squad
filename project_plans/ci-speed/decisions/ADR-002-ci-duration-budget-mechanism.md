# ADR-002: CI-Duration Budget/Gate — Native `timeout-minutes:` + Advisory `gh api` Trend Script

**Date**: 2026-08-27
**Status**: Accepted
**Context**: ci-speed — requirements.md's Observability Requirements and Success Metrics ask for "a CI-duration budget/gate ... so a newly added slow job or dependency creep is caught automatically."

---

## Context

Three candidate mechanisms were evaluated in `research/build-vs-buy.md` §2: (A) a hand-rolled custom script comparing step timestamps, (B) an existing GitHub Marketplace action (`komiya-atsushi/action-enforce-timeout-minutes`), and (C) GitHub's native `timeout-minutes:` combined with (D) a small repo-owned scheduled script querying the Actions REST API for trend regression.

## Decision

- **Hard ceiling ("this job is now clearly runaway/hung")**: native `timeout-minutes:` on every job, sized from the baseline table's own Max column with headroom (Phase 4, Epic 4.1) — not a custom script. 11 of 14 workflow files currently have no job-level timeout at all (`research/build-vs-buy.md`), defaulting to GitHub's 6-hour cap.
- **Soft/trend signal ("this job is creeping slower over weeks, still under any reasonable ceiling")**: a small, repo-owned, advisory-only scheduled workflow (Phase 4, Epic 4.3) that calls `gh api repos/tstapler/stapler-squad/actions/runs` (list-level, not per-job, to stay under GitHub's secondary rate limits per `research/pitfalls.md` §6) and records/compares durations. It never fails a required check — a bug in it can produce a false/missed alert, never a blocked merge.
- **Explicitly rejected**: a custom bash/Go script that *blocks* a PR based on hand-rolled timestamp comparison (Option A) — this duplicates `timeout-minutes:` with strictly worse failure modes (an off-by-one/race in custom timing code can either silently fail to gate or falsely block an unrelated PR, violating requirements.md's "must not break required checks" constraint) — and the single-maintainer marketplace action (Option B), which only checks that `timeout-minutes:` is *set* (a lint, not a runtime gate) and adds a supply-chain dependency for a capability a 10-line `grep`/`yq` check already covers if wanted.

## Consequences

- No new marketplace action is added to the trust boundary of any required-check-path workflow.
- The trend script (Phase 4, Epic 4.3) is the one piece of genuinely new custom logic this initiative writes, and it is scoped narrowly (query + rolling-average comparison, advisory output only) per `research/build-vs-buy.md` §3's "custom code is justified only where no native feature does the job" criterion.
- A native `timeout-minutes:` hit produces a GitHub `timed_out` check-run conclusion, which is already in `server/services/github_webhook_pr_fix.go`'s `failureShapedConclusions` map — Phase 4, Epic 4.2 addresses the resulting auto-fix-webhook ambiguity separately (see that epic's story), rather than by avoiding native timeouts.

**2026-08-27 addendum — soft-timeout wrapper redesign (post-adversarial-review)**: the original Epic 4.2 design tried to have a job set its own check run's `output.title` (via `actions/github-script` + `checks: write`) *after* a `timeout-minutes:`-triggered kill — this cannot work, since GitHub terminates the job immediately on a `timeout-minutes:` breach and no later step (including that github-script step) ever runs. The mechanism is redesigned as: (a) for jobs where hitting `timeout-minutes:` under normal conditions is plausible (Epic 4.1's "wrapper-eligible" jobs), wrap the job's actual long-running command in a shell-level `timeout` set a few minutes below the job's `timeout-minutes:` hard ceiling, so the *step* fails (and can reliably emit a `CI-BUDGET-EXCEEDED:` `::error::` annotation) before GHA's own hard ceiling ever fires; (b) the Go-side webhook reads that marker from the check run's annotations (fetched by `check_run.id`, already in the webhook payload) instead of trying to match a job's YAML id (`context.job`) against its display name to look up and update its own check run — a lookup architecture review confirmed doesn't work for jobs with a custom `name:` override (e.g. `build.yml`'s `test` job, `name: Test`). `timeout-minutes:` remains the hard ceiling/backstop for every job (Decision, unchanged); the wrapper is additive, scoped only to the subset of jobs where a routine budget breach is plausible enough to be worth disambiguating from a real failure.

## Alternatives Considered

- **Custom timestamp-comparison script (blocking)** — rejected; see Decision.
- **`komiya-atsushi/action-enforce-timeout-minutes` (lint-only marketplace action)** — rejected as unnecessary; its one capability (checking `timeout-minutes:` is set) is subsumed by actually setting them all in Phase 4, Epic 4.1.
- **No trend detection at all, ceiling only** — rejected: a hard ceiling alone cannot catch a job slowly regressing while staying under budget, which is exactly the "tame tail latency" statistical property requirements.md's Success Metrics ask for, not just a worst-case cap.
