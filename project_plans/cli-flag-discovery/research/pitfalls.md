# Pitfalls: cli-flag-discovery

Confidence labels: VERIFIED = opened/ran in this worktree; INFERRED = reasoned, not run.

## 1. RPC that execs a caller-supplied path (RCE-ish surface)
- Risk: `ProbeProgram(command)` runs whatever binary the caller names. The :8543 listener has no auth (`SetupAuth` is only wired for the remote server: `main.go:1538` passes `middleware.Auth(sessions)` to `srv.StartRemote`, `server/middleware/auth.go:16-39`). Remote clients on :8444 are passkey-gated, but any authenticated remote client (or any local process / DNS-rebinding browser tab against :8543) could exec arbitrary binaries with `--help`, plus relative paths and `./x` in the cwd. VERIFIED (auth wiring); rebinding exposure INFERRED.
- Mitigation:
  - Enforce AC8 server-side, not just in the UI. Only probe (a) the `command` of a saved `ProgramConfig`, or (b) the first token of a command that resolves through `exec.LookPath`/absolute path to a regular, executable file.
  - Reject empty tokens, tokens containing NUL/newline, and relative paths with a slash. Never run via `sh -c`.
  - Pass args as exactly `[]string{"--help"}`. Do not forward the user's other args, so `rm`-style destructive flags are impossible.
  - Set `cmd.Dir` to a fresh empty temp dir so a PATH-relative `.` entry cannot hijack.
  - Log each probe through the existing audit path (`executor/audit.go`).
  - Rate-limit per resolved path so the cache also caps repeated execs.
  - Note in docs that on remote access the probe is authenticated, not sandboxed.

## 2. Zombies, orphans, goroutine leaks on timeout
- Risk: `exec.CommandContext` default kills only the child. Wrappers such as `node`/`npx`/shell scripts spawn grandchildren that keep the stdout pipe open, so `Wait` hangs past the deadline. Also easy to forget `Wait` after `Kill`, leaving a zombie. Same failure mode documented in `executor/safeexec/safeexec_pg.go:35-40`. VERIFIED.
- Mitigation: reuse `safeexec.CommandContextPG` (Setpgid, `cmd.Cancel` SIGTERMs the group, `WaitDelay` escalates to SIGKILL after `sigkillGrace`=5s, `safeexec_pg.go:22`) or `executor.ShortLivedCmd` (`executor/shortlived.go`, which has `WithTimeout`, `WithDir`, env replacement, stdin nil = /dev/null). Note the 3s hard timeout plus 5s grace means the RPC could take up to 8s; either shorten the grace for this call or return after 3s. Windows has separate stubs (`safeexec_pg_windows.go`), so keep the build tag parity. Always `Wait` on every path. Test with `goleak` (per `golang-testing` skill) and by asserting the process group is gone.

## 3. Pipe deadlock with the 256KB output cap
- Risk: stopping reads at 256KB leaves the child blocked on a full pipe (64KB buffer). The timeout then kills it, but until then it holds a goroutine and the process. `io.LimitReader` alone hides this.
- Mitigation: use a capped writer as `cmd.Stdout`/`cmd.Stderr` that keeps the first 256KB and discards the rest, so `exec` keeps draining and the child never blocks. Set a `truncated` flag and, optionally, kill the group once the cap is exceeded. Merge stderr into the same capped buffer, since many tools print help to stderr (argparse on error, some Go tools). Cap the combined total, not each stream separately. Test with a fixture that writes >1MB and assert prompt return and no leaked goroutines.

## 4. Environment sanitization
- Risk: inheriting the full env leaks secrets (`GITHUB_TOKEN`, `ANTHROPIC_API_KEY`) to an arbitrary binary and can change help output or launch interactive behavior.
- Mitigation: build the env from an allowlist: `PATH`, `HOME`, `LANG`/`LC_ALL=C.UTF-8`, `TERM=dumb`, `NO_COLOR=1`, `CI=1` (discourages TUIs/prompts), `COLUMNS=200` (avoids wrapped descriptions), `USER`. Use `ShortLivedCmd`'s `replaceEnv`. Drop `LD_PRELOAD`/`DYLD_*`. Keep `PATH` identical to what tmux sessions use, or `found=true` from `LookPath` may differ from what the session will actually run. Verify that the service's PATH under systemd/launchd (minimal) matches the session's PATH; INFERRED risk of false "not found" for brew/asdf/nvm-installed tools when the service runs with a thin PATH.

## 5. Programs that ignore `--help`
- Risk: TUIs, REPLs, or tools that treat `--help` oddly (start a server, wait for a tty, open a browser) or exit non-zero after printing help. The existing findings note this (`research/findings.md:11`).
- Mitigation: closed stdin, no tty, timeout, kill the group. Treat non-zero exit with usable output as success. Treat timeout or empty output as `flags=[]`, not an RPC error (AC4). Probe only on explicit blur/select, never on each keystroke.

## 6. Cache staleness (symlink vs target)
- Risk: `os.Lstat` or `os.Stat` on the unresolved path gives the symlink's own mtime (Lstat), or stays the same when a shim/symlink (`/usr/local/bin/claude -> .../versions/1.2.3/claude`, asdf/brew shims) repoints. Then the cache serves old flags after an upgrade.
- Mitigation: `filepath.EvalSymlinks(resolved)` then `os.Stat` on the final target. Key on `(realpath, mtime, size)`. Also re-key when `ProbeProgram` sees a different resolved path. Shims (asdf, pyenv, mise) resolve to a shim binary whose mtime never changes while the underlying tool version does; either include a short TTL (e.g. 10 min) or document the limit. Bound the map size (use LRU or cap entries) rather than unbounded `sync.Map`, and use singleflight so concurrent blur events do not spawn duplicate execs. Cache negative results (timeout) briefly so a hanging binary is not re-run on every blur.

## 7. UI race: stale probe responses while typing
- Risk: responses arrive out of order; the indicator shows "found" for `clau` after the user has typed `claude-x`. Session-creation and settings forms both hit this. Also possible: probing a partial path on each change.
- Mitigation: probe on blur/select only, or debounce (~400ms). Track a request counter/token in the hook and drop responses whose token is not latest; pass an `AbortSignal` to the ConnectRPC call (`{signal}` call option) and abort on change/unmount. Reset the indicator to "checking" state when the command changes. Test with jest fake timers plus two deferred promises resolved out of order. Because both forms need it, put the logic in one hook (`useProgramProbe`) to avoid the jscpd duplicate (see 11).

## 8. Regex catastrophic backtracking
- Risk: Go's `regexp` is RE2 (linear time), so classic ReDoS does not apply. VERIFIED by language spec (no backreferences/lookaround). The real risks are quadratic behavior in hand-written loops (e.g. repeated `strings.Index` on a 256KB blob with no newlines) and per-line cost with pathological lines.
- Mitigation: parse line by line with `bufio.Scanner` (set `Buffer` larger than the default 64KB token limit, or the scanner errors on one long line and silently truncates: `bufio.ErrTooLong`); skip lines longer than ~2KB; cap total flags (e.g. 500) and description length; add a fuzz test (`go test -fuzz`) and an adversarial fixture (256KB of `-` / single line) with a time bound. Do not use a third-party parser (findings decision). Anchor regexes (`^\s*-`).

## 9. Flaky tests from sleeping scripts
- Risk: tests that write `sleep 10` shell scripts to test the timeout are slow and load-sensitive; repo skills `deterministic-fast-tests` and `fix-flaky-tests-dont-defer` forbid this. Also, the existing 5s `sigkillGrace` is a var that tests can shorten (`safeexec_pg.go:22`).
- Mitigation: inject the `CommandExecutor` (`config/executor.go:13`) or a `runHelp func(ctx, path) ([]byte, error)` seam so the parser and cache logic are tested with fakes and no process. For the one real-process timeout test, use a sub-100ms configurable timeout with the test binary re-exec pattern (helper process via `os.Args[0]` with env flag, as `executor/safeexec/safeexec_sigkill_helper_test.go` does) rather than `sh -c sleep`. Make the timeout and cap package vars/options, not constants. Portability: skip or stub on Windows. Use `t.TempDir()` scripts, `t.Parallel()` only when isolated. Oversize-output test needs no sleep (child writes and exits).

## 10. Generated proto and registry
- Risk: `gen/`, `web/src/gen/`, `web-app/src/gen/` are gitignored (`.gitignore:33-35`), so a `git status` after `make proto-gen` shows nothing and someone may try `git add -f`. Tests fail locally until `make build` regenerates. Adding an RPC also requires `make registry-generate` and committing the changed `docs/registry/features/*.json`; `// +api:` marker in the handler. Optional edits must stay backward compatible (add fields/RPC, no renumbering).
- Mitigation: commit only `session.proto` and registry files; run `make build` before tests; run `make registry-generate` and commit the result; never `git add -f` generated code (the CLAUDE.md warns about this for ent too).

## 11. Lint and duplication gates
- Limits (VERIFIED, `.golangci.yml:37-43,62-63`): gocyclo min-complexity 25, gocognit 40, funlen 150 lines / 100 statements, dupl threshold 150 tokens; all enforced new-code-only via `--new-from-rev=origin/main` in CI (`.github/workflows/lint.yml`), and test files are excluded (`.golangci.yml:176-184`). jscpd: minLines 20, minTokens 200, threshold 0.12% (`web-app/.jscpd.json`); the plan-feedback memory notes only ~0.10% margin was left after PR #815, so a new heavily-mocked jest file can tip it.
- Mitigation:
  - Parser: split into small pieces (`splitLine`, `parseFlagToken`, `parseValueHint`, `joinContinuationLines`) so no function nears gocognit 40; a single big line-classifier `switch` with many regex branches is the likely offender. Table-driven format handlers avoid dupl between GNU/cobra/clap styles.
  - Frontend: share one hook and one indicator component between ProgramsManager and session creation instead of copying; keep jest.mock blocks minimal and reuse a shared test helper/mock module (jest.mock hoisting means it must be the mock factory file, not a helper function). Run `pnpm run lint:duplicates` in `web-app/` before pushing. Use pnpm, not npm.
  - Run `make ready-complexity-gate` and `make ready` locally.

## 12. E2E conventions (CI-checked)
- Risk: a spec violating any convention fails CI. Rules from CLAUDE.md and `e2e-test-conventions` skill: first line `// @feature ...` header (add the new feature ID to the registry, e.g. `program:probe`); no `waitForTimeout`; `data-testid`/ARIA locators only; page helpers in `tests/e2e/pages/`.
- Mitigation: assert the indicator with `expect(locator).toHaveText(...)` (auto-wait). The e2e server spawns with its own env, so it may lack the tools you probe; probe `sh` or the test binary or a fixture script created in the test dir, not `claude`. Add `data-testid` for the indicator, flag suggestions, and tooltip. Tooltip must work on tap (AC9): use a click/focus-triggered popover (`aria-describedby`), not `title=`/hover; touch targets >= 44px.

## 13. Other traps
- `~` expansion: `exec.LookPath` does not expand `~`; expand the first token manually (`os.UserHomeDir`) before LookPath. Quoted first tokens with spaces (`"/Applications/My App/x"`) need shell-word splitting (use `shlex`-style already in repo, if any; else a small quote-aware splitter). Env-var prefixes (`FOO=1 claude`) make the first token not a binary: skip or strip leading `KEY=VAL` tokens.
- Unknown-flag warnings must be non-blocking and must not fire when the parse returned zero flags (a parse failure would otherwise flag every flag as unknown). Also handle `--flag=value`, combined short flags (`-abc`), `--no-` negations, and flags the help omits (hidden flags): warn only, never block.
- Executable check: `LookPath` returns success for anything with the x bit, including directories on some platforms (fixed since Go 1.x, but Stat and check `IsRegular` after resolving).
