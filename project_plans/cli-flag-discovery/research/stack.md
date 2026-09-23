# Stack research: cli-flag-discovery

Labels: VERIFIED = opened/grepped in this worktree; INFERRED = reasoned, not run.

## Bottom line
No new dependencies are needed on either side. Everything required is already in the repo.

## Go backend

| Need | Use | Status |
|---|---|---|
| Go / RPC versions | go 1.26.6 (`go.mod:3`), `connectrpc.com/connect v1.20.0` (`go.mod:17`), `protobuf v1.36.12` (`go.mod:64`), `golang.org/x/sync v0.22.0` (`go.mod:222`, indirect) | VERIFIED |
| Process-group exec with timeout | `safeexec.CommandContextPG(ctx, name, args...)` at `executor/safeexec/safeexec_pg.go:32`. Sets `Setpgid: true` and `cmd.Cancel` sends SIGTERM to `-pgid`, then SIGKILL after `sigkillGrace` (5s, `:22`). `WaitDelay` = 2s (`safeexec.go:~22`). | VERIFIED |
| Caveat on the above | The 5s SIGTERM->SIGKILL grace can exceed the requirement's 3s hard timeout. `sigkillGrace` is an unexported var. Options: (a) add a variant or option for immediate SIGKILL of the group, (b) call `syscall.Kill(-pid, SIGKILL)` from a probe-specific `cmd.Cancel`. Total wall time is roughly 3s + WaitDelay 2s; decide whether "hard timeout" includes WaitDelay. | VERIFIED (code) / INFERRED (impact) |
| Alternative wrapper | `executor/shortlived.go` `New(ctx, name, args, WithTimeout, WithReplaceEnv, WithStdin, WithoutProcessGroup)` (`:52-109`) already supports timeout, replaced env (sanitized env) and stdin. It is a candidate instead of a hand-rolled cmd. Not read in full whether Output caps size. | VERIFIED (options exist) / UNVERIFIED (behavior) |
| Testable injection | `config.CommandExecutor` (`config/executor.go:13`, Command/Output/LookPath). It is `Output`-based with no size cap and no stdin control, so the probe needs its own small interface (`LookPath` + `RunHelp(ctx, path) ([]byte, error)`) rather than reusing it. | VERIFIED |
| LookPath | `exec.LookPath(firstToken)`. Note that since Go 1.19 a relative-dir hit returns `exec.ErrDot`. Treat as not found for bare names. `~` must be expanded first; I grepped for an existing expand helper in config/utils and found none, so use `os.UserHomeDir` + `filepath.Join`. | VERIFIED (none found) / INFERRED (ErrDot) |
| Output cap | Custom `io.Writer` (`capWriter{buf, max=256<<10}`) assigned to `cmd.Stdout` and `cmd.Stderr` (merged: many CLIs print help to stderr). On overflow, drop the excess and set `truncated`. It must keep returning `len(p), nil` so the child does not get SIGPIPE, or return an error and cancel the ctx to kill early. `io.LimitedReader` only wraps readers, so it does not fit `cmd.Stdout`. | INFERRED (standard os/exec semantics) |
| Stdin closed | Leave `cmd.Stdin = nil` (os/exec connects it to /dev/null). | INFERRED |
| Sanitized env | `cmd.Env = []string{"PATH="+os.Getenv("PATH"), "HOME="+home, "TERM=dumb", "NO_COLOR=1", "LANG=C"}`. Do not inherit secrets such as `GITHUB_TOKEN`/`ANTHROPIC_API_KEY`. Setting `TERM=dumb` and `NO_COLOR` avoids ANSI in help output. Strip ANSI in the parser anyway. | INFERRED |
| Cache | `sync.Map` keyed by `struct{path string; mtime int64; size int64}`, storing the parsed result. Stat via `os.Stat(resolvedPath)`. Follows symlinks, so the target's mtime is used. Add singleflight (`golang.org/x/sync/singleflight`, module already in go.mod but marked indirect, so `go mod tidy` would make it direct) to dedupe concurrent probes of one binary. Unbounded growth is negligible (one entry per distinct binary). | VERIFIED (module present) / INFERRED (design) |
| Existing Setpgid users | `executor/managed_process_darwin.go:15-24`, `executor/shortlived.go:41,93`, `executor/managed_process.go:100` | VERIFIED |

### Portability of process-group kill
- Linux/macOS: `Setpgid: true` plus `syscall.Kill(-pgid, sig)` works on both; existing `safeexec_pg.go` is the POSIX path.
- Windows: `safeexec_pg_windows.go:19` is a no-op shim delegating to `CommandContext` (plain child kill, no group). Grandchildren may survive on Windows. Job Objects or `CREATE_NEW_PROCESS_GROUP` would be needed for parity; out of scope since the app is Linux/macOS-first (see repo `CLAUDE.md`). Reusing `safeexec.CommandContextPG` gets build-tag handling for free, so the probe needs no build-tagged files of its own.
- Do not use it for TTY programs (documented at `safeexec_pg.go:26-28`); a help probe has no TTY so this is fine.

### Proto / RPC conventions (VERIFIED)
- All RPCs are on `SessionService` (`proto/session/v1/session.proto:11`), including program config: `ListProgramsConfig`/`UpsertProgramConfig`/`DeleteProgramConfig` at `:565-571`. Messages live near `ProgramConfigProto` (`:2526`, fields command=3, cli_flags=4).
- Handler lives on `DefaultsService` (`server/services/defaults_service.go`; type at `:27`, constructor `:62`). Handler signature and marker: `// +api: program_config:list` comment then `func (d *DefaultsService) X(ctx, req *connect.Request[sessionv1.XRequest]) (*connect.Response[sessionv1.XResponse], error)` (`:667-690`).
- Add `rpc ProbeProgram(ProbeProgramRequest) returns (ProbeProgramResponse) {}` next to `:571`, and messages `ProbeProgramRequest{command}`, `ProbeProgramResponse{found, resolved_path, repeated ProgramFlag flags, truncated?, error_message?}`, `ProgramFlag{name, short, takes_value, description}`. Use a `// +api: program_config:probe` marker, then `make proto-gen` (gen/ is gitignored) and `make registry-generate`, and commit only the registry files (per repo `CLAUDE.md`).
- `defaults_service.go` is 794 lines; put probe logic in a new package (suggest `session/proghelp` or `config/programprobe`) and keep the handler a thin adapter, to limit growth of a large file. Inject the probe into `DefaultsService` via a setter like `SetSharedBacklogConfig` (`:~70`), nil-safe.
- Missing binary: return `found=false` with a normal response, not `connect.NewError` (requirement 2).
- Security gate (req 8): the handler must take the command from the request only, run `--help` only, and never a shell. Consider rejecting a request whose first token is empty or has NUL bytes.

## Frontend (web-app/, pnpm)
- Package manager pnpm 10.27.0 (`web-app/package.json:5`); Next 15.3.2 and React ^19 (`:97-98`); vanilla-extract (`@vanilla-extract/css ^1.20.1`, `recipes ^0.5.7`, `:82,146`). Styles are colocated `*.css.ts` files.
- RPC client: `createClient(SessionService, getConnectTransport())`, used in `ProgramsManager.tsx:7,77`. After `make proto-gen` the new `probeProgram` method appears on that client.
- Debounce: `web-app/src/lib/hooks/useDebounce.ts` already exists (`useDebounce<T>(value, delay)`). No new package needed. Requirement 5 says probe on blur, so debounce is only needed for the flags-input validation (local, no RPC) and, if wanted, typing in the command field. Use blur-triggered RPC plus a request-sequence ref to drop stale responses.
- Autocomplete: `web-app/src/components/ui/AutocompleteInput.tsx` exists (props `id, value, onChange, onBlur, suggestions: string[], filterFn`, `:1-30`), with a test file. Prefer extending it over a native `<datalist>`: datalist cannot show per-option descriptions and its behavior on mobile browsers is inconsistent (INFERRED). It filters with a substring match by default; a custom `filterFn` prop exists so flag-token-aware filtering (only the last whitespace-separated token) can be passed in. Not verified whether it supports multi-token input; may need a small extension.
- Tooltip: `web-app/src/components/ui/Tooltip.tsx` wraps `@radix-ui/react-tooltip ^1.2.8` (`package.json:77`), hover/focus only, `delayDuration=400` (`Tooltip.tsx:~10`). Radix tooltips do not open on tap for touch devices (INFERRED from Radix design; verify). For req 9, use a tap-to-toggle disclosure instead: an `aria-expanded` info button that toggles an inline description, or `@radix-ui/react-popover` (not installed; `react-dialog` is). Simplest with zero deps: inline description text under each selected/unknown flag chip, toggled by tap.
- Touch targets: follow repo mobile+desktop rule (memory `feedback_mobile_desktop_ux`); at least 44px hit area for the info button.
- Form integration point: `ProgramsManager.tsx` inputs `prog-command` (`:313-321`) and `prog-flags` (`:326-334`), test ids `prog-command-input`, `prog-flags-input`. Session-creation program picker: not located (grepping `ProgramSelect|selectedProgram` returned nothing usable); likely inside Omnibar (`web-app/src/components/sessions/Omnibar.tsx`) per `docs/reference/session-creation-registry.md`. That doc needs reading before planning req 7 (UNVERIFIED).
- Tests: jest via `cd web-app && npx jest --no-coverage` (repo `CLAUDE.md`); jscpd gate has only 0.10% margin (memory `project_plan_feedback_ship_learnings`), so build one shared `useProgramProbe` hook and one `FlagAutocomplete` component used by both forms rather than duplicating.

## Suggested shape
1. `probe` package: `Probe(ctx, command) Result` with injected runner and `sync.Map` cache; pure `ParseHelp(text) []Flag` (regex, GNU/argparse/cobra/clap) with fixtures under `testdata/`.
2. Runner: `safeexec.CommandContextPG` + capWriter + sanitized env + probe-specific fast SIGKILL cancel.
3. Proto RPC + thin handler in `DefaultsService`.
4. `useProgramProbe(command)` hook (blur-triggered, stale-response guard) and a shared flag input component; reuse `AutocompleteInput`.

## Open items / gaps
- Whether `executor/shortlived.go` can serve as the runner (unread beyond option list).
- Whether `AutocompleteInput` handles multi-token flags input.
- Session-creation program picker location.
- Radix tooltip tap behavior on iOS/Android not tested.
