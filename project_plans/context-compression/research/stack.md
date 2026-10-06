# Stack Research: Context Compression

Scope: resolve the technology-stack questions for the context-compression backlog
item, building on the confirmed prior finding that `session/claude_controller.go`
supervises the `claude` CLI as a subprocess (tmux/PTY), not the raw Anthropic API,
and on the adjacent unimplemented research in `project_plans/context-compaction-detection/`
(PTY-pattern detection of the CLI's own compaction spinner) and
`project_plans/context-health-monitoring/` (broader degradation UX).

## 1. Does the `claude` CLI expose context-usage percentage programmatically?

**No structured "current context %" query exists for the interactive session this
feature targets.** Two distinct invocation modes exist in this codebase, and only one
of them uses structured JSON — and it is not the one that stays open long enough to
need compression:

- **Interactive / long-running session** (what `claude_controller.go` and
  `Instance.buildClaudeCommand` launch for normal work) — plain PTY output, `--resume
  <session_id>`, no `--output-format` flag at all
  (`session/instance_tmux.go:158-165`: `--output-format json` is only appended
  `if i.OneShot`). This is the mode a multi-hour work session runs in, and it emits no
  machine-readable usage/context metadata to the controller — only terminal text,
  which is why `context-compaction-detection`'s approach (PTY regex matching on the
  CLI's own status-line text, e.g. "N% until auto-compact") is the only detection
  mechanism available at that layer.
- **One-shot / headless calls** (`session/headless/`, review/triage/summary
  generation) — `-p --output-format json` returns a single JSON object:
  `{"type":"result","result":"...","session_id":"uuid","cost_usd":0.012}`
  (`docs/project_plans/headless-llm/research/stack.md:66`, confirmed against real
  parsing code in `session/session_driver.go:790` `parseJSONField`/
  `parseClaudeSessionID`, and test fixtures in `session/session_driver_test.go:519`
  showing `total_cost_usd`). This schema has cost, not a context-window percentage,
  and it's only available for short one-shot calls, not the interactive session that
  accumulates the context this feature is meant to protect.

**No CLI flag, MCP resource, or status file was found in this repo's code or docs
that exposes a live "% of context window used" figure for a running interactive
session.** The closest thing that exists is what Claude Code shows in its own PTY
status line (already the subject of `context-compaction-detection`) and the
`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` env var (external, confirmed below) which only
*sets* the trigger threshold, it doesn't let another process *read* current usage.

### Existing infrastructure that changes the picture: `session/tokens`

Separately from the CLI's live output, this codebase already has a fully built,
already-shipped package — `session/tokens/` (`Parser`, `TokenStore`,
`ParseResult`, `TurnStats`, `PricingTable`, `Associator`) — that parses Claude Code's
**own persisted JSONL transcript files** (`~/.claude/projects/<encoded-path>/<uuid>.jsonl`)
line-by-line and extracts the exact `usage` object Anthropic's API returned for each
assistant turn:

```go
// session/tokens/jsonl_types.go
type jsonlUsage struct {
    InputTokens              int64 `json:"input_tokens"`
    OutputTokens             int64 `json:"output_tokens"`
    CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}
```

`ParseResult.TurnTimeline []TurnStats` (`session/tokens/types.go`) is **already
per-turn token usage**, populated from real Claude Code transcripts, with a
background fsnotify-driven `TokenStore` that keeps it fresh as the file grows
(`session/tokens/store.go`). It's wired into `SessionSummaryGenerator` today
(`session/session_summary_service.go:69,133,377`) for cost/summary display, and into
`InsightsService` (`server/services/insights_service_test.go`).

This means **acceptance criterion 1 ("Token usage tracked per turn from Claude API
response metadata") is effectively already solved in this codebase** — just not by
literally reading "API response metadata" in `claude_controller.go` (which can't, per
above), but by parsing the CLI's own transcript file, which contains the same `usage`
object the requester's Hermes reference implementation reads directly from API
responses. Any compression-threshold feature should build on `session/tokens`, not
reimplement token tracking.

**Gap confirmed**: nothing in `session/tokens` or elsewhere in the repo computes a
**context-window percentage** — no `ContextWindow`/`context_length` constant per
model family was found (`grep -rl "ContextWindow\|context_window\|context_length"`
across `*.go`/`*.ts`/`*.tsx` returned zero hits outside unrelated capacity-monitor
files for a different provider-limits feature: `server/services/anthropic_limits_client.go`,
`provider_limits.go`, `capacity_monitor.go` — these track API rate/usage limits, not
context-window occupancy). `TokenStore` gives cumulative/per-turn token *counts*, not
counts-as-percent-of-window; a compression feature would need to add a per-model
context-window-size table (mirroring `PricingTable`'s per-model-family map pattern in
`session/tokens/pricing.go`) to turn raw token counts into the 85%-threshold check
the requirements ask for.

## 2. Go tokenization libraries in go.mod

**None.** `grep -n "tiktoken\|Tokenizer\|tokenizer\|encoding" go.mod go.sum` found no
tokenizer library (no `tiktoken-go`, `pkoukk/tiktoken-go`, or similar) — the only
`encoding` hit is the unrelated `github.com/segmentio/encoding` (fast JSON, not a
tokenizer). This is moot for the primary mechanism above: since `session/tokens`
already extracts exact token counts from the CLI's transcript `usage` field (the real
count the API billed, not an estimate), there is no need to estimate tokens
client-side via a tokenizer library — the existing package sidesteps that problem
entirely for anything that already appears in a completed turn. A tokenizer would
only become relevant if something needed to *predict* token count before sending
(e.g. pre-flight sizing of a synthetic injected message), which is a much narrower
need than general usage tracking.

## 3. Does Claude Code CLI already do what this backlog item asks, natively?

**Largely yes**, reinforcing the requirements doc's own "open architectural question."
Confirmed via web search (not previously covered by the two sibling research passes,
which focused on PTY string detection, not configurability):

- **Auto-compact already exists and already has a configurable threshold**, via the
  `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` environment variable (accepts 1-100, percentage of
  context window at which auto-compaction triggers) and `CLAUDE_CODE_AUTO_COMPACT_WINDOW`
  (shrinks the effective window). Reported default is in the ~83-92% range depending on
  source — close to, though not identical to, this backlog item's proposed 85% default.
  [How to Configure Claude Code Auto-Compact Settings with Environment Variables](https://docs.bswen.com/blog/2026-03-21-claude-code-auto-compact-settings/),
  [Feature: configurable context compaction threshold · Issue #41818](https://github.com/anthropics/claude-code/issues/41818).
- **`/compact` already supports custom, explicit summarization instructions** —
  `/compact <instructions>` lets the operator (or a wrapper) tell it exactly what to
  preserve, and a `compactInstructions` settings key provides a default when invoked
  bare. This is conceptually the same lever the backlog item's `compress()` step wants
  (head/tail protection, "## Active Task" preservation) — Claude Code exposes it as
  free-text steering rather than a rigid head/tail/N-turn algorithm.
  [FEATURE: Add compactInstructions setting](https://github.com/anthropics/claude-code/issues/55905),
  [How to Use the /compact Command](https://www.mindstudio.ai/blog/claude-code-compact-command-context-management).
- **What Claude Code does *not* natively provide**, per the same search and consistent
  with §1 above: a way for a *supervising* process (i.e., Stapler Squad's Go
  controller) to (a) read live context-usage percentage without PTY-scraping, or (b)
  inject a synthetic REFERENCE-ONLY-prefixed summary message from outside the CLI's
  own compaction pipeline. Multiple open feature requests
  ([#14160](https://github.com/anthropics/claude-code/issues/14160),
  [#28728](https://github.com/anthropics/claude-code/issues/28728)) ask Anthropic for
  exactly this kind of external controllability and are unresolved as of this search.

### Net implication for this backlog item

This converges with what `context-compaction-detection` already concluded for its own
narrower scope: the CLI already owns threshold-based compaction end-to-end (detection,
configurable trigger, and content-preservation-via-instructions). The literal proposed
work items 1 and 3 in requirements.md — "track token usage from Claude API response
metadata in claude_controller.go" and "inject a compression turn... as a synthetic user
message" — are not things the Go controller can do to a live interactive session (no
API metadata is visible to it; no injection channel exists outside the CLI's own PTY
input, which would just be typed text, not a privileged synthetic-message API). The
gap Stapler Squad can actually close, using infrastructure that already exists in this
repo, is: (a) surface the CLI's own compaction events/threshold in the session-detail
UI (`context-compaction-detection`'s scope), and (b) build a "context budget" view on
top of the already-shipped `session/tokens` `TurnTimeline` data — e.g. an
operator-visible running percentage once a per-model context-window-size table is
added — rather than reimplementing token tracking or message injection that the CLI
already does more capably from inside the process.

## Summary of concrete file touchpoints identified

| Question | File(s) | Finding |
|---|---|---|
| Interactive session launch args | `session/instance_tmux.go:100-165` | No `--output-format` for interactive/resumed sessions; only one-shot (`i.OneShot`) gets `-p --output-format json` |
| One-shot JSON schema | `session/session_driver.go:742-800`, `session/headless/caller.go:52-54` | `{"session_id","result","cost_usd"}` — no token/context fields |
| Existing per-turn token tracking | `session/tokens/{types,parser,jsonl_types,store}.go` | Already parses real `usage` objects (input/output/cache tokens) per turn from Claude's own JSONL transcripts; wired into `SessionSummaryGenerator` and `InsightsService` today |
| Context-window-size table | *(does not exist)* | Gap — would need a new per-model-family map, same shape as `session/tokens/pricing.go`'s `PricingTable` |
| Tokenizer library | `go.mod` | None present (`tiktoken-go` etc. absent); not needed given `session/tokens` reads exact billed counts |
| CLI native auto-compact config | external (web) | `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW` env vars; `/compact <instructions>` and `compactInstructions` setting |
