# Stack Research: Custom Program Env Vars Not Applied to Session Process

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509

## Scope

Narrow technology/stack question: does this bug plausibly stem from a Go stdlib or
dependency quirk, rather than application logic? Findings below rule out every stdlib/
dependency hypothesis in the task brief and identify the one non-stdlib mechanism that
best explains the confirmed repro (no override anywhere in `tmux show-environment`).

## go.mod (relevant)

`go 1.26.6`. Dependencies touching this code path: none of the persistence (encoding/json,
stdlib), atomics (stdlib `sync/atomic`), or process/tmux (stdlib `os/exec`, no tmux Go
binding — this repo shells out to the `tmux` CLI directly via `session/tmux/tmux_session_start.go`)
are third-party. `github.com/tstapler/tymux/clients/go v0.1.0` is present but is used only
for control-mode *attach*/streaming (`-C` flag, `session/tmux/control_mode.go`), not for
`new-session` creation — ruled out below.

## Hypotheses checked and ruled out

1. **`encoding/json` map key loss/reordering on persist.** `json.Marshal` on
   `map[string]string` sorts keys deterministically; no key loss. `config/defaults.go`'s
   `ExpandEnvVars` (used by `ResolveProgramConfig`, `config/defaults.go:211-227`) only
   rewrites `${VAR}` tokens via `os.LookupEnv`; a plain value like
   `http://127.0.0.1:47000` has no `${...}` token, so `envVarPattern.ReplaceAllStringFunc`
   never invokes its callback and the value passes through unchanged, `allSet` stays true.
   Not the cause.

2. **`os/exec` env handling for the tmux subprocess.** The `-e KEY=VALUE` args passed to
   `tmux new-session` (`session/tmux/tmux_session_start.go:211-219`) are argv to the `tmux`
   binary, not `cmd.Env` — tmux applies them to the *new session's* environment table
   itself. `exec.Cmd.Env` being nil/inherited (default `os.Environ()`) is irrelevant to
   whether tmux honors `-e`. Not the cause.

3. **`sync/atomic.Pointer` snapshot publish timing.** `resolveExtraEnvVars()`
   (`session/instance_tmux.go:585-599`) reads `snap.Program` and `snap.EnvVars` from
   `i.Snapshot()` (race-free per `.claude/rules/instance-lock-free-reads.md`), but then
   calls `config.LoadConfig()` **fresh from disk on every call** — it does not read a
   cached/atomic-published copy of the *program config*. So a stale snapshot publish
   can't explain a fully-missing override; the read path re-loads config every time.
   Not the cause (and this call happens synchronously during session creation — no
   plausible race window for `snap.Program` either, since it's set before
   `wireTmuxSession` runs).

4. **tmux CLI's own `-e` flag limits/version.** `-e` on `new-session` requires tmux ≥ 3.2.
   `docs/how-to/bundle-tmux.md:7` pins the bundled build to **tmux 3.4** — well past the
   minimum. `show-environment -t <session>` reads exactly the session-scoped table that
   `-e` populates at `new-session` time, so if `-e ANTHROPIC_BASE_URL=...` had actually
   been on the argv, it would appear there. Its total absence (not a stale/wrong value)
   means the flag was never constructed for that key — i.e., `resolveExtraEnvVars()`
   returned a map without it, not that tmux dropped a flag it was given. Not a tmux/CLI
   quirk.

5. **`tstapler/tymux` control-mode dependency swallowing `-e`.** Control mode
   (`StartControlMode`, `session/tmux/control_mode.go:81`) is an attach-time `-C`
   streaming feature, started *after* `new-session -d` has already created the session
   with its `-e` flags. It has no involvement in session creation or env injection.
   Ruled out.

## One dead-but-harmless field found (not the bug, but a maintenance trap)

`TmuxSession` has **two** env-carrying fields: an exported `ExtraEnv []string`
(`session/tmux/tmux.go:251-253`, documented as "additional KEY=VALUE pairs... to pass as
-e flags") and an unexported `extraEnv` set only via `SetExtraEnv()`
(`session/tmux/tmux.go:1172-1176`). Both are iterated when building `-e` args
(`session/tmux/tmux_session_start.go:215` and `:619`), but `grep` across all non-test `.go`
files found **no production call site that ever assigns the exported `ExtraEnv` field** —
only `SetExtraEnv()` (writing `extraEnv`) is used, from `wireTmuxSession()`
(`session/instance_tmux.go:669-671`). The exported field is effectively dead code today;
harmless (empty-slice no-op) but a naming collision worth cleaning up separately since it
invites exactly this kind of "which field actually feeds the -e flags" confusion during
debugging. Not relevant to this bug's root cause.

## Most likely actual mechanism (outside narrow stack scope, flagged for the architecture/logic research lane)

`resolveExtraEnvVars()` calls `config.LoadConfig()` (`session/instance_tmux.go:589`), which
resolves to `config.GetConfigDirForDir("")` (`config/config.go:104-106`) — a **process-wide**
resolution (test-dir env override → `STAPLER_SQUAD_INSTANCE` → test-mode auto-detect →
workspace-preference file → opt-in `STAPLER_SQUAD_WORKSPACE_MODE` → global
`~/.stapler-squad/`), independent of which workspace the *session being launched* belongs
to. `FindProgramConfig` (`config/defaults.go:189-200`) then does a case-insensitive `ID`
match against whatever `cfg.SessionDefaults.Programs` that resolution returned. If the
config actually holding the registered `netflix-model-gateway` program lives under
`~/.stapler-squad/workspaces/<hash>/config.json` (a workspace-scoped file, per the
requirements doc) but the running server process resolves `LoadConfig()` to a *different*
config dir (e.g. the global shared one, if workspace mode/preference isn't active for that
process) at session-launch time, `FindProgramConfig` returns `nil`, `ResolveProgramConfig`
falls back to `IsCustom: false` / `EnvVars: nil`, and `resolveExtraEnvVars()` silently
produces an empty map for that key — exactly matching the confirmed symptom (no override
anywhere in `tmux show-environment`, not a wrong-value case). This is a config-resolution/
scoping question, not a stdlib or dependency defect, so it belongs in the
architecture-focused research lane rather than being resolved here — but it is the
strongest lead this pass found for *why* the env map buildExtraEnv() receives is empty.

## Summary of dependency versions checked

| Component | Version | Relevant? |
|---|---|---|
| Go | 1.26.6 (go.mod) | No known env/json/atomic quirks at this version affecting this path |
| Bundled tmux | 3.4 (`docs/how-to/bundle-tmux.md:7`) | `-e` on `new-session` supported since 3.2 — not a version gap |
| `tstapler/tymux/clients/go` | v0.1.0 | Used only for control-mode attach, not session creation — not in this path |
