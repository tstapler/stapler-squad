# Stack Research: Custom Shells

## PTY / tmux Handling

The existing PTY path is:

1. `TmuxSession.Start(workDir)` — runs `tmux new-session -d -s {name} -e CLAUDECODE= -c {workDir} {program}` to spawn the process.
2. `TmuxSession.AttachToExisting()` / `buildAttachCommand()` — runs `tmux attach-session -t {name}`, then wraps it with `pty.Start()` (via `creack/pty`). The resulting `*os.File` is stored as `t.ptmx`.
3. `TmuxProcessManager.GetPTY()` delegates to `TmuxSession.GetPTY()`, returning `t.ptmx`.
4. `Instance.GetPTYReader()` is the interface the session service calls; it proxies through `TmuxProcessManager`.
5. Resize is done via `tmux resize-window -t {name} -x {cols} -y {rows}` plus `pty.Setsize(ptmx, winsize)`.

Key constraint: **one PTY per tmux session** (one `attach-session`). There is no usage of `new-window` or `split-window` anywhere in the tmux layer. Each shell would require its own `tmux new-window -t {session}:{index}` and a corresponding `attach-session -t {session}:{window}.{pane}` PTY.

tmux target format: colons and periods are special (`session:window.pane`). `TmuxSession.sanitizeName()` already replaces colons with underscores, so window/pane index suffixes must be numeric and appended after the sanitized session name using the standard tmux target syntax.

## Session / Instance Model

`session.Instance` represents a single Claude-running session. Relevant fields:
- `TmuxPrefix` — the sanitized tmux session name.
- `tmuxManager TmuxProcessManager` — holds a single `TmuxSession`.
- `Status` — Running / Ready / Loading / Paused / NeedsApproval / Creating / Stopped.

There is **no concept of sub-processes or windows** on the Instance. Shells would be a new first-class entity managed separately from the parent session.

The `session/mux/multiplexer.go` package contains an alternative PTY-based multiplexer (used by claude-mux/external sessions). It shows the `pty.Start(cmd)` pattern independent of tmux, which is another viable approach for shells (direct PTY without tmux attach).

## ConnectRPC Bidirectional Streaming

The existing `StreamTerminal` RPC (`rpc StreamTerminal(stream TerminalData) returns (stream TerminalData)`) is a bidirectional stream multiplexed over a single WebSocket. The session is identified by `session_id` in the first `TerminalData` message.

The `TerminalData` message's `oneof data` currently contains 16 variants (output, input, resize, error, scrollback_request/response, flow_control, state, diff, input_echo, ssp_negotiation, resize_quiescence). The discriminator is the `oneof` case, not a message-level session/shell ID.

To support multiple shells, the options are:
1. **New stream per shell** — client opens a separate WebSocket stream for each shell tab, passing a shell-specific ID. This reuses the existing RPC with minimal proto changes (add `shell_id` field to `TerminalData`).
2. **Multiplex shells on one stream** — add a `shell_id` field to `TerminalData` and route server-side. Higher complexity, not needed for V1.

Option 1 is strongly recommended: it maps cleanly onto the existing per-connection PTY read loop in `StreamTerminal`.

## ent Schema

The ent client uses **`client.Schema.Create(ctx)`** (in `session/ent_repository.go` line 84). This is ent's auto-migration — it creates/alters tables on startup to match the current schema. It does **not** do destructive drops; it only adds columns and tables.

Important: the correct generate command (from `session/ent/generate.go`) requires `--feature sql/upsert`:
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

SQLite WAL mode + `MaxOpenConns=1` means all writes are serialized. Adding a `Shell` entity with a foreign key to `Session` is straightforward; no migration tooling beyond the existing auto-migrate path is needed.

Existing schema entities for reference: Session, ClaudeSession, Worktree, Tag, BacklogItem, Project, Conversation.

## Reuse vs. New Code

| Concern | Existing Code | Reuse Strategy |
|---------|--------------|----------------|
| PTY attach | `TmuxSession.AttachToExisting()` | Reuse; extend to accept a window/pane target |
| PTY resize | `TmuxSession.updateWindowSize()` | Reuse per-shell with window index |
| Terminal streaming RPC | `StreamTerminal` | Reuse; add optional `shell_id` field to `TerminalData` |
| Scrollback recording | `session/scrollback/` | Reuse with shell-scoped key |
| Frontend terminal component | `TerminalOutput` + `useTerminalStream` | Reuse; pass `shellId` to differentiate stream |
| Flow control | `FlowControl` proto message | Reuse unchanged |
| Zombie reaping | `StartZombieReaper` | Reuse unchanged |
| Session entity | `session.Instance` | Extend with shell registry (map[string]*Shell) |
| ent persistence | ent `Session` entity | Add new `Shell` entity with FK to Session |
