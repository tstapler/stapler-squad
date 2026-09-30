# Research: Pitfalls of the Three Candidate Fixes

Agent 4 (Pitfalls) — `sdd:2-research` for backlog item `7cc29340-330b-413d-8e26-5418c550ccde`.

Source read in full: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts` (431 lines).
Repo: `web-app/package.json` pins `"jest": "^30.2.0"`; no `fakeTimers`/`timers` config found in
`jest.config*` or `package.json` (checked via grep — no hits), so Jest's Jest-27+ default
("modern" fake timers, `sinon`-based, supports `advanceTimersByTimeAsync`/`runAllTimersAsync`)
applies file-wide if invoked.

## File structure (relevant to Q1)

Everything lives inside **one** top-level `describe('Flow Control Stress Tests', ...)`
(line 12), with six nested `describe` blocks as siblings, all sharing the single
`WatermarkTracker` class defined once at the top of the outer describe (lines 14-60):

| Nested describe | Tests | Timer usage |
|---|---|---|
| `Large Volume Tests` | `handles 500KB plain text...`, `handles 1MB with ANSI...` | Fire-and-forget writes; poll completion via real `setInterval(..., 10)` |
| `Rapid Small Writes` | `handles 10000 small writes...` | Same `setInterval(10)` polling pattern |
| `Control Code Heavy Output` | `handles Claude Code style animations` (100 iters), `handles cursor positioning sequences` (1000 iters) | **Await-per-iteration** on `tracker.write()`'s real `setTimeout(1)`, same pattern as the failing test, plus a real `setTimeout(16)` "60fps" delay in the animations test |
| `Mixed Content Stress` | **`handles alternating text and control codes`** (the failing test, 5000 iters) | Await-per-iteration + `setTimeout(0)` yield every 100 iterations |
| `Watermark Behavior` | 3 tests | Real `setInterval(10)` polling, and a real `setTimeout(50)` delay per cycle in `pause/resume cycles work correctly` |
| `Error Recovery` | 2 tests | Await-per-iteration, small counts (5, 100) |

`WatermarkTracker.write()` itself (lines 22-43) unconditionally calls the real, un-mockable
(from this file's tests) `setTimeout(fn, 1)` — every single test in the file depends on that
timer firing, whichever style of consumption it uses (poll vs. await).

## 1. `jest.useFakeTimers()` pitfalls

**Scope leakage risk is real and specific to this file.** Jest's timer mode (`useFakeTimers`/
`useRealTimers`) is a **global mock installed for the whole module under test at the point
it's called**, not scoped to a `describe` block by default — scoping to one test or one
`describe` requires explicit `beforeEach(() => jest.useFakeTimers())` /
`afterEach(() => jest.useRealTimers())` placed *inside* that specific `describe`, and doing so
correctly is easy to get wrong in exactly the ways that would break siblings in this file:

- If `jest.useFakeTimers()` is called at the outer `describe('Flow Control Stress Tests', ...)`
  level (e.g., in a top-level `beforeEach`) instead of scoped to `Mixed Content Stress` only,
  every other nested describe's real `setInterval(10)` polling loops (`Large Volume Tests`,
  `Rapid Small Writes`, `Watermark Behavior`) and real `setTimeout(50)`/`setTimeout(16)` delays
  stop advancing on wall-clock time. None of those tests call
  `jest.advanceTimersByTime()`/`runAllTimersAsync()` today, so their `await new Promise(resolve
  => { setInterval(...) })` blocks would simply **never resolve** — they'd hang until Jest's
  own per-test timeout fires, converting currently-passing, currently-stable tests into
  guaranteed failures. This is the single biggest risk: an incorrectly-scoped fake-timer change
  silently breaks 8+ passing sibling tests while "fixing" the 1 flaky one.
- Correct scoping (a local `beforeEach`/`afterEach` inside `describe('Mixed Content Stress', ...)`
  only) avoids that, but is not the default outcome of a naive `jest.useFakeTimers()` call
  dropped at the top of the failing test — it requires deliberate placement and an explicit
  `afterEach` restore, and a reviewer/future editor moving the test between describe blocks
  (this file already restructures over time — see the `Control Code Heavy Output` /
  `Mixed Content Stress` split) can accidentally widen or narrow that scope again later.
- Even correctly scoped, faking timers **does not automatically drain** the real `setTimeout(fn,
  1)` inside `WatermarkTracker.write()` — something in the test body must call
  `jest.advanceTimersByTimeAsync(1)` (or `runAllTimersAsync()`) per iteration, interleaved with
  the `await` on the write's callback promise. Forgetting that interleaving is a **silent
  deadlock**, not a compile error or an assertion failure: the promise inside `write()`'s
  `setTimeout` callback never resolves, the `await` never returns, and the test hangs until the
  (possibly newly-raised) Jest timeout — i.e., a fake-timers migration done carelessly produces
  the *exact same symptom* (timeout) as today, just via a different mechanism, and is easy to
  misdiagnose as "still flaky" rather than "wired wrong."
- `new EscapeSequenceParser()` and the chunk-building string/regex work per iteration are
  synchronous and timer-independent — faking timers has no effect on them and introduces no new
  risk there. The risk is entirely in the timer-interleaving semantics described above, not in
  the parser/regex work the requirements ask to preserve (AC2).
- Net: fake timers are the mechanistically correct fix for "don't depend on real wall-clock
  scheduling" (AC1), but the implementation must (a) scope `useFakeTimers`/`useRealTimers` to
  only the `Mixed Content Stress` describe (or the single test) via local `beforeEach`/`afterEach`,
  and (b) explicitly advance fake time once per real timer the code under test schedules. Doing
  either wrong reintroduces a hang indistinguishable from the original timeout without the
  safety net of "at least it sometimes passes."

## 2. "Just raise the timeout" — deferred failure, not a fix

The root cause named in `requirements.md` is **unbounded compounding of per-timer slippage
under contention**: ~5000 sequential real `setTimeout(1)` round-trips, each individually able to
slip from ~1ms to "several ms" when the event loop is contended (concurrent `pnpm install`,
`make proto-gen`, 4000+ other Jest tests). There is no external bound on how much a single
Node.js timer callback can be delayed under load — Node's timer wheel guarantees a *minimum*
delay, never a *maximum*; under sufficient OS scheduler contention, GC pauses, or (per this
repo's other flake report, `docs/bugs/open/BUG-051-...md`) resource exhaustion in a shared CI
runner, a single callback can be delayed arbitrarily.

Consequently:
- **No timeout value is provably safe.** Raising 15000ms → 30000ms only requires the
  contention-induced average per-timer slippage to roughly double (from ~2ms to ~4ms average
  over 5000 iterations, ballpark) before the test flakes again — it does not change the shape of
  the failure mode, it just moves the threshold. The same reasoning that produced the original
  15000ms budget (some earlier author presumably judged 5000ms floor + margin = "safe") applies
  identically to any new fixed number: it's a bet on a maximum contention level, not a
  structural guarantee.
- **CI runners vs. local machines compound the problem, they don't bound it.** Shared/virtualized
  CI runners (noisy-neighbor CPU steal, throttled vCPUs) can exhibit *worse* per-timer slippage
  than a developer laptop hitting `pnpm install` + `make proto-gen` concurrently — the repro in
  `requirements.md` was captured on a dev machine, so CI-specific contention (which this repo's
  own e2e conventions and `BUG-051` both flag as a real, recurring source of resource contention
  in this project) is a plausible *worse* environment, not a better one. A timeout tuned against
  one real repro is not validated against the CI environment where flakes are most costly
  (blocking merges).
- This directly matches the repo's `CLAUDE.md` engineering discipline: **"No fix without root
  cause... Symptom fixes without a root-cause statement are not done."** Raising the timeout
  treats the symptom (test occasionally exceeds an arbitrary budget) without changing the
  mechanism (5000 sequential real-timer dependencies with no slack), so it fails that bar
  directly — it's a lower-flake-probability band-aid, not a fix, and the requirements
  (`AC1`: "must not depend on real wall-clock timer scheduling to determine pass/fail") already
  rule it out as satisfying the acceptance criteria on its own.

## 3. "Measure relative throughput instead" — mechanism and its own flakiness risk

**Likely mechanism**: run a baseline/warm-up measurement early in the test (e.g., time the first
N iterations via `performance.now()`/`Date.now()`), then assert something relative rather than
absolute — e.g. "the full run completes within Kx the baseline-implied rate" or "throughput does
not degrade by more than some factor across the run" — instead of a single fixed wall-clock
`toBeLessThan(15000)`-style budget.

Pitfalls specific to this pattern:
- **The comparison signal is still real-clock-based.** Both the baseline measurement and the
  full-run measurement still ride on the same real `setTimeout(1)` chain inside
  `WatermarkTracker.write()` — this approach doesn't remove real-timer dependency, it just
  changes what's asserted about the *timing data*. It therefore does **not** satisfy AC1
  ("must not depend on real wall-clock timer scheduling to determine pass/fail") on its own:
  pass/fail still literally depends on real wall-clock timer scheduling, just relatively instead
  of absolutely.
- **New flakiness source: baseline variance.** A short baseline sample (e.g., the first 100 of
  5000 iterations) has high variance under the exact same contention this bug is about — if
  contention is present from the start of the test, the baseline itself is already slow, so a
  *relative* comparison could mask a real regression (false negative — AC3 requires the test
  "still fail if the underlying flow-control logic regresses," and a self-relative baseline
  measured under the same degraded conditions as the rest of the run is weaker at catching
  regressions than an absolute floor). Conversely, if contention appears *mid-test* (a second
  process starts partway through the 5000-iteration loop, plausible in the CI scenario described
  in `requirements.md`), the baseline (measured early, uncontended) and the later measurement
  (contended) diverge for reasons unrelated to the code under test — reintroducing exactly the
  kind of environment-dependent flake this fix is meant to eliminate, just measured differently.
- **Picking the relative threshold (Kx) is the same unprovable-constant problem as Q2.**
  Whatever multiplier is chosen ("must not be more than 3x slower than baseline") is an arbitrary
  constant tuned against observed behavior, with no more of a formal safety bound than the fixed
  15000ms timeout it replaces — it's the same category of fix wearing different units.
- **Doesn't reduce wall-clock cost.** The test still performs ~5000 real macrotask round-trips
  regardless of what's asserted about them, so the ~5s floor (before any contention) remains —
  this approach only changes the *assertion*, not the *test's actual runtime dependency* on real
  timers, so it doesn't address the underlying design smell the other sibling tests already
  avoid (see requirements.md's contrast: `handles 500KB plain text...` fires all writes
  without awaiting each one).
- Net: this is the weakest of the three options against the stated acceptance criteria — it
  keeps a real-timer dependency in the critical path (violating AC1's literal wording) while
  trading one arbitrary constant (a fixed ms budget) for a different, less-tested one (a relative
  multiplier), and adds a genuine new false-negative risk under regression (AC3).

## 4. Repo-specific prior art on flaky-test handling

**No `.claude/rules/fix-flaky-tests-dont-defer.md` (or similarly-named) rule file exists in this
repo.** Verified by:
- `ls .claude/rules/` — 13 files present, none about flaky tests or timeout budgets (topics
  covered: CSS architecture, e2e conventions, ent schema gen, feature registry ×2, `gh pr merge`
  flag, Go double-checked locking, interface pollution, go-git preference, SDD artifact commits,
  session-creation registry, systemd service, tmux-keep-server).
- `grep -rli "flaky" .claude/` (this worktree) — zero hits in `.claude/rules/` or `.claude/docs/`.
- `find . -iname "*flaky*"` (this worktree) — only unrelated hits: `project_plans/flaky-hook-url-tests/`
  (a different backlog item's planning dir) and `docs/bugs/open/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md`.
- Also checked the user's global `~/.claude/rules/` and `~/.claude/skills/` — no
  flaky-test-specific rule file there either.

**Conclusion: the backlog item's citation of a `fix-flaky-tests-dont-defer.md` rule does not
correspond to any file actually present in this repo — treat it as unverifiable/absent, not as
existing guidance to follow.**

The closest actual repo-specific prior art is **`docs/bugs/open/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md`**,
a different flaky-test case (Go, `session/tmux` package, tmux-server contention under `go test`
parallelism) that is instructive by analogy even though it's a separate bug:
- Same failure shape: tests **pass reliably in isolation** and only flake under concurrent
  resource contention (there: real tmux servers under `go test` parallelism; here: real timers
  under concurrent `pnpm install`/`make proto-gen`/full Jest suite).
- Same diagnostic discipline: the bug report explicitly names root cause ("real tmux-server
  contention/resource exhaustion... not a logic bug") rather than proposing a timeout bump, and
  defers a real fix (serializing tests, sharing a test-scoped server) to a follow-up investigation
  rather than band-aiding it — consistent with this repo's `CLAUDE.md` "No fix without root
  cause" discipline that governs the current triage too.
- It also documents (line 16) that this exact package had a **prior dedicated fix for an
  adjacent race** (`e476853dc`) that didn't fully close the flake class — a cautionary precedent
  that a narrow, mechanism-blind fix (e.g., raising a timeout) can leave the same class of issue
  to resurface later in a different guise.

## Summary implication for `sdd:3-plan`

Of the three candidates, only **(a) fake timers** can structurally satisfy AC1 ("must not
depend on real wall-clock timer scheduling") — but only if `jest.useFakeTimers()`/
`useRealTimers()` are scoped locally to the `Mixed Content Stress` describe block (not the file
or outer describe) and every real timer the code under test schedules (`WatermarkTracker`'s
`setTimeout(1)`, plus the loop's own `setTimeout(0)` yields) is explicitly advanced via
`jest.advanceTimersByTimeAsync`/`runAllTimersAsync`, interleaved correctly with the `await`s so
nothing deadlocks. (b) raising the timeout and (c) relative-throughput assertions both leave a
real-timer dependency in the critical path and are, at best, lower-probability versions of the
same flake — neither satisfies AC1 on its own, and (b) is explicitly disqualified by this repo's
own root-cause-first discipline.
