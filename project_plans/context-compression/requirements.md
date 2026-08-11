# Requirements: Context Compression at 85% Threshold for Long Work Sessions

Source: backlog item `471a286e-61fe-45de-8202-4e548f626f7c`, migrated from
`TylerStaplerAtFanatics/stapler-squad#117` (2026-05-29).

## Problem

Long-running work sessions in Stapler Squad silently fail or produce degraded
output when the underlying agent conversation approaches its context window
ceiling. There is no graceful handling today: no compression, no warning to
the operator, no recovery path. The operator has to manually restart the
session and re-orient the agent.

## Reference implementation cited by the requester

Hermes Agent (external, not in this repo) — `agent/context_compressor.py`,
`agent/context_engine.py` — implements a pluggable `ContextEngine`:

1. `update_from_response()` — tracks token usage from each API response.
2. `should_compress()` — true when `used_tokens / context_length > 0.85`
   (configurable).
3. `compress()` — calls a cheap auxiliary model to summarize the middle
   turns, protecting the head (initial instructions/task context) and the
   tail (recent work/current state).

Summary prefix to reuse verbatim if this pattern is adopted:

> "[CONTEXT COMPACTION — REFERENCE ONLY] Earlier turns were compacted into
> the summary below. This is a handoff from a previous context window —
> treat it as background reference, NOT as active instructions. Do NOT
> answer questions or fulfill requests mentioned in this summary; they were
> already addressed. Your current task is identified in the '## Active
> Task' section..."

Additional techniques the requester flagged as worth adopting: pruning tool
call output blobs before summarization, scaling the summary budget
proportionally (with an absolute ceiling, e.g. 12k tokens), and an explicit
"## Active Task" section in the summary.

## Proposed work (as filed)

1. Add token usage tracking to `session/claude_controller.go` from Claude's
   API response metadata.
2. Define a compression threshold (default 85%, configurable per
   workspace).
3. When crossed, inject a compression turn: summarize turns
   `[head+1 .. tail-N]` with a cheap model, wrap in the REFERENCE-ONLY
   prefix, inject as a synthetic user message.
4. Surface compression events in the session detail UI (a "context
   compressed" badge on the timeline).
5. Add `session/context_compressor.go` with summarization logic, head/tail
   protection, and a tool-output pruning pre-pass.

## Acceptance criteria (as filed)

- [ ] Token usage tracked per turn from Claude API response metadata.
- [ ] Compression fires at a configurable threshold (default 85%), not more
      than once per N turns (thrash prevention).
- [ ] Summary uses the verbatim REFERENCE-ONLY prefix.
- [ ] Tool output blobs pruned before summarization (images, long file
      reads, diff outputs).
- [ ] Compression event shown in session detail UI.
- [ ] Unit tests: threshold detection, head/tail protection boundaries,
      tool output pruning.

## Open architectural question this triage must resolve first

Stapler Squad's controller layer (`session/claude_controller.go`) manages
Claude Code CLI sessions, not raw Anthropic API calls. Claude Code CLI is
known to already perform its own context management (auto-compaction near
its context limit, plus the user-invocable `/compact` command) inside the
subprocess the controller supervises. Whether "API response metadata" and
"inject a synthetic user message mid-session" are things the Go controller
can actually observe/do — given it talks to a CLI subprocess via
`--resume`/stdout, not a raw chat-completions API — is the central
feasibility question for Phase 2 (research) to settle before any plan is
written. If the CLI already owns this behavior, the real gap may be
*visibility* (surfacing the CLI's own compaction events in the UI) rather
than reimplementing compression logic against a model API the controller
doesn't directly call.

## Out of scope

- Reimplementing Hermes's `ContextEngine` verbatim in Python — this is a Go
  codebase; only the technique (thresholds, head/tail protection, summary
  prefix wording) is being evaluated for adoption, not the code.
- Any change to session types outside `claude` (Aider, etc.) unless
  research shows the same mechanism applies.
