# Stack Research: program-env-injection

Prior research reused: `project_plans/program-env-not-applied/research/stack.md` (cited below as "prior stack.md"). Each prior finding was re-checked against current code in this worktree (HEAD 453c8098f). Confidence labels: VERIFIED = source opened or command run in this pass; INFERRED/UNVERIFIED otherwise.

## 1. Mechanism summary (VERIFIED)

1. `Instance.wireTmuxSession` calls `i.buildExtraEnv()` and, if non-empty, `session.SetExtraEnv(extraEnv)` (`session/instance_tmux.go:794-796`).
2. `buildExtraEnv` = `STAPLER_SESSION_UUID` (if snap.UUID != "") + every `resolveExtraEnvVars()` entry as `KEY=VALUE` (`session/instance_tmux.go:729-739`).
3. `resolveExtraEnvVars` loads config fresh (`config.LoadConfig()`, `:714`), applies `config.ResolveProgramConfig(cfg, snap.Program)` when `IsCustom` (`:715-719`), then overlays `snap.EnvVars` so instance-level wins (`:720-722`).
4. `TmuxSession.Start` renders `new-session -d -s <name> -e CLAUDECODE= [-e kv for ExtraEnv] [-e kv for extraEnv] -c <workDir> "env HISTFILE=... <program>"` (`session/tmux/tmux_session_start.go:221-228`). The recreate-missing-session path renders the same via `newSessionArgs` (`:638-646`).
5. For claude programs, `buildClaudeCommand` also appends `--settings '<json>'` from `claudeSettingsEnvOverrideArgs` (`session/instance_tmux.go:480-482`, `:756-767`).

## 2. tmux `new-session -e` and version (mostly VERIFIED)

- `new-session` synopsis includes `[-e environment]` and the man page says "-e takes the form VARIABLE=value" (`man tmux` on this host, run in this pass; local binary is `tmux 3.6a` via `tmux -V`, `/home/linuxbrew/.linuxbrew/bin/tmux`).
- Project-pinned/bundled tmux is 3.4: `Makefile:281,302`, `.github/workflows/build.yml:255,552-555`, `docs/how-to/bundle-tmux.md:7`.
- Minimum version "3.2 for -e on new-session": carried over from prior stack.md; I could not open tmux's CHANGES file here (submodule `third_party/tmux` not checked out), so the exact minimum is INFERRED/UNVERIFIED. It does not matter in practice: pinned 3.4 and local 3.6a both exceed it, and the man page confirms the flag. No version gate exists in code that needs changing.
- `-e` values are argv to the tmux client, not `cmd.Env`; they populate the session environment table read by `show-environment -t`. The regression test asserts exactly that (`server/services/session_service_create_test.go:729-731`).
- Quoting: `-e KEY=VALUE` needs no shell quoting (argv). Quoting matters only for the shell command string tmux executes (`programWithHistory`, `:220`), which is where `shellQuote` is used.
- Caveat: `-e` sets the session environment, which is inherited by new panes/the first pane's process. It is not a per-command override, so a settings-file `env` block can still beat it inside Claude Code (see section 4).

## 3. shellQuote (VERIFIED)

`shellQuote` single-quotes and escapes embedded `'` as `'\''` (`session/instance_tmux.go:433-435`); `shellQuoteFields` quotes per whitespace token (`:442-449`). `--settings` JSON is passed through `shellQuote` (`:766`), so `$`, backticks and `"` in env values are inert. Go's `%q` is explicitly avoided (comment `:428-432`). `json.Marshal` on `map[string]map[string]string` sorts keys, so the flag is deterministic.

## 4. Claude Code `--settings` env precedence (code-comment VERIFIED, upstream doc UNVERIFIED)

The code comment (`session/instance_tmux.go:741-755`) asserts: settings.json `env` beats an inherited process env var; the CLI `--settings` flag outranks user/project settings files (below org-managed), per https://code.claude.com/docs/en/settings.md, and merges by key. I did not re-fetch that doc in this pass; treat the precedence ordering as INFERRED from the in-repo comment (which cites GitHub issue #852). Implication: `-e` alone is insufficient when the user's global `~/.claude/settings.json` sets the same key; `--settings` is the second injection path and `resolveExtraEnvVars` keeps both in sync by construction. Applies only to claude programs (it lives in `buildClaudeCommand`).

## 5. Findings of prior stack.md, re-checked

| # | Prior finding | Status | Evidence now |
|---|---|---|---|
| 1 | `encoding/json` / `ExpandEnvVars` pass plain values (e.g. `http://127.0.0.1:47000`) unchanged | Still holds | `config/defaults.go:244-` : `${VAR}` regex only (`envVarPattern`), key omitted only when a referenced var is unset; `ResolveProgramConfig` at `:211-227`. NEW nuance: a `${VAR}` value whose var is unset in the server process drops the key silently (warn log only). |
| 2 | `-e` args are tmux argv, not `cmd.Env` | Still holds | `tmux_session_start.go:221-228`. Line numbers moved from prior `:211-219`. |
| 3 | `resolveExtraEnvVars` reloads config from disk each call, so no snapshot-staleness cause | Still holds | `session/instance_tmux.go:714` (prior cited `:585-599`, `:589`; now `:710-724`, `:714`). It is called twice per claude launch (`buildExtraEnv` and `claudeSettingsEnvOverrideArgs`), so both paths could in principle diverge if config changes between calls; `LoadConfig` is not cached. UNVERIFIED whether that window matters. |
| 4 | tmux >= 3.2 required for `-e`; bundled 3.4 is above it; absence in `show-environment` means flag never built | Still holds (version), minimum 3.2 still unverified | See section 2. Bundled version cite unchanged (`docs/how-to/bundle-tmux.md:7`). |
| 5 | tymux control-mode client has no part in session creation | Still holds (not re-opened in depth) | `new-session` args built entirely in `tmux_session_start.go`; control mode attaches after. Not re-verified beyond that file. |
| 6 | Exported `TmuxSession.ExtraEnv` is dead code; only `SetExtraEnv` -> `extraEnv` is used | **No longer holds** | `grep` for assignments finds production writers: `session/instance.go:1641,1648,1785,1792` do `sess.ExtraEnv = append(sess.ExtraEnv, displayEnv/cdpEnvs...)` (VNC DISPLAY and CDP envs). Both fields are still iterated into `-e` (`tmux_session_start.go:222-227`, `:640-645`). The two-field naming trap remains, but the exported field is live. Fields declared at `session/tmux/tmux.go:129-130` (`extraEnv`) and `:251-253` (`ExtraEnv`); setter at `:1236-1239` (prior cited `:1172-1176`, `:251-253`). |
| 7 | Most likely cause: process-wide `LoadConfig()` resolves to a different config dir than the one holding the program, so `FindProgramConfig` returns nil and env map is empty | Partly holds (mechanism), root cause superseded | `LoadConfig` -> `GetConfigDir()` still process-wide (`config/config.go:1384-1394`), `FindProgramConfig` case-insensitive ID match (`config/defaults.go:190-200`) so the mechanism is real. The requirements doc (`project_plans/program-env-injection/requirements.md`) names a different verified root cause (pre-fix `initTmuxSession` called `SetExtraEnv` with only `STAPLER_SESSION_UUID`); this pass did not re-derive that root cause. The config-dir scoping risk should stay on the list as a residual, not as the cause. |

Go version / dependency table from prior stack.md: not re-checked beyond the tmux rows above (go.mod not opened this pass).

## 6. Patterns and libraries that apply

- No third-party library is involved: stdlib `encoding/json`, `fmt`, `os/exec` plus the tmux CLI (shelled out via `buildTmuxCommand`).
- Single source of truth pattern: `resolveExtraEnvVars` feeds both injection paths (tmux `-e`, claude `--settings`); any new injection site should call it rather than re-resolving.
- Map iteration order in `buildExtraEnv` (`:735`) is random; argv order of `-e` flags is nondeterministic across runs. Harmless to tmux, but tests must not assert exact argv order (sort keys if a deterministic argv is wanted).
- Regression test: `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (`server/services/session_service_create_test.go:673`) uses `tmux.Binary()` (honors `TMUX_BIN`, `session/tmux/binary.go:16`) and `tmux -L <socket> show-environment -t <name>`; it checks the tmux session table only, not the `--settings` path.

## Gaps

- tmux's exact minimum version for `new-session -e` (UNVERIFIED, immaterial given 3.4+).
- Claude Code `--settings` precedence not re-verified against upstream docs (INFERRED from in-repo comment).
- Whether the second `LoadConfig` in `claudeSettingsEnvOverrideArgs` can diverge from the first (UNVERIFIED).
