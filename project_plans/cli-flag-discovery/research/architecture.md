# Architecture Research: cli-flag-discovery

Evidence labels: VERIFIED = opened/ran in this worktree; INFERRED = reasoned, not confirmed.
EventStorming table skipped (simple request/response feature).

## (a) Probe code placement and executor injection

- VERIFIED: `config.CommandExecutor` (`config/executor.go:13`) has `Command/Output/LookPath`. Its implementations are unexported (`timeoutCommandExecutor`, `lookPathOnlyExecutor`), so a new package cannot reuse them without importing `config`, and `config` already owns `ProgramConfig` (`config/types.go:474`).
- VERIFIED: `executor/shortlived.go` already provides a builder for one-shot subprocesses (`WithTimeout`, `WithDir`, `WithReplaceEnv`, nil stdin = `/dev/null`, process-group kill by default, audit entries). It matches AC3/AC8 (timeout, closed stdin, sanitized env, kill group) with no new exec code. `executor/safeexec` is mandated by the `norawexec` lint rule (`.golangci.yml`), so raw `exec.Command` is not allowed.
- Recommendation: new leaf package `config/clihelp` (or `session/clihelp`; prefer a package outside `config` to keep `config` from growing). Contents:
  - `parser.go`: pure `ParseHelp(text string) []Flag` (regex; no I/O; fixture-testable).
  - `prober.go`: `type Prober struct{ lookPath func(string)(string,error); run func(ctx, path string, args ...string) ([]byte, error); stat func(string)(os.FileInfo,error); cache sync.Map }` with `Probe(ctx, command string) Result`.
  - Inject `lookPath` / `run` as small function fields or a 2-method interface rather than the 3-method `config.CommandExecutor`: the probe needs `LookPath` plus a bounded run, not `Command`+`Output`. In production, `run` wraps `executor.ShortLivedCmd` (timeout 3s, cap 256KB via `io.LimitedReader`/capped buffer, `WithReplaceEnv(minimal)`); in tests, a fake returns fixture bytes, sleeps past the timeout, or emits >256KB.
  - Decision to confirm at plan time: reusing `CommandExecutor` is possible (`config/config.go:1267` uses `executor.LookPath`), but its `Output` has no size cap and no stdin/env control, so it would need widening. Prefer the narrow interface.
- Cache: `sync.Map` keyed `resolvedPath + "|" + mtimeUnixNano` (AC3). Entry = parsed flags. Unbounded but tiny (one entry per distinct binary version); add no eviction (YAGNI), but note that `found=false` results are NOT cached (LookPath is cheap and PATH changes matter).

## (b) RPC hosting and registration

- VERIFIED: `DefaultsService` (`server/services/defaults_service.go:27`) is NOT registered as its own Connect handler. `server/server.go:414` registers only `sessionv1connect.NewSessionServiceHandler(deps.SessionService, ...)`, and `SessionService` owns `defaultsSvc *DefaultsService` (`session_service.go:171`, built at `:841` via `NewDefaultsService()`), with a one-line delegating method per RPC (e.g. `ListProgramsConfig` at `session_service.go:5216-5217`).
- So adding the RPC touches:
  1. `proto/session/v1/session.proto`: `rpc ProbeProgram(ProbeProgramRequest) returns (ProbeProgramResponse) {}` next to `ListProgramsConfig` (~line 565); new messages after `ProgramConfigProto` (line 2526): `ProbeProgramRequest{ string command }`, `ProbeProgramResponse{ bool found; string resolved_path; repeated CliFlagProto flags }`, `CliFlagProto{ name, short, takes_value, description }`. Then `make proto-gen` (gen/ is gitignored; do not commit).
  2. `server/services/defaults_service.go`: handler `ProbeProgram` with `// +api: program_config:probe` immediately above it (existing markers at lines 666/690/758 use `program_config:list|upsert|delete`).
  3. `server/services/session_service.go`: delegating wrapper `func (s *SessionService) ProbeProgram(...) { return s.defaultsSvc.ProbeProgram(ctx, req) }` (needed; the generated handler interface requires it or the build fails).
  4. `DefaultsService` needs a `prober` field. `NewDefaultsService()` (line 62) takes no args and is called from `session_service.go:841` (and tests); add a package-level default prober built in `NewDefaultsService()` plus a `SetProber(p)` test seam, to avoid changing the constructor signature at every call site.
- Feature registry: `make registry-generate` and commit changed `docs/registry/features/*.json`. UI marker `// +feature: settings-programs` already exists at `ProgramsManager.tsx:4`; the Omnibar file's marker would be added/kept in place. New shared hook/component (see below) needs its own `// +feature:` line in the first 10 lines.
- Handler is thin: validate, call `prober.Probe`, map `Result` to proto. Missing binary returns `found=false` with no error (AC2); only an empty command returns `CodeInvalidArgument`.

## (c) Session-creation program picker (FOUND)

- VERIFIED: Picker is a `<select id="omnibar-program">` in `web-app/src/components/sessions/OmnibarCreationPanel.tsx:936-950` (inside the "Advanced Options" collapsible). Options come from `useAvailablePrograms()` (`web-app/src/lib/hooks/useAvailablePrograms.ts`, which calls `listProgramsConfig`), filtered by `getPickerPrograms` (`web-app/src/lib/constants/programs.ts`). `OmnibarCreationPanel.tsx:350` and `:951-955` already show a "not found in PATH" style warning via `isProgramRecognized` (a purely client-side list-membership check, not a binary check).
- Extra flags: `extraCliFlags` is in `Omnibar.tsx:224` and is sent at `Omnibar.tsx:1507`; the "CLI flags" input for sessions lives with the creation form state (`program`, `setFormField`).
- Important mismatch: the picker's value is a program ID (`p.id` mapped to `value`), not a command. Session-creation probing must resolve ID to `ProgramConfigProto.command`, either (i) client resolves it from the `listProgramsConfig` list it already fetched and sends `command`, or (ii) the RPC also accepts `program_id`. See (e).
- `docs/reference/session-creation-registry.md` has zero mentions of "program" (VERIFIED with grep). It lists session-creation mode touchpoints, not the picker, so it does not constrain this feature. Not a new creation mode, so the 7-touchpoint rule does not apply.
- Settings form: `web-app/src/components/settings/ProgramsManager.tsx` (`prog-command`, `prog-flags` inputs; test `settings/__tests__/ProgramsManager.test.tsx`).
- Recommendation: one shared hook `useProbeProgram(command)` (debounced/blur-triggered, AbortController, per-command in-memory memo) and one shared presentational component (`BinaryStatusBadge` + `FlagInputHints`) used in both places. This avoids duplicating logic across the two forms, which matters because `jscpd` gate is at 0.12% and memory notes the margin is thin.

## (d) Data flow and caching/consistency

```
form blur / program select
  -> useProbeProgram(command)  [client memo, abort on change]
  -> SessionService.ProbeProgram (Connect)
  -> DefaultsService.ProbeProgram -> authorize (see e)
  -> clihelp.Prober: expand ~, split first token, LookPath
       -> not found: {found:false}
       -> found: stat mtime -> cache hit? return : run `<path> --help` (3s, 256KB, no shell)
                 -> ParseHelp -> cache -> return
  -> UI: badge (found/not found), autocomplete + unknown-flag warning (non-blocking)
```

- Consistency: cache key includes mtime, so an upgraded binary invalidates naturally; PATH changes are picked up since `LookPath` runs on every call and the key uses the resolved path. Single-process server, so no cross-instance coherence concern.
- Concurrency: concurrent probes of the same uncached binary should be de-duplicated with `golang.org/x/sync/singleflight` if already a dependency (INFERRED; check `go.mod` at plan time), else accept duplicate runs (bounded by the 3s timeout).
- Program-ID change / saved config change does not need cache invalidation (cache is keyed on binary, not config).
- `--help` may print to stderr for some tools: capture combined output (argparse and some Go tools use stderr on usage).

## (e) Security boundary

Threat model: the RPC runs a client-supplied path on the server. The Connect endpoint on `:8543` (and `:8444` remote-access, mobile app via Tailscale) is reachable by anyone authenticated to the UI; an unrestricted `command` is an arbitrary-binary-execution primitive with one fixed arg (`--help`), which still allows e.g. `/some/user-writable/script --help`. Note the app already lets the same user create sessions that run any program, so the marginal privilege is small but the probe runs without a user gesture in a session, so restrict anyway.

Recommendation (enforced server-side, not in the UI):
1. Request carries `command` only (AC1). Server derives the allow-list at request time: the set of `command` first-tokens from `config.LoadConfig().SessionDefaults.Programs` plus `BuiltInPrograms()` (`defaults_service.go:628`). A `command` whose expanded first token equals an allow-listed one is probed.
2. The "form under edit" case (unsaved new/changed command in ProgramsManager) cannot be in saved config by definition. Two options:
   - (A, recommended) Allow any command for probing but only through hardened execution: resolve via `LookPath` (absolute path required after resolution; reject if it contains path separators outside the allowed-forms `name`, `~/...`, `/abs/...`), reject a first token containing shell metacharacters (we never use a shell anyway), args fixed to exactly `--help`, replaced minimal env (`PATH`, `HOME`, `LANG`, no secrets/tokens, so `ANTHROPIC_API_KEY` etc. from the server env are not leaked to an arbitrary binary), cwd = temp/empty dir, stdin `/dev/null`, process-group SIGKILL on timeout, one concurrent probe per binary, and a global rate limit (e.g. semaphore of 2). Rationale: the same user can already save that command and launch it as a session, so the allow-list adds friction without a real boundary.
   - (B, stricter) Require the request to include `program_id` for saved programs; for unsaved commands require an explicit `unsaved=true` flag and additionally require the resolved binary to be a regular executable file not world-writable. More code; only worth it if remote-access exposure is considered hostile.
   Recommend (A), and record it in the plan as a decision needing owner sign-off, since the requirement text says "only allowed for the command the user is configuring" and the server cannot verify user intent, only shape the command.
3. Reject flag-like injection: first token must not start with `-`; arguments after the first token in `command` are ignored (never forwarded), so `claude --dangerously-skip-permissions` probes `claude --help` only. Hence the probe never passes user flags.
4. Test coverage (AC8): timeout kill, oversized output truncation, env sanitization (assert no inherited secret), first-token-only.
- INFERRED: whether the Connect interceptors apply auth on this RPC follows from it being on `SessionService` (same as all others); not independently verified.

## Prior analysis and hotspots

- VERIFIED: no prior `project_plans/*/research/architecture.md` covers program config or flag probing (listed architecture.md files overlap other features only; the closest is `alias-settings-manager`, which touched the same `DefaultsService` upsert/delete pattern).
- VERIFIED sizes / 6-month churn (`git log --oneline --since=6.months -- <file> | wc -l`):

| File | Lines | Commits (6 mo) |
|---|---|---|
| `server/services/defaults_service.go` | 794 | 16 |
| `web-app/src/components/settings/ProgramsManager.tsx` | 486 | 1 |
| `web-app/src/components/sessions/OmnibarCreationPanel.tsx` | 1040 | 36 |
| `server/services/session_service.go` | 5000+ (delegation at 5058-5217) | not counted |
| `proto/session/v1/session.proto` | n/a | 89 |

- `ProgramsManager.tsx` is small and stable: low risk. `defaults_service.go` is mid-sized with moderate churn and simple CRUD-style methods; adding one thin handler does not raise its complexity. `OmnibarCreationPanel.tsx` is the real hotspot (1040 lines, 36 commits): adding probe state inline would worsen it.

## Tech Debt Disposition

**Isolate via seam.** Put all new logic in a new `config/clihelp` package (parser + prober with injected lookPath/run) and a shared `useProbeProgram` hook plus small component, so `defaults_service.go` gains only a thin handler and `OmnibarCreationPanel.tsx` (high-churn, 1040 lines) gains only one hook call and one badge element; no pre-refactor of existing files is needed.

## Open items for the plan phase
- Confirm allow-list option (A vs B) with the owner.
- Confirm `singleflight` availability in `go.mod`.
- Decide whether `ProbeProgramRequest` also takes `program_id` for the session-creation path (picker values are IDs).
- Session-creation CLI-flags input location: the `extraCliFlags` field feeds through `Omnibar.tsx:224/1507`; locate the exact input element before task breakdown.
