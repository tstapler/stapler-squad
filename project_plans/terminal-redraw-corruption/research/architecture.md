# Terminal Output Pipeline Architecture: tmux pane → browser pixel

Research for `terminal-redraw-corruption`. Scope: trace every buffering/chunking/dedup/drop
point between a tmux pane emitting bytes and xterm.js rendering them, to find where a longer
status line's erase-in-line (EL) sequence could be lost before a shorter replacement line
overwrites it on-screen.

Out of scope per requirements: `server/mcp/ansi.go`'s `stripANSI` (unrelated MCP tool-output
path — not touched, not read). The MOSH state-sync system in `project_plans/new-renderer/` is
confirmed STALE — deleted in commit `0ac0ca1dad9f102d7072857c87714fb2d1905e05` (PR #163). The
live path is raw-byte passthrough with no client/server screen-state reconciliation.

No fix is proposed here — this is architecture tracing only.

## Pipeline diagram (control-mode path — the primary managed-session path)

```
┌────────────────────┐
│   tmux pane (PTY)   │  Claude Code / Ink TUI writes raw bytes incl. ANSI escapes
└──────────┬──────────┘
           │ tmux's own internal event loop batches PTY reads into %output notifications
           ▼
┌───────────────────────────────────────────────────────────────────────┐
│ tmux -C control-mode subprocess (one per tmux session, refcounted)     │
│ Emits newline-delimited protocol lines: %output %<pane> <octal-escaped>│
└──────────┬──────────────────────────────────────────────────────────--┘
           │ stdout pipe
           ▼
┌───────────────────────────────────────────────────────────────────────┐
│ session/tmux/control_mode.go                                          │
│                                                                        │
│  readControlModeOutput()  [:243]                                      │
│    bufio.NewScanner(t.controlModeStdout) — DEFAULT buffer, NOT pooled │
│    (commit 5a4fb9d6b's scanbuf pooling only touches                   │
│     session/artifacts/scan.go, session/history.go,                   │
│     session/tokens/parser.go — confirmed NOT this file)               │
│    for each scanned line:                                             │
│      hasOutputPrefix(b) [:632] → handleOutputBytes(b) [:638]  (hot path,│
│        zero-alloc via scanner.Bytes())                                │
│      else → processControlModeLine(scanner.Text())  (%begin/%end/etc) │
│                                                                        │
│  handleOutputBytes(b) [:638-655]                                      │
│    parses "%output %PANE_ID DATA"                                     │
│    → decodeControlModeOutput(data) [:660-678]  (undoes tmux's \ooo    │
│      octal escaping of bytes <32 and backslash)                       │
│    → broadcastControlModeUpdate(decoded) [:683-697]                   │
│                                                                        │
│  ***broadcastControlModeUpdate(data []byte)*** [:683-697]             │
│    for each subscriber:                                               │
│      select { case ch <- data: default: log.Warn(...); DROP }        │
│    ← non-blocking send, SILENT DROP on full channel                   │
│    ← per-subscriber; a slow/backlogged browser tab drops its OWN copy │
│      independent of other subscribers                                 │
│                                                                        │
│  SubscribeToControlModeUpdates() [:702-723]                           │
│    ch := make(chan []byte, 100)  [:707] — bounded, no backpressure    │
│    signal to producer if consumer falls behind                        │
└──────────┬──────────────────────────────────────────────────────────--┘
           │ per-subscriber buffered channel (100 slots of already-decoded []byte frames)
           ▼
┌───────────────────────────────────────────────────────────────────────┐
│ server/services/connectrpc_websocket.go                               │
│                                                                        │
│  streamTerminal() [:464-540] — top-level per-connection dispatcher     │
│    shell tabs        → streamShellViaControlMode [:495] (if CM env on)│
│                       → streamViaTmuxCapturePane  [:498] (fallback)    │
│    main session, CM  → streamViaControlMode       [:525]              │
│      (needs instance.Snapshot().IsManaged AND CM env var enabled)     │
│    main session, else→ streamViaTmuxCapturePane   [:539]              │
│    ⇒ ONE path is selected per connection at stream start — the two    │
│      paths are NOT run concurrently against the same xterm.js         │
│      instance within a single connection (see "Two capture paths"     │
│      section below for the reconnect-race caveat)                     │
│                                                                        │
│  streamViaControlMode() [:554-1140]                                   │
│    Goroutine 1 — output-forwarding + coalescing [~786-892]:           │
│      updateChan := subscribe to broadcastControlModeUpdate's channel   │
│      on each frame:                                                    │
│        buf := append(pooled_buf, data...)                              │
│        coalesce: drain up to maxBatchFrames=32 MORE already-queued     │
│                  frames non-blockingly, appending IN ARRIVAL ORDER     │
│        escapeParser.ParseStage2(buf, ...)  (Stage-2 escape tracking)  │
│        sendData(buf) → single WebSocket TerminalData_Output message   │
│      ⇒ order-preserving; does not itself reorder or split an EL       │
│        sequence from text that both survive to reach updateChan.      │
│        Its role is to forward WHATEVER SURVIVED candidate #1's drop,  │
│        with no way to detect a gap occurred.                          │
│                                                                        │
│    Goroutine 2 — WebSocket read loop [~991-1140]:                     │
│      handles TerminalData_Input, TerminalData_Resize,                 │
│      TerminalData_ScrollbackRequest ONLY.                             │
│      *** NO case for TerminalData_FlowControl anywhere in this        │
│      function or file (verified: whole-file grep for                 │
│      "FlowControl|Paused|pause" → zero matches in                    │
│      connectrpc_websocket.go). Client-side pause/resume watermark     │
│      signals (if sent) are never read here — DEAD LETTER. ***         │
│                                                                        │
│    Resize-nudge / quiescence [~587-681]:                              │
│      quiescenceCh created [~?] but explicitly NOT wired to a real     │
│      producer for the initial post-resize wait (self-documented gap   │
│      in inline comments) — degenerates to a fixed 500ms/200ms timer.  │
│                                                                        │
│  streamShellViaControlMode() [:1148-...]                              │
│    Same SubscribeToControlModeUpdates() / same bounded-100-channel /  │
│    same broadcastControlModeUpdate producer as streamViaControlMode — │
│    candidate #1 applies identically to shell tabs (verified: uses     │
│    identical subscribe/coalesce/send pattern, lines ~1237-1267).      │
└──────────┬──────────────────────────────────────────────────────────--┘
           │ WebSocket binary frame: ConnectRPC envelope wrapping TerminalData_Output
           ▼
┌───────────────────────────────────────────────────────────────────────┐
│ web-app/src/lib/hooks/useTerminalStream.ts                             │
│   textDecoderRef = new TextDecoder()  [:113] — persistent, stream:true│
│   decode(decodedData, {stream:true})  [:257]                          │
│   Decoder reset on close/reconnect [:327-330, :411-412] to avoid      │
│   stale partial-UTF8-byte state leaking across reconnects.            │
│   (Message-type routing switch not fully re-verified this pass; no    │
│   evidence found of any buffering/reordering here beyond the decoder.)│
└──────────┬──────────────────────────────────────────────────────────--┘
           │ decoded string chunk, in the order WebSocket frames arrived
           ▼
┌───────────────────────────────────────────────────────────────────────┐
│ web-app/src/lib/terminal/TerminalStreamManager.ts                     │
│                                                                        │
│  write(output) [:228-248]                                             │
│    → redrawThrottler.process(output)  [class at :42-92]               │
│    → escapeParser.processChunk(result)                                │
│    → handleProcessedOutput(safeOutput) [:328-358]                     │
│                                                                        │
│  ***RedrawThrottler.process(chunk)*** [:42-92]                        │
│    isFullRedraw = /^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/.test(chunk[0..32])│
│    if NOT full-redraw-shaped → flushPending(); return chunk (in order)│
│    if full-redraw-shaped → OVERWRITES this.pendingRedraw (not merged, │
│      not queued) and arms a 33ms setTimeout; if a second matching     │
│      chunk arrives before the timer fires, the FIRST is discarded     │
│      entirely — onFlush() is never called for it.                     │
│    ⇒ regex only matches cursor-up + \x1b[2K (erase WHOLE line) or     │
│      \x1b[J (erase to end of screen) — NOT plain \x1b[K (erase to     │
│      end of line, no "2"). This repo's own escape-code test fixtures  │
│      (web-app/src/lib/test-generators/escape-codes/library.ts:42,    │
│      generators.ts:317,374,409) use plain '\x1b[K', matching the      │
│      conventional Ink/status-spinner redraw shape (cursor-up + CR +   │
│      erase-to-EOL + new text). INFERRED, not directly captured from a │
│      live Claude Code session: if Claude Code's actual spinner redraw │
│      uses \x1b[K (not \x1b[2K), it does NOT match isFullRedraw and    │
│      bypasses this throttle/discard path entirely, going straight     │
│      through flushPending()+return chunk in original order.           │
│                                                                        │
│  handleProcessedOutput() [:328-358]                                    │
│    routes to writeDirectWithFlowControl (≤16KB CHUNK_SIZE) or          │
│    enqueueWrite (chunked via requestAnimationFrame, >16KB) — both      │
│    preserve order; watermark constants (HIGH=100000/LOW=10000) exist   │
│    but pair with a server-side FlowControl message type that the      │
│    server-side control-mode path never reads (see above) — the        │
│    client's pause signal has no effect on this path.                  │
└──────────┬──────────────────────────────────────────────────────────--┘
           │ terminal.write(chunk) — xterm.js internal parser applies escapes to screen buffer
           ▼
┌────────────────────┐
│   xterm.js canvas   │  final rendered pixels
└────────────────────┘
```

## Two capture paths in production (requirement #4)

`streamTerminal()` (`server/services/connectrpc_websocket.go:464-540`) selects exactly one of
three handlers **once, at connection start**, based on `shellID != ""`, the
`STAPLER_SQUAD_USE_CONTROL_MODE` env var, and `instance.Snapshot().IsManaged`:

- `streamShellViaControlMode` (line 495) — shell tabs, control-mode enabled
- `streamViaTmuxCapturePane` (lines 498, 539) — shell tabs w/ CM disabled, or unmanaged/external
  sessions, or CM globally disabled
- `streamViaControlMode` (line 525) — main managed session, CM enabled

VERIFIED: within a single WebSocket connection's lifetime, only one handler function runs —
there is no code path where both write to the same `stream`/xterm.js instance concurrently
*within one connection*.

UNVERIFIED / not fully closed: whether a **reconnect** (client drops and re-establishes a new
WebSocket while the *old* connection's goroutines haven't fully torn down yet) could produce a
brief window where an old handler's in-flight `stream.WriteMessage` races a new handler's first
writes to the *same underlying xterm.js instance* on the client (as opposed to the same Go
`stream` object, which is connection-scoped). This would require tracing the frontend
reconnect/dispose sequence in `useTerminalStream.ts` and the old handler's `doneChan`/context
cancellation teardown timing, which was not completed in this pass — flagged as a gap, not
ruled in or out.

## Requirement-by-requirement answers

**#1 — full pipeline, exact file/function names**: see diagram above. Confirmed commit
`5a4fb9d6b` (bufio.Scanner buffer pooling) does **not** touch `session/tmux/control_mode.go` at
all (`git show --stat 5a4fb9d6b` shows only `session/artifacts/scan.go`, `session/history.go`,
`session/scanbuf/scanbuf.go`, `session/tokens/parser.go`) — ruled out as a contributor.
`control_mode.go`'s own `bufio.NewScanner` (line 243) uses an unpooled, default-size buffer,
unrelated to that commit.

**#2 — buffering with a flush trigger that could separate an EL sequence from its text**: three
candidates identified, in order of likelihood (see "Root cause" section below):
1. `broadcastControlModeUpdate`'s per-subscriber silent drop-on-full (control_mode.go:683-697)
2. `RedrawThrottler`'s discard-on-supersede (TerminalStreamManager.ts:42-92) — likely inapplicable
   per the escape-sequence-shape analysis above (INFERRED)
3. Go-side coalescing loop (connectrpc_websocket.go ~786-892) — analyzed and **ruled out** as a
   direct cause: it only concatenates already-arrived frames in arrival order, never reorders or
   splits them.

**#3 — does control_mode.go operate on raw bytes or do line-splitting/reassembly**: VERIFIED —
it operates on newline-delimited **protocol lines** via `bufio.Scanner` (tmux control mode's own
wire format: `%output %pane data\n`), not an undifferentiated byte stream. No evidence found of
any splitting/reassembly applied to `%output` *payload* content itself — `cmdBodyBuf`
accumulation exists only for `%begin`/`%end` command-response bodies (a different protocol
message class), not for `%output` frames. Each `%output` line's decoded payload is forwarded to
`broadcastControlModeUpdate` as one atomic `[]byte` — meaning a single `%output` frame is never
itself corrupted by control_mode.go's own scanning; the risk is a **whole frame being dropped**
by the downstream broadcast, not a frame being mis-parsed.

**#4 — two capture paths, could they write to the same xterm.js instance at overlapping times**:
see "Two capture paths" section above. Mutually exclusive per-connection as dispatched;
reconnect-race window not fully ruled out (named as a gap).

**#5 — single source of truth for "what should be on screen"**: NO. VERIFIED: the MOSH-based
state-sync/reconciliation system that would have provided this is deleted
(`0ac0ca1dad9f102d7072857c87714fb2d1905e05`, PR #163). The current control-mode path is a pure
raw-byte passthrough with **no resync mechanism** other than:
- A full-content resend on resize (`instance.CapturePaneContentRaw()` + `withCursorSync`,
  triggered by the resize-handling goroutine, connectrpc_websocket.go ~894-989)
- A full reconnect from the client (new handshake → fresh `CapturePaneContentRaw()` snapshot)

Between those two events, if `broadcastControlModeUpdate` drops a frame, there is **no
detection and no recovery** — the client and the real tmux pane silently diverge until the next
resize or reconnect. This directly matches the reported symptom's persistence characteristics
(a stray fragment that "survives" until something forces a fresh full-screen redraw).

## Flow-control gap (found investigating requirement #2)

The frontend computes watermark-based backpressure (`HIGH_WATERMARK`/`LOW_WATERMARK`,
`TerminalStreamManager.ts:31-34`) and a server-side PTY-pause mechanism exists
(`pauseCh`/`ptyPaused`, `server/services/session_service.go:2384-2409`, handling
`TerminalData_FlowControl` at ~lines 2565-2577) — but VERIFIED via the function's own doc
comment (`session_service.go:2268-2272`):

> "NOTE: browser clients never reach this method directly — the WebSocket handler
> (connectrpc_websocket.go) intercepts StreamTerminal calls made over its custom websocket
> transport before they reach here. This handler exists to satisfy the ConnectRPC service
> interface and could be used by non-browser gRPC/Connect clients."

This flow-control implementation is **dead code for the browser/tmux path** — it only serves a
different (non-tmux, direct-PTY) ConnectRPC bidi-stream method that browser clients never call.
Cross-checked against `connectrpc_websocket.go`: a whole-file grep for
`"FlowControl|Paused|pause"` returns zero matches, and `streamViaControlMode`'s WebSocket read
loop (goroutine 2, lines ~1008-1131) only handles `Input`, `Resize`, and `ScrollbackRequest`
cases — no `FlowControl` case exists. **The live tmux-backed web terminal path has no
backpressure mechanism at all.** Nothing throttles the producer side when
`broadcastControlModeUpdate`'s 100-slot channel fills, other than the drop itself.

## Root cause: most likely site of data loss (VERIFIED reasoning chain, INFERRED final link)

**Primary candidate: `broadcastControlModeUpdate`'s non-blocking, silent, per-subscriber
drop-on-full-channel (`session/tmux/control_mode.go:683-697`, feeding the 100-slot channel
created at `control_mode.go:707`).**

Reasoning chain:
1. tmux's own `%output` batching is not delta-per-escape-sequence — a single `%output` line can
   contain an arbitrary number of interleaved escape sequences and text bytes, but it is also
   possible for tmux to emit the EL sequence and the replacement text as **separate** `%output`
   notifications if they were written to the pty in separate writes/flushes by the TUI process
   (a plausible pattern for a status-line redraw done as cursor-up + erase, then a second write
   of new content — INFERRED, not directly observed from a captured tmux session transcript).
2. If so, each becomes a separate frame pushed through `broadcastControlModeUpdate`.
3. Under load (a burst of TUI redraws, e.g. rapid tool-call status updates), the 100-slot
   channel can fill. The select-with-default drop is **silent** (only a log warning) and
   **independent per subscriber** — meaning exactly one specific frame (e.g., one containing
   only the EL sequence for the old longer line) can be dropped while the very next frame
   (containing the new shorter text, with no EL of its own) survives and is delivered.
4. Nothing downstream can detect this: the Go-side coalescing loop (connectrpc_websocket.go
   ~786-892) only concatenates whatever arrives, in order, with no gap detection. The frontend
   decoder and `TerminalStreamManager.write()` path likewise have no way to know a frame was
   skipped upstream.
5. There is no resync mechanism (requirement #5) until the next resize or reconnect triggers a
   full-snapshot recapture — matching the reported bug's "stray fragment persists" behavior.
6. This mechanism is confirmed to apply identically to both the main-session path
   (`streamViaControlMode`) and shell tabs (`streamShellViaControlMode`), since both subscribe
   to the same `broadcastControlModeUpdate` producer via `SubscribeToControlModeUpdates()`.

Status of this chain: steps 2-6 are VERIFIED against the actual code. Step 1 (whether tmux
actually splits an EL-then-text redraw into two separate `%output` notifications under real
Claude Code/Ink TUI behavior, and whether the channel realistically fills under normal —not
synthetically loaded— operation) is INFERRED, not confirmed against a captured live trace. This
is the one link needed to move this from "most structurally plausible candidate" to "confirmed
root cause" — recommend capturing a raw `tmux -C attach-session` transcript during a reproduction
of the bug to check whether frame drops (via the existing "control mode subscriber channel full,
dropping update" log line) or the split-notification pattern actually occurs at the reported
time.

**Secondary candidate (downgraded): `RedrawThrottler`'s discard-on-supersede
(`TerminalStreamManager.ts:42-92`)**. Initially the leading hypothesis given its exact structural
match to "debounce with flush trigger that discards data" (requirement #2), but the isFullRedraw
regex (`/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/`) requires `\x1b[2K` or `\x1b[J`, not plain `\x1b[K`.
This repo's own escape-code test fixtures use plain `\x1b[K` for line-clear, which is also the
more common Ink/status-spinner idiom (cursor-up + erase-to-end-of-line, not erase-whole-line).
If Claude Code's actual spinner sequence matches that (INFERRED, not captured live), it bypasses
this throttle entirely. Not ruled out — only downgraded — because it's plausible some redraw
paths (e.g. full-screen clears on larger UI transitions) do use `\x1b[2K`/`\x1b[J` and could
still exhibit this discard behavior for those specific redraws.

**Ruled out**: 
- Commit `5a4fb9d6b` scanner-buffer pooling (doesn't touch `control_mode.go`).
- The Go-side output-coalescing loop in `streamViaControlMode` (order-preserving by construction).
- `streamViaTmuxCapturePane` / `ExternalTmuxStreamer`'s drop-on-full `outputChan`
  (`connectrpc_websocket.go` ~1685-1699, `session/external_tmux_streamer.go`) — these deliver
  full-snapshot content per notification (not incremental deltas), so a drop here is self-healing
  on the next poll/notification, unlike the incremental delta drop in `broadcastControlModeUpdate`.
- Two-capture-paths overlap within a single connection (mutually exclusive by dispatch).

## Open gaps not resolved in this pass

- No live-captured transcript confirming tmux actually splits an EL-then-text redraw across two
  `%output` notifications (root-cause chain step 1, above).
- Reconnect-race window between an old and new WebSocket handler writing to the same client-side
  xterm.js instance not fully ruled out (requirement #4 caveat).
- `useTerminalStream.ts`'s full message-dispatch switch not re-read line-by-line this pass; no
  evidence of additional buffering found via the sections read, but not exhaustively verified.
- No captured example of Claude Code/Ink's actual spinner-line escape sequence bytes was found
  in this repo's test fixtures or docs to directly confirm/refute the `RedrawThrottler` regex
  mismatch — inferred from the codebase's own EL-sequence test fixtures using plain `\x1b[K`.
