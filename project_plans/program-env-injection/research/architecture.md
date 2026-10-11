# Architecture Research: program env injection data flow

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509
Prior research reused: `project_plans/program-env-not-applied/research/architecture.md` (cited as PRIOR).
All file:line references below were opened against the current worktree (branch `backlog/stapler-squad-program-env-not-applied`). Confidence: VERIFIED unless marked INFERRED.

## 1. Data flow, end to end

| # | Stage | Location | What happens |
|---|---|---|---|
| 1 | Write | `server/services/defaults_service.go:709-773` (`UpsertProgramConfig`) | `config.LoadConfig()` (:742), replaces/appends `config.ProgramConfig{..., Env}` in `cfg.SessionDefaults.Programs`, `config.SaveConfig(cfg)` (:~767). |
| 2 | Persist | `config/config.go:1384` (`LoadConfig`) -> `GetConfigDir()` (:104) | File is `<configDir>/config.json`; same function used for read and write. |
| 3 | Request merge | `server/services/session_service_create.go:280-293` (was `session_service.go:2520-2532` in PRIOR) | When `program != ""` and `ResolveProgramConfig(cfg, program).IsCustom`, program env is copied into `instanceEnvVars` for keys the request did not set. CLIFlags deliberately not merged (:290-291). |
| 4 | Construct | `session/instance.go` `NewInstance`, via `InstanceOptions{Program, EnvVars}` | Raw program ID kept in `Instance.Program`; merged env in `Instance.EnvVars`. |
| 5 | Start | `session/instance.go:1611` (restart/resume branch), `:1749` (first-time worktree), `:1907`, `:2039` | Every `Start` variant calls `i.initTmuxSession()` after worktree setup, before `pm().Start()`. |
| 6 | Wire | `session/instance_tmux.go:806-823` (`initTmuxSession`) -> `:771-803` (`wireTmuxSession`) | Builds launch command, then constructs `tmux.TmuxSession`, calls `buildExtraEnv()` (:794) and `session.SetExtraEnv` (:795), then `SetSession` (:798). |
| 7 | Resolve | `session/instance_tmux.go:710-724` (`resolveExtraEnvVars`), `:729-739` (`buildExtraEnv`) | `config.LoadConfig()` fresh each call; program env from `ResolveProgramConfig(cfg, snap.Program)`; then `snap.EnvVars` overlays (instance wins). `STAPLER_SESSION_UUID` prepended. |
| 8 | Inject | `session/tmux/tmux_session_start.go:216-226` (`start`) and `:638-647` (`newSessionArgs`, recreate path) | `new-session -d -s <name> -e CLAUDECODE= [-e k=v for ExtraEnv] [-e k=v for extraEnv] -c <dir> <program>`. |
| 9 | Settings override | `session/instance_tmux.go:480` -> `:756-766` | `claudeSettingsEnvOverrideArgs()` re-calls `resolveExtraEnvVars()` and emits `--settings {"env":{...}}`, so tmux env and Claude settings override share one resolver. |

## 2. Specific confirmations

### (a) buildExtraEnv runs before new-session: VERIFIED
`wireTmuxSession` sets the env on the `TmuxSession` object (`instance_tmux.go:794-795`) before publishing it with `SetSession` (:797-798). Both `initTmuxSession()` (`instance.go:1611`, `:1749`, `:1907`, `:2039`) and `pm().Start()` come after, and `start()` reads `t.extraEnv` only when it builds argv (`tmux_session_start.go:225-227`). `SetExtraEnv` doc (`session/tmux/tmux.go:1236-1237`) states "Must be called before Start()".

### (b) LoadConfig reads the dir the API writes: VERIFIED (single process); caveat for multi-process
Both sides call `config.LoadConfig()` -> `GetConfigDir()` -> `GetConfigDirForDir("")` (`config/config.go:104-125`). Priority (`config.go:95-103`, :122-170):
1. `STAPLER_SQUAD_TEST_DIR`
2. `STAPLER_SQUAD_INSTANCE` (`instances/<id>`; `shared` = base dir)
3. test-mode auto-detect (`test/test-<pid>`)
4. preferred-workspace file (`SwitchDatabase`)
5. per-directory workspace hashing (`STAPLER_SQUAD_WORKSPACE_MODE=true`), resolved from the server's `os.Getwd()`
6. shared `~/.stapler-squad`

All inputs are process-global (env vars, cwd, preference file), so the API handler and the session start path in the same process resolve the same dir. A divergence is only possible across two processes with different env/cwd (e.g. the live service vs a manual instance, or a CLI invoked with a different `STAPLER_SQUAD_INSTANCE`). Unverified risk: the preference file (priority 4) can change at runtime via `SwitchDatabase`, so a config written before a switch is not visible after it; that is a workspace-switch semantic, not an injection bug.

### (c) Every start/restart/revive path goes through wireTmuxSession: VERIFIED for local tmux; GAP for remote
Callers of `wireTmuxSession` (non-test):
- `initTmuxSession`: `instance_tmux.go:822` (reached from `instance.go:1611,1749,1907,2039`).
- Resume/restart-with-UUID: `instance.go:2399` (rebuilds with `buildLaunchCommand`, then `wireTmuxSession`).
- Crash/recovery relaunch: `instance.go:2600`.
- Deserialisation for Paused/Stopped/Hibernated/Crashed/Active: `instance_serialization.go:467,472,536,550,563`. Hibernated/Crashed are wired but not started, so a later resume re-wires through `instance.go:2399`/`2600` with fresh env.
- Runtime relaunch of a confirmed-missing pane: `tmux.WithProgramProvider(i.currentLaunchCommand)` (`instance_tmux.go:779`, `:344-346`) rebuilds the command through `buildLaunchCommand` (which re-resolves program CLI flags and the `--settings` env override at :480) and `recreateMissingSession` uses `newSessionArgs` (`tmux_session_start.go:638-647`), which includes `t.extraEnv`; the object keeps its env from the original wire. VERIFIED by code reading; not executed.

Other `tmux.NewTmuxSession*` constructions that do NOT call `SetExtraEnv` (non-test):
- `session/backend_factory.go:110,116` (`newTmuxBackendFromOpts`): empty/placeholder session, doc says callers populate via `SetSession` in `initTmuxSession`; subsequent `wireTmuxSession` overwrites it. Not a bypass in practice (INFERRED from the doc comment at :89-90 plus callers; not exhaustively traced).
- `server/services/session_service_create.go:554,556` (remote pre-creation): see gap below.
- `*FromExisting*` (`external_discovery.go:178`, shell websocket): attach-only, never spawn.
- `tymux` backend (`session/backend_factory.go:~96` `newTymuxBackendFromOpts`): not a tmux `-e` path at all. INFERRED: env injection for the tymux gRPC backend is not covered by `buildExtraEnv`; `wireTmuxSession` type-asserts `*TmuxBackend` (:781, :793-797) so tymux-backed sessions skip it. Not traced further; flag for the plan.

**Gap (remote execution target): VERIFIED by code.** Remote sessions are created by `EnsureRemoteSession` -> `createRemoteSession`, whose argv is `new-session -A -d -s <name> -c <dir> <program>` with no `-e` flags (`tmux_session_start.go:~492`). `Instance.start` for a remote target deliberately skips `pm().Start()` and only calls `EnsureRemoteSession` again (`instance.go:1752-1775`). The pre-creation session at `session_service_create.go:554-558` is a fresh `TmuxSession` with no `SetExtraEnv`, and even the one on the Instance carries `extraEnv` that `createRemoteSession` never reads. Net: a custom program's env (and `STAPLER_SESSION_UUID`) never reaches a remote tmux session. Whether the product requires remote support is a requirements question (out of scope per requirements.md does not list it explicitly). Also note the `program` passed at `session_service_create.go:554` is the pre-`buildLaunchCommand` value, so the CLI-flag/--settings path is likewise not applied on remote.

### (d) ExtraEnv vs extraEnv: VERIFIED
- `TmuxSession.ExtraEnv` (exported, `session/tmux/tmux.go:251-253`): appended to directly by VNC/CDP wiring (`instance.go:1641,1648,1785,1792`, after `initTmuxSession()` and before `pm().Start()`), not by `SetExtraEnv`.
- `TmuxSession.extraEnv` (unexported, `tmux.go:129-130`): written only by `SetExtraEnv` (`tmux.go:1238-1240`), which assigns (replaces) the slice.
- Both are emitted, `ExtraEnv` first then `extraEnv` (`tmux_session_start.go:222-227`, `:640-645`). For duplicate keys later `-e` wins, so program/instance env (extraEnv) overrides VNC/CDP entries on collision (INFERRED from tmux `-e` semantics; not executed). PRIOR's claim that "ExtraEnv is unused by this path" is accurate for the program-env path but ExtraEnv is used by VNC/CDP.
- Hazard: `sess.ExtraEnv = append(...)` at `instance.go:1641/1785` accumulates if the same `TmuxSession` is reused across Start calls (initTmuxSession reuses a live session, `instance_tmux.go:807-810`). Duplicates are harmless for correctness. Two exported-and-unexported fields with overlapping purpose is a naming trap worth a rename/merge but not required for this item.

## 3. PRIOR findings against CURRENT code

| PRIOR finding | Status | Evidence |
|---|---|---|
| Root cause not reproducible on HEAD; fix introduced in `cdfd4e5cf2` | still holds | `resolveExtraEnvVars`/`buildExtraEnv` present at `instance_tmux.go:710-739`; requirements.md records the passing regression test. |
| Merge in `CreateSession` at `session_service.go:2520-2532` | no longer holds (location only) | Moved to `server/services/session_service_create.go:280-293`; behaviour identical (merge `ResolveProgramConfig(...).EnvVars` for unset keys). |
| `UpsertProgramConfig` at `defaults_service.go:709-773` | still holds | Same range, re-read. |
| `config.LoadConfig()` fresh per request, no cache | still holds | `config/config.go:1384-1399` reads file each call. |
| `wireTmuxSession` at `instance_tmux.go:646-676` calls `SetExtraEnv` before `SetSession` | still holds, line numbers drifted | Now `:771-803` (SetExtraEnv :795, SetSession :798). |
| `TmuxSession.start()` appends both `ExtraEnv` and `extraEnv` at `tmux_session_start.go:167-221` / `:215-220` | still holds, lines drifted | Now `:216-227`; also `newSessionArgs` `:638-647`. |
| `NewInstance` sets `Program`/`EnvVars` directly; first snapshot published before visibility | still holds (not re-traced to line) | `instance.go` construction; PRIOR cited `:1093,1139,1219-1228`. Lines not re-verified. |
| Merge runs again redundantly in `resolveExtraEnvVars` | still holds | `session_service_create.go:282` and `instance_tmux.go:715` both call `ResolveProgramConfig`. |
| Custom program ID must reach the RPC unresolved (client-side resolution risk) | still holds as a risk; not re-checked | Web client files not re-read in this pass. |
| `Start()` calls `initTmuxSession()` at `instance.go:1607`, `pm().Start` at `:1654` | no longer holds (line drift) | `initTmuxSession` call sites now `:1611,1749,1907,2039`. Behaviour unchanged. |
| (new, absent from PRIOR) | n/a | Remote execution target gap and tymux backend not covered by `buildExtraEnv`; see 2(c). |

## 4. Hotspot / architecture coverage of `session/instance_tmux.go`

- `docs/architecture-audit-session-package-split-2026-09-07.md:98,108,141` lists `instance_tmux.go` in the Instance/actor cluster, rank 7 at 4,050 lines (its own count; the file is now 1,602 lines per `wc -l`, i.e. it has since been split).
- `docs/architecture-audit-2026-07-01.md:187` lists it (rank 31).
- `project_plans/program-env-not-applied/implementation/architecture-review.md` references the file for a test-coverage scoping gap, not a hotspot.
- Churn: 40 commits touching the file in the last 90 days (`git log --since=90.days --oneline -- session/instance_tmux.go | wc -l`).

No existing analysis proposes a refactor of the env-injection functions specifically.

## 5. Recommended disposition: Extend as-is

The injection seam is already narrow and single-sourced: one resolver (`resolveExtraEnvVars`) feeds both the tmux `-e` set and the Claude `--settings` override, and every local start/restart/revive path funnels through `wireTmuxSession`. The file is high-churn but this item's remaining work (regression coverage, optional remote-target and tymux handling) is additive; refactoring the 1.6k-line file first would add risk without reducing it for this change. If remote support is brought into scope, add it as a small seam: have `createRemoteSession` accept env from `TmuxSession.extraEnv` (no change to `resolveExtraEnvVars`), rather than refactor `instance_tmux.go`.

## Open questions for the plan
1. Is remote execution target in scope for env injection (VERIFIED gap, section 2c)?
2. Do tymux-backed sessions need program env (INFERRED not covered)?
3. Should `ExtraEnv`/`extraEnv` be unified (cosmetic; not needed for correctness)?
