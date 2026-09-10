# Adversarial Review: ci-speed

**Date**: 2026-08-27
**Verdict**: CONCERNS
**Note**: Iteration 2 — re-review of previously BLOCKED items only.

## Blockers

(none)

## Concerns

- [ ] **Task 4.2.1d doesn't say the new `checkRunHasBudgetMarker` API call happens *after* signature verification.** Story 4.2.1's acceptance criteria only anchor the marker check to "after `extractCheckOrWorkflowRunEvent` returns `actionable = true`" — that point in `handlePRFixEvent` (`server/services/github_webhook_pr_fix.go:234-247`) is *before* `verifySignatureForRepo` (line 249). The self-actor filter a few lines later is deliberately placed *after* verification specifically "so an unauthenticated request can never force selfLogin's GitHub API call" (line 263-264 comment) — the new marker check makes an equivalent outbound GitHub API call and needs the same placement rule, but nothing in Task 4.2.1d says so. Low severity (a forged, unsigned `check_run` payload could only force a read of public annotation data, costing one rate-limited API call — no write, no data exposure), but worth a one-line addition to the acceptance criteria so an implementer doesn't place it before `verifySignatureForRepo` by default. Recommendation: add "after signature verification, mirroring the self-actor filter's placement rationale" to Story 4.2.1's acceptance criteria or Task 4.2.1d.

## Minors

- Task 4.2.1b's task description says the wrapper reuses "the same exit-124-on-expiry convention `pty-race-regression`'s step already uses today at line 505-509" — but that existing step treats `timeout`'s exit 124 as **success** ("ran clean for the whole budget" — `|| [ $? -eq 124 ]`), the opposite of the Soft-Timeout Wrapper's semantics (timeout expiry = budget exceeded = failure). The wrapper's actual code (`timeout <N>m <cmd> || { echo "::error::..."; exit 1; }`) is correct on its own terms; only the cited "convention" analogy in the task's prose is backwards. Cosmetic — fix the citation, no code impact.
- The 4 minors carried from the prior review (Epic 2.7 zero-wall-clock-savings scope drift; Epic 4.3's unreviewed weekly `contents: write` push to `main`; no `Retry-After` backoff in the duration-trend script; the embed-stub substitution's feedback-latency shift for a broken-frontend-build signal) are unchanged by this iteration's plan edits and remain minor, non-blocking — not re-litigated in depth per the task scope.

## Prior Blockers — Resolution Status

- **Blocker 1 (marker mechanism): RESOLVED.**
  1. **Soft-timeout math**: all 7 wrapper-eligible jobs in Story 4.2.1's table have a soft budget strictly below their hard `timeout-minutes:` ceiling, with a consistent 4-6 minute margin (`test` 44m/50m, `integration-coverage` 20m/25m, `pty-race-regression` 14m/18m, `mcp-integration` 25m/30m, `e2e-video` 20m/25m, `go-lint` 16m/20m, `web-lint` 16m/20m). Consistent and reasonable.
  2. **Go-side design**: the redesign drops the `context.job`-vs-display-name lookup entirely — the wrapped step emits a plain `::error::CI-BUDGET-EXCEEDED:` annotation (GitHub attaches these to the job's own check run automatically, no `checks: write` or job-name matching needed on the YAML side), and the Go handler fetches that check run's annotations directly via `check_run.id`, a field already present in the webhook payload. Verified technically sound: `GET /repos/{owner}/{repo}/check-runs/{id}/annotations` is a real, documented GitHub Checks API read endpoint, and `github/http_client.go` (`newGHRequest`/`ghHTTPClient`) already provides exactly this kind of native, authenticated GET-to-arbitrary-path infrastructure used by other read calls (`GetPR`, `GetIssue`, `GetCommit`) in this package — distinct from `github/commit_status.go`'s documented constraint that Checks API **writes** are GitHub-App-only (irrelevant here since the annotation write happens automatically via the Actions workflow command, not a `checks.update()` call from this service). No API-support gap found.
  3. **Scope-mismatch**: resolved via the plan's explicit wrapper-eligible (7 jobs) vs. backstop-only (every other new `timeout-minutes:`) split, with a stated rationale for each backstop-only job (e.g. `registry-validation.yml`'s 10m cap against a 3m avg/7.4m max explicitly framed as "a generous safety net, not a budget nobody expects to hit"), matching option (c) from the original blocker's own recommendation — narrow the ceilings and state the false-positive-tolerance decision explicitly rather than implying full coverage. No job is silently left uncovered without a stated reason.

- **Blocker 2 (success-metric measurement): RESOLVED.**
  1. Epic 1.2's methodology computes `max(updatedAt across all of a PR's triggered workflow runs) − min(createdAt across the same set)`, per PR, via `gh pr list`/`gh run list` filtered on `headRefOid` — a genuine aggregate across every workflow a PR triggers, matching the Domain Glossary's "Wall-Clock Time-to-Green" definition, not a single workflow's duration.
  2. Phase 5 / Epic 5.1 is sequenced to ship last in the Dependency Visualization (`E12 --> E51`, `Phase2/3/4 --> E51`), explicitly gated on "Phase 2–4's structural changes have had time to accumulate a handful of real merged PRs to measure," with a concrete minimum sample size (~15-20 post-fix merged PRs) rather than a calendar-only gate.
  3. The before/after comparison is concrete: identical methodology re-run, median/p90 delta computed as `(baseline − post-ship)/baseline`, an explicit statement of whether the ~50% target was met, and — if not — a requirement to name the specific epic whose expected win didn't materialize rather than reporting a bare number. Not a vague "feels faster" check.
