# Requirements: Context-Compaction Detection

Source: backlog item `9b063255-c8ee-48a6-8121-e4984e016522`, migrated from
[TylerStaplerAtFanatics/stapler-squad#179](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/179).

## Problem

When a Claude Code session auto-compacts its context, the session enters a
distinct state that today's PTY-output detection cannot distinguish from
ordinary processing. The session card stays pinned in `PROCESSING`/`EXECUTING`
for the full compaction duration (reportedly 30-60s) with no dedicated
indicator, so a user watching the board can't tell "still thinking" from
"compacting context right now."

## Existing System (confirmed by reading the code)

- `session/detection/detector.go` defines the `DetectedStatus` enum
  (`StatusUnknown`, `StatusReady`, `StatusProcessing`, `StatusNeedsApproval`,
  `StatusInputRequired`, `StatusError`, `StatusTestsFailing`, `StatusIdle`,
  `StatusExecuting`, `StatusSuccess`, `StatusWaitingForAgent`) and the
  `StatusDetector` that runs regex `StatusPattern`s (from
  `session/detection/dtypes/dtypes.go`) grouped by status over PTY output.
- Per-binary pattern sets live in `session/detection/binaries/*.go`
  (`ClaudeDetector` for Claude Code). Existing Claude patterns already match
  `esc to interrupt` and a `synthesizing`/thinking-verb spinner as `Active`.
- **Claude Code already emits an "until auto-compact" percentage indicator**
  in its status line — confirmed in existing test fixtures
  (`session/detection/testdata/claude_active.txt:5`,
  `claude_thinking_verb.txt:5`, `claude_asterism_active.txt:5`):
  `esc to interrupt   10% until auto-compact`. This is the *approaching*
  state, not proof of the exact string Claude Code prints once compaction is
  actually in progress — that string needs to be confirmed from live/recent
  Claude Code output before a pattern is written against it (the issue's
  `Compacting context...` / `[context window compressed]` phrasing is a
  guess, not verified).
- `session/detection/proto_mapping.go` (`DetectedStatusToProto`) is the single
  authoritative mapping from the internal `DetectedStatus` iota to the
  `sessionv1.DetectedStatus` proto enum — any new Go status needs a matching
  proto enum value and a case here.
- `web-app/src/lib/utils/deriveWorkingState.ts` maps backend detected status
  to frontend working-state / UI badge; this is the frontend consumer that
  needs a new mapping branch.
- Session card component(s) under `web-app/src/components/sessions/` render
  the badge/spinner per working state and would need a new visual variant.

## Goal

Add a distinct `ContextCompacting` detection state so the UI can show a
specific "compacting context" indicator instead of leaving the session card
in an undifferentiated processing/executing state during auto-compaction.

## Acceptance Criteria

1. The exact terminal string(s) Claude Code emits when auto-compaction is
   actively in progress are identified and captured as fixture(s) under
   `session/detection/testdata/`, distinct from the existing "N% until
   auto-compact" approaching-threshold indicator.
2. A new `StatusCompacting` (or equivalently named) value is added to the
   `DetectedStatus` Go enum in `session/detection/detector.go`, with a
   corresponding pattern group and regex pattern(s) registered for the Claude
   binary detector in `session/detection/binaries/claude.go`.
3. `session/detection/proto_mapping.go` maps the new Go status to a new
   `sessionv1.DetectedStatus` proto enum value (added to
   `proto/session/v1/*.proto`, regenerated via `make proto-gen`).
4. Given PTY output containing the confirmed compaction-in-progress string,
   `StatusDetector.Detect`/`DetectWithContext` returns the new compacting
   status, verified by a Go test using the new fixture(s) (following the
   existing `detector_test.go` / `shared_detector_test.go` pattern).
5. `web-app/src/lib/utils/deriveWorkingState.ts` maps the new proto status to
   a distinct frontend working state, with a Jest test covering the mapping.
6. The session card UI shows a visually distinct badge/spinner (e.g. `⟳
   Compacting context`) when a session is in the compacting state, without
   changing behavior for any other existing state.
7. The new state is additive only: no existing `DetectedStatus` pattern,
   priority ordering, or test fixture's expected classification changes as a
   side effect (`go test ./session/detection/...` and the frontend
   `deriveWorkingState` test suite both pass unchanged for pre-existing
   cases).
8. Feature registry entries are added/updated per
   `.claude/rules/feature-registry.md` if this introduces a new
   frontend-observable feature surface (session card compaction badge), and
   `make registry-generate` is run.

## Success Metric / Why Now

**Added 2026-08-06 (Product triad review gap)**: this item has no quantitative usage data — it's sourced from a single migrated GitHub issue (#179), and the target user ("a user watching the board during compaction") is not sized by frequency. Named explicitly rather than left implicit:

- **Metric**: post-ship, `DetectionEvent` ring buffer entries for `StatusCompacting` are non-zero within the first week of real usage (proves the regex actually fires — see pre-mortem P1 #1's canary). No user-facing survey/NPS metric is proposed; this is a small clarity fix, not a feature with a funnel.
- **Why now**: low cost to defer, but also low cost to ship — it's purely additive (no behavior change, no new user action, small blast radius per the plan's Risk Control section) and was already fully planned in this session. The counter-argument (PM lens: narrow reach — only visible during compaction windows — against a ~20-task implementation spanning a proto wire change and two hand-maintained adapter switches) is real and not fully resolved here; if priority is contested at review time, the cheaper alternative is reusing an existing `SubStatus` value (e.g. `PROCESSING`) with only a frontend label override keyed off a raw-text heuristic, deferring the wire-format change. This plan does not adopt that cheaper alternative — it was surfaced by triad review, not chosen against.

## Non-Goals

- Blocking or otherwise altering session lifecycle/control behavior during
  compaction — this is detection/UI only, matching the issue's own framing
  ("session is not blocked, not idle — it's self-managing").
- Detecting compaction for non-Claude binaries (Aider, OpenCode, etc.) —
  out of scope unless research finds it's trivially the same mechanism.
- Building a general "context usage" progress meter beyond the existing "N%
  until auto-compact" pattern — only the compacting-in-progress state is new.

## Open Question Carried Into Research

The single largest unknown is ACT 1 above: the literal string(s) Claude Code
prints while compaction is actively running has not been confirmed against
real Claude Code output or upstream source in this triage pass. Research
phase must verify this before planning is finalized, since the entire
detection pattern depends on it.

*(status after Phase 2 research — see `research/stack.md` and
`research/build-vs-buy.md`)*: **partially resolved, still unverified against
a live capture.** `strings` on the locally installed Claude Code binary
(2.1.223) surfaced the literal `"Compacting conversation"` clustered with
`isCompacting`/`compactingHintText`/`applyCompactProgress` symbols, and
[anthropics/claude-code#30115](https://github.com/anthropics/claude-code/issues/30115)
independently mocks up near-identical text (`⠋ Compacting conversation...
42%`). This is strong circumstantial convergence but not a byte-accurate PTY
fixture. **Accepted as unresolved uncertainty** (no user present to
interview in this triage session) — plan.md's task breakdown makes "trigger
`/compact` live and capture the raw PTY fixture" the first implementation
task, before any regex is written against the inferred string.

Research also surfaced a stronger long-term alternative not adopted in this
pass: Claude Code has documented `PreCompact`/`PostCompact` hooks (see
`research/build-vs-buy.md`) that would give a deterministic signal instead of
PTY-regex-scraping. Out of scope here (would exceed this item's committed
regex-based scope) but flagged as a follow-up suggestion.
