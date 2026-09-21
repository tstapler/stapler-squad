# Architecture Research: app-scrollback-forwarding

**Agent**: 3 (Architecture Research), SDD Phase 2
**Date**: 2026-09-14

## 1. The "adapter pattern" named in the requirements is the wrong adapter

The requirements doc assumes `session/claude_adapter.go`, `session/agy_adapter.go`,
`session/pi_adapter.go` are the per-CLI integration points for runtime PTY behavior. They
are not. All three implement `HistoryAdapter`
([session/history_adapter.go:7](session/history_adapter.go#L7)):

```go
type HistoryAdapter interface {
    Name() string
    CanHandle(program string) bool
    Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error)
    Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error
}
```

`ClaudeAdapter`/`AgyAdapter` ([session/claude_adapter.go:17-27](session/claude_adapter.go#L17-L27),
[session/agy_adapter.go:19-29](session/agy_adapter.go#L19-L29)) convert each CLI's on-disk
transcript format (JSONL files, etc.) to/from a `CanonicalTurn` representation, used by
`PortSessionHistory` when a session's `Program` is switched (e.g. claude → agy) so chat
history survives the switch. `resolveHistoryAdapter(program)`
([session/history_adapter.go:21](session/history_adapter.go#L21)) dispatches by program
string. None of this touches the PTY, tmux, or live terminal I/O — it reads/writes files
on disk after the fact. `session/pi_adapter.go` isn't even a `HistoryAdapter`; it's a
`PiEventReader` that parses pi's own JSONL event stream file
([session/pi_adapter.go:167-245](session/pi_adapter.go#L167-L245)).

**Implication for this project**: there is no existing "per-CLI runtime behavior" seam to
plug into. A new one has to be designed — see §4 for a proposed `ScrollAdapter` (or
similar) shaped for *this* problem, kept deliberately separate from `HistoryAdapter` since
the two have nothing in common except "dispatch by program string."

## 2. How the "current program" is known today — and it is not runtime-detected

`Instance.Program` ([session/instance.go:807](session/instance.go#L807)) is a plain string
field, set at session creation (`opts.Program` at
[session/instance.go:968](session/instance.go#L968)) and changed only through the explicit,
user/config-driven `Instance.SwitchProgram`
([session/instance_program.go:59](session/instance_program.go#L59)). Dispatch is by
substring match: `isClaude(program)` / `isPi(program)`
([session/instance_tmux.go:148-183](session/instance_tmux.go#L148-L183)),
`isClaudeAntigravityFamily` / `isClaudeAntigravityCrossSwitch`
([session/instance_program.go:18-27](session/instance_program.go#L18-L27)).

This is a **configured launch command**, not an observation of what's actually running in
the pane right now. Nothing in the codebase samples `pane_current_command` (tmux's own
live foreground-process name) to detect drift. The nearest thing —
`GetPanePID`/`tmux_process_manager.go`'s `panePID` cache
([session/instance_tmux.go:1251](session/instance_tmux.go#L1251),
[session/tmux_process_manager.go:54](session/tmux_process_manager.go#L54),
[session/tmux/tmux.go:3625](session/tmux/tmux.go#L3625)) — resolves the pane's foreground
PID for process-liveness checks (`IsBackendProcessAlive`, `PaneProcessDead`), not identity.

**Answering requirement 1's open question directly**: the code has no way today to tell
"session launched as `claude`, but the user is currently at a bare shell prompt inside that
pane" from "the pane is currently showing Claude Code's TUI." `i.Program` will say `claude`
in both cases. A forwarding feature that trusts `i.Program` alone will forward Page-Up
keystrokes to a plain shell (harmless — Page Up usually does nothing or triggers shell
history search, degrading gracefully) but will also fail to detect the reverse case (a
generic-program session where the user manually launches `claude` inside it). This is a
real gap the plan phase needs to size, not a research artifact to gloss over — see §6.

## 3. Where the request enters the server, and how forwarding would hook in

### Client-side gesture (today)

`TerminalOutput.tsx`'s scroll listener
([web-app/src/components/sessions/TerminalOutput.tsx:1034-1063](web-app/src/components/sessions/TerminalOutput.tsx#L1034-L1063))
is a DOM `scroll` handler on xterm.js's `.xterm-viewport` element, firing
`requestScrollback(oldestSequence, 500)` when `terminal.buffer.active.viewportY < 200`.

**Critical wrinkle not called out in the requirements**: `viewportY` reflects xterm.js's own
scrollback buffer, which is native-terminal scrollback — the same thing `tmux capture-pane`
serves. A TUI that runs in the **alternate screen** (`ESC[?1049h`, DECSET 1049 — which
Claude Code's ink-based UI almost certainly uses, like vim/htop/less) has **no xterm
scrollback to speak of**: alt-screen mode suppresses it at the terminal-emulation layer, so
`viewportY` will sit at 0 and this scroll listener may never even fire for the case this
project targets. This means the client can't reuse its existing "near-top-of-buffer" signal
as the trigger for app-managed-scrollback forwarding — it needs a *different* trigger (e.g.
scroll-up while alt-screen is active regardless of `viewportY`, or a dedicated UI affordance)
rather than a server-side branch under the same gesture.

### Server-side dispatch (today)

`ScrollbackRequest` arrives as one arm of `TerminalData`'s oneof
([proto/session/v1/events.proto:95](proto/session/v1/events.proto#L95)), parsed in
`dispatchInputReadLoopFrame` ([server/services/connectrpc_websocket.go:3074-3102](server/services/connectrpc_websocket.go#L3074-L3102))
and handed to `handleScrollbackRequest`
([server/services/connectrpc_websocket.go:3116-3148](server/services/connectrpc_websocket.go#L3116-L3148)),
which calls the injected `onScrollbackRequest(startLine, endLine string) (string, error)`
callback. Both call sites wire that callback directly to
`instance.GetScrollbackHistory(startLine, endLine)`
([server/services/connectrpc_websocket.go:1343-1345](server/services/connectrpc_websocket.go#L1343-L1345)
for `streamViaControlMode`, [connectrpc_websocket.go:2053-2054](server/services/connectrpc_websocket.go#L2053-L2054)
for `streamViaHub`), which is a thin wrapper over
`i.pm().CapturePaneContentWithOptions(startLine, endLine)`
([session/instance_tmux.go:1067-1069](session/instance_tmux.go#L1067-L1069)) — a
`tmux capture-pane -S/-E` call over a historical line range.

**What would need to change**: `onScrollbackRequest`'s signature
(`func(startLine, endLine string) (string, error)`) is tmux-capture-range-shaped — it has no
way to express "press PageUp N times and give me the redraw" as an operation. It needs to
become adapter-aware, e.g.:

```go
onScrollbackRequest func(ctx context.Context, req ScrollbackForwardRequest) (ScrollbackResult, error)
```

so the callback can branch internally on whether the session's current mode is
"native tmux history" (today's path, unchanged) vs. "forward a scroll gesture" (new path),
without `handleScrollbackRequest` itself needing to know which. Both `streamViaControlMode`
and `streamViaHub` (and the shell-tab analog,
`handleShellScrollbackRequest` at [connectrpc_websocket.go:2415](server/services/connectrpc_websocket.go#L2415))
independently wire this callback today — a new implementation has exactly two call sites to
update, not a scattered set.

## 4. Input path a forwarded keystroke would use

Regular keyboard input already takes exactly the path a forwarded scroll keystroke would
need. In `streamViaControlMode`'s `onInput` callback
([server/services/connectrpc_websocket.go:1315-1339](server/services/connectrpc_websocket.go#L1315-L1339)):

1. Try `instance.SendInputViaControlMode(ctx, data)` → `TmuxSession.SendInputViaControlMode`
   ([session/tmux/control_mode.go:1230-1246](session/tmux/control_mode.go#L1230-L1246)), which
   builds `send-keys -t <session> -H <hex bytes...>` and enqueues it on the control-mode
   command channel, waiting for tmux's `%begin`/`%end` ack.
2. On error, fall back to subprocess `sendInputToTmux(...)` (a direct `tmux send-keys`
   subprocess invocation).

Both paths ultimately do a tmux `send-keys -H` with the raw byte sequence for whatever key
is being sent — this already supports sending arbitrary control sequences (Page Up is
`\x1b[5~` in most terminfo entries; Claude Code, being an ink/React-based TUI, likely reads
raw stdin rather than checking `$TERM` capabilities, so the exact byte sequence to send is
itself part of the per-CLI research called for in requirement 2, not knowable from this
architecture pass). **No new input-plumbing is needed** — a forwarded scroll keystroke is
just another `SendInputViaControlMode` call with a specific byte sequence, called from the
new scrollback-forwarding branch instead of from the regular `onInput` path (since it must
be triggered by a `ScrollbackRequest`, not a `TerminalInput` frame, to avoid the client
needing to fake a keypress).

## 5. Capturing "the redraw" and framing it back

After forwarding a scroll keystroke, the server needs to capture what the app just drew.
The existing building blocks are:

- `CapturePaneContentPriority`/`CapturePaneContentRawPriority`
  ([session/instance_tmux.go:942-980](session/instance_tmux.go#L942-L980)) — capture-pane
  through the control-mode-priority path, already used for resync snapshots
  (`handleCurrentPaneRequest`, [connectrpc_websocket.go:481](server/services/connectrpc_websocket.go#L481)).
- The existing **quiescence** machinery
  (`resizeSettling`/`quiescenceCh`/`ResizeQuiescence` message,
  [connectrpc_websocket.go:1064-1094](server/services/connectrpc_websocket.go#L1064-L1094),
  proto message at [events.proto:141-149](proto/session/v1/events.proto#L141-L149)) — built
  for "send input, wait for the terminal to stop changing, then snapshot," which is exactly
  the same shape as "send a scroll keystroke, wait for the TUI's redraw to settle, then
  capture." Reusing this pattern (rather than a fixed sleep-then-capture) avoids
  reintroducing a race the resize path already solved once.

**Framing back to the client** — do not reuse `ScrollbackResponse`/`ScrollbackChunk`
verbatim. `ScrollbackResponse` ([events.proto:224-230](proto/session/v1/events.proto#L224-L230))
carries `total_lines`/`oldest_sequence`/`newest_sequence` — pagination bookkeeping that
assumes a stable, indexable line range, which a forwarded app-redraw fundamentally isn't
(the app decides what it shows; there's no sequence number to hand back for "page 2"). The
closer existing shape is `CurrentPaneResponse`/`TerminalOutput` — a raw content blob that
*replaces* the visible pane, exactly what `handleCurrentPaneRequest` already sends for
resyncs, with `ansiSnapshotPrefix` prepended so xterm.js clears and repaints
([connectrpc_websocket.go:148-155](server/services/connectrpc_websocket.go#L148-L155),
mirrored client-side at
[web-app/src/lib/terminal/TerminalStreamManager.ts:37-50](web-app/src/lib/terminal/TerminalStreamManager.ts#L37-L50)).
A new message type (e.g. `AppScrollbackResponse`) that piggybacks on the same
"full-pane-replacement snapshot" framing — content + a flag saying "this is an app-scroll
result, not live output" — is the better fit than shoehorning it into `ScrollbackResponse`'s
sequence-number contract.

## 6. Interaction risk with the alt-screen resync/snapshot mechanism

This is the sharpest integration risk in the whole feature. `TerminalStreamManager.ts`
detects a full-snapshot replacement purely by **byte-prefix sniffing**:
`output.startsWith(ANSI_SNAPSHOT_PREFIX)`
([TerminalStreamManager.ts:268](web-app/src/lib/terminal/TerminalStreamManager.ts#L268)),
where `ANSI_SNAPSHOT_PREFIX = "\x1b[!p\x1b[2J\x1b[H"` — DECSTR + erase-screen + cursor-home.
The server sends this exact prefix for **every** resync/snapshot path today (initial
handshake, post-resize snapshot, `CurrentPaneRequest` replies — at least 8 call sites across
`connectrpc_websocket.go`, all listed via the grep for `ansiSnapshotPrefix`).

If a forwarded-scroll "page" is delivered using this same prefix (because it's sent as a
`TerminalOutput`/full-pane-replacement to fit the existing client machinery), the client has
**no way to distinguish** "this is the app's own scrolled-back transcript" from "this is a
routine resize/resync snapshot of the live pane." Consequences:

- If the client's resync logic treats it as a normal snapshot, a concurrently-arriving real
  resync (e.g. triggered by a resize the user does mid-scroll) could interleave with or
  clobber the forwarded-scroll content — the same "history/live boundary overlap,
  cursor-corruption risk on prepend" class of race the requirements doc already flags for
  the native scrollback path, but worse here because *both* sides use identical framing.
- Conversely, if scroll-forwarded content reuses `ANSI_SNAPSHOT_PREFIX` and the client's
  resync-correlation logic (`resync_id`, [events.proto:155-160](proto/session/v1/events.proto#L155-L160))
  isn't extended to cover it, a stale forwarded-scroll reply arriving after the user has
  already scrolled further (or dismissed the app's scrollback view) could silently overwrite
  newer content with no way to tell it's stale.

**Recommendation**: a forwarded-scroll response must carry its own explicit discriminator —
either a distinct message type (not reusing `TerminalOutput`/`ScrollbackResponse` at all) or,
at minimum, a distinct prefix constant plus its own `resync_id`-equivalent correlation field
— so `TerminalStreamManager.ts`'s prefix-sniffing dispatch can route it to a different
handler than `onFullSnapshot`. Silently overloading `ANSI_SNAPSHOT_PREFIX` is the single
highest-risk implementation shortcut this project could take.

## 7. Disposition

**Extend as-is, via a new seam** — not a hotspot and not a SOLID violation requiring
refactor-first. `connectrpc_websocket.go` is large (3965 lines) but the relevant handlers
(`handleScrollbackRequest`, `onScrollbackRequest` callback wiring) are already isolated
behind a narrow function-typed field precisely so behavior can be swapped per call site —
that's the seam this feature should widen (change the callback's signature once, branch
inside the two implementations), not a place to restructure. The one *new* structural
element genuinely needed is a small `ScrollAdapter`-style interface (§1) for
per-CLI scroll-forwarding behavior — deliberately not `HistoryAdapter`, which solves an
unrelated problem (transcript file conversion) and would be an interface-pollution violation
if scroll-forwarding methods were bolted onto it.

## 8. Event-Command-Policy table (EventStorming)

| Domain Event | Policy Trigger | Command | Actor / System |
|---|---|---|---|
| User scrolls up near top of terminal viewport | Client detects `viewportY` near top **or** (new) alt-screen-active + scroll-up gesture | `RequestScrollback` (existing) / `RequestAppScrollback` (new) sent over WebSocket | Client (`TerminalOutput.tsx`) |
| `ScrollbackRequest`/new request frame received | Server checks whether session's foreground state is "app-managed scrollback" | Branch: native tmux capture (unchanged) vs. forward-scroll (new) | Server (`handleScrollbackRequest` / new dispatch) |
| Session identified as app-managed-scrollback-capable | `i.Program` matches a `ScrollAdapter`-registered program **and** (ideally) the pane's actual foreground process still matches (§2 gap) | `ResolveScrollAdapter(program)` | Server (new, mirrors `resolveHistoryAdapter`) |
| Scroll-forwarding approved | Adapter reports a known scroll-up keystroke exists for this program | `SendInputViaControlMode(ctx, scrollKeystrokeBytes)` | Server → PTY (existing tmux control-mode path) |
| Keystroke delivered to pane | App redraws its own transcript view | (app-internal; no stapler-squad command) | Foreground app (Claude Code, etc.) inside PTY |
| App finishes redrawing | Quiescence detected (reuse resize-quiescence pattern, §5) | `CapturePaneContentPriority` | Server |
| Captured redraw ready | — | Send new `AppScrollbackResponse` (own discriminator, not `ANSI_SNAPSHOT_PREFIX` reuse — see §6) | Server → Client |
| `AppScrollbackResponse` received | Response's session/stream still matches current pane state (correlation ID check) | Render as replacement content in a way visually distinct from live pane (open question in requirements — "should the client indicate app-scrollback mode") | Client |
| Foreground program changes mid-session (e.g. user exits Claude Code to a shell) | No existing signal fires this today (§2) | **Gap**: needs either periodic `pane_current_command` polling or heuristic (last-keystroke-sent correlation) before scroll-forwarding can safely re-arm/disarm | Server (not yet implemented anywhere) |

## Summary of concrete integration points

| Need | File:Line | Current shape | Change needed |
|---|---|---|---|
| Per-CLI scroll capability lookup | new file, mirror [session/history_adapter.go](session/history_adapter.go) | n/a | New `ScrollAdapter` interface + `claude`/`agy`/`pi` implementations |
| Server dispatch callback | [server/services/connectrpc_websocket.go:3010](server/services/connectrpc_websocket.go#L3010), wired at [:1343](server/services/connectrpc_websocket.go#L1343) and [:2053](server/services/connectrpc_websocket.go#L2053) | `func(startLine, endLine string) (string, error)` | Adapter-aware signature, two call sites |
| Keystroke delivery | [session/tmux/control_mode.go:1230](session/tmux/control_mode.go#L1230) `SendInputViaControlMode` | Already generic hex-encoded send-keys | No change — reuse directly |
| Redraw capture | [session/instance_tmux.go:942](session/instance_tmux.go#L942) `CapturePaneContentPriority` | Already generic | No change — reuse, paired with quiescence wait |
| Response framing | [proto/session/v1/events.proto:224](proto/session/v1/events.proto#L224) `ScrollbackResponse` | Sequence-number pagination model | New message type, own discriminator (not `ANSI_SNAPSHOT_PREFIX`) |
| Client dispatch | [web-app/src/lib/terminal/TerminalStreamManager.ts:268](web-app/src/lib/terminal/TerminalStreamManager.ts#L268) | Prefix-sniffing on `ANSI_SNAPSHOT_PREFIX` | New prefix/discriminator + new handler, not `onFullSnapshot` reuse |
| Trigger gesture | [web-app/src/components/sessions/TerminalOutput.tsx:1034](web-app/src/components/sessions/TerminalOutput.tsx#L1034-L1063) | `viewportY < 200` on native xterm scrollback | New trigger condition for alt-screen sessions (viewportY likely always 0) |
| Feature flag | [config/config.go:1676](config/config.go#L1676) `FeaturePiSupport` as precedent | n/a | New live-settable flag, per repo convention (see `config/config.go`'s flag registry pattern) |
