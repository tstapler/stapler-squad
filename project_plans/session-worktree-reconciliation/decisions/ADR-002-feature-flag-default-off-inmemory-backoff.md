# ADR-002: Feature-Flag-Gated Default-Off Rollout, In-Memory Backoff for Repeat Notifications

**Status**: Accepted
**Date**: 2026-09-23

## Context

Two rollout-shaped decisions have no strong existing precedent in research to inherit directly:

1. **Which live-settable mechanism gates the sweep, and what's its default at ship.** stack.md §5 confirms two live-settable mechanisms exist in the repo: a dedicated per-feature config struct (config.json-only, no RPC/UI wiring — a dead end for "live-settable") and the generic `config.FeatureFlags map[string]bool` + `UpdateFeatureFlag` RPC (genuinely live-settable end-to-end today, used by `stream_hub`/`tymux`/`triage_guidance_halt`).
2. **How to avoid re-notifying every 15-minute tick for a still-unresolved, genuinely unfixable finding** (e.g. a `base_commit_sha` that will never resolve without a human fixing the repo). pitfalls.md §1 names "Backoff-gated repair, not every-tick repair" as a hard lesson from `reconcileStaleWorkSessions`'s own 14x bounce-loop incident, but that lesson doesn't specify a mechanism for a brand-new feature with no persisted state of its own for this purpose.

## Decision

- Use the generic `config.FeatureFlags` map with a new key `"worktree_consistency_sweep"` (`FeatureFlagWorktreeConsistencySweep`), re-checked on every `sweep(ctx)` call (not just at `Start`), so it can be disabled mid-run with no restart. **Default `false` at ship** — this sweep performs DB writes (`Worktree` row creation) on unambiguous matches, and per `feedback_document_ai_decisions_in_edge_cases.md`'s standing instinct, a new class of automated repair should be observed before being on-by-default.
- Track a `lastNotifiedAt map[(sessionID, issueKind)]time.Time` in the sweep's own in-memory loop state (not a new persisted DB field). A process restart resets this map, causing at most one extra re-notification per still-broken finding — an acceptable, bounded cost, consistent with the existing sweepers' own willingness to accept in-memory-only backoff state.

## Consequences

- Enabling/disabling the sweep is a single RPC call, no deploy — matches `feedback_rollout_flags_live_settable_no_env_vars.md`'s standing instinct.
- **Backoff duration decided at 24 hours** per `(sessionID, issueKind)` pair (plan.md's "Decided" section, resolved 2026-09-23 during Phase 3 plan repair — no human owner was available to pick a value, so it was derived from the sweep's own 15-minute tick interval and this repo's stated low tolerance for notification-panel spam, per pitfalls.md's 14x-bounce-loop incident). Revisit against real notification volume once the flag is flipped on in staged rollout; it remains a single unexported `const`, trivially retunable without a schema change.
- Because the backoff state is in-memory-only, a multi-instance deployment (per `docs/reference/state-isolation.md`) tracks backoff independently per instance — acceptable since each instance's `*session.Storage` is already independently scoped (stack.md/pitfalls.md §4 confirm no cross-instance DB visibility exists today).
