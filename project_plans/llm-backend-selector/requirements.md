# Requirements: llm-backend-selector

## Problem
Programmatic LLM calls are hard-wired to the `claude` CLI. Low-value work (tagging, summaries, intent parse, PR drafts, rules generation) cannot move to the free-tier consolette proxy (127.0.0.1:47000, Anthropic-compatible) or to agy/gemini/opencode. Source: 2026-10-04 quota audit.

## Current state (verified in code)
- `headless.PoolClient` (session/headless/client.go) — one method `CallBlocking(ctx, FeatureKey, sys, user, CallOptions, CostSink)`; implemented by `*Pool` (claude) and `*GeminiCaller`.
- Only `BacklogService.headlessCallers` (backlog_service.go ~583-633) selects by name, and only for stage executors.
- `AIClient`/`CLIAgentSpec` (server/services/ai_interfaces.go, cli_ai_client.go): separate chain claude→gemini→agy→opencode→HTTP, for rules generation.
- `ProgramConfig{Command,CLIFlags,Env}` (config/types.go:487): interactive tmux only.
- Hard-coded bypasses: unfinished_work_service.go:371, session_service.go:5798, instance_claude.go:505 (`claude -p --resume`), anthropic_client.go:13 base URL constant.
- Many feature callers go through `CallBlocking` with a `FeatureKey` (features.go, approval_handler.go, gate_custom_check.go, backlog triage/intent).

## Goals
1. One selector: global default backend + per-feature override (keyed by `FeatureKey`).
2. One provider interface implemented for: claude, Anthropic-compatible base-URL backend (consolette), agy, gemini, opencode.
3. Model-name translation per backend; capability flags (resume, system prompt, tool restrictions, WorkDir).
4. Settings live-editable (config + settings RPC/panel), NO env vars.
5. Capacity monitor's Anthropic probe is unaffected by backend choice.

## Non-goals
- Changing interactive session `Program` selection.
- Replacing the capacity monitor probe.

## Acceptance criteria
1. A `Backend` provider interface exists with claude, anthropic-compat (base URL), agy, gemini, opencode implementations, each declaring capability flags.
2. Config holds a global default backend and per-feature overrides; changes apply without restart and without env vars.
3. All `headless.Pool`/`CallBlocking` feature sites and the AIClient rules-generation path resolve their backend via the selector.
4. Hard-coded sites (unfinished_work_service, session_service custom prompt, instance_claude resume, HTTP AnthropicAIClient base URL) go through the selector or explicitly declare a required capability.
5. Requests needing an unsupported capability (e.g. resume on gemini, WorkDir/tools on HTTP backend) fall back to claude (or fail closed) with a logged, persisted reason.
6. Model names are translated per backend (e.g. sonnet/haiku aliases → consolette/gemini model IDs).
7. The consolette backend is selectable by base URL and reports unavailability when 127.0.0.1:47000 is down, falling back per AC5.
8. Cost reporting via `CostSink` works (or reports unpriced) for every backend.
9. Settings UI/RPC to set default and per-feature backend; registry docs updated.
10. Capacity monitor probe verified unchanged by tests.
11. Unit tests with fake backends; `make ci` green.
