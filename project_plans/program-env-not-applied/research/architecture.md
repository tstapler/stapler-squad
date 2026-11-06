# Architecture Research: Custom Program's Registered Env Vars Aren't Applied

## Root cause (bottom line up front)

**NOT REPRODUCIBLE on current HEAD (`dd1848f9b`).** A live, end-to-end
reproduction of the exact reported scenario (custom program registered via
`UpsertProgramConfig`, a genuinely isolated `SESSION_TYPE_NEW_WORKTREE`
session created against it, verified via real `tmux show-environment`)
**passes**: `ANTHROPIC_BASE_URL=http://127.0.0.1:47000` is present in the
tmux session's environment table.

The code path that fixes this — merging a custom program's registered `Env`
into the request's `instanceEnvVars` inside `CreateSession`
(`server/services/session_service.go:2520-2532`) — **did not exist before
commit `cdfd4e5cf2` ("feat(tagging): batched classify-once session tagger,
classifier settings UI, agy prompt detection (#825)", merged 2026-09-21)**.
That single large squashed PR is where the entire custom-program-management
feature (config schema, `UpsertProgramConfig`/`ResolveProgramConfig`,
`buildExtraEnv`'s program-env resolution, and the `claudeSettingsEnvOverrideArgs`
fix for GH #852) was introduced — including, per its own sub-commit messages,
an internal fix-up cycle:

- `fix(session): restore Restart LaunchCommand, read tmux wiring via Snapshot, tidy initTmuxSession doc` (+ "Adds custom-program env/flag tests.")
- `fix(session): stop double-applying custom program CLIFlags in CreateSession`

i.e. the bug this item describes was very plausibly real *during that PR's
development*, and was fixed as part of landing it, two days before HEAD.
The `requirements.md` "confirmed" repro almost certainly predates that merge,
or was run against a build/branch that hadn't picked it up yet.

**Recommendation for planning:** re-verify against the *actual* environment
the reporter used (confirm it's running a build that includes `cdfd4e5cf2`)
before designing a fix. If it's confirmed stale, this item should be closed
as already-fixed (see Verification below for the exact command to prove it
in one shot). If somehow still reproducible on a build that does include this
commit, the next place to look is the client that issued the `CreateSession`
RPC — see "If still reproducible" below.

## Traced pipeline (all confirmed correct by direct code reading + live test)

1. **`UpsertProgramConfig`** (`server/services/defaults_service.go:709-773`):
   validates and persists `config.ProgramConfig{ID, Command, Env, ...}` into
   `cfg.SessionDefaults.Programs` via `config.SaveConfig`. Confirmed
   round-trips correctly (`Env map[string]string \`json:"env,omitempty"\``,
   `config/types.go:493`).

2. **`CreateSession` RPC handler** (`server/services/session_service.go:2304`):
   - `cfg := config.LoadConfig()` once at top (line 2366), fresh from disk,
     no staleness/caching between requests.
   - `program := req.Msg.Program` (line 2452) — the raw ID string, e.g.
     `"netflix-model-gateway"`, is never overwritten to a resolved command
     anywhere before construction.
   - **The fix, added in #825** (lines 2520-2532): unconditionally (as long
     as `program != ""`, independent of `SkipDefaults`/alias/directory-rule
     branches above it) calls `config.ResolveProgramConfig(cfg, program)`
     and merges `resolvedProg.EnvVars` into `instanceEnvVars` for any key not
     already set by an explicit request-level `env_vars` entry.
   - `instanceOpts := session.InstanceOptions{..., Program: program, EnvVars: instanceEnvVars, ...}`
     (lines 2835-2859) carries both the raw program ID and the fully merged
     env map forward.

3. **`session.CreateManagedInstance`** (`session/create_managed_instance.go:107`):
   `NewInstance(opts)` sets `instance.Program = opts.Program` and
   `instance.EnvVars = opts.EnvVars` directly on the struct
   (`session/instance.go:1093,1139`) — not via an actor-mailbox round trip,
   so there's no window where these fields are stale relative to what the
   RPC handler computed. `finishInstanceConstruction` immediately publishes
   the first `Snapshot()` (`session/instance.go:1219-1228`), so
   `Snapshot().Program`/`Snapshot().EnvVars` are correct from the moment the
   instance exists — refutes the requirements.md hypothesis about a
   "lazily built, pre-publication" snapshot ever seeing empty values here.

4. **`runBackgroundResolutionPipeline`** (`server/services/session_creation_pipeline.go`)
   operates on the *same* in-memory `*session.Instance` pointer end to end
   (never reloads from storage before starting) and calls
   `p.instance.Start(true)` (line 258).

5. **`Instance.start()`** (`session/instance.go`, firstTimeSetup branch,
   ~line 1580-1667): for `SessionTypeNewWorktree`, `gitManager.Setup()` runs
   first, then `i.initTmuxSession()` (line 1607) — which calls
   `buildLaunchCommand()` then `wireTmuxSession(enrichedProgram)`
   (`session/instance_tmux.go:646-676`). `wireTmuxSession` computes
   `i.buildExtraEnv()` (→ `resolveExtraEnvVars()`, which merges
   `config.ResolveProgramConfig(cfg, snap.Program).EnvVars` *again*,
   redundantly-but-harmlessly, plus `snap.EnvVars`) and calls
   `session.SetExtraEnv(extraEnv)` **before** `tb.TmuxManager().SetSession(session)`
   — no gap where a later caller could overwrite the session object without
   the env already attached. `i.pm().Start(startPath)` (line 1654) runs
   after, on the exact same `*TmuxSession` object.

6. **`TmuxSession.start()`** (`session/tmux/tmux_session_start.go:167-221`):
   appends `-e KEY=VALUE` for every entry in both `t.ExtraEnv` (exported,
   unused by this path) and `t.extraEnv` (set by `SetExtraEnv`, the one that
   matters) to the `tmux new-session` argv — confirmed by the two-field
   design in requirements.md's own orientation notes, and confirmed by
   direct string inspection of the constructed args in the live test below.

## Verification (live, not just read)

Wrote and ran a throwaway integration test (`server/services`, deleted after
running — not committed) that:
- seeds `config.SessionDefaults.Programs` with a `netflix-model-gateway`
  entry (`Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}`) via the
  same `config.LoadConfig()`/`SaveConfig()` pair `UpsertProgramConfig` uses,
- calls the real `SessionService.CreateSession` RPC handler with
  `Program: "netflix-model-gateway"`, `SessionType: SESSION_TYPE_NEW_WORKTREE`,
  against a real git repo (`git init` + empty commit) — a genuinely isolated
  worktree, not a directory-collision case,
- polls the live `*session.Instance` to `Active`,
- shells out to the **real** tmux server the test's isolated
  `SessionService` used (`svc.testTmuxServerSocket`) and runs
  `tmux -L <socket> show-environment -t <session>` — the exact command
  requirements.md's repro used.

Result (full output captured, `go test ./server/services -run
TestZZZReproCustomProgramEnvVarsAppliedOnCreateSession -v`):

```
final status = Active
inst.EnvVars = map[string]string{"ANTHROPIC_BASE_URL":"http://127.0.0.1:47000"}
tmux -L test_server_services_2204839_1 show-environment -t staplersquad_program-env-repro-worktree:
ANTHROPIC_BASE_URL=http://127.0.0.1:47000
CLAUDECODE=
...
STAPLER_SESSION_UUID=75800555-2096-44f3-9dbb-ff2ca1278309
...
--- PASS: TestZZZReproCustomProgramEnvVarsAppliedOnCreateSession (0.81s)
```

`ANTHROPIC_BASE_URL` is present in the session-scoped environment table,
exactly where requirements.md says it was confirmed absent.

## Hypotheses from requirements.md: confirmed / refuted

| Hypothesis | Verdict |
|---|---|
| `resolveExtraEnvVars`/`buildExtraEnv`/`wireTmuxSession` wiring itself broken | **Refuted** — traced and live-tested correct |
| Two parallel `ExtraEnv`/`extraEnv` fields in `TmuxSession`, one not read at `start()` | **Refuted** — both are appended (`tmux_session_start.go:215-220`); `SetExtraEnv` writes the one `start()` actually uses |
| `config.LoadConfig()` resolves a different config dir for the `UpsertProgramConfig` write vs. the `CreateSession` read (workspace-mode per-directory hashing) | **Refuted for the single-process case** — both go through `GetConfigDir()` → `GetConfigDirForDir("")`, which resolves via the server process's own `os.Getwd()`/preference file, identical for every call in one process. Not ruled out as a *multi-process* concern (e.g. two different `stapler-squad` instances), but that's a different bug shape than the one described. |
| `Snapshot()`'s lazy pre-publication path returns an empty/stale `Program` | **Refuted** — `NewInstance` sets `Program`/`EnvVars` directly on the struct and `finishInstanceConstruction` publishes the first real snapshot before the instance is visible to any other goroutine |
| `FindProgramConfig`/`ResolveProgramConfig` case-sensitivity/trimming bug | **Refuted** — `strings.EqualFold` match confirmed correct in the live test with an exact-case ID |
| CreateSession RPC handler loses the program ID (resolves to raw command before `Instance.Program` is set) | **Refuted** — `program := req.Msg.Program` is carried through unmodified to `instanceOpts.Program` |

## If still reproducible after re-confirming the build includes `cdfd4e5cf2`

The one layer this research did **not** exercise end-to-end is the actual
RPC *client* — i.e. whatever the reporter used to call `CreateSession`. The
backend, given `program: "netflix-model-gateway"` in the request, behaves
correctly. If a live repro on current HEAD still fails, the next place to
look is **whether the client actually sends the raw program *ID*** in
`CreateSessionRequest.program`, or whether it resolves the custom program to
its underlying `command` (e.g. `"claude"`) client-side first and sends that
instead — which would make `config.ResolveProgramConfig(cfg, "claude")`
return `IsCustom: false` and skip the env merge entirely, silently
reproducing exactly this symptom. Check:
`web-app/src/components/sessions/Omnibar.tsx` (the `program` form field,
~line 1502) and `web-app/src/lib/omnibar/actions/dispatch.ts` (~lines 55,
100) for how the selected custom program maps to the RPC field. Also worth
checking any MCP tool or script-based session-creation entry point for the
same client-side resolution mistake.
