# Research: Pitfalls — ssh-remote-workspaces

## 1. General SSH remote-execution pitfalls

- **Connection drops mid-command leave orphaned state on both ends.** A dropped TCP/SSH
  session doesn't kill the remote process unless it was launched with a session-bound PTY and
  no `nohup`/`setsid`/tmux detachment. Since this feature explicitly wants the tmux session to
  *survive* disconnect (per requirements), the remote process lifecycle must be decoupled from
  the SSH channel by construction — the tmux session lives on independent of the ssh.Client
  that created it. The risk is the inverse: if the coordinator's remote command executor treats
  "SSH channel closed" as "command failed" and retries `tmux new-session`, it can spawn a
  duplicate session or attach a second client to the same session with divergent state. The
  local `session/tmux_backend.go` control-mode design already draws this same line (out-of-band
  polling vs. attached control connection) — the remote analog needs an equivalent "is a
  session already running" existence check before create, mirroring the existing
  `DoesSessionExist()` polling pattern referenced in this repo's own log-pattern conventions.
- **Host key verification bypass / TOFU risk.** `ssh.InsecureIgnoreHostKey()` is the first thing
  every code sample uses and the easiest thing to accidentally ship. Trust-on-first-use (TOFU,
  i.e. accept-and-pin on first connect, like OpenSSH's default) is the minimum bar; silently
  ignoring host keys makes every remote workspace connection MITM-able, which is unacceptable
  given this feature's own credential is flowing over that channel. There is no existing
  `known_hosts`-equivalent store in this codebase — one must be designed (see stack-specific
  section).
- **Zombie remote processes when the local coordinator dies.** If Stapler Squad crashes/restarts
  (see `.claude/rules/tmux-keep-server-on-restart.md` — this exact failure mode already bit the
  *local* tmux server on a service restart missing `--tmux-keep-server`), remote tmux sessions
  and any SSH-forwarded child processes must not depend on the local process being alive. This
  argues for the remote tmux server being a detached, `nohup`'d process (which tmux already is
  by default) and for the coordinator treating "local restart" as "reconnect to existing remote
  session," not "recreate." The existing local reconciliation-on-restart logic
  (`session/tmux_backend.go`) is the pattern to extend, not reinvent.
- **SSH agent forwarding (`ForwardAgent`) is a lateral-movement risk.** Forwarding the local
  SSH agent to the remote host means any process running as the SSH login user on that remote
  host — including a compromised/malicious agent session running inside the very tmux pane this
  feature creates — can request signatures from the forwarded agent and impersonate the local
  user against *any other host* that key is trusted on. Given this feature's whole point is
  running an autonomous coding agent's shell commands on the remote host, agent forwarding is
  actively dangerous here: don't forward the interactive user's real agent. If the remote git
  worktree needs to push, use a separate, narrowly-scoped credential (deploy key / PAT) rather
  than `ForwardAgent`.
- **PTY resize race conditions over a network channel.** SIGWINCH-on-local-resize →
  `ResizePTY(cols, rows)` → SSH channel `window-change` request → remote tmux `resize-window` is
  a 3-hop async path. If resize requests are sent unthrottled on every browser resize event (as
  the existing local path already guards against — see `session/instance_tmux.go:455`
  `ResizePTY` and the IntelliJ SIGWINCH workaround at `session/instance_tmux.go:569`), the SSH
  channel can reorder or coalesce window-change requests, and a client that reconnects mid-resize
  can end up with a tmux window size that doesn't match its actual browser viewport until the
  next resize event fires. The local WebSocket handler already has an explicit comment about
  this exact race — "unconditionally sends a real SIGWINCH on every reconnect... makes [reattach
  robust]" (`server/services/connectrpc_websocket.go:1612`) — the remote transport needs the
  same unconditional resize-on-reconnect, not a delta-based one, because delta state itself may
  be stale after a network partition.
- **Character encoding / control-sequence corruption over compressed/multiplexed channels.**
  SSH compression (`zlib@openssh.com`) buffers output in blocks; if the local delta-protocol
  streamer reads from the SSH channel expecting timely partial writes (as `external_tmux_streamer.go`
  presumably does for local tmux `pipe-pane` output), compression can hold back a partial escape
  sequence across a block boundary, so a naive byte-buffer splitter can slice a multi-byte UTF-8
  sequence or an ANSI CSI sequence in half. This is a correctness issue distinct from local tmux
  streaming, which never has this class of buffering. Disable SSH compression for the terminal
  data channel (interactive traffic is latency-sensitive, not bandwidth-bound, so compression is
  pure risk with no benefit) or make sure whatever parses the stream is escape-sequence-boundary
  aware, not byte-count based.

## 2. Stack-specific risks (golang.org/x/crypto/ssh, tmux-over-SSH, config.json key storage)

- **`golang.org/x/crypto/ssh` is not currently a direct dependency of this repo** — `go.mod`
  only pulls it in transitively (`go.sum` entries only, no `require`). Adding it as a direct
  dependency means picking up its API defaults fresh, which are unsafe out of the box:
  - `ssh.ClientConfig.HostKeyCallback` has **no default** — the zero value is `nil`, and
    `ssh.Dial` will panic/error rather than silently connect insecurely, which is good, but it
    means every code sample that "just works" during development uses
    `ssh.InsecureIgnoreHostKey()` and that's exactly the line most likely to survive into a
    shipped build. Use `knownhosts.New()` from `golang.org/x/crypto/ssh/knownhosts` against a
    Stapler-Squad-managed known-hosts file, not the user's `~/.ssh/known_hosts` (see below).
  - **Algorithm negotiation**: `x/crypto/ssh`'s default `Config` (ciphers, KEX, MACs) lags
    OpenSSH's defaults and has in the past included algorithms later deprecated for weakness
    (e.g. older `diffie-hellman-group1-sha1` era defaults, arcfour ciphers were supported in old
    versions). Pin an explicit modern algorithm allowlist (`curve25519-sha256` KEX,
    `chacha20-poly1305@openssh.com`/`aes256-gcm@openssh.com` ciphers) rather than trusting the
    package default across upgrades — check the CHANGES file / release notes when bumping the
    dependency, since algorithm defaults have changed silently across x/crypto minor versions
    before.
  - **CVE history to check at pin time**: `golang.org/x/crypto` has had multiple SSH-specific
    CVEs (e.g. GHSA advisories for `ssh.NewSignerFromKey` / certificate parsing panics on
    malformed input, and a `x/crypto/ssh` DoS via crafted key exchange messages). Run
    `govulncheck ./...` (already implied by this repo's `make security`/`make analyze` targets)
    after adding the dependency and keep it in the regular dependency-update cadence, not a
    one-time check.
- **SSH key path referenced from `config.json` can go stale.** If the key material lives at a
  filesystem path recorded in config (e.g. `~/.ssh/stapler-squad-remote-host1`), a user
  rotating/regenerating that key file outside the app, or the path moving (different `$HOME` on
  a synced dotfiles machine, or the file being deleted by an unrelated cleanup script) breaks
  the remote connection silently until the next connect attempt — and the error surfaces as an
  opaque SSH auth failure, not "key file missing," unless the code explicitly `os.Stat`s the
  path first and returns a distinguishable error. Given this codebase already migrated GitHub
  tokens *off* plaintext config storage and onto `zalando/go-keyring` (see
  `github/keychain.go`) specifically to avoid this class of staleness/leakage, SSH private key
  material should follow the same precedent from day one rather than repeating the
  path-in-config mistake and needing a second migration later.
- **Key file permissions.** OpenSSH's client refuses (or `x/crypto/ssh`'s `ParsePrivateKey`
  succeeds but the *system* ssh-agent/other tooling would refuse) to use a private key file that
  is group/world-readable. If Stapler Squad ever writes an SSH private key to disk itself (e.g.
  as a keychain-unavailable fallback, or when generating a new keypair for a host), it must
  `os.WriteFile` with `0600` and verify the resulting mode — a key written with the process's
  default umask (often `0644` on some CI/container images) is silently readable by every local
  user, and a leaked key file becomes a much bigger problem than a leaked token because SSH keys
  are rarely short-lived/revocable within this app's control (see compliance section below).
- **Key added to the wrong/different OS keychain than expected.** `zalando/go-keyring`
  (already a dependency, used by `github/keychain.go`) abstracts macOS Keychain / Secret Service
  (D-Bus, Linux) / Windows Credential Manager, but on Linux specifically, Secret Service
  availability depends on a running session bus and an unlocked keyring daemon (gnome-keyring or
  kwallet) — the same class of environment-dependency already called out in this repo's
  `systemd-user-service.md` rule for `DBUS_SESSION_BUS_ADDRESS`. If Stapler Squad runs as a
  headless `systemd --user` service (which it does — see that rule) with no desktop session
  attached, Secret Service writes/reads for a new remote-host SSH key can silently fail or block
  waiting on a keyring unlock prompt that never appears in a headless context. `keychain.go`
  already guards concurrent access with `keychainMu sync.Mutex` because backend thread-safety
  isn't guaranteed — the same mutex discipline must extend to whatever key namespace this
  feature adds (a new `keychainService`/key-prefix scheme, not reusing `github-token:` keys).
- **tmux-over-SSH specific issues:**
  - **Nested tmux.** If the local dashboard is itself sometimes run inside a tmux pane (plausible
    for a dev running Stapler Squad from a terminal), and the remote session is also tmux, `$TMUX`
    env var leakage over the SSH connection can cause the remote tmux client to think it's
    already inside a session and misbehave (`tmux: sessions should be nested with care` or worse,
    attaching to the wrong socket). The SSH command that launches the remote tmux must explicitly
    unset `TMUX` in the remote environment before invoking `tmux new-session`/`attach-session`.
  - **`$TERM` mismatch.** The remote tmux server negotiates terminfo based on the `$TERM` sent
    over the SSH session (typically inherited from the *local* shell's `$TERM`, e.g.
    `xterm-256color` or increasingly `tmux-256color`). If the remote host's terminfo database
    lacks that entry (common on minimal/container base images), tmux falls back to a degraded
    terminal type and control-sequence rendering breaks for the xterm.js client on the other end
    of the delta-streaming protocol — this is a distinct failure mode from the local case, where
    `$TERM` is never cross-host. Force a known-good `$TERM` value explicitly in the remote
    command rather than trusting SSH's environment passthrough (which is disabled by default in
    most sshd configs via `AcceptEnv` anyway, so this can silently fall back to the remote's
    default `$TERM` regardless of the local value — verify behavior empirically, don't assume).
  - **Shared-host socket permission issues.** tmux sockets default to
    `/tmp/tmux-<uid>/default`, scoped by UID, so multi-user collision on a shared remote host is
    inherently mitigated by tmux itself *if* every Stapler Squad remote session connects as a
    distinct SSH user. The actual risk is the *reverse*: if multiple local Stapler Squad
    instances (e.g. this repo's own multi-instance pattern —
    `.claude/docs/state-isolation.md` — used for e2e tests and manual testing) all SSH into the
    same remote host as the *same* SSH user, they'll collide on the same tmux socket namespace
    unless each session name is derived from a value that's unique per local instance
    (`STAPLER_SQUAD_INSTANCE` or equivalent), not just per session ID that could theoretically
    collide across instances.

## 3. Compliance / blast radius — SSH key compromise

- **Blast radius without scoping is "full shell access to the remote host as that user."**
  Requirements ask for `base_path` scoping context; a standard SSH private key with no
  server-side restriction grants everything the target account can do — not just operations
  inside a given worktree directory. If the private key material is compromised (support bundle,
  log, crash dump — the exact vector named in requirements), the attacker gets an interactive
  shell on every remote host that key is authorized against, full stop. `base_path` is purely a
  client-side/application-level convention (this repo's local session isolation already scopes
  worktrees under a base path in config — see `config/config.go`) and provides **zero**
  enforcement against a stolen key used directly with `ssh`/`scp` outside the app.
- **The one server-side mitigation that actually reduces blast radius: a forced command /
  restricted key in the remote `authorized_keys`.** OpenSSH supports per-key restrictions in
  `authorized_keys` — `command="<wrapper-script>",restrict` (or the older
  `no-agent-forwarding,no-port-forwarding,no-X11-forwarding,no-pty` flags individually).
  `restrict` (added OpenSSH 7.2+) disables agent/port/X11 forwarding and PTY allocation by
  default, all of which must then be explicitly re-enabled per-flag — but this feature *needs*
  PTY allocation for the interactive tmux session, so the correct posture is `restrict,pty` plus
  a forced `command=` wrapper that constrains what the key can execute (e.g. only permitting
  `tmux -S <fixed-socket> ...` and `git` operations scoped under one directory prefix, rejecting
  anything else). This turns a compromised key from "full shell as the user" into "can only
  drive tmux/git within the sanctioned workspace," which is the actual scoping mechanism
  requirements are asking about — `base_path` alone doesn't provide it; `authorized_keys`
  restriction is what does. This should be documented as a **setup requirement for the user**
  (Stapler Squad can generate the recommended `authorized_keys` line during host onboarding, but
  can't force the remote sshd config), not something the app can silently guarantee — flag this
  explicitly in the plan as a "secure by default, but only if the user follows the onboarding
  flow" gap.
- **Per-host, per-purpose keys — not one key for every remote host.** A single SSH keypair
  reused across all configured remote hosts means compromise of one host (or the credential
  store) grants access to *all* of them. Generate a distinct keypair per remote host at
  onboarding time (mirroring the existing per-host token pattern already used for GitHub —
  `GetKeychainTokenForHost(host)` keys tokens by host, not one shared token) so revocation and
  blast-radius containment are per-host operations, not global ones.
- **Logging/redaction discipline — keys and passphrases must never reach
  `~/.stapler-squad/logs/`.** This repo already has a redaction primitive for exactly this class
  of problem: `executor/audit.go`'s `redactArgs`/`WithRedactArgs`/`WithProcessRedactArgs`
  replaces argv positions with `<redacted>` before they're logged. Any SSH-invoking code path —
  whether it shells out (`safeexec.CommandContext("ssh", ...)`, which per
  `.claude/rules/prefer-go-git-over-subshells.md` should only happen where `x/crypto/ssh`
  genuinely can't do the job) or logs `ssh.ClientConfig` construction — must ensure:
  - Private key **contents** (PEM bytes) are never passed as a CLI argument (they shouldn't be
    anyway — `-i <path>` is a path, not the key material — but a passphrase prompt or an
    in-memory key loaded from config-decrypted bytes must never be interpolated into a log
    string, error message, or `fmt.Errorf("... %v", cfg)` where `cfg` embeds the raw key).
  - `ssh.Signer`/`x/crypto/ssh` errors from a failed auth attempt can include the *public* key
    fingerprint, which is safe to log, but code must be reviewed to confirm no path
    accidentally stringifies the private half (e.g. via a naive `%+v` on a struct that embeds
    the raw `crypto.Signer`).
  - Passphrases for encrypted private keys, if supported, need the same treatment as the
    existing `MachineEncryptionKey`/`EncryptToken` pattern (`config/config.go:299`,
    `session/backlog_crypto.go`) — encrypted at rest, decrypted only in memory for the duration
    of the SSH handshake, never round-tripped through a log statement or error wrap.
  - Crash dumps / `--profile` pprof output (`.claude/docs/profiling.md`) can capture goroutine
    stacks including local variables in some debug builds — while Go's release pprof doesn't
    dump variable values, any custom panic-recovery logging that does `%+v` on a request struct
    carrying an in-flight SSH config must exclude the key field explicitly (e.g. via `json:"-"`
    equivalent + a custom `String()`/`LogValue()` that redacts it), the same discipline this repo
    already applies to GitHub tokens.

## 4. Design-against-from-day-one requirements

- **Reconnect/backoff strategy — don't hammer the remote host on a flaky network.** The repo
  already has two backoff precedents to reuse rather than reinvent: `executor/circuit_breaker.go`
  (open/half-open/closed state machine for command execution) and the WebSocket reconnect
  handling in `server/services/connectrpc_websocket.go`. A naive "retry SSH dial on every failed
  poll" loop with no backoff will, under a real network partition, either flood the remote sshd
  (risking its own rate-limiting/fail2ban-style lockout, which then blocks *legitimate*
  reconnects too) or busy-loop locally. Use exponential backoff with jitter and a max-retry
  circuit-open state per remote host (not global), so one flaky host doesn't starve retry budget
  from healthy ones.
- **Timeout budgets for remote git/tmux commands — a hung SSH connection must not block the
  dashboard UI thread.** Every remote-executed git/tmux command needs an explicit
  `context.WithTimeout`, sized per operation class (a `tmux has-session` existence check should
  time out in low single-digit seconds; a `git worktree add` with a fresh clone could
  legitimately take much longer). Because this is a ConnectRPC server backing a synchronous-feeling
  web UI, any handler that blocks on an SSH round-trip without a bounded context risks the same
  category of problem this repo's own `DoesSessionExist()` polling / control-mode logic was
  built to avoid for local tmux — the remote case is strictly worse because SSH TCP-level
  half-open connections can hang far longer than a local exec before the OS/kernel notices
  (no local process-exit signal to key off of). Budget timeouts per RPC, not per underlying SSH
  library call, so a single slow op can't wedge the whole request.
- **Reverse-tunnel/callback-URL approval path must not become an open relay.** If a remote agent
  needs to phone home to request approval (per requirements: "route approval requests from
  remote agent back to local dashboard via reverse tunnel or callback URL"), that endpoint is a
  new network-reachable surface that didn't exist before this feature. Concretely:
  - The callback URL/endpoint must require a per-session, single-use or short-TTL bearer
    credential minted by the coordinator at session-creation time — not a static shared secret
    baked into the remote host's environment, which would let any process on that remote host
    (not just the intended agent) forge approval-request traffic back to the local dashboard.
  - If implemented as an SSH reverse tunnel (`ssh -R`), the tunnel should bind to
    `127.0.0.1`/localhost on the remote side by default (SSH's default `GatewayPorts no`), not
    exposed on the remote host's public interface — otherwise *any* process/user on that remote
    machine (or, if the remote host has a public IP, anyone on the network) can reach the local
    dashboard's approval endpoint directly, bypassing the SSH-authenticated channel entirely.
    Explicitly verify `GatewayPorts` posture rather than assuming the default holds across every
    target sshd config.
  - The endpoint itself needs the same authn as every other privileged local RPC in this app
    (existing ConnectRPC middleware auth, per `server/` — reuse it, don't build a parallel
    unauthenticated path "just for this one feature"), plus rate-limiting so a compromised/rogue
    remote session can't use the callback path to DoS the local dashboard or probe for other
    session IDs.
