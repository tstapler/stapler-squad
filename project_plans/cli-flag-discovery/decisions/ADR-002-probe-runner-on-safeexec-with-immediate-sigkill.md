# ADR-002: Probe runner built on `safeexec.CommandContextPG` with immediate SIGKILL

**Status**: Accepted (coordinator decision)
**Date**: 2026-09-21 (revised after review)

## Context
The probe needs: process-group kill, closed stdin, replaced env, empty cwd, a 3s hard timeout, and a 256KB combined output cap that keeps draining or kills the child. `norawexec` forbids raw `exec.Command` outside `executor/` and `executor/safeexec` (`.golangci.yml`).

- `executor.ShortLivedCmd` (`executor/shortlived.go:97-249`) supports timeout, `WithDir`, `WithReplaceEnv`, nil stdin and process groups, but its outputs (`Run`/`Output`/`CombinedOutput`, `:166-231`) buffer unboundedly; no capped writer.
- `safeexec.CommandContextPG` (`executor/safeexec/safeexec_pg.go:32-74`) returns a plain `*exec.Cmd` with `Setpgid: true`. Its `cmd.Cancel` sends SIGTERM then SIGKILL after `sigkillGrace` (5s, `:23`), exceeding the 3s limit. `WaitDelay` defaults to 2s (`executor/safeexec/safeexec.go:26`). The file warns that `Setpgid` without `Setsid` can cause SIGTTIN/SIGTTOU when a terminal is involved (`safeexec_pg.go:29-31`).

## Decision
Build the `exec.Cmd` with `safeexec.CommandContextPG`, then override on the returned struct:
- `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}` (own session and group, no controlling terminal).
- `cmd.Cancel` = immediate `syscall.Kill(-pid, SIGKILL)` (ESRCH ignored).
- `cmd.WaitDelay = 200ms`, so worst-case wall time is about `timeout + 200ms`.
- `cmd.Stdout = cmd.Stderr =` one `capWriter` (`os/exec` uses one copy goroutine for identical writers, so no mutex); on first overflow the writer kills the group (no burning the full timeout).
- After `Wait` returns, on every path, kill the group once more so backgrounded grandchildren do not survive a normal exit.
- `context.Canceled` yields an error, never a partial "success"; only `DeadlineExceeded` sets `TimedOut`.

The SIGKILL/Setsid code lives in `//go:build !windows` files; the Windows fallback is a closure `func() error { return cmd.Process.Kill() }`. No change to `executor/` or `safeexec`.

**Prober concurrency.** Probes coalesce through `singleflight.Group.DoChan` keyed by the cache key. The flight body runs on `context.WithoutCancel(ctx)` (bounded by `Limits.Timeout`), so a cancelled leader cannot poison followers or cache a sticky `ERROR`/`FOUND_NO_FLAGS`; each caller `select`s on the result channel or its own `ctx.Done()` and a cancelled caller returns `ERROR` uncached. The 2-slot semaphore is try-acquired inside the flight body (leader only) and released when the flight ends, so followers share the result rather than each consuming a slot, and a cancelled caller cannot let more than 2 processes run. The flight goroutine is bounded (timeout + `WaitDelay`) and the `DoChan` channel is buffered, so it exits even when every caller has cancelled (asserted by a `goleak`/`Eventually` test).

**Login-shell PATH runs on the same runner.** `deriveLoginPath` uses `runWith` (Setsid, group kill, capped output, 2s) with an `inheritEnv` spec, not bare `safeexec.CommandContext`.

**Test seam.** The runner core is an unexported `runWith(ctx, runSpec{path, args, extraEnv, lim})`. Production `Run` is exactly `runWith(ctx, helpSpec(path, lim))` with `args=["--help"]` and `extraEnv=nil`; only `_test.go` files build other specs. `runWith` accepts extra env entries only with the `CLIHELP_TEST_` prefix, which lets a re-exec'd test binary (same convention as `executor/safeexec/safeexec_sigkill_helper_test.go`) receive its mode variable despite the sanitized allowlist env. `probeEnv(parent)` takes the parent environment as a parameter so the no-leak assertion needs no `t.Setenv`.

**Injection shape.** `RunFunc = func(ctx, ResolvedPath, Limits) (RunOutput, error)`; the prober passes its `Limits` on every call, so fakes injected via `WithRun` can assert limit plumbing.

## Alternatives rejected
- **`ShortLivedCmd`**: cannot cap output; would widen a shared package for one caller.
- **Lower `sigkillGrace`**: unexported, shared by all callers.
- **`cmd.Output()` with an `io.LimitReader`**: does not fit `cmd.Stdout` and blocks the child on a full pipe.

## Consequences
- A probe emits no `executor` audit entry; the Prober's `program_probe` Info log line (ADR-001) is the audit record.
- **Accepted limitation: no parent-death cleanup.** Overriding `SysProcAttr` wholesale drops `Setpgid` and the Linux `Pdeathsig` handling `CommandContextPG` sets (`executor/safeexec/safeexec_pdeathsig_linux.go`). If the server is killed mid-probe, the child runs to the end of its `--help` (at most the 3s timeout is no longer enforced by us) rather than being killed with the parent. Accepted: the run is short, low-privilege (empty env and cwd), and the alternative (`Setpgid` without `Setsid`) reintroduces the SIGTTIN risk.
- Windows falls back to plain child kill (grandchildren may survive) and the re-exec tests are `!windows`; acceptable since the app is Linux/macOS-first.
