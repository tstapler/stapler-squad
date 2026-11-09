# Requirements: mobile-input-layer

**Date**: 2026-11-06
**Type**: feature addition (regression fix + new subsystem)
**Complexity**: 3 — system design (multiple epics, client + server, rollout flag, observability)
**Priority (ICE, INFERRED, triad product review 2026-11-08)**: Impact 9, Confidence 6, Ease 4 (product 216; regression fix for the owner's primary mobile surface, ships first as PR-A1).

## Problem Statement

On Android Chrome, text typed with the soft keyboard (Gboard) never reaches the tmux pane in the web terminal; autocomplete picks and autocorrect rewrites are lost. Toolbar buttons (arrows, Esc, Tab) still work, and backend writes (nudges, `write_to_session`) still work. The owner spends much of their time driving sessions from a phone, so this makes the product unusable there.

Evidence so far (verified 2026-11-06):
- tmux pane is healthy: `pane_input_off=0`, `pane_dead=0`, `client_readonly=0`; a backend write of `ssqtest` appeared at the prompt.
- Server marks every subscriber `can_write:true`; no dropped-input logs.
- `ghostty_renderer: true` is set in `~/.stapler-squad/workspaces/d685c4b1a423cca3/config.json:122` (flag added in `ac9758981`, 2026-11-01, default off). It swaps xterm.js for ghostty-web 0.4.0.
- ghostty-web 0.4.0 ignores keydowns with `keyCode === 229` (`dist/ghostty-web.js:851`) and calls `preventDefault()` on every `beforeinput` (`dist/ghostty-web.js:2330`), so Android IME text never produces data.
- The xterm.js fallback is also weak on Gboard (xterm.js issue #3600: orphaned `input` events, duplicated text around Enter/Backspace).
- UNVERIFIED: that turning the flag off restores phone typing. Phase 2 must confirm this on a real device or an emulated one.

## Baseline

Today: with `ghostty_renderer` on, phone soft-keyboard typing is dead; only toolbar keys work. With it off, xterm.js handles plain letters but is unreliable for autocomplete, rewrites, swipe typing and Enter/Backspace composition wrapping. There is no server-side signal showing whether a phone's keystrokes arrive, and the client silently drops input when the stream is disconnected (`sendInput`, `web-app/src/lib/hooks/useTerminalFlowControl.ts:241`, console.warn only).

## Users / Consumers

- The owner, typing into sessions from an Android phone (Chrome, Gboard; also Samsung Keyboard) over the remote listener (`:8444`) and over LAN/Tailscale.
- Desktop users (hardware keyboards) must see no change.
- Both terminal renderers (`XtermTerminal.tsx`, `GhosttyTerminal.tsx`), which consume the input layer.
- Operators reading logs and metrics.

## Success Metrics

- A replayed Gboard event stream (typing, autocomplete pick, autocorrect rewrite, swipe word, Backspace, Enter, emoji) produces exactly the bytes a hardware keyboard would for the same final text, under both renderers (Jest replay suite, 100% of recorded scenarios pass).
- A Playwright mobile-emulated e2e types text into a live session pane and asserts it appears in the pane, under both renderers.
- Server exposes per-session input frames and bytes, split into escape-sequence vs plain-text, so "phone typed but nothing arrived" is diagnosable from a metric/log in under a minute. Baseline: not observable at all. Proven by a timed drill (plan Task 5.1.2d): three scripted faults (layer off, read-only denial, WebSocket blocked), each named correctly from the how-to alone in under 60 s.
- Client counts input dropped because the stream was disconnected (first PR), and surfaces a visible indicator instead of a console.warn (PR-B, behind `mobile_input_queue`).
- Desktop typing latency and behavior unchanged (existing terminal tests stay green).
- Manual acceptance on the owner's phone: typing, autocomplete, autocorrect and swipe typing all work in `stelekit` through the deployed build. Numeric pass threshold (plan Story 5.1.4): a scripted list of at least 22 phrases (plain, autocorrect picks, suggestion picks, swipe words, Backspace runs, Enter and symbols) passes 22 of 22 exact per renderer with zero eaten prompt characters and zero duplicates, and a 200-character script arrives with 0% keystroke loss. Baselines are measured, not qualitative (plan Task 0.1.3c): exact-match count out of 22 (expected 0/22 with ghostty on), keystroke loss, and desktop keydown-to-echo p50/p95; the added p95 on desktop and the phone's `beforeinput`-to-`onData` delta must each be <= one frame (16.7 ms).
- Accessibility and small screens (plan Stories 1.4.3, 1.4.4, 1.2.5): TalkBack announces committed words once; an autocorrect rewrite shows at most one intermediate state; with toolbar, compose bar, chip and keyboard present the terminal keeps at least 5 rows (3 in landscape or on short phones).

## Size

Large, INFERRED: the plan's verified cost tracks at about 16.9M CU after the triad-review repair (above the nominal 15M edge of the 3–15M Large band; bands are uncalibrated and the figure is a tracking number, not a scope gate): new client subsystem, two renderer integrations, server counters, a recorded-event test corpus, e2e, a rollout gate. Full scope is the default.

**Wall-clock blockers**: owner phone baseline and keyboard-resize spikes (owner, plan Tasks 0.1.3b/c, 0.1.5a), owner phone pre-merge check on the isolated manual instance including the TalkBack and re-measure rows (owner), owner phone acceptance test after redeploy (owner), the timed diagnosis drill (owner plus a second device); CI runtime; recording real Gboard/Samsung event streams may need the owner's device (owner). Everything else is code.

## Constraints

- Renderer-agnostic: a renderer swap must not be able to break phone typing again.
- Web-app is Bun-only; `unset npm_config_user_agent` before `make build`.
- Follow repo gates: `make ready` (dupl, jscpd 0.14% ratchet, lint, zero-warning web lint), e2e conventions, `@/lib/nav` facade, vanilla-extract CSS, `data-testid`/ARIA locators.
- Rollout flags must be live-settable through the feature-flag panel, not env vars (owner feedback `feedback_rollout_flags_live_settable_no_env_vars`).
- No force push; PR opened ready-for-review (not draft); merge with `gh pr merge --repo tstapler/stapler-squad`; redeploy with `make install-service` (tmux sessions survive via `--tmux-keep-server`).
- Not touching the live instance for manual testing before merge: use the isolated manual-instance recipe in CLAUDE.md.

## Non-functional Requirements

- **Performance SLO**: no added keystroke-to-send latency beyond one animation frame on mobile; zero added work on the desktop hardware-keyboard path; batching with existing `terminal-input-batching` behavior preserved. Covered by plan Task 1.2.2d (synchronous reducer-to-`onData` latency-budget test, no timers or rAF per keystroke when not composing), Task 1.2.4a (existing batching cases pass unchanged with the widened `SendResult` return), Task 3.1.1b (atomic edits vs chunking) and Story 1.2.2's zero-listener desktop assertion.
- **Scalability**: many terminals per page; the layer attaches to visible/docked terminals only (so the tap target exists before the first tap); only one holds focus, unfocused docked layers keep a hidden element and a tap binding but are `inputmode="none"`, and hidden or undocked terminals dispose theirs (plan Story 1.4.1, ADR-006).
- **Security classification**: internal. Input bytes are never logged; counters carry sizes and classes only.
- **Data residency**: no special requirements.

## Scope

### In Scope

- A renderer-agnostic soft-keyboard input layer: hidden input element driven by `beforeinput` (`insertText`, `insertReplacementText`, `insertCompositionText`, `deleteContentBackward`, `insertLineBreak`) with composition tracking, diff-based rewrites (backspaces + new text), `keyCode 229` handling, autocorrect/autocapitalize/spellcheck attribute policy, and orphaned-`input` fallback.
- Integration into `XtermTerminal.tsx` and `GhosttyTerminal.tsx` behind one shared seam.
- Server observability: per-session input frame/byte counters (escape vs plain text) in `dispatchInputReadLoopFrame` (`server/services/connectrpc_websocket.go:3129`) and the other input handlers (`connectrpc_websocket_shell.go:265`, `:3849`, `session_service_stream_terminal.go:439`), exposed via metrics and a new actuator endpoint beside `/actuator/ws-unary`.
- Client observability: counter + visible indicator for input dropped while disconnected; optional client beacon of IME event shapes (class only) for diagnosing new keyboards.
- Immediate client-side escape hatch: a `?mobile_input=0|1` query param that applies in the same page load and persists per origin, plus a visible one-line hint when the flag is off on the Android class (plain-language copy with no query parameter or flag identifier, so rollback never silently lands the user on a weak renderer; the parameter is documented only in the how-to).
- A minimal read-only diagnostics panel (`?inputdebug=1`: content-free ring buffer, build id, renderer and flag header, Copy report) delivered in the first PR so a failing keyboard can be recorded the same day.
- Scope notes (plan Delivery Slicing, owner-approved ADR-sourced items beyond the original bullets): the delivery ledger and invalidate-on-boundary logic (plan Story 1.2.4) ship with the first PR; the bounded offline queue and the visible drop chip (PR-B, sub-flag `mobile_input_queue`), the full diagnostics sheet and capture-for-fixture (PR-C, `mobile_input_diagnostics`), and the opt-in compose bar and IME-fallback notice (PR-D, `mobile_input_compose`) are later PRs behind sub-flags, default off until the owner opts in. The "visible indicator for dropped input" Success Metric is therefore met when PR-B merges and `mobile_input_queue` is enabled; the first PR ships only the counter.
- Test corpus: recorded/synthesized Gboard + Samsung event sequences, Jest replay harness, Playwright mobile e2e.
- A gate so `ghostty_renderer` cannot be enabled for the Android touch-primary class unless the mobile-input layer is active (or it falls back to xterm.js there). iOS Safari and touch laptops keep today's renderer choice and attach no layer; iPhone under `ghostty_renderer` keeps the same dead-soft-keyboard risk and is tracked as an iOS parity follow-up (plan Story 6.1.2).
- Research on state of the art (xterm.js #3600, Codeman PR 65, ghostty-web upstream, Termux, Blink Shell, hterm, VS Code web terminal, Monaco/CodeMirror 6 EditContext and beforeinput approaches) and a design that goes beyond it.

### Out of Scope

- iOS Safari parity beyond not regressing it (noted as follow-up unless trivially covered).
- Replacing the renderer or contributing upstream fixes to ghostty-web (may be filed as follow-ups).
- Redesigning the on-screen extra-keys toolbar.
- Voice dictation specifics beyond what `beforeinput` already delivers.

## Rabbit Holes

- Gboard does not follow a stable event contract; composition events may be absent, wrapped around Enter/Backspace, or duplicated. The layer must be tested against recorded sequences, not assumed behavior.
- Diff-based rewrite can emit wrong backspace counts when the terminal app (Claude Code's input box) has its own cursor/wrapping model; rewrites must be bounded to the current composition region.
- Hidden textarea vs contenteditable vs the emerging EditContext API: browser support and soft-keyboard behavior differ.
- Interaction with the existing mouse-report filter (`isMouseReport`), sticky Ctrl/Alt/Shift toolbar modifiers, paste chunking (`PASTE_CHUNK_SIZE`), and the gesture/scroll touch handlers that call `preventDefault` on touch events (soft keyboard may fail to open if focus is blocked).
- Capturing real device event streams requires the owner's phone.

## Alternatives Considered

- Turn `ghostty_renderer` off and stay on xterm.js: immediate mitigation only; xterm.js still mishandles Gboard (issue #3600).
- Patch xterm.js with the Codeman orphaned-input workaround: relies on xterm internals (`triggerDataEvent`), fragile, xterm-only.
- Patch/fork ghostty-web: couples us to an upstream we do not control.
- Dedicated renderer-agnostic input layer (chosen direction): one place to own IME correctness and test it.
- EditContext API based input: promising but limited support; evaluate in research as a progressive enhancement.

## Feasibility Risks

- Real Gboard/Samsung event sequences are needed for a trustworthy corpus; synthesized ones may miss quirks.
- Android keyboard behavior varies by version and settings (suggestion strip on/off, swipe typing).
- Tmux/TUI cursor model mismatch for rewrites inside multi-line prompt boxes.

## Observability Requirements

- Server: counters `terminal_input_frames_total{kind=text|escape|other}` and `terminal_input_bytes_total` per session class (labels bounded; no session-title labels), a log line at Info on first input per stream and a throttled summary, and an entry in the existing actuator view. Alert condition: a remote-listener stream with sustained client-reported typing but zero received text frames, delivered as a documented diagnostic query, a dashboard panel and a non-paging alert rule (plan Tasks 2.2.3a and 2.2.3b; if the repo has no alert-provisioning convention, Task 2.2.3b stops and records an owner decision that the panel plus query satisfy this requirement).
- Client: `input_dropped_disconnected_total`, `ime_event_sequence_unrecognized_total`, composition-state anomalies; surfaced through the existing client telemetry beacon (`docs/reference/client-network-telemetry.md` pattern).
- Never log input content.

## Risk Control

- New live-settable flag `mobile_input_layer` (global + per-scope override via the feature-flag panel). It merges default ON for the Android touch-primary class ONLY IF the pre-merge phone check on the isolated manual instance passes (plan Task 5.1.4c); otherwise it merges default OFF with a per-scope override for the owner. Desktop and iOS never attach whatever the flag says. Kill switch restores the current behavior.
- `ghostty_renderer` gated: on the Android touch-primary class it falls back to xterm.js unless the input layer is active.
- Rollback: turn the flag off in the panel (no rebuild; two page loads to apply from the panel) or use `?mobile_input=0` for an immediate same-load escape on one origin; revert the PR if needed.

## Open Questions

Status refreshed 2026-11-08 to match `implementation/plan.md` Unresolved Questions (each open item is owned by a spike or task):

- Does turning `ghostty_renderer` off alone restore plain-letter typing on the phone? RESOLVED AS A BLOCKER: the layer owns input under both renderers, so the fix does not depend on the answer. The result is still recorded for how-to wording (plan Story 0.1.3, Task 0.1.3c baseline).
- What exact event sequences do Gboard (suggestion strip on/off, swipe typing) and Samsung Keyboard emit on the owner's phone? OPEN: synthetic corpus ships first (plan Stories 1.1.4); device fixtures arrive via capture-for-fixture in PR-C (Task 5.1.4a). Blocks only Story 5.1.4's device-fixture row.
- Is `beforeinput` on a hidden textarea sufficient everywhere, or is EditContext worth adopting as the primary path on Chrome Android? DECIDED for v1 (ADR-006: textarea primary, EditContext only behind sub-flag `mobile_input_edit_context`, plan Story 6.1.1); the device comparison stays OPEN and blocks only Story 6.1.
- How do other web terminals (VS Code, Termux web, Blink, hterm, JupyterLab, ttyd/wetty) handle IME rewrites, and which of those designs can we beat? ANSWERED by `research/features.md`, `research/build-vs-buy.md` and ADR-002 (live atomic rewrite, bounded to the composition region).
- Where is the right metric home on the server (Prometheus registry vs actuator JSON)? DECIDED in ADR-005: both, with class-only bounded labels (plan Stories 2.1.1, 2.1.3).
- Does the soft keyboard resize the layout viewport, only the visual viewport, or overlay the page on Android Chrome, Samsung split/floating/one-handed modes and floating Gboard, and does the cursor row stay visible? OPEN: the existing keyboard-avoidance layout was designed for iOS Safari (plan Story 0.1.5, Tasks 0.1.5a/b); blocks Story 1.4.3.
- Does TalkBack announce typed characters correctly with the value-is-composition-only design? OPEN until the device check (plan Task 5.1.4d); a failure reopens Story 1.4.4.
- Does the repo have an alert-provisioning convention for the OBS-4 non-paging alert? OPEN: if not, the owner decision that the dashboard panel plus documented query satisfy the requirement is awaiting the owner (plan Task 2.2.3b fallback; validation.md Gap 2).
- Is a Ctrl-W (`\x17`) honored by Claude Code's input box, and does Backspace on the textarea after a commit-time reset produce any event? OPEN, owned by plan Tasks 0.1.3a, 0.1.2b and 0.1.3b; they block Task 1.1.2a's keymap and Backspace design.
