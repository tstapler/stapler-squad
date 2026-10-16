# Build vs. Buy — import-external-session

## Context

Existing components already implement most of the primitives this feature needs:

| Concern | Existing implementation | File |
|---|---|---|
| PID → open-files inspection (gopsutil) | `ProcessFileInspector` / `procinfo.NewProcessInspector()` | `session/procinfo/`, used by `session/history_detector.go` |
| Correlate PID/path → Claude JSONL conversation UUID | `HistoryFileDetector.Detect` / `.DetectByPath` | `session/history_detector.go` |
| Background poll + fsnotify to keep correlation fresh | `HistoryLinker` | `session/history_linker.go` |
| tmux enumeration (batched, single `list-panes -a` call) | `batchPTYInfo`, `batchPaneActivity`, `batchIsClaudeProcess` | `session/pty_discovery.go` |
| tmux session control (custom wrapper, no 3rd-party tmux lib) | `session/tmux/tmux.go` (2311 lines), `session/tmux/tmux_unix.go` | `session/tmux/` |
| ssq-mux-wrapped external terminal discovery → `Instance` | `ExternalSessionDiscovery` | `session/external_discovery.go` |
| External-session kill (tmux-aware) | `Instance.KillExternalSession()` — `tmux kill-session -t <name>` | `session/instance_tmux.go:201` |
| Native (non-tmux) process kill (SIGTERM → process group) | `NativeProcessManager.Close()` — `syscall.Kill(-pid, SIGTERM)` | `session/native_process_manager.go:187-215` |
| Conversation format abstraction (turns/blocks) | `CanonicalTurn`/`CanonicalBlock` | `session/canonical.go` |
| Per-program history parsing/writing | `HistoryAdapter` interface, `ClaudeAdapter`, `AgyAdapter` | `session/history_adapter.go`, `session/claude_adapter.go`, `session/agy_adapter.go` |
| Cross-program history porting orchestration | `PortSessionHistory()` | `session/history_transfer.go` |

`go.mod` has no tmux client library (no `gotmux`, no `wtmux`, etc.) — tmux control is 100% hand-rolled `os/exec`-based wrapper in `session/tmux/`. gopsutil (`github.com/shirou/gopsutil/v4`) is the only process-inspection dependency in use.

---

## 1. Existing OSS library or framework

### Process adoption / safe cross-platform kill
- **No dedicated Go library for "process adoption"** exists in the OSS ecosystem in a form better than stdlib `os`/`syscall` + gopsutil. gopsutil already provides `Process.IsRunning()`, `Process.Kill()`, `Process.SendSignal()` cross-platform, but the repo does not use it for kill — it uses `os/exec` + `syscall.Kill` directly (see `native_process_manager.go`) and shells out to `tmux kill-session` for tmux-managed processes. This split (native vs tmux) is already the correct axis for import; no additional library changes that.
  - **Verdict: Not recommended to add a new library.** Reuse `syscall.Kill(-pid, SIGTERM)` pattern (already tested, already handles process groups) for non-tmux adoption, and `tmux kill-session`/`kill-pane` for tmux adoption — exactly mirroring `KillExternalSession`.

### tmux control library
- Repo already owns a **2300+ line custom tmux wrapper** (`session/tmux/`) covering session creation, attach, resize, pane capture, socket resolution, exec-slot throttling, batch queries. This is far more capable than generic Go tmux bindings (e.g. `github.com/GianlucaGuarini/go-tmux`, unmaintained) and is deeply integrated with the rest of the process-manager abstraction (`ProcessManager`, `TmuxBackend`).
  - **Verdict: Recommended — extend `session/tmux/`, do not introduce a 3rd-party tmux library.** Any new pane/session enumeration needed for "raw tmux" import (as opposed to ssq-mux-wrapped) should be added as new functions in `session/pty_discovery.go` / `session/tmux/tmux.go` following the existing `batchPTYInfo`-style single-call batching pattern.

### JSONL streaming/parsing for Claude history
- `ClaudeAdapter.Import`/`Export` and `HistoryFileDetector` already parse and locate Claude JSONL conversation files. No external JSONL library is used (nor needed) — stdlib `encoding/json` + `bufio.Scanner` line-splitting is sufficient given JSONL's simplicity, and adopting a generic streaming-JSON library (e.g. `jsoniter`, `gojay`) would add a dependency for no measurable benefit at this file size/frequency.
  - **Verdict: Not recommended to add a library. Recommended to extend `ClaudeAdapter`/`AgyAdapter`/`HistoryFileDetector` directly.**

### SaaS/managed API
- Not applicable. This is a purely local OS-process/file operation (no network service can observe another user's local tmux panes or PTYs). The only plausible future SaaS angle — remote session sync/backup of ported conversation history — is out of scope for this feature and not worth pulling in a managed API now.
  - **Verdict: Not applicable / Not recommended for this feature.**

---

## 2. LLM-generated implementation vs. battle-tested library

### (a) Process-kill logic (cross-platform, tmux-aware)
The repo already has two orthogonal, tested kill paths that cover the exact fork points import-external-session needs:
1. **Native process (plain terminal, not inside tmux):** `NativeProcessManager.Close()` sends `SIGTERM` to the negative PID (process group) via `syscall.Kill`, then closes the PTY. This is the correct, already-vetted signal-handling pattern (`NM-3` invariant tested by `native_process_manager` tests).
2. **tmux-hosted process (raw tmux pane or ssq-mux terminal):** `Instance.KillExternalSession()` shells out to `tmux kill-session -t <name>`.

Import's "confirm-before-kill" flow should **call these existing methods directly** rather than write new signal-handling code. The only genuinely new logic is:
- Detecting *which* kill path applies to an adopted external process (was it found via tmux pane inspection, or via bare PID/PTY inspection with no tmux ancestor?) — this is new decision logic, not new kill mechanics.
- A confirmation gate (UI/RPC round-trip before invoking kill) — new, but trivial (no library needed).

**Verdict: Recommended — reuse `NativeProcessManager.Close()` / `Instance.KillExternalSession()` verbatim; do not reimplement signal handling.**

### (b) New JSONL correlation/matching logic
`HistoryFileDetector` already solves PID→JSONL correlation (`Detect`) and path→most-recent-JSONL correlation (`DetectByPath`), and `HistoryLinker` already solves the "keep correlation live via poll + fsnotify" problem, including exponential backoff for sessions that don't resolve. Import's correlation need — take a discovered external PID/tmux pane and find its Claude conversation file — is the **same problem** `HistoryLinker`/`HistoryFileDetector` already solve for internally-launched sessions.

Any new code should be a thin extension:
- If the external process's cwd/PID is known (from tmux `list-panes` or `ExternalSessionDiscovery`), call `HistoryFileDetector.Detect(pid)` directly — no new fuzzy matching needed.
- If PID inspection fails (e.g. process is a shell wrapper), fall back to `DetectByPath(cwd)` — already implemented for exactly this "dead session" case.

**Verdict: Recommended — extend `HistoryFileDetector`/`HistoryLinker`; do not write new fuzzy-matching from scratch.** The only net-new piece is a "one-shot" correlation entry point (call once during import, not the continuous background loop) — a small wrapper function, not new matching logic.

---

## 3. Fork or adapt — rough breakdown

Estimated composition of "wire existing components together" vs. genuinely net-new code for import-external-session:

**~60% wiring existing components, ~40% net-new.**

### Extend existing (no new abstraction needed)
- `HistoryFileDetector.Detect` / `.DetectByPath` — call directly for PID/path correlation during import (reuse, maybe add a batch variant for multi-import).
- `HistoryLinker` — after import, register the newly-adopted `Instance` via `AddInstance` so ongoing correlation (UUID changes via `/clear`, etc.) keeps working — no new correlation code.
- `ExternalSessionDiscovery` — the ssq-mux-wrapped IDE terminal case is **already fully solved** by this type; "importing" that kind of session is close to just calling `handleNewSession`-equivalent logic plus persisting it as a first-class managed `Instance` instead of a transient external one.
- `PortSessionHistory` / `HistoryAdapter` (`ClaudeAdapter`, `AgyAdapter`) — reuse verbatim for multi-program (Antigravity) support; this is the exact mechanism the feature needs for "port conversation history into a fully-managed Instance," already generalized across programs via `CanHandle`.
- `Instance.KillExternalSession()` / `NativeProcessManager.Close()` — reuse verbatim for the confirm-before-kill terminate step.
- `session/tmux/` wrapper (`batchPTYInfo`, pane/session enumeration, socket resolution) — reuse for raw-tmux (non-mux) discovery; extend with new batched queries following the same pattern rather than a new tmux client.

### Must be built net-new (no existing equivalent)
1. **Raw-tmux / plain-terminal discovery path** — `ExternalSessionDiscovery` only handles ssq-mux-wrapped sessions (requires `mux.Discovery` metadata). Detecting a bare `claude` process running in a plain terminal or an un-instrumented tmux pane (no ssq-mux metadata) requires new enumeration: walk `ps`/gopsutil process tree for `claude`/`agy` processes, cross-reference with `batchPTYInfo`-style tmux pane listing when present, and construct candidate `Instance` shells. This is the single largest net-new chunk.
2. **"Adopt into fully-managed Instance" transition logic** — converting a transient `InstanceType: External` `Instance` (or a freshly-discovered raw process) into a persisted, `processManager`-owned, worktree-tracked `Instance` with normal lifecycle (start/stop/restart) is new: current code treats external instances as inherently ephemeral/observation-only (see `ExternalSessionDiscovery` comments: "Skip sessions without tmux integration"). Adoption needs new state-transition code in `session/` (something like `AdoptExternalInstance(pid, tmuxTarget) (*Instance, error)`).
3. **Confirm-before-kill UX/RPC flow** — new ConnectRPC endpoint + confirmation dialog; the kill mechanics are reused (item 2a above) but the request/response contract, two-phase confirm, and audit/undo consideration are new.
4. **Batch import** — importing N discovered sessions at once is new orchestration (loop + partial-failure handling + progress reporting) layered on top of the single-session adoption path; no existing batch entry point.
5. **Multi-program (Antigravity) discovery matching** — `AgyAdapter` already parses Antigravity's own history format for *export*, but discovering a *running* Antigravity process and locating its conversation file needs a new `HistoryFileDetector`-equivalent for Antigravity's file layout (`~/.gemini/antigravity-cli/history.jsonl` per `history_transfer.go:85`) — analogous to `claudeProjectsPattern`/`ClaudeProjectDirName`, but net-new regex/path logic for the Antigravity layout.

---

## Summary Table

| Sub-component | Build/Buy verdict | Recommended action |
|---|---|---|
| Process→file correlation | Reuse | Extend `HistoryFileDetector`, no new library |
| tmux enumeration/control | Reuse | Extend `session/tmux/`, no 3rd-party tmux lib |
| JSONL parsing | Reuse | Extend `ClaudeAdapter`/`AgyAdapter`, no new JSONL lib |
| Cross-platform kill (native) | Reuse verbatim | Call `NativeProcessManager.Close()` |
| Cross-platform kill (tmux) | Reuse verbatim | Call `Instance.KillExternalSession()` |
| History porting (multi-program) | Reuse | Call `PortSessionHistory`/`HistoryAdapter` |
| Raw-tmux/plain-terminal discovery | Net-new | New enumeration in `session/pty_discovery.go` style |
| Adoption state transition | Net-new | New `AdoptExternalInstance`-style function |
| Confirm-before-kill RPC/UI | Net-new | New proto RPC + React confirm dialog |
| Batch import orchestration | Net-new | New orchestration layer over single-import path |
| Antigravity running-process discovery | Net-new | New detector analogous to `HistoryFileDetector` for `.gemini/antigravity-cli` layout |
| SaaS/managed API | Not applicable | None — purely local OS operation |
