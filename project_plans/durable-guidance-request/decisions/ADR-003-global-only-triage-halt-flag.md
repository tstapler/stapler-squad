# ADR-003: `triage_guidance_halt` Feature Flag Is Global-Only — No Per-Item Override in v1

**Date**: 2026-09-12
**Status**: Accepted

## Context

Requirements' Constraints section says the AC3 feature flag "must use the existing live-settable rollout pattern — `config.FeatureFlags`-backed global default + per-scope override, both settable at runtime via the flag's own RPC + settings panel, no env var, no rehearsal-gate precondition — mirroring `stream_hub`/`tymux`/`native_git_*`." Both `stream_hub` (`server/services/stream_hub_rollout_service.go`) and `tymux` have a per-*session*-name override map on top of the global flag. The question: does `triage_guidance_halt` need an equivalent per-item (or per-item-category) override in v1, or is a plain global boolean (`config.GetFeatureFlag`/`SetFeatureFlag` + a `GetFeatureFlags`/`UpdateFeatureFlag`-style RPC pair) sufficient?

## Decision

**Global-only** for v1: one `config.FeatureFlags["triage_guidance_halt"]` boolean, default OFF, read via `EffectiveTriageGuidanceHaltEnabled(cfg) = cfg.GetFeatureFlagWithDefault(TriageGuidanceHaltFeatureFlag, false)`, set via the plain feature-flag RPC pair (`server/services/feature_flag_service.go`'s existing `GetFeatureFlags`/`UpdateFeatureFlag` pattern), surfaced in the existing settings panel. No `*RolloutService`, no per-item override map, no new RPCs beyond registering the flag name.

## Rationale

- **The Constraint's "per-scope override" language describes the *mechanism family* (`stream_hub`/`tymux` both being cited as models), not a hard per-item-override requirement for this specific flag.** `stream_hub`/`tymux` needed per-session overrides because those flags gate a live *connection/session-runtime* behavior where a single canary session needs to diverge from the global default while a rollout is staged. `triage_guidance_halt` gates a *decision at triage-start* ("halt and create a request, or proceed") for a per-item, one-shot headless run — there's no live-connection canary use case; a triage run either halts on ambiguity or it doesn't, once, per attempt.
- **YAGNI, with an explicit reversibility check**: requirements' Risk Control section says "the rest of the feature is additive with no flag — rollback is a straight revert" and appetite is Medium (1-2 weeks) for a v1. A `*RolloutService` (`stream_hub`'s is 145 lines, `tymux`'s 117) is a real, non-trivial addition (RPC definitions, proto messages, settings-panel UI, session/override-key resolution) that has no identified consumer in this plan — no story needs "halt on item X's triage but not item Y's" independent of the global default. Building it now is speculative.
- **Reversibility is not compromised**: if a per-item override is needed later (e.g. an operator wants guidance-halt on for one flaky item category while still evaluating it globally), it is a strictly additive follow-up (new override map + RPC + panel control) — nothing in this plan's global-only design needs to be torn out or migrated to add it, since the accessor function (`EffectiveTriageGuidanceHaltEnabled`) is already the single call site triage consults; a future per-item check slots in before the global fallback with no call-site churn beyond that one function.

## Consequences

- Simpler v1: `server/services/feature_flag_service.go`'s existing generic flag RPC covers registration; no new proto messages, no new settings-panel component beyond adding one row to the existing feature-flags list.
- Triage integration re-reads the flag at the exact halt-decision instant via `EffectiveTriageGuidanceHaltEnabled(config.LoadConfig())`, not cached at pass start — per `research/pitfalls.md`'s explicit warning that this is a background pipeline, not a request/response RPC, and per-item override or not, staleness at the decision point is the risk that matters.
- If a per-item override becomes a real requirement post-v1, it follows the `stream_hub_rollout_service.go` template directly (already fully worked out in research) — flagged as a natural, low-risk follow-up, not re-litigated from scratch.

## Alternatives Rejected

| Alternative | Rejected because |
|---|---|
| Per-item override (`*RolloutService` pattern, keyed by backlog item ID) | No identified v1 consumer; triage halt is a one-shot per-attempt decision, not a live-canary scenario the per-session override pattern was built for; adds ~150 lines + new RPCs + panel UI for a need that hasn't materialized. |
| Per-item-*category* override (e.g. by `pipeline_mode` or label) | Same objection, plus no existing precedent for category-scoped flag overrides to copy from (`stream_hub`/`tymux` are both per-session, not per-category) — would be net-new design, not reuse. |
