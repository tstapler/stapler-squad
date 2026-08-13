# Architecture Research: ci-hookurl-race-flake

## Prior-art check — an almost-complete duplicate project already exists

`Grep` for `server_integration_test.go|instance_tmux|build.yml` across `project_plans/*/research/architecture.md`
turned up **`project_plans/flaky-hook-url-tests/`**, an SDD project on the exact same bug (same two test
names, same root-cause diagnosis, same files) that is **already complete through Phase 4 (validate)**:

```
project_plans/flaky-hook-url-tests/
  requirements.md
  research/{architecture,stack,features,pitfalls,build-vs-buy,ux}.md
  decisions/ADR-001-p1-flag-over-isolated-invocation.md
  implementation/{plan,validation,pre-mortem,architecture-review,adversarial-review}.md
```

Git history confirms this is real prior work, not stale scratch: commits `a6d4e532e` (architecture research),
`d715e53a5` (plan + ADR), `60a82a7cb` (validation plan), `7f68dae7e` (validation/review artifacts) — all
`chore(sdd):` commits for this exact project name.

I verified directly against current code (not just trusting the prior doc) that **none of this plan has been
implemented yet**:
- `server/server_integration_test.go:336,338,407,409` still pass `30*time.Second` to
  `waitForLiveInstance`/`waitForPermissionRequestHookCommand` (the plan's Task 1.1.1 would bump the hook-wait
  call sites to `60*time.Second`).
- Neither helper uses `require.Eventually` yet (plan's Task 1.2.1-1.2.4) — still hand-rolled `for`/`time.Sleep`
  loops.
- `.github/workflows/build.yml:155` still reads `go test -race -coverprofile=coverage.out -covermode=atomic
  ./server/... ./session/... ./config/...` — no `-p 1` flag (ADR-001/Task 2.1.1 not applied).

**Recommendation for the planning pass this doc feeds**: do not re-derive a second, parallel plan under
`ci-hookurl-race-flake`. The prior project already answers all four research questions posed for this pass, in
more depth than a fresh derivation would produce in the same budget, and it went through an adversarial review
and pre-mortem. The correct next action is almost certainly to resume `flaky-hook-url-tests` at Phase 5
(implement) rather than open a second requirements/plan under a new name. The remainder of this doc answers
the four assigned research questions directly, citing that prior work where it already covers the ground and
adding direct verification/citations against current code where useful.

---

## 1. The async flow — every wait/poll/sleep point, with current values verified

`CreateSession` (`server/services/session_service.go`) returns to the RPC caller quickly, then does the real
work in a detached goroutine (verified at `session_service.go:1524` onward, unchanged from the prior research's
citation):

```go
go func() {
    ...
    instance.SetCreationProgress("Starting session...")
    if startErr := instance.Start(true); startErr != nil {       // (A) tmux spin-up, blocking
        instance.SetCreationProgress(fmt.Sprintf("Startup failed: %s", startErr.Error()))
        return
    }
    instance.SetCreationProgress("")
    if err := InjectHookConfig(instanceRootDir, instanceTitle); err != nil {  // (B) writes settings.local.json
        ...
    }
    ...
    if ctrlErr := instance.StartController(); ctrlErr != nil { ... }
    session.StartSessionDriver(instance, instanceRootDir)
    ...
}()
```

**(A) `instance.Start(true)` → tmux session creation, `session/tmux/tmux.go`.** Verified current constants and
line anchors:
- `sessionCreateTimeout = 10 * time.Second` (`tmux.go:182`)
- `sessionPollInitialDelay = 5 * time.Millisecond` (`tmux.go:183`)
- Fast path: `t.DoesSessionExistNoCache()` checked immediately after `new-session` (`tmux.go:998`), no sleep.
  If it hits, a bounded secondary wait for the push-based session registry to catch up:
  `registryDeadline := time.Now().Add(2 * time.Second)` (`tmux.go:1010`), polling — non-fatal, logs and
  continues past the cap.
- Slow path (`tmux.go:1023-1027`): exponential backoff starting at `sessionPollInitialDelay` (5ms), doubling,
  against the `sessionCreateTimeout` (10s) hard deadline. Timing out here produces the error that aborts
  `Start()` entirely — this is the "Race A" early-exit hang the prior research names (see §3 below), not the
  marginal-timeout flake this task is about.

**(B) `InjectHookConfig`** (`server/services/approval_handler.go`) runs synchronously right after `Start()`
returns, in the same goroutine — no separate wait of its own. It writes via temp-file + `os.Rename` (atomic,
no torn-read window), so it is not itself a source of latency variance; all of the wait budget's cost lives in
(A), the preceding tmux spin-up.

**Test-side polling helpers**, all in `server/server_integration_test.go` (current values, re-verified):

| Helper | Poll interval | Timeout as called | Line |
|---|---|---|---|
| `waitForResolvedAddr` | 10ms | 10s | `:424` (def), `:308` (call) |
| `waitForLiveInstance` | 20ms | **30s** (both call sites) | `:441` (def), `:336`,`:407` (calls) |
| `waitForPermissionRequestHookCommand` | 50ms | **30s** (both call sites) | `:463` (def), `:338`,`:409` (calls) |
| `waitForTmuxTeardown` (in `t.Cleanup`, non-fatal — `t.Logf` not `t.Fatalf`) | 20ms | 5s | `:527` (def), `:332`,`:404` (calls) |

`waitForPermissionRequestHookCommand`'s own doc comment (`:454-462`, unchanged since the prior research) already
states the diagnosis: the write only happens after tmux spin-up completes, and under a contended CI runner
running the full `-race` suite that "can take much longer than the file write itself" — the comment claims
callers "pass 60s" but **the call sites still pass `30*time.Second`**, a stale-comment/never-applied-fix bug the
existing plan's Task 1.1.1 targets directly. This is the single most concrete, cheapest fix available and it is
still unapplied in the tree today.

All four helpers are plain `for + time.Sleep` loops — no channel, no `context.Context` cancellation, no
subscribable signal. See prior research §3 for the full survey of why no such signal exists today
(`events.EventBus` publishes `creation_progress` but nothing for "hook injected"; no test-only callback seam
exists on `ApprovalHandler`/`SessionService`) — confirmed still true, not re-derived here.

---

## 2. Integration points: test process ↔ real tmux binary ↔ filesystem — nondeterminism sources

Beyond generic CPU contention, three specific, already-documented-in-code sources of nondeterminism (all
verified present in current code, matching the prior research's citations exactly):

1. **Shared per-process tmux server socket.** `testSocketOnce = sync.OnceValue(...)` (`session/tmux/tmux.go:336`)
   computes one PID-keyed socket name — every integration test in the `server` test binary shares one real
   tmux server process. `DeleteSession`'s teardown runs in an unawaited goroutine, so a still-tearing-down
   session from an earlier test in the same binary can pile onto the same socket for the next test's
   `CreateSession`. `waitForTmuxTeardown`'s own comment documents this was reproduced locally with
   `go test -race -count=10` and shows monotonically increasing latency as more create/delete cycles run in the
   same process — i.e., contention gets structurally worse the longer the test binary runs, which is exactly
   the "passes alone, flakes under full-suite `-race`" shape reported.
2. **`sessionCreateTimeout = 10s`** is a hard sub-budget nested inside the test's outer 30s
   `waitForLiveInstance` wait — tight under heavy contention even though the outer 30s was already sized to
   absorb some of it.
3. **`installFakeClaudeBinary`'s doc comment** (`server_integration_test.go:36-51` region) documents a
   *different* failure mode ("Race A" in the prior plan's glossary): if the fake `claude` shell exits before
   `Start()`'s readiness check observes the tmux session as live (a startup race against tmux's
   `remain-on-exit` default), `CreateSession`'s goroutine returns early — before ever calling
   `InjectHookConfig` — so the waiters time out no matter how large the budget is, because the write they're
   waiting for will never happen on that run. Current mitigation is the fake binary `sleep 60`ing to outlive
   the readiness check; this is a mitigation, not a guarantee, and is explicitly **out of scope** for both this
   task and the prior plan (touching `sessionCreateTimeout` or the tmux-side poll interval risks reintroducing
   or interacting with this separate race).

None of the three is "polling interval too coarse" or "fixed sleep with no backoff" in the naive sense — the
tmux-side wait already does exponential backoff (5ms→50ms cap), and the test-side polls use small, reasonable
intervals (10-50ms). The nondeterminism is structural (shared socket across create/delete cycles in one
process) and load-driven (a fixed 10s/30s budget that's marginal, not broken, under `-race` + full-suite CPU
contention on a 4-vCPU runner), not a poll-loop implementation bug.

---

## 3. `build.yml`'s `test` job structure — could this be split into its own job?

Verified directly by reading `.github/workflows/build.yml` (current, full file):

- The `test` job (`build.yml:119-264`) runs one single `go test -race -coverprofile=coverage.out
  -covermode=atomic ./server/... ./session/... ./config/...` step (`:153-157`) covering three package trees in
  one invocation, on one `ubuntu-latest` (4 vCPU) runner. `go test` builds/runs up to `GOMAXPROCS` (=`NumCPU`=4)
  package binaries concurrently by default — so these three packages' `-race`-taxed binaries compete for the
  same 4 cores today.
- **This has already been evaluated and explicitly rejected** by the prior project's `decisions/ADR-001-p1-flag-over-isolated-invocation.md`.
  The ADR frames exactly this task's question 3 as "Option 3: isolate `server_integration_test.go` (or the
  whole `server` package) into a second, still-gating `go test -race` invocation, merging its `-coverprofile`
  into `coverage.out` before the coverage gate runs" — and rejects it for four concrete reasons:
  1. **Coverage-merge correctness is unverified and risky.** No `gocovmerge` dependency exists in `go.mod`
     (adding one would violate the "no new external dependencies" constraint); naive text-concatenation of two
     `mode: atomic` profiles is only safe when the profiles cover disjoint source lines, which a package-level
     split cannot cleanly guarantee.
  2. **Job-setup duplication is an ongoing drift risk** — a second invocation needs its own tmux build/cache
     step and Go setup, doubling the places a future Go-version/tmux-version bump must be applied.
  3. **Disproportionate to the evidence** — `research/build-vs-buy.md` found this exact test pair has been
     patched three separate times before for CI-only timing flakiness, all three times via wait-tuning, never
     via CI topology changes (3-for-3 precedent against, 0-for-0 for).
  4. **It doesn't even fully solve the diagnosed problem** — the shared-tmux-socket contention (`testSocketOnce`,
     §2 above) is a *within-binary* problem; isolating just these two tests into their own invocation would not
     fix it, since other tests in the same package/binary still share the socket.
- **The adopted alternative** (ADR-001 "Decision," plan.md Task 2.1.1): append `-p 1` to the *existing single*
  gating invocation — same command, same `-coverprofile=coverage.out`, no new job/step, serializing the three
  package binaries instead of running them concurrently so whichever one is running (including `server`) gets
  the full 4 vCPUs. This is reversible with a one-line revert and was still unimplemented as of this research
  pass (confirmed above).
- **Concretely, what a "split into its own job" would look like**, for completeness (since this task asks for
  it explicitly, even though the prior ADR already rejected it): a new job `test-integration` with
  `needs: prepare`, its own `Set up Go` / tmux-cache/build steps (duplicating `test`'s), running
  `go test -race -coverprofile=integration-coverage.out -covermode=atomic ./server/...` scoped to just the
  package containing the flaky tests (package-granularity is `go test`'s minimum unit — file-level isolation
  isn't possible), then a merge step (`go tool covdata` textfmt after both invocations write to the same
  `GOCOVERDIR`, or a manual profile concatenation) before `coverage.out` is handed to the `vladopajic/go-test-coverage@v2`
  gate. `test`'s own invocation would need `./session/... ./config/...` only (dropping `./server/...`).
  This is exactly the shape ADR-001 evaluated and rejected — reproducing it here confirms the plan's Rejection
  is not hand-wavy: the coverage-merge and job-setup-duplication costs are real and concrete, not just modeled.

**Verdict for this research question**: yes, it's structurally possible, but it was already evaluated in depth
and rejected in favor of `-p 1` on the existing invocation. Re-opening it would need new evidence the prior ADR
didn't have (e.g., if `-p 1`'s measured wall-clock cost — Task 2.1.2, still unmeasured/unapplied — turns out to
be unacceptable in practice).

---

## 4. Consistency/ordering — coverage aggregation impact if this were split

Directly answering this task's question 4, confirmed by reading `build.yml`'s `Coverage gate` step (`:162-168`):

```yaml
- name: Coverage gate
  uses: vladopajic/go-test-coverage@v2
  with:
    profile: coverage.out
    global-threshold: 60
    local-threshold: 0
    badge-file-name: coverage.svg
```

This step reads `coverage.out` — the single artifact produced by the one gating `go test -coverprofile=...`
invocation two steps earlier in the same job. Today there is exactly one producer and one consumer, in the
same job, same filesystem, no artifact upload/download round-trip. Splitting the flaky tests into a separate
job would break this 1:1 relationship in one of two ways:

1. **Two separate coverage profiles, each independently gated** — simplest, but changes the *meaning* of the
   60% global threshold (it would now apply separately to a smaller `./session/... ./config/...` profile and a
   separate `./server/...` profile, rather than one combined view) — a behavior change to the gate's semantics,
   not just its plumbing, and not something either research doc found justification to introduce.
2. **Merge the two profiles before gating** — requires either a new dependency (`gocovmerge`, explicitly
   rejected as violating the "no new external dependencies" constraint) or manual `mode: atomic` text
   concatenation, which ADR-001 already flags as "merge-correctness-fragile and unverified" for this exact
   scenario. It would also require the new job to run *before* (or in parallel with an explicit merge/`needs:`
   dependency into) the `test` job's `Coverage gate` step — currently `test` has no `needs:` on any other test
   job, so this would introduce a new inter-job ordering dependency that doesn't exist today, and the
   coverage-gate step's `profile: coverage.out` input would need to become a downloaded/merged artifact instead
   of the same-job build product it is now.

Both paths are real, structural complexity — not just "add a job." This is exactly why the ADR concluded `-p 1`
on the existing single invocation is preferable: it makes **zero** change to the coverage-artifact production
or consumption path (same one command, same one file, same one gate step, same job), fully avoiding this
consistency question rather than needing to answer it.

---

## Key files/lines (for the planning phase, cross-referenced against current code)

- `server/services/session_service.go:1524-1571` — the async goroutine (`Start` → `InjectHookConfig` →
  controller/driver wiring); confirmed current, unchanged from prior research's citation.
- `server/services/approval_handler.go` — `InjectHookConfig` (atomic write, not the bottleneck).
- `session/tmux/tmux.go:182-183` (`sessionCreateTimeout`, `sessionPollInitialDelay`), `:998-1027` (fast/slow
  path wait loop), `:336` (`testSocketOnce` — shared per-process socket, primary suspected root cause of
  within-binary contention).
- `server/server_integration_test.go:336,338,407,409` — the four still-30s call sites (Task 1.1.1's target,
  still unapplied).
- `server/server_integration_test.go:441-501` (`waitForLiveInstance`, `waitForPermissionRequestHookCommand`
  helper bodies), `:503-539` (`waitForTmuxTeardown`).
- `.github/workflows/build.yml:119-264` (`test` job), `:153-157` (gating `go test` invocation, no `-p 1` yet),
  `:162-168` (`Coverage gate` step, single-producer/single-consumer on `coverage.out`), `:270-448` (`build`,
  `install-check`, `benchmark-gate`, `web-build-smoke` jobs — all `needs: prepare` not `needs: test`, so they
  run concurrently with `test` on *separate* GitHub-hosted VMs, ruled out as same-VM contention per the prior
  plan's Task 2.2.1).
- `project_plans/flaky-hook-url-tests/decisions/ADR-001-p1-flag-over-isolated-invocation.md` — the CI-topology
  decision (reject job-split, adopt `-p 1`) directly answering this task's question 3.
- `project_plans/flaky-hook-url-tests/implementation/plan.md` — full task breakdown (Epic 1: test-side
  `require.Eventually` + 60s budget; Epic 2: `-p 1` + wall-clock measurement; Epic 3: stress-repro
  documentation) — none of it applied yet in the current tree.
- `project_plans/flaky-hook-url-tests/implementation/validation.md` — requirement→test mapping and stress-repro
  verification steps, already written.
