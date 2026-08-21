# Research: Feature Landscape — ssh-remote-workspaces

Scope: comparable-tool patterns for remote workspace provisioning/reattach/credentials,
this codebase's local-only assumptions (with exact file:line references), unstated
failure modes, and unstated user needs implied by "remote workspace."

## 1. What comparable tools do

### VS Code Remote-SSH
- **Provisioning**: installs a "VS Code Server" process on the remote host over the SSH
  connection on first open; the local client is a thin UI that talks to that server
  over the SSH tunnel. Most extensions execute on the remote host, not locally — a
  clean local/remote split by *where code executes*, not just where files live.
- **Reattach**: the extension itself doesn't provide session persistence — its own docs
  point users at `tmux`/`screen` on the remote host for that (`tmux new -A -s main`
  style), i.e. it explicitly delegates reattach to a remote multiplexer rather than
  building its own.
- **Credential UX**: reuses the user's local `~/.ssh/config` and SSH agent; no
  credential store of its own.
- Relevance: stapler-squad already *has* a remote-capable terminal multiplexer story
  (tmux) — the gap is that today it only ever runs the tmux server locally.
- Sources: [Remote Development using SSH](https://code.visualstudio.com/docs/remote/ssh), [Supporting Remote Development and GitHub Codespaces](https://code.visualstudio.com/api/advanced-topics/remote-extensions)

### JetBrains Gateway
- **Provisioning**: a local "Gateway" launcher starts a headless IDE backend on the
  remote/cloud host, then a "thin client" connects to it. Backend and frontend are two
  separate processes from the start (unlike Remote-SSH's single extension host).
- **Reattach**: because the IDE backend is a long-lived remote process independent of
  any one client connection, closing/reopening the thin client just reconnects to the
  still-running backend — persistence is a property of the architecture, not a
  bolt-on multiplexer.
- **Credential UX**: TLS 1.3 end-to-end encryption for the client↔backend protocol even
  when tunneled over SSH; plugin/settings sync between client and host.
- Sources: [A Deep Dive Into JetBrains Gateway](https://blog.jetbrains.com/blog/2021/12/03/dive-into-jetbrains-gateway/), [Connect and work with JetBrains Gateway](https://www.jetbrains.com/help/idea/remote-development-a.html)

### Coder (coder.com)
- **Provisioning**: a "workspace agent" process runs inside each provisioned workspace
  and maintains a persistent outbound connection back to `coderd` (the control plane)
  — i.e. the remote side dials home rather than the control plane dialing in. This
  sidesteps inbound-firewall/NAT problems entirely.
- **Credential UX**: generates an SSH key pair per user automatically; supports
  SSH-agent forwarding (`$SSH_AUTH_SOCK`) and GPG-agent forwarding; has an "External
  Auth" subsystem that gets Git-provider tokens into the workspace without the user
  ever touching a key file. Recent versions support ephemeral, scoped-credential
  workspaces with network allowlists.
- Relevance: the "agent dials home" pattern is the direct answer to this feature's
  "approval requests round-trip local↔remote" requirement — instead of the local
  dashboard reaching into the remote host, the remote agent process could hold a
  persistent connection back to the local (or hosted) stapler-squad server.
- Sources: [Agent Architecture](https://deepwiki.com/coder/coder/3.1-agent-architecture), [External Auth for Git Providers](https://coder.com/docs/admin/external-auth), [Workspace Services](https://deepwiki.com/coder/coder/3.3-workspace-services)

### Emdash (named in the source issue)
- Desktop app, each agent task gets its own git worktree; for remote it uses **SSH +
  SFTP** directly against a user-specified remote path rather than provisioning a
  managed backend process — closest architectural sibling to what issue #48 proposes.
  "Point Emdash at the remote and run agents there, each agent gets its own remote
  worktree."
- **Credential UX**: explicitly stores credentials in the **OS keychain** (macOS
  Keychain / Windows Credential Manager / Linux Secret Service), and supports
  SSH-agent, key-file, and password auth as three separate credential modes — this
  directly validates the requirement's "ideally stored in OS keychain, not plaintext
  in config.json."
- Sources: [Emdash GitHub README](https://github.com/generalaction/emdash/blob/main/README.md), [Remote Projects — Emdash Docs](https://docs.emdash.sh/remote-projects), [Emdash AI Coding Agent Review](https://www.morphllm.com/emdash-ai-coding-agent)

### cmux (named in the source issue)
- `cmux ssh user@remote` creates a workspace bound to a remote machine. Two details
  worth stealing:
  - **Network transparency**: "browser panes" (its embedded preview) route HTTP/WS
    traffic through the remote machine's network so typing `localhost:3000` locally
    reaches the dev server running on the remote box, with **no manual `-L` port
    forwarding**. This is exactly the class of problem stapler-squad's hardcoded
    `http://localhost:8543` hook-callback URL (see §2) will hit in reverse — a
    process running on the remote host needs to reach the *local* dashboard.
  - **scp drag-and-drop**: detects the foreground SSH process by TTY and routes a
    dropped file through the existing SSH `ControlMaster` multiplexed connection —
    no separate credential prompt, reuses the already-authenticated channel.
  - **Notification round-trip**: a `cmux notify` call issued *on the remote box*
    surfaces in the local sidebar — direct precedent for "approval requests round-trip
    local↔remote."
- Sources: [cmux SSH docs](https://cmux.com/docs/ssh), [cmux SSH blog post](https://cmux.com/blog/cmux-ssh), [first-class ssh workspaces issue](https://github.com/manaflow-ai/cmux/issues/1664)

### mosh + tmux (the underlying pattern all of the above build on)
- Division of labor: **mosh** owns connectivity (roams across IP changes, survives
  network drops, reconnects automatically) while **tmux** owns session persistence
  (detach/reattach, multiple windows/panes). The canonical combined invocation is
  `mosh user@host -- tmux new-session -A -s main` — `-A` attaches-if-exists /
  creates-if-not, so every reconnect lands in the same session regardless of how many
  times the network dropped.
- **Known limitation directly relevant to failure-mode analysis (§3)**: mosh survives
  network drops but **not a server restart** — UDP-based state is lost if the remote
  host reboots, same as this feature's stated "remote host reboots mid-session" edge
  case. And mosh requires UDP, which strict corporate VPNs/firewalls block — SSH
  (TCP) is the fallback.
- Sources: [Mosh and Tmux: Uninterrupted Remote Terminal Sessions](https://hoop.dev/blog/mosh-and-tmux-uninterrupted-remote-terminal-sessions), [Get Better Remote Sessions on Linux With Mosh and Tmux](https://www.makeuseof.com/get-better-remote-sessions-on-linux-with-mosh-and-tmux/)

---

## 2. Local-only assumptions in this codebase

Grep/read-based survey of `session/instance.go`, `session/instance_worktree.go`,
`session/tmux/`, `session/git/`, plus the hook-callback wiring that turned out to be
the most load-bearing local assumption of all. Findings grouped by mechanism.

### 2a. tmux server is always the local machine's tmux binary

`session/tmux/tmux.go` and `session/tmux/server_registry.go` route **every** tmux
operation through `safeexec.CommandContext(Binary(), args...)` where `Binary()`
(`session/tmux/binary.go:16`, `session/tmux/binary_embedded.go:34`) resolves to a path
on the *local* filesystem (either the system `tmux` or the embedded one — see
`.claude/docs/bundling-tmux.md`). Concretely:

- `session/tmux/tmux.go:285` — `exec.CommandContext(r.ctx, Binary(), args...)` starts
  the tmux control-mode attach process as a **local child process**.
- `session/tmux/tmux.go:500-520` (`EnsureServerRunning`) and `:623-642`
  (`CreateKeepaliveSession`) shell out to local `tmux start-server` / `new-session`.
- The `Socket` type (`session/tmux/tmux.go:354-407`, `ResolveSocket`/`prependSocket`)
  models "which local Unix-domain socket does this tmux server listen on" — the `-L
  <socket>` flag targets a socket file in the local filesystem's `/tmp` (or
  `TMUX_TMPDIR`). There is no concept of "which host" in this type at all — only
  "which socket on this host."
- `session/tmux/pty.go:22-35` — `pty.Start`/`pty.StartWithSize` (the `creack/pty`
  library) allocates a **kernel PTY device on the local machine** and returns an
  `*os.File` (`ptmx`) that is a local file descriptor. This is the deepest assumption
  in the whole stack: a PTY is fundamentally a local kernel object. Remote execution
  would need to either (a) SSH into the remote host and let *its* kernel allocate the
  PTY (i.e. run `ssh user@host tmux attach ...` as the local child process, with the
  local ptmx now wrapping the SSH client, not tmux directly), or (b) run a thin agent
  binary on the remote host that owns the local-to-that-host PTY and streams bytes
  back over a control channel — closer to the Coder "workspace agent dials home"
  model than to a direct local exec model.

### 2b. git worktrees assume a local filesystem path

- `session/git/worktree.go:19-24` — the worktree base directory is
  `filepath.Join(configDir, "worktrees")` where `configDir` comes from
  `config.GetConfigDir()`, i.e. always a path under `~/.stapler-squad/` on the
  *local* machine.
- `session/git_worktree_manager.go:223` — `os.Stat(worktreePath)` and `:326` —
  `safeexec.CommandContext(revCtx, "git", "-C", dir, "rev-parse", "HEAD")` both assume
  `dir` is locally statable and locally exec-able.
- `session/instance_worktree.go:89-140` — path resolution (`os.Stat`,
  `filepath.IsAbs`, `filepath.Join(basePath, i.WorkingDir)`) all operate on local
  paths; there is no host-qualification anywhere in this resolution chain.
- `session/instance_worktree.go:303` — `safeexec.CommandContext(ctx, "git", "diff",
  baseSHA+"..HEAD")` for diff generation runs `git` as a local subprocess against the
  local worktree directory.
- Per the repo's own `.claude/rules/prefer-go-git-over-subshells.md`, most of these
  should already prefer `go-git` (`github.com/go-git/go-git/v5`) which operates on a
  local `*os.File`-backed repo — go-git itself has **no built-in remote-filesystem
  transport**, so a remote worktree can't be opened by go-git directly either; the
  existing `IsCommitOnMain`-style hybrid (go-git first, CLI fallback for one named
  failure mode) sets a precedent for how a remote fallback might be structured, but
  every current go-git callsite still assumes a local `repoPath`.

### 2c. Approval/hook round-trip assumes `localhost:8543` is reachable from the agent process

This is the single most consequential local-only assumption for the requirement
"route approval requests from the remote agent back to the local dashboard":

- `server/services/hook_injector.go:51` —
  `var hookBaseURLFn = func() string { return "http://localhost:8543" }`, overridden
  at server wiring time via `SetHookBaseURLFn` to read the *real* listen address
  lazily (comment at `hook_injector.go:45-50` explains this is for `PORT=0` support),
  but even the real value is `config.ListenAddress`, whose default is
  `"localhost:8543"` (`config/config.go:427`) unless explicitly set to `0.0.0.0:8543`
  for "remote access" (`config/config.go:234` comment).
- The injected Claude Code hooks are `curl` commands (see
  `hookCommandReferencesURL` doc comment, `hook_injector.go:61-70`) that the **agent
  process itself executes** on `PermissionRequest`/`Stop`/`PreToolUse`/etc. If the
  agent process runs on a remote host, `curl 'http://localhost:8543/...'` resolves to
  the *remote* host's own loopback interface, not back to the local dashboard — the
  hook silently fails to reach the server (likely connection-refused, since nothing
  listens on the remote host's port 8543).
- This is exactly the reverse-tunnel/callback-URL problem the requirements doc names
  explicitly, and it's not hypothetical — it's a hardcoded `localhost` string baked
  into every hook command generated for every session today, local or (hypothetically)
  remote.

### 2d. Approval detection is local PTY-output screen-scraping keyed by local socket path

- `session/external_approval.go` — `ExternalApprovalMonitor.MonitorSession` and
  `GetPendingApprovals` are keyed by `socketPath string` (the tmux Unix-domain
  socket), and `createConsumer`/`createTmuxConsumer` (`:208`, `:465`) attach an
  `OutputConsumer` that scrapes the **local** PTY/tmux output stream for approval
  prompt text patterns (via the `detection` package). There's no network hop in this
  path at all today — detection happens by reading bytes off a local socket/PTY, not
  by receiving a webhook. A remote target needs this detection loop to run *against
  a stream sourced from the remote host* (either forwarded PTY bytes over the control
  channel, or the detection logic relocated to run on the remote agent process itself
  and reported back).

### 2e. `SessionType` has no host dimension

- `config/types.go:202-216` defines `SessionType` as a flat string enum
  (`directory`, `new_worktree`, `existing_worktree`, `new_project`, `one_off`) with no
  concept of "local" vs "remote" baked in. Per
  `.claude/rules/session-creation-registry.md`'s own documented pattern (the
  `autonomous` mode reuses `SESSION_TYPE_DIRECTORY` + a bool flag rather than a new
  enum value), "remote" is architecturally more likely to land as an **orthogonal
  flag/field** (a `RemoteTarget` struct: host, base_path, ssh_key) threaded through
  the existing session types, not a new `SessionType` value — consistent with how
  `one_off` and `autonomous` were both added as flags in `session/instance.go:481-482`
  and the 7-touchpoint registry, rather than forking the whole creation flow.

---

## 3. Unstated-but-necessary edge cases / failure modes

| Failure mode | Why it's necessary, not optional | Precedent |
|---|---|---|
| **Remote host unreachable at session-creation time** | `EnsureServerRunning`-style local retry/backoff (`session/tmux/tmux.go:500-520`, `ensureServerRunningWithRetry`) exists for the local tmux server already going down transiently; an SSH connect failure needs the same class of bounded-retry-then-fail-fast handling, surfaced as a distinct session creation error (not folded into the generic "path is required" validation error class documented in the session-creation registry). | Codebase precedent: `EnsureServerRunning`'s retry/backoff loop |
| **Remote disk full mid-session** | Confirmed real-world failure class in adjacent tooling: Claude Code's own Cowork sandbox mode has an open issue for exactly this ("per-session disk leak fills ephemeral disks; new sandboxes fail with `useradd: No space left on device`"). A remote worktree clone/checkout that fails partway through disk exhaustion needs to be distinguishable from "worktree creation failed for a git reason" so the UI can say "remote disk full" rather than a generic git error. | [Cowork disk-leak issue](https://github.com/anthropics/claude-code/issues/59856) |
| **Remote host reboots mid-session** | mosh's own documented limitation is "survives network drops but not a server restart" — the tmux server and every pane die with the host. Unlike a local restart (where stapler-squad's own `.claude/rules/tmux-keep-server-on-restart.md` shows the team already treats "tmux server died, sessions destroyed and silently rebuilt" as a *bug*, not acceptable behavior), a remote reboot is outside stapler-squad's control entirely — the UI needs a distinct "session lost, host rebooted" state rather than silently reattaching to a resurrected-but-empty tmux server and presenting it as if scrollback/state survived. | mosh limitation; local precedent in `tmux-keep-server-on-restart.md` |
| **SSH key rotated while sessions are running** | None of the tools researched document this explicitly, but Coder's per-user auto-generated keypairs plus external-auth-token model implies the answer: don't tie a *running* session's transport to a key that can be silently invalidated — either keep the existing SSH connection alive (already-authenticated channel survives key rotation on the client side until the connection drops) and only re-validate on next-connect, or explicitly surface "this remote's stored key no longer matches — reauthenticate" as a distinct connection-health state (see §4 UI needs) rather than a generic connection failure. |  |
| **`base_path` doesn't exist on the remote** | Direct remote analogue of the existing local check at `session/instance_worktree.go:140` (`os.Stat(startPath); os.IsNotExist(err)`) — the same check has to run over SSH (`ssh host stat <path>` or SFTP stat) before worktree creation begins, and needs its own error class distinct from "remote unreachable" (host reachable, path wrong vs. host not reachable at all are different fixes for the user). | Local precedent: `instance_worktree.go:140` |
| **Concurrent sessions on the same remote exhausting resources** | Coder's model of ephemeral, resource-scoped workspaces with network allowlists exists specifically because unbounded concurrent sessions on shared infrastructure is a known failure mode, not a hypothetical. stapler-squad already has a documented WIP-limit instinct for its own backlog automation (memory: "cap concurrent backlog work sessions at 2... 2026-07-12 OOM incident") — the same class of resource exhaustion applies per-remote-host once sessions can fan out beyond one laptop, and arguably needs it *more* since multiple local users could point at the same shared remote. | Coder's scoped/ephemeral workspace model; internal OOM-incident precedent |
| **Clock skew between local and remote** | Relevant specifically because tmux session timestamps, approval-request timestamps, and this codebase's own `instance_timestamp_signature_test.go` / hibernation-sweeper logic (`session/hibernation_sweeper.go`) reason about elapsed time to decide staleness/timeout. A remote host with skewed clock could make a fresh session look stale (or a stale one look fresh) to any timeout logic that compares a remote-stamped time against a local `time.Now()`. |  |
| **Remote git version mismatch** | `session/git_worktree_manager.go` and `session/git/*` shell out to a specific `git` CLI surface (worktree add/list/prune, rev-parse, diff) whose exact flag support varies by git version; an older remote git binary could silently reject a flag the local code assumes exists. No graceful version-check exists locally today either, so this would be new work, not a gap being closed. |  |

---

## 4. Unstated user needs implied by "remote workspace"

Beyond the explicit ask (create worktree/tmux/agent on remote host, stream terminal,
round-trip approvals, manage SSH credentials), the following needs are implied and
should be scoped explicitly (in or out) rather than left implicit:

1. **Remote host resource visibility before creating another session there.**
   Directly implied by the "concurrent sessions exhausting resources" failure mode
   above — a user deciding whether to spin up session #6 on a remote box needs to see
   its current CPU/mem/disk *before* committing, not discover exhaustion after the
   worktree clone fails. None of the three closest competitors researched (Emdash,
   cmux, Coder) surface this in a lightweight SSH-based client — Coder's is closest,
   but only because it owns full workspace provisioning (Kubernetes/cloud), not a bare
   SSH target. For a bare-SSH model like this feature's, the cheapest version is
   probably a periodic `ssh host uptime/df` health probe reused for the connection-
   status indicator the requirements already call for.

2. **Killing/cleaning up orphaned remote worktrees.** This is not a nice-to-have — it's
   a documented failure pattern in *every* adjacent tool researched: VS Code Server
   "doesn't clean up after itself, leaving orphaned data... that accumulates into
   gigabytes over time on shared development servers"; Gitpod-style workspace pods
   "remain running in the cluster and never get terminated" after their DB record is
   deleted; and a paired remote-server teardown can "fail filesystem deletion with
   'Directory not empty'... and still remove workspace metadata" (local state says
   gone, remote disk says otherwise). stapler-squad's local worktrees already need
   cleanup (`session/git/worktree_ops.go` — not read in depth here, but the pattern
   exists locally); a remote target doubles the ways state can desync (local DB says
   deleted, remote disk still has it — or the reverse, remote host was wiped/rebuilt
   and local DB doesn't know). A reconciliation sweep (list remote worktrees on
   connect, diff against known sessions, surface orphans) is implied, not optional.

3. **File transfer (drag-and-drop or equivalent).** The requirements doc's own
   "Competitive context" section flags this — both named competitors (Emdash: SFTP;
   cmux: scp-over-existing-ControlMaster-connection) treat it as a baseline feature,
   not a stretch goal. cmux's implementation detail (reuse the already-authenticated
   SSH channel rather than prompting for credentials again) is the right bar: file
   transfer should ride the same credential/connection the terminal session already
   established, not open a second auth flow.

4. **A path from "SSH key stored, host configured" to "credential is actually still
   valid"** — i.e. a connect-time or periodic credential health check surfaced as
   part of the same connection-status indicator (connected/reconnecting/
   disconnected) the requirements already ask for. This falls out of combining the
   "SSH key rotated while sessions are running" failure mode (§3) with the OS-keychain
   storage the requirements call for: once a key lives in a keychain instead of
   plaintext config, there's no user-visible file to notice has gone stale, so the
   UI becomes the only signal.

5. **Network-transparent localhost for previews** (lower priority, but worth naming
   as an explicit non-goal if not building it). cmux's "browser panes route through
   the remote machine's network so `localhost:3000` just works" is a materially
   different feature from terminal streaming — it implies proxying arbitrary TCP/HTTP
   traffic, not just PTY bytes. Given this codebase's terminal streaming is
   purpose-built around the tmux delta protocol (not a generic port-forwarder), this
   is worth explicitly scoping out of v1 rather than letting it creep in via "the
   competitors do it."

---

## Sources referenced (full list)

- [Remote Development using SSH — VS Code Docs](https://code.visualstudio.com/docs/remote/ssh)
- [Supporting Remote Development and GitHub Codespaces — VS Code Extension API](https://code.visualstudio.com/api/advanced-topics/remote-extensions)
- [A Deep Dive Into JetBrains Gateway](https://blog.jetbrains.com/blog/2021/12/03/dive-into-jetbrains-gateway/)
- [Connect and work with JetBrains Gateway](https://www.jetbrains.com/help/idea/remote-development-a.html)
- [Coder Agent Architecture — DeepWiki](https://deepwiki.com/coder/coder/3.1-agent-architecture)
- [Coder Workspace Services — DeepWiki](https://deepwiki.com/coder/coder/3.3-workspace-services)
- [Coder External Auth for Git Providers](https://coder.com/docs/admin/external-auth)
- [Emdash GitHub README](https://github.com/generalaction/emdash/blob/main/README.md)
- [Emdash Remote Projects docs](https://docs.emdash.sh/remote-projects)
- [Emdash AI Coding Agent Review (YC W26)](https://www.morphllm.com/emdash-ai-coding-agent)
- [cmux SSH docs](https://cmux.com/docs/ssh)
- [cmux SSH blog post](https://cmux.com/blog/cmux-ssh)
- [cmux first-class SSH workspaces issue #1664](https://github.com/manaflow-ai/cmux/issues/1664)
- [Mosh and Tmux: Uninterrupted Remote Terminal Sessions](https://hoop.dev/blog/mosh-and-tmux-uninterrupted-remote-terminal-sessions)
- [Get Better Remote Sessions on Linux With Mosh and Tmux](https://www.makeuseof.com/get-better-remote-sessions-on-linux-with-mosh-and-tmux/)
- [Claude Code Cowork sandbox disk-leak issue #59856](https://github.com/anthropics/claude-code/issues/59856)
- [Cleaning Up After VS Code — Morgan Davis](https://www.morgandavis.net/post/cleaning-up-after-vs-code)
- [Orphaned workspace pods GitLab issue #555112](https://gitlab.com/gitlab-org/gitlab/-/issues/555112)

Codebase references (this repo, paths relative to repo root):
- `server/services/hook_injector.go:45-70`
- `config/config.go:234,427`
- `session/tmux/tmux.go:285,354-407,500-520,623-642`
- `session/tmux/binary.go:16`, `session/tmux/binary_embedded.go:34`
- `session/tmux/pty.go:22-35`
- `session/git/worktree.go:19-24`
- `session/git_worktree_manager.go:223,326`
- `session/instance_worktree.go:89-140,303`
- `session/external_approval.go:208,465`
- `config/types.go:202-216`
- `session/instance.go:481-482`
- `.claude/rules/prefer-go-git-over-subshells.md`
- `.claude/rules/session-creation-registry.md`
- `.claude/rules/tmux-keep-server-on-restart.md`
- `.claude/docs/bundling-tmux.md`
