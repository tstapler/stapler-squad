# Backlog / Autonomous Driver Cross-Platform Risk Audit

Scope: read-only investigation into why the "backlog" autonomous feature has
worked (partially) on macOS but never on this Linux machine.

## 1. Feature flag mechanism

- Default: **off**. `config/config.go:805-815` — `GetFeatureFlag(name)` returns
  `false` whenever the key is absent from `Config.FeatureFlags` (a
  `map[string]bool` persisted in `config.json`). Comment: "Absent key returns
  false — all feature flags default to disabled."
- Enable path: **no env var**. Must be flipped via the `UpdateFeatureFlag`
  RPC, which the web UI calls from `web-app/src/lib/contexts/FeatureFlagsContext.tsx`
  (`setFlag` → `client.updateFeatureFlag({name, enabled})`). There is a
  Settings/flags UI surface for this; there is no config.json field a user
  would hand-edit by convention, and no documented env var toggle.
- Persistence is **per-config-file**, and config.json lives under the
  workspace-hash directory described in `.claude/docs/state-isolation.md`
  (`~/.stapler-squad/workspaces/{sha256(cwd)}/config.json`, or
  `~/.stapler-squad/instances/{name}/` if `STAPLER_SQUAD_INSTANCE` is set).
  **This means the flag is scoped per machine AND per workspace directory** —
  toggling it on one machine/workspace has zero effect on another.
- When disabled: two independent gates.
  1. Frontend: `docs/tasks/put-backlog-behind-a-feature-flag-by-default/plan.md`
     documents a client-side layout redirect (`web-app/src/app/backlog/layout.tsx`
     → `router.replace("/")`) that hides the Backlog nav/route entirely when
     the flag is off — the user would see no Backlog tab at all, not an error.
  2. Backend: `server/interceptors/feature_flag_interceptor.go` wraps every
     `BacklogService` RPC and returns `connect.CodeNotFound` — "feature does
     not exist," not "unauthorized" — wired only to `BacklogServiceHandler` in
     `server/server.go:369-375`.
  3. `server/dependencies.go:788` (`if cfg.GetFeatureFlag("backlog") { ... }`)
     also gates whether the backend `BacklogLifecycleController` is enabled at
     all (`server/dependencies.go:804`, `sessionService.SetFeatureController("backlog", backlogCtrl)`).
- **Verified on this machine**: checked all `config.json` files under
  `~/.stapler-squad/workspaces/*/config.json`. The workspace matching this
  repo checkout (`d685c4b1a423cca3`) has `"feature_flags": {"backlog": true}`
  — already enabled. Two *other* workspace hashes on the same machine have no
  `feature_flags` key at all (i.e., default off). **This confirms the
  workspace-scoping risk is real and easy to hit in practice**: if the user
  ever opens stapler-squad from a different working directory (a different
  workspace hash) than the one where they enabled the flag, Backlog will
  silently disappear with no error — which plausibly explains "I've only ever
  half-seen this work" even on a single machine, let alone a second one. On
  this specific worktree's workspace the flag is on, so the flag itself is
  **not** the blocker for this exact environment right now — but it is a
  strong candidate for "never seen it work" if the second machine's
  config.json/workspace was never explicitly re-toggled.

## 2. Platform-conditional code found

| File:line | What differs | Which OS "wins" by default |
|---|---|---|
| `executor/managed_process_darwin.go:9-27` | `Noctty` is **intentionally never applied** on macOS ("skip Noctty on macOS to avoid 'operation not supported by device' errors"). Only `Setpgid`/`Setsid` are honored. | macOS silently ignores a config flag that Linux honors. |
| `executor/managed_process_linux.go:14-33` | `Noctty` **is** applied on Linux when `cfg.noctty && !cfg.setsid`, and the comment explicitly documents that this fails with `ENOTTY` when the parent has no controlling terminal — i.e., whenever stapler-squad itself runs as a systemd service. | Linux is the only platform where this failure mode can occur at all. |
| `session/vnc/manager.go:404,456` | Still calls `executor.WithNoControllingTerminal()` (the flag documented above as broken under systemd). VNC is explicitly gated to Linux-only (`session/vnc/deps_check.go:28-31`, "VNC requires Linux"), so this is a Linux-only feature carrying a Linux-only landmine — not currently used by backlog/autonomous flow, but worth flagging since it's the same bug class that already bit headless triage once (see §4). |
| `scripts/install-service.sh:95-115` (Linux) vs `:196-269` (macOS) | **Asymmetric PATH handling.** Linux's systemd unit gets `Environment=PATH=$PATH` — a raw, static snapshot of whatever `$PATH` the shell running `make install-service` happened to have at that moment, with **no fallback list**. macOS's LaunchAgent plist explicitly appends `/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin` after the user's `$PATH` "so tools like tmux, git, and claude are found even if not already on the shell PATH" (comment at line 207-211). | macOS has a documented, intentional PATH-hardening fallback for exactly the binaries backlog/autonomous depend on (`claude`, `tmux`, `git`). Linux has none — if `claude` lives somewhere not captured in the PATH snapshot at install time (a later `nvm`/`asdf`/`npm` reinstall, a different install method, a non-interactive shell at install time), the systemd service simply won't find it, silently, with zero fallback. |
| `session/vnc/deps_check.go:28-31` | Hard OS gate: `if runtime.GOOS != "linux" { Reason: "platform not supported: VNC requires Linux" }` | Inverse of the usual pattern — this one feature is Linux-only. Unrelated to backlog but shows the codebase does have real per-OS feature gating elsewhere, so "no OS branching found" would be an incomplete claim. |
| `session/tmux/binary_embedded.go:58` | Bundled/embedded tmux cache path is keyed by `runtime.GOOS+"_"+runtime.GOARCH` | Both platforms supported; not itself a risk, but confirms embedded-tmux caching is per-arch — a stale cache from a previous arch/OS won't be reused, which is correct, but means first-run-after-migration always re-extracts. |

## 3. Timing / environment assumptions

- **`session/headless/caller.go:36`** — `NewPool` calls `exec.LookPath("claude")`
  directly: a **raw PATH lookup with no shell/alias resolution**, unlike
  `config/config.go:GetClaudeCommand()` (used elsewhere for per-session
  program resolution) which explicitly spawns `$SHELL -c "source ~/.zshrc/.bashrc; alias claude || which claude"`
  to resolve shell aliases and rc-file PATH mutations. The headless pool used
  by backlog triage/autonomous driving does **not** get this alias/rc
  resolution — it only sees whatever PATH the *server process itself*
  inherited at startup (i.e., whatever `scripts/install-service.sh` baked into
  the systemd unit or LaunchAgent, per §2 above).
- **`server/dependencies.go:450-464`** — headless pool construction failure is
  **non-fatal and silent-by-default**: `poolErr != nil` → `log.Warn("headless
  pool disabled: claude binary not found", ...)` and the server keeps running
  with `headlessPool == nil`. Every downstream caller
  (`server/services/session_service.go:784,802,1338,1520`,
  `server/services/backlog_service.go:1174`,
  `session/autonomous_driver.go:111`) checks `headlessPool == nil` and either
  logs a warning and no-ops, or (per `session_service.go:1520`) returns an
  error only when the *user* explicitly tries to toggle `AutonomousMode` on —
  triage itself just quietly never completes. There is no user-facing banner
  or health-check surfacing "claude binary not found," so this failure mode
  looks identical to "backlog just doesn't do anything."
- **Verified on this machine right now**: `claude` resolves to
  `/home/tstapler/.local/bin/claude`, and the live systemd unit's baked-in
  `Environment=PATH=...` does include `/home/tstapler/.local/bin`. So on
  *this specific* running instance, the PATH-lookup failure mode described
  above is not currently the blocker — but it is exactly the kind of thing
  that silently breaks after a `nvm`/`npm`/reinstall PATH change without a
  service reinstall, and there is no fallback (§2) the way macOS has one.
- **`session/autonomous_driver.go:80-84,176-187`** — 60s default startup idle
  timeout (`startupTimeout`), and a 5-minute per-turn idle timeout
  (`session/autonomous_driver.go:243`). ADR-022 (see §4) documents that these
  fixed timeouts are already known to be too short for triage-style workloads
  that spawn 4 parallel subagents taking 8-15 minutes — this is a
  timing/workload mismatch, not strictly an OS difference, but it means the
  driver is more likely to falsely time out on a busier/slower machine
  (e.g., a Linux box also running many other resource-heavy tmux/Claude
  sessions concurrently — this machine currently shows the systemd service's
  cgroup at "Tasks: 7647" and "Memory: 52.5G" — versus a lighter-loaded Mac).
- **`server/services/connectrpc_websocket.go:437-457`** —
  `STAPLER_SQUAD_USE_CONTROL_MODE` (default true) selects tmux control-mode
  streaming vs. capture-pane polling for the **web UI terminal view**. This
  is orthogonal to `AutonomousDriver`'s own status detection (which listens
  via `ClaudeController.AddStatusChangeListener` /
  `session/claude_controller.go:140-150`, not the websocket streaming path),
  so control-mode on/off should not directly break autonomous driving — but
  it does mean tmux control-mode parsing bugs (see §4, "unknown control mode
  notification" spam observed live in this machine's log) are a live,
  ongoing source of noisy/uncertain state detection that autonomous flows
  depend on transitively via the same `ClaudeController`.

## 4. Known historical platform bugs (git log)

- **`095e09e3` / `c4e2e84b` / `b35276ce`** (duplicated across fork-sync
  merges) — *"fix(headless): use Setsid instead of Noctty for headless runner
  subprocess."* Root cause: `WithNoControllingTerminal()` set
  `SysProcAttr.Noctty=true` on Linux, which calls `ioctl(0, TIOCNOTTY)` in the
  child — this returns `ENOTTY` **specifically when the parent has no
  controlling terminal, i.e., exactly the systemd-service case**. Every
  headless triage call failed with `fork/exec .../claude: inappropriate ioctl
  for device` (exit code 1) until fixed by switching to `Setsid`
  (`WithNewSession()`). This is a **directly-cited, previously-shipped,
  Linux-systemd-only bug in the exact subsystem (`session/headless`) that
  now underpins backlog triage.** It is fixed in current code, but is strong
  evidence this exact failure class (controlling-terminal assumptions under a
  headless service) has already bitten this feature once on Linux and not on
  macOS (macOS's `buildSysProcAttr` never applied `Noctty` in the first
  place, so it could never have hit this bug).
- **`2d7e116c` / `b1f63bab` (same fix, two branches)** — *"fix(backlog):
  replace idle triage sessions with headless pool calls"* — this is the
  commit implementing ADR-022 (see below): removing `AutonomousDriver`+tmux
  entirely from the triage path in favor of direct headless pool calls.
- **`ADR-022-headless-triage-over-autonomous-driver.md`** (2026-06-22, dated
  after the `Setsid` fix) documents that `TriggerTriage` via
  `AutonomousDriver` + tmux **"in practice, this flow never works"**, for
  four reasons, all pre-existing and OS-agnostic in themselves but compounded
  by the fact the headless pool nil-gate (reason #2) is exactly the
  PATH/binary-discovery failure mode in §3:
  1. `Prompt` vs `InitialPrompt` field mismatch — the driver's orchestration
     goal reads `inst.Prompt`, but the actual session was launched with
     `inst.InitialPrompt`, so the LLM orchestrator never saw the real task.
  2. Silent `headlessPool == nil` gate — tmux session spawns and sits idle
     forever if `claude` wasn't found at server startup.
  3. 5-minute per-turn idle timeout fires mid-parallel-subagent-run.
  4. No completion-signal path from `submit_triage_result` back to the driver.
  - **This ADR describes exactly the *pre-fix* behavior** — note that the
    current codebase (`server/services/backlog_service.go:28,1101`,
    `session/backlog_triage.go`) has already moved triage entirely off
    `AutonomousDriver` onto a direct `pool.CallBlockingWithOptions` call with
    synthetic `headless-triage-<uuid>` sessions. **`AutonomousDriver` is
    still used, unchanged, for the "drive a session to execute the plan"
    step** (`server/services/session_service.go:787,805,1338`) — all still
    passing `inst.Prompt`/`instance.Prompt` as the goal, i.e. **failure mode
    #1 from ADR-022 (`Prompt` vs `InitialPrompt` mismatch) has not been fixed
    for the execution-driving phase**, only for triage. This is a
    functional bug independent of OS, but it means even a perfectly-configured
    Linux (or Mac) box would see the *execution* half of "backlog: triage →
    plan → autonomously execute" behave like ADR-022 describes: the
    orchestrator LLM never receiving the real goal.
- No commits found specifically referencing `codesign`/`tcc`/`launchd` +
  backlog/autonomous in the same commit — the code-signing/TCC machinery
  (`.claude/docs/codesigning.md`) is unrelated to backlog logic, it only
  affects whether the *service itself* survives rebuilds without re-prompting
  for Full Disk Access on macOS; it has no Linux equivalent gate (Linux
  systemd services don't need FDA), so it's not a source of "works on Mac,
  not on Linux" — if anything it's a Mac-only extra hoop, not a Linux gap.

## 5. Open risks (not verified safe)

- **Stale systemd-unit PATH after environment changes.** The Linux
  `Environment=PATH=$PATH` snapshot (§2) is written once at `make
  install-service` time and never refreshed automatically. If `claude` is
  later reinstalled via a version manager (`nvm`, `asdf`, `volta`) that
  changes its resolved path, or if `make install-service` is ever run from a
  non-interactive/CI-like shell with a stripped PATH, the systemd unit's
  PATH goes stale silently — headless pool construction
  (`server/dependencies.go:452`) fails, logs a `Warn`, and the server keeps
  running with backlog partially non-functional. **Not verified**: whether
  this has actually happened on the "second machine" the user mentions — I
  only confirmed it is *possible* and *asymmetric vs. macOS's explicit
  fallback list*. Recommend checking that machine's actual
  `~/.config/systemd/user/stapler-squad.service` `Environment=PATH=` line
  against `which claude` in an interactive shell there.
- **`AutonomousDriver`'s `Prompt`/`InitialPrompt` mismatch for the execution
  phase** (§4) — confirmed present in current code by line reference, but I
  did not trace whether some other code path backfills `inst.Prompt` from
  `inst.InitialPrompt` before `NewAutonomousDriver` is called elsewhere. If
  unpatched, this alone could fully explain "backlog auto-execution never
  works," independent of platform — worth ruling in/out before chasing
  further OS-specific theories.
- **Workspace-hash flag scoping** (§1) — confirmed structurally and confirmed
  live on this machine (two of three workspace config.json files have no
  `feature_flags` key at all). Not verified: whether the "second machine" in
  question has the flag enabled in the *specific* workspace directory the
  user actually runs stapler-squad from day-to-day — this is the single
  fastest thing to check on that machine.
- **tmux control-mode parse gaps.** Live log on this machine shows repeated
  `"unknown control mode notification"` entries (e.g.
  `%client-session-changed ...`) and many `"attach-session process
  exited","exitErr":"exit status 1"` lines in a tight loop. I did not trace
  whether the tmux version installed on this Linux box (Manjaro — likely a
  newer/rolling-release tmux build) emits control-mode notification formats
  the parser doesn't recognize, versus whatever tmux version macOS/Homebrew
  ships or the project's pinned/embedded tmux 3.4
  (`third_party/tmux`, `make build-embedded`). This is a plausible
  Linux-distro-specific tmux-version skew that could degrade status
  detection quality feeding `AutonomousDriver`'s idle-detection loop, but I
  could not confirm root cause within this investigation's scope — would
  need `tmux -V` on both machines and a diff against the parser's known
  notification set (`session/tmux/*.go`).
- **`session/vnc/manager.go`'s residual `WithNoControllingTerminal()` calls**
  — same bug class as the fixed headless-runner bug (§4), still present in
  Linux-only VNC code. Not currently in the backlog/autonomous call path, so
  low priority, but flagged since it's the identical landmine in a sibling
  Linux-only subsystem.

## Files referenced

- `config/config.go` (feature flags: 805-824; claude binary resolution: 444-523)
- `server/interceptors/feature_flag_interceptor.go`
- `server/server.go:369-375`
- `server/dependencies.go:443-464,788,804`
- `web-app/src/lib/contexts/FeatureFlagsContext.tsx`
- `docs/tasks/put-backlog-behind-a-feature-flag-by-default/plan.md`
- `session/autonomous_driver.go`
- `session/headless/caller.go`, `session/headless/runner.go`
- `server/services/session_service.go:756-805,1230-1345,1519-1536`
- `server/services/backlog_service.go:28,1101,1174`
- `session/backlog_triage.go`
- `docs/adr/ADR-022-headless-triage-over-autonomous-driver.md`
- `executor/managed_process_darwin.go`, `executor/managed_process_linux.go`, `executor/managed_process_other.go`
- `session/vnc/manager.go:404,456`, `session/vnc/deps_check.go:28-31`
- `scripts/install-service.sh` (Linux: 74-141; macOS: 196-336)
- `.claude/docs/state-isolation.md`, `.claude/docs/codesigning.md`, `.claude/docs/pty-multiplexing.md`
- `server/services/connectrpc_websocket.go:437-457`
- Live evidence on this machine: `~/.config/systemd/user/stapler-squad.service`,
  `~/.stapler-squad/logs/staplersquad.log`,
  `~/.stapler-squad/workspaces/{d685c4b1a423cca3,20761b3c035451cd,ab2461e5c3f3f8f6}/config.json`
