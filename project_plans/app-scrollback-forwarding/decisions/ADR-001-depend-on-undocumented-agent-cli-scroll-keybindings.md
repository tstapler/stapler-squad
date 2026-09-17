# ADR-001: Depend on undocumented, version-fragile agent-CLI scroll keybindings

**Status**: Accepted
**Date**: 2026-09-14
**Project**: app-scrollback-forwarding

## Context

To forward a scroll gesture into an agent CLI's self-managed transcript (Claude
Code first; pi and agy later), stapler-squad must send the exact byte sequence
each CLI's TUI event loop interprets as "scroll." Phase 2 research
(`research/stack.md`, `research/features.md`) found:

- Claude Code: `Ctrl+O` (`0x0F`) toggles a transcript viewer; `[` dumps the
  full transcript to native scrollback; `PageUp`/`PageDown` scroll the main
  fullscreen conversation view. Documented in Claude Code's own docs
  (`code.claude.com/docs/en/interactive-mode`, `.../fullscreen`), version
  `2.1.270` verified.
- pi: `tui.altScreen.pageUp`/`pageDown`/`lineUp`/`lineDown`/`top`/`bottom` —
  documented, remappable keybinding IDs
  (`earendil-works/pi/packages/coding-agent/docs/keybindings.md`).
- agy (Antigravity CLI): alt-screen vs. inline rendering modes are documented,
  but the specific scroll-keybinding IDs were **not** located during research
  — remains an open item, blocking Phase 3.

None of this is a stable, versioned protocol. It is each CLI's internal input
handling, which can change across releases without notice — unlike an escape
sequence a CLI *emits* (loud, visibly corrupts rendering when it changes, per
this repo's own BUG-013/BUG-025 history), a keybinding that stops matching
degrades **silently**: scrolling just stops doing anything, indistinguishable
from "no more history" (`research/pitfalls.md` §3).

## Decision

Depend on these documented-but-unversioned keybindings, gated by three
mitigations so a drift is cheap to detect and cheap to recover from without a
deploy:

1. **Per-adapter capability record, not inline literals.** Each
   `ScrollAdapter` implementation carries a `ScrollForwardCapability` value
   documenting the exact byte sequence, the CLI version it was verified
   against, and known failure modes (`session/scroll_forward_capability.go`).
   A future version bump has something explicit to diff against instead of
   an assumption buried in a `SendKeys` call.
2. **Live canary, not just a static flag.** After sending the scroll
   keystroke, verify the pane's captured content actually changed in a way
   consistent with "scrolled" within a bounded window (reusing this
   feature's own `RedrawQuiescence` capture), and log a structured warning
   when it doesn't — mirroring `compactingCanary`/`shellMonitorWordingCanary`
   (`session/detection/detector.go:361-413`), this repo's existing pattern
   for detecting when a textual assumption about a CLI's behavior has
   silently stopped holding.
3. **Per-adapter feature flag, live-settable.** Each adapter's forwarding
   path is gated by its own flag (`terminal:app-scrollback-forwarding:claude`,
   `:pi`, `:agy`), so a confirmed drift on one CLI can be disabled instantly
   without a deploy and without affecting the others — per this repo's
   existing live-settable flag convention (`config/config.go`'s
   `FeatureFlags` map, `server/services/feature_flag_service.go`'s
   `knownFeatureFlags`), not an env var.

## Fallback path (Claude Code specifically)

If the Phase 1 feasibility spike (Story 1.2.1) finds `PageUp`/`PageDown`
gesture-forwarding does not reliably scroll Claude Code's transcript (e.g.
because fullscreen rendering virtualizes content more aggressively than
documented, or because the byte sequence collides with another binding),
Claude Code's adapter falls back to **`NativeDumpFallbackStrategy`**: send
`Ctrl+O` then `[` (the documented transcript-to-native-scrollback dump), then
serve the result through the *existing*, already-shipped
`GetScrollbackHistory`/`tmux capture-pane` pagination path unchanged. Both
strategies implement the same `ScrollAdapter` interface (GoF Strategy) —
`KeySequences(dir ScrollDirection) [][]byte` (an ordered list of separate
writes, so this strategy's two-write `Ctrl+O` then `[` sequence and
`GestureForwardStrategy`'s single `PageUp` write are both representable) plus
`CaptureVia() ScrollCaptureMode` (`NativeScrollbackDelegateCapture` here vs.
`RedrawQuiescenceCapture` for gesture-forwarding) — so this substitution
requires no change to the detection or gating built in Epic 1.1, and no
change to the response framing (`AppScrollbackResponse`) built in Epic 1.3,
only which strategy `ClaudeScrollAdapter` is constructed with and which
branch `Instance.ForwardScroll` takes based on that strategy's own declared
`CaptureVia()`. (An earlier single-write `ScrollGesture([]byte, bool)` shape
could not represent this strategy's two-write, different-capture-path
behavior — architecture review flagged this as breaking substitutability
between the two strategies; the interface above is the corrected shape.)
This is the build-vs-buy research's identified cheaper fallback
(`research/build-vs-buy.md`, "Recommendation" section); this ADR records it
as the adapter's designed escape hatch, not an afterthought.

## Consequences

- A future Claude Code / pi / agy release can silently break this feature.
  The canary converts "silent" into "logged," and the per-adapter flag
  converts "requires a deploy to mitigate" into "flip a flag."
- Per-adapter capability records must be kept current; a new CLI version
  should prompt a re-verification pass before the record's confidence can be
  trusted for canary comparison.
- agy is explicitly out of Phase 1/2 scope until its own keybinding research
  closes the gap noted above (tracked as an Unresolved Question in
  `implementation/plan.md`).

## Alternatives considered

- **Pin to a specific CLI version and refuse to run forwarding otherwise.**
  Rejected: this product doesn't control which CLI version a user has
  installed, and refusing wholesale trades a soft degradation (no-op) for a
  hard one (feature never available) with no proportional safety gain over
  the canary+flag approach.
- **Read structured transcript files instead of forwarding keystrokes**
  (`ClaudeAdapter.Import`'s JSONL parsing, already in the codebase).
  Rejected as the primary mechanism per requirements.md's explicit ask for
  gesture forwarding, but this is exactly the class of read-only alternative
  the `NativeDumpFallbackStrategy` above uses as an escape hatch for Claude
  Code specifically once dumped through `[`.
