# Architecture Research: Custom Shells

## Shell Data Model vs. ent Schema

### Proposed ent `Shell` Entity

```go
// session/ent/schema/shell.go
type Shell struct {
    ent.Schema
}

func (Shell) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").StorageKey("id").Immutable(),        // UUID
        field.String("name"),                                    // user-visible label
        field.String("command").Default(""),                     // command to run (empty = default shell)
        field.String("working_dir").Optional(),                  // override working dir
        field.Int("tmux_window_index"),                          // window index in parent tmux session
        field.String("status").Default("running"),               // running/stopped/error
        field.Int("exit_code").Optional(),                       // last exit code
        field.Time("started_at").Default(time.Now),
        field.Time("stopped_at").Optional().Nillable(),
        field.Int("order_index").Default(0),                     // display order in tab strip
    }
}

func (Shell) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("session", Session.Type).
            Ref("shells").
            Unique().
            Required(),
    }
}
```

The `Session` schema gets a corresponding edge:
```go
edge.To("shells", Shell.Type),
```

### In-Memory `Shell` Struct (Go)

```go
// session/shell.go
type Shell struct {
    ID            string
    Name          string
    Command       string
    WorkingDir    string
    TmuxWindow    int    // window index in the parent session's tmux session
    Status        ShellStatus
    tmuxSession   *tmux.TmuxSession  // the attach-session PTY for this window
    mu            sync.Mutex
}
```

`Instance` gains a `shells map[string]*Shell` field and methods `SpawnShell`, `GetShell`, `StopShell`, `RestartShell`.

## tmux Window Architecture for Shells

### Current model
Each `Instance` has one `TmuxSession` which manages window 0 (the default window created by `new-session`). The program (Claude) runs in window 0.

### Extended model for shells
- Window 0: Claude process (unchanged).
- Window 1+: shell processes, one per `Shell`.

tmux commands:
```bash
# Spawn a new shell window
tmux new-window -t {tmuxPrefix}:{index} -c {workDir} {command}

# Attach to a specific window for PTY streaming
tmux attach-session -t {tmuxPrefix}:{index}

# Stop a shell
tmux kill-window -t {tmuxPrefix}:{index}
```

### TmuxSession extension

`TmuxSession` currently assumes it is attached to the session's default window. Extending it to support a window target requires:
1. Add `windowIndex int` field (0 = current default behavior, N = shell window).
2. `buildAttachCommand()` appends `:{windowIndex}` to the target if `windowIndex > 0`:
   ```go
   target := t.sanitizedName
   if t.windowIndex > 0 {
       target = fmt.Sprintf("%s:%d", t.sanitizedName, t.windowIndex)
   }
   // tmux attach-session -t {target}
   ```
3. `updateWindowSize()` similarly targets `{name}:{windowIndex}`.

Alternatively, a lighter `ShellTmuxSession` type can embed/wrap `TmuxSession` with only the needed methods (Start, GetPTY, Resize, Close), avoiding changes to the core struct.

## Required ConnectRPC RPCs

### New unary RPCs (in `session.proto`)

```protobuf
rpc StartShell(StartShellRequest) returns (StartShellResponse);
rpc StopShell(StopShellRequest) returns (StopShellResponse);
rpc RestartShell(RestartShellRequest) returns (RestartShellResponse);
rpc ListShells(ListShellsRequest) returns (ListShellsResponse);
rpc DeleteShell(DeleteShellRequest) returns (DeleteShellResponse);
```

### New proto messages (in `types.proto` or a new `shell.proto`)

```protobuf
message Shell {
    string id = 1;
    string session_id = 2;
    string name = 3;
    string command = 4;
    string working_dir = 5;
    ShellStatus status = 6;
    int32 exit_code = 7;
    int64 started_at = 8;
    int32 order_index = 9;
}

enum ShellStatus {
    SHELL_STATUS_UNSPECIFIED = 0;
    SHELL_STATUS_RUNNING = 1;
    SHELL_STATUS_STOPPED = 2;
    SHELL_STATUS_ERROR = 3;
}

message StartShellRequest {
    string session_id = 1;
    string name = 2;
    string command = 3;      // empty = user's default shell ($SHELL or /bin/sh)
    string working_dir = 4;  // empty = session's working_dir
}
message StartShellResponse { Shell shell = 1; }

message StopShellRequest { string session_id = 1; string shell_id = 2; }
message StopShellResponse {}

message RestartShellRequest { string session_id = 1; string shell_id = 2; }
message RestartShellResponse { Shell shell = 1; }

message ListShellsRequest { string session_id = 1; }
message ListShellsResponse { repeated Shell shells = 1; }

message DeleteShellRequest { string session_id = 1; string shell_id = 2; }
message DeleteShellResponse {}
```

### StreamTerminal extension

Add optional `shell_id` to `TerminalData`:
```protobuf
message TerminalData {
    string session_id = 1;
    string shell_id = 17;   // empty = main Claude terminal (backwards compat)
    oneof data { ... }
}
```

Add `ShellStatusUpdate` to the `oneof` so the server can push exit events:
```protobuf
message ShellStatusUpdate {
    string shell_id = 1;
    ShellStatus status = 2;
    int32 exit_code = 3;
}
// In TerminalData.oneof:
ShellStatusUpdate shell_status_update = 18;
```

## Service Layer Architecture

```
server/services/session_service.go
  ├── StreamTerminal (existing) — routes by shell_id:
  │     shell_id==""  → existing Claude PTY path
  │     shell_id!=""  → ShellPTYReader for that shell
  ├── StartShell  → instance.SpawnShell(req)
  ├── StopShell   → instance.StopShell(req.ShellId)
  ├── RestartShell → instance.RestartShell(req.ShellId)
  ├── ListShells  → storage.ListShells(sessionId) + instance.GetShellStatus(id)
  └── DeleteShell → instance.StopShell + storage.DeleteShell

session/instance.go
  └── shells map[string]*Shell  (in-memory registry)
      SpawnShell(cmd, workDir) *Shell
      GetShell(id) *Shell
      StopShell(id) error
      GetShellPTYReader(id) (*os.File, error)

session/ent_repository.go
  └── CreateShell, ListShells, UpdateShellStatus, DeleteShell
```

The `StreamTerminal` handler's fork point is clean: before entering the PTY read loop, check `req.ShellId`; if set, call `instance.GetShellPTYReader(req.ShellId)` instead of `instance.GetPTYReader()`. Everything downstream (flow control, scrollback, output forwarding) is identical.
