# Implementation Plan: ssh-remote-workspaces

**Feature**: Run agent sessions (worktree, tmux, terminal streaming, approvals) on a
remote Linux host over SSH, driven from the existing local Stapler Squad dashboard.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**:
- `decisions/ADR-001-remote-as-orthogonal-flag.md` — remote as orthogonal field vs new `SessionType`
- `decisions/ADR-002-commandrunner-in-session-tmux.md` — `CommandRunner` interface location
- `decisions/ADR-003-multiplex-approval-channel-no-tunnel-lib.md` — no reverse-tunnel library
- `decisions/ADR-004-authorized-keys-scoping-is-recommendation-not-enforcement.md` — authorized_keys scoping posture

---

## Step 0.5 — Creative Pass: Alternative Architectures Considered

Three distinct high-level approaches for how remote execution plugs into the existing
session/tmux/git stack were brainstormed before committing to a design:

**Approach A — Command-execution seam (`CommandRunner`).** Introduce a narrow interface at
the subprocess-exec boundary inside `session/tmux` (and reused by `session/git`), with
`LocalRunner`/`SSHRunner` implementations. *Strength*: minimal surface area — reuses ~100% of
existing tmux control-mode, streaming, and worktree logic unchanged above the exec boundary,
no new deployable artifact. *Weakness*: a naive single-method `Run(cmd) ([]byte, error)`
interface doesn't express the persistent, piped-stdio `tmux -C` control-mode attach flow,
which needs incremental read/write, not one buffered result — requires a slightly richer
two-method interface (`Run` + `Start`) to fully cover the existing streaming path.

**Approach B — Remote agent daemon (Coder-style "dial home").** Deploy a second small binary
on the remote host that owns local tmux/git operations there and talks back to the central
server over its own RPC channel, initiated outbound from the remote side (sidesteps inbound
firewall/NAT concerns). *Strength*: cleanly isolates remote-host lifecycle from the SSH
connection's liveness, agent-side buffering could survive local-server outages better than raw
SSH exec. *Weakness*: a second deployable artifact with its own version/compatibility matrix,
install/update flow, and auth model — disproportionate operational surface for a solo-user,
SSH-based feature (`research/build-vs-buy.md` §2 explicitly rejects wholesale orchestration
platforms on this basis).

**Approach C — Embed an existing remote-workspace platform (Coder OSS).** Run Coder (or
similar) as a subsystem to own remote workspace lifecycle end to end, bridging stapler-squad's
session model on top of it. *Strength*: inherits a mature, battle-tested provisioning/
reconnect/health model instead of building one from scratch. *Weakness*: requires bridging or
wholesale replacing stapler-squad's own session/tmux/git orchestration — two competing notions
of "a workspace" in one app — plus AGPLv3 licensing implications; `research/build-vs-buy.md`
§2 concludes this is strictly more work than the gap being filled.

**Chosen: Approach A**, refined with the two-method `CommandRunner` interface (`Run` for
one-shot exec, `Start` for the persistent piped-stdio control-mode case) to close the stated
weakness. See ADR-002 for the full interface shape and rationale.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Remote execution seam (this decision) | Command-execution seam (`CommandRunner`, Approach A) | This creative pass | Remote agent daemon (Approach B) | Second deployable artifact, version/compat/auth surface disproportionate to a solo-user SSH feature |
| Remote execution seam (this decision) | Command-execution seam (`CommandRunner`, Approach A) | This creative pass | Embedded workspace platform (Approach C, Coder OSS) | Requires bridging/replacing existing session/tmux/git orchestration; AGPLv3 licensing; more work than the gap being filled |

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `CommandRunner` | Interface (`session/tmux`) abstracting "how to execute a tmux/git command" — `Run` for one-shot exec, `Start` for a persistent piped-stdio process, `IsRemote() bool` identifying which concrete runner (`LocalRunner`/`SSHRunner`) a given value is. | Defined in the consumer package per ADR-002; reused by `session/git`. `IsRemote()` is what code holding only a `CommandRunner` (e.g. `tmux.go`, which cannot see `session.Instance`'s `ExecutionTarget` due to package layering) calls instead of a type-switch — see `ExecutionTarget` glossary entry. |
| `LocalRunner` | `CommandRunner` implementation wrapping today's `safeexec.CommandContext(...)` local subprocess exec. | Zero-behavior-change default; Phase 1 output. |
| `SSHRunner` | `CommandRunner` implementation executing commands over a persistent `*ssh.Client` connection. | Phase 2 output; built on `golang.org/x/crypto/ssh`. |
| `ExecutionTarget` | Sum type (`session/instance.go`) — `LocalTarget{}` or `RemoteExecutionTarget{Target RemoteTarget, Runner *tmux.SSHRunner}` — the single field `Instance` holds to express "does this session run locally or remotely, and via which runner." Exposes `IsRemote() bool` and `Runner() tmux.CommandRunner`. | Replaces two independently-settable fields (`Instance.RemoteTarget` pointer + whichever `CommandRunner` happened to be installed on `TmuxSession`/`GitWorktree`) that made "remote target set but local runner installed" a representable, silently-wrong state — see architecture-review.md Blocker 1. Constructed atomically at session-creation time (Task 4.2.1c); every "is this remote" branch (env wrapping, hook routing, approval-relay attach) reads this one field, or the `CommandRunner.IsRemote()` method on the exact runner value it wraps — never a separately-derived signal. |
| `RemoteTarget` | Go struct (`session/instance.go`) + proto nested message (`CreateSessionRequest`) identifying which configured remote and resolved base path a session runs against. | Orthogonal to `SessionType` per ADR-001. Carried as the `Target` field inside `RemoteExecutionTarget`, not as a separate `Instance` field. |
| `RemoteConfig` | `config.json` entry (`config/config.go`) describing one named SSH remote: `Name`, `Host`, `User`, `IdentityRef`, `BasePath`. | New `Remotes []RemoteConfig` field on `Config`. |
| `IdentityRef` | Opaque string in `RemoteConfig` pointing at a keychain-stored SSH credential. | Never a raw key path or key byte content. |
| `sshremote.KeyStore` | Package wrapping `zalando/go-keyring` (mirrors `github/keychain.go`) for SSH private key/passphrase storage, keyed by remote name. | New package `session/sshremote/keystore.go`. |
| `KnownHostsStore` | App-managed `known_hosts` file + wrapper around `golang.org/x/crypto/ssh/knownhosts`, used for TOFU host-key verification. | Distinct from the user's personal `~/.ssh/known_hosts`. |
| `HostKeyFingerprint` | SHA256 fingerprint string computed from an unrecognized host key, shown in the web UI's Trust/Cancel confirmation dialog. | Matches the format OpenSSH/VS Code display. |
| `RemoteConnectionState` | Enum (`connected` / `reconnecting` / `disconnected`) describing live SSH connection health for a configured remote. | Distinct from a session's lifecycle status (`getStatusText`). |
| `RemoteHealthProber` | Background component that periodically checks SSH connection liveness per `RemoteConfig` and publishes health-change events. | One prober per configured remote; no per-render polling. |
| `NewRemoteHealthChangedEvent` | `pkg/events` constructor publishing a `RemoteConnectionState` transition onto the existing `EventBus`. | Follows the `New*Event` naming convention already in `pkg/events/types.go`. |
| `RemoteWorktreeOps` | `session/git` type mirroring `GitWorktree`'s mutating operations (worktree add, init) but executed via a `CommandRunner` instead of go-git. | The documented exception to `prefer-go-git-over-subshells.md`. |
| `RemoteApprovalRelay` | Component opening a second multiplexed channel (`direct-streamlocal@openssh.com`) over the same `*ssh.Client` used for terminal streaming, carrying approval-request/response payloads. | See ADR-003; no new listening port. |
| `RemotePtySession` | The `*ssh.Session` wrapping `RequestPty`+`Shell`/`Start` for a remote tmux control-mode process, exposing `StdoutPipe`/`StdinPipe`/`WindowChange` in place of the local `*os.File` ptmx. | Backs `SSHRunner.Start`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Command execution seam | Strategy (GoF) — `CommandRunner` interface, `LocalRunner`/`SSHRunner` implementations | ADR-002 | Adapter forcing `*ssh.Session` to impersonate `*exec.Cmd` | `*exec.Cmd`-specific surface (PID, signals) has no SSH analog; several call sites (e.g. `kill -WINCH <panePID>`) depend on it |
| Remote/local execution target representation | Sum type — `ExecutionTarget` (`LocalTarget{}` / `RemoteExecutionTarget{Target, Runner}`) as `Instance`'s single field, with `IsRemote()`/`Runner()` methods | architecture-review.md Blocker 1 remediation | Two independently-settable fields (`Instance.RemoteTarget *RemoteTarget` + whichever `CommandRunner` happened to be installed on `TmuxSession`/`GitWorktree`) | The two-field design made "remote target set but a local runner installed" (or vice versa) a representable, silently-wrong state that would misroute `InjectHookConfig`'s approval hooks with no error — a single atomically-constructed field makes that state unrepresentable |
| Remote target integration | Composable flag/field (mirrors `autonomous_mode` precedent) | ADR-001 | New `SESSION_TYPE_REMOTE_*` enum values | Combinatorial explosion — every existing mode is meaningful remotely too |
| Credential storage | Facade over `zalando/go-keyring`, mirroring `github/keychain.go`'s mutex-wrapped Get/Set/Delete | `research/stack.md` §3, `research/build-vs-buy.md` §3 | `99designs/keyring` | Redundant second keyring library for a problem already solved and tested in this repo |
| Credential storage fallback | N/A — fail loudly, no fallback | `research/build-vs-buy.md` §3 | Custom encrypted-file-at-rest fallback | Reinvents key derivation/at-rest encryption, a security-sensitive area better left to an audited library |
| Approval callback transport | Channel-reuse over existing multiplexed SSH connection | ADR-003 | Hand-rolled `ssh -R` reverse tunnel | Opens a remote-side listener even if localhost-scoped — larger blast radius than channel-reuse for the same outcome |
| Approval callback transport | Channel-reuse over existing multiplexed SSH connection | ADR-003 | Third-party tunnel lib (chisel/ngrok/inlets) | Wrong threat model (NAT traversal between unrelated hosts) or unmaintained; unjustified new dependency |
| Terminal/PTY streaming | Adapter — generalize `PtyFactory`/control-mode data source to `io.Reader`/`io.Writer` | `research/stack.md` §4, `research/build-vs-buy.md` §5 | New remote-specific streaming protocol | Existing delta protocol, ConnectRPC framing, and xterm.js consumer are already transport-agnostic; reimplementing would duplicate all three for no behavioral benefit |
| Remote git worktree ops | Shell out via `CommandRunner` (documented exception), mirroring `ops.go`'s function shape | `research/stack.md` §2, `.claude/rules/prefer-go-git-over-subshells.md` | SFTP-backed `go-billy` filesystem | No such implementation exists in the go-git ecosystem for working-tree (not just object-store) operations; would be new, unproven infrastructure |
| Host key trust | TOFU via `knownhosts.New()` against an app-managed `known_hosts` file | `research/pitfalls.md` §2, `research/ux.md` §1 | `ssh.InsecureIgnoreHostKey()` | Makes every remote connection MITM-able; explicitly ruled out by the requirements' security framing |
| Host key trust | TOFU via `knownhosts.New()` against an app-managed `known_hosts` file | `research/ux.md` §1 | Reuse the user's personal `~/.ssh/known_hosts` | A headless server process has no terminal to prompt through; app-managed store lets the web UI own the confirm/cancel decision |
| Remote health distribution | Observer/pub-sub via existing `pkg/events` `EventBus`, structurally parallel to `sessionsSlice`/`ConnectionIndicator.tsx` | `research/architecture.md` §4 | Frontend polling a REST/health endpoint | Fights the existing push-driven pattern already proven for session connection status; adds per-render polling cost |
| Remote command construction | Explicit env normalization (`unset TMUX`, forced `$TERM`) before every remote tmux invocation | `research/pitfalls.md` §2 | Trust SSH env passthrough | `AcceptEnv` is disabled by default in most sshd configs; nested-tmux and terminfo-mismatch failures are silent otherwise |
| SSH connection lifecycle | Shared, reference-counted `*ssh.Client` per remote (Task 2.1.0a), work opened as channels on it | pre-mortem.md Failure #1 (P1) | One `*ssh.Client` dialed per session/prober | Burst session creation at the stated 10-20-concurrent goal collides with sshd's default `MaxStartups` throttle if every session/prober dials independently |

---

## Migration Plan

Additive-only. `config.Config.Remotes []RemoteConfig` (`json:"remotes,omitempty"`) and
`CreateSessionRequest.remote` (new field 28) are both new, optional fields — existing
`config.json` files and existing RPC clients are unaffected; zero-value (absent) `Remotes`/
`remote` behaves identically to today. No `ConfigVersion` bump is required (that field's
existing comment reserves it for schema changes that affect *existing* data shape, which this
is not). No data backfill, no destructive schema change, no proto field removal or renumbering.

## Observability Plan

- **Logs**: `SSHRunner` dial/auth/reconnect/disconnect events logged at `info`/`warn` via the
  existing structured logger, with private key material and passphrases redacted per
  `executor/audit.go`'s `redactArgs` pattern (never interpolated into a log string or
  `fmt.Errorf`). `RemoteHealthProber` logs state transitions (`connected → reconnecting →
  disconnected`) per remote name. `RemoteApprovalRelay` logs channel open/close, not payload
  contents.
- **Metrics**: no formal metrics/counter system exists in this repo today (verified: no
  Prometheus/statsd client in `go.mod`); OpenTelemetry tracing is available
  (`.claude/docs/opentelemetry.md`) — add a span around `SSHRunner.dial` and
  `RemoteWorktreeOps` calls so remote-latency is visible in the same trace view already used
  for other request paths, rather than inventing a new metrics pipeline for this feature alone.
- **Alerts**: no standalone alerting system exists; repeated `disconnected` transitions for a
  remote are surfaced through the existing `NotificationPrefs` delivery channel by publishing
  through `pkg/events.NewNotificationEvent` (already wired to the user's configured
  notification destinations), not a new alert mechanism.

## Risk Control

- **Feature flag**: no formal feature-flag system exists in this codebase. The feature is
  naturally dark-launched by data presence — `Remotes` defaults to an empty list, so the
  Settings → Remotes UI, the Omnibar's remote selector, and host badges only render once ≥1
  remote is configured (mirrors how `research/ux.md` §2 already recommends this UX). Rollback
  of any individual phase is "stop configuring remotes," not a flag flip.
- **Rollback procedure**: every phase's changes are additive (new fields, new files, new
  optional proto fields) with no destructive migration — rolling back to a prior commit/release
  requires no data cleanup. `RemoteConfig` entries left in `config.json` after a rollback are
  simply ignored by code that no longer reads them.
- **Staged rollout**: Phase 1 is an invisible, behavior-preserving refactor (validated by
  characterization tests before any SSH capability exists). Phases 2-3 land backend
  infrastructure with no UI exposure (SSH transport, credential storage) — testable via unit/
  integration tests against a real or containerized sshd, not yet reachable from the dashboard.
  Phase 4 wires session creation end to end but is reachable only via direct RPC calls until
  Phase 6 ships the UI. Phase 5 (approvals) and Phase 6 (UI) ship last, once the underlying
  transport has proven stable in Phases 1-4.

## Unresolved Questions

- [ ] Is remote-host resource visibility (CPU/mem/disk before creating another session,
  `research/features.md` §4.1) in scope for v1, or explicitly deferred to a follow-up? —
  blocks Story 6.1.1 — owner: Tyler
- [ ] Should Stapler Squad generate an `authorized_keys` forced-command wrapper *script*
  (not just the recommended text line) during onboarding, or is displaying the line alone
  sufficient for v1 per ADR-004? — blocks Story 3.2.2 — owner: Tyler
- [ ] Does the connection-status indicator need Coder's finer-grained states
  (`connecting`/`timeout` in addition to `connected`/`reconnecting`/`disconnected`,
  `research/ux.md` §1), or is the three-state model sufficient to match the requirements'
  literal wording? — blocks Story 6.2.2 — owner: Tyler
- [x] ~~Is orphaned remote-worktree reconciliation (list remote worktrees on connect, diff
  against known sessions, surface orphans — `research/features.md` §4.2) in scope for this
  feature's v1, or a fast-follow?~~ **Resolved** (adversarial-review.md Blocker 3): **full
  reconciliation-on-connect is deferred to a fast-follow** — not in v1 scope. v1 *does* include
  best-effort cleanup-on-failure for the one concrete partial-failure mode this plan already
  identifies — remote `git worktree add` succeeds but the subsequent remote `tmux new-session`
  fails, or the SSH connection drops between the two steps (Task 4.2.1e). That narrower
  guarantee does not cover orphans from other causes (local process crash, `kill -9`, manual
  deletion on the remote host) — proactive reconciliation for those remains the fast-follow. No
  longer blocks Phase 2/Phase 6 scoping. — owner: Tyler

## Dependency Visualization

```
Phase 1: CommandRunner Seam (local-only, behavior-preserving)
  session/tmux/command_runner.go — CommandRunner, LocalRunner
        │
        ├───────────────────────────────┐
        v                                v
Phase 2: SSH Transport             Phase 3: Credential Storage + Config
  SSHRunner, RemoteWorktreeOps       RemoteConfig, sshremote.KeyStore,
  (ssh.ClientConfig injected            KnownHostsStore, TOFU confirm RPC
   directly for dev/testing —              │
   Phase 4 wires the real one)             │
        │                                  │
        └────────────────┬─────────────────┘
                          v
        Phase 4: Registry Integration (7 touchpoints) + Streaming
        Proto RemoteTarget field, resolveRemoteTarget, ExecutionTarget
        construction (atomic Target+Runner, Blocker 1), best-effort
        compensating cleanup on partial failure (Blocker 3),
        Omnibar/OmnibarCreationPanel/OmnibarContext/useSessionService,
        PtyFactory generalization → SSHRunner.Start stdio
                          │
                          v
        Phase 5: Approval Round-Trip
        RemoteApprovalRelay (multiplexed channel, ADR-003),
        hookBaseURLFn override, response delivery back to remote agent
                          │
                          v
        Phase 6: UI Surfaces + Registries
        Remotes settings UI, session-card host badge,
        RemoteConnectionIndicator + remotesSlice, feature registry, e2e tests
```

---

## Phase 1: CommandRunner Abstraction + Local-Only Regression Safety Net

### Epic 1.1: Define `CommandRunner` and `LocalRunner`

**Goal**: Introduce the execution seam with zero behavior change — every session still runs
locally, but through an interface that can later be swapped for SSH.

#### Story 1.1.1: Define the `CommandRunner` interface and `LocalRunner`
**As a** maintainer, **I want** a `CommandRunner` interface with a local implementation,
**so that** tmux/git operations can later be executed through a swappable execution seam
without changing any call site's observable behavior today.
**Acceptance Criteria**:
- `CommandRunner` has `Run` (one-shot, buffered output) and `Start` (persistent, piped
  stdin/stdout) methods, defined in `session/tmux`.
  - *Given* `LocalRunner{}` is constructed, *When* `Run(ctx, "tmux", "list-sessions")` is
    called, *Then* it returns the same `([]byte, error)` shape as today's
    `safeexec.CommandContext(ctx, "tmux", "list-sessions").Output()`.
- `LocalRunner.Start` returns a live `io.WriteCloser`/`io.ReadCloser` pair backed by a real
  `exec.Cmd`'s stdin/stdout pipes plus a `wait func() error`.
  - *Given* `LocalRunner{}` is constructed, *When* `Start(ctx, "cat")` is called and bytes are
    written to the returned stdin, *Then* the same bytes are readable from the returned
    stdout before `wait()` is called.
**Files**: `session/tmux/command_runner.go` (new), `session/tmux/command_runner_test.go` (new)

##### Task 1.1.1a: Define `CommandRunner` interface (~3 min)
- Create `session/tmux/command_runner.go` with the `CommandRunner` interface (`Run`, `Start`,
  and `IsRemote() bool` signatures per the Domain Glossary entry) and a doc comment explaining
  the local/remote seam per ADR-002. `IsRemote()` is the single mechanism code holding only a
  `CommandRunner` value (e.g. `tmux.go`) uses to branch on remoteness — see
  architecture-review.md Blocker 1 and ADR-002's update.
- Files: `session/tmux/command_runner.go`

##### Task 1.1.1b: Implement `LocalRunner.Run` + `IsRemote` (~4 min)
- Add `LocalRunner` struct and `Run` method wrapping `safeexec.CommandContext(ctx, name,
  args...).CombinedOutput()`, matching today's error/output shape used at `tmux.go:328`.
  `LocalRunner.IsRemote()` returns `false`.
- Files: `session/tmux/command_runner.go`

##### Task 1.1.1c: Implement `LocalRunner.Start` (~5 min)
- Add `Start` method wrapping `safeexec.CommandContext(ctx, name, args...)` with
  `cmd.StdinPipe()`/`cmd.StdoutPipe()` and `cmd.Start()`, returning a `wait` closure over
  `cmd.Wait()`.
- Files: `session/tmux/command_runner.go`

##### Task 1.1.1d: Unit tests for `LocalRunner` (~5 min)
- Table-driven test: `Run` against `echo`/a failing command; `Start` against `cat` verifying
  bytes round-trip before `wait()`.
- Files: `session/tmux/command_runner_test.go`

### Epic 1.2: Migrate `tmux.go` call sites to `CommandRunner`

**Goal**: Replace every `safeexec.CommandContext(ctx, Binary(), args...)` call site in
`session/tmux/tmux.go` with `runner.Run`/`runner.Start`, with `TmuxSession` defaulting to
`LocalRunner{}` so behavior is unchanged.

#### Story 1.2.1: `TmuxSession` accepts an injected `CommandRunner`
**As a** maintainer, **I want** `TmuxSession` to hold a `CommandRunner` field defaulting to
`LocalRunner{}`, **so that** every existing tmux operation keeps working unchanged while
becoming swappable.
**Acceptance Criteria**:
- Existing `NewTmuxSessionWithServerSocket`-style constructors compile and behave identically
  with no explicit `CommandRunner` argument (default applied internally).
  - *Given* a `TmuxSession` constructed via today's existing constructor call sites, *When*
    `EnsureServerRunning` is invoked, *Then* the resulting `tmux start-server` subprocess is
    identical (same binary, same args, same socket) to pre-change behavior.
**Files**: `session/tmux/tmux.go`

##### Task 1.2.1a: Add `runner CommandRunner` field + default (~4 min)
- Add a `runner CommandRunner` field to `TmuxSession`'s struct; set to `LocalRunner{}` in
  every existing constructor unless already provided (new optional constructor parameter,
  keeping existing call signatures via a wrapping constructor or functional option per
  `golang-design-patterns` functional-options convention).
- Files: `session/tmux/tmux.go`

##### Task 1.2.1b: Migrate call sites — one-shot exec group 1 (lines 298, 328, 509) (~5 min)
- Replace the `safeexec.CommandContext(...).Output()`/`.CombinedOutput()` calls at these three
  sites with `t.runner.Run(ctx, Binary(), args...)`.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1c: Migrate call sites — one-shot exec group 2 (lines 533, 555, 612) (~5 min)
- Same substitution for these three call sites.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1d: Migrate call sites — one-shot exec group 3 (lines 631, 642, 898) (~5 min)
- Same substitution; line 631 uses `.Run()` (not `.Output()`/`.CombinedOutput()`) — confirm
  `CommandRunner.Run`'s error-only-on-nonzero-exit semantics match before swapping.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1e: Migrate call sites — remaining group (lines 1902, 2295, 2318) (~5 min)
- Same substitution for the final three call sites.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1f: Migrate the persistent control-mode attach process (~5 min)
- Identify the control-mode `tmux -C attach-session` subprocess start (in
  `session/tmux/control_mode.go` or `tmux.go`'s control-mode section) and convert it from
  direct `exec.Cmd` construction to `t.runner.Start(...)`.
- Files: `session/tmux/control_mode.go` or `session/tmux/tmux.go` (whichever owns the control-mode start)

### Epic 1.3: Extend the seam into `session/git` worktree mutations

**Goal**: `GitWorktree`'s mutating operations route through the same `CommandRunner`, per the
documented exception to `prefer-go-git-over-subshells.md`.

#### Story 1.3.1: `GitWorktree.runGitCommand`/`runExec` take a `CommandRunner`
**As a** maintainer, **I want** `GitWorktree`'s subprocess-exec helpers to accept the same
`CommandRunner` interface `session/tmux` uses, **so that** remote worktree creation (Phase 2)
can reuse this seam instead of inventing a second one.
**Acceptance Criteria**:
- `runGitCommand`/`runExec` (`session/git/worktree_git.go:27,301`) call `runner.Run(...)`
  instead of constructing `exec.Cmd` directly, with `GitWorktree` defaulting to
  `tmux.LocalRunner{}`.
  - *Given* a `GitWorktree` constructed via today's existing constructor, *When*
    `CommitChanges` is called, *Then* the resulting `git commit` subprocess is identical to
    pre-change behavior.
**Files**: `session/git/worktree_git.go`

##### Task 1.3.1a: Add `runner tmux.CommandRunner` field + default (~4 min)
- Add the field to `GitWorktree`'s struct, defaulting to `tmux.LocalRunner{}` in existing
  constructors.
- Files: `session/git/worktree_git.go`

##### Task 1.3.1b: Migrate `runGitCommand` (line 27) (~4 min)
- Replace direct `exec.Cmd` construction with `g.runner.Run(ctx, "git", args...)`.
- Files: `session/git/worktree_git.go`

##### Task 1.3.1c: Migrate `runExec` (line 301) and its caller at line 309 (~4 min)
- Same substitution.
- Files: `session/git/worktree_git.go`

### Epic 1.4: Regression safety net

**Goal**: Prove Phase 1's refactor is behavior-preserving before any SSH capability exists,
so Phase 2+ bugs are attributable to new code, not to this migration.

#### Story 1.4.1: Characterization tests for local-runner parity
**As a** maintainer, **I want** characterization tests that pin `LocalRunner`'s exact
output/error shape against the pre-refactor `safeexec.CommandContext` behavior, **so that**
Phase 1's migration is provably behavior-preserving.
**Acceptance Criteria**:
- All existing `session/tmux` and `session/git` unit/integration tests pass unmodified after
  Phase 1's migration (no test assertions needed to change).
  - *Given* the full pre-Phase-1 test suite for `session/tmux` and `session/git`, *When* run
    against the post-Phase-1 code, *Then* every test passes with zero modifications to
    expected values.
**Files**: `session/tmux/tmux_test.go`, `session/git/worktree_git_test.go`

##### Task 1.4.1a: Run full existing test suite, diff against baseline (~5 min)
- `make build && go test ./session/tmux/... ./session/git/...`, confirm zero failures and zero
  required assertion changes; record any discrepancy as a bug in this migration, not a test
  update.
- Files: (test execution only, no file changes expected)

##### Task 1.4.1b: Add one explicit `LocalRunner`-vs-`safeexec` parity test (~5 min)
- New test asserting `LocalRunner{}.Run(ctx, "tmux", "-V")` returns byte-identical output to
  `safeexec.CommandContext(ctx, "tmux", "-V").CombinedOutput()`, as a permanent regression
  guard for this seam.
- Files: `session/tmux/command_runner_test.go`

---

## Phase 2: SSH Transport + Remote Worktree/tmux

### Epic 2.1: `SSHRunner` — connection lifecycle and command execution

**Goal**: Implement `CommandRunner` over a persistent `*ssh.Client`, with TOFU host-key
verification and reconnect/backoff.

**Design decision — one shared `*ssh.Client` per remote, not one per session (pre-mortem.md
Failure #1, P1):** `SSHRunner` instances for the same `RemoteConfig` share a single dialed
`*ssh.Client`, keyed by remote name in a small connection registry (`session/tmux/ssh_pool.go`),
with each session's/prober's actual work (`Run`/`Start`/PTY) opened as a new SSH *channel* on
that shared connection rather than a new TCP+SSH handshake. Without this, a burst of session
creation at the feature's stated 10-20-concurrent-session goal produces a burst of concurrent
handshakes that collides with sshd's default `MaxStartups` (10:30:100) throttle, surfacing as
opaque connection failures on exactly the scenario this feature exists to serve. The registry is
reference-counted (last channel closing does not tear down the client; the client is torn down
only on explicit remote-config removal or a detected dead connection, at which point the next
caller redials and re-registers). `RemoteHealthProber` (Epic 6.4) reuses the same pooled client
for its liveness checks rather than opening a dedicated connection.

##### Task 2.1.0a: Implement per-remote `*ssh.Client` pool/registry (~5 min)
- `session/tmux/ssh_pool.go`: `GetOrDial(remoteName, ssh.ClientConfig) (*ssh.Client, error)`,
  reference-counted, one entry per remote name; concurrent callers for the same remote-name-not-
  yet-dialed coalesce onto a single in-flight dial (singleflight) rather than racing two dials.
- Files: `session/tmux/ssh_pool.go`

##### Task 2.1.0b: Load test — burst session creation stays under `MaxStartups` (~5 min)
- Against a test sshd configured with a realistic `MaxStartups`, create 15-20 sessions
  concurrently against one remote and assert zero dial/handshake failures, verifying the pool
  is actually being shared (e.g. by asserting the number of distinct TCP connections opened
  does not scale with session count).
- Files: `session/tmux/ssh_pool_test.go`

#### Story 2.1.1: `SSHRunner` dials and executes via `golang.org/x/crypto/ssh`
**As a** maintainer, **I want** an `SSHRunner` that implements `CommandRunner` over an
authenticated `*ssh.Client`, **so that** `session/tmux` and `session/git` can execute commands
on a remote host through the exact same interface `LocalRunner` satisfies.
**Acceptance Criteria**:
- `SSHRunner.Run` executes a command over `ssh.Client.NewSession()` and returns combined
  stdout+stderr, matching `CommandRunner.Run`'s contract.
  - *Given* an `SSHRunner` dialed against a test sshd container, *When* `Run(ctx, "echo",
    "hi")` is called, *Then* it returns `[]byte("hi\n")` with a nil error.
- `SSHRunner` never uses `ssh.InsecureIgnoreHostKey()`; unknown host keys return a distinct,
  typed error (`ErrUnknownHostKey`) rather than connecting.
  - *Given* an `SSHRunner` configured with a `KnownHostsStore` that has no entry for the
    target host, *When* `Dial` is called, *Then* it returns `ErrUnknownHostKey` (wrapping the
    computed `HostKeyFingerprint`) and does not complete the SSH handshake.
- Every blocking `ssh.Dial`/`Session.Run`/`Session.CombinedOutput`/`Session.Start` call is
  raced against `ctx.Done()` and force-closes the underlying connection/session on timeout,
  with a documented per-operation-class timeout budget (`research/pitfalls.md` §4: dial ~10s;
  existence-check-class commands like `has-session` low single-digit seconds; longer-running
  commands like `git worktree add` up to ~60s) — `golang.org/x/crypto/ssh` has no native
  `context.Context` support, so cancelling `ctx` does nothing on its own (adversarial-review.md
  Blocker 2).
  - *Given* an `SSHRunner` dialing a host that accepts the TCP connection but never completes
    the SSH handshake (simulated via a test listener that accepts and stalls), *When* `Dial` is
    called with a 5s-timeout `ctx`, *Then* `Dial` returns a context-deadline error within ~5s
    (not blocking indefinitely) and the underlying TCP connection is force-closed.
  - *Given* an `SSHRunner.Run` call whose remote command never terminates and never produces
    output (simulated via a test sshd command that sleeps past the bound), *When* `Run` is
    called with a bounded `ctx`, *Then* it returns a context-deadline error at the bound and the
    underlying SSH session is closed rather than leaked.
**Files**: `session/tmux/ssh_runner.go` (new), `session/tmux/ssh_runner_test.go` (new)

##### Task 2.1.1a: Define `SSHRunner` struct + constructor (~4 min)
- `session/tmux/ssh_runner.go`: `SSHRunner` struct wrapping `*ssh.Client`; constructor takes
  host, `ssh.ClientConfig` (built from `IdentityRef`-resolved signer + `knownhosts.New()`
  callback — Phase 3 wires the real identity resolution, this task accepts a pre-built
  `ssh.ClientConfig` directly for testability). Implements `CommandRunner.IsRemote() bool`
  returning `true` (paired with `LocalRunner.IsRemote()` returning `false`, Task 1.1.1a) — the
  single mechanism every "is this remote" check reads, per architecture-review.md Blocker 1.
- Files: `session/tmux/ssh_runner.go`

##### Task 2.1.1b: Implement `SSHRunner.Run` (~5 min)
- `client.NewSession()` → `session.CombinedOutput(cmd)`, matching `CommandRunner.Run`'s
  signature.
- Files: `session/tmux/ssh_runner.go`

##### Task 2.1.1c: Implement `SSHRunner.Start` (persistent piped session) (~5 min)
- `client.NewSession()` → `session.StdinPipe()`/`session.StdoutPipe()` → `session.Start(cmd)`,
  returning a `wait func() error` over `session.Wait()`. Explicitly `session.Setenv` is
  skipped (most sshd `AcceptEnv` disabled by default) — env normalization is handled by
  prefixing the remote command itself (Story 2.3.1), not by `Session.Setenv`.
- Files: `session/tmux/ssh_runner.go`

##### Task 2.1.1d: Pin modern algorithm allowlist in `ssh.ClientConfig` (~4 min)
- Explicit `Config.KeyExchanges`/`Ciphers`/`MACs` allowlist (curve25519-sha256 KEX,
  chacha20-poly1305/aes256-gcm ciphers) rather than trusting package defaults, per
  `research/pitfalls.md` §2.
- Files: `session/tmux/ssh_runner.go`

##### Task 2.1.1e: Unit tests against a local test sshd (~5 min)
- Table-driven test using a lightweight in-process SSH server (e.g. `gliderlabs/ssh`, already
  an indirect test dep per `research/stack.md`) verifying `Run` and `Start` round-trip bytes
  correctly.
- Files: `session/tmux/ssh_runner_test.go`

##### Task 2.1.1f: Race every blocking SSH call — and the keychain read that precedes it — against `ctx.Done()` with a per-operation-class timeout budget (~7 min)
- `golang.org/x/crypto/ssh`'s `Dial`, `Session.Run`/`CombinedOutput`, and `Session.Start` are
  all blocking with zero context awareness. Wrap each in a goroutine whose result is selected
  against `ctx.Done()`; on `ctx` expiry, force-close the underlying `*ssh.Client`/`*ssh.Session`
  and return a context-deadline error rather than leaving the goroutine to leak silently.
  Document the timeout budgets from `research/pitfalls.md` §4 (dial ~10s, existence-check-class
  commands low single-digit seconds, longer-running commands up to ~60s) as named constants —
  budgeted per RPC/operation class, not one global timeout.
- **Also wrap `sshremote.KeyStore.GetIdentity` (Task 3.2.1c) in the same ctx-bounded pattern.**
  pre-mortem.md Failure #3: on a headless `systemd --user` service, `go-keyring`'s Linux Secret
  Service backend can hang on a D-Bus unlock prompt that never appears (no user session to
  satisfy it) — Task 4.2.1c calls `GetIdentity` *before* `SSHRunner.Dial` even starts, so a
  keychain hang wedges `CreateSession` indefinitely even with this task's SSH-side timeout in
  place. Same technique: race the `go-keyring` call against `ctx.Done()` in a goroutine, timeout
  budget ~5s (config/credential reads are not long-running operations).
- Files: `session/tmux/ssh_runner.go`, `session/sshremote/keystore.go`

##### Task 2.1.1g: Test — hung dial and hung command are cancelled within their timeout budget (~5 min)
- Against a test listener that accepts-and-stalls (dial case) and a test sshd command that
  sleeps past the budget (`Run` case); assert both return within budget, not indefinitely, and
  that the underlying connection/session is closed afterward (no fd/goroutine leak).
- Files: `session/tmux/ssh_runner_test.go`

#### Story 2.1.2: Reconnect/backoff for `SSHRunner`
**As a** maintainer, **I want** `SSHRunner` to retry a dropped connection with exponential
backoff and jitter, scoped per remote host, **so that** a flaky network doesn't hammer the
remote sshd or busy-loop the local process.
**Acceptance Criteria**:
- After a connection drop, `SSHRunner` retries with increasing backoff and a max-retry
  circuit-open state, not a tight retry loop.
  - *Given* an `SSHRunner` whose underlying TCP connection is severed, *When* the next `Run`
    call is attempted, *Then* it retries with exponential backoff (not immediate retry) and
    surfaces a distinct "circuit open" error after the configured max-retry threshold, without
    blocking the caller indefinitely.
**Files**: `session/tmux/ssh_runner.go`, `session/tmux/ssh_runner_backoff.go` (new)

##### Task 2.1.2a: Add backoff/circuit-open state per `SSHRunner` (~5 min)
- Reuse the shape of `executor/circuit_breaker.go`'s open/half-open/closed state machine,
  scoped per `SSHRunner` instance (i.e. per remote host, not global).
- Files: `session/tmux/ssh_runner_backoff.go`

##### Task 2.1.2b: Wire backoff into `Run`/`Start`'s dial-retry path (~5 min)
- On connection-dead detection (`ssh.Client.Wait()` returning), route reconnect attempts
  through the backoff state machine before retrying.
- Files: `session/tmux/ssh_runner.go`

##### Task 2.1.2c: Unit test backoff timing and circuit-open behavior (~5 min)
- Table-driven test asserting increasing intervals and a terminal circuit-open state after N
  failures.
- Files: `session/tmux/ssh_runner_backoff_test.go`

### Epic 2.2: `RemoteWorktreeOps`

**Goal**: Remote-host git worktree creation/init, executed via `CommandRunner`, mirroring
`ops.go`'s function shape.

#### Story 2.2.1: Remote `git worktree add`/`git init` over `CommandRunner`
**As a** user, **I want** worktree creation to run on the remote host when a remote target is
configured, **so that** the worktree, its files, and subsequent git operations all live on
that host, not locally.
**Acceptance Criteria**:
- `RemoteWorktreeOps.CreateWorktree` executes `git worktree add` via the injected
  `CommandRunner` and returns the same result shape as the local `GitWorktree` equivalent.
  - *Given* an `SSHRunner` connected to a remote host with an existing bare repo at
    `/srv/repos/foo.git`, *When* `RemoteWorktreeOps.CreateWorktree(ctx, "feature-x")` is
    called, *Then* a new worktree directory exists on the remote host under the configured
    `base_path`, verified via a follow-up `runner.Run(ctx, "test", "-d", path)`.
- `base_path` existence is checked before worktree creation begins, with a distinct error
  class from "remote unreachable."
  - *Given* an `SSHRunner` connected to a reachable host whose configured `base_path` does not
    exist, *When* `CreateWorktree` is called, *Then* it returns `ErrRemoteBasePathMissing`
    (not a generic git error), verified via `runner.Run(ctx, "test", "-d", basePath)` failing
    before any `git` command runs.
**Files**: `session/git/remote_worktree.go` (new), `session/git/remote_worktree_test.go` (new)

##### Task 2.2.1a: Define `RemoteWorktreeOps` type + `base_path` check (~5 min)
- New type taking a `tmux.CommandRunner`; `CreateWorktree` first runs `test -d <base_path>`
  via the runner, returning `ErrRemoteBasePathMissing` on failure.
- The check is advisory only — a TOCTOU race against the subsequent `git worktree add` (Task
  2.2.1b) is possible (e.g. `base_path` removed/unmounted in between); it exists to give a
  fast, distinguishable error for the common case, not as a guarantee. If the race is lost, the
  real error surfaces from `git worktree add` itself.
- Files: `session/git/remote_worktree.go`

##### Task 2.2.1b: Implement `git worktree add` execution (~5 min)
- Shell `git worktree add <path> <branch>` via `runner.Run`, mirroring `ops.go`'s local
  worktree-add function's argument construction.
- Files: `session/git/remote_worktree.go`

##### Task 2.2.1c: Implement `git init` path for new-project-on-remote (~4 min)
- Mirrors `session.SessionTypeNewProject`'s local init flow, shelled remotely.
- Files: `session/git/remote_worktree.go`

##### Task 2.2.1d: Unit tests against a test sshd + tmp remote repo fixture (~5 min)
- Files: `session/git/remote_worktree_test.go`

##### Task 2.2.1e: Implement `RemoteWorktreeOps.RemoveWorktree` — best-effort compensating cleanup (~4 min)
- `git worktree remove <path> --force` via the runner, mirroring `CreateWorktree`'s argument
  construction. Consumed by Task 4.2.1e's partial-failure compensating action
  (adversarial-review.md Blocker 3) — if remote tmux setup fails after `CreateWorktree`
  succeeded, or the SSH connection drops between the two steps, the orchestrating caller
  attempts this before surfacing the original error.
- Files: `session/git/remote_worktree.go`

### Epic 2.3: Remote tmux session lifecycle

**Goal**: `EnsureServerRunning`/session-creation/existence-check logic works against a remote
tmux server reached via `SSHRunner`, handling the nested-tmux/`$TERM` pitfalls.

#### Story 2.3.1: Remote-safe tmux command construction
**As a** maintainer, **I want** every remote tmux invocation to explicitly unset `$TMUX` and
force a known-good `$TERM`, **so that** nested-tmux misdetection and terminfo mismatches
(`research/pitfalls.md` §2) don't silently break remote sessions.
**Acceptance Criteria**:
- Remote tmux commands are wrapped with `env -u TMUX TERM=xterm-256color tmux ...` (or
  equivalent) rather than relying on SSH env passthrough.
  - *Given* a remote host whose sshd has `AcceptEnv` disabled (the common default) and whose
    default `$TERM` differs from the local shell's, *When* a session is created against that
    remote, *Then* the remote tmux server negotiates `xterm-256color` terminfo (verified via
    `tmux display -p '#{client_termname}'` over the same runner), not the remote's own default.
**Files**: `session/tmux/tmux.go`, `session/tmux/remote_env.go` (new)

##### Task 2.3.1a: Add `wrapRemoteCommand` helper (~4 min)
- New helper prefixing remote-bound tmux commands with `env -u TMUX TERM=xterm-256color`.
- Files: `session/tmux/remote_env.go`

##### Task 2.3.1b: Apply wrapper at `SSHRunner.Run`/`Start` call sites in `tmux.go` (~5 min)
- Only when `t.runner.IsRemote()` is true — the single mechanism (per architecture-review.md
  Blocker 1), not a `t.runner` type-switch — local commands are unaffected.
- Files: `session/tmux/tmux.go`

##### Task 2.3.1c: Unit test wrapper output against known inputs (~3 min)
- Files: `session/tmux/remote_env_test.go`

#### Story 2.3.2: Existence check before remote session create (avoid duplicate on reconnect)
**As a** maintainer, **I want** an explicit `tmux has-session` existence check before
attempting `tmux new-session` on a remote host, **so that** an SSH channel drop treated as
"command failed" doesn't spawn a duplicate remote tmux session (`research/pitfalls.md` §1).
**Acceptance Criteria**:
- Remote session creation always checks existence first, mirroring the local
  `DoesSessionExist()` polling pattern.
  - *Given* a remote tmux session that already exists (created by a prior connection attempt
    that then dropped), *When* the local coordinator retries session creation against the
    same remote+name, *Then* it attaches to the existing session (`new-session -A`) instead of
    creating a second one, verified by `tmux list-sessions` on the remote showing exactly one
    matching session.
**Files**: `session/tmux/tmux.go`

##### Task 2.3.2a: Add remote existence check ahead of remote `new-session` (~5 min)
- Reuse the existing `hasCtx`/`has-session` call shape (line 631) via `t.runner.Run`, gating
  the subsequent `new-session -A` call.
- Files: `session/tmux/tmux.go`

##### Task 2.3.2b: Integration test: retry after simulated channel drop (~5 min)
- Against a test sshd, kill the `*ssh.Client` mid-create, retry, assert exactly one remote
  tmux session exists afterward.
- Files: `session/tmux/tmux_remote_test.go` (new)

---

## Phase 3: Credential Storage + Config

### Epic 3.1: `RemoteConfig` in `config.json`

**Goal**: A named list of configured remotes, persisted in `config.json`, with no secret
material stored there.

#### Story 3.1.1: `Remotes []RemoteConfig` field on `config.Config`
**As a** user, **I want** to register a remote host once (name, host, user, base path,
identity reference) and have it persist across restarts, **so that** I don't re-enter
connection details for every session I create against it. *(Covers requirements.md AC1.)*
**Acceptance Criteria**:
- A remote can be registered with name, host (`user@host`), base path, and SSH identity
  reference, with no secret material in `config.json`.
  - *Given* a fresh `config.json` with no `remotes` key, *When* a `RemoteConfig{Name:
    "prod-box", Host: "prod.example.com", User: "tyler", BasePath: "/srv/workspaces",
    IdentityRef: "ssh-key:prod-box"}` is saved via `config.Save`, *Then* the resulting
    `config.json` contains a `remotes` array with those exact fields and **no** `private_key`,
    `passphrase`, or PEM-shaped string value anywhere in the file (verified by a test asserting
    `strings.Contains(rawJSON, "BEGIN") == false`).
**Files**: `config/config.go`, `config/types.go`, `config/config_test.go`

##### Task 3.1.1a: Define `RemoteConfig` struct (~4 min)
- New struct in `config/types.go`: `Name`, `Host`, `User`, `BasePath`, `IdentityRef` (all
  `string`), each field's doc comment stating what it is and is not (no key material).
- Files: `config/types.go`

##### Task 3.1.1b: Add `Remotes []RemoteConfig` field to `Config` (~3 min)
- `json:"remotes,omitempty"`, placed alongside `SessionDefaults`/`Notifications` per the
  existing flat-list-of-named-things pattern (`config/config.go:284-287`).
- Files: `config/config.go`

##### Task 3.1.1c: Round-trip save/load test + no-plaintext-secret assertion (~5 min)
- Files: `config/config_test.go`

##### Task 3.1.1d: Add `RemoteConfig` lookup-by-name helper (~4 min)
- `Config.RemoteByName(name string) (*RemoteConfig, bool)`, used by session creation (Phase 4)
  and Settings UI validation (Phase 6).
- Files: `config/config.go`

### Epic 3.2: `sshremote.KeyStore` — keychain-backed identity storage

**Goal**: SSH private key/passphrase material lives in the OS keychain, mirroring
`github/keychain.go`'s proven pattern, keyed per remote (not one shared key).

#### Story 3.2.1: Keychain-backed SSH identity storage
**As a** user, **I want** my SSH private key/passphrase stored in the OS keychain rather than
plaintext in `config.json`, **so that** a leaked config file or support bundle doesn't expose
my SSH credentials. *(Covers requirements.md AC1's "no secrets stored in plaintext"
sub-clause.)*
**Acceptance Criteria**:
- Storing an identity for a remote writes to the OS keychain (via `zalando/go-keyring`), not
  to any file under `~/.stapler-squad/`.
  - *Given* a remote named `"prod-box"` with private key bytes `keyPEM`, *When*
    `sshremote.KeyStore.SetIdentity("prod-box", keyPEM)` is called, *Then*
    `keyring.Get("stapler-squad-ssh", "ssh-key:prod-box")` returns `keyPEM`, and no file under
    `~/.stapler-squad/` contains that byte sequence (verified by a test that greps the config
    dir after the call).
- Concurrent Get/Set/Delete calls are serialized behind a mutex, per the existing
  `github/keychain.go` discipline.
  - *Given* 10 goroutines concurrently calling `SetIdentity`/`GetIdentity` for different remote
    names, *When* run under `go test -race`, *Then* no data race is reported.
**Files**: `session/sshremote/keystore.go` (new), `session/sshremote/keystore_test.go` (new)

##### Task 3.2.1a: Define `sshremote` package + `keychainService`/key-prefix constants (~3 min)
- `keychainService = "stapler-squad-ssh"`, `keyPrefix = "ssh-key:"`, distinct from
  `github/keychain.go`'s `"github-token:"` namespace (per `research/stack.md` §3 and
  `research/pitfalls.md` §2's "new key namespace, not reusing github-token keys" guidance).
- Files: `session/sshremote/keystore.go`

##### Task 3.2.1b: Implement `keyringMu`-wrapped Get/Set/Delete (~5 min)
- Mirror `github/keychain.go`'s exact `keyringGet`/`keyringSet`/`keyringDelete` wrapper shape,
  with its own `sync.Mutex` (not shared with `github`'s — different package, different
  contention domain).
- Files: `session/sshremote/keystore.go`

##### Task 3.2.1c: Implement `KeyStore.SetIdentity`/`GetIdentity`/`DeleteIdentity` (~5 min)
- Keyed by `keyPrefix + remoteName`; `SetIdentity` accepts either raw private key bytes or a
  passphrase (two logical value kinds under one key namespace, encoded via a small tagged
  JSON envelope so `GetIdentity` can distinguish them).
- Files: `session/sshremote/keystore.go`

##### Task 3.2.1d: Fail-loud test for on-disk fallback + hang case (~6 min)
- Even though there's no primary on-disk fallback (per Pattern Decisions), assert that *if*
  `go-keyring` returns "no backend available," the code fails loudly (per
  `research/build-vs-buy.md` §3) rather than silently writing to disk.
- **Also test the hang case, not just the immediate-error case** (pre-mortem.md Failure #3):
  simulate a `go-keyring` call that never returns (headless Secret Service D-Bus unlock prompt
  with no session to answer it) and assert `GetIdentity` returns a context-deadline error at
  Task 2.1.1f's ~5s budget rather than blocking `CreateSession` forever.
- Files: `session/sshremote/keystore_test.go`

##### Task 3.2.1e: Concurrency test (`go test -race`) (~4 min)
- Files: `session/sshremote/keystore_test.go`

#### Story 3.2.2: Per-remote keypair generation at onboarding
**As a** user, **I want** Stapler Squad to generate a distinct SSH keypair per remote host
rather than reusing one key everywhere, **so that** compromising one remote's credential
doesn't grant access to every other configured remote (`research/pitfalls.md` §3).
**Acceptance Criteria**:
- Adding a new remote via the (Phase 6) onboarding flow generates a fresh Ed25519 keypair,
  stores the private key via `KeyStore.SetIdentity`, and returns the public key text for the
  user to install in the remote's `authorized_keys` (with the ADR-004 recommended
  `command=`/`restrict,pty` line, not just the bare public key).
  - *Given* two remotes `"box-a"` and `"box-b"` onboarded in sequence, *When* their identities
    are read back via `KeyStore.GetIdentity`, *Then* the two private keys are byte-distinct.
**Files**: `session/sshremote/keygen.go` (new), `session/sshremote/keygen_test.go` (new)

##### Task 3.2.2a: Implement Ed25519 keypair generation (~4 min)
- `ssh.NewSignerFromKey`-compatible keypair via `crypto/ed25519` + `golang.org/x/crypto/ssh`
  marshaling helpers.
- Files: `session/sshremote/keygen.go`

##### Task 3.2.2b: Format the recommended `authorized_keys` line (ADR-004) (~4 min)
- `command="<wrapper>",restrict,pty <public-key-text>` string builder, with the wrapper script
  path left as a documented placeholder per ADR-004's "recommendation, not enforcement" scope.
- Files: `session/sshremote/keygen.go`

##### Task 3.2.2c: Unit test uniqueness across remotes (~3 min)
- Files: `session/sshremote/keygen_test.go`

### Epic 3.3: `KnownHostsStore` + TOFU host-key confirm flow

**Goal**: An app-managed known-hosts store backing `knownhosts.New()`, with a backend RPC
surface the Settings UI (Phase 6) can drive for the "Trust and connect" confirmation.

#### Story 3.3.1: App-managed `KnownHostsStore`
**As a** maintainer, **I want** a `known_hosts`-equivalent store scoped to Stapler Squad (not
the user's personal `~/.ssh/known_hosts`), **so that** host-key trust decisions are made
explicitly through the web UI, not silently inherited from unrelated system state.
**Acceptance Criteria**:
- `KnownHostsStore.Verify(host, key)` returns `ErrUnknownHostKey` for a never-seen host, and
  succeeds silently for a previously-trusted host+key pair.
  - *Given* an empty `KnownHostsStore` at `~/.stapler-squad/ssh_known_hosts`, *When*
    `Verify("prod.example.com", someHostKey)` is called, *Then* it returns
    `ErrUnknownHostKey` wrapping a computed SHA256 `HostKeyFingerprint`.
  - *Given* a `KnownHostsStore` where `Trust("prod.example.com", someHostKey)` was previously
    called, *When* `Verify("prod.example.com", someHostKey)` is called again, *Then* it
    returns nil.
**Files**: `session/sshremote/known_hosts.go` (new), `session/sshremote/known_hosts_test.go` (new)

##### Task 3.3.1a: Implement `KnownHostsStore` backed by `knownhosts.New()` (~5 min)
- File-backed at `~/.stapler-squad/ssh_known_hosts`, using
  `golang.org/x/crypto/ssh/knownhosts`'s standard `HostKeyCallback` shape.
- Files: `session/sshremote/known_hosts.go`

##### Task 3.3.1b: Implement `Trust`/`Verify`/fingerprint computation (~5 min)
- SHA256 fingerprint format matching OpenSSH's `SHA256:<base64>` display convention.
- Files: `session/sshremote/known_hosts.go`

##### Task 3.3.1c: Unit tests: unknown host, trusted host, key-mismatch cases (~5 min)
- Files: `session/sshremote/known_hosts_test.go`

#### Story 3.3.2: Backend RPC for Test Connection + Trust/Cancel
**As a** user, **I want** a "Test connection" action in Settings that surfaces an unknown host
key's fingerprint for me to explicitly trust or cancel, **so that** host-key verification
happens once per remote, at configuration time, not silently or per-session.
**Acceptance Criteria**:
- A `TestRemoteConnection` RPC attempts to dial the remote; on an unknown host key, it returns
  a structured response containing the fingerprint (not an opaque error) for the frontend to
  render as a confirm dialog; a follow-up `TrustRemoteHostKey` RPC commits the trust decision.
  - *Given* a newly-configured remote whose host key isn't yet trusted, *When*
    `TestRemoteConnection` is called, *Then* the response's `host_key_unknown` field is true
    and `fingerprint` is populated, and no session-creation capability is unlocked for that
    remote yet.
  - *Given* the user then calls `TrustRemoteHostKey` with that fingerprint, *When*
    `TestRemoteConnection` is called again, *Then* it succeeds (`host_key_unknown` is false).
**Files**: `proto/session/v1/remote.proto` (new), `server/services/remote_service.go` (new)

##### Task 3.3.2a: Define `TestRemoteConnection`/`TrustRemoteHostKey` RPCs (~5 min)
- New proto file `proto/session/v1/remote.proto` (kept separate from `session.proto` since
  it's a distinct resource, not a `CreateSessionRequest` extension) with request/response
  messages including `fingerprint string`, `host_key_unknown bool`.
- Files: `proto/session/v1/remote.proto`

##### Task 3.3.2b: Run `make proto-gen` (~2 min)
- Regenerates Go + TS bindings.
- Files: `session/gen/session/v1/remote_pb.go` (generated), `web-app/src/gen/session/v1/remote_pb.ts` (generated)

##### Task 3.3.2c: Implement `RemoteService.TestRemoteConnection` handler (~5 min)
- Dials via `SSHRunner`/`KnownHostsStore.Verify`, mapping `ErrUnknownHostKey` to the
  structured response shape rather than a raw ConnectRPC error.
- Files: `server/services/remote_service.go`

##### Task 3.3.2d: Implement `RemoteService.TrustRemoteHostKey` handler (~4 min)
- Calls `KnownHostsStore.Trust`, requires the caller to pass back the exact fingerprint shown
  (defense against blindly trusting a different key than the one displayed).
- Files: `server/services/remote_service.go`

##### Task 3.3.2e: Register `RemoteService` in `server/server.go` (~3 min)
- Files: `server/server.go`

##### Task 3.3.2f: Handler unit tests (unknown host → trust → verified) (~5 min)
- Files: `server/services/remote_service_test.go`

---

## Phase 4: Session-Creation Registry Integration (7 Touchpoints) + Terminal Streaming Over SSH

### Epic 4.1: Proto changes

**Goal**: Add `RemoteTarget` to `CreateSessionRequest` without touching the `SessionType`
enum, per ADR-001.

#### Story 4.1.1: `RemoteTarget` nested message on `CreateSessionRequest`
**As a** developer, **I want** `CreateSessionRequest` to carry an optional `RemoteTarget`,
**so that** session creation can specify a remote by name without a new `SessionType` value.
*(Covers requirements.md AC2's "results in a worktree and tmux session created on the remote
host over SSH" — proto layer.)*
**Acceptance Criteria**:
- `CreateSessionRequest.remote` is field 28 (next available after `alias_name = 27`), a
  nested `RemoteTarget { string remote_name = 1; }` message (referencing a `RemoteConfig` by
  name, not carrying host/user/key inline).
  - *Given* a `CreateSessionRequest` with `remote.remote_name = "prod-box"` and
    `session_type = SESSION_TYPE_NEW_WORKTREE`, *When* the request is round-tripped through
    proto marshal/unmarshal, *Then* both fields are preserved independently (proving they
    compose rather than conflict).
**Files**: `proto/session/v1/session.proto`

##### Task 4.1.1a: Add `RemoteTarget` message + `remote` field 28 (~4 min)
- Files: `proto/session/v1/session.proto`

##### Task 4.1.1b: Run `make proto-gen` (~2 min)
- Files: `session/gen/session/v1/session_pb.go` (generated), `web-app/src/gen/session/v1/session_pb.ts` (generated)

### Epic 4.2: Go handler wiring

**Goal**: `resolveRemoteTarget` resolves the request's `remote` field to a `session.RemoteTarget`
and constructs the right `CommandRunner` for the session's `TmuxSession`/`GitWorktree`.

#### Story 4.2.1: `resolveRemoteTarget` + mode-specific block + `Instance` wiring
**As a** user, **I want** creating a session with a remote target to actually run worktree
creation and tmux startup on that remote host, **so that** the session's process lives there,
not on my local machine. *(Covers requirements.md AC2 fully.)*
**Acceptance Criteria**:
- When `req.Msg.Remote.RemoteName` is set, `CreateSession` resolves the named `RemoteConfig`,
  builds an `SSHRunner` for it, and atomically constructs a `RemoteExecutionTarget{Target,
  Runner}` — never sets a `RemoteTarget` and a runner as two independent steps — which the
  session's `TmuxSession`/`GitWorktree` are then built from via `.Runner()` instead of
  `LocalRunner{}` (architecture-review.md Blocker 1).
  - *Given* a `CreateSessionRequest` with `remote.remote_name = "prod-box"`,
    `session_type = SESSION_TYPE_NEW_WORKTREE`, and `branch = "feature-x"`, *When*
    `CreateSession` is called, *Then* the resulting `session.Instance`'s `ExecutionTarget.IsRemote()`
    is `true`, its `TmuxSession`/`GitWorktree` are backed by the same `SSHRunner` dialed to
    `prod-box`'s configured host, and a `tmux has-session` check against that host (via a
    second, independent SSH dial in the test) confirms the session exists there — not in the
    local tmux server.
- An unknown `remote_name` returns a distinct, actionable error, not a generic session-creation
  failure.
  - *Given* a `CreateSessionRequest` with `remote.remote_name = "does-not-exist"`, *When*
    `CreateSession` is called, *Then* it returns `connect.CodeInvalidArgument` with a message
    naming the unknown remote, before any worktree/tmux work begins.
- If remote worktree creation succeeds but the subsequent remote tmux session setup fails (or
  the SSH connection drops between the two steps), `CreateSession` attempts a best-effort
  remote `git worktree remove` before surfacing the error, so a failed creation attempt does
  not silently leave an orphaned worktree directory on the remote host (adversarial-review.md
  Blocker 3; full orphan reconciliation-on-connect remains a fast-follow — see Unresolved
  Questions).
  - *Given* a `CreateSessionRequest` with `remote.remote_name = "prod-box"` where
    `RemoteWorktreeOps.CreateWorktree` succeeds but the subsequent remote `tmux new-session`
    call fails, *When* `CreateSession` returns its error to the caller, *Then*
    `RemoteWorktreeOps.RemoveWorktree` has already been attempted against the remote host,
    verified by a test asserting the worktree directory no longer exists on the remote.
  - *Given* the SSH connection drops between `CreateWorktree` succeeding and the tmux-setup step
    starting (so the compensating call itself cannot even be attempted, or also fails), *When*
    `CreateSession` returns its error, *Then* the error message explicitly names the remote path
    as possibly orphaned rather than failing silently — this is a best-effort synchronous
    attempt, not a guarantee.
**Files**: `server/services/session_service.go`, `session/instance.go`

##### Task 4.2.1a: Define `ExecutionTarget` sum type + add single field to `session.CreateOptions`/`Instance` (~5 min)
- New `session/instance.go` sum type `ExecutionTarget` with two constructors — `LocalTarget{}`
  and `RemoteExecutionTarget{Target RemoteTarget, Runner *tmux.SSHRunner}` — exposing
  `IsRemote() bool` and `Runner() tmux.CommandRunner`. `Instance` (and
  `session.CreateOptions`) holds one `ExecutionTarget` field (defaulting to `LocalTarget{}`),
  replacing the previously-planned separate `RemoteTarget *RemoteTarget` field. This closes
  architecture-review.md Blocker 1: a `RemoteTarget` pointer plus whichever `CommandRunner`
  happened to be installed on `TmuxSession`/`GitWorktree` were two independently-settable
  fields with no guarantee they agreed; one field constructed atomically (Task 4.2.1c) makes
  the disagreement state unrepresentable.
- Files: `session/instance.go`

##### Task 4.2.1b: Implement `resolveRemoteTarget(msg, cfg) (*session.RemoteTarget, error)` (~5 min)
- Looks up `cfg.RemoteByName(msg.Remote.RemoteName)`, returns `CodeInvalidArgument` on miss;
  placed alongside `resolveSessionType` in `session_service.go`. Returns the plain data struct
  (name/host/base path) — Task 4.2.1c is what pairs it with a dialed runner into the atomic
  `ExecutionTarget`.
- Files: `server/services/session_service.go`

##### Task 4.2.1c: Construct `SSHRunner` from resolved `RemoteConfig` + `KeyStore`/`KnownHostsStore`, build `RemoteExecutionTarget` (~5 min)
- The mode-specific block (alongside the existing `AutonomousMode`/`OneOff` blocks at
  `session_service.go:1361,1625`): resolves `IdentityRef` via `sshremote.KeyStore`, builds the
  `ssh.ClientConfig` with the `KnownHostsStore` callback, dials (through Task 2.1.1f's
  ctx-bounded path). Wraps the resolved `RemoteTarget` and the dialed `SSHRunner` into a single
  `RemoteExecutionTarget{Target, Runner}` value in the same step — never sets the two fields
  independently.
- Files: `server/services/session_service.go`

##### Task 4.2.1d: Pass the constructed `ExecutionTarget` into `TmuxSession`/`GitWorktree` construction (~5 min)
- Threads the single `ExecutionTarget` through wherever `session.Instance` currently builds its
  `TmuxSession`/`GitWorktree` (`session/instance.go` construction path); those constructors call
  `.Runner()` on it rather than taking a raw `CommandRunner` from a second, separately-tracked
  source — local sessions unaffected (still effectively get `LocalRunner{}` via
  `LocalTarget{}.Runner()`).
- Files: `session/instance.go`

##### Task 4.2.1e: Best-effort remote worktree cleanup on partial-failure (~5 min)
- If `RemoteWorktreeOps.CreateWorktree` succeeds but the subsequent remote tmux session-setup
  step (or the SSH connection itself) fails before `CreateSession` returns, call
  `RemoteWorktreeOps.RemoveWorktree` (Task 2.2.1e) as a best-effort compensating action before
  surfacing the original error. If the compensating call itself fails (e.g. the connection is
  also down), the surfaced error explicitly names the path as possibly orphaned instead of
  failing silently. This is v1's cleanup-on-failure guarantee (adversarial-review.md Blocker
  3); full reconciliation-on-connect for orphans from other causes (crash, `kill -9`, manual
  deletion) is deferred to a fast-follow — see Unresolved Questions.
- Files: `server/services/session_service.go`

##### Task 4.2.1f: Test — partial failure triggers compensating cleanup; connection-drop-during-cleanup surfaces an explicit orphan warning (~5 min)
- Two cases against a test sshd: (1) worktree succeeds, tmux setup fails → assert the remote
  worktree directory no longer exists; (2) worktree succeeds, connection drops before cleanup
  can run → assert the returned error text names the path as possibly orphaned.
- Files: `server/services/session_service_test.go`

##### Task 4.2.1g: Handler test — remote target resolves and dials (~5 min)
- Against a test sshd; asserts `CodeInvalidArgument` on unknown remote name.
- Files: `server/services/session_service_test.go`

### Epic 4.3: Frontend touchpoints 5-7

**Goal**: `Omnibar.tsx`, `OmnibarCreationPanel.tsx`, `OmnibarContext.tsx`, and
`useSessionService.ts` thread a `remote` selection through session creation, composing with
the existing `sessionType` union rather than replacing it (per ADR-001, mirroring the
`autonomousMode` flag pattern).

#### Story 4.3.1: `Omnibar.tsx` — `remoteName` form field + `canSubmit`/`handleSubmit`
**As a** user, **I want** a "Remote host" selector in the session creation Omnibar that
composes with whichever session type I've picked, **so that** I can create a `new_worktree` or
`existing_worktree` (etc.) session against a remote host without a separate creation flow.
*(Covers requirements.md AC2's dashboard UI clause.)*
**Acceptance Criteria**:
- `OmnibarFormState` gains `remoteName?: string`, defaulting to `undefined` (local).
  `canSubmit` does not require it; `handleSubmit` passes it through only when set.
  - *Given* the Omnibar with `sessionType: "new_worktree"` and `remoteName: "prod-box"`,
    *When* the user submits, *Then* the `CreateSessionRequest` sent includes both
    `session_type: SESSION_TYPE_NEW_WORKTREE` and `remote: { remote_name: "prod-box" }`.
  - *Given* the Omnibar with `remoteName` left unset (default), *When* the user submits,
    *Then* the `CreateSessionRequest` omits the `remote` field entirely — today's local
    behavior is byte-identical to pre-change.
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.3.1a: Add `remoteName?: string` to `OmnibarFormState` (~3 min)
- Files: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.3.1b: Thread `remoteName` into `handleSubmit`'s request construction (~4 min)
- Alongside the existing `autonomousMode: isAutonomous ? true : undefined` pattern at line
  ~1122.
- Files: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.3.1c: Add `dispatch.test.ts`-style unit test for the remote-composed submit path (~5 min)
- Per `.claude/rules/feature-testing-registry.md`, verify no existing action-type test breaks
  and the new field passes through correctly.
- Files: `web-app/src/lib/omnibar/actions/dispatch.test.ts`

#### Story 4.3.2: `OmnibarCreationPanel.tsx` — remote selector control
**As a** user, **I want** a dropdown of my configured remotes (defaulting to "This machine")
in the creation panel, **so that** picking a remote is a single, discoverable step, not a
fifth session-type radio option (ADR-001).
**Acceptance Criteria**:
- The remote selector renders only when ≥1 remote is configured (per `research/ux.md` §2);
  it appears as a control composing with `SESSION_TYPES`, not inside it.
  - *Given* zero configured remotes, *When* `OmnibarCreationPanel` renders, *Then* no remote
    selector is present in the DOM (verified via `queryByTestId("remote-selector")` returning
    null).
  - *Given* ≥1 configured remote, *When* `OmnibarCreationPanel` renders with
    `sessionType: "existing_worktree"` selected, *Then* the remote selector is visible and,
    when a remote is chosen, the existing `existing_worktree`-specific fields remain visible
    and functional (composability, not replacement).
**Files**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`, `web-app/src/components/sessions/OmnibarCreationPanel.css.ts` (new or extend)

##### Task 4.3.2a: Add remote selector control (conditional render) (~5 min)
- New control near the "Autonomous mode" checkbox (~line 462), gated on `remotes.length > 0`
  (remotes list passed as a prop, sourced from Phase 6's `remotesSlice`).
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 4.3.2b: vanilla-extract styles for the selector (~4 min)
- Per `.claude/rules/css-architecture.md` — no new `.module.css`; use `.css.ts` with `vars.*`
  tokens.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.css.ts`

##### Task 4.3.2c: Component test — hidden with zero remotes, visible + composable with ≥1 (~5 min)
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.test.tsx`

#### Story 4.3.3: `OmnibarContext.tsx` + `useSessionService.ts` passthrough
**As a** developer, **I want** the `remoteName` form value threaded through
`OmnibarContext`'s `createSession` call and `useSessionService`'s RPC body, **so that** the
value set in the UI actually reaches the backend.
**Acceptance Criteria**:
- `createSession` in `OmnibarContext.tsx` passes `remote: data.remoteName ? { remoteName:
  data.remoteName } : undefined` through to `useSessionService`'s `createSession`, which sets
  it on the ConnectRPC request body.
  - *Given* `OmnibarSessionData.remoteName = "prod-box"`, *When* `createSession` is invoked,
    *Then* the ConnectRPC `CreateSessionRequest` sent over the wire has
    `remote.remote_name === "prod-box"`, verified via a mocked transport asserting on the
    captured request object.
**Files**: `web-app/src/lib/contexts/OmnibarContext.tsx`, `web-app/src/lib/hooks/useSessionService.ts`

##### Task 4.3.3a: Add `remoteName` passthrough in `OmnibarContext.tsx`'s `createSession` call (~4 min)
- Alongside `autonomousMode: data.autonomousMode ?? false` at line ~228.
- Files: `web-app/src/lib/contexts/OmnibarContext.tsx`

##### Task 4.3.3b: Add `remote` field to `useSessionService.ts`'s RPC body construction (~4 min)
- Alongside `autonomousMode: request.autonomousMode ?? false` at line ~277.
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 4.3.3c: Test — mocked transport asserts `remote.remote_name` on the wire (~5 min)
- Files: `web-app/src/lib/hooks/useSessionService.test.ts`

### Epic 4.4: Terminal streaming over SSH

**Goal**: Generalize the PTY/control-mode data source to `io.Reader`/`io.Writer` so a remote
session's terminal streams through the exact same delta protocol, ConnectRPC framing, and
xterm.js consumer as a local session. *(Covers requirements.md AC3 in full.)*

#### Story 4.4.1: Generalize `PtyFactory`/control-mode source to `io.Reader`/`io.Writer`
**As a** user, **I want** a remote session's terminal to behave identically to a local
session's in the xterm.js view, **so that** I cannot tell from the terminal experience alone
that the session is remote (aside from the host badge).
**Acceptance Criteria**:
- `streamViaControlMode` (`server/services/connectrpc_websocket.go`) reads from an
  `io.Reader` sourced from either a local dup'd PTY fd or an `SSHRunner.Start`-backed remote
  session's stdout — the streaming/flow-control/proto-framing code above that boundary is
  unchanged.
  - *Given* a remote session created against `prod-box`, *When* a client connects via
    `StreamTerminal` and types a character, *Then* the character appears in the remote tmux
    pane (verified via `tmux capture-pane` on the remote) and the resulting output byte(s)
    arrive back at the xterm.js client as a `TerminalData` message — the same round-trip shape
    a local session produces, differing only in transport underneath.
- Resize (`WindowChange`) is sent unconditionally on reconnect for remote sessions, matching
  the existing local reconnect-resize behavior (`connectrpc_websocket.go:1612`), because delta
  state may be stale after a network partition (`research/pitfalls.md` §1).
  - *Given* a remote session whose SSH connection drops and reconnects, *When* the client
    reattaches, *Then* an unconditional `WindowChange`/SIGWINCH-equivalent is sent to the
    remote tmux process regardless of whether the browser viewport size changed.
**Files**: `session/tmux/pty.go`, `server/services/connectrpc_websocket.go`, `server/services/session_service.go`

##### Task 4.4.1a: Generalize `PtyFactory` interface to accept a `CommandRunner` (~5 min)
- `PtyFactory.Start`/`StartWithSize` gain an `SSHPtyFactory` counterpart (or the interface is
  extended to be satisfied by an `SSHRunner`-backed type) returning `io.ReadWriteCloser`-shaped
  access instead of assuming `*os.File`.
- Files: `session/tmux/pty.go`

##### Task 4.4.1b: Implement `SSHRunner`-backed PTY session with `RequestPty`+`Shell` (~5 min)
- `session.RequestPty("xterm-256color", rows, cols, modes)` then `session.Shell()`, exposing
  `StdoutPipe`/`StdinPipe`/`WindowChange`.
- Files: `session/tmux/ssh_runner.go`

##### Task 4.4.1c: Update `streamViaControlMode`'s data-source construction to accept either source (~5 min)
- Swap the hardcoded local-PTY-fd assumption for an `io.Reader` parameter, selected based on
  whether the session's `Instance.RemoteTarget` is set.
- Files: `server/services/connectrpc_websocket.go`

##### Task 4.4.1d: Update `StreamTerminal` handler's PTY read goroutine for the `io.Reader` abstraction (~5 min)
- `server/services/session_service.go:2230` onward — same substitution for the raw-PTY
  fallback path.
- Files: `server/services/session_service.go`

##### Task 4.4.1e: Unconditional resize-on-reconnect for the remote path (~4 min)
- Mirror the existing local unconditional-SIGWINCH-on-reconnect comment/behavior at
  `connectrpc_websocket.go:1612`, calling `WindowChange` instead of `kill -WINCH`.
- Files: `server/services/connectrpc_websocket.go`

##### Task 4.4.1f: Disable SSH compression on the terminal data channel (~3 min)
- Explicit `ssh.ClientConfig` setting avoiding `zlib@openssh.com`, per `research/pitfalls.md`
  §1's escape-sequence-boundary-corruption risk — interactive traffic is latency-sensitive,
  not bandwidth-bound.
- Files: `session/tmux/ssh_runner.go`

##### Task 4.4.1g: Integration test — remote terminal round-trip via test sshd + tmux (~5 min)
- Files: `server/services/connectrpc_websocket_remote_test.go` (new)

---

## Phase 5: Approval Round-Trip Over Multiplexed Channel

### Epic 5.1: `RemoteApprovalRelay`

**Goal**: A second channel over the same `*ssh.Client` carrying approval-request payloads from
the remote agent to the local `ExternalApprovalMonitor`, per ADR-003.

#### Story 5.1.1: Remote-side socket + local `direct-streamlocal` channel read
**As a** user, **I want** approval requests raised by a remote-running agent to reach my local
dashboard's existing approval UI, **so that** I can approve/deny remote agent actions the same
way I do for local sessions. *(Covers requirements.md AC4, request-direction half.)*
**Acceptance Criteria**:
- The remote agent process (via its injected hooks) writes an approval-request payload to a
  Unix domain socket on the remote host; `RemoteApprovalRelay` reads it via a
  `direct-streamlocal@openssh.com` channel over the existing `*ssh.Client`, and forwards it
  into `ExternalApprovalMonitor` using the session's `socketPath`-keyed model.
  - *Given* a remote session with `RemoteApprovalRelay` attached, *When* a test payload
    matching the `detection.ApprovalRequest` shape is written to the remote Unix socket,
    *Then* `ExternalApprovalMonitor.GetPendingApprovals(sessionKey)` returns that request
    within a bounded poll interval, and it renders in the local approval UI identically to a
    local-session approval.
**Files**: `session/external_approval.go`, `session/sshremote/approval_relay.go` (new)

##### Task 5.1.1a: Define `RemoteApprovalRelay` type + remote-socket path convention (~4 min)
- Fixed remote path convention (e.g. `<base_path>/.stapler-squad-approval.sock`) documented
  and passed to the remote agent's hook injection (Epic 5.2).
- Files: `session/sshremote/approval_relay.go`

##### Task 5.1.1b: Implement `direct-streamlocal@openssh.com` channel dial via `ssh.Client` (~5 min)
- `golang.org/x/crypto/ssh`'s `Client.Dial` for Unix-domain-forwarded channels (the same
  primitive `ssh -L`/`-R` use internally, per `research/stack.md` §5).
- Files: `session/sshremote/approval_relay.go`

##### Task 5.1.1c: Parse relayed payloads into `detection.ApprovalRequest` and forward into `ExternalApprovalMonitor` (~5 min)
- New method on `ExternalApprovalMonitor` (or a small adapter) accepting relay-sourced
  requests keyed the same way local `socketPath`-based ones are.
- Files: `session/external_approval.go`

##### Task 5.1.1d: Per-session bearer credential for the relay payload (~5 min)
- Per `research/pitfalls.md` §4: a per-session, short-TTL bearer token minted at session
  creation, required in the relayed payload, rejecting anything else — so the relay isn't an
  open channel any remote-host process could forge traffic into.
- Files: `session/sshremote/approval_relay.go`

##### Task 5.1.1e: Integration test — write to remote socket, assert local pending-approval visibility (~5 min)
- Files: `session/sshremote/approval_relay_test.go`

#### Story 5.1.2: Relay channel re-establishment on reconnect (pre-mortem.md Failure #2, P1)
**As a** user, **I want** a pending approval request to survive a transient network blip,
**so that** a remote agent doesn't silently stall forever waiting on an approval channel that
died with the connection. ADR-003 frames "relay liveness = terminal-stream liveness" as a
benefit, but ties a real correctness gap to it if nothing re-opens the relay channel after the
parent `*ssh.Client` reconnects: without this, an agent blocked reading its remote-side approval
socket at the exact moment the connection drops is left blocked forever — no error, just
silence, until the user notices the agent has been idle far longer than expected.
**Acceptance Criteria**:
- When the parent `*ssh.Client` (Task 2.1.0a's pooled connection) re-dials after a drop,
  `RemoteApprovalRelay` re-opens its `direct-streamlocal@openssh.com` channel against the new
  connection before any new approval traffic is expected to flow.
  - *Given* a `RemoteApprovalRelay` attached to a session, *When* the underlying `*ssh.Client`
    is killed and reconnects, *Then* a fresh `direct-streamlocal` channel is opened without
    requiring the session to be recreated, verified by writing a test payload to the remote
    socket post-reconnect and observing it reach `ExternalApprovalMonitor`.
- A request the remote agent was blocked on when the connection dropped is not silently lost:
  either the agent's blocking read itself retries against a freshly-opened socket connection
  (agent-side responsibility, documented as a constraint on the hook script), or the relay
  buffers and redelivers the in-flight request once the channel reopens — one behavior, chosen
  explicitly, not left unspecified.
  - *Given* a pending approval request in flight when the connection drops, *When* the
    connection reconnects and the relay channel re-opens, *Then* the local approval UI still
    shows the request as pending (not silently dropped), and approving it still unblocks the
    remote agent.
**Files**: `session/sshremote/approval_relay.go`, `session/sshremote/approval_relay_test.go`

##### Task 5.1.2a: Subscribe `RemoteApprovalRelay` to the pooled `*ssh.Client`'s reconnect event (~5 min)
- Files: `session/sshremote/approval_relay.go`

##### Task 5.1.2b: Re-open the `direct-streamlocal` channel on reconnect (~5 min)
- Files: `session/sshremote/approval_relay.go`

##### Task 5.1.2c: Decide and implement in-flight-request survival behavior (agent-side retry vs. relay buffering) (~5 min)
- Files: `session/sshremote/approval_relay.go`

##### Task 5.1.2d: Integration test — connection drops with a pending approval in flight, then reconnects (~5 min)
- Files: `session/sshremote/approval_relay_test.go`

### Epic 5.2: Hook callback URL fix for remote sessions

**Goal**: Remote-injected `curl` hooks route through the relay instead of the remote host's own
(unreachable) `localhost:8543`, per ADR-003's consequences.

#### Story 5.2.1: `hookBaseURLFn` override scoped to remote sessions
**As a** user, **I want** the Claude Code hooks injected into a remote session to actually
reach my local dashboard, **so that** approval/notification hooks work identically whether the
session is local or remote.
**Acceptance Criteria**:
- For a remote session, `InjectHookConfig` builds hook commands that write to the relay's
  remote-side Unix socket (e.g. via a small `curl --unix-socket <path> ...` or an equivalent
  local write) instead of `curl 'http://localhost:8543/...'`.
  - *Given* a remote session, *When* `InjectHookConfig` generates the `PermissionRequest` hook
    command, *Then* the generated command targets the relay's remote-side socket path, not
    `localhost:8543`, verified by a string assertion on the generated hook command.
  - *Given* a local session (unaffected by this change), *When* `InjectHookConfig` generates
    the same hook, *Then* the command is byte-identical to pre-Phase-5 behavior
    (`http://localhost:8543/...`).
**Files**: `server/services/hook_injector.go`

##### Task 5.2.1a: Add remote-aware branch to hook command construction (~5 min)
- `InjectHookConfig`/`InjectHooksConfig` gains a parameter (or reads `Instance.RemoteTarget`)
  to select between the existing `hookBaseURLFn()`-based URL and a relay-socket-targeted
  command.
- Files: `server/services/hook_injector.go`

##### Task 5.2.1b: Unit test — remote hook command targets relay, local hook command unchanged (~5 min)
- Files: `server/services/hook_injector_test.go`

### Epic 5.3: Approval response round-trip

**Goal**: The local dashboard's approval response is delivered back to unblock the remote
agent process.

#### Story 5.3.1: Response delivery back to the remote agent
**As a** user, **I want** my approve/deny decision on a remote session's approval request to
actually unblock the remote agent process, **so that** the round trip is complete, not
one-directional. *(Covers requirements.md AC4's response-direction half, completing full AC4
coverage.)*
**Acceptance Criteria**:
- `MarkApprovalHandled` (`session/external_approval.go:295`), when called for a
  relay-sourced approval, writes the response back over the same `direct-streamlocal` channel
  to the remote socket, where the remote agent's hook-side blocking read is waiting.
  - *Given* a pending relay-sourced approval request and a local "approve" action, *When*
    `MarkApprovalHandled(socketPath, requestID, true)` is called, *Then* the remote-side
    process blocked reading the approval socket receives an "approved" response within a
    bounded time, verified by the remote-side test harness's blocking read returning `true`.
**Files**: `session/external_approval.go`, `session/sshremote/approval_relay.go`

##### Task 5.3.1a: Extend `MarkApprovalHandled` to route relay-sourced responses back through the channel (~5 min)
- Files: `session/external_approval.go`

##### Task 5.3.1b: Implement the relay's response-write path (~4 min)
- Files: `session/sshremote/approval_relay.go`

##### Task 5.3.1c: End-to-end test — request → local approve → remote unblocks (~5 min)
- Files: `session/sshremote/approval_relay_test.go`

---

## Phase 6: UI Surfaces (Remote Tab, Host Badge, Connection Status) + Registries

### Epic 6.1: Remotes settings UI

**Goal**: A Settings surface to add, test-connect, and trust remotes — the onboarding flow
Epics 3.2/3.3's backend RPCs exist to support.

#### Story 6.1.1: Add/Test/Trust remote form
**As a** user, **I want** a Settings page listing my configured remotes with an "Add remote"
form (name, host, user, base path) and a "Test connection" action, **so that** I configure
credentials once and reuse them for every future session creation. *(Covers requirements.md
AC1's UI half.)*
**Acceptance Criteria**:
- Submitting the "Add remote" form calls `TestRemoteConnection`; on an unknown host key, a
  confirmation dialog shows the fingerprint with explicit Trust/Cancel actions before the
  remote is saved to `config.json`.
  - *Given* a user filling in name/host/user/base-path for a new remote and clicking "Test
    connection", *When* the target host's key is unrecognized, *Then* a modal dialog renders
    showing the host, port, and SHA256 fingerprint, with "Trust and connect" and "Cancel"
    buttons — the remote is not persisted until "Trust and connect" is clicked.
**Files**: `web-app/src/app/settings/remotes/page.tsx` (new), `web-app/src/app/settings/remotes/RemotesPage.css.ts` (new), `web-app/src/components/settings/AddRemoteForm.tsx` (new), `web-app/src/components/settings/HostKeyTrustDialog.tsx` (new)

##### Task 6.1.1a: Scaffold `RemotesPage` list view (~5 min)
- Files: `web-app/src/app/settings/remotes/page.tsx`

##### Task 6.1.1b: `AddRemoteForm` component (name/host/user/base-path fields) (~5 min)
- Files: `web-app/src/components/settings/AddRemoteForm.tsx`

##### Task 6.1.1c: `HostKeyTrustDialog` (modal, not inline — per `research/ux.md` §1's VS Code precedent) (~5 min)
- `createPortal(..., document.body)` per `.claude/rules/css-architecture.md`'s overlay
  convention.
- Files: `web-app/src/components/settings/HostKeyTrustDialog.tsx`

##### Task 6.1.1d: vanilla-extract styles for the new page/components (~4 min)
- Files: `web-app/src/app/settings/remotes/RemotesPage.css.ts`, `web-app/src/components/settings/AddRemoteForm.css.ts`

##### Task 6.1.1e: Wire form submit → `TestRemoteConnection` → `TrustRemoteHostKey` → save `RemoteConfig` (~5 min)
- Files: `web-app/src/components/settings/AddRemoteForm.tsx`

##### Task 6.1.1f: Display the generated `authorized_keys` recommendation text (ADR-004) (~4 min)
- Copyable text block with the explicit "Stapler Squad cannot verify this was applied"
  caveat.
- Files: `web-app/src/components/settings/AddRemoteForm.tsx`

##### Task 6.1.1g: Component tests — add flow, trust flow, cancel flow (~5 min)
- Files: `web-app/src/components/settings/AddRemoteForm.test.tsx`

### Epic 6.2: Session card host badge + `RemoteConnectionIndicator`

**Goal**: A host badge on remote sessions' cards and a live, accessible connection-status
indicator. *(Covers requirements.md AC5 in full.)*

#### Story 6.2.1: Host badge on `SessionCard.tsx`
**As a** user, **I want** a session card to visibly show which host a remote session is
running on, **so that** I can tell at a glance without opening the session.
**Acceptance Criteria**:
- A remote session's card renders a host badge (mirroring the existing `externalBadge`
  pattern at `SessionCard.tsx:459-469`); a local session's card renders no such badge.
  - *Given* a session with `remoteName: "prod-box"` set, *When* its `SessionCard` renders,
    *Then* a badge with `role="img"` and `aria-label="Running on prod-box"` (or equivalent) is
    present in the DOM.
  - *Given* a session with no `remoteName` (local), *When* its `SessionCard` renders, *Then*
    no host badge is present — a purely local session's card is unchanged from today.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 6.2.1a: Add host badge render, gated on `session.remoteName` (~4 min)
- Adjacent to the `externalBadge` block, following its exact `role="img"`+`aria-label`
  pattern.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 6.2.1b: vanilla-extract style for the host badge (~3 min)
- Files: `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 6.2.1c: Component test — badge present for remote, absent for local (~4 min)
- Files: `web-app/src/components/sessions/SessionCard.test.tsx`

#### Story 6.2.2: `RemoteConnectionIndicator` — accessible, push-driven connection status
**As a** user (including screen-reader users), **I want** a connection status indicator
(connected/reconnecting/disconnected) for each remote session that's announced on state
change, **so that** I know without opening the session whether a remote agent might be stuck
due to network issues, not agent issues.
**Acceptance Criteria**:
- The indicator pairs an icon/color with visible text (never color alone), reusing the
  `SessionCard.tsx:792`-style persistent `aria-live="polite"` region for state-transition
  announcements, distinct from the session's own lifecycle status badge.
  - *Given* a remote session whose `RemoteConnectionState` transitions from `connected` to
    `reconnecting`, *When* the transition occurs, *Then* the card's badge text updates to
    "Reconnecting…" with `aria-label` text, and the persistent `aria-live="polite"` region's
    content updates to announce the change (verified via testing-library's
    `screen.getByRole("status")` content assertion) — without requiring focus to be on the
    card.
  - *Given* a remote session in `disconnected` state due to an auth failure, *When* the
    terminal state is reached, *Then* the announcement uses `role="alert"`
    (`assertive`), matching the existing `inlineEditError` convention for failures requiring
    user action, distinct from the `polite` connecting/reconnecting transitions.
- Connection status is driven by push events (`watchSessions`-style stream / `remotesSlice`),
  never per-render polling.
  - *Given* `RemoteConnectionIndicator` is mounted, *When* no health-change event has fired,
    *Then* no network request is issued by the component itself (state comes from Redux, fed
    by the existing stream subscription).
**Files**: `web-app/src/components/sessions/RemoteConnectionIndicator.tsx` (new), `web-app/src/components/sessions/RemoteConnectionIndicator.css.ts` (new), `web-app/src/lib/store/remotesSlice.ts` (new)

##### Task 6.2.2a: Define `remotesSlice` (Redux, parallel to `sessionsSlice`) (~5 min)
- `selectRemoteConnectionState(remoteName)` selector; fed by the events stream, not polling.
- Files: `web-app/src/lib/store/remotesSlice.ts`

##### Task 6.2.2b: `RemoteConnectionIndicator` component — badge + tooltip + `aria-live` region (~5 min)
- Structurally copy `ConnectionIndicator.tsx`'s `STATE_LABEL`/`STATE_ANNOUNCE` pattern,
  `polite` for connecting/reconnecting, `assertive`/`role="alert"` for a terminal failure
  state.
- Files: `web-app/src/components/sessions/RemoteConnectionIndicator.tsx`

##### Task 6.2.2c: vanilla-extract `statusConnected`/`statusReconnecting`/`statusDisconnected` variants (~4 min)
- Follows the existing `statusPaused`/`statusPausedDistinct` naming family in
  `SessionCard.css.ts`.
- Files: `web-app/src/components/sessions/RemoteConnectionIndicator.css.ts`

##### Task 6.2.2d: Mount `RemoteConnectionIndicator` in `SessionCard.tsx`, distinct from `getStatusText`'s badge (~4 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 6.2.2e: Component tests — state transitions announce correctly, no polling call issued (~5 min)
- Files: `web-app/src/components/sessions/RemoteConnectionIndicator.test.tsx`

### Epic 6.3: Feature registry + e2e tests

**Goal**: Register every new backend/frontend feature per `.claude/rules/feature-registry.md`,
and add e2e coverage per `.claude/rules/feature-testing-registry.md` /
`.claude/rules/e2e-test-conventions.md`. *(Covers requirements.md AC6 in full.)*

#### Story 6.3.1: Register remote-workspace features and write e2e coverage
**As a** maintainer, **I want** every new RPC and UI surface this feature adds registered in
`docs/registry/features/` with matching e2e test coverage, **so that** `make
registry-generate` shows no net-new coverage gap and the feature-testing/session-creation
registries stay in sync per repo convention.
**Acceptance Criteria**:
- `docs/registry/features/backend/` gains entries for `TestRemoteConnection`,
  `TrustRemoteHostKey`, and `CreateSession`'s remote-target extension; `docs/registry/features/
  frontend/` gains entries for the remotes settings page, host badge, and
  `RemoteConnectionIndicator`.
  - *Given* `make registry-generate` is run after this phase, *When* `docs/registry/
  coverage-gaps.json` is diffed against its pre-feature state, *Then* the net count of
    untested features does not increase (every new entry has `tested: true` with a matching
    `testIds` entry once e2e tests below land).
- New e2e specs exist for the remote tab, host badge, and connection status indicator, each
  starting with a `// @feature` annotation and using only `data-testid`/ARIA-role locators, no
  `waitForTimeout`.
  - *Given* `tests/e2e/remote-workspaces.spec.ts` and the running isolated test server, *When*
    a remote is configured, a session is created against it, and the connection indicator
    transitions, *Then* the test asserts on `data-testid`/role locators and
    `expect(locator).toHaveValue(...)`/`toHaveText(...)`-style waits, never
    `page.waitForTimeout`.
**Files**: `docs/registry/features/backend/test-remote-connection.json` (new), `docs/registry/features/backend/trust-remote-host-key.json` (new), `docs/registry/features/backend/create-session-remote-target.json` (new), `docs/registry/features/frontend/remotes-settings.json` (new), `docs/registry/features/frontend/remote-host-badge.json` (new), `docs/registry/features/frontend/remote-connection-indicator.json` (new), `tests/e2e/remote-workspaces.spec.ts` (new), `tests/e2e/pages/RemotesSettingsPage.ts` (new)

##### Task 6.3.1a: Add backend per-feature JSON entries (~5 min)
- Three files: `test-remote-connection.json`, `trust-remote-host-key.json`,
  `create-session-remote-target.json`, following `docs/registry/schema.json`.
- Files: `docs/registry/features/backend/test-remote-connection.json`, `docs/registry/features/backend/trust-remote-host-key.json`, `docs/registry/features/backend/create-session-remote-target.json`

##### Task 6.3.1b: Add frontend per-feature JSON entries (~5 min)
- Three files: `remotes-settings.json`, `remote-host-badge.json`,
  `remote-connection-indicator.json`.
- Files: `docs/registry/features/frontend/remotes-settings.json`, `docs/registry/features/frontend/remote-host-badge.json`, `docs/registry/features/frontend/remote-connection-indicator.json`

##### Task 6.3.1c: Add `// +api:`/`// +feature:` markers in the corresponding handlers/components (~4 min)
- `// +api: remote:test-connection` etc. in `server/services/remote_service.go`; `// +feature:
  remotes-settings` etc. in the new frontend files.
- Files: `server/services/remote_service.go`, `web-app/src/app/settings/remotes/page.tsx`, `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/RemoteConnectionIndicator.tsx`

##### Task 6.3.1d: `RemotesSettingsPage` e2e page-object helper (~5 min)
- Files: `tests/e2e/pages/RemotesSettingsPage.ts`

##### Task 6.3.1e: e2e spec — add remote, test connection, trust, create remote session, verify badge (~5 min)
- `// @feature remotes-settings, remote-host-badge, session:create` header.
- Files: `tests/e2e/remote-workspaces.spec.ts`

##### Task 6.3.1f: e2e spec — connection indicator reflects simulated disconnect/reconnect (~5 min)
- Files: `tests/e2e/remote-workspaces.spec.ts`

##### Task 6.3.1g: Run `make registry-generate`, verify no coverage-gap regression (~3 min)
- Files: `docs/registry/backend-features.json` (generated), `docs/registry/frontend-features.json` (generated), `docs/registry/coverage-gaps.json` (generated)

### Epic 6.4: Backend health events

**Goal**: `RemoteHealthProber` publishes push-driven connection-health transitions feeding
`remotesSlice`, closing the loop for Epic 6.2's indicator.

#### Story 6.4.1: `RemoteHealthProber` + `NewRemoteHealthChangedEvent`
**As a** user, **I want** a remote's connection status to update live without me refreshing
the page, **so that** the connection indicator (Story 6.2.2) reflects reality in real time.
**Acceptance Criteria**:
- A background prober per configured `RemoteConfig` checks `SSHRunner` liveness (via
  `ssh.ClientConn.Wait()` signaling disconnects, not a poll-and-block loop) and publishes
  `NewRemoteHealthChangedEvent` on state transitions.
  - *Given* a remote whose `SSHRunner` connection is severed externally, *When* the
    underlying `ssh.ClientConn.Wait()` returns, *Then* `RemoteHealthProber` publishes a
    `connected → disconnected` event via the `EventBus` within one prober tick, and the
    frontend's `remotesSlice` (subscribed to the same stream `sessionsSlice` uses) reflects
    the new state without a page reload.
**Files**: `session/sshremote/health_prober.go` (new), `pkg/events/types.go`, `server/server.go`

##### Task 6.4.1a: Add `NewRemoteHealthChangedEvent` constructor (~3 min)
- Follows the existing `New*Event` naming/shape convention.
- Files: `pkg/events/types.go`

##### Task 6.4.1b: Implement `RemoteHealthProber` (~5 min)
- One goroutine per configured remote, driven by `ssh.ClientConn.Wait()` plus a periodic
  lightweight liveness check (e.g. `SSHRunner.Run(ctx, "true")`) for the "reconnecting" state
  between hard disconnects.
- Files: `session/sshremote/health_prober.go`

##### Task 6.4.1c: Wire `RemoteHealthProber` startup into server wiring (~4 min)
- One prober started per `RemoteConfig` on server start, alongside existing background
  services in `server/server.go`.
- Files: `server/server.go`

##### Task 6.4.1d: Unit test — simulated disconnect triggers event within one tick (~5 min)
- Files: `session/sshremote/health_prober_test.go`

##### Task 6.4.1e: Frontend — subscribe `remotesSlice` to the health-change stream (~4 min)
- Extends whatever stream `sessionsSlice` already subscribes to, or a new lightweight
  `WatchRemotes` RPC if bundling into the existing session stream proves awkward (per
  `research/architecture.md` §4's noted design choice).
- Files: `web-app/src/lib/store/remotesSlice.ts`
