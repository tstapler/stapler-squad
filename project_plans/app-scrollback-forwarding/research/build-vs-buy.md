# Build vs. Buy: app-scrollback-forwarding

**Date**: 2026-09-14
**Researcher**: Agent 6 (Phase 2, SDD)
**Repo state cited**: `stapler-squad` @ `c412906c32fc7c51b6e10a0c8db9cb287b2035bb`

## TL;DR

No off-the-shelf library or SaaS solves "forward a scroll gesture into a
foreground TUI app's own scrollback and capture the redraw" — this is
confirmed niche, not assumed. The only viable path is **Option 4: extend
existing internal infrastructure** (the adapter pattern, the PTY write path,
and the lazy-scrollback pagination path already shipped for tmux-native
scrollback). Net-new work is: (a) an alt-screen/foreground-app detector, (b)
a per-adapter "scroll command" capability, (c) a client-side branch that
routes to app-forwarded scroll instead of `GetScrollbackBefore` when the
foreground app self-manages scrollback. One independent finding changes the
calculus for the Claude Code case specifically: Claude Code's classic
renderer (`CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN=1`) disables the alt-screen
scrollback entirely and falls back to the terminal's native scrollback,
which the existing pagination path (`session/scrollback/manager.go`,
`TerminalOutput.tsx`) already serves correctly with zero new code. That's
worth flagging to the planning phase as a candidate stop-gap even though the
requirements explicitly ask for gesture forwarding as the primary mechanism.

---

## Option 1: Existing OSS library or framework

**Verdict: Not recommended (confirmed absence) — but note adjacent building
blocks.**

Searched for "PTY scroll gesture forwarding," "terminal app scrollback
detection," and general prior art. No library exists for this specific
combination (detect app-managed scrollback + forward a scroll gesture as the
app's own input + capture the redraw as a paginatable unit). This is
corroborated by other projects hitting the identical problem and solving it
ad hoc, not via a shared library:

- `vogt` (terminal client) issue #592 — swipe-to-scroll is a no-op while a
  TUI holds the alternate screen; the project's own conclusion is "back-scroll
  there means translating the swipe into wheel or arrow input for the
  application" — i.e., exactly this feature, built bespoke, no shared library
  used. ([TheDancingDeveloper-org/vogt#592](https://github.com/TheDancingDeveloper-org/vogt/issues/592))
- `pocketshell` issue #2555 — "finger swipe sends arrow keys instead of
  scrolling, so scrolling back in an agent rewrites the prompt" — same
  problem class (gesture → keystroke forwarding ambiguity for agent CLIs),
  also solved inline, not via a library.
  ([PocketShell-io/pocketshell#2555](https://github.com/PocketShell-io/pocketshell/issues/2555))
- `hermes-agent` issue #108621 and `tuios` (a Go terminal multiplexer) both
  independently reimplement wheel-scroll-to-copy-mode translation rather than
  reusing a shared package.

**Adjacent building blocks that exist and are relevant if bespoke ANSI/alt-screen
work is needed** (see Option 3 for why the repo's own detector likely makes
these unnecessary):

| Library | What it offers | Relevance |
|---|---|---|
| [`hinshun/vt10x`](https://pkg.go.dev/github.com/hinshun/vt10x) (and forks) | Full VT10x terminal emulation in Go, including `ModeAltScreen` state tracking | Could detect DECSET 1049 (alt-screen enter/exit) if the repo ever needs *generic* alt-screen detection instead of per-adapter program-name matching |
| [`creack/pty`](https://github.com/creack/pty) | PTY allocation/control | Already effectively in use — this repo shells to `tmux`/`tymux` rather than driving PTYs directly, so not a new dependency |
| [`owenthereal/tmux`](https://pkg.go.dev/github.com/owenthereal/tmux), [`GianlucaP106/gotmux`](https://github.com/GianlucaP106/gotmux) | Typed Go wrappers around tmux CLI/control-mode | Not needed — this repo has its own tmux control-mode client (`session/tymux/`, `session/external_tmux_streamer.go`) already built and tested |

None of these close the actual gap (per-app scroll semantics + redraw
capture); they'd only replace small pieces of plumbing the repo already has
working.

---

## Option 2: SaaS / managed API

**Verdict: Not applicable — no managed API for this exists; noting prior art only.**

This is inherently a local PTY-forwarding mechanism; there's no SaaS surface
to buy. The closest prior art is Warp's terminal, which has the identical
unsolved problem in its own agent mode:

- Warp issue #9959, "block navigation (CMD-UP/arrow keys) should work in
  agent mode," is an **open, unresolved** feature request — Warp's block
  navigation (their analogue to scroll-through-history) explicitly does not
  work inside agent-mode output today. The documented workaround is
  `Shift+Cmd+Up/Down` to scroll *within* one agent response block, not
  genuine pagination through an agent's self-managed transcript.
  ([warpdotdev/warp#9959](https://github.com/warpdotdev/warp/issues/9959))
- Warp exposes MCP integration (so external agents can plug into Warp) but
  no documented public API/SDK for "forward a scroll gesture to a hosted
  block/transcript and get the redraw back" — nothing to buy or integrate
  against.

Conclusion: this is an industry-wide unsolved UX problem for agent-CLI host
products, not a solved-elsewhere problem stapler-squad is reinventing.

---

## Option 3: LLM-generated bespoke ANSI/PTY code vs. reusing `session/detection/`

**Verdict: Recommended — reuse and extend `session/detection/`, do not write
parallel bespoke detection.**

### What `session/detection/` already does

- `session/detection/detector.go` and `pattern_set.go` implement a
  **regex-over-captured-plain-text** status detector, not a raw ANSI/VT
  emulator. `PatternSet.MatchLines` (`session/detection/pattern_set.go:124`)
  scans already-stripped pane text (see `pkg/ansi/csi.go`'s `StripCSI`,
  `pkg/ansi/osc.go`'s `ExtractLastOSC`) against per-program regex sets to
  derive `DetectedStatus` (idle/thinking/waiting/etc.).
- Each supported CLI has its own pattern file under
  `session/detection/binaries/` (`claude.go`, `agy.go`, `pi.go`, `aider.go`,
  `gemini.go`, `opencode.go`) — this is exactly the per-adapter extension
  point the feature needs for "does this app expose a scroll command," just
  currently scoped to status detection rather than capability description.
- There is **no existing alt-screen (DECSET 1049) tracking** anywhere in the
  Go codebase — confirmed by grep across `session/` and `pkg/` for
  `AltScreen`/`1049`/`smcup`/`rmcup`; the only hits are an analytics
  escape-code description table (`pkg/analytics/escape_code_descriptions.go`)
  and an unrelated streamhub snapshot file, neither of which tracks live
  mode state.

### Bespoke vs. reuse assessment

- **Detecting which program is in the foreground** — reuse, not bespoke.
  `HistoryAdapter.CanHandle(program)` (`session/history_adapter.go:9`) and
  `resolveHistoryAdapter` (`session/history_adapter.go:20`) already solve
  "is this Claude/Antigravity/pi" from the program string. The scroll
  feature's "detect when the foreground/active program is one of the agent
  CLIs" requirement (Scope, requirements.md) is this exact mechanism reused,
  not reimplemented.
- **Detecting whether the app is *currently* in an app-managed-scrollback
  mode (e.g., alt-screen)** — this is genuinely net-new. The repo's
  pattern-matching detector works on text content, and text content alone
  can't reliably tell you "the terminal is in DECSET 1049 mode" — that's a
  mode bit, not a rendered string. Two implementation choices:
  1. Extend `pkg/ansi` with a small DECSET-1049 tracker (bespoke, ~20-40
     lines, mirrors the existing `StripCSI`/`ExtractLastOSC` style already in
     that package) — **recommended**, keeps the dependency surface at zero
     and matches the codebase's existing preference for small hand-rolled
     ANSI utilities over a full VT emulator.
  2. Pull in `hinshun/vt10x` for its `ModeAltScreen` tracking — only
     justified if the feature grows into needing a fuller terminal model
     (e.g., diffing full screen redraws cell-by-cell); premature for the
     "detect mode + forward one command" scope in requirements.md.
- **Parsing what changed in a redraw** (requirements.md's open question
  under Feasibility Risks) — likely doesn't need bespoke diffing at all.
  The existing tmux `capture-pane` pipeline (`session/external_tmux_streamer.go:465`,
  `session/backend_tymux.go:126`) already captures full-pane snapshots on
  every redraw; reuse that as the "page" content directly (the pane *is* the
  app's rendered scroll state) rather than building a separate diff/redraw
  detector.

**Bespoke work is confined to**: the alt-screen mode bit, and a small
per-adapter capability table ("does this CLI have a forwardable scroll
command, and what is it"). Everything else — program detection, PTY write
path, pane capture — is reuse.

---

## Option 4: Fork or adapt existing code in this repo

**Verdict: Recommended — this is the primary path; almost nothing here is
plausibly bought.**

### What extends cleanly

| Existing piece | File:line | How this feature extends it |
|---|---|---|
| Adapter pattern / `HistoryAdapter` interface | `session/history_adapter.go:8-16` | Add a capability, e.g. `ScrollCommand() (keys string, ok bool)`, to each adapter (`ClaudeAdapter`, `AgyAdapter`, future `PiAdapter`) rather than defining a parallel "scrollable adapter" interface. `resolveHistoryAdapter` (`session/history_adapter.go:20`) is already the single resolution point for "which adapter handles this program" and should stay that way. |
| PTY input write path | `SendKeys` / `SendInputViaControlMode`, `session/instance_tmux.go:1234` and `:1244` | This is the forwarding mechanism itself — sending the app's scroll keybinding is a `SendKeys`/`SendInputViaControlMode` call with the adapter-supplied key sequence, no new PTY-write plumbing needed. |
| Pane capture | `CapturePaneContentWithOptions`, `session/backend_tymux.go:130`; `capturePane`, `session/external_tmux_streamer.go:465` | Reuse directly to capture the redraw after forwarding the scroll keys — this is the "page" of content requirements.md asks for. |
| Lazy scrollback pagination (client) | `TerminalOutput.tsx:1034-1063` (near-top-of-viewport `scroll` listener on `.xterm-viewport`, calling `requestScrollback(oldestSequenceReceivedRef.current, 500)`) | This is the trigger point to branch from. The DOM-scroll-near-top detection stays; what changes is what happens next when the foreground app is known to self-manage scrollback — instead of (or in addition to) `requestScrollback`, dispatch an app-scroll-gesture RPC. |
| Scrollback RPC/proto | `ScrollbackRequest` (`proto/session/v1/events.proto:218`), `scrollbackResponse` handling (`web-app/src/lib/hooks/useTerminalStream.ts:459-486`) | Either extend this message with an "app-forwarded" variant, or add a parallel RPC — a planning-phase call, but the wire-level pattern (request → streamed chunks → `hasMore`/sequence metadata) is directly reusable. |
| `ScrollbackManager` (server) | `session/scrollback/manager.go:83` (`GetScrollback`), `:310` (`GetScrollbackBefore`) | **Not directly reusable for the app-managed case** — it's a circular buffer of literal PTY bytes written to the terminal (`AppendOutput`, `session/scrollback/manager.go:63`). An app's internal transcript was never written to this buffer (that's the whole problem statement), so this manager can't serve app-forwarded pages; the new code path bypasses it and instead calls the pane-capture-after-forwarded-scroll flow above. |
| Mobile touch-scroll gesture recognizer | `useTerminalGestures.ts` (5-state gesture machine: IDLE→PENDING→SCROLLING/SELECTING/TAPPING) | Already distinguishes a scroll gesture from selection/tap on mobile; the *output* of a recognized SCROLLING gesture is what needs a new branch (forward as app keys vs. scroll xterm's own buffer), not the gesture recognition itself. |

### What's net-new (no internal code to fork)

1. Per-adapter scroll-command capability + the actual keybinding/command
   per CLI (research question, not yet answered here — that's this
   project's core feasibility risk per requirements.md, and belongs in a
   dedicated per-adapter research doc, not this build-vs-buy doc).
2. Foreground-app / alt-screen-mode detection to decide *when* to route
   through the new path vs. the existing `GetScrollbackBefore` path (Option
   3 above).
3. The client-side branch in `TerminalOutput.tsx` and the
   proto/RPC surface for "forwarded app scroll" responses.

### Why this is decisively "adapt, don't build parallel"

The adapter pattern, PTY write path, and pane-capture pipeline are each
independently mature (all have existing test suites — e.g.
`session/claude_adapter_test.go`, `session/detection/detector_test.go`) and
directly reusable. Building a standalone mechanism instead — e.g., a new
PTY-control subsystem outside the adapter pattern — would duplicate
`SendKeys`/`CapturePaneContentWithOptions` and fragment "which CLI does
what" logic away from the single resolution point (`resolveHistoryAdapter`)
the rest of the codebase already depends on for history import/export. The
only genuinely new *architectural* piece is the capability descriptor (does
this adapter support scroll-forwarding, and how) — everything else is
wiring existing, tested primitives together.

---

## Recommendation

**Build, using Option 4 (fork/adapt internal infrastructure) as the
mechanism and Option 3's reuse of `session/detection/` for the one net-new
piece of ANSI-adjacent detection (alt-screen mode).** Do not evaluate OSS
libraries or SaaS further — Option 1 and 2 are confirmed non-solutions for
this niche, corroborated by three independent open-source projects and one
commercial product (Warp) hitting the same wall and solving it bespoke or
leaving it unsolved.

One input worth surfacing to Phase 3 planning: Claude Code's
`CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN=1` / `/tui fullscreen` +
`Ctrl+O` + `[` (dump transcript to native scrollback) mechanisms are
documented, real escape hatches that route Claude Code's transcript back
through the terminal's *native* scrollback — which the existing
`GetScrollbackBefore`/`TerminalOutput.tsx` pipeline already serves with zero
new code. This doesn't replace the requirement's explicit ask (gesture
forwarding, not a config toggle or an extra keypress before scrolling), but
it's a materially cheaper fallback for the Claude Code adapter specifically
if the true forwarding mechanism proves infeasible during per-adapter
research, and is worth a one-line mention in the plan's risk section.

Sources for Claude Code's alt-screen/transcript behavior (unverified beyond
the search snippets — flag as INFERRED, confirm against a live pane capture
during per-adapter research before relying on it):
[anthropics/claude-code#38283](https://github.com/anthropics/claude-code/issues/38283) (configurable scrollback buffer size for TUI alt screen),
[Fullscreen rendering docs](https://code.claude.com/docs/en/fullscreen).
