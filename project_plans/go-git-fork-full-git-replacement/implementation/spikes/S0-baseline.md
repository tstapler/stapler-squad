# S0 baseline: git spawn count and `session` test wall time (Story 0.1.2)

Measured 2026-10-08, branch `chore/sdd-go-git-fork-planning` @ `a7f0809ef`, macOS arm64 18-core, go1.26.6. All numbers VERIFIED from the run artifacts in `/tmp/s0/` (not committed).

## Method (exact)
- Shim `/tmp/ssq-shim/git` (POSIX sh): appends the first non-flag subcommand (skipping `-C <dir>`/`-c k=v`) to `$SSQ_GIT_SPAWN_LOG`, then `exec /usr/bin/git "$@"`. Kept in /tmp, not in the repo (no product changes). The plan's path `scripts/git-spawn-shim/git` is not created yet.
- Driver `/tmp/s0/run.sh`: `go test -c -o /tmp/s0/session.test ./session` once (so wall time excludes compilation), then in `./session` with `env -u STAPLER_SQUAD_TEST_DIR -u STAPLER_SQUAD_INSTANCE` (the agent shell had both set; unset in script):
  `/usr/bin/time -l /tmp/s0/session.test -test.count=1 -test.timeout=20m -test.v` (runs strictly sequential, one at a time).
- Runs: shim1-3 (PATH=shim first, real git = /usr/bin/git), pathgit1 (PATH git = `~/.local/bin/git` dotfiles wrapper, no shim), mutex1 (/usr/bin/git first on PATH, `-test.mutexprofile -test.mutexprofilefraction=1`).

## Results
| Run | Wall (s) | user | sys | Load avg (1 min) at start | Exit |
|---|---|---|---|---|---|
| shim1 | 72.10 | 27.39 | 45.58 | 11.91 | 1 (known failure only) |
| shim2 | 61.88 | 25.88 | 45.06 | 18.44 | 1 |
| shim3 | 61.21 | 25.15 | 45.90 | 15.86 | 1 |
| pathgit1 (wrapper git, no shim) | 82.48 | 27.00 | 45.06 | 16.96 | 1 |
| mutex1 (profile fraction=1, adds overhead) | 75.26 | 27.03 | 48.27 | 31.91 | 1 |

- Spread over 3 shim runs: wall 61.2 to 72.1 s (median 61.9). Max RSS 307 to 338 MB. user+sys is the test process tree (children included, `/usr/bin/time -l`), about 70 s CPU.
- Wall times are at or below the post-#955 expectation (72 to 92 s), but load was 12 to 32 throughout (other agents, CrowdStrike, Chrome), so treat as noisy; the wrapper run (82.5 s) vs shim runs differ by about 13 ms x spawns (1006 x 13 ms = 13 s) plus noise.
- Only failing test in all 5 runs: `TestSessionRestartWithConversationContinuity` (10 to 13 s each; the known pre-existing failure). 3404 PASS lines each.
- **Git spawn count (via PATH shim): 1006 in every one of the 3 runs (deterministic).**
  By subcommand: rev-parse 447, config 135, fetch 79, commit 53, add 51, diff 40, init 33, checkout 32, remote 27, merge-base 20, worktree 19, ls-remote 18, branch 16, status 11, ls-files 8, clone 8, version 2, merge 2, check-ref-format 2, push 1, mv 1, log 1.
- **Summed mutex delay** (mutex1, `go tool pprof -top /tmp/s0/session.test /tmp/s0/mutex.prof`): 51.62 s total contention delay (summed across goroutines, so it exceeds wall time). Single run, fraction=1.

## Caveats
- The shim only sees spawns that resolve `git` through PATH. Code that execs an absolute `/usr/bin/git`, or tests that `t.Setenv("PATH", ...)` to a stripped value (20 sites per the audit), are not counted: 1006 is a floor for PATH-resolved spawns. Not cross-checked with a second method (e.g. `dtruss`/EndpointSecurity needs sudo).
- Not done: spawn count for a full live create -> work -> pause -> cleanup cycle. It needs a separate manual instance (CLAUDE.md "Manual/interactive testing", ports 62871/62872, private tmux socket) driven through the UI/MCP; no non-interactive harness exists, and the live service must not be touched. Recommend doing it when Phase 5 has a harness.
- Mutex delay was measured once and with fraction=1, which perturbs timing (wall 75 s vs 61 to 72 s).
- `pprof` top for mutex is dominated by `sync.(*Mutex).Unlock` (43 s flat), not attributed to a repo function here; no per-package breakdown done.
