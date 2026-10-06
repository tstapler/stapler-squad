# Research findings (VERIFIED by reading code unless marked)

- Triage already pins a model: `HeadlessTriageModelOrDefault` defaults to `family:sonnet`, "none" = account default (config/config.go:1011-1038); `resolveTriageModel` (server/services/backlog_service_triage_skip.go:24) is the pattern to generalize.
- Tagging already defaults to haiku (`config/config.go:1177`).
- Work stage: `ExecutorFor` -> `ResolveExecutorProgram` -> `programOverride` (server/services/backlog_service_triage.go:1035-1052). Empty for default pipeline, so no `--model`. Review stage same pattern at ~:2993.
- Headless pool: `PoolConfig.DefaultModel` exists (session/headless/pool.go:29) and is applied as `--model` (caller.go:210-229) but dependencies.go leaves it unset. Features in session/headless/features.go (SummarizeBacklogItem, DraftPRDescription, GenerateSessionCompletionNarrative, GenerateHandoffSummary) pass no per-call model; only tagging passes "haiku" (:507).
- Model alias map: server/workflows/model_families.go (haiku/sonnet/opus -> IDs); `session.ResolveModel` resolves `family:<alias>`.
- Live settings: `Config.FeatureFlags map[string]bool` is bool-only; string settings (like HeadlessTriageModel) live as Config fields. A new string-valued settings surface (or extending the existing config RPC/panel) is needed. UNVERIFIED: exact RPC/panel that exposes HeadlessTriageModel today — confirm in impl.
- CLAUDE_CODE_SUBAGENT_MODEL / effort: UNVERIFIED against current CLI; cited docs (code.claude.com/docs/en/costs) not fetched. Plan includes a spike to confirm env var and `--effort`/settings support before wiring.

## Design
Add `config.ModelPolicy` (map feature-key -> {model, effort}) with built-in defaults, one `ResolveFeatureModel(cfg, families, feature, program)` helper (generalizing resolveTriageModel, claude-only guard), consumed by: default-pipeline ExecutorFor fallback (work/review), headless feature call sites via CallOptions.Model, and pool DefaultModel as safety net. Opus gated: only accepted when explicitly set.
