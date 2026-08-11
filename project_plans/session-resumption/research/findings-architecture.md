# Architecture Research: Session Resumption

_Researched: 2026-04-03_

---

## Session Checkpoint Schema Patterns

### Recommended: Adjacency List (Parent-ID Self-Reference)

The simplest and most portable schema. A `Checkpoint` struct has a `ParentID` that references another checkpoint. Sessions carry a `CurrentCheckpointID`. This models a DAG identically to how git models commits.

```go
// Add to session/storage.go (existing JSON storage model)
// NOTE: This codebase does NOT use Ent ORM — sessions are stored as JSON files.
// Do not introduce Ent; extend the existing JSON storage structs.

type Checkpoint struct {
    ID             string    `json:"id"`              // UUID
    SessionID      string    `json:"session_id"`      // owning session
    ParentID       string    `json:"parent_id,omitempty"` // empty = root
    Label          string    `json:"label,omitempty"`
    ScrollbackSeq  int       `json:"scrollback_seq"`  // line index in scrollback JSONL
    ScrollbackPath string    `json:"scrollback_path"` // absolute path to scrollback file
    ClaudeConvUUID string    `json:"claude_conv_uuid,omitempty"` // ~/.claude/projects/<hash>/<UUID>
    GitCommitSHA   string    `json:"git_commit_sha,omitempty"`
    CreatedAt      time.Time `json:"created_at"`
}

// Additions to existing Instance struct
type Instance struct {
    // ... existing fields ...
    Checkpoints    []Checkpoint `json:"checkpoints,omitempty"`
    CheckpointID   string       `json:"checkpoint_id,omitempty"` // active checkpoint
    ForkedFromID   string       `json:"forked_from_id,omitempty"` // source checkpoint for fork
}
```

**Why not Ent ORM?** The codebase stores sessions in `session/storage.go` as JSON files, not via Ent ORM. The `session/ent/schema/` directory exists but the primary session storage is JSON-based. Adding checkpoint fields to existing JSON structs is the right fit.

**Why adjacency list?** Works in both JSON storage and any future relational DB without extensions. Mirrors git commit model — well-understood. Tree traversal with `parent_id` chain is fast for the depth expected (tens, not thousands, of checkpoints per session).

### Materialized Path (Optional Enhancement)

For queries like "show all checkpoints descended from checkpoint X", store a path string:
```go
Path string `json:"path,omitempty"` // e.g. "root/ckpt-1/ckpt-2/ckpt-3"
```

Skip this in the initial implementation — adjacency list is sufficient for the expected usage.

---

## Forking / Branching State

### Fork Model: Full Clone (Recommended)

At fork time, copy the scrollback file up to the checkpoint sequence into a new file. The forked session gets a fresh `FileScrollbackStorage` backed by the cloned file.

```go
func ForkScrollback(srcPath string, upToSeq int, dstPath string) error {
    src, err := os.Open(srcPath)
    if err != nil { return err }
    defer src.Close()

    dst, err := os.Create(dstPath)
    if err != nil { return err }
    defer dst.Close()

    scanner := bufio.NewScanner(src)
    const maxLineBuf = 10 * 1024 * 1024
    scanner.Buffer(make([]byte, maxLineBuf), maxLineBuf)

    for i := 0; scanner.Scan() && i < upToSeq; i++ {
        dst.WriteString(scanner.Text() + "\n")
    }
    return scanner.Err()
}
```

**Why full clone over copy-on-write?**
- Terminal scrollback files are small (typical: < 5 MB per session)
- Fork operations are infrequent (user-initiated)
- Full clone keeps `FileScrollbackStorage` read path identical — no interface changes
- No shared-state complexity; fork is fully independent; parent deletion doesn't affect child

### Claude Conversation Fork

Claude's JSONL conversation history (at `~/.claude/projects/<hash>/<uuid>`) also needs to be forked at a message index. Two approaches:

**Option A — Native Claude fork (preferred if available)**: Invoke `claude --fork <parentMessageUUID>` which creates a new session file natively with `isSidechain: true`. stapler-squad then tracks the new UUID.

**Option B — Manual JSONL copy + truncate**: Copy the conversation JSONL file to a new UUID path under the same project directory, truncated to the message index at checkpoint time. Start the forked session with `claude --resume <new-uuid>`.

```go
func ForkClaudeConversation(srcConvPath string, upToMessage int, dstDir string) (string, error) {
    newUUID := uuid.New().String()
    dstPath := filepath.Join(dstDir, newUUID)

    src, err := os.Open(srcConvPath)
    if err != nil { return "", err }
    defer src.Close()

    dst, err := os.Create(dstPath)
    if err != nil { return "", err }
    defer dst.Close()

    scanner := bufio.NewScanner(src)
    scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)
    msgCount := 0
    for scanner.Scan() {
        line := scanner.Text()
        // Count user/assistant messages
        if strings.Contains(line, `"type":"user"`) || strings.Contains(line, `"type":"assistant"`) {
            if msgCount >= upToMessage { break }
            msgCount++
        }
        dst.WriteString(line + "\n")
    }
    return newUUID, scanner.Err()
}
```

---

## PTY Proxy / Adopted Process Bridging

### The Architecture (Already Correct)

The existing `claude-mux` binary (`cmd/claude-mux/main.go`, `session/mux/multiplexer.go`) is the PTY master for all externally-started sessions. Its Unix socket model already supports multiple concurrent clients — stapler-squad connects as another client.

```
Original Terminal ←── STDIO ──→ claude-mux ←── PTY master ──→ Claude process
                                     │
                               Unix socket
                           /tmp/claude-mux-<PID>.sock
                                     │
                   ┌─────────────────┴──────────────────┐
                   │                                    │
             stapler-squad                    (future: 2nd observer)
          (Output → scrollback + UI)
          (UI Input → PTY via socket)
```

The original terminal's stdio are wired directly to `claude-mux` stdin/stdout — **not** through the socket. Socket clients get the same output but don't affect the original terminal.

### Adopted Process Bridge Implementation

```go
// New file: session/bridge/adopted_bridge.go
type AdoptedBridge struct {
    socketPath string
    scrollback ScrollbackWriter
    broadcast  OutputBroadcaster
    done       chan struct{}
}

func (b *AdoptedBridge) Run(ctx context.Context) error {
    var conn net.Conn
    var err error

    // Retry on startup race (socket may not exist yet)
    for _, delay := range []time.Duration{0, 100*time.Millisecond, 300*time.Millisecond, time.Second} {
        time.Sleep(delay)
        conn, err = net.Dial("unix", b.socketPath)
        if err == nil { break }
    }
    if err != nil { return fmt.Errorf("failed to connect to claude-mux socket: %w", err) }
    defer conn.Close()

    go b.readOutputLoop(conn)   // socket Output msgs → scrollback + broadcast
    go b.writeInputLoop(conn)   // web UI input → socket Input msgs
    <-ctx.Done()
    return nil
}

// Checkpoint creation requires NO process interruption:
func (b *AdoptedBridge) TakeCheckpoint(label string) Checkpoint {
    return Checkpoint{
        ID:             uuid.New().String(),
        ScrollbackSeq:  b.scrollback.LineCount(),
        ScrollbackPath: b.scrollback.Path(),
        CreatedAt:      time.Now(),
        Label:          label,
    }
}
```

### New Sessions: io.MultiWriter

For sessions started directly by stapler-squad (not adopted), tee PTY output to both scrollback and web UI:

```go
multiWriter := io.MultiWriter(scrollbackWriter, webSocketBroadcaster)
// Wire to tmux capture or pty master io.Copy
```

---

## Event Sourcing / Replay Patterns

### The Scrollback File IS the Event Log

`FileScrollbackStorage` already stores terminal output as append-only JSONL — this is an event log. A checkpoint is a `(line_index, timestamp)` pointer into this log. No additional event infrastructure needed.

### Replay Strategy for Cold Restore

When resuming from a checkpoint after a cold restart (tmux session dead):

1. **Fast path** (if `TerminalState` snapshots are added later): deserialize rendered VT state — O(1). Skip for initial implementation.
2. **Current path**: Read scrollback lines from 0 to `ScrollbackSeq` and feed into a new tmux pane to reconstruct visible history. O(N) but fast for typical session sizes.

```go
// Feed saved scrollback into new tmux pane (simplified)
entries, _ := storage.Read(sessionID, 0, checkpoint.ScrollbackSeq)
for _, entry := range entries {
    // Write raw terminal data to tmux pane stdin
    tmuxPane.Write(entry.Data)
}
```

### Snapshot Optimization (Future)

If replay latency becomes a problem for large scrollbacks (>50k lines), add a `TerminalState []byte` field to `Checkpoint` storing a serialized `vterm` or `xterm.js` state dump. Not needed for initial implementation.

---

## Recommended Architecture Summary

| Concern | Recommendation |
|---|---|
| Checkpoint storage | Add `Checkpoint` struct to existing JSON session storage (`session/storage.go`) |
| Checkpoint tree | Adjacency list (`ParentID` self-ref in `Checkpoint`) |
| Scrollback fork | Full clone via `ForkScrollback()` — simple, independent, no shared state |
| Claude conversation fork | Copy + truncate JSONL to new UUID (or `claude --fork` if available) |
| PTY bridge for adopted sessions | Connect as additional `claude-mux` socket client (already the right model) |
| PTY bridge for new sessions | `io.MultiWriter` to tee output to scrollback + web UI |
| Session identity | Composite: `{ClaudeConvUUID, MuxSocketPath, PID, PIDStartTime}` |
| VT state snapshots | Skip initially; add as optimization if replay latency is a problem |
| Ent ORM | Do NOT introduce. Extend existing JSON storage structs. |

### Fork Operation Sequence

```
User triggers "fork from checkpoint C" on session S
│
├─ 1. Create new Instance record
│     - New UUID, ForkedFromID = C.ID
│     - New git branch (git checkout -b from C.GitCommitSHA)
│     - New tmux session name
│
├─ 2. Clone scrollback
│     ForkScrollback(C.ScrollbackPath, C.ScrollbackSeq, newScrollbackPath)
│
├─ 3. Clone Claude conversation
│     ForkClaudeConversation(C.ClaudeConvUUID, C.MessageIndex, newProjectDir)
│     → returns newClaudeUUID
│
└─ 4. Start new session
      - New tmux session, replay scrollback clone into pane
      - Launch: claude --resume <newClaudeUUID>
      - Register AdoptedBridge or direct PTY management for new session
```

---

## Sources

1. Codebase inspection: `session/storage.go`, `session/instance.go`, `session/scrollback/storage.go`, `session/mux/multiplexer.go`, `session/history.go`
2. https://entgo.io/docs/schema-edges — Ent ORM edge docs (confirmed: not used for primary session storage)
3. https://event-driven.io/en/how_to_do_snapshots_in_event_sourcing/ — Event sourcing snapshot patterns
4. https://martinfowler.com/eaaDev/EventSourcing.html — Event Sourcing by Fowler
5. https://pkg.go.dev/github.com/creack/pty — Go PTY library
6. https://man7.org/linux/man-pages/man7/pty.7.html — PTY semantics
7. https://github.com/nelhage/reptyr — PTY adoption reference (Linux model)
8. Brave Search: "conversation branching fork checkpoint immutable history tree database schema" — 2026-04-03
9. Brave Search: "event sourcing session checkpoint replay Go" — 2026-04-03
10. Brave Search: "Unix PTY proxy bridge multiple readers tee io.MultiWriter" — 2026-04-03
11. Direct inspection of `~/.claude/projects/` JSONL format — 2026-04-03
