# Requirements: app-scrollback-forwarding

**Date**: 2026-09-14
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement
When the foreground program in a session manages its own internal scrollback/transcript
view instead of relying on the terminal's native scrollback, stapler-squad's scroll-up
pagination has nothing to return. Confirmed for Claude Code; suspected for the other
agent CLIs this product manages sessions for (Antigravity/agy, pi — see the adapter
files under `session/*_adapter.go`). `tmux capture-pane` history reflects only what was
written to the terminal's screen buffer, not an app's own internal transcript state, so
users scrolling up in the web/mobile terminal see no additional history — the existing
`ScrollbackRequest` pagination silently does nothing.

## Why Now
On desktop, a user stuck with this silent no-op has a workaround: scroll natively in
their own terminal emulator, or fall back to reading the transcript file directly. On
mobile there is no equivalent escape hatch — no keyboard, no native terminal scrollback
to fall back to — so gesture-forwarding isn't just an improvement over today's no-op,
it's the only path to reviewing an agent's past output on a touch device at all.

## Baseline
Today, scrolling to the top of the terminal viewport triggers a `ScrollbackRequest`
(`handleScrollbackRequest` in `server/services/connectrpc_websocket.go`, wired to
`Instance.GetScrollbackHistory` in `session/instance_tmux.go`) that calls
`tmux capture-pane` over a historical line range. For plain shells and programs that
print append-only output to the primary screen, this returns real history and
pagination works (this path — lazy tail-on-connect plus scroll-triggered paging,
`TerminalOutput.tsx`'s `requestScrollback`/`prependScrollbackBatch` — is already shipped
and out of scope here). For agent CLIs whose own UI manages scrollback (confirmed:
Claude Code), the request returns empty or insufficient content, so scrolling up does
nothing: the user is stuck seeing only what's currently rendered, with no way to review
anything the app itself has scrolled past.

## Users / Consumers
stapler-squad users viewing agent CLI sessions (Claude Code, and other adapters under
`session/*_adapter.go`: Antigravity/agy, pi) via the web app or mobile client.

## Success Metrics
Scrolling up inside a session running an agent CLI with app-managed scrollback
(starting with Claude Code) forwards the gesture to the app's own scroll mechanism and
the client renders the resulting view, instead of doing nothing. Measurable: 0%
silent-no-op rate on scroll-up for Claude Code sessions (down from ~100% today),
extended to other adapter-covered agents as their scroll mechanisms are confirmed
feasible in research.

This is a proxy metric, not a direct one: a session that's permanently `Blocked` (e.g.
multi-client contention) or hits `AtTop` almost immediately still counts as "not a
silent no-op" even though it didn't actually help the user recover lost content. Once
Story 1.2.1's spike produces real recoverable-history-depth data (see Feasibility
Risks / pre-mortem P2 finding #4), track that as a secondary metric alongside this one.

## Appetite
Medium (1–2 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.
Given the scope below covers every adapter-integrated agent CLI, sequence Claude Code
first as the proof of concept and extend to the others only as time and feasibility
research allow — do not stretch the appetite to force-fit an app whose scroll mechanism
turns out to need bespoke research.)*

## Constraints
None named beyond appetite.

## Non-functional Requirements
- **Performance SLO**: not specified beyond "no worse than current pagination latency"
- **Scalability**: not applicable — per-session, per-scroll-gesture feature
- **Security classification**: internal — no new trust boundary; keystrokes already
  flow client → server → PTY for regular input
- **Data residency**: no special requirements

## Scope
### In Scope
- Detect when the foreground/active program in a session is one of the agent CLIs
  already integrated via the adapter pattern (`session/claude_adapter.go`,
  `agy_adapter.go`, `pi_adapter.go`, etc.)
- Per-adapter research: does the app expose a keybinding/command to scroll its own
  transcript, and can it be sent over the PTY the same way regular input is?
- On client scroll-up, when the foreground app is known to self-manage scrollback,
  forward a scroll gesture as the app's own scroll input and capture the resulting
  redraw, streaming it back as the "page" of content in place of the current empty
  `ScrollbackRequest` response.
- Start with Claude Code; extend to other adapters as feasibility is confirmed.

### Out of Scope
- Generic passthrough for arbitrary non-agent TUI apps (vim, less, htop) — explicitly
  excluded this round.
- Rebuilding tmux-native scrollback delivery — already shipped and works correctly for
  plain shells; this project only covers the app-managed case.
- Any new keybinding/config that changes an agent CLI's own behavior — scope is
  stapler-squad's forwarding mechanism, not changes to the agents themselves.

## Rabbit Holes
- Reliably detecting "this pane is currently showing an app with self-managed
  scrollback" vs. "this pane has real tmux history" — may need per-adapter state
  tracking beyond simple alt-screen detection (`\x1b[?1049h/l`, already used elsewhere
  in this repo for resync), and could be wrong at session-start or mid-transition.
- Some agent CLIs may expose no scroll-back keybinding via terminal input at all —
  needs per-adapter research before committing to "forward gesture" for that adapter;
  if infeasible for one, that adapter should drop out of scope rather than blocking
  the whole project.
- Distinguishing a user-initiated forwarded scroll from live output arriving
  concurrently — the same class of race the existing lazy-scrollback implementation
  already documents (history/live boundary overlap, cursor-corruption risk on prepend).
- What "the resulting redraw" looks like varies by terminal size and app render
  behavior, and how many pages to buffer before giving up is undefined.

## Alternatives Considered
- Read structured transcript files directly (e.g. Claude Code's JSONL, already parsed
  by `session/claude_adapter.go` for review purposes) instead of forwarding keystrokes.
  Not chosen as the primary approach — the user explicitly wants gesture forwarding —
  but remains a candidate fallback for an adapter with no forwardable scroll command.
- Detect-and-degrade (hide pagination UI when app-managed scrollback is detected).
  Rejected: it stops the silent no-op but doesn't solve the actual need to review
  content the app has scrolled past.

## Feasibility Risks
- Unconfirmed whether any target CLI (Claude Code, Antigravity/agy, pi) actually
  exposes a terminal-forwardable scroll command — the central open question Phase 2
  research must resolve per adapter before planning can commit to an implementation.
- If an app's own scrollback is virtualized (only ever renders a visible window, never
  writes full history to the terminal even transiently), forwarding a scroll keystroke
  may only reveal what the app chooses to redraw — there may be a hard reach limit set
  by the app itself, not by stapler-squad.

## Observability Requirements
Log which scrollback path served a given scroll-up request (tmux-native history vs.
app-forwarded), and log when an adapter is encountered with no known forward mechanism
so coverage gaps are visible rather than silently degrading. Emit a counter for
forwarded-scroll attempts vs. successes, labeled per adapter.

## Risk Control
Gate the new forwarding behavior behind a live-settable feature flag (this repo's
flag panel/RPC — never an env var), scoped per-adapter if practical, so it can be
disabled without a deploy if a given app's forwarded scroll misbehaves (garbles the
live view, sends the app into an unexpected mode). Rollback is flipping the flag off;
the existing tmux-native pagination path is unchanged and remains the fallback for
every session this feature doesn't cover.

## Open Questions
- ~~Does Claude Code (and Antigravity/agy, pi) expose any terminal-input scroll/transcript
  command today?~~ **Answered by Phase 2 research** (`research/stack.md`,
  `research/features.md`): Claude Code exposes `Ctrl+O` to toggle its transcript viewer,
  and DECSET 1007 Alternate Scroll Mode (a standard terminal mechanism, not
  Claude-Code-specific) already translates mouse-wheel/PageUp/PageDown into PTY input
  while the alt-screen is active. pi exposes granular
  `tui.altScreen.pageUp/pageDown/lineUp/lineDown/top/bottom` keybindings. agy
  (Antigravity)'s specific keybindings were not located — remains open per-adapter,
  hand to Phase 3 planning as a scoping decision (research it then, or defer agy).
- ~~Can the current foreground process be identified as one of these adapters without
  new app-specific process sniffing?~~ **Answered by Phase 2 research**
  (`research/architecture.md`, `research/features.md`): No — `Instance.GetProgram()` is
  a launch-time label, not live foreground-process detection, and no alt-screen/DECSET
  1049 tracking exists in the codebase today. New detection logic is required; this
  repo has already shipped one drift bug from a duplicated capability check disagreeing
  with itself (`isClaudeAntigravityFamily` vs. `AgyAdapter.CanHandle`,
  `session/instance_program.go:12-20`), so the new detector must not become a second
  disagreeing source of truth — reuse/extend one canonical check.
- Should the client UI indicate "you're now viewing the app's own scrollback" vs.
  normal tmux history, and if so, how? *(unresolved after Phase 2 research — UX
  research flagged this needs a decision but found no existing pattern in this repo to
  anchor a default on. Deferred to Phase 3 planning per user direction 2026-09-14.)*
- **New from Phase 2 research, not in the original list**: forwarding a scroll gesture
  is a PTY *write*, not a read — the resulting redraw streams live to every connected
  client via the existing `streamViaControlMode`/`streamViaTmuxCapturePane` paths, so
  one client scrolling up would visibly hijack every other connected client's live view
  (`research/pitfalls.md`). This is a regression versus today's silent no-op and needs
  an explicit design decision (e.g. serialize/block forwarding when 2+ clients are
  connected, or notify other clients). *(Deferred to Phase 3 planning per user
  direction 2026-09-14 — Phase 3 must propose a concrete default, not leave this open.)*
