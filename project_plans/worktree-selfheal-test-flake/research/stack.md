# Research: Stack — worktree-selfheal-test-flake

Agent 1 (Stack) findings for backlog item a60ce219-38ac-43c8-81bb-5e5e69704865.

## 1. Existing flake-reproduction tooling in this repo

- **`go-stress` (`golang.org/x/tools/cmd/stress@v0.47.0`)** is the repo's established
  amplified-repetition mechanism, used today in the `pty-race-regression` CI job
  (`.github/workflows/build.yml:400-438`). Pattern: compile a `-race` test binary
  (`go test -race -c -o server.test ./server`), then run it under `stress` for a fixed
  wall-clock budget (`timeout 90s stress ./server.test -test.run='<regex>' || [ $? -eq 124 ]`
  — exit 124 = ran clean for the whole budget = success). Comment at build.yml:413-421
  explicitly frames this as a replacement for serial `go test -count=20`: same goal
  (repeat until a timing bug surfaces) but packs far more repetitions into the budget by
  running in parallel across all CPUs, and fails fast on first bad run. This is the
  precedent to reuse for reproducing `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`
  rather than inventing a new mechanism:
  ```
  go install golang.org/x/tools/cmd/stress@v0.47.0
  go test -race -c -o worktree_ops.test ./session/git
  timeout 90s stress ./worktree_ops.test \
    -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate' \
    || [ $? -eq 124 ]
  ```
  `stress` is **not** wired into `Makefile`'s `install-tools` target (checked: no `stress`
  hits in Makefile or `tools.go`) — it's currently CI-workflow-only tooling, installed
  ad hoc in the `pty-race-regression` job step. `tools.go` (`//go:build tools`) only tracks
  `buf`, `protoc-gen-connect-go`, `protoc-gen-go` — no test-stress tool is build-tag-tracked.

- **`make test-race`** (Makefile:531-534) is the closest existing Make target: runs
  `go test -race -short -timeout=20m -p 1 ./session ./session/mux ./session/tmux ./testutil`
  for the tmux-heavy packages, then a second invocation for everything else at default
  parallelism. `session/git` falls into the second (non-`-p 1`) invocation, so it runs
  with default per-package parallelism under `-race`, not serialized.

- CI's actual invocation covering `session/git` — the `test` job's "Run tests with
  coverage" step (`.github/workflows/build.yml:207-217`):
  ```
  TMUX_BIN="$(pwd)/bin/tmux" go test -race -p 1 -timeout=20m -coverprofile=coverage.out \
    -covermode=atomic ./server/... ./session/... ./config/... \
    || (TMUX_BIN="$(pwd)/bin/tmux" go test -race -timeout=20m -v ./...; exit 1)
  ```
  Note `-p 1` **is** set here (serializes package *binaries* — one package's tests run to
  completion before the next package starts — comment at build.yml:208 cites
  `project_plans/flaky-hook-url-tests/decisions/ADR-001`). This reduces cross-package
  `-race` CPU contention but does **not** serialize tests *within* `session/git` itself —
  `t.Parallel()` fan-out inside that package still contends for CPU against every other
  goroutine the package's own test binary spawns, and the runner is a shared
  `ubuntu-latest` host with other jobs (`prepare`, `pty-race-regression`,
  `integration-coverage`, etc.) potentially running concurrently in the same CI run,
  which is the "full-suite CI load" referenced in the requirements doc.
  No explicit `GOMAXPROCS` override is set anywhere in `build.yml` — `-race` and the Go
  runtime use the default (`runtime.NumCPU()`), i.e. whatever `ubuntu-latest` exposes
  (2-core hosted runner, standard GitHub-hosted spec — not overridden in this repo).

- `make ci` (Makefile:798) chains `build test test-race vet lint lint-css-tokens
  test-integration fmt-check registry-generate actor-field-guard ptmx-field-guard` — the
  "definitive pre-push check" — runs `session/git` tests at least twice (`test`, then
  `test-race`'s second non-`-p1` invocation), plus again in `test-integration`
  (Makefile:548, non-`-p 1` invocation for `session/git`'s siblings — worth double-checking
  if `session/git` itself is covered there, but the non-serialized invocation is where
  contention would show up).

- `make bench-compare` / `benchmark.yml` (Makefile:908-928) use `-count=8` for benchmark
  stability, unrelated to this race — noted only because it's the other `-count=N` pattern
  in the repo, so as not to conflate it with a stress-repro mechanism.

## 2. go-git version and git CLI error-string pin

- **go.mod**: `github.com/go-git/go-git/v5 v5.14.0` (go.mod:29), `go-billy/v5 v5.6.2`
  (go.mod:132), `gcfg v1.5.1-0.20230307220236-3a3c6141e376` (indirect, go.mod:131).
  `go 1.26.3` (go.mod:3).
- The self-heal fallback (`session/git/worktree_ops.go`) does **not** use go-git for the
  `git worktree add -b` race path — it shells out via `runGitCommand` (git CLI subprocess,
  `session/git/worktree_git.go:38`, 30s fixed `context.WithTimeout`) and pattern-matches
  the CLI's stderr text. go-git's version is not directly relevant to this race; the CLI's
  is.
- **String-match sites**, both matching on raw `err.Error()` substrings, no git-version
  branching in code:
  - `worktree_ops.go:136`: `strings.Contains(err.Error(), "already checked out") ||
    strings.Contains(err.Error(), "already used by worktree")` — doc comment at
    `worktree_ops.go:132-136` explicitly notes older git said "already checked out",
    "current git (verified 2.50.1) says 'already used by worktree at'" — both variants are
    matched, so this specific site is already dual-covered.
  - `worktree_ops.go:336` area: `strings.Contains(err.Error(), "already exists")` — the
    second layered race window cited in the requirements doc (worktree_ops.go:336 in the
    doc's line numbering — confirm exact line against current file since line numbers
    shift; this grep hit is the only `"already exists"` match in the file).
- **No CI-pinned git CLI version**: grepped `.github/workflows/*.yml` for git-version
  pins, `apt-get install git`, `setup-git` actions — zero hits. CI runs whatever git
  ships on the `ubuntu-latest` GitHub-hosted runner image (not pinned by this repo).
  Local dev environment here has **git 2.53.0** — newer than the "current git (verified
  2.50.1)" cited in the code comment, so the "already used by worktree at" string is very
  likely still what any git ≥2.50 emits, but this hasn't been independently verified
  against 2.53.0's actual stderr text in this research pass (would need an actual forced
  race or a manual `git worktree add` collision to confirm the exact string didn't drift
  again between 2.50.1 and 2.53.0).
- Since CI's runner git version floats with whatever GitHub ships on `ubuntu-latest` at
  build time (no pin), a future git release changing this stderr string again is a latent
  risk this string-matching approach doesn't guard against — worth flagging as a
  robustness gap independent of this specific flake's root cause.

## 3. Stress-testing tool availability

- `golang.org/x/tools/cmd/stress@v0.47.0` — **available via `go install`**, already used
  in CI (see §1), not vendored/pinned in `go.mod` (it's a `go install`-at-CI-time tool, not
  a module dependency) and not in `tools.go`'s build-tag-tracked tool list. To reproduce
  locally: `go install golang.org/x/tools/cmd/stress@v0.47.0` (adds it to `$GOPATH/bin` /
  `$(go env GOPATH)/bin`).
  Fastest local repro command:
  ```
  go test -race -c -o /tmp/worktree_ops.test ./session/git
  stress -p $(nproc) /tmp/worktree_ops.test \
    -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate'
  ```
  (`-p` flag on `stress` itself controls parallel copies of the binary, separate from
  `go test -race`'s own within-binary `GOMAXPROCS`/`t.Parallel()` concurrency — combining
  both is how CI's `pty-race-regression` job amplifies CPU contention.)
- No `bradfitz/gostress` reference found anywhere in the repo (go.mod, Makefile, workflows)
  — the repo's only stress tool is the `golang.org/x/tools/cmd/stress` one described above.

## 4. CI CPU/parallelism environment

- All jobs in `build.yml` run on `runs-on: ubuntu-latest` (GitHub-hosted, standard spec —
  2 vCPU as of GitHub's current default hosted runner, not a larger runner tier; no
  `runs-on: [self-hosted, ...]` or larger-runner labels found in this workflow).
- No `GOMAXPROCS` env var set anywhere in `.github/workflows/*.yml` — Go runtime defaults
  to `runtime.NumCPU()` (i.e. whatever `ubuntu-latest` exposes).
- `-p 1` is set only in the `test` job's coverage step (build.yml:215, serializes
  **package binaries**, not intra-package parallelism) and in `test-race`'s first Makefile
  invocation for tmux-heavy packages (`./session ./session/mux ./session/tmux ./testutil`).
  `session/git` is **not** in either `-p 1`-scoped list in `test-race` — it always runs
  with default per-package build/test parallelism, both in `make test-race`'s second
  invocation and in CI's `test` job (`./session/...` glob includes `session/git`, under
  `-p 1` there, so package-level serialization is present in the CI path that's actually
  failing — but within-package `t.Parallel()` goroutines in
  `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` and its
  siblings still compete for the runner's ~2 CPUs against every other goroutine `-race`
  spins up for instrumentation).
- No explicit `-parallel N` flag override found on any `go test` invocation in
  `build.yml` or `Makefile` — default `go test -parallel` value (`GOMAXPROCS`) applies
  throughout.

## Summary for root-cause investigation (AC-2/AC-3)

The requirements doc's timeout hypothesis is structurally plausible given what's confirmed
here: `runGitCommand`'s 30s fixed timeout (`worktree_git.go:38`) is **not scaled** for CI
load anywhere in this codebase (unlike, e.g., `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS`,
which IS widened to 30s specifically for CI in `build.yml:206` for tmux session creation —
no equivalent CI-only widening exists for `runGitCommand`'s git-subprocess timeout). A
`ubuntu-latest` 2-vCPU runner under a `-race`-instrumented, `-p 1`-serialized-across-packages
but still-`t.Parallel()`-fanned-out-within-package test run is exactly the kind of contended
environment where a 30s-budget git subprocess plausibly blows its timeout without ever
producing either matched stderr string — consistent with the requirements doc's "test
timing" hypothesis over "unrecognized git error string" (both matched variants are already
handled) or "real fallback gap" (no code defect found in the matching logic itself).
