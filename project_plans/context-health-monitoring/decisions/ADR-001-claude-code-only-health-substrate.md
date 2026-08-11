# ADR-001: Scope ContextHealth v1 to Claude Code JSONL transcripts

**Date**: 2026-08-02
**Status**: Accepted
**Project**: context-health-monitoring
**Supersedes / conflicts with**: `research/build-vs-buy.md` Option 4 (which recommended `session/detection`)

## Context

`requirements.md` scopes ContextHealth to two heuristics: (1) 3+ consecutive same-tool-similar-args
tool calls, and (2) repeated apology/self-correction language in agent output. Two candidate
substrates exist in this repo, and the Phase 2 research documents disagree about which to use:

- `research/architecture.md` §1 recommends `session/tokens` (the Claude Code JSONL transcript parser).
- `research/build-vs-buy.md` Option 4 recommends `session/detection`'s `PatternSet`/`eventRing`.

The disagreement is decidable on facts, not preference:

- `PatternSet.MatchLines` classifies terminal text into one of ten status categories. It carries **no
  tool identity and no arguments at all** (`session/detection/pattern_set.go`). The per-session ring
  buffer stores only a matched category plus a 512-byte text snippet
  (`session/detection/events.go:8-26`). Heuristic (1) needs `(toolName, args)`, which this path
  does not have.
- `session/tokens`' parser already decodes `tool_use` blocks with the exact `Name` and full `Input`
  (`session/tokens/jsonl_types.go:34-45`) and assistant `text` blocks in the same walk. Both
  heuristics' inputs are already in hand there.

`session/tokens` only parses **Claude Code's** JSONL transcript format (`session/tokens/doc.go`).
Aider, Gemini, OpenCode, and Agy sessions write no such file. `research/pitfalls.md` §2 confirms the
asymmetry is already severe and observable today at the *detection* layer too:
`session/detection/binaries/claude.go` has ~30 tuned patterns while `binaries/aider.go` has empty
pattern slices for every category except `NeedsApproval`.

## Decision

**ContextHealth v1 computes signals exclusively from the Claude Code JSONL transcript via
`session/tokens`.** Sessions running any other agent binary produce `ContextHealth` =
`CONTEXT_HEALTH_UNSPECIFIED`, and the badge is suppressed for them.

Multi-binary support is a documented follow-on, not a gap in this plan.

## Consequences

**Positive**
- Both heuristics get their required inputs with **zero** additional file reads: the signals are a
  by-product of a parse that already happens on every transcript change.
- Reuses a pipeline already wired end to end (fsnotify → worker pool → cache → subscriber channel →
  ConnectRPC), rather than the `session/detection` plugin/hot-reload path, which
  `project_plans/detector-plugins/research/architecture.md` found has no existing precedent.
- No new goroutine per session, no new mutex — satisfies the NFR against new per-session lock
  contention.

**Negative**
- Aider/Gemini/OpenCode/Agy sessions get **no** health signal in v1. This is the cost being accepted
  in exchange for shipping inside the Medium (1–2 week) appetite `requirements.md` sets.
- The failure mode is *silence*, not a wrong answer: `HealthUnknown` → `CONTEXT_HEALTH_UNSPECIFIED` →
  `ContextHealthBadge` renders `null`. A non-Claude session shows no badge rather than a misleading
  green, which is exactly the behaviour `research/ux.md` §4 prescribes for "insufficient data".

**Mitigation / exit path**
If multi-binary support is later required, the natural extension is a second signal source
(e.g. the PostToolUse HTTP hook at `server/services/hook_receivers.go`, which already carries
`ToolName` + `ToolInput` live) feeding the *same* `ContextHealthSignals` struct and the *same*
`EvaluateContextHealth` function. Because thresholds are applied in a pure function over raw
signals — not inside the parser — adding a second producer requires no change to evaluation, the
proto surface, or the frontend.
