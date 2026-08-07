# Architecture Research: ssh-remote-workspaces

## Prior-research check

- `project_plans/instance-actor-concurrency/research/architecture.md` — relevant tangentially (Instance concurrency/locking invariants), no SSH/remote content. Not cited further; this feature doesn't change Instance's locking model.
- `project_plans/agent-protocol-architecture/research/architecture.md` — grepped for "remote"/"SSH", zero hits. Not relevant.

No prior work targets remote execution. This is fresh ground.

## EventStorming table

Skipped per the requirements' own guidance. This is infrastructure-shaped (where does a command run, how does a byte stream get from host A to host B), not a business-rule surface. The one candidate business rule — "approval requests from a remote agent must route back to the local dashboard" — is already fully handled by the existing `InjectHookConfig` / HTTP-hook-callback mechanism (see `server/services/session_service.go:1595-1599`, `InjectHookConfig(instanceRootDir, instanceTitle)`), which posts to the Stapler Squad server's own HTTP endpoint over the network today. A remote session only needs that URL to be reachable from the remote host (already true if the server binds `0.0.0.0` per `Config.ListenAddress`, `config/config.go:234`) — no new actor, no new business rule, just a reachability requirement. Covered in §4 below, not modeled as EventStorming.

---

## (1) Where the local/remote seam lives

### Precedent already in the codebase: `SessionStreamer`

`server/services/session_streamer.go` is the canonical example of exactly the pattern requested — and it's already doing this repo's `interface-pollution-checklist` correctly:

```go
// server/services/session_streamer.go
package services

// SessionStreamer is the interface the WebSocket streaming handler requires from a session.
// Defined in the consumer package (server/services) to prevent import cycles and to keep
// the interface minimal — only what this package legitimately needs for terminal streaming.
//
// *session.Instance satisfies this interface via delegation methods.
type SessionStreamer interface {
	StartControlMode() error
	StopControlMode() error
	SubscribeControlModeUpdates() (string, <-chan []byte)
	UnsubscribeControlModeUpdates(id string)
}
```

Four methods, defined in the consuming package (`server/services`), satisfied implicitly by `*session.Instance` via delegation methods in `session/instance_tmux.go:540-556`. This is the template to replicate, not a new pattern to invent.

### Where the actual local-vs-remote fork needs to happen

Tracing what `session.Instance` currently does that is inherently local-filesystem/local-process:

- **tmux invocation**: `session/tmux/tmux.go` shells out via `safeexec.CommandContext(ctx, Binary(), args...)` (e.g. lines 298, 328, 509, 533, 555, 612, 631, 642, 1902, 2295, 2318) — every one of these assumes the `tmux` binary and its server socket (`session/tmux/tmux.go`'s `Socket` type, `ResolveSocket`/`prependSocket`, lines ~346-415) are on the local machine. The `Socket` abstraction already exists for *local* multi-server isolation (e2e test isolation, instance-scoped state — see `.claude/docs/state-isolation.md`), but it has no concept of "which host." That's the gap.
- **git worktree creation/management**: `session/git/worktree_git.go` — `GitWorktree` methods (`PushChanges`, `CommitChanges`, `CreatePR`, etc.) call `runGitCommand`/`runExec`/`runCombinedOutput` (lines 27, 301, 309) which wrap `exec.Cmd` directly, and separately the repo's own `.claude/rules/prefer-go-git-over-subshells.md` establishes that read/local-metadata git operations increasingly go through `go-git` (`github.com/go-git/go-git/v5`) opening a **local filesystem path**. Both paths — shelled `git` and go-git's `PlainOpen` — assume the repo lives on the same disk as the Stapler Squad process.
- **PTY/process management**: `session/instance_tmux.go` (`GetPTYReader`, `WriteToPTY`, `ResizePTY`, `CapturePaneContent`) and `session/tmux/pty.go` assume a local PTY file descriptor.

### Recommended shape

Do **not** build one large `SessionExecutor`/`WorkspaceProvider` god-interface with `Create`/`Attach`/`Stream`/`Kill`. That would violate the interface-pollution checklist's smell #1 (speculative, over-broad) and smell #6 (struct-wraps-struct). Instead, follow the `SessionStreamer` precedent and cut the seam at each of the three points above independently, each interface minimal and defined where it's consumed:

1. **Command execution seam** (lowest level, highest leverage): `session/tmux` already routes every tmux invocation through `safeexec.CommandContext`. Introduce a `CommandRunner` interface *inside `session/tmux`* (it's the consumer of the current subprocess calls):
   ```go
   // session/tmux — consumer package
   type CommandRunner interface {
       Run(ctx context.Context, name string, args ...string) ([]byte, error)
   }
   ```
   `LocalRunner` wraps today's `safeexec.CommandContext(...).CombinedOutput()`. `SSHRunner` wraps `golang.org/x/crypto/ssh` (`Session.CombinedOutput` on a persistent `*ssh.Client`). Every `safeexec.CommandContext(ctx, Binary(), args...)` call site in `tmux.go` becomes `runner.Run(ctx, Binary(), args...)`. This is the single highest-leverage seam: it makes *all* of tmux's remote-vs-local behavior fall out of which `CommandRunner` a `TmuxSession` was constructed with — no duplication of the 20+ tmux command call sites.

2. **Worktree-creation seam**: `session/git` needs an equivalent for the handful of *mutating* worktree-setup operations that must run on the target host (`git worktree add`, `git init`, initial commit) — as opposed to the read-mostly go-git operations (`IsDirty`, `IsCommitOnMain`) which only need `go-git` if the repo is local. For a remote worktree, go-git's local `PlainOpen` cannot reach it at all; those operations must also route through the same `CommandRunner`-shaped seam, executing `git` on the remote host. Practically this means `GitWorktree`'s `runGitCommand`/`runExec` (`session/git/worktree_git.go:27,301`) take the same `CommandRunner` used by the tmux layer, rather than calling `exec.Cmd` directly. **This is the one place the `prefer-go-git-over-subshells` rule's normal preference inverts**: go-git's value (typed results, no subprocess) only applies when the repo is on local disk; for a remote target, shelling `git` through the `CommandRunner` — local or SSH — is the *only* option, so the rule's own "when a subshell is still fine" carve-out applies squarely.

3. **PTY/stream seam**: this is where "reverse tunnel vs. remote agent process" actually gets decided (see §3) — it is not a generic `Attach()`/`Stream()` interface but a specific transport decision for tmux control-mode's byte stream.

Where this plugs into `session/instance.go`: `Instance` already holds a `*tmux.TmuxSession` (`GetTmuxSession()`, `instance.go:527`) and a `GitWorktree` reference. Neither of those types needs a new "remote" concept baked into `Instance` itself — `Instance` stays host-agnostic. The host selection happens at *construction* time: `NewTmuxSessionWithServerSocket`/`NewInstance` receive a `CommandRunner` (local by default, SSH when the create request specifies a remote target), and everything downstream (`Start`, `Kill`, `CapturePaneContent`, `ResizePTY`) is unchanged because it already goes through the `TmuxSession`/`GitWorktree` methods that would now be parameterized by that runner. This keeps the "seam" at the bottom of the stack (subprocess execution) rather than top (session lifecycle), which is why `Instance`'s ~30 public methods don't all need remote-aware variants — only the handful of `session/tmux` and `session/git` internals that currently call `exec.Cmd` directly do.

**Naming**: avoid `WorkspaceProvider` (too abstract/speculative per smell #1 — there's no second "provider" shape competing for that name) and avoid `SessionExecutor` (collides conceptually with the *existing* `session.CommandExecutor` in `session/command_executor.go`, which is an unrelated PTY-command-queue construct — reusing the name would be confusing). `CommandRunner` in `session/tmux` (and reused by `session/git`) is concrete, scoped, and named for what it actually does.

---

## (2) Integration point: orthogonal field, not a parallel enum

Traced the actual switch statement — `resolveSessionType` in `server/services/session_service.go:1651-1675`:

```go
func resolveSessionType(msg *sessionv1.CreateSessionRequest, branch string) session.SessionType {
	if msg.SessionType != sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		switch msg.SessionType {
		case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
			return session.SessionTypeDirectory
		case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
			return session.SessionTypeNewWorktree
		case sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE:
			return session.SessionTypeExistingWorktree
		case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
			return session.SessionTypeNewProject
		case sessionv1.SessionType_SESSION_TYPE_ONE_OFF:
			return session.SessionTypeOneOff
		default:
			return session.SessionTypeDirectory
		}
	}
	...
}
```

and the enum it switches on, `proto/session/v1/types.proto:354-366`:

```protobuf
enum SessionType {
  SESSION_TYPE_UNSPECIFIED = 0;
  SESSION_TYPE_DIRECTORY = 1;
  SESSION_TYPE_NEW_WORKTREE = 2;
  SESSION_TYPE_EXISTING_WORKTREE = 3;
  SESSION_TYPE_NEW_PROJECT = 4;
  SESSION_TYPE_ONE_OFF = 5;
}
```

**Verdict: orthogonal field, following the `autonomous_mode` precedent exactly** — not a new `SessionType` value, and not a parallel enum.

Reasoning, grounded in what `SessionType` actually encodes vs. what "remote" means:
- Every existing `SessionType` value answers "what does the *worktree/directory setup* look like" (new worktree vs. existing vs. plain directory vs. new project vs. temp one-off). "Remote" answers an orthogonal question: "on which host does that setup run." All five existing modes are meaningful on a remote host too (you can create a new worktree remotely, attach to an existing remote worktree, run a one-off remotely, etc.) — a combinatorial product, not a sibling value. Adding `SESSION_TYPE_REMOTE_*` variants for each would 2x (or 5x) the enum and the `resolveSessionType` switch for no semantic gain, and would immediately violate the session-creation-registry's own intent (`.claude/rules/session-creation-registry.md`'s "`autonomous` exception" is the documented precedent for exactly this situation: "Use this pattern when the backend session type is shared but behavior is driven by additional request parameters").
- `CreateSessionRequest` (`proto/session/v1/session.proto:472` onward) already has a directly analogous field for this: `bool autonomous_mode = 23;` (line 557) — a flag that layers onto whatever `session_type` was chosen, handled by `Instance.AutonomousMode bool` (`session/instance.go:166,531`) rather than a session-type fork. The next available field number in `CreateSessionRequest` is past 23 (need to check current HEAD count via `grep 'message CreateSessionRequest' -A200`, but the pattern is: add `RemoteTarget` as a new field, e.g. `optional string remote_host = <next>;` or a small nested message `optional RemoteTarget remote = <next>;` if more than a hostname is needed (user, key ref, base path)).

Concretely, the 7-touchpoint registry (`.claude/rules/session-creation-registry.md`) is satisfied like this:
1. **Proto enum** — no change.
2. **Proto request message** — add `optional RemoteTarget remote = <next field number>;` (nested message: `host`, `ssh_user`, `identity_ref` — a reference into wherever SSH keys are managed, not raw key material over the wire) to `CreateSessionRequest`, mirroring how `autonomous_mode` was added as a flag.
3. **Go handler** (`server/services/session_service.go`) — `resolveSessionType` is untouched; a new resolution step alongside it (`resolveRemoteTarget(msg) *session.RemoteTarget`) plugs into the same mode-specific-logic block pattern as the existing `if req.Msg.OneOff { ... }` block (line ~1625's `AutonomousMode` handling is the closer analog: `if instance.AutonomousMode && s.headlessPool != nil { ... }`).
4. **`session/instance.go` `SessionType` constants** — no new constant (mirrors the "autonomous exception": lifecycle doesn't structurally differ, only where it executes).
5. **`Omnibar.tsx`** — `sessionType` union unchanged; add a sibling `remoteHost?: string` (or `remote?: {...}`) field to `OmnibarFormState`, analogous to how `autonomous_mode` isn't in the `sessionType` union either.
6. **`OmnibarCreationPanel.tsx`** — no new `SESSION_TYPES` entry; add a remote-target selector (dropdown of configured remotes, see §4) that composes with whichever `SESSION_TYPES` value is chosen, the same way the existing "Fix Autonomously (Beta)" entry is *itself* `SessionTypeDirectory` + a flag rather than a new radio value — remote should follow the flag pattern, not the radio-value pattern, since it composes with every mode rather than replacing one.
7. **`OmnibarContext.tsx` + `useSessionService.ts`** — thread `remote` through `createSession` call body exactly as `oneOff`/`autonomousMode` are threaded today.

---

## (3) Data flow: control-mode streaming today, and what changes remotely

Traced the streaming path in `server/services/connectrpc_websocket.go`. Key facts:

- The **primary** path (managed sessions, `STAPLER_SQUAD_USE_CONTROL_MODE` unset or `true`, `connectrpc_websocket.go:522-526`) is `streamViaControlMode` (line 554 onward): it starts tmux's native control mode (`tmux -C`, via `instance.StartControlMode()`, delegating to `session/tmux`'s control-mode process — see `session/tmux/control_mode.go`), subscribes to structured `%output`/`%session-changed` notifications over that process's stdout, and forwards them to the ConnectRPC `StreamTerminal` bidi stream (`proto/session/v1/session.proto:33`, `rpc StreamTerminal(stream TerminalData) returns (stream TerminalData) {}`).
- The **fallback** path (`streamViaTmuxCapturePane`, referenced at line 539) is capture-pane polling with delta computation — the comment at `connectrpc_websocket.go:534-537` is explicit: "It polls tmux's internal pane buffer at regular intervals... it detects content changes and only sends deltas." This is the actual "delta protocol" the requirements doc refers to — it's a polling+diff scheme, not a named wire protocol, and it's the fallback, not the primary path.
- Both paths ultimately read from a **local OS pipe/process** (`tmux -C` control-mode stdout, or `tmux capture-pane` subprocess output) that only exists because `session/tmux` shelled out to a local `tmux` binary.

### What changes when tmux itself is remote

The requirements pose two options: SSH channel-forwarding the existing control-mode byte stream vs. a lightweight remote agent process. Given the architecture above (the `CommandRunner` seam), the two aren't actually competing designs — they're the same design at different points on one dial:

- **`tmux -C` over an SSH exec channel** is the direct extension of the `CommandRunner` seam from §1: instead of `safeexec.CommandContext(ctx, "tmux", "-C", "attach-session", ...)` running a local subprocess, `SSHRunner` opens an `ssh.Client.NewSession()` and runs the identical `tmux -C ...` command remotely, wiring `Session.Stdout`/`Session.Stdin` in place of the local `exec.Cmd`'s pipes. **This requires zero changes to `streamViaControlMode`, the control-mode parser (`session/tmux/control_mode.go`), or the `TerminalData` proto** — the control-mode protocol is already a text stream over a pipe; SSH just relocates where the far end of that pipe lives. This is the minimal-surface-area option and should be the default design.
- **A remote agent process** (a second small binary/daemon running on the remote host that itself manages tmux locally there and talks back over its own RPC channel) is strictly more machinery: a second deployable artifact, its own versioning/compatibility story against the central server, and its own auth. It would only earn its cost if the raw SSH-exec approach hits a concrete wall — e.g. sustained high-frequency control-mode traffic proving too latency-sensitive over a plain SSH channel (unlikely; SSH channels are already TCP-multiplexed and control-mode's structured-line protocol is low-bandwidth), or a requirement for the remote session to keep functioning through extended local-server outages with local queuing (a real reliability requirement — see below). Recommend: build the direct SSH-exec-forwarded `tmux -C` path first; treat the remote-agent design as a fallback to revisit only if a concrete latency or partition-tolerance gap shows up in practice, not speculatively up front (this is the same "no unjustified generic / no speculative interface" discipline as §1, applied to transport choice).
- **Reliability corollary**: "remote tmux session survives local disconnect" is already true for *local* sessions today by construction — tmux's server process outlives any single attach; `session/tmux/tmux.go`'s `RestoreWithWorkDir`/reattach logic (referenced at `connectrpc_websocket.go:606-619`) is exactly the reattach-and-recover-scrollback path already exercised whenever the WebSocket drops and reconnects. Over SSH this is unchanged in kind, only the failure surface grows: an SSH channel drop looks identical to a local reconnect from `streamViaControlMode`'s point of view (it just gets an error/EOF from `SubscribeControlModeUpdates()`'s channel and re-runs the same restore path against a **new** SSH session to the same remote tmux server) — provided the `SSHRunner` itself does connection-liveness detection and exposes reconnect the same way. This is a strong reason to keep tmux's own server-side durability doing the heavy lifting rather than inventing a new remote-specific reliability mechanism.

---

## (4) `config.json` remotes list + connection-health without polling-on-render

Confirmed via grep: **no `remotes`/`Remote` concept exists anywhere in `config/*.go` or `session/*.go` today** — this is wholly new config surface, not an extension of an existing list.

### Config shape

Add to `config.Config` (`config/config.go:229`, alongside the existing flat list-of-named-things pattern already used for `SessionDefaults`/`Notifications`, lines 284-287):
```go
// Remotes holds named SSH targets available for remote session creation.
Remotes []RemoteConfig `json:"remotes,omitempty"`
```
```go
type RemoteConfig struct {
    Name        string `json:"name"`
    Host        string `json:"host"`
    User        string `json:"user"`
    IdentityRef string `json:"identity_ref"` // reference into 1Password/ssh-agent, never a raw key path or key material in config.json
}
```
Static config (what remotes exist) is separate from **live health** (is this remote currently reachable) — the latter must not live in `config.json` at all, since that file is disk-persisted user config, not runtime state, and writing to it on every health-check tick would thrash disk I/O and fight the existing config-save/reload machinery.

### Live health: reuse the event-bus pattern, don't poll

The frontend already has exactly the pattern needed, proven out for session connection status: `ConnectionIndicator.tsx` (`web-app/src/components/layout/ConnectionIndicator.tsx`) reads `selectConnectionState` from a Redux slice (`sessionsSlice`) that's driven by push updates over the existing `watchSessions` ConnectRPC stream — not polling. `STATE_LABEL`/`STATE_ANNOUNCE` maps `connected`/`stale`/`disconnected` to UI state, and `reconnectAttemptCount` comes from `useSessionServiceContext()`.

The backend equivalent already exists too: `pkg/events` (`EventBus`, `NewSessionUpdatedEvent`, etc., `pkg/events/types.go`) is the pub/sub bus that `server/services/session_service.go` already publishes to (`s.eventBus.Publish(events.NewSessionCreatedEvent(instance))`, line 1563) and that session list watchers subscribe to.

Recommended: add a `NewRemoteHealthChangedEvent(remoteName string, status RemoteHealthStatus)` constructor next to the existing `New*Event` constructors in `pkg/events/types.go`, published by whatever component owns SSH liveness (a small background prober per configured `RemoteConfig`, checking the underlying `ssh.Client` connection — SSH's own keepalive/`ClientConn.Wait()` already signals disconnects without a poll loop on the hot path). The frontend adds a `remotesSlice` structurally parallel to `sessionsSlice`, fed by the same `watchSessions`-style stream (or a new lightweight `WatchRemotes` stream if bundling health into the session stream is awkward), and a `RemoteConnectionIndicator` component structurally identical to `ConnectionIndicator.tsx` — same `connected`/`stale`/`disconnected` states, same push-driven `useAppSelector` pattern, zero per-render polling.

---

## Summary of concrete file touchpoints

| Concern | File | Change |
|---|---|---|
| Command execution seam | `session/tmux/tmux.go` | New `CommandRunner` interface + `LocalRunner`/`SSHRunner`; replace direct `safeexec.CommandContext` call sites |
| Worktree mutation seam | `session/git/worktree_git.go` | `runGitCommand`/`runExec` take a `CommandRunner` instead of calling `exec.Cmd` directly |
| Proto request field | `proto/session/v1/session.proto` | New `optional RemoteTarget remote = <next>;` on `CreateSessionRequest`, no enum change |
| Go handler | `server/services/session_service.go` | New `resolveRemoteTarget()` alongside `resolveSessionType()`; mode-specific block alongside the existing `AutonomousMode`/`OneOff` blocks |
| Config | `config/config.go` | New `Remotes []RemoteConfig` field |
| Events | `pkg/events/types.go` | New `NewRemoteHealthChangedEvent` |
| Frontend state | new `remotesSlice` (parallel to `sessionsSlice`) | Push-driven health state |
| Frontend indicator | new `RemoteConnectionIndicator.tsx` (parallel to `ConnectionIndicator.tsx`) | Structurally copy the existing component |
| Omnibar (registry touchpoints 5-7) | `Omnibar.tsx`, `OmnibarCreationPanel.tsx`, `OmnibarContext.tsx`, `useSessionService.ts` | Add `remote` as a flag/field composing with existing `sessionType`, not a new union member |
