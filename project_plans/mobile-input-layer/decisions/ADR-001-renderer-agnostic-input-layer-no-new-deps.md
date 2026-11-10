# ADR-001: Build a thin renderer-agnostic input layer with no new runtime dependencies

## Status
Accepted (2026-11-06)

## Context
Android soft-keyboard text never reaches the pane under ghostty-web 0.4.0 (keyCode 229 ignored, every `beforeinput` prevented), and xterm.js is weak on Gboard (xterm.js #3600, open). Upstream fixes are not on our timeline: ghostty-web PR #110 is merged on main but unreleased, and xterm.js has no linked fix (`research/build-vs-buy.md` 1a, 1b). Every renderer-internal fix (Codeman PR #65 uses private `_core.coreService.triggerDataEvent`) is single-renderer and timing-heuristic based.

## Decision
Build `web-app/src/lib/terminal/mobileInput/`: a pure DOM-free reducer `reduce(state, event) -> {state, commands}`, a bounded rewrite diff, and a DOM adapter `attachMobileInput(container, {onData, getModifiers, isReadOnly, onAnomaly})` where `onData: (bytes, seq: UnitSeq) => SendResult | void` from the start (`seq` lets the delivery ledger of plan Story 1.2.4 attribute delivery per unit), attaching only when `shouldAttachMobileInput()` (Android touch-primary class) is true. Primary input source is a hidden `<textarea>`. No new runtime or dev dependency: `fast-check`, `@testing-library/user-event`, jsdom and Playwright already exist. No `diff`/`fast-diff`: the needed operation is a single contiguous common-prefix/suffix edit measured in graphemes (`Intl.Segmenter`).

## Alternatives rejected
- Patch/`bun patch` xterm.js or ghostty-web: renderer-specific, re-applied on every upgrade, coupled to internals.
- Adopt Codeman's orphaned-input patch: private API, 50 ms timing window, no rewrite handling. Kept as a design reference for the orphaned-`input` fallback only.
- Wrap a hidden CodeMirror `EditorView`: heavy, needs doc-change to byte mapping. Kept as a design reference (one-rAF key delay, snapshot diff).
- Wait for ghostty-web release: no control over timing, and it would not fix xterm.

## Consequences
We own IME correctness, so the replay corpus (ADR-007) is mandatory. A renderer swap can no longer break phone typing. Follow-ups (not in scope): file ghostty-web and xterm.js upstream issues.
