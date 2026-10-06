# ADR-001: Reframe "Context Compression" as Restart-With-Handoff-Summary, Not Live Mid-Session Injection

**Status**: Accepted
**Date**: 2026-08-06

## Context

`requirements.md` (migrated from `TylerStaplerAtFanatics/stapler-squad#117`) proposed
porting Hermes Agent's `ContextEngine` pattern: track token usage from API response
metadata, and at an 85% threshold, call a cheap model to summarize the middle turns
of the conversation and splice the summary back in as a synthetic message, protecting
the head (original task) and tail (recent work) from being summarized away.

All six Phase 2 research passes (`research/stack.md`, `research/features.md`,
`research/architecture.md`, `research/pitfalls.md`, `research/ux.md`,
`research/build-vs-buy.md`) independently converged on the same finding:
**this design is not achievable against stapler-squad's actual architecture.**

- `session/claude_controller.go` drives Claude Code CLI as an interactive subprocess
  inside a tmux pane — it talks to the CLI via PTY bytes and `--resume <uuid>`, never
  via a raw Anthropic Messages API call it owns. Confirmed at
  `session/claude_controller.go:452-514` (`SendCommand`/`SendCommandImmediate`) and
  `session/instance_tmux.go:105-166` (no `--output-format` for the interactive/resumed
  case).
- There is no messages array for stapler-squad to edit. Hermes's `compress()` works
  because Hermes owns the `messages` array sent to a chat-completions API and can
  delete/replace turns before the next call. stapler-squad's only channel into a live
  CLI subprocess is `SendCommand`, which types literal keystrokes into the pane
  (`session/command_executor.go:297-304`) — indistinguishable from a human typing.
  This channel can only **append** a new turn at the current tail; it cannot remove or
  replace anything the CLI subprocess already holds and will resend to the model on
  its next request.
- Consequently, "inject a synthetic message that reduces context" does not compress
  anything even if built — it would add tokens on top of an already-full context, the
  opposite of the stated goal (`research/build-vs-buy.md` Option 1).
- Claude Code CLI already runs its own auto-compaction near its context ceiling,
  with head-preserving, LLM-summarized behavior stapler-squad's controller cannot
  replicate from outside the process (`research/pitfalls.md` §1, `research/features.md`
  §4). A parallel Go-side compressor would race the CLI's own compaction with no
  coordination mechanism, risking double-summarization.
- Token tracking (AC-1) is separately, already solved by `session/tokens`, which
  parses the CLI's own JSONL transcript files — no `claude_controller.go` change is
  needed for that half of the original proposal regardless of this decision.

## Decision

Retarget this backlog item from "compress context live, mid-conversation" to
**"detect degradation, then offer a restart into a fresh session pre-seeded with an
AI-generated handoff summary"** — the follow-on `context-health-monitoring` named in
its requirements (`context-health-monitoring/requirements.md:9,24`) but explicitly
deferred out of its shipped plan (`implementation/plan.md`'s Explicit Follow-Ons #3).

This is `research/build-vs-buy.md`'s Option 3. Concretely:

1. A new `HandoffSummary` generation pipeline (mirroring the already-proven
   `SessionSummaryGenerator` async-dispatch/dedup/persist shape in
   `session/session_summary_service.go`) produces a REFERENCE-ONLY-prefixed,
   head/middle/tail-aware summary from the source session's real transcript content
   (`session/history.go`'s `ClaudeSessionHistory`, which — unlike `session/tokens` —
   retains actual message text).
2. A user (manually, or via a future RED-`ContextHealth`-gated auto-suggestion once
   `context-health-monitoring` ships) triggers "Restart with summary," which creates
   a **new** session in the same working directory, delivering the summary as the new
   process's spawn-time `prompt` (`CreateSessionRequest.prompt`, field 7) — a clean,
   timing-safe append at a fresh process start, not a race-prone mid-session
   injection into a live PTY.
3. The Hermes techniques (REFERENCE-ONLY prefix wording, head/tail-aware framing, an
   explicit "## Active Task" section, tool-output pruning, a proportional summary
   budget with an absolute ceiling) are preserved verbatim inside this new pipeline's
   prompt construction — the *techniques* transfer even though the *delivery
   mechanism* (append-at-restart vs. splice-mid-history) does not.

## Consequences

- Acceptance criteria 2 and 3 as originally filed ("compression fires... injects...
  as a synthetic user message") are **superseded**, not implemented — see
  `requirements.md`'s Acceptance Criteria vs. `implementation/plan.md`'s reframed set.
- No `session/context_compressor.go` is built. No token-usage-percentage tracking is
  added to `session/claude_controller.go`.
- This item now has a **soft dependency** on `context-health-monitoring` for its
  primary (RED-verdict-gated) trigger path — that project is itself unimplemented as
  of this writing. The manual "Restart with summary" trigger (a button always
  available in session detail) has no such dependency and can ship independently;
  see `implementation/plan.md`'s Unresolved Questions for the exact blocked story.
- `context-compaction-detection` (visibility into Claude Code's own native
  auto-compaction) remains a separate, already-fully-planned item and is unaffected
  by this decision — it addresses a different half of the original problem statement
  (silent degradation visibility, not restart/handoff).

## Alternatives Considered

See `research/build-vs-buy.md` for the full analysis. Summary:

| Option | Verdict |
|---|---|
| 1. Build custom live compressor (as filed) | Rejected — core injection mechanism cannot achieve compression given the PTY/subprocess architecture |
| 2. Surface Claude Code's native auto-compaction only | Adopted as a prerequisite (`context-compaction-detection`), but only satisfies the visibility half of the problem |
| 3. Restart with handoff summary | **Chosen** — reuses proven infrastructure, satisfies the intent behind the original ACs |
| 4. Hermes `ContextEngine` verbatim port | Not applicable (different language/architecture) — techniques adopted, code not ported |
