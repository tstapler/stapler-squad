# ADR-003: `ClassifyOSCTitle` Is a Free Function in `binaries/claude.go`, Not a `dtypes.BinaryDetector` Interface Method

**Status**: Accepted
**Date**: 2026-08-06
**Project**: osc-status-signals

## Context

Two research documents reach opposite leanings on the same question:

- `research/architecture.md` §5 (Option C) recommends **against** adding an OSC method to
  `dtypes.BinaryDetector`, citing `.claude/rules/interface-pollution-checklist.md`: the requirements'
  own Non-Goals section explicitly excludes "extending to binaries other than Claude Code," so 4 of
  the interface's 5 implementations (`gemini`, `aider`, `opencode`, `agy`) would gain a permanent
  no-op method solely to satisfy the interface shape — the checklist's "forwarding-only/speculative
  surface" smell.
- `research/features.md` §7 leans the other way: `BinaryDetector` already has 5 implementations (not
  literally speculative in the single-implementation sense), and the registry
  (`session/detection/registry.go`, `binary_detector.go`) is already structured per-binary, so an
  optional interface method with a no-op default would "leave a natural seam for later."

The task brief for this plan explicitly calls out this exact tension and requires it be resolved
here, in planning, with the rejected alternative recorded.

Two additional facts from research bear directly on this:

1. `research/architecture.md` §1 established that **`session/detection/binaries/claude.go`'s
   `ClaudeDetector.Patterns()` is not what drives production Claude status detection today** —
   `session/claude_controller.go`'s `cc.statusDetector` (a `*detection.StatusDetector` built from
   `getDefaultPatterns()`, a hand-duplicated pattern set) is. The `DetectorRegistry`/
   `DetectForProgram` path that would actually dispatch through `BinaryDetector` per-binary is
   confirmed dead in production (test-only callers).
2. `ClaudeController` (the only production caller this feature wires into — see `plan.md` Phase 4)
   is **already Claude-only by construction** — it is not a generic multi-binary controller that
   looks up a `BinaryDetector` by name at runtime.

## Decision

`ClassifyOSCTitle(title string) (dtypes.OSCStatus, bool)` is a plain free function in
`session/detection/binaries/claude.go`, called directly by `session/claude_controller.go`. It is not
a method on `ClaudeDetector`, and `dtypes.BinaryDetector` is not modified.

## Consequences

- Satisfies the Non-Goal ("extending to binaries other than Claude Code" is out of scope) literally:
  zero lines change in `gemini.go`, `aider.go`, `opencode.go`, or `agy.go`.
- No registry/dispatch change is needed — `registry.go`'s `DefaultRegistry()` and
  `binary_detector.go`'s `DetectorRegistry` are untouched, since the caller (`ClaudeController`) never
  goes through them for this feature (consistent with fact 1 above: it doesn't go through them for
  text-pattern detection today either).
- **Rejected alternative**: an optional `ParseOSCTitle(title string) (StatusHint, bool)`-shaped method
  added to `dtypes.BinaryDetector` with a no-op default. Rejected per
  `.claude/rules/interface-pollution-checklist.md` smell #1/#4 (speculative interface surface /
  forwarding-only additions) — the "seam for later" `features.md` raises can be added later, if and
  when a second binary's OSC convention is actually implemented, by promoting the free function to an
  interface method at that point (a mechanical, low-risk refactor once a second concrete need exists)
  rather than paying the speculative-surface cost now for a generalization this plan's own Non-Goals
  say isn't needed yet.
- If a future project *does* need OSC classification for another binary, the correct sequence per Go
  idiom (and this repo's own checklist) is: implement that binary's classifier first as its own free
  function, and only then — once two concrete implementations exist — consider whether a shared
  interface method is warranted, defined in the *consumer* package rather than `dtypes`.
