# Requirements: OSC Title/Progress Sequences as High-Priority Status Signals

Source: backlog item `2b6b4fcc-0df6-478d-a213-f089ad8f5e31`, migrated from
[TylerStaplerAtFanatics/stapler-squad#177](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/177).

## Problem

Claude Code emits two OSC (Operating System Command) escape sequences that
directly encode its status:

- **OSC window title** (`\x1b]0;...\x07`) — contains a Braille spinner
  character (⠋⠙⠹...) while working, and `✳` when idle/done.
- **OSC progress** (`\x1b]4;...`) — `\x1b]4;0` signals completion/idle.

Today, `session/detection/detector.go`'s `ansiStripRegex` (line ~120) matches
and *discards* OSC sequences (`\x1b\][^\x07]*\x07`) purely so they don't
corrupt text-pattern matching — the title content itself is thrown away, not
captured or used as a signal. All status detection in
`session/detection/binaries/claude.go` is regex matching against the
stripped screen text. This means: when Claude Code's visible pane content
looks idle (e.g. a blank prompt during a slow tool call) but the OSC title
still shows a spinner, the detector can report `Idle`/`Ready` when the
process is actually still `Executing`/`Processing` — a false-idle state.

Reference implementation for the pattern: [herdr](https://github.com/ogulcancelik/herdr)
treats OSC title/progress as a higher-priority, debounce-bypassing signal
ahead of text-pattern detection (`src/detect/manifests/claude.toml`,
`src/pane/agent_detection.rs`).

## Goal

Capture OSC title (and progress, if present) content from the PTY stream
alongside screen text, feed it into the Claude binary detector as a
first-class signal, and give it priority over text-pattern-derived status —
skipping any stabilization/debounce delay that applies to text-based
transitions.

## Acceptance Criteria

1. OSC window-title content (`\x1b]0;...\x07` payload) is captured from PTY
   output during the same pass that currently strips it, without changing
   existing stripped-text behavior for downstream pattern matching.
2. The captured OSC title is threaded into the detection pipeline as a
   distinct input (not just re-embedded in the stripped text) reaching
   `session/detection/binaries/claude.go`'s detector.
3. A Braille spinner character (U+2800–U+28FF block, e.g. ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) in
   the OSC title maps to `StatusExecuting` (or `StatusProcessing`, matching
   existing status semantics — resolved during planning).
4. A `✳` character in the OSC title maps to an idle/ready status
   (`StatusReady` or `StatusIdle`, resolved during planning against existing
   status semantics).
5. When OSC-derived status and text-pattern-derived status disagree, the
   OSC-derived status wins for at least the false-idle case described above
   (OSC shows spinner, text pattern would otherwise report idle/ready).
6. OSC-derived status transitions are not subject to the same debounce/
   stabilization delay as text-pattern transitions (mirrors herdr's design),
   or an explicit, documented reason is given if full debounce-bypass is
   infeasible in this codebase's architecture.
7. Existing text-pattern-based detection continues to work unchanged for
   binaries/scenarios where no OSC title is present (graceful fallback).
8. No regression in existing `session/detection` test suite
   (`go test ./session/detection/...`).
9. New behavior has test coverage: unit tests for OSC title parsing
   (spinner detection, `✳` detection) and for the priority/override
   behavior against conflicting text-pattern signals.

## Non-Goals

- OSC progress sequences (`\x1b]4;...`) beyond the `\x1b]4;0` completion
  signal are out of scope unless research finds Claude Code emits richer
  progress data worth using.
- Extending OSC-signal detection to binaries other than Claude Code.
- Changing the debounce/stabilization architecture for text-pattern
  detection itself (only adding an OSC-priority bypass path).

## Context / Prior Art in This Repo

- `session/detection/detector.go:120` — OSC sequences currently
  stripped via `ansiStripRegex`, title content not retained.
- `session/detection/binaries/claude.go` — current text-pattern-only
  Claude detector (`Patterns()` returns `dtypes.StatusPatterns`).
- `session/detection/terminal_detector.go`, `session/detection/registry.go`
  — pipeline wiring between PTY output and binary detectors (to be mapped
  in detail during research).
- Related existing plans with no OSC overlap (checked, no conflict):
  `project_plans/detection-architecture-refactors/`,
  `project_plans/remote-detection-manifests/`.
