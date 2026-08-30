# Build vs Buy: ssh-remote-workspaces

Research for `project_plans/ssh-remote-workspaces/requirements.md`. Covers the five
sub-components called out in the research question: SSH client/PTY, wholesale remote-dev
orchestration, OS keychain, reverse tunnel/approval callback, and delta-protocol reuse.

## 1. SSH client / PTY over Go

**Options considered:** `golang.org/x/crypto/ssh` (stdlib-adjacent, low-level) vs.
`melbahja/goph` (thin wrapper) vs. rolling something heavier.

### `golang.org/x/crypto/ssh` — low-level, direct
- **Pros:** Official Go subrepo, part of the same trust boundary as the Go toolchain itself.
  Already an indirect dependency of this repo (`go.mod:182`, pulled in transitively by
  `go-git/go-git/v5` for its SSH transport — confirmed via `grep golang.org/x/crypto go.mod`).
  Full control over `Session.RequestPty`, `Session.Shell`, channel multiplexing, and — crucially
  for §4 below — the ability to open arbitrary additional channels/streams over one
  already-authenticated `ssh.Client` connection (SSH's channel layer is explicitly
  multiplexed: many logical channels share one encrypted TCP connection per RFC 4254). No
  extra transitive dependency risk since it's already in the tree.
- **Cons:** Verbose — PTY sizing, terminal modes (`ssh.TerminalModes{ECHO: 0, ...}`), and
  session lifecycle are all hand-rolled boilerplate that a wrapper would otherwise hide.
- **Verdict: Recommended.** It's already resident in the dependency graph, it's the substrate
  every wrapper (including goph) is built on, and this feature needs exactly the low-level
  channel control that wrappers abstract away (see §4 — reusing one SSH connection for both
  the terminal stream and approval callbacks requires direct channel access, not a
  `Run()`/`Shell()`-shaped convenience API).

### `melbahja/goph` — higher-level wrapper
- **Pros:** MIT licensed. Actively maintained (repo activity within the last month as of this
  research, packaged for Debian at v1.4.0 in late 2025 per the Debian ITP bug tracker).
  Convenience methods for `Run`, `Shell`, SFTP upload/download, and known-hosts handling that
  would save boilerplate for simple exec-style remote commands (e.g. `git worktree add` on the
  remote host).
- **Cons:** Its `Shell()`/`Run()` API is oriented around single command execution or a single
  interactive shell per `Client` — it doesn't expose a first-class way to open a second
  parallel channel on the same connection for an approval callback path, so §4's "reuse the
  existing control channel" design would require dropping to the underlying `x/crypto/ssh`
  client goph wraps anyway, partially negating the abstraction.
- **Verdict: Viable**, but only for the narrow "run a remote command and get output" slice
  (e.g. remote worktree creation via `git worktree add` executed over SSH `exec`). Not a
  replacement for direct `x/crypto/ssh` use in the PTY-streaming and channel-multiplexing
  paths, so adopting it would mean carrying two SSH abstractions (goph for exec, raw
  `x/crypto/ssh` for PTY+multiplexed channels) rather than one. Given that overlap, plain
  `x/crypto/ssh` for everything is simpler to reason about even though it costs a bit more
  boilerplate at the exec call sites.

## 2. Remote dev environment orchestration, wholesale

**Question:** Does an existing OSS project already solve "SSH to a host, create workspace,
attach terminal, stream to browser" well enough to embed/adapt instead of building from
scratch?

### Coder (coder.com, OSS core)
- **Pros:** Directly solves the "provision a workspace on a remote/cloud host, connect an
  IDE or terminal, stream to browser" problem, written in Go, self-hostable, Community
  edition (AGPLv3, confirmed via the repo's own `LICENSE` file) is free and unrestricted in
  workspace/template count. Uses a WireGuard-based tunnel (via Tailscale's `tailscale.com`
  library) for connectivity rather than raw SSH, which would replace this feature's own
  SSH-based transport entirely.
- **Cons — the decisive one:** Coder's whole architecture assumes it *owns* workspace
  lifecycle, defined declaratively via Terraform against Kubernetes/Docker/VM provisioners.
  Stapler-squad already owns an equivalent but structurally different stack — its own
  session model (`session/instance.go`), git worktree orchestration
  (`session/git/worktree.go`, `worktree_ops.go`, `worktree_branch.go`), tmux session
  management (`session/tmux/`), and a ConnectRPC-based terminal delta-streaming protocol
  (`server/services/connectrpc_websocket.go`, `session_service.go` — see §5). Embedding
  Coder would mean either (a) running it as a second orchestration system alongside the
  existing one and bridging session state between the two — the sessions UI, backlog
  integration, and approval-rule engine would all need to understand two different notions
  of "a workspace" — or (b) replacing the existing local tmux+worktree model with Coder's
  Terraform-provisioned workspace model everywhere, which breaks local (non-SSH) session
  creation, the dominant existing use case. AGPLv3 licensing also has copyleft implications
  for embedding vs. running as a separate service that this repo (currently no strong
  copyleft dependencies observed) would need to evaluate.
- **Verdict: Not recommended for embedding.** Worth referencing for architectural ideas
  (workspace lifecycle state machine, agent-side heartbeat/health reporting) but adopting it
  wholesale is *more* work than the from-scratch approach implied by the requirements, because
  it requires bridging or replacing an existing, working orchestration layer rather than
  filling a gap.

### code-server (`coder/code-server`) + plain SSH
- **Pros:** Solves "browser-based terminal/editor over a remote connection," MIT licensed,
  much lighter than full Coder.
- **Cons:** It's a full VS Code-in-browser product, not a terminal-streaming primitive —
  adopting it means adopting its UI (competing with stapler-squad's own xterm.js-based
  `XtermTerminal.tsx`) and its own connection/auth model, for a scope (full IDE) far beyond
  "stream a tmux session over SSH."
- **Verdict: Not recommended.** Wrong grain — a full editor product where a terminal-stream
  primitive is needed.

### Eclipse Che / Gitpod OSS / Theia
- **Pros:** Same category as Coder — proven browser-based remote dev environment products.
- **Cons:** Same objection as Coder, amplified — these are Kubernetes-native platforms (Che,
  Gitpod OSS) or a full IDE framework (Theia) with even heavier operational footprints
  (cluster-per-workspace models) that assume container/pod-based workspace isolation, not
  "SSH to an arbitrary existing host and use its filesystem," which is what the requirements
  describe (`session on a remote machine`, not `session in a newly-provisioned container`).
- **Verdict: Not recommended.** Wrong isolation model (container/pod orchestration vs. SSH to
  an existing host) on top of the same "second orchestration system" problem as Coder.

**Overall §2 conclusion:** No existing OSS project solves this at the right grain for
stapler-squad. All the strong candidates (Coder, Che, Gitpod) are workspace-provisioning
platforms that assume they own infrastructure lifecycle; stapler-squad needs a
*connection primitive* (SSH to an existing host, run existing tmux+worktree logic
remotely) that plugs into infrastructure it already owns. Building the SSH+PTY+worktree
layer directly, reusing the existing delta protocol (§5), is less work than bridging or
replacing the existing session model to accommodate a wholesale platform.

## 3. OS keychain integration

**Options considered:** `zalando/go-keyring` vs. `99designs/keyring` vs. custom
encrypted-file-at-rest fallback.

**Existing codebase precedent (decisive factor):** `zalando/go-keyring v0.2.8` is already a
direct dependency (`go.mod`) and already in production use in this exact repo —
`github/keychain.go` wraps it for GitHub token storage (`keyringGet`/`keyringSet`/
`keyringDelete`, serialized behind a `sync.Mutex` because the underlying OS backends aren't
guaranteed thread-safe). It stores per-account tokens keyed by `github-token:<host>:<username>`
and already handles macOS Keychain and Linux Secret Service (D-Bus) backends in this
codebase's actual deployment targets.

### `zalando/go-keyring`
- **Pros:** Already a dependency, already has a working, tested integration pattern in this
  repo to copy (`github/keychain.go`'s mutex-wrapped get/set/delete plus its `MockInit()`
  test-mode hook — search `keychainMu`, `keyringGet` for the pattern). Cross-platform: macOS
  Keychain, Linux Secret Service/D-Bus, Windows Credential Manager. No new license to vet
  (MIT, already accepted into the dependency tree).
- **Cons:** Narrower backend support than 99designs/keyring — no KWallet (KDE) support, and
  open PRs for custom keyring-name support have sat unmerged for a while per community
  comparisons. For a personal-use Linux-primary tool (per the requirements'
  target-user context) this gap is unlikely to matter in practice.
- **Verdict: Recommended.** Reuse the existing `github/keychain.go` pattern directly —
  same library, same mutex-wrapping discipline, a new `keychainService`/key-prefix scoped to
  SSH host credentials (e.g. `ssh-host-key:<host>`) rather than `github-token:*`. Introducing
  a second keyring library for one feature when a working one is already wired in and tested
  would be pure duplication.

### `99designs/keyring`
- **Pros:** Broader backend coverage (adds KWallet, custom keyring names, and a file-backed
  encrypted fallback backend built in).
- **Cons:** Would be a second, redundant dependency for the same job zalando/go-keyring
  already does in this codebase; two keyring libraries means two OS-integration surfaces to
  maintain, test, and reason about thread-safety for.
- **Verdict: Not recommended** — not because it's a worse library, but because it duplicates
  an already-solved, already-tested problem in this specific codebase.

### Custom encrypted-file-at-rest fallback
- **Pros:** Full control, no OS keychain dependency (useful for headless/CI or a
  keychain-less Linux box without Secret Service running, e.g. some WSL2/CI setups).
- **Cons:** Reinvents key derivation, at-rest encryption, and file permission hardening — a
  correctness- and security-sensitive area that's exactly where a purpose-built,
  security-audited library earns its keep over bespoke code.
- **Verdict: Viable only as a documented fallback path**, not a primary mechanism — e.g. if
  `go-keyring` returns "no keyring backend available" on a given Linux target (common on
  headless boxes without a D-Bus session), fail loudly and instruct the user to run a Secret
  Service provider, rather than silently downgrading to on-disk storage of SSH key material.

## 4. Reverse tunnel / approval callback path

**Question posed by requirements:** hand-rolled `ssh -R` via `x/crypto/ssh` vs. a tunneling
library (inlets/ngrok SDK/chisel) vs. **not needing a separate tunnel at all**.

### Do we need a reverse tunnel at all? — the load-bearing question
The terminal-streaming connection already requires an **outbound** SSH connection from the
local stapler-squad process to the remote host (to attach the remote tmux session and stream
its output back). SSH's channel model is explicitly multiplexed: a single authenticated
`ssh.Client` connection can carry an arbitrary number of independent logical channels
(RFC 4254 channel layer — confirmed via the `x/crypto/ssh` docs and RFC references above).
That means the *same already-open, already-authenticated* connection used for the terminal
stream can carry a second channel dedicated to approval-request messages — e.g. the remote
agent writes an approval-request payload to a Unix domain socket or named pipe on the remote
host, and the local process opens a second `ssh.Client.Dial`-style direct-streamlocal channel
(`direct-streamlocal@openssh.com`, the same mechanism `ssh -L`/`-R` use under the hood) to
read it — **with no separate listening port opened anywhere, and no second SSH connection.**

- **Pros of channel-reuse (no new tunnel):** Zero new exposed network surface — the explicit
  goal called out in the requirements ("Favor the option that avoids adding a new exposed
  network surface"). One fewer credential/auth handshake to manage. One fewer failure mode
  (a second tunnel dying independently of the terminal stream). Reuses the exact SSH
  connection object already being held open and monitored for the terminal stream, so
  connection-health tracking (also required) only has one connection to watch, not two.
- **Cons:** Requires the remote-side agent process to write approval requests somewhere
  reachable via a channel type the local side can dial (a Unix socket via
  `direct-streamlocal@openssh.com`, or simpler: just poll/read a file over SFTP, or push
  messages over a second `session` channel running a long-lived reader process). This is
  slightly more design work up front than "just open an `-R` port," but it's a one-time
  design cost, not an ongoing maintenance cost.
- **Verdict: Recommended.** No new tunnel, no new library. This directly satisfies the
  requirement to avoid new exposed network surface and reuses infrastructure (the SSH
  connection, and health tracking already needed for it) the feature needs anyway.

### Hand-rolled `ssh -R` via `x/crypto/ssh`
- **Pros:** If channel-reuse above turns out to be insufficient (e.g. the approval flow needs
  the *remote* host to initiate a connection back to a local HTTP callback endpoint rather than
  the local side polling), `x/crypto/ssh`'s `Listen`/`ListenTCP` on the client side plus a
  standard `-R`-style remote forward is well-documented and uses the same already-open
  connection and library already in the dependency tree — still zero new libraries.
- **Cons:** Opens a listening port on the remote host (even if bound to localhost there),
  which is a larger blast radius than channel-reuse, and duplicates functionality
  channel-reuse already provides for this use case.
- **Verdict: Viable as a fallback** if pure channel-reuse (Unix socket / streamlocal) proves
  insufficient for the approval-callback shape, but shouldn't be the default design given a
  local-initiated channel accomplishes the same thing with less surface area.

### Third-party tunneling libraries (inlets, ngrok SDK, chisel)
- **inlets:** The open-source version is now source-available only and receives no updates —
  it was superseded by "inlets PRO," a commercial product. Adopting the OSS version means
  adopting an unmaintained dependency.
- **chisel (`jpillora/chisel`):** Actively maintained, MIT licensed, and functionally capable
  (TCP/UDP tunnel over HTTP, secured via SSH, with reverse port forwarding) — but it's
  designed for punching through firewalls/NATs from an *external* network into an internal
  one (its most visible real-world usage is in penetration-testing writeups), which is a
  different threat model than "two ends already have SSH access to each other." Pulling in a
  whole second tunneling protocol/tool when the existing SSH connection already has a
  multiplexed channel primitive available is unjustified complexity.
- **ngrok SDK:** Requires routing through ngrok's hosted relay infrastructure — a third-party
  network dependency and an external hop for what should be a direct local↔remote path,
  plus a new account/credential to manage.
- **Verdict: Not recommended**, any of the three. They solve "connect two hosts that don't
  already have a channel between them" — a problem this feature doesn't have, since the
  terminal-streaming SSH connection already provides that channel.

## 5. Reuse the existing delta protocol vs. reimplement

**Finding: this codebase already has a working terminal delta-streaming protocol; reuse it
rather than inventing a new one for the remote-transport case.**

Located via `grep -rn delta server/services/connectrpc_websocket.go server/services/session_service.go`:

- `server/services/connectrpc_websocket.go` implements `streamViaControlMode` (tmux control
  mode, `-C` flag — real-time structured `%output` notifications, see the function's doc
  comment referencing `https://github.com/tmux/tmux/wiki/Control-Mode`) and
  `streamViaTmuxCapturePane` (polling fallback that "detects content changes and only sends
  deltas," per the comment at `connectrpc_websocket.go:536`).
- `server/services/session_service.go` (`StreamTerminal` handler, around line 2346) runs a
  PTY-output-reading goroutine that reads raw bytes off a duplicated PTY file descriptor and
  sends them as `sessionv1.TerminalData` proto messages over a ConnectRPC bidi stream, with
  its own flow-control (pause/resume channel keyed to xterm.js backpressure) and a `sendMu`
  mutex to serialize concurrent `stream.Send()` calls from the output goroutine and an
  input-error-reply path.
- The frontend consumer is `web-app/src/components/sessions/XtermTerminal.tsx`, which already
  knows how to render this delta stream via xterm.js.

**Why reuse, not reimplement:** The remote case only changes *where the bytes come from* — a
local tmux control-mode session or a local PTY dup'd fd, versus the same tmux control-mode
protocol or PTY bytes read across an SSH `session` channel (`session.StdoutPipe()` from
`x/crypto/ssh`, functionally the same `io.Reader` shape as the local dup'd PTY fd already
consumed at `session_service.go:2389`). The proto message shape (`sessionv1.TerminalData`),
the ConnectRPC bidi-stream framing, the flow-control pause/resume mechanism, and the
xterm.js-side consumer are all transport-agnostic already — none of them assume "local PTY,"
they assume "some `io.Reader`/`io.Writer` pair that yields terminal bytes." Reimplementing a
parallel delta protocol for the SSH case would duplicate this proto contract, this
flow-control logic, and this frontend consumer for zero behavioral benefit — the same
control-mode-vs-capture-pane fallback structure that exists for local tmux sessions applies
identically once the tmux commands are being run over an SSH `exec`/`session` channel instead
of a local subprocess.

- **Verdict: Recommended — extend, don't reimplement.** The integration point is
  `streamViaControlMode`/`streamViaTmuxCapturePane`'s data source: swap the local tmux
  control-mode pipe / dup'd PTY fd for an SSH `session` channel's stdout/stdin, keep
  everything above that (proto shape, flow control, ConnectRPC framing, xterm.js consumer)
  unchanged.

## Summary table

| Sub-component | Recommended approach | Verdict |
|---|---|---|
| SSH client/PTY | `golang.org/x/crypto/ssh` directly (already an indirect dep via go-git) | Recommended |
| — wrapper alternative | `melbahja/goph` for simple exec-only calls | Viable (narrow scope only) |
| Wholesale orchestration (Coder/Che/Gitpod/code-server) | Build on existing session/tmux/worktree stack instead | Not recommended (any of them) |
| OS keychain | `zalando/go-keyring` — already a dependency, already integrated in `github/keychain.go` | Recommended |
| — alternative | `99designs/keyring` | Not recommended (redundant) |
| — fallback | custom encrypted file | Viable as documented fallback only |
| Reverse tunnel/approval callback | Reuse the existing outbound SSH connection's multiplexed channels (Unix-socket/streamlocal), no new tunnel | Recommended |
| — fallback | hand-rolled `ssh -R` via `x/crypto/ssh` | Viable fallback |
| — third-party tunnel libs (inlets/chisel/ngrok) | — | Not recommended (any of them) |
| Delta/terminal streaming protocol | Reuse existing `connectrpc_websocket.go` + `session_service.go` protocol, swap PTY source for SSH channel | Recommended |

## Sources

- [melbahja/goph (GitHub)](https://github.com/melbahja/goph)
- [goph LICENSE](https://github.com/melbahja/goph/blob/master/LICENSE)
- [zalando/go-keyring (GitHub)](https://github.com/zalando/go-keyring)
- [99designs/keyring (GitHub)](https://github.com/99designs/keyring)
- [Coder Community: Our Open-Source Offering (coder.com blog)](https://coder.com/blog/coder-community-open-source)
- [golang.org/x/crypto/ssh package docs](https://pkg.go.dev/golang.org/x/crypto/ssh)
- [jpillora/chisel (GitHub)](https://github.com/jpillora/chisel)
- [awesome-tunneling list (inlets OSS status)](https://github.com/anderspitman/awesome-tunneling)
- In-repo: `github/keychain.go`, `go.mod`, `server/services/connectrpc_websocket.go`,
  `server/services/session_service.go`, `web-app/src/components/sessions/XtermTerminal.tsx`
