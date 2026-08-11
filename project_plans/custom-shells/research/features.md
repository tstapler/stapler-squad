# Features Research: Custom Shells

## Existing Tab / Multi-Panel UI Patterns

`SessionDetailView` currently renders a tab strip with a static union type:
```ts
export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files";
```

The tab strip is hard-coded HTML. To support dynamic shell tabs, this union will need to extend to `"shell:{shellId}"` or a separate dynamic tab section alongside the static tabs.

The **terminal pool** in `SessionDetailView` is a key existing pattern: up to 8 xterm instances are kept alive using `display:none` / `display:block` CSS toggling (absolute positioning trick). Each pool slot is assigned a `sessionId`; switching tabs swaps visibility without unmounting. Shell tabs should reuse this same pool mechanism — each shell tab becomes a pool slot identified by `shellId`.

The `isExternal` prop on `useTerminalStream` and the `TerminalOutput` component shows that the streaming path already supports endpoint variants (e.g., `/ws/external`). A `shellId` prop could similarly route to a shell-specific stream.

## xterm.js Integration

`TerminalOutput.tsx` (1396 lines) owns the full xterm.js lifecycle:
- `XtermTerminal` ref holds the xterm instance.
- `TerminalStreamManager` abstracts the stream ↔ xterm write path.
- `useTerminalStream` hook manages the ConnectRPC WebSocket.
- Resize stability detection: 50ms debounce, cached dimensions via `getCachedDimensions` / `saveDimensions`.
- Scrollback: 5000-line buffer configured in xterm options; paged historical scrollback loaded on scroll-to-top via `requestScrollback`.
- Flow control: `sendFlowControl` sends pause/resume signals to the server.
- Toolbar: debug mode, logstream, recording, streaming mode selector.

For shells, `TerminalOutput` can be reused with a `shellId` parameter that overrides the connection target. The key change is that `useTerminalStream` needs to pass `shell_id` in the initial handshake `CurrentPaneRequest` so the server routes it to the right PTY.

Scrollback for shells should be scoped by `shellId`. The existing scrollback recording infrastructure (`session/scrollback/`) stores data keyed by session ID — extending with `{sessionId}/{shellId}` is straightforward.

## Status Indicator Requirements (US-3)

The requirements call for running/stopped/error status indicators on shell tabs. The closest existing pattern is `SessionStatus` in the session list — status badges rendered with CSS variables from `globals.css` (`--success`, `--error`, `--warning`). Shell status should follow the same design system (vanilla-extract styles consuming the same tokens).

Status lifecycle:
- `running` — PTY is attached and process is alive.
- `stopped` — process exited normally (exit code 0).
- `error` — process exited non-zero or PTY error.

The frontend will need server-push notification when shell status changes. Options:
1. Add a `ShellStatusUpdate` message type to the `TerminalData` oneof — server sends it when the watched process exits.
2. Poll via a separate RPC.

Option 1 is consistent with the existing pattern (ResizeQuiescence, Error messages are already pushed via the stream).

## Shell Restart / Stop / Close (US-4)

Existing analogues for session-level actions: `pause_session`, `resume_session`, `delete_session` are all separate ConnectRPC unary RPCs (`PauseSession`, `ResumeSession`, `DeleteSession` in `session.proto`). Shell operations should follow the same pattern:
- `StartShell(StartShellRequest) returns (StartShellResponse)` — creates and starts a shell.
- `StopShell(StopShellRequest) returns (StopShellResponse)` — sends SIGTERM to shell process.
- `RestartShell(RestartShellRequest) returns (RestartShellResponse)` — stop + start.

Closing a shell tab (removing from UI) is distinct from stopping the process — the tab can be closed while leaving the process running, or close-and-kill can be a combined action.

## Persistence Across Pause/Resume (US-5)

The existing pause/resume flow for sessions:
- Pause: `messageQueueRef.current.close()` on frontend → `abortController.abort()` → server closes stream.
- Resume: `connect()` on frontend → new `StreamTerminal` RPC → server re-attaches PTY.

For shells, the PTY survives in the tmux window after the client disconnects (tmux keeps the process running). On resume, the server re-attaches to the same tmux window. The shell's persisted state (ID, tmux window index, start command, status) must be stored in ent so it can be restored after server restart.

`session/scrollback/` already handles scrollback persistence across reconnects for the main terminal. Shell scrollback should use the same infrastructure keyed by `{sessionId}/{shellId}`.

## Multiple Shells Per Session (US-2, US-6)

The requirements specify multiple shells per session, each with its own tab. Implementation checklist:
1. Backend: `Session` has many `Shell` entities via ent FK.
2. Each shell maps to one tmux window in the session's tmux session.
3. Frontend: dynamic tab list derived from a `ListShells(session_id)` RPC response.
4. Terminal pool slots assigned per `shellId` (same pool, different key).
5. Tab strip renders static tabs (Terminal, Diff, etc.) + dynamic shell tabs.

The `SessionDetailTab` union type in `SessionDetail.tsx` will need to either be loosened (`string`) or extended with a discriminated pattern. The dynamic tabs should NOT be part of the static TypeScript union — they're runtime data. A clean pattern: static union for fixed tabs, a separate `shellTabs: ShellTab[]` array for dynamic ones, rendered as a separate section of the tab strip.
