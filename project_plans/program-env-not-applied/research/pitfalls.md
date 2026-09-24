# Research: Pitfalls for "Resolved Value Never Reaches the Spawned Process"

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509

## 1. Where this codebase's env-injection pipeline actually lives

The current pipeline (`session/instance_tmux.go`) is:

```
resolveExtraEnvVars()          -- snap := i.Snapshot(); config.LoadConfig();
                                   config.ResolveProgramConfig(cfg, snap.Program)
  ├── buildExtraEnv()          -- renders map -> []string "KEY=VALUE", called from
  │                                wireTmuxSession() -> session.SetExtraEnv(extraEnv)
  └── claudeSettingsEnvOverrideArgs()  -- renders map -> --settings '<json>' (fix for #852)
```

`wireTmuxSession` is called from four sites: `session/instance.go:2252`,
`session/instance.go:2453`, and `session/instance_serialization.go:463/468/521/535/548`
(restore/resume paths). **Every one of these call sites is a distinct place the bug
could exist independently** — a fix verified against the create-session path does not
prove the resume/restore paths inject the same env.

`config.ResolveProgramConfig` (`config/defaults.go:211`) itself is a pure function —
`FindProgramConfig` + `ExpandEnvVars`, both correct in isolation (case-insensitive ID
match via `strings.EqualFold`, `json:"env"` tag matches what `UpsertProgramConfig`
persists in `config/types.go:493`). **This confirms the requirement's framing**: the
resolver is not the suspect; the wiring between resolver and subprocess is.

## 2. Common causes of "resolved value never applied" — mapped to this codebase

### a. Stale snapshot / two-snapshot divergence (the class this repo already named)
`buildLaunchCommand()` (`session/instance_tmux.go:262-288`) and `buildExtraEnv()` /
`resolveExtraEnvVars()` (`instance_tmux.go:585-611`) each call `i.Snapshot()`
**independently**, in two separate function calls, both from `wireTmuxSession`'s
callers. `buildLaunchCommand`'s own comment says the quiet part out loud:

> "One Snapshot() read: SetProgram mutates Program under i.mu, so two raw reads could
> observe different values (.claude/rules/instance-lock-free-reads.md)."

That comment is about *within* `buildLaunchCommand`. It does not cover the case one
level up: `buildLaunchCommand(claudeSessionID)` is called to produce `enrichedProgram`,
then `i.wireTmuxSession(enrichedProgram)` is called, which internally does *its own*
`i.Snapshot()` call inside `buildExtraEnv`. If a concurrent `SetProgram`/`SwitchProgram`
actor command lands between those two calls, the launched command line and the
`-e`/`--settings` env vars would be built from **different** `Program` values — the
exact "resolved-in-isolation, wrong-at-the-call-site" failure shape this item
describes, and it would reproduce exactly the way `instance-lock-free-reads.md`
describes for its own race: no error, no test failure, just a silently
wrong result. Grep for all `i.Snapshot()` call sites inside a single logical launch
sequence and confirm they either share one `snap` variable or are proven safe against
interleaving actor writes.

### b. Field-name / struct-shape mismatches
Checked: `config.ProgramConfig.Env` (`json:"env"`) matches what
`UpsertProgramConfig` (`server/services/defaults_service.go:709`,
`server/services/session_service.go:5327`) writes, and matches what
`ResolveProgramConfig`/`FindProgramConfig` read. No `Env`/`EnvVars`/`env` mismatch
found in this path — ruled out, not just assumed.

### c. Config loaded from the wrong directory/instance scope
`resolveExtraEnvVars()` calls `config.LoadConfig()` with **no explicit
instance/workspace argument** — it relies on ambient env vars
(`STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_WORKSPACE_MODE`) the same way
`log.GetConfigDir()` is documented to mirror `config.GetConfigDir()`'s priority list
(see this repo's root `CLAUDE.md`, "Application Data" section). Because this call
happens process-wide (not per-request), a divergence would require the *whole process*
to have been started with different scoping than the client that called
`UpsertProgramConfig` — e.g. the program was registered against a workspace-scoped
config (`workspaces/<hash>/config.json`) while the session's own actor/session
machinery reads the shared/default config path, or vice versa. This is exactly the
kind of bug that's invisible to a unit test that constructs `cfg` and passes it
directly to `ResolveProgramConfig` (see §4) — it never exercises `LoadConfig()`'s path
resolution at all. Worth an explicit trace: does the request path that persists the
program and the actor goroutine path that later resolves it agree on
`config.GetConfigDir()`'s inputs at both write and read time?

### d. Silent error-swallowing in a resolve/build step
`claudeSettingsEnvOverrideArgs()` already demonstrates the codebase's own convention
for this: on `json.Marshal` failure it logs a `log.Warn` and returns `("", "")` —
i.e., silently degrades to "no override" rather than propagating an error. That's a
deliberate, logged degradation for a genuinely rare failure (marshal of a
`map[string]map[string]string` essentially can't fail) — but it establishes the
pattern to check for elsewhere in this exact area: does `buildExtraEnv`, `SetExtraEnv`,
or the tmux session's `new-session` argument builder have any similar "return empty
slice / early return nil" branch that swallows a `-e` flag on some other error path
(e.g., a socket-mode session, `TmuxServerSocket != ""`, taking a different code path
that doesn't call `SetExtraEnv` at all — worth confirming `SetExtraEnv` is applied
identically for both the `NewTmuxSessionWithServerSocket` and
`NewTmuxSessionWithPrefix` branches in `wireTmuxSession`, which it currently is,
since `SetExtraEnv` is called once after the `if/else` — but any future refactor that
moves it inside one branch would silently break the other).

### e. Reused/resumed tmux session skipping the wiring path entirely
`wireTmuxSession` *constructs a new* `tmux.TmuxSession` object and calls
`SetExtraEnv` on it. `initTmuxSession()` (`instance_tmux.go:681`, doc comment: "creates
(or reuses) the tmux.TmuxSession object without starting it... Reuse needs
HasSession() AND (cached IsAlive() OR IsBackendProcessAlive())") is a second path that
may **reuse** an existing `TmuxSession` object rather than rebuilding it. If reuse
returns an already-constructed session object from before a program's env was
registered (or before this instance's `Program` field was switched to the custom ID),
the env vars baked into that pre-existing object are stale, and no error/log surfaces
— this is the single highest-suspicion candidate for "config value correct, but never
reaches the spawned process" given the requirement's exact framing, and should be the
first place the root-cause agent instruments/traces.

## 3. tmux-specific pitfalls with `-e KEY=VALUE`

- `SetExtraEnv` builds a Go `[]string`, passed through `os/exec`-style argv, not a
  shell string — so classic shell-quoting escapes (spaces, `$`, backticks) are **not**
  needed for the `-e` value itself, since there's no shell re-parsing between Go and
  the tmux binary. This matters because `claudeSettingsEnvOverrideArgs()` *does* go
  through a shell-quoting builder (`shellQuote(string(payload))`) for the `--settings`
  JSON blob elsewhere in the same launch sequence — two different quoting regimes
  co-exist in the same function family, and copying `-e`-style handling to a
  shell-string context (or vice versa) is an easy transplant error. Confirm at the
  actual `tmux.TmuxSession` implementation that `-e` values are placed into an argv
  slice, not string-concatenated into a shell command anywhere downstream (e.g. if the
  session is later re-issued through `safeexec.CommandContext` with a joined string).
- tmux's own `-e` limit: values containing `=` are fine (tmux splits on the *first*
  `=` only), but a value containing tmux's own `%` format-expansion characters in
  certain tmux versions/configs can misbehave in status-line contexts — not directly
  relevant to `new-session -e`, but worth a smoke test since `ANTHROPIC_BASE_URL`-style
  values containing `://` and `:` are exactly the shape likely to trip up any
  accidental shell/format reinterpretation.
- If tmux's global/session `update-environment` option or a prior
  `set-environment`/`show-environment -g` global default exists for the same key, a
  later `attach`/`new-window` inside the same *server* (not session) could re-inherit
  the global value instead of the session-local `-e` override, depending on ordering.
  The requirement's own diagnostic (`tmux show-environment` showing **no override at
  all**, not a precedence loss) argues against this specific mechanism for the
  current bug, but it's worth naming as a distinct, real class along with the #852
  precedence bug it's easy to conflate this with.

## 4. Has this codebase hit this bug class before?

Yes — directly on point, and the fix pattern is documented in the code itself:

- **GitHub issue #852 / commit `5da2af7bf`** (`fix(session): also override claude's
  settings.json env via --settings`) is "a resolved value reaches the process
  environment, but a *downstream consumer* (Claude Code's own settings.json merge)
  still doesn't apply it" — a precedence bug, not an injection bug. The current
  requirement is upstream of that: `tmux show-environment` shows *nothing*, meaning
  injection itself is failing, not precedence between two correctly-injected sources.
  Do not fix this by extending `claudeSettingsEnvOverrideArgs` — that function's
  entire premise is "the env vars are already correctly present in `resolveExtraEnvVars()`'s
  output; this only fixes who wins." If `resolveExtraEnvVars()` itself never gets a
  chance to run (or runs against a stale/reused session object, see §2e), that fix does
  nothing for this bug.
- No other `project_plans/*/research/*.md` in this repo references "env var not
  applied" or "config not applied" as a prior bug pattern (checked via
  `git log --all --grep` and a repo-wide grep for related phrasing) — this is the
  first item to research this class from scratch, so there is no other fix pattern to
  reuse beyond #852's neighboring code.
- `instance-lock-free-reads.md` is the closest **structural** precedent, but for a
  different bug: a raw-field read racing an actor-goroutine write causing
  `go test -race` failures, caught by the race detector. This item's bug, per the
  requirement text, is a *deterministic* miss (env "not applied," not "applied to the
  wrong value sometimes") — so `-race` alone will not catch it, and the
  investigation should not stop at "is this field read via Snapshot()" (it already
  is, in the paths checked in §1) but should specifically check the **object-reuse**
  and **cross-call-site** angles in §2a/§2e.

## 5. What a robust regression test for this class must assert

The existing unit tests (`session/instance_tmux_test.go`,
`TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars`,
`TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`) construct
`&Instance{Program: "netflix-model-gateway", ...}` **directly as a struct literal**
and call `buildExtraEnv()`/`claudeSettingsEnvOverrideArgs()` **directly**. This is
precisely why they didn't catch the bug the requirement describes, for two
independent reasons:

1. **They bypass the actor/snapshot layer entirely.** `Snapshot()`'s doc comment says
   it "lazily builds one" when nil — meaning a struct-literal `Instance` with no
   published snapshot yet gets a snapshot built fresh from the raw fields the test
   just set, which trivially agrees with them. This proves `resolveExtraEnvVars()`'s
   *pure logic* is correct but proves nothing about whether the real construction
   path (`SetProgram`/`SwitchProgram` actor commands, `UpsertProgramConfig` persisting
   to disk, `config.LoadConfig()` re-reading it, `wireTmuxSession` being called at the
   right moment relative to snapshot publication) ever produces that same state in
   practice.
2. **They call the env-resolution functions directly, never `wireTmuxSession` and
   never `SetExtraEnv` on a real (or fake) `tmux.TmuxSession`, and never inspect what
   `tmux new-session`/`tmux show-environment` would actually report.** The
   requirement's own repro evidence is `tmux show-environment` showing nothing — a
   unit test that stops at "the Go `[]string` this function returns contains the
   right entries" cannot regress-test the actual bug, because the bug (per the
   research above) most likely lives in *whether `wireTmuxSession`/`SetExtraEnv` runs
   at all for the real session object*, not in whether `resolveExtraEnvVars()`'s
   output is correct once it does run.

A regression test that would actually catch this bug class needs to:

- Exercise the **real construction path**: `UpsertProgramConfig` → persisted config on
  disk → a session actually created via `CreateSession` (or whatever public API,
  not a struct literal) with that program ID → assert on the `tmux.TmuxSession`
  object's env state (or, better, an integration-level assertion against
  real/fake tmux's `show-environment` output, matching the requirement's own repro
  method) rather than a Go map/slice the resolver function returns in isolation.
- Cover **all four `wireTmuxSession` call sites** (create, resume-after-pause,
  restore-from-serialization, hibernate/checkpoint restore) — a table-driven test
  parameterized by launch path, since §2e above identifies session-object reuse
  (skipping `wireTmuxSession` rebuild) as the leading suspect and that only manifests
  on the resume/reuse paths, not the fresh-create path the current tests exercise.
  `session/instance_serialization.go`'s four `wireTmuxSession` call sites are good
  candidates for this parameterization.
- Assert **absence of pre-existing state carrying over**: create a session, change or
  add a custom program's env var via `UpsertProgramConfig` *after* the tmux session
  object already exists, then trigger whatever "reuse" path applies, and assert the
  new env var is (or per design, is not) reflected — this directly targets the
  `initTmuxSession()` reuse-vs-rebuild branch in §2e, which the current tests never
  touch at all.
- Where feasible, prefer a fake/mock `CommandRunner` (the codebase already threads
  `tmux.WithCommandRunner(runner)` through) that captures the literal argv passed to
  `tmux new-session`, and assert `-e KEY=VALUE` is present in that captured argv —
  this is the closest unit-testable proxy for "reached the spawned process" without
  needing a real tmux binary, and catches both a wiring failure (§2a/§2e) and an
  argv-construction/quoting failure (§3) in one assertion.

## Summary of concrete file/line pointers for the implementation phase

| Concern | Location |
|---|---|
| Env resolution (pure, verified correct) | `config/defaults.go:211` `ResolveProgramConfig`; `config/types.go:487-494` `ProgramConfig` |
| Env → tmux `-e` / `--settings` rendering | `session/instance_tmux.go:579-641` (`resolveExtraEnvVars`, `buildExtraEnv`, `claudeSettingsEnvOverrideArgs`) |
| Session (re)construction, `SetExtraEnv` call | `session/instance_tmux.go:646-676` (`wireTmuxSession`) |
| Session object reuse (leading suspect) | `session/instance_tmux.go:681` (`initTmuxSession`, reuse-vs-rebuild doc comment) |
| All `wireTmuxSession` call sites | `session/instance.go:2252,2453`; `session/instance_serialization.go:463,468,521,535,548` |
| Existing tests that don't catch this class | `session/instance_tmux_test.go:1206-1264` |
| Prior related fix (precedence, not injection) | commit `5da2af7bf`, GitHub issue #852 |
