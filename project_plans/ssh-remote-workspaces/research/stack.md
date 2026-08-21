# Research: Stack — SSH Remote Workspaces

Scope: which Go libraries solve SSH connectivity, remote git worktree creation,
cross-platform credential storage, remote PTY streaming, and reverse tunneling
for the approval-callback path. Cross-referenced against `go.mod` and the
existing `session/git/`, `session/tmux/`, `github/keychain.go` packages.

## What's already in go.mod (verified via `go list -m all` / `go mod why`)

| Package | Version | Status in go.mod | Currently pulled in by |
|---|---|---|---|
| `golang.org/x/crypto` | v0.51.0 | **indirect** | Two paths: (1) `server/services` → `github.com/SherClockHolmes/webpush-go` → `golang.org/x/crypto/hkdf`; (2) `session/git` → `go-git/v5` → `plumbing/transport/ssh` → `golang.org/x/crypto/ssh` (confirmed by reading `auth_method.go`/`common.go` in the vendored module — go-git's SSH transport for `git clone`/`fetch` over `ssh://` already uses `golang.org/x/crypto/ssh` under the hood) |
| `github.com/zalando/go-keyring` | v0.2.8 | **direct** | Already used in production: `github/keychain.go` wraps it for GitHub PAT storage (macOS Keychain / Secret Service / Windows Credential Manager), with a package-level `sync.Mutex` (`keychainMu`) serializing all Get/Set/Delete because the OS backends aren't guaranteed thread-safe |
| `github.com/creack/pty` | v1.1.24 | **direct** | `session/tmux/pty.go` — local PTY allocation only (`pty.Start`/`pty.StartWithSize`); has no SSH/remote capability, not reusable for remote PTYs |
| `github.com/gliderlabs/ssh` | v0.3.8 | **indirect (test-only)** | Pulled in solely as `go-git/v5/plumbing/transport/ssh.test`'s dependency for testing go-git's SSH transport. **Not usable as an SSH client** — it's a server-side SSH framework (`gliderlabs/ssh` implements an SSH *server*, the inverse of what this feature needs) |
| `github.com/gorilla/websocket` | v1.5.3 | **direct** | Existing local terminal streaming path (`server/services/cdp_stream_handler.go`, `connectrpc_websocket_test.go`) — the delta protocol already flows over a `gorilla/websocket` connection wrapped by ConnectRPC's WebSocket bridge (`connectWebSocketStream` in `server/services/connectrpc_websocket_test.go`) |
| `pkg/sftp` | — | **absent** | Not in the module graph at all; needed only if SFTP-based file transfer is chosen over shelling out `git`/`rsync` remotely |

**Key finding: `golang.org/x/crypto/ssh` is already transitively present and version-pinned (v0.51.0).** Promoting it from indirect to direct (just import it somewhere in `session/`) costs nothing new in the dependency graph — no new module needs to be added to get a full SSH client.

---

## 1. SSH client connections + remote command execution / PTY allocation

**Recommendation: `golang.org/x/crypto/ssh`, already in go.mod at v0.51.0 (promote indirect → direct).**

- This is the standard-adjacent (golang.org/x/...) low-level SSH client/server implementation; it is what every higher-level Go SSH wrapper (including `gliderlabs/ssh` and go-git's own transport) is built on. Actively maintained as part of the Go project itself.
- Core flow for this feature:
  1. `ssh.Dial("tcp", host, config)` → `*ssh.Client`
  2. `client.NewSession()` → `*ssh.Session`
  3. `session.RequestPty("xterm-256color", rows, cols, modes)` to allocate a remote PTY
  4. `session.StdinPipe()` / `session.StdoutPipe()` (or `session.Stdout = ...`) to wire the remote PTY's I/O to the same delta-protocol writer that `session/tmux/pty.go`'s local `*os.File` currently feeds
  5. `session.Shell()` or `session.Start("tmux new-session -A -s <name>")` to attach/create the remote tmux session
- **Higher-level wrapper consideration**: `github.com/melbahja/goph` and `github.com/appleboy/easyssh-proxy` are commonly cited convenience wrappers over `x/crypto/ssh`, but neither is in go.mod, neither adds capability beyond what `x/crypto/ssh` exposes directly (they mostly simplify one-shot command execution, not the long-lived interactive-PTY + resize + reconnect flow this feature needs), and adding one would be a second SSH abstraction alongside the one go-git's transport already uses internally. **Recommend using `x/crypto/ssh` directly** — it slots into `session/tmux/` the same way `creack/pty` does today: a small `RemotePtyFactory` type implementing an interface analogous to `tmux.PtyFactory` (`session/tmux/pty.go:10-17`), returning something that exposes `io.ReadWriter` + resize, so the rest of `session/tmux/tmux.go`'s control-mode/session logic doesn't need to know whether the PTY is local or remote.
- Auth methods needed: `ssh.PublicKeys()` from a parsed private key (`ssh.ParsePrivateKey` / `ssh.ParsePrivateKeyWithPassphrase` for encrypted keys) — the key bytes themselves should come from the keychain-backed credential store (§3), not be read as plaintext from `~/.ssh/id_ed25519` directly, per the "no SSH keys/passphrases in plaintext config" constraint. `ssh.FixedHostKey()` or `knownhosts.New()` (from `golang.org/x/crypto/ssh/knownhosts`, same module) for host-key verification — do not use `ssh.InsecureIgnoreHostKey()` in production paths.
- Reconnection: `x/crypto/ssh` has no built-in reconnect/keepalive; the "network partition tolerance" non-functional requirement means the *remote* side (tmux session + agent process) must be designed to survive the SSH `*ssh.Client` connection dying, and the local side needs its own retry loop that re-dials and re-attaches (`tmux attach-session` / `new-session -A`) — this is application logic, not something a library provides.

## 2. Remote git worktree creation over SSH

**Answer: go-git cannot operate against a remote filesystem for worktree creation — this requires shelling out over the SSH session.**

- go-git's `go-git-billy` filesystem abstraction (`session/git/worktree.go` etc. currently use the local OS filesystem via `go-billy/v5`) has no SFTP or SSH-backed `billy.Filesystem` implementation in the go-git ecosystem for *worktree* operations (working tree files, not just the `.git` object store). go-git's SSH support (`plumbing/transport/ssh`, the package pulling in `golang.org/x/crypto/ssh` per the go.mod audit above) is **only** for the git *wire protocol* (clone/fetch/push over `ssh://` or `git@host:repo`) — it lets go-git talk to a *remote git server*, not create a working tree *on* a remote machine.
- Since the requirement is "create a git worktree on the remote filesystem" (i.e., the repo, worktree, and agent process all live on the remote host — not a local clone that pushes to a remote), the actual operations (`git worktree add`, `git clone`, branch checkout) must run **on the remote host**, which means executing `git` CLI commands over the SSH session from §1 (`session.Run("git worktree add ...")` / `session.CombinedOutput(...)`).
- **This is the documented exception to `.claude/rules/prefer-go-git-over-subshells.md`** — that rule already carves out "any operation needing a credential helper for push/fetch against a real remote" and "go-git genuinely can't do the job" as legitimate reasons to shell out; running `git` on a machine go-git has no filesystem access to is squarely that case, not a violation of the convention.
- Design implication for `session/git/`: the existing `worktree.go`/`ops.go` functions take a local `repoPath string` and operate via go-git + go-billy. A remote-workspaces feature needs a **parallel code path** (not a drop-in replacement) — e.g. a `RemoteWorktreeOps` type in a new `session/git/remote.go` (or a new `session/sshremote/` package) that takes an `*ssh.Client` (or a thin `RemoteExecutor` interface: `Run(cmd string) (stdout, stderr []byte, err error)`) and shells out `git` commands remotely, mirroring the shape of `ops.go`'s functions but backed by SSH exec instead of go-git. Keep the two paths behind a common interface consumed by `session/instance.go` so session lifecycle code doesn't fork on local-vs-remote at every call site.

## 3. OS keychain / cross-platform credential storage

**Recommendation: `github.com/zalando/go-keyring` v0.2.8 — already a direct dependency, already proven in production for exactly this class of secret (`github/keychain.go`).**

- Backends: macOS Keychain, Linux Secret Service (D-Bus, i.e. libsecret-compatible — gnome-keyring/kwallet), Windows Credential Manager. This matches all three platforms named in the requirement.
- Maintenance: zalando/go-keyring is a small, stable, widely-used library (no recent breaking churn); v0.2.8 is what's already pinned.
- **Reuse the existing pattern directly** rather than inventing a new one:
  - `github/keychain.go`'s `keyringGet`/`keyringSet`/`keyringDelete` wrappers around a package-level `keychainMu sync.Mutex` exist specifically because go-keyring's OS backends aren't guaranteed thread-safe under concurrent access. A new `config` or `session/sshremote` package storing SSH private keys/passphrases should either (a) reuse `github.keychainService`-style scoping with a distinct service name (e.g. `"stapler-squad-ssh"`) and its own mutex, following the identical wrapper pattern, or (b) generalize `github/keychain.go`'s wrappers into a small shared internal package if both GitHub tokens and SSH keys end up needing the same serialization guarantee — worth a design-phase decision, not a stack question.
- What to store: given the proposed config shape (`"ssh_key": "~/.ssh/id_ed25519"`), the *path* to a key can stay in `config.json` (it's not a secret), but if the key is passphrase-protected, the **passphrase** must go in the keychain (never plaintext config), keyed by something like `service="stapler-squad-ssh", key="passphrase:<remote-name>"`. If instead the feature generates/stores its own dedicated keypair per remote (an alternative worth considering at design time, avoiding any dependency on the user's personal `~/.ssh` key), the **private key material itself** goes in the keychain as the secret value, using the same account-scoped key pattern (`accountKey`-equivalent) already established for multi-account GitHub tokens.

## 4. Streaming a remote PTY's output over the existing ConnectRPC/WebSocket channel

**No new library needed — this is an architectural seam, not a missing dependency.**

- The existing local terminal-streaming path (confirmed via `server/services/cdp_stream_handler.go` and `server/services/connectrpc_websocket_test.go`) already flows through `gorilla/websocket` (v1.5.3, direct dep) wrapped by a ConnectRPC bridge (`connectWebSocketStream`), with `StreamTerminal` routed specially ahead of the general ConnectRPC handler (see `server/services/stream_terminal_routing_test.go`).
- Today's source of bytes is the local PTY (`*os.File` returned by `creack/pty`, `session/tmux/pty.go`). For a remote session, the source becomes `session.StdoutPipe()` (an `io.Reader`) from the `*ssh.Client` session in §1. Since both are just `io.Reader`s, the delta-protocol encoder that currently reads from the local PTY file should be refactored to accept an `io.Reader` interface rather than being hardcoded to `*os.File` — at which point plugging in the SSH session's stdout is a substitution, not new plumbing. Same applies to writes (`io.Writer` for keystrokes → local PTY write vs. `session.StdinPipe()` write) and resize (local `pty.Setsize` vs. SSH `session.WindowChange(rows, cols)`, both of which `x/crypto/ssh` supports natively via `Session.WindowChange`).
- No additional Go package is required for this piece; it's a refactor of `session/tmux/pty.go`'s `PtyFactory` interface (and whatever consumes it in `tmux.go`/the streaming handler) to abstract over "local PTY file" vs. "SSH session stdio," not a new dependency.

## 5. SSH reverse tunneling / port-forwarding for the approval-callback path

**Recommendation: `golang.org/x/crypto/ssh` again — it has full support for both directions of TCP forwarding natively, no separate library needed.**

- `x/crypto/ssh` supports SSH port forwarding via the same `*ssh.Client`:
  - **Local→remote forward** (`ssh -L`): `client.Dial("tcp", remoteAddr)` — dial a TCP connection *through* the SSH client, useful if the local dashboard needs to reach a remote-only service.
  - **Remote→local forward / reverse tunnel** (`ssh -R`, what the approval-callback path needs — remote agent calls back to local dashboard): `client.Listen("tcp", remoteBindAddr)` — this asks the *remote* sshd to open a listener and tunnel accepted connections back through the existing SSH connection to the local process. This is the standard Go pattern for "reverse tunnel without a separate library" and is exactly what tools like `chisel`/`ngrok`-style SSH tunneling wrap.
  - Requires the remote sshd to have `GatewayPorts`/`AllowTcpForwarding` enabled (a remote-host sshd_config concern, not a Go library concern) — worth flagging as a deployment prerequisite in the plan phase, since some hardened sshd configs disable remote forwarding by default.
- **Alternative worth naming for the design phase**: instead of a raw reverse TCP tunnel, the approval callback could be a **remote→local HTTP POST wrapped in the reverse tunnel** (`client.Listen` handed to a local `net/http.Serve`), which is more robust than a bespoke TCP protocol and reuses the existing ConnectRPC approval-request shape — this is a design decision, not a library gap, since both directions are just `net.Listener`/`net.Conn` once `client.Listen` is called.
- No `pkg/sftp` or third-party tunneling library needed; adding one (e.g. `github.com/hashicorp/yamux` for multiplexing multiple logical streams over one tunnel) would only be justified if a single reverse TCP forward per remote turns out to be insufficient for concurrent approval requests across 10-20 sessions on one host — worth revisiting in the plan phase if that need materializes, not upfront.

---

## Summary table

| Concern | Library | In go.mod? | Action needed |
|---|---|---|---|
| SSH client / remote exec / PTY | `golang.org/x/crypto/ssh` | Yes, indirect v0.51.0 | Promote to direct (import it) |
| Host key verification | `golang.org/x/crypto/ssh/knownhosts` | Same module, already available | Use `knownhosts.New()`, avoid `InsecureIgnoreHostKey` |
| Remote git worktree ops | none (shell `git` over the SSH session) | N/A | New `session/git/remote.go`-style package using SSH exec, mirroring `ops.go`'s shape; documented exception to the go-git-over-subshells rule |
| OS keychain | `github.com/zalando/go-keyring` | Yes, direct v0.2.8 | Reuse `github/keychain.go`'s mutex-wrapped Get/Set/Delete pattern under a new service scope |
| Remote PTY → ConnectRPC/WebSocket streaming | none (refactor to `io.Reader`/`io.Writer`) | N/A (uses existing `gorilla/websocket` v1.5.3) | Generalize `session/tmux/pty.go`'s `PtyFactory` interface to not assume `*os.File` |
| Reverse tunnel for approval callback | `golang.org/x/crypto/ssh` (`Client.Listen`) | Same as above | No new dependency; deployment prerequisite: remote sshd must allow `AllowTcpForwarding`/`GatewayPorts` |

**Net new go.mod dependencies required: zero.** Every capability needed is either already a direct dependency (`go-keyring`, `gorilla/websocket`, `creack/pty` as the local-side precedent) or already present transitively at a pinned version (`golang.org/x/crypto` → promote to direct for `ssh`/`ssh/knownhosts`). The only "missing" piece — remote git worktree creation — isn't a missing library, it's an architectural requirement to shell out over SSH, which is already the documented, sanctioned exception pattern in this repo's own git conventions.
