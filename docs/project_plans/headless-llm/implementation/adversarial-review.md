# Adversarial Review: headless-llm

**Date**: 2026-05-26
**Verdict**: CONCERNS

---

## Blockers

*(none)*

---

## Concerns

- [ ] **Mutex deadlock on per-key lock inside `acquireSession`** — The plan holds the per-key mutex inside `acquireSession()` for the entire duration of building args, but `rotateSession()` also tries to acquire the per-key mutex. If `Call()` holds the per-key lock while executing a subprocess and then calls `rotateSession()` on error (which tries the same lock), you get a deadlock. Recommendation: `acquireSession()` should only hold the lock long enough to read/update session state and produce args; the subprocess must run outside the lock. The plan's text is ambiguous — `acquireSession()` should return args and release the lock before `Call()` starts the process. Make this explicit in the implementation note.

- [ ] **`DefaultPool` global mutable var is a race condition in tests** — `headless.DefaultPool = pool` is assigned in `BuildDependencies()` without synchronization. If any test concurrently reads `DefaultPool` (e.g., `RunOneShot` handler tests that call the real service), there is a data race. Recommendation: guard `DefaultPool` assignment and reads with a `sync.Once` or a package-level `sync.RWMutex`, or avoid the global entirely (always inject the pool explicitly; keep `DefaultPool` only as a convenience for non-test code where race conditions are controlled by startup ordering).

- [ ] **`ProcessRunner.Run()` leaks `ManagedProcess` if caller abandons the channel before `Stop()` is called** — The plan creates a `ManagedProcess` in `Run()` and returns `p.Stdout()`. The goroutine in `Call()` is supposed to call `proc.Stop()` on `ctx.Done()`. However, if the goroutine panics or the caller never reads the channel, the `ManagedProcess` is orphaned. The existing `ManagedProcess` has a GC finalizer as last resort, but that is unreliable for tests with `goleak`. Recommendation: move `ManagedProcess` creation into `Call()` itself (not inside `Run()`), so `Call()` can hold a direct reference to `proc` and unconditionally call `proc.Stop()` in a `defer`. Alternatively, the `ClaudeRunner` interface should return `(io.ReadCloser, Stopper, error)` so `Call()` can hold the `Stopper`.

- [ ] **`spawnReviewGate` blocking on `CallBlocking()` blocks the lifecycle goroutine** — The plan replaces `SpawnReviewSession` (which returned immediately with a session that ran asynchronously) with `l.headlessPool.CallBlocking(ctx, ...)` which blocks until the LLM completes. `spawnReviewGate` is already launched in a goroutine (`go l.spawnReviewGate(item, is)`), so this is acceptable — but the plan should explicitly note that the goroutine inherits the lifecycle listener's `context.Background()`-derived context, not the original RPC context. If the server shuts down mid-review, the goroutine and its subprocess must be cancellable. Recommendation: pass a pool-owned context derived from a server-lifecycle context (not `context.Background()`) into the spawned goroutine; document this in the implementation task.

- [ ] **`RunOneShot` loses working directory context** — Task 4.1.1a includes: "Remove `workDir` injection for the headless call (pass empty string; pool uses no working dir for stateless calls)". However the existing `RunOneShot` sets `cmd.Dir = workDir` which affects git operations (PR URL extraction involves checking git state). Removing `workDir` will silently break the PR URL extraction logic. Recommendation: either thread `workDir` into `Pool.Call()` as an option (e.g., `CallOption{WorkDir string}`), or keep a separate code path for `RunOneShot` that passes `workDir` to the subprocess, or document explicitly why removing `workDir` is safe for this specific call.

- [ ] **No error surfacing from `claude -p` non-zero exit to `StreamChunk`** — The plan notes "check stdout for error details on non-zero exit" but the `StreamChunk` only has `Err error`. When the LLM returns an error in stdout (not stderr), the goroutine needs to parse stdout, detect the error, and propagate it as `StreamChunk{Err: err, Done: true}`. The plan doesn't specify how the goroutine distinguishes a successful multi-line response from an error response embedded in stdout. Recommendation: add an explicit check in the goroutine: if subprocess exits non-zero and stdout was collected, wrap stdout content as an error in the final `StreamChunk`. Treat exit code 1 as `ErrLLMError`, code 2 as `ErrUsageError`, code 130 as `ErrInterrupted`.

- [ ] **`MaxConcurrentSessions` limit is defined in `PoolConfig` but never enforced** — The struct has `MaxConcurrentSessions: 5` but no implementation task describes how this limit is checked. If 10 different feature keys are used simultaneously, 10 subprocesses start. Recommendation: add a semaphore (`chan struct{}` buffered to `MaxConcurrentSessions`) in the `Pool` struct, acquired before starting any subprocess and released on subprocess exit. Add a task in Epic 1 for this.

- [ ] **`FakeRunner` does not simulate `--output-format json` first-call vs resumed-call behavior** — The `FakeRunner` returns raw string responses regardless of the args passed. Tests that verify session ID capture will fail silently if the pool's first-call JSON parsing path is tested with a plain-text response rather than a valid JSON response. Recommendation: `FakeRunner.Run()` should inspect the args slice for `"--output-format"` and `"json"` and return the corresponding scripted response (JSON vs plain text). The plan's `TestPool_CallBlocking_FirstCall_CapturesSessionID` test relies on a correctly formatted JSON response — document this requirement explicitly in Task 1.1.4a.

- [ ] **`BacklogLifecycleListener` imports `session/headless` — potential circular import** — `BacklogLifecycleListener` lives in `session/backlog_lifecycle.go` (package `session`). The headless package is `session/headless`. The plan says "no import of `session` package internals". This is correct for `headless → session`, but the reverse dependency (`session → headless`) is fine as long as `headless` does not import `session`. Verify this is the intended direction. If `headless` needs any type from `session` (e.g., `*ent.BacklogItem`), there will be a circular import. Recommendation: add an explicit note in the implementation that `session/headless` must never import `session` or `server`; any types needed by feature functions must be primitive (string/[]byte) only.

---

## Minors

- The plan counts 9 new files in the summary but lists 11 filenames in parentheses — the count is inconsistent. Fix for accuracy.
- Task 6.2.1a uses both a `//go:build integration` tag and an `os.Getenv("CLAUDE_INTEGRATION_TESTS")` skip — pick one mechanism; using both is redundant and confusing (build tag gates compilation; env var gates runtime skip). The build tag approach is cleaner.
- `features.go` places `FeatureKey*` constants in the headless package alongside pool logic. If feature prompts grow large, consider a separate `session/headless/prompts.go` file. Minor organization concern; not a blocker.
- `pool.go` plan defines `FeatureKey = string` as a type alias (not a defined type). This means `FeatureKey` and `string` are interchangeable everywhere, defeating the purpose of the type. Use `type FeatureKey string` (a defined type) instead of `= string` (alias).
- The proto `RunHeadlessCallResponse.cost_usd` field will always be 0.0 for resumed calls (cost not available without `--output-format json`). Consider omitting from the proto or documenting the limitation to avoid frontend confusion.
- `acquireSession()` increments `callCount` before the subprocess succeeds. If the subprocess fails on start (e.g., binary not found), `callCount` is incremented unnecessarily. Minor — rotate logic will catch this eventually, but it wastes a rotation slot.
- The plan states "Epic 1 must be complete before any other Epic begins" in the implementation order but the dependency diagram shows Epics 3, 4, 5 can proceed in parallel after Epic 1 — these statements are consistent; just make the "parallel after Epic 1" wording clearer.
