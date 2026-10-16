# Stack Research: import-external-session

## Headline finding

Every piece of plumbing this feature needs already exists in-repo and is battle-tested. This is
almost entirely a **composition** problem (wire existing packages into a new action/flow), not a
new-dependency problem. No new Go modules are required for the core mechanics described in the
constraints (proc inspection, process kill, tmux discovery/attach). The only genuinely new surface
is UI/state for "batch import" and "confirm-before-kill", which is Go/React application code, not
a library gap.

## (a) Correlating an OS process to files it has open

**Already implemented — reuse as-is:**

- `session/procinfo/inspector.go` (`ProcessInspector`, darwin) and `session/procinfo/inspector_other.go`
  (linux/other) wrap `github.com/shirou/gopsutil/v4/process` (already in `go.mod`, v4.26.5) for
  `Cwd(pid)`, `CreateTime(pid)`, `IsAlive(pid, expectedCreateTimeMs)`.
- `session/procinfo/openfiles_darwin.go` — macOS-only, `cgo` + `libproc.h`, calls
  `proc_pidinfo(PROC_PIDLISTFDS)` + `proc_pidfdinfo(PROC_PIDFDVNODEPATHINFO)` directly. This is the
  "proc_pidinfo" mechanism named in the task — it deliberately avoids shelling out to `lsof`.
  Linux equivalent is not yet implemented (`inspector_other.go` has no `OpenFiles`); Linux would
  read `/proc/<pid>/fd/*` symlinks — cheap to add, no new dependency, gopsutil's
  `process.Process.OpenFiles()` also wraps this natively on Linux if we want to avoid a bespoke
  syscall path there.
- `session/history_detector.go` — `HistoryFileDetector` takes a `ProcessFileInspector` interface
  (`OpenFiles`, `Cwd`, etc. — consumer-defined interface, correctly scoped per this repo's
  interface-pollution rule) and correlates a PID to its `~/.claude/projects/*.jsonl` file via
  `Detect(pid)` (open-file based) and `DetectByPath(projectPath)` (path-hash based, using
  `ClaudeProjectDirName`). This is exactly the JSONL correlation path called out in the
  requirements' constraints.
- `session/history_linker.go` — `HistoryLinker` wraps `HistoryFileDetector` + `fsnotify`-based
  `HistoryFileWatcher` (`golang.org/x/... ` — actually `github.com/fsnotify/fsnotify v1.9.0`,
  already in `go.mod`) and does periodic + event-driven correlation with backoff
  (`sessionBackoff`) for sessions that haven't yet produced a matching file.
- `session/pty_discovery.go` — `PTYDiscovery`/`PTYConnection` batches `tmux list-panes` output with
  PID lookups (`batchProcessStates`, `batchIsClaudeProcess`) to classify PTYs as
  managed/orphaned/external-Claude. This is the mechanism that currently powers "external Claude
  session" visibility and is the natural extension point for the plain-tmux (no ssq-mux) import
  path.

**Net-new for this feature:** none for macOS. For Linux, either extend `inspector_other.go` with an
`OpenFiles` implementation (read `/proc/<pid>/fd`, ~20 lines, stdlib only) or switch both platforms
to gopsutil's cross-platform `process.Process.OpenFiles()` (already vendored) for parity — worth a
design decision in planning, not a new dependency.

## (b) Safely killing an external process cross-platform, with confirmation

**Already implemented, mature pattern — reuse `executor` package idioms:**

- `executor/managed_process.go` + `executor/managed_process_unix.go` (`killProcessGroup`) implement
  the canonical graceful-kill pattern this repo already uses everywhere: SIGTERM to the process
  group first (`syscall.Kill(-pgid, syscall.SIGTERM)`), wait up to a configurable grace period
  (`WithGracePeriod`, default 5s), escalate to SIGKILL on timeout ("belt-and-suspenders"). This is
  the pattern to reuse for "confirm-before-kill" of the external session's process — it already
  separates "ask nicely" from "force," which maps directly onto the requirement that kill must
  follow explicit confirmation and only after import is verified.
- `daemon/daemon.go` has a simpler `os.FindProcess(pid)` + `proc.Kill()` fallback for daemon-owned
  PIDs — lighter weight, useful precedent if the external process isn't a full process-group leader
  (e.g. attaching to a plain tmux pane's shell rather than a process we spawned).
- `session/vnc/display_alloc.go` uses `os.FindProcess` + `Signal(0)`-probe (`syscall.Kill(pid, 0)`
  equivalent) purely to test liveness without killing — a good "is this still the same process I
  saw when I imported" pre-kill sanity check (guards the requirement that kill only happens after
  import is verified against the *same* process, avoiding a PID-reuse race).
- `session/mux/hooks.go` also does the `os.FindProcess` + `Signal(nil)` liveness probe pattern.

No new library needed — `os.FindProcess`/`syscall.Kill` (stdlib `syscall`, already used pervasively)
covers Unix; this repo doesn't currently target Windows for tmux-backed sessions, so no
`golang.org/x/sys/windows`-specific kill path is needed unless Windows support is separately in
scope (check — `session/tmux/tmux_windows.go` exists, so Windows *is* a supported OS; if so, the
existing Windows kill path should be audited, likely via `github.com/Microsoft/go-winio` deps
already indirect in `go.mod`, or `taskkill`/`os.Process.Kill()` which on Windows calls
`TerminateProcess` directly — no group-signal semantics, so the executor's Unix-specific
`killProcessGroup` will need a Windows counterpart if this feature must support Windows-hosted
external sessions).

**PID-reuse safety:** `procinfo.ProcessInspector.IsAlive(pid, expectedCreateTimeMs)` already exists
specifically to guard against killing a *different* process that happens to reuse a PID — this
should be the required pre-kill check for the confirm-before-kill flow (verify `CreateTime` still
matches what was recorded at discovery/import time).

## (c) tmux session/pane discovery and attach/detach semantics

**Already implemented — shells out to real `tmux` binary, no Go tmux library used or needed:**

- `session/tmux/tmux.go` — `TmuxSession` wraps `attach-session`/`detach` via `exec.Cmd` + a PTY
  (`github.com/creack/pty v1.1.24`, already in `go.mod`). `ListAllSessions`, `IsServerDown` shell
  out to `tmux list-sessions`. Socket isolation via `-L <serverSocket>` flag consistently threaded
  through `Socket` type / `ResolveSocket` / `prependSocket` (enforced by a custom lint rule,
  `tools/lint/tmuxsocketscope`).
- `session/tmux/control_mode.go` — `StartControlMode`/`StopControlMode` use `tmux -C attach-session`
  (control mode protocol) for structured pane I/O without a raw PTY attach; this is the mechanism
  `STAPLER_SQUAD_USE_CONTROL_MODE` toggles and is the preferred path per `CLAUDE.md`.
  `SubscribeToControlModeUpdates`/broadcast machinery already supports multiple consumers watching
  the same pane — directly reusable for "attach live to verify import before confirming kill."
- `session/mux/discovery.go` (`Discovery.Scan`, `ScanFromUserOptions`, `StartPolling`) and
  `session/mux/multiplexer.go` are the `ssq-mux` side: `DiscoveredSession` + `SessionMetadata`
  represent an unmanaged, ssq-mux-wrapped external session. `probeSocket` liveness-checks a socket
  directly. This is the exact `DiscoveredSession`-to-`Instance` promotion path named in scope item
  1 — the import action for this path is "take a `DiscoveredSession`, construct an `Instance` with
  matching `SessionType`/path/pid, and call the existing `HistoryLinker`/adapter machinery to pull
  history," not new tmux code.
- `session/pty_discovery.go` batches `tmux list-panes` + PID checks for the plain-tmux
  (no-ssq-mux) discovery path — `discoverOrphanedPTYs`/`discoverExternalClaude` already identify
  candidate external Claude PTYs; import for scope item 2 is largely "let the user pick one of
  these and run the same promotion logic."
- `session/instance_hibernate.go` documents the destructive kill-session semantics
  (`tmux kill-session`) already used for stapler-squad's own lifecycle — same command applies to
  killing the external tmux session/pane post-confirmation once it's a plain-tmux target rather than
  a process-group kill.

No tmux Go binding library (e.g. no `gotmux`) is used or should be introduced — the repo's
established pattern is shelling out to the real `tmux(1)` binary with careful socket/argument
handling, and introducing a library binding at this point would be inconsistent with the rest of
the codebase and the bundling-tmux single-binary deployment model (`.claude/docs/bundling-tmux.md`).

## HistoryAdapter / conversation import-export

- `session/history_adapter.go` — tiny consumer-defined interface (`Name`, `CanHandle`, `Import`,
  `Export`) — exactly scoped, no speculative generality.
- `session/claude_adapter.go` (`ClaudeAdapter`) parses `~/.claude/projects/*.jsonl` turn-by-turn.
- `session/agy_adapter.go` (`AgyAdapter`) parses Antigravity/Gemini's sqlite-backed step log via
  `github.com/mattn/go-sqlite3` (already in `go.mod`) — this is the concrete precedent for "any
  `HistoryAdapter`-supported non-Claude program" named in scope.
- Both adapters produce `CanonicalTurn` — the natural target type for "verify import succeeded
  before allowing kill" (diff imported `CanonicalTurn`s against the source JSONL/sqlite to confirm
  zero data loss, satisfying the requirements' success metric).

## Dependency inventory summary

| Need | In-repo already | Net-new |
|---|---|---|
| Proc → open files (macOS) | `session/procinfo` (cgo + `proc_pidinfo`) | — |
| Proc → open files (Linux) | gopsutil `process.Process` available but not wired for `OpenFiles` | small addition, no new module |
| PID liveness / create-time guard (anti PID-reuse) | `procinfo.IsAlive`, `vnc/display_alloc.go`, `mux/hooks.go` | — |
| Graceful kill (SIGTERM→grace→SIGKILL, process-group aware) | `executor/managed_process*.go` | Windows-specific kill path if Windows-hosted external sessions are in scope |
| tmux discovery/list-panes/list-sessions | `session/tmux`, `session/pty_discovery.go` | — |
| tmux attach/detach/control-mode | `session/tmux/tmux.go`, `control_mode.go`, `creack/pty` | — |
| ssq-mux discovered-session model | `session/mux/discovery.go`, `multiplexer.go` | promotion logic: `DiscoveredSession` → `Instance` (new, but composition not new tech) |
| JSONL/UUID correlation | `session/history_detector.go`, `history_linker.go` | — |
| Conversation import/export | `session/history_adapter.go`, `claude_adapter.go`, `agy_adapter.go` | — |
| Batch orchestration + confirm-before-kill UX | none | new: Go service layer (likely `server/services/session_service.go` + new RPC) and React modal/flow; see session-creation-registry.md if this becomes a new `SessionType` |

## Recommendation

Treat this as a new action/flow composing existing packages rather than a new `SessionType`
(7-touchpoint mode) — the imported session, once promoted, should almost certainly become
`SessionTypeDirectory` or a lifecycle matching its resolved worktree/path, with the "import" itself
being a one-time RPC + confirmation UI, not a persistent creation mode. This mirrors the
`autonomous` exception pattern in `.claude/rules/session-creation-registry.md` (reuse existing
`SessionType`, drive behavior via request flags) rather than adding new enum values — final call
belongs to Phase 3 planning, but the stack strongly favors "lighter action bolted onto discovery
flows" per the open question in requirements.md, since every piece of *infrastructure* needed
already lives outside the 7-touchpoint system entirely.
