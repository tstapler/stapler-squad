# Go idiom + concurrency review: cli-flag-discovery

Scope: `git diff main...HEAD` for config/clihelp (non-test and test), server/, main.go.
Verified by opening the code. Also ran `go test -race -count=3 ./config/clihelp` (ok, 23.8s), `GOOS=windows go vet ./config/clihelp` (clean), `GOOS=darwin go build` (ok).
The skills (golang-development/concurrency/safety) were not invoked; standards applied from memory.

Counts: MUST FIX 0, SUGGEST 6, NITPICK 8.

No MUST FIX items. Semaphore release (`defer` at prober.go:106), leader/follower ctx handling, capWriter single-goroutine claim, and Wait/kill ordering all hold up.

## SUGGEST

1. config/clihelp/execute.go:46-48 and prober.go:98 - flight body has no `recover`. `singleflight.DoChan` runs `fn` in its own goroutine and re-panics there on a panic, so a panic in `ParseHelp`/`classify`/an injected `RunFunc` kills the whole server process instead of being caught by connect's handler. The parser is fuzzed, so the risk is low, but this is a public RPC fed by untrusted program output. Fix: wrap the body in `defer func(){ if r := recover(); r != nil { res = ProbeResult{Status: ProbeStatusError, ResolvedPath: path} } }()` (named return) and log it. Verified: read execute.go:46, and DoChan semantics (goroutine panics are unrecoverable by callers).

2. config/clihelp/runner.go:132 and loginpath_unix.go:50 - `killGroup` after `cmd.Wait()` signals `-pid` once the leader has been reaped. If no group member remains, the pgid is free and the kernel may reuse it, so the SIGKILL can hit an unrelated process group (rare but a kill of a stranger). Under Setsid the pgid stays reserved only while some member is alive. Fix: probe with `syscall.Kill(-pid, 0)` first (ESRCH means the group is gone, skip the kill), or accept the window. The ADR mandates the post-Wait kill, so at minimum add a one-line comment naming the pid-reuse window. Verified: read both call sites; kill happens strictly after Wait returned.

3. config/clihelp/runner.go:92-94 - `Limits` with only one field set is silently replaced by `DefaultLimits()` for both fields (`Limits{Timeout: 10s}` becomes 3s/256KiB). Surprising for a zero-value-usable struct. Fix: default each field independently (`if lim.Timeout <= 0 { lim.Timeout = def.Timeout }` and likewise for MaxBytes). Verified: read runner.go:90-94.

4. config/clihelp/prober.go:22-39 - `Prober` has no usable zero value (nil `cache`, `confirmed`, `sem`, `stat`, `now`), and `DefaultsService.SetProber(nil)` (defaults_service.go) would nil-deref in `StartProgramProbeLoginPath`/`ProbeProgram`. Only tests call `SetProber`, so low risk. Fix: document "construct with NewProber" on the type, and have `SetProber` ignore nil. Verified: read prober.go and defaults_service.go diff.

5. config/clihelp/execute.go:30-48 and prober.go:154-175 - TOCTOU window between stat/`readHead` (native-binary check) and the exec: a file can be swapped from native to script after `isNativeFile` passes, bypassing the confirm gate. Post-run `stat` (prober.go:113) only protects the cache, not the execution. The precondition is write access to the file's directory (the file itself is not world-writable, but the parent dir is not checked). Fix: also reject when the parent directory is world-writable in `usableExecutable`/`locate`; accept the residual same-user race and say so in ADR-001. Verified: read locate/usableExecutable/mayRun/flight.

6. config/clihelp/prober.go:198-214 (`logProbe`) - logs with `context.Background()`, dropping trace/request correlation the caller's ctx carries. Fix: pass `ctx` to `logProbe`. Cheap; audit line is the ADR-001 record so correlation is worth having. Verified: read Probe/logProbe.

## NITPICK

1. config/clihelp/execute.go:51 - unchecked `r.Val.(ProbeResult)` and `r.Err` ignored. Safe today (the closure always returns `ProbeResult, nil`), but a checked assertion (`res, ok := r.Val.(ProbeResult)`) returning `ProbeStatusError` costs nothing.

2. config/clihelp/execute.go:39-45 - `mayRun` records consent (`confirmed.add`) before the `ctx.Err()` check, so a cancelled Check click still records consent for that (path, mtime, size). Move the ctx check above `mayRun`, or accept because the user did click Check.

3. config/clihelp/execute.go:46 - flight key omits `opts`, and the leader's `opts` govern `cachedFor` inside the flight. A ConfirmExecute follower that joins a non-confirm leader can receive a cached TIMEOUT instead of a retry. Narrow window; only matters for the "Check retries timeout" rule.

4. config/clihelp/loginpath.go:141-144 - every `Dirs()` call during a retry-due window spawns a `go s.Refresh(...)`; Refresh dedups via `inflight`, so it is correct but a burst of lookups spawns a burst of short goroutines. Set `inflight = true` under the lock in `Dirs` (or gate the spawn on it) to spawn at most one. Also with `errNoLoginShell` (SHELL unset or `sh`) this retries every 30s forever; treat that error as permanent.

5. config/clihelp/loginpath_unix.go:14-26 - `limitedBuffer` duplicates `capWriter` (capwriter.go:9-28) minus the overflow hook. Reuse `capWriter{max: ...}` (nil `onOverflow`) to avoid two near-identical writers (dupl may flag it later).

6. config/clihelp/loginpath.go:38-46 - script uses `-c` (not `-l`), so it is "interactive-ish rc sourcing", not a login shell; names and comments say "login". The comment already says it mirrors config/config.go, so this is naming only.

7. config/clihelp/parser.go:124-129 - duplicate-flag merge drops `Aliases` of the discarded entry; and `parseForm` reports `--foo=` (empty hint) as not taking a value (parser_lines.go:78 + 64). Both minor precision losses; add a table row if intended.

8. server/server.go:414-421 - `append(ConnectOptions(...), ...)` appends onto a slice returned by another function; safe here because `ConnectOptions` builds a fresh literal each call, but prefer `slices.Concat` or a named `extra` slice to avoid aliasing if that function ever returns a shared slice.

## Verified-OK (no action)

- Semaphore: try-acquire inside the flight body with `defer` release (prober.go:104-109) covers every return, including `run` returning an error; the followers share the leader's result, cancelled callers cannot raise the process count.
- `context.WithoutCancel` flight + caller `select` on `ctx.Done()` (execute.go:46-55): flight bounded by `Limits.Timeout` + `waitDelay`; `DoChan` channel is buffered by singleflight, so no goroutine leak (asserted by goleak tests, prober_run_test.go:209-251).
- capWriter: `min/max` arithmetic correct, exactly-max output is not truncated, `onOverflow` fires once, never returns a short write; single copy goroutine holds because `Stdout == Stderr` (same pointer), and `-race -count=3` passes.
- runner ordering: `Wait` then read of `cw.buf` is safe (Wait joins copy goroutines, or closes pipes and then returns after `WaitDelay`); `Cancel` override replaces safeexec's SIGTERM-then-delayed-SIGKILL, matching ADR-002; `context.Canceled` yields an error, only `DeadlineExceeded` sets TimedOut.
- Cache: `sync.Mutex` + map with oldest-insertion eviction is the right primitive (bounded, needs ordered eviction; `sync.Map` cannot do it). Non-cacheable statuses correctly skipped.
- Build tags: `!windows` / `windows` file pairs are symmetric (`newProbeCmd`, `killGroup`, `runShellScript`); `GOOS=windows go vet` is clean.
- Error wrapping: the only wrapped errors use `%w` (runner.go:99,106,129); statuses, not errors, cross the package boundary by design, so no `errors.Is/As` targets are needed.
- Tests: timeouts use `wait.ScaleTimeout` and `RequireEventually`; no bare sleeps beyond the helper subprocess (runner_helper_test.go:61, inside the child, 200ms). No flakiness seen over 3 race runs.
- Complexity: longest functions are `runWith` (about 52 lines) and `mayRun`/`execute` (about 30); none near gocognit 40. `runWith` is the closest to funlen and could split out `buildCmd`, optional.
- ProbeGuard (server/middleware/probeguard.go): POST-only, loopback-bound, Host and Origin checks; `ProbeProgram` is reachable only via the `/api` + procedure path (the single registration at server.go:435), so exact-path matching is sufficient.
