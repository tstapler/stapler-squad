# Plan: llm-backend-selector

## Architecture
- New package `session/headless/backend`(or in headless): `Backend` interface = `PoolClient` + `Capabilities() Caps{Resume,SystemPrompt,ToolRestriction,WorkDir}` + `Available() bool` + `Name()`.
- Implementations: `claude` (existing Pool), `consolette` (Pool/claude CLI with ANTHROPIC_BASE_URL from settings), `cli` backend generic over CLIAgentSpec (agy, opencode), `gemini` (existing GeminiCaller).
- `Selector` : `Resolve(feature FeatureKey, need Caps, stageProgram string) (Backend, Resolution)`; precedence stage program > per-feature override > global default; capability/availability miss -> claude with reason (generalizes resolveHeadlessCaller). Reads settings via atomic pointer snapshot (live).
- `ModelMap` per backend translating aliases (sonnet/haiku/opus) to backend IDs.
- Settings: `config` struct `LLMBackends{Default, PerFeature map, ConsoletteBaseURL, ModelMaps}` + RPC + settings panel; no env vars.
- Selector implements `PoolClient` wrapper (`SelectingClient{feature}`) so existing call sites change by injection only.

## Tasks
1. Define Backend/Caps/Selector + precedence/fallback logic with tests.
2. Settings model, persistence, live reload (atomic snapshot), proto RPC.
3. Consolette backend (claude CLI + base URL) with availability probe.
4. CLI backend for agy/opencode (reuse CLIAgentSpec), adapt GeminiCaller to Backend.
5. Model-name translation maps.
6. Migrate headless feature sites + BacklogService.headlessCallers to selector.
7. Migrate AIClient rules generation + configurable HTTP base URL.
8. Handle hard-coded sites (unfinished_work, session_service, instance_claude) via capability declarations.
9. Cost reporting parity.
10. Settings UI panel (desktop+mobile).
11. Tests: fakes, fallback matrix, capacity-monitor-unaffected test.
12. Docs (docs/reference) + feature registry.

## Adversarial review
- Silent quality regression: default claude; override opt-in; log+persist backend used per call.
- Typed-nil in interface registration (see SetGeminiCaller comment): constructors return interface only when non-nil.
- Tool-restricted calls to weaker backends: fail closed.
- gocognit/dupl gates: share one CLI runner.
- Capacity monitor must keep own Anthropic credential path.
