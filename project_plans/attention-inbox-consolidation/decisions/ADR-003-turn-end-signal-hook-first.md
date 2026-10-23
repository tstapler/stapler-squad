# ADR-003: Hook-first turn-end signal with per-program screen fallback

**Status**: Proposed (payload shape gated on Spike 0.1.1)
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Context

Nothing in the stack produces a "turn ended" fact. `HandleStop` (`server/services/hook_receivers.go:65-70`) drains and logs; `scripts/ssq-hook-handler` `stop` (lines ~448-463) always emits `Task Complete` LOW using `.stop_reason // .reason`, never reading the transcript. Screen patterns (`session/detection/binaries/claude.go`) never classify prose. Regexing a `?` on screen text flags most finished turns (agents end almost every turn with a question-like sentence).

## Decision

1. **Trigger** = Claude `Stop` hook. `HandleStop` parses a bounded body (existing 64 KB `readBody` cap), resolves the session, and records a `TurnEnded{Episode, Kind, Excerpt, Source}` fact on the Instance via an actor setter published to `InstanceSnapshot` (per `.claude/rules/instance-lock-free-reads.md`).
2. **Text source** = last assistant message. Spike 0.1.1 decides between a payload field and reading the tail of `transcript_path` JSONL. Official docs show `transcript_path` and `stop_hook_active`; `last_assistant_message` appears only in third-party guides (UNVERIFIED), so the design assumes the transcript tail and treats a payload field as an optimization.
3. **Classifier** = pure function `ClassifyTurnEnd(text) (TurnEndKind, Confidence)`; heuristic first (trailing question, interrogative lead-ins, enumerated options, no pending tool_use). Applied to the last assistant message. The only screen-derived input is the screen-fallback stage (point 5, Story 3.1.3), which extracts the bottom-most prose paragraph above an idle prompt first (never a raw screen tail) and ships `Finished`-only until a labelled per-program corpus gates `Question`. When unsure it returns `Finished` (low priority), not `Question`.
4. **LLM tiebreak** = optional second stage behind `attention_prose_question_llm_tiebreak` (default off), via the existing `session/headless` pool (`server/llm_selector.go`), one flight per episode, cache by message hash, timeout fails open to a visible low-priority item, output constrained to an enum, never on the poll path, raw text redacted before send. Adversarial text ("ignore previous instructions") is in the fixture corpus.
5. **Non-Claude programs** (aider, agy, pi, gemini, opencode) have no Stop hook; a per-program signal table maps each to {hook | screen heuristic | idle-for-N | none}, and "none" is shown as "cannot detect", not silently omitted. Coverage is measured per program on a fixture corpus.
6. `ssq-hook-handler stop` and the HTTP endpoint must not both emit: `HookDeduper` (`server/services/hook_deduper.go`) keys the episode so one stop yields one fact and at most one notification.

7. **Phase 0 go/no-go (added in the Phase 4 patch)**: this design rests on the Stop hook firing for the sessions the operator actually runs. Spikes 0.1.1/0.1.4 must record per-creation-path coverage. If coverage is below the exit criterion in plan.md Story 0.1.5, Phase 3 switches to the named fallback recorded there (F1: throttled transcript-tail polling for Claude; F2: `Finished`/`awaiting_direction` only with "cannot detect" shown, no prose-question kind), decided before Story 3.1.1 starts. Per-program hook coverage is also a precondition for flipping `attention_prose_question_detection` on.

## Alternatives rejected

Screen-regex only (is the bug); LLM as primary detector (cost, nondeterminism, outage hides approvals); external inbox/router products (build-vs-buy: none own the "session needs me" domain logic).

## Consequences

Claude coverage is near-exact; other programs are weaker and visibly labeled. Excerpts persisted are capped and scrubbed (ADR-004).
