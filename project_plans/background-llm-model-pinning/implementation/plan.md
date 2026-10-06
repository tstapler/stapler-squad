# Plan
1. Spike (1h): confirm CLI support for effort and CLAUDE_CODE_SUBAGENT_MODEL; record in research.
2. Config: ModelPolicy type, defaults, getters, validation, tests (backend).
3. Resolver `ResolveFeatureModel` generalizing resolveTriageModel; table tests incl. non-claude program, bad alias, none.
4. Work/review default-pipeline fallback in backlog_service_triage.go (spawn + review sites); keep ComputeExecutorHash from raw pipeline values so drift detection isn't false-flagged.
5. Headless features: pass per-feature model via CallOptions; set Pool DefaultModel to haiku in dependencies.go.
6. Effort + subagent model injection into spawned background session env/args (if spike says supported).
7. Live settings: expose policy via existing config RPC + web settings panel (proto change -> make proto-gen); hot-reload without restart.
8. Tests: unit, plus integration asserting spawned program string contains --model.
9. Docs: docs/reference entry; feature registry regenerate.

## Adversarial review
- Risk: pinning Sonnet for items whose pipeline mode pins nothing could degrade hard work -> opt-out value "none" and per-item pipeline pin still wins.
- Risk: executor hash drift -> hash only pipeline-supplied values.
- Risk: non-claude programs -> claude-only guard.
- Risk: Haiku quality for handoff summaries -> make configurable; spot-check.
- Risk: model IDs stale -> use family aliases.
