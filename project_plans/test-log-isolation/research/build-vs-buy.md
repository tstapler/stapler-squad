# Research: Build vs. Buy — test-log-isolation

**Agent**: 6 (Build vs. Buy) — SDD Phase 2
**Scope**: fix the confirmed `-race` collision between `captureLogs`-style tests
(swap `slog.Default()` for a `bytes.Buffer`-backed handler) and any
`server/services` test whose background goroutine/`httptest.Server` hang
detector logs through the same global via stdlib `log.Print`. See
`project_plans/test-log-isolation/requirements.md` for full context.

## Verified codebase facts (via `git show origin/main:<path>` / `git grep origin/main`)

- `slogDefaultMu` + `captureLogs` (`server/services/autonomous_orchestration_service_test.go:407-431`)
  serialize only tests that *know to opt in*. 14 call sites across 6 test
  files (`autonomous_orchestration_service_test.go` x4, `connectrpc_websocket_test.go`,
  `deep_link_resolver_test.go`, `session_service_test.go`,
  `slack_interactive_handler_test.go`, `slack_notifier_test.go` x7) hold the
  mutex; `anthropic_client_test.go` (the confirmed-unfixed instance) is not
  among them and has no reason to be — it never touches `slog` directly.
- The package's own logging wrapper (`log/log.go`) does not thread a logger
  instance anywhere. `logAt()` (`log/log.go:634-642`) calls `slog.Default()`
  fresh on **every single call**:
  ```go
  func logAt(level slog.Level, msg string, args ...any) {
      logger := slog.Default()
      ...
  }
  ```
  `ForSession()` (`log/log.go:606`) does the same: `slog.Default().With(...)`.
  There is no seam to inject a per-test/per-call logger without changing this
  function's signature or adding a context-based override.
- Call-site volume: `git grep -o 'log\.\(Warn\|Info\|Error\|Debug\)(' origin/main -- 'server/services/*.go'`
  returns **524** matches (0 in `_test.go` files — all production code).
  This is the same order of magnitude as the 386 figure cited in
  requirements.md (exact count depends on grep pattern/whether `log.Fatal`
  etc. are included); either way it is "hundreds," not "a handful."
- A separate, already-shipped fix (PR #576, `log/log.go`'s `atomicLogger`)
  solved a *different* race: concurrent swap-and-read of package-level
  `*log.Logger` vars (`WarningLog`, `InfoLog`, etc.), using
  `atomic.Pointer[log.Logger]` with `Swap`/`Load`. That pattern is a
  **swap-the-whole-pointer**, not a **shared-mutable-buffer**, race fix — see
  Q3 below for why it doesn't directly transplant.

---

## Q1: Existing OSS library for "concurrent-safe slog output capture in tests"

**Searched**: pkg.go.dev-style queries for slog test-capture libraries;
stdlib's own `testing/slogtest` (confirmed: handler-conformance testing only
— it feeds a `Handler` a `bytes.Buffer` and parses output to validate the
`slog.Handler` contract, not to capture output for arbitrary test assertions
across goroutines — this is the "different concern" the task brief flags).

Candidates found:

| Library | Stars | License | Last push | What it does |
|---|---|---|---|---|
| [`neilotoole/slogt`](https://github.com/neilotoole/slogt) | 82 | MIT | 2026-05-23 | Bridges `slog` output to `t.Log()`, so output threads through `testing.T`'s own (already synchronized) output-writing rather than a manually-managed buffer. Returns a **`*slog.Logger` instance to inject**, not a global-default swapper. |
| [`samber/slog-mock`](https://github.com/samber/slog-mock) | 10 | MIT | 2026-08-01 | A mock `slog.Handler` with a customizable `Handle` callback for assertions. Also instance-based. |

Neither is a household name (compare `testify`'s ~20k+ stars) or has notable
independent adoption signal beyond their own READMEs. Both are single-file,
single-maintainer utility packages solving "make slog output testable,"
which is the *opposite* half of this bug — they help a test observe log
output cleanly, but neither one solves or even addresses the specific
failure here: **a process-global default being swapped out from under
unrelated concurrent goroutines**. Adopting either would still require the
call site (`log/log.go`) to accept an injected `*slog.Logger` instead of
reading `slog.Default()` — i.e., still Direction 1 (scoped logger) from
requirements.md — at which point the library is a thin convenience on top of
work that has to happen anyway.

**Corroborating evidence this is a known/recurring class of bug, not novel**:
`uber-go/zap`'s own `zaptest` package hit the identical failure shape —
[uber-go/zap#687](https://github.com/uber-go/zap/issues/687) ("zaptest: test
logger seems susceptible to data races") — multiple goroutines writing
through a shared `testingWriter` racing under `-race`. Zap's resolution
was **not** to pull in an external library; it added a mutex inside
`zaptest`'s own `testingWriter`/`Buffer` (see
[go.uber.org/zap/zaptest](https://pkg.go.dev/go.uber.org/zap/zaptest) and
[uber-go/zap#399](https://github.com/uber-go/zap/issues/399)) and the
ecosystem's stated best practice is a **scoped logger per test**
(`zaptest.NewLogger(t)`), not a shared global swapped between tests. This is
the same Direction-1 shape requirements.md already identifies.

**Verdict: Not recommended.** No library targets this exact problem
(global-swap-during-concurrent-background-logging), the two closest
candidates are low-adoption utilities that would still leave the actual
root-cause fix (removing/scoping the global swap) to be done by hand, and
the most relevant prior art (zap) explicitly solved this in-house with a
mutex rather than reaching for a dependency. This also satisfies the
requirements.md constraint "No new external dependencies" without any
tension — there's nothing worth the exception.

---

## Q2: Is a hand-rolled concurrent-safe buffer sufficient?

Yes, for the part of the problem that's about *buffer safety*. A
`sync.Mutex`-guarded `bytes.Buffer` (or `io.Writer` wrapping one) is a
10-line primitive:

```go
type syncBuffer struct {
    mu  sync.Mutex
    buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.buf.String()
}
```

This is correct, sufficient, and exactly the kind of primitive
`.claude/rules/interface-pollution-checklist.md` and the repo's general
stdlib-first norm call for over pulling in a dependency for a few lines of
mutex-guarded I/O.

**But it does not fix the confirmed bug by itself.** `TestAnthropicAIClient_Complete_CancelsOnCtxDone`
never writes to `captureLogs`'s buffer *intentionally* — its `httptest.Server`
hang-detector log line lands in **whichever buffer `slog.Default()` currently
points at**, because the buffer swap itself (not the buffer's internal
thread-safety) is the shared mutable state. Making the buffer's `Write`
method internally mutex-safe would only stop `-race` from firing on the
`Write` call; it would not stop the *semantic* race of one test's unrelated
log line polluting/being interleaved into another test's capture window, and
`buf.String()` racing a concurrent `Write` from an unrelated goroutine would
still need synchronization between the swap-holder and any leaked writer —
which is precisely what `slogDefaultMu` already does for the tests that know
about it, and precisely what `anthropic_client_test.go` doesn't participate
in.

**Verdict: Viable, but insufficient alone.** A mutex-guarded buffer is the
right primitive *inside* `captureLogs` (and it's arguably already implied,
since the existing `slogDefaultMu` pattern plus a per-`captureLogs`-call
buffer achieves the same effect via serialization rather than per-buffer
locking). It does not by itself close the gap for non-participating tests —
that requires either Direction 1 (stop sharing the global) or Direction 2
(stop the trigger), per requirements.md's own framing.

---

## Q3: Reuse `log/log.go`'s `atomicLogger` pattern (PR #576)?

The `atomicLogger` pattern (`log/log.go`, `atomic.Pointer[log.Logger]` +
`Load`/`Store`/`Swap`, with `SetWarningLogForTest`-style helpers returning
the previous value for `t.Cleanup` restoration) is a strong **structural**
precedent already in this codebase and should inform the design of whichever
direction Phase 3 picks — but it doesn't transplant onto `slog.Default()`
unmodified, for one concrete reason:

`slog.Default()` is stdlib's own global, stored behind an *unexported*
`atomic.Pointer[Logger]` inside `log/slog` — there is no way to wrap it in
this codebase's own `atomicLogger` type; `slog.SetDefault`/`slog.Default`
are the only accessor surface stdlib exposes, and they already use
`atomic.Pointer` internally (confirmed via the `slogDefaultMu` doc comment
in `autonomous_orchestration_service_test.go:407-410`: *"`slog.Default()` is
stored in an unexported atomic pointer, so `-race` never flags concurrent
swaps, but two `t.Parallel()` tests both redirecting it still race
semantically"*). The bug here is not a torn/racy *read* of the pointer
(atomics already prevent that) — it's two logical owners of the *same slot*
disagreeing about who owns it during an overlapping time window. Swapping
`sync.Mutex` for `atomic.Pointer` doesn't change that; `slogDefaultMu`
already is functionally the mutex-based version of exactly this problem, and
it already fails to help non-participating tests for the same reason
`atomicLogger` would: **an opt-in convention can't bind code that has no
reason to know it exists** (this is requirements.md's Success Metrics #3,
verbatim).

The part of the `atomicLogger` pattern genuinely worth reusing is the
**shape**, not the primitive: swap-with-restore via `t.Cleanup`, returning
the previous value rather than mutating in place. That shape is already
present in `captureLogs` itself (`prev := slog.Default()` /
`t.Cleanup(func() { slog.SetDefault(prev) })`). The gap isn't in that
shape — it's that the swap target is process-global. Extending
`atomicLogger`-the-type doesn't reach `slog.Default()`; the reusable lesson
is "own the pointer inside application code and inject it," i.e. Direction 1
(give call sites in `log/log.go` a package-level `atomic.Pointer[slog.Logger]`
that production code sets once at startup and tests can swap per-instance —
this is a closer structural cousin of `atomicLogger` than reaching for
`slog.SetDefault` at all).

**Verdict: Partially reusable / Viable as design inspiration, not a direct
port.** The swap-with-restore-via-`t.Cleanup` idiom transfers cleanly to
Direction 1's per-test logger injection. The `atomic.Pointer` swap mechanism
itself doesn't need porting because `slog` already provides the same
guarantee for its own global (that guarantee is exactly why this is a
*semantic* race, not a torn-read race — `-race` catches the latter, not the
former).

---

## Q4: Is there real algorithm/data-structure risk here, or is the "battle-tested library" question a non-fit?

**Stated plainly: this is squarely stdlib-usage-correctness territory, not
an algorithm/data-structure problem.** There is no non-trivial concurrent
data structure to select or implement here — no lock-free queue, no custom
hash map, no novel synchronization protocol. The entire fix surface is one
of:

- A mutex-guarded buffer (Q2) — a solved, ~10-line problem.
- Where a `*slog.Logger` pointer is read from (global default vs. an
  injected/scoped instance) — an architecture/threading question, not a
  data-structure question.
- Bounding an `httptest.Server`'s teardown latency so it can't hit the
  stdlib hang detector under `-race` load (Direction 2) — a timeout/context
  plumbing fix, same shape as the already-fixed instance #1
  (`ForceTeardown()` in `connectrpc_websocket_test.go`).

The "should we trust a library's battle-tested implementation over our own"
question presupposes a nontrivial algorithm where subtle bugs are easy to
introduce (e.g., a concurrent skip-list, a custom rate limiter, a consensus
protocol). None of that applies here. The risk in this bug was never "our
mutex logic is wrong" — `slogDefaultMu` mutex code is already correct as far
as it goes; the risk was **scope**: a correct synchronization primitive
guarding an incomplete set of participants. Buying a library would not
change who participates in the synchronization; only a design change
(Direction 1 or 2) does that. This confirms requirements.md's own
Alternatives-Considered framing is complete and Phase 3 should treat this as
a pure design-direction decision, not a make-or-buy one.

**Verdict: Not applicable / squarely build.** No amount of library
sophistication substitutes for deciding who's allowed to touch the shared
slot and when.

---

## Summary table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Adopt `slogt` / `slog-mock` (or similar) | Slightly nicer test ergonomics (`t.Log`-routed output); MIT-licensed | Doesn't address the root cause (global swap under concurrency); low-adoption niche packages; still requires the same call-site injection work as building it in-house; violates the "no new external dependencies" constraint for no offsetting benefit | **Not recommended** |
| Hand-rolled mutex-guarded buffer | ~10 lines, stdlib-only, matches repo norms, easy to review | Fixes buffer-write thread-safety but not the semantic ownership race between a swap-holder and a non-participating logger | **Viable** (necessary but not sufficient on its own) |
| Reuse/extend `atomicLogger` (PR #576) pattern | Familiar, already-reviewed shape in this codebase; the `t.Cleanup`-swap-restore idiom transfers directly | Can't literally wrap `slog.Default()` (stdlib owns that pointer, already atomic); the primitive isn't the missing piece — participation is | **Viable as design inspiration**, not a direct port |
| Build a small, scoped fix (Direction 1 injected logger, or Direction 2 bounded teardown) | Matches problem shape exactly; zero new dependencies; consistent with the already-fixed instance #1 pattern | Direction 1 requires auditing which of the 524 call sites actually need to be observable by `captureLogs`-style tests (time-boxed per requirements.md's Rabbit Holes) | **Recommended** |

## Recommendation

Build, using stdlib only — this is not a build-vs-buy decision in any
meaningful sense; no OSS library targets the actual failure mode
(non-participating global-logger swap under concurrent tests), and the fix
is a few lines of correct Go regardless of direction chosen. Phase 3 should
decide between Direction 1 (scoped `*slog.Logger` injection, informed by the
`atomicLogger` swap-with-restore shape) and Direction 2 (bound
`anthropic_client_test.go`'s `httptest.Server` teardown the same way
instance #1 was fixed) — that is a design-direction question, not a
make-or-buy one.
