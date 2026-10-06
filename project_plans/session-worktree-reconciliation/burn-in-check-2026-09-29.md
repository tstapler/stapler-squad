# Burn-in check: `worktree_consistency_sweep` — 2026-09-29

Backlog item 38241d87-d101-47f8-b7d0-4c3eb405ba17.

## Decision

**Do not flip the default yet.** The burn-in has not started, because the flag is not enabled on the primary instance.

## Evidence

- **Flag state:** `~/.stapler-squad/workspaces/d685c4b1a423cca3/config.json` (live instance) has no `worktree_consistency_sweep` setting, so it runs at the shipped default `false`. The other two workspace configs (`20761b3c035451cd`, `ab2461e5c3f3f8f6`) were not inspected.
- **Log grep:** `grep -h "worktree consistency sweep: tick complete" logs/staplersquad.log*` in the same workspace returned **zero lines**. The log line is emitted at `session/worktree_consistency_sweep.go:922`.
- **Threshold:** the rule needs ticks on at least 12 of 14 days. Current count is 0 of 14.
- **Notification history:** not reviewed, since there is no burn-in window to review.

## Flip location (for later)

`session/worktree_consistency_sweep.go:897`: `GetFeatureFlagWithDefault(FeatureFlagWorktreeConsistencySweep, false)` → `true`. `config/config.go` holds no flag-specific literal.

## Next steps

1. Enable the flag on the primary (non-isolated) instance via the feature-flag RPC/panel and record the date.
2. **Re-triage no earlier than 2026-10-08**, and only once 14 days of ticking have accrued after enablement.
3. At that point: count tick days, page through `get_notification_history` for the window, apply the decision rule from `implementation/plan.md` Risk Control, then flip and run `make build`, `make test`, `make lint`.
