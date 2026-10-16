# Pitfalls: bundling and supervising tymuxd

Research for `project_plans/tymux-bundled-integration/requirements.md`. Each item is a
concrete risk plus a mitigation the plan (Phase 3) should design against, not just note.

## 1. Lessons from tmux bundling/supervision that transfer to tymuxd

Source docs: `.claude/docs/tmux-keep-server-on-restart.md`,
`.claude/docs/state-isolation.md`, `.claude/docs/service-restart-orphan-process.md`,
`.claude/docs/bundling-tmux.md`, `session/tmux/binary_embedded.go`.

- **Restart tears down the supervised process unless explicitly told not to.**
  `tmux-keep-server-on-restart.md` documents a real incident: the Linux systemd unit's
  `ExecStart` omitted `--tmux-keep-server`, so every `make install-service` restart killed
  the tmux server and every live session, silently rebuilt from scratch minutes later
  (confirmed live: 2026-08, `staplersquad_stapler-squad-bklg` session recreated, scrollback
  lost). **Risk**: a `tymuxd` supervisor that ties the daemon's lifetime 1:1 to the Go
  process's lifetime will kill every tymux-backed session on every service restart/upgrade,
  the same way tmux was killed before `--tmux-keep-server` existed. **Mitigation**: design
  an equivalent `--tymuxd-keep-server`-style flag (or default tymuxd's lifetime to
  outlive the Go parent, matching tmux's actual server/client model) from day one, not as a
  follow-up fix after the first incident — and audit `scripts/install-service.sh` for
  *both* platforms' `ExecStart`/`ProgramArguments` before shipping, since the tmux incident
  specifically happened because Linux and macOS drifted (macOS's LaunchAgent had the flag,
  Linux's systemd unit didn't).

- **launchd/systemd restart guarantees don't always hold — orphaned processes race the new
  one over shared state.** `service-restart-orphan-process.md` documents four orphaned
  `stapler-squad` binaries found running simultaneously on macOS, one bound to the live
  port while `launchctl` reported the service as "not running" — `bootout` is documented to
  block until the old process exits, but in practice didn't. **Risk**: if a `tymuxd`
  supervisor's stop-then-start-new sequence has the same non-atomicity, an old `tymuxd`
  can keep listening on `127.0.0.1:7419` while a new one tries to bind the same port and
  fails (or binds a different port silently), and both processes race writing to whatever
  session-state file tymuxd persists. **Mitigation**: don't assume "I sent SIGTERM and it
  returned" means the old process actually released the port — poll for the port/socket to
  become free (or actively health-check that a *new* process instance is answering, not
  just that *a* process is) before considering the restart complete, mirroring the
  `ps`/`lsof`/`launchctl print` cross-check the doc recommends for manual diagnosis.

- **Embedded-binary extraction has no integrity check today.** `binary_embedded.go`'s
  `extractEmbeddedTmux()` writes the `go:embed`-compiled bytes to
  `$UserCacheDir/stapler-squad/tmux/<os>_<arch>/tmux` with `0755` perms and only skips
  rewriting if the existing file's *length* matches (not a hash) — and `Binary()` lets
  `TMUX_BIN` unconditionally override the path with zero validation. This is a documented,
  accepted pattern for tmux; carrying it forward unexamined for tymuxd would additionally
  carry forward its weaknesses (see §5).

- **State isolation must extend to the daemon, not just the Go process.** `state-isolation.md`'s
  instance hierarchy (`STAPLER_SQUAD_INSTANCE`, workspace hash, `~/.stapler-squad/instances/<id>/`)
  isolates config/sessions/worktrees per instance, but nothing in it currently isolates a
  *second listening process*. Two manual dev instances (per `CLAUDE.md`'s "Manual dev port
  block", ports 62871-62880) both defaulting to `tymuxd` at `127.0.0.1:7419` would collide
  today. **Mitigation**: `TYMUXD_ADDR`/the tymuxd listen port must be derived from
  `STAPLER_SQUAD_INSTANCE` (or the workspace hash) the same way state directories already
  are, not left as a single global default — see §3 below for the concrete port-collision
  scenario.

## 2. General pitfalls of bundling and supervising a second external daemon from Go

- **Orphan/zombie risk if the Go parent crashes without cleanup.** `os/exec`-started child
  processes are not reparented to the parent's process group by default on Linux (no
  `Pdeathsig`) or on macOS (no equivalent signal at all) — a hard crash (SIGKILL, OOM-kill,
  `panic` in a signal handler) of the Go parent leaves `tymuxd` running, reparented to PID 1,
  exactly like the confirmed orphaned-`stapler-squad` incident in
  `service-restart-orphan-process.md`, just one level down the process tree. **Mitigation**:
  set `SysProcAttr.Pdeathsig = syscall.SIGTERM` on Linux (no equivalent on macOS — must rely
  on an explicit `stop-on-shutdown` path plus a startup health-check that detects and reaps a
  stale orphan by port/PID-file before spawning a new one, mirroring the `ps`/`lsof` manual
  check the orphan doc recommends).

- **Port conflict if `127.0.0.1:7419` is already bound** — by another stapler-squad instance,
  by a manually-run `cargo run` tymuxd left over from before this feature existed (the
  requirements doc notes this exact out-of-band workflow was the *status quo*), or by an
  unrelated process. **Risk**: naive "start tymuxd, ignore bind errors" supervision either
  silently no-ops (leaving the *old* tymuxd running and the new session pointed at possibly
  stale/incompatible state) or crashes session creation outright. **Mitigation**: on a bind
  failure, health-check the *existing* listener first — if it answers tymuxd's own health
  RPC and reports a compatible version, treat it as "already running, reuse it" (this is
  the correct steady-state case for the "start-if-not-running" requirement); if it doesn't
  answer as tymuxd (port squatted by something else) or answers as an incompatible version,
  fail loudly rather than silently misrouting sessions to the wrong daemon. Per-instance
  port derivation (§1) avoids the common case entirely; this handles the residual case where
  a human left a manual daemon running on the plain default port.

- **Binary compatibility across platforms.** Rust cross-compilation for a bundled binary has
  gotchas Go's `CGO_ENABLED=0` static builds mostly avoid: glibc-linked Linux builds only run
  on hosts with a compatible-or-newer glibc (musl target, e.g.
  `x86_64-unknown-linux-musl`, avoids this but is a separate build target to maintain and
  test); macOS builds need both `x86_64-apple-darwin` and `aarch64-apple-darwin` (or a
  universal binary) since this repo's own `codesigning.md` implies developer machines are
  Apple Silicon-first but Intel Macs aren't ruled out. **Mitigation**: if going the
  submodule-compile route (mirroring tmux), `make build-tymuxd` needs an explicit target
  triple decision documented (musl on Linux to sidestep glibc skew) and CI needs a matching
  toolchain per platform — the exact cost the requirements doc's Constraints section asks
  to be weighed against a prebuilt-binary-download approach, where this cost is paid once
  in `tstapler/tymux`'s own release CI instead of on every stapler-squad build.

- **macOS codesigning/notarization for a second embedded binary.** `codesigning.md` documents
  that stapler-squad's *own* binary is signed with a self-signed dev cert (`StaplerSquadDev`)
  purely to keep TCC grants stable across rebuilds — this is not Apple notarization and
  would not by itself satisfy Gatekeeper for a bundled Rust binary shipped to other users
  (self-signed certs don't pass notarization checks; Gatekeeper can quarantine/block an
  unsigned or ad-hoc-signed executable extracted at runtime, especially one downloaded fresh
  as opposed to compiled locally). **Risk**: a `tymuxd` binary extracted to disk and `exec`'d
  at runtime (same pattern as `binary_embedded.go`) could hit a Gatekeeper block on a machine
  where it wasn't already trusted, with a confusing failure mode (silent exec failure or a
  quarantine dialog) unrelated to anything the supervision code did wrong. **Mitigation**:
  the plan must explicitly state whether `tymuxd` needs its own codesign/notarization step
  (likely yes, if shipped as a prebuilt binary to other machines) or whether the
  compile-from-submodule route sidesteps it because the binary is built locally on the
  developer's own already-trusted toolchain (need to verify this assumption against current
  Gatekeeper behavior for locally-`cargo build`-produced binaries — this repo does not yet
  document that case).

- **Version skew between the bundled `tymuxd` binary and the generated gRPC client.**
  `session/tymux/transport.go` already imports generated types from
  `github.com/tstapler/tymux/clients/go/gen/tymux/v1` — a separate Go module dependency
  pinned by `go.mod`, while the *binary* would be pinned separately (submodule commit or
  release tag). **Risk**: nothing today enforces these two pins move together — a
  `go.mod` bump of the generated client without a matching `tymuxd` binary bump (or vice
  versa) compiles and links fine but can silently break at the RPC layer (new required
  field, changed enum, renamed RPC) only visible at runtime against a live daemon. This is
  a strictly two-repo version of the same problem `proto-gen` already manages within this
  one repo. **Mitigation**: pin both from the same source of truth (e.g. the submodule
  commit SHA also determines which `tstapler/tymux` release tag's Go client `go.mod`
  requires), and add a startup health-check that compares tymuxd's self-reported version
  against the client's expected version, refusing to proceed (or falling back to
  spawning the bundled binary instead of trusting a stale already-running one) on mismatch
  — closing the same gap §1's port-conflict mitigation leaves ("answers as tymuxd" is not
  the same check as "answers as a *compatible* tymuxd").

## 3. Rollout-mechanics risks in adapting streamhub's flag/override pattern

Source: `config/config.go`'s `ResolveGlobalStreamHubDefault` /
`RecordRollbackRehearsalCompleted` / `GetStreamHubSessionOverride` /
`SetStreamHubSessionOverride`, and `project_plans/terminal-multi-connection-streaming/research/pitfalls.md`
§3c ("Rollback must not orphan hub state") and §5 point 4 ("a same-day rollback path must be
exercised, not just asserted").

- **In-flight migration risk: flipping the global default while sessions are already
  pinned to the old backend.** streamhub's own pitfalls research (§3c) already identifies
  this for its *in-process* ownership switch and requires an explicit answer: does the flag
  gate only *new* hub creation (existing sessions finish out their lifecycle on whichever
  model they started with), or does a flip force a controlled reconnect of everything? For
  a *process-backend* switch (tmux vs. tymux), the equivalent question has strictly higher
  stakes: a session created against a running `tmuxd`/tmux server cannot be silently
  "migrated in place" to tymux mid-session the way an in-process hub reference can be
  swapped — the backing process itself is different. **Mitigation**: the plan must state
  explicitly (not leave implicit, per streamhub's own lesson about "silently trying to
  migrate in place is the highest-risk option and should be explicitly rejected") that the
  global/per-session backend selection is resolved once, at session-creation time, and is
  immutable for that session's lifetime — matching `ProcessManagerOptions.Backend`'s
  existing shape (a field consulted at creation, not polled continuously) — with a flag
  flip only affecting *subsequently created* sessions, never reaching back into
  already-running ones.

- **The rollback-rehearsal gate proves the mechanism works, not that the rollback path
  itself is safe for a process backend.** `ResolveGlobalStreamHubDefault`'s rehearsal gate
  (`RollbackRehearsalCompletedAt`) is satisfied by rehearsing streamhub's rollback — flip a
  per-session override on, use it, remove the override, confirm a clean reconnect under the
  *legacy in-process path*. Blindly reusing the same gate for tymux would let an operator
  satisfy "the rehearsal gate is green" without ever having exercised tymux's actual
  rollback scenario, because the two rollbacks are mechanically different: streamhub's
  rollback re-attaches a connection to a different in-process object; a tmux/tymux rollback
  means a *different backend process* must have been supervising the session all along, or
  the session becomes unreachable/needs to be recreated on rollback. **Risk**: shipping
  "the rehearsal gate" as a rename/copy of the streamhub mechanism without also verifying
  what "rollback" concretely means for a session created under tymux (does rolling back
  kill and recreate it under tmux? does it require the tymux session to already be
  gracefully handed off? is a tymux-backed session simply *not* rollback-able mid-flight,
  making "rollback" only apply to the *default for new sessions*?) ships a false sense of
  safety — a green gate that has never actually rehearsed the failure mode it exists to
  catch. **Mitigation**: the plan must define tymux's rollback semantics explicitly (most
  likely: rollback only changes the default for new sessions, and existing tymux-backed
  sessions are explicitly documented as non-rollback-able without recreation) before
  reusing `RecordRollbackRehearsalCompleted`'s gate, and the rehearsal procedure itself must
  be rewritten to actually exercise that defined behavior, not copy streamhub's rehearsal
  steps verbatim.

- **Reconciling the new gate with the existing coarser `ProcessManagerBackend` selector**
  (requirements.md's open question) is itself a rollout-safety risk if got wrong: if the
  rehearsal-gated flag and `process_manager_backend: tymux` are two independently-settable
  knobs that can disagree (e.g. gate says rehearsal incomplete, but
  `process_manager_backend` is hand-edited to `tymux` in `config.json` anyway), the gate
  provides no actual protection — it must be the single source of truth that
  `process_manager_backend: tymux`'s resolution *goes through*, not a parallel check that
  can be bypassed by setting the older field directly.

## 4. Security/robustness risks of a locally-listening gRPC daemon

- **No documented auth/TLS on the tymuxd gRPC connection today.** `transport.go` connects
  over plain `http2`/`connect.NewClient` to `TYMUXD_ADDR` (default
  `http://127.0.0.1:7419`) with no TLS (`crypto/tls` is imported but not obviously wired to
  a real cert for a loopback-only default) and no credential/token exchange visible in the
  transport seam. For a genuinely loopback-only daemon this mirrors this repo's own
  documented precedent for ssq-mux's Unix socket (`prefer-go-git...` sibling doc
  `terminal-multi-connection-streaming/research/pitfalls.md` §4a: "filesystem ownership *is*
  the auth boundary, with zero application-level authentication"). **Risk**: unlike a Unix
  socket with `0600` permissions scoping access to the owning user, a TCP port bound to
  `127.0.0.1` is reachable by **any local user on a shared/multi-user machine**, not just
  the process owner — a materially weaker boundary than ssq-mux's filesystem-permission
  model. On a single-user developer laptop this is low risk; on a shared dev box or CI
  runner with multiple concurrent users it is not. **Mitigation**: either (a) explicitly
  scope tymuxd to loopback + document the shared-machine caveat as accepted risk (matching
  how this repo already accepts the equivalent risk for `localhost:8543`'s own HTTP
  server), or (b) prefer a Unix domain socket over TCP if tymuxd supports it, inheriting the
  same filesystem-permission boundary ssq-mux already relies on instead of introducing a
  weaker TCP-loopback boundary net-new.

- **Port hijacking / squatting.** Nothing stops another local process (malicious or merely
  another misconfigured tool) from binding `127.0.0.1:7419` first, in which case
  stapler-squad's "start-if-not-running" health check needs to distinguish "tymuxd is
  already healthy there" from "something else already owns that port and isn't tymuxd" —
  see §2's port-conflict mitigation (health-check the protocol, not just the TCP accept).
  Silently talking gRPC to whatever answers on that port without verifying it's actually
  tymuxd is the concrete failure mode this must guard against, since a squatting process
  could return crafted/malformed gRPC responses.

- **Supply-chain risk if the binary at the expected path is substituted.** This applies
  differently to the two bundling approaches the requirements doc asks to be weighed:
  - *Compile-from-submodule* (tmux's pattern): the binary is built locally from a pinned
    git submodule commit, verified by git's own commit-hash integrity — substitution
    requires compromising the submodule source or the local build toolchain, a higher bar.
  - *Download-prebuilt-release*: fetching a binary from `tstapler/tymux` GitHub Releases at
    build time introduces a new supply-chain surface — a compromised release, a
    MITM'd download (if not fetched over HTTPS with certificate validation), or a build
    script that doesn't pin/verify a checksum against the release. `binary_embedded.go`'s
    existing tmux pattern offers **no worked example to copy here** — it only handles bytes
    already compiled into the Go binary via `go:embed`, with no separate integrity check
    at extraction time (content-length comparison only, not a hash) because the trust
    boundary was already crossed at `go build` time. A prebuilt-download approach for
    tymuxd would need its own checksum verification (e.g. pin a SHA-256 per release
    asset, verify before `exec`) that has no precedent in this codebase to lean on.
  - Either way, **`TYMUXD_BIN`-style env var overrides (mirroring `TMUX_BIN` today) must be
    treated as a deliberate escape hatch, not a silently-trusted input** — the existing
    `TMUX_BIN` override in `binary_embedded.go` has no validation at all; carrying that
    forward for tymuxd means anyone who can set env vars for the process can point it at an
    arbitrary binary. This is an accepted risk for a local dev override today; the plan
    should at minimum document it as intentional rather than let it silently apply to
    tymuxd without a decision being made.

## Summary of highest-severity items for the plan to design against

1. **Restart/upgrade must not kill live tymux-backed sessions** unless explicitly intended —
   design the keep-alive/detach-on-restart behavior before shipping, not as a follow-up
   fix after the first incident (this exact sequence already happened once for tmux).
2. **Backend selection must be resolved once at session-creation time and be immutable for
   that session's lifetime** — no in-place migration of a running session between tmux and
   tymux, and the rollback-rehearsal gate must be redefined for what "rollback" concretely
   means for a process backend before it's reused verbatim from streamhub.
3. **Health-checking "is tymuxd running" must verify protocol identity and version
   compatibility, not just port reachability** — this single check underpins the
   port-conflict, orphan-reuse, and version-skew mitigations above.
