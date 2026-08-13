# Requirements: Headless LLM Interface

## Problem Statement

The current codebase has two patterns for making AI calls that are both suboptimal:

1. **`RunOneShot`** (`server/services/session_service.go:2743`) — spawns a fresh `claude -p` process per call with no session resumption, hardcoded 300s max timeout, no streaming, no caching strategy. Each call pays full input token cost.

2. **`SpawnReviewSession`** (`server/services/session_service.go:464`) — creates a full heavyweight interactive tmux session just to run a review prompt. This is expensive in process overhead, file descriptors, and tmux state for what is a stateless one-shot LLM call.

Neither approach takes advantage of:
- Session resumption (`--resume <id>`) for system-prompt caching (~3-4x token reduction)
- `--exclude-dynamic-system-prompt-sections` for stable prefix caching
- Streaming output for UI responsiveness
- The OAuth tokens already present in `~/.claude/` (no `ANTHROPIC_API_KEY` required)

## Goal

Build a `session/headless` Go package that wraps `claude -p` with session-pool management, streaming output, and cache-optimized calling conventions. Wire it into existing features (review gate, `RunOneShot`) and use it as the foundation for new background AI features.

## Scope

### In scope

- **`session/headless` package**: Core headless caller with per-feature session pools
- **Session pool management**: Per-feature pool key, ~25-call rotation, error-triggered reset, concurrency-safe
- **Streaming output**: `Call()` returns `<-chan StreamChunk` for incremental text; `CallBlocking()` for convenience
- **Cache optimization**: Always pass `--exclude-dynamic-system-prompt-sections`; use `--output-format json` on first call to capture `session_id`, plain output on resumed calls
- **Auth via OAuth**: Uses claude CLI's existing `~/.claude/` OAuth tokens — no `ANTHROPIC_API_KEY` required
- **Replace `SpawnReviewSession`**: Rewrite review gate to use headless pool instead of creating a tmux session
- **Replace `RunOneShot`**: Route through headless package with streaming support
- **New background features** (each as a thin service on top of headless):
  - Backlog item summarization + acceptance criteria generation
  - PR description drafting (triggered when session pushes a branch)
  - Commit message suggestion (from git diff)
- **`RunHeadlessCall` RPC**: New ConnectRPC endpoint so the frontend can trigger headless calls directly and stream results

### Out of scope

- Direct Anthropic SDK (`anthropic-go` client) — OAuth path only, SDK path not needed
- Replacing full interactive Claude sessions (those remain tmux-based)
- Multi-model routing (always uses claude CLI; model selection via flag only)
- Rate limit backpressure beyond what claude CLI already handles

## Functional Requirements

### FR-1: Session Pool (session/headless)

- Pool keyed by a `FeatureKey` string (e.g., `"review"`, `"summarize"`, `"pr-description"`)
- Each key has exactly one active session at a time
- Session established on first call via `--output-format json` to capture `session_id`
- Subsequent calls use `--resume <session_id>`
- Session rotated after `maxCalls` (default 25) or on any non-zero exit from the subprocess
- Global pool singleton (`headless.DefaultPool`) with option to create named pools for testing

### FR-2: Streaming Output

- `Call(ctx, featureKey, systemPrompt, userPrompt) (<-chan StreamChunk, error)` — starts a goroutine, streams chunks, closes channel on completion or error
- `CallBlocking(ctx, featureKey, systemPrompt, userPrompt) (string, error)` — convenience wrapper, collects all chunks
- `StreamChunk` carries `{ Text string; Err error; Done bool }`
- Context cancellation stops the subprocess and closes the channel

### FR-3: Cache Optimization

- Always pass `--exclude-dynamic-system-prompt-sections`
- First call in a session: `--output-format json` → parse `session_id` and `result`
- Resumed calls: plain output (session carries system prompt in server-side context)
- System prompt passed via `--system-prompt <text>` to enable clean prefix separation from user message
- Model configurable per call; default from `config.DefaultModel()` (falls back to `"sonnet"`)

### FR-4: Auth

- No `ANTHROPIC_API_KEY` required
- Subprocess inherits the calling process's environment (claude CLI picks up `~/.claude/` OAuth tokens automatically)
- If claude binary is not in `PATH`, return a clear `ErrClaudeNotFound` error

### FR-5: Review Gate (replace SpawnReviewSession)

- `BacklogLifecycleListener.spawnReviewGate` uses headless pool instead of `sessionCreator.SpawnReviewSession`
- `SpawnReviewSession` interface and `ReviewGateSpawner` interface are removed
- Review prompt runs via `headless.DefaultPool.CallBlocking(ctx, "review", systemPrompt, prompt)`
- Review session result is parsed and persisted via `SaveReviewVerdict` as before

### FR-6: RunOneShot Enhancement

- `RunOneShot` RPC routes through `headless.DefaultPool.Call()` with streaming
- Response streams chunks back via `ServerStream` (or collects + returns if no streaming RPC yet)
- Timeout raised to 900s default, configurable via request field

### FR-7: New Background AI Features

Each feature is a thin service function that calls `headless.DefaultPool.CallBlocking()`:

- **Summarize**: `SummarizeBacklogItem(ctx, item) (string, error)` — one-paragraph summary + suggested tags
- **Generate AC**: `GenerateAcceptanceCriteria(ctx, title, description string) ([]string, error)` — 3-5 testable AC items
- **Draft PR description**: `DraftPRDescription(ctx, diff, branchName string) (string, error)` — Conventional Commit-aware PR body
- **Suggest commit message**: `SuggestCommitMessage(ctx, diff string) (string, error)` — Conventional Commit format

### FR-8: RunHeadlessCall RPC

- New proto message `RunHeadlessCallRequest { string feature_key; string system_prompt; string user_prompt; string model; int32 timeout_seconds; }`
- New proto message `RunHeadlessCallResponse { string text; bool done; bool is_error; string error_message; float cost_usd; }`
- Server-streaming RPC `RunHeadlessCall` that streams `RunHeadlessCallResponse` chunks as they arrive
- Feature keys exposed: `"review"`, `"summarize"`, `"pr-description"`, `"commit-message"`, `"custom"` (for ad-hoc UI calls)

## Non-Functional Requirements

### NFR-1: Concurrency

- Pool operations are concurrency-safe (one mutex per feature key)
- Calls on the same feature key are serialized (no concurrent session sharing — this avoids the "concurrent session conflict" pitfall from the headless scripting wiki)
- Calls on different feature keys execute in parallel

### NFR-2: Resource Limits

- Maximum 5 concurrent feature sessions globally (configurable)
- Subprocess timeout enforced via `context.WithTimeout`; cleanup via `cmd.Process.Kill()` on timeout

### NFR-3: Observability

- Pool stats logged at startup and on rotation: `session_id`, call count, reason for rotation
- Cost logged per call from `--output-format json` `cost_usd` field (first call in session only; estimated for resumed calls)
- Structured log fields: `feature_key`, `session_id`, `call_count`, `cost_usd`, `duration_ms`

### NFR-4: Testability

- Pool accepts a `ClaudeRunner` interface (one method: `Run(ctx, args []string) (stdout io.Reader, err error)`)
- `FakeRunner` in test files for deterministic unit testing without spawning claude
- Integration tests use real claude binary (skipped if `CLAUDE_INTEGRATION_TESTS=false`)

### NFR-5: Backward Compatibility

- `RunOneShot` RPC signature unchanged (enhanced internally)
- `SpawnReviewSession` interface removed but existing callers updated in the same PR

## Acceptance Criteria

- [ ] `session/headless` package builds with `go build ./session/headless/...`
- [ ] Unit tests pass with `FakeRunner` (no real claude needed)
- [ ] `SpawnReviewSession` is removed; review gate uses headless pool
- [ ] `RunOneShot` routes through headless package
- [ ] All 4 new AI feature functions compile and have unit tests
- [ ] `RunHeadlessCall` RPC is registered and returns chunks for a simple prompt (integration test)
- [ ] `make ci` passes

## Context: Best Practices from Headless Scripting Wiki

From `[[Claude Code Headless Scripting]]` (synthesized 2026-05-25):

```
# First call — establish session, capture ID
claude -p \
  --system-prompt "..." \
  --output-format json \
  --exclude-dynamic-system-prompt-sections \
  "user prompt" | jq -r '.session_id'

# Resumed calls — only new user turn billed at full price
claude -p \
  --resume "$SESSION_ID" \
  --exclude-dynamic-system-prompt-sections \
  "next user prompt"
```

Critical pitfalls to avoid:
- Never share one session across concurrent workers — use separate sessions per concurrent caller
- Rotate sessions every ~25 calls to cap context growth
- Always check `result.stderr or result.stdout` for error details (non-zero exit puts details in stdout)
- Use `--output-format json` only on first call; plain output on resumed calls (session_id already known)
