# Architecture Research: flaky-hook-url-tests

## Prior-art check

`Glob` over `project_plans/*/research/architecture.md` found no prior SDD research doc that covers
`approval_handler.go`'s async hook-injection path, `waitForLiveInstance`/`waitForPermissionRequestHookCommand`,
or the shared per-process tmux test socket. (`grep -l "approval_handler|hook injection|InjectHookConfig|settings.local.json"`
matched `github-autonomous-fix`, `core-domain-decomposition`, and `isolated-dev-stacks` architecture docs, but
none of them discuss this integration-test flakiness or the tmux test-socket sharing model — they're tangential,
not overlapping.) This doc is derived fresh, not cited from an earlier run.

## 1. The async hook-injection path — every blocking/polling step

`CreateSession` (`server/services/session_service.go:1524` onward) returns to the RPC caller within
milliseconds, then does the real work in a detached goroutine:

```
go func() {
    s.wireCallbacks(instance)
    instance.SetCreationProgress("Starting session...")
    instance.Start(true)                      // <-- (A) real tmux spin-up + git worktree, blocking
    instance.SetCreationProgress("")
    InjectHookConfig(instanceRootDir, instanceTitle)   // <-- (B) writes .claude/settings.local.json
    ...
    instance.StartController()
    session.StartSessionDriver(instance, instanceRootDir)
    ...
}()
```

**(A) `instance.Start(true)` → `startLocked` (`session/instance.go:817-858`) → tmux session creation
(`session/tmux/tmux.go`)**. The actual session-creation wait loop (`tmux.go:985-1040`, inside the function
that issues `tmux new-session`):

- Fast path: `t.DoesSessionExistNoCache()` right after `new-session` returns (no sleep) — the common case.
  If it hits, there's a small additional wait: `registryDeadline := time.Now().Add(2 * time.Second)` polling
  every `time.Sleep(5 * time.Millisecond)` until the push-based session registry catches up
  (`tmux.go:1009-1018`), capped at 2s, non-fatal (logs `Warn` and continues past the cap).
- Slow path (session not immediately visible, e.g. tmux server under load): polls
  `t.DoesSessionExistNoCache()` in a loop with **exponential backoff starting at
  `sessionPollInitialDelay = 5 * time.Millisecond`, doubling up to a 50ms cap**, against an overall
  **`sessionCreateTimeout = 10 * time.Second`** deadline (`tmux.go:182-183`, loop at `tmux.go:1025-1040`).
  Timeout here produces the `"timed out waiting for tmux session %s"` error that aborts `Start()`.

**(B) `InjectHookConfig`** (`server/services/approval_handler.go:672-792`) runs synchronously right after
`Start()` returns, no goroutine of its own:
- Reads/parses existing `.claude/settings.local.json` if present (with a JSON-repair fallback path for
  common corruption, `approval_handler.go:696-712`).
- Merges in the PermissionRequest hook entry (`command` type; `curl` to the approval URL), with
  `hookTimeout = 300` seconds embedded in the hook payload itself (`approval_handler.go:653` — this is
  the Claude-Code-side hook timeout, not a Go-side wait).
- **Writes atomically**: `os.WriteFile(tmpPath, ...)` then `os.Rename(tmpPath, settingsPath)`
  (`approval_handler.go:785-788`) — no partial-write window visible to a reader; a reader either sees the
  old file or the fully-written new one. This directly answers requirement Q4 below.
- This call itself is not the bottleneck — it's a single small file write. The bottleneck is everything
  that must happen *before* it's reached: (A) above.

**No callback/signal exists today** for "hook injection just completed." The only observability the async
goroutine publishes is `events.NewSessionUpdatedEvent(instance, []string{"creation_progress"})`, fired before
`Start()` and again after (clearing the message) — nothing is published after `InjectHookConfig` succeeds or
fails (its error is only logged via `log.Warn`, swallowed as "non-fatal").

## 2. Test-side polling helpers — current values

All in `server/server_integration_test.go`:

| Helper | Poll interval | Timeout (as called) |
|---|---|---|
| `waitForResolvedAddr` (:424) | 10ms | 10s |
| `waitForLiveInstance` (:441) | 20ms | 30s (both flaky tests) |
| `waitForPermissionRequestHookCommand` (:463) | 50ms | 30s (both flaky tests) |
| `waitForTmuxTeardown` (:527, runs in `t.Cleanup`, non-fatal) | 20ms | 5s |

`waitForPermissionRequestHookCommand`'s own doc comment (`:458-462`) already states the diagnosis this
research would otherwise derive: *"the write only happens after `instance.Start(true)` ... returns, and on
a contended CI runner running the full `-race` suite in parallel that can take much longer than the file
write itself. Observed CI flakiness at 30s (this test intermittently timed out waiting on scheduling, not on
a real hang) motivated the wider budget."* I.e. the 30s budget was already bumped once and is still observed
flaky per the requirements doc — this is a scheduling/contention problem, not a logic bug in the wait loop
itself.

Both polling helpers are pure `for + time.Sleep` loops — no channel, no `context.Context` cancellation, no
signaling mechanism. Nothing in `approval_handler.go` or `session_service.go` exposes a hook to subscribe to
today (see Q3).

## 3. Integration points — is there a subscribable signal today?

No. Candidates that exist but aren't wired for this:
- `events.EventBus` (`s.eventBus.Publish(...)`) — used for `creation_progress` and session lifecycle events,
  but `InjectHookConfig`'s completion is not published to it. Adding
  `s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"hook_injected"}))` right after the
  `InjectHookConfig` call at `session_service.go:1547-1549` (success or failure) would give tests (and the UI)
  a real push signal, replacing file-mtime/content polling with an event-bus subscription — but this is a
  production code change, not test-only, since `EventBus` has no test-only subscription surface today.
- `instance.SetCreationProgress(...)` — already used for "Starting session..." messaging; could carry a
  "Hook injected" message the same way, observable via the existing `creation_progress` field tests could
  poll on `FindLiveInstance` (still polling, but polling an in-memory field instead of doing repeated disk
  I/O + JSON parsing — cheaper per iteration, not fewer iterations).
- No existing test-only callback/channel hook exists in `ApprovalHandler` or `CreateSession`'s goroutine that
  a test could `Wait()` on directly (e.g. no `sync.WaitGroup`, no injectable "on hook written" func field).
  Adding one would be a new test seam, e.g. an optional `onHookInjected func(sessionID string)` field on
  `SessionService` (mirroring the existing pattern of optional narrow-interface fields on `ApprovalHandler`,
  e.g. `notificationStamper`, `autoApprovalLog`) that tests set via a constructor/setter and production code
  leaves nil.

## 4. Write/read race — does a signal-based approach need special handling?

**No torn-read risk from the file write itself.** `InjectHookConfig` writes to a temp file then
`os.Rename`s it into place (`approval_handler.go:785-788`), which is atomic on the same filesystem (both
paths are under `<rootDir>/.claude/`, so no cross-filesystem rename risk). A reader (test or otherwise) that
opens `settingsPath` at any point either sees the fully-old content or the fully-new content — never a
partial write. `waitForPermissionRequestHookCommand`'s current approach (open, `json.Unmarshal`, check for
the `PermissionRequest` hook key, retry with `t.Sleep(50ms)` on any parse/miss) is already correct with
respect to torn reads; its only real cost is elapsed calendar time waiting for (A) tmux spin-up to finish,
which a signal can't shortcut, only more efficiently detect the moment it's done.

If a signal-based approach were built on the event bus or a callback, it would still need to tolerate: the
goroutine's `InjectHookConfig` **failing** (`session_service.go:1547-1549` treats it as non-fatal and only
logs) — a test-side "wait for signal" would need a distinct failure signal/timeout path too, or it could hang
forever waiting for a success event that will never come. This is a new edge case introduced by choosing to
signal only on success; the current polling approach naturally degrades to its own timeout instead.

## Root cause for the flakiness (per existing in-repo evidence, not re-derived)

Three separate, already-documented-in-comments contributing factors, all found without needing to reproduce
failures:

1. **Shared per-process tmux server socket across the whole test binary.** `session/tmux/tmux.go:336`
   (`testSocketOnce`) computes one PID-keyed socket name via `sync.OnceValue` — every integration test in
   `server_integration_test.go` (indeed every test in the process) shares one real tmux server. `DeleteSession`
   tears down tmux/git in an unawaited goroutine (`waitForTmuxTeardown`'s comment, `:503-526`), so a
   still-tearing-down session from an earlier test can pile onto the same socket for the next test's
   `CreateSession`. The comment states this was reproduced locally with `go test -race -count=10` and shows
   **monotonically increasing latency** — i.e. contention gets worse the more create/delete cycles run in
   the same process, which is exactly the shape of "passes alone, flakes under full-suite `-race`."
2. **`sessionCreateTimeout = 10s` inside tmux spin-up itself** (`tmux.go:182`) is a hard sub-budget inside the
   test's outer 30s `waitForLiveInstance` wait — under heavy CI contention this can itself be tight, but the
   30s outer budget was already sized to absorb it once (see `waitForPermissionRequestHookCommand`'s comment
   that 30s was already an increase from an earlier, apparently-still-flaky value).
3. **`installFakeClaudeBinary`'s own doc comment (`:36-51`) describes a *different*, already-observed failure
   mode**: if the fake `claude` shell exits before `Start()`'s readiness check observes the tmux session as
   live (a startup race against tmux's `remain-on-exit` default), `CreateSession`'s goroutine returns early
   *before ever calling `InjectHookConfig`* — meaning `waitForPermissionRequestHookCommand` times out no
   matter how large its budget, because the write it's waiting for will never happen on that run. The current
   fix for this specific mode is the `sleep 60` fake binary, which is a mitigation, not a guarantee — a CI
   runner slow enough to blow through more of that 60s window before the readiness check fires would
   reproduce the same "never happens" timeout, indistinguishable from ordinary scheduling slowness in the
   test's failure output.

These three are pre-existing, load-bearing comments already in the test file/tmux package — not conjecture.
They directly map to the requirements' three "in scope" questions: (1) `-race` contention is real and already
reproduced via the shared-socket monotonic-latency finding; (2) hook-injection's own file write is already
optimal (atomic rename, no polling inside it) — the slow part is tmux spin-up's `sessionCreateTimeout`
interacting with a shared, contended tmux server, not the hook write; (3) GH Actions runner sizing plausibly
explains why this reproduces in CI more than locally, since the shared-socket contention scales with how many
tests ran before it in the same process and how much CPU those tmux spin-ups compete for.

## Key files/lines for the planning phase

- `server/services/session_service.go:1524-1553` — the async goroutine (Start → InjectHookConfig → controller/driver wiring)
- `server/services/approval_handler.go:672-792` — `InjectHookConfig` (atomic write already in place)
- `session/tmux/tmux.go:182-183` (`sessionCreateTimeout`, `sessionPollInitialDelay`), `:985-1040` (wait loop)
- `session/tmux/tmux.go:336` (`testSocketOnce` — shared per-process socket, the primary suspected root cause)
- `server/server_integration_test.go:36-60` (`installFakeClaudeBinary`, documents the remain-on-exit race)
- `server/server_integration_test.go:441-501` (`waitForLiveInstance`, `waitForPermissionRequestHookCommand`)
- `server/server_integration_test.go:503-539` (`waitForTmuxTeardown`, documents the shared-socket monotonic-latency finding)
