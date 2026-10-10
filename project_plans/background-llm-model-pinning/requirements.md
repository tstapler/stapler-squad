# Requirements: background-llm-model-pinning

Source: backlog item 357d72ec (2026-10-04 quota audit).

## Problem
Background LLM work runs on the account default model. `CachingPipelineEngine.ExecutorFor` returns ("","") for the default pipeline (session/pipeline_engine.go:516-520), so work and review sessions are unpinned; the headless pool is built without `DefaultModel` (server/dependencies.go ~756). Opus costs several times Sonnet, which costs more than Haiku, against subscription quota.

## Goals
1. Per-feature model and effort settings, resolved through one function.
2. Defaults: Haiku for tagging (exists), completion/handoff summaries, intent parse, PR-description drafts; Sonnet for work and review; Opus only by explicit opt-in.
3. Evaluate CLAUDE_CODE_SUBAGENT_MODEL and a lower effort level for app-launched background sessions.
4. Settings are live-editable (config / feature-flag panel); no env vars as the control surface.

## Non-goals
Changing interactive user-created sessions; non-claude programs (aider, gemini) must never receive a Claude model ID.

## Acceptance criteria
- Default-pipeline work and review sessions launch with the configured model (default Sonnet) instead of the account default.
- Headless pool features (completion narrative, handoff summary, intent parse, PR draft) default to Haiku via a per-feature setting.
- Opus is only used when explicitly configured.
- Each setting is changeable at runtime without restart and without env vars.
- Non-claude programs are never passed a Claude model.
- An effort setting is applied to background sessions where the CLI supports it; the evaluation of CLAUDE_CODE_SUBAGENT_MODEL is documented.
- Unresolvable values fall back to the built-in default and log a warning.
