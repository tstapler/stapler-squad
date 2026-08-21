# Research Plan: LLM Omnibar

## Subtopics

### 1. Stack
**Focus**: The three LLM backend options and their Go integration
- Claude CLI subprocess (`claude -p`) — latency, auth, structured output via `--output-format json`
- Anthropic Go SDK — availability, structured output (tool use / JSON mode), streaming
- AWS Bedrock Go SDK — Claude model access, credential chain, latency vs direct API
**Axes**: latency, auth complexity, structured output support, error handling, Go SDK maturity
**Search cap**: 4 searches
**Output**: `findings-stack.md`

### 2. Features
**Focus**: UX patterns for NL → structured form pre-fill; comparable tools
- Raycast AI, Linear AI, VS Code Copilot Chat command intent, Cursor composer
- How they handle ambiguity, low-confidence results, and form pre-fill
- What structured fields they extract and how they present the preview
**Axes**: UX clarity, confidence handling, structured output fidelity, latency tolerance
**Search cap**: 4 searches
**Output**: `findings-features.md`

### 3. Architecture
**Focus**: Design of pluggable IntentParser interface; prompt engineering for reliable JSON; MCP tool-use
- Interface design: Go interface + factory; config-driven backend selection
- Prompt engineering: tool use vs JSON mode vs constrained generation for reliable structured output
- MCP tool-use during intent resolution: giving the LLM `list_sessions`, `search_sessions` as tools
- Sharing the headless `/api/sessions/intent` endpoint with the UI and MCP tool
**Axes**: reliability of structured output, extensibility, testability, latency budget
**Search cap**: 4 searches
**Output**: `findings-architecture.md`

### 4. Pitfalls
**Focus**: Known failure modes for each component
- CLI subprocess: cold start (~1-3s), PATH issues, auth expiry mid-request, zombie processes
- Structured output: LLM hallucinating paths/branches that don't exist, JSON parse failures
- Bedrock: credential rotation complexity, region/model availability, different API shape
- Context window: how much starter context is too much; truncation strategies
- MCP tool calls during intent: latency budget, infinite loops, tool errors
**Axes**: likelihood, severity, mitigation difficulty
**Search cap**: 3 searches
**Output**: `findings-pitfalls.md`

## Execution Order
All 4 subtopics run in parallel (no dependencies between them).
Parent agent runs web searches after all subagents complete.
Synthesis written after web search results are appended.
