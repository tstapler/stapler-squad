# LLM backend selector

Headless (non-interactive) LLM calls pick a backend through one selector instead of being hard-wired to the `claude` CLI. Interactive tmux sessions (`Program`) are unaffected.

## Backends

| Name | Implementation | Resume | System prompt | Tool restriction | WorkDir | Cost |
|---|---|---|---|---|---|---|
| `claude` | `headless.Pool` | yes | yes | yes | yes | priced |
| `consolette` | claude CLI with `ANTHROPIC_BASE_URL` = router URL | yes | yes | yes | yes | unpriced |
| `gemini` | `headless.GeminiCaller` | no | prepended | no | yes | priced if pricing known |
| `agy` | one-shot CLI (`agy --print`) | no | prepended | no | yes | unpriced |
| `opencode` | one-shot CLI (`opencode run`) | no | prepended | no | yes | unpriced |

Code: `session/headless/backend*.go`. Every backend reports cost to the `CostSink` exactly once; backends that cannot price report `(0, false)`.

## Resolution

Precedence: stage program (backlog stage executor) > per-feature override > global default > `claude`. A feature key like `gate-custom-check:<id>` matches the override for `gate-custom-check`.

The chosen backend is replaced by `claude` when it is unregistered (`unsupported_program`), unavailable (`<name>_unavailable`), or lacks a capability the call needs (`<name>_lacks_<cap>`, e.g. `gemini_lacks_tool_restriction`). If `claude` cannot serve the call either, it fails with `ErrNoBackend`. Every fallback is logged and appended to `<config dir>/llm_backend_fallbacks.jsonl`.

Calls needing a working directory with `AllowedTools`/`DisallowedTools`/`PermissionMode` need the tool-restriction capability, so they only reach `claude` or `consolette`.

## Settings (live, no env vars)

`config.json` → `llm_backends`: `default`, `per_feature`, `consolette_base_url` (default `http://127.0.0.1:47000`), `anthropic_base_url`, `model_maps`. Edit through Settings → LLM Backends or the `LLMBackendService` RPCs (`GetLLMBackendSettings`, `UpdateLLMBackendSettings`); changes apply on the next call.

Model names: `model_maps[backend][alias]` (alias = `haiku`/`sonnet`/`opus`, matched against full Claude IDs by family). Unmapped names pass through for `claude`/`consolette` and become "backend default" for the others. Built-in gemini defaults are in `builtinModelMaps`; the consolette router's real model names are unverified — set them in the UI.

## Call sites

Routed through the selector: backlog triage/review/intent (`BacklogService`), session tagging, session/handoff summaries, PR description drafting, autonomous drivers and approvals, `RunOneShot` custom prompts, unfinished-work summaries, rules generation (`WrapRulesAIClient`, only when the `rules-generation` feature is routed away from claude — otherwise the original CLI chain runs).

`Instance.RunWithResume` stays a claude subprocess but declares `Resume` and resolves `instance-resume` through `headless.ResumeEnv`, so it can never land on a backend without resume.

## Capacity monitor

`AnthropicLimitsClient` keeps the fixed `anthropicAPIURL`; `anthropic_base_url` only affects `AnthropicAIClient`. Guarded by `TestAnthropicHTTPClientBaseURLConfigurableButProbeIsNot`.
