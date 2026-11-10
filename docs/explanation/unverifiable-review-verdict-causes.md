# UNVERIFIABLE review verdict causes

Measured 2026-11-10 over the 30-day Flow window from `review_verdicts` in the workspace `sessions.db`
(`overall_outcome='UNVERIFIABLE'`), bucketed by `summary` text (keyword match, first rule wins). The backlog
item cited 45 verdicts; the window had moved to 47 by measurement time.

| # | Cause | Count | Share of 47 |
|---|---|---|---|
| 1 | Capability self-check failed (system, no review attempted) | 17 | 36% |
| 2 | Diff truncated before the item's files (reviewer prompt diff cap) | 13 | 28% |
| 3 | Item has no acceptance criteria (placeholder verdict) | 7 | 15% |
| 4 | Diff unrelated or out of scope (wrong diff basis, CI/config-only) | 5 | 11% |
| 5 | Other: criterion needs runtime/prod data, or no diff computable | 5 | 11% |
| 6 | Timeout (`codebase-read call timed out`) | 0 | 0% |

Causes 2 and 4 (the reviewer's diff did not cover the item) total 18, 38%, the largest combined group.

Also found: `diff_truncated` is `false` on 12 of the 13 truncated rows, and `reviewed_commit_sha` is empty on
46 of 47, so neither is usable for per-verdict diagnosis yet.

## Fix shipped

Cause 2: the review diff cap cut the raw byte stream, so large CI/config/`.claude` files at the top of the
diff consumed the budget. `TruncateDiffByFile` (`session/review_diff_budget.go`) now cuts whole files, ordering
so source files survive, and `session/backlog_review.go` uses it. Tests: `session/review_diff_budget_test.go`.

## Re-measure (open)

Target is under 10% UNVERIFIABLE. The fix is not deployed yet, so no post-fix number exists. Re-run
`get_flow_summary` (`review.unverifiable`) over a window that starts after deploy. Baseline on 2026-11-10:
51 of 175 (29.1%).
