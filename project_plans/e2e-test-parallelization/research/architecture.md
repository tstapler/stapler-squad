# Research: Lifting (or Not Lifting) the Single-Shared-Backend-Server Constraint

Agent 3 (Architecture), Phase 2 — e2e-test-parallelization

## 0. Where the shard jobs actually live (orientation note)

The local checkout's `main` is 25 commits behind `origin/main` (`git fetch origin main` confirmed
this). `#1166` (`5a7279bdb8`, "ci(e2e): shard e2e-next/e2e-bun and make e2e-next report-only") is
on `origin/main` but not yet in the local branch. All line numbers below are against
`origin/main`'s `.github/workflows/build.yml` (fetched to `/tmp/build-origin-main.yml` for this
research; current HEAD there is `d901621ea`), not the local working copy's `build.yml`, which
predates the sharding work entirely.

## 1. What `TestServer` actually needs to become per-worker (read in full)

`tests/e2e/helpers/test-server.ts`'s `TestServer` class already has **more per-instance isolation
than the single-shared-server mental model suggests**:

| Field | Current scoping | Per-worker-ready? |
|---|---|---|
| `port` | `findFreePort()` picks a free OS port at `start()` time (or `TEST_SERVER_PORT` override) | **Yes, already.** Each `new TestServer().start()` call gets its own free port independent of any other instance in the process. |
| `testDir` | Defaults to `` `/tmp/stapler-squad-test-${process.pid}` `` ([`test-server.ts:53`](https://github.com/tstapler/stapler-squad/blob/c1596445bcea3d50520ab3708f17c19a100eb6be/tests/e2e/helpers/test-server.ts#L53)) | **Yes, already** — PID-scoped, and each Playwright worker is a distinct OS process with a distinct PID. |
| `STAPLER_SQUAD_TMUX_SOCKET` | `` `ssq-e2e-${this.config.port}` `` ([`test-server.ts:149-151`](https://github.com/tstapler/stapler-squad/blob/c1596445bcea3d50520ab3708f17c19a100eb6be/tests/e2e/helpers/test-server.ts#L149-L151)) | **Yes, already** — port-scoped, and ports are already unique per instance. |
| `binDir` (PID-scoped copy of the shared repo-root binary, [`test-server.ts:230-235`](https://github.com/tstapler/stapler-squad/blob/c1596445bcea3d50520ab3708f17c19a100eb6be/tests/e2e/helpers/test-server.ts#L230-L235)) | `os.tmpdir()/stapler-squad-e2e-bin/${process.pid}` | **Yes, already** — the comment at that line says this exists specifically so concurrent e2e runs against this same checkout can't race a rebuild out from under one another. |

So the `TestServer` class's **internals** need no change to support N coexisting instances. The
actual blocker is entirely about *lifecycle and discovery*, not instance identity:

1. **`global-setup.ts` runs once**, in the single Playwright *main* process, before any worker is
   forked. It calls `startGlobalTestServer()` once and does `process.env.TEST_SERVER_URL =
   getGlobalTestServer().getBaseUrl()` ([`global-setup.ts:14-24`](https://github.com/tstapler/stapler-squad/blob/0b0ae53ea20e2df7afd2166119a59098229dd478/tests/e2e/global-setup.ts#L14-L24)) — a
   **mutation of the process's own env**, which Playwright workers inherit as child processes at
   spawn time. Every worker that spawns after this point sees the *same* `TEST_SERVER_URL` and
   `TEST_SERVER_TESTDIR`, because there is exactly one value to inherit. There is no mechanism in
   today's code for worker 2 to see a different port than worker 1 — `process.env` is a single
   flat namespace, not one that Playwright partitions per worker.
2. Two specs read this directly and would break first: `connection-count-indicator.spec.ts:39` and
   `launcher-presets.spec.ts:21` both do `const TEST_DIR = process.env.TEST_SERVER_TESTDIR` to
   write/verify a config file on disk outside the RPC surface. Under per-worker servers each
   worker's "its own server's testDir" is a different value — reading a module-level `const` off
   `process.env` captured once at import time (same value for whichever worker imports it) cannot
   express "whichever server *this worker's* tests are talking to."
3. One spec (`remote-workspaces.spec.ts`) already manages its *own* additional shared resource (a
   per-file `sshd`), and `global-setup.ts:26-33`'s comment explains exactly why: a single global
   sshd shared across every Playwright *project* that runs that spec file would only show the TOFU
   (trust-on-first-use) host-key dialog to the first project, because `KnownHostsStore` is keyed
   server-side by `host:port` for the run. That's a second, independent instance of "one shared
   thing, multiple consumers" friction already discovered and solved ad hoc — evidence that this
   general class of bug (global/shared state that should have been per-consumer) has bitten this
   suite before, in a narrower form, and was fixed by giving that one spec its own private
   lifecycle rather than by changing the shared architecture.

## 2. The integration point: a worker-scoped Playwright fixture, not `process.env`

Playwright's own mechanism for "give each worker something different" is the worker-scoped
fixture — `test.extend<TestFixtures, WorkerFixtures>({ name: [async ({}, use) => {...}, { scope:
'worker' }] })`. A worker-scoped fixture function runs once per worker process (lazily, on first
use by that worker) and its return value is cached for every test that worker runs; `testInfo
.workerIndex` is available inside it if the fixture needs a stable integer index (e.g. to derive a
human-readable `testDir` suffix, though `TestServer`'s PID-scoping already gives this for free).

Concretely, lifting the constraint would mean:

- Replace `global-setup.ts`'s single `startGlobalTestServer()` + `process.env.TEST_SERVER_URL`
  write with a **worker-scoped `testServer` fixture** that calls `new TestServer().start()` once
  per worker and tears it down in the fixture's cleanup (after `use()` returns), mirroring
  `TestServer.stop()`'s existing SIGTERM→timeout→SIGKILL pattern
  ([`test-server.ts:119-146`](https://github.com/tstapler/stapler-squad/blob/c1596445bcea3d50520ab3708f17c19a100eb6be/tests/e2e/helpers/test-server.ts#L119-L146)) — no new teardown logic needed, just a new
  caller.
- **Override the built-in `baseURL` fixture** (not `use.baseURL` in the config) to resolve to
  *that worker's* `testServer.getBaseUrl()`. This is the actual "discovery" mechanism: existing
  specs call `page.goto('/')`/use relative paths against the configured `baseURL`, and Playwright
  already resolves `baseURL` through the fixture system per-test, so overriding the fixture (not
  the static config value) transparently redirects every spec's existing `page.goto()` calls to
  the right worker's server with zero per-spec changes — **except** the two specs above that
  bypass fixtures and read `process.env.TEST_SERVER_TESTDIR` directly; those two would need a
  parallel `testServerDir` fixture.
- `remote-workspaces.spec.ts`'s already-isolated per-file `sshd` pattern needs no change — it's
  already file-scoped, which is exactly the granularity a worker-scoped fixture produces (a whole
  spec file is assigned to one worker; Playwright does not split a single file's tests across
  workers unless the file opts into `test.describe.configure({ mode: 'parallel' })`, which this
  suite doesn't use).

This is a genuine, well-documented Playwright capability — not a workaround — but it has not been
exercised anywhere in this codebase today; every current fixture use in `tests/e2e/` is test-scoped
or file/global-scoped, so adopting it here would be the first worker-scoped fixture in the suite.

## 3. Does the isolated-dev-stacks precedent apply, and how strongly?

Per the briefing, [`project_plans/isolated-dev-stacks/research/architecture.md:189-214`](https://github.com/tstapler/stapler-squad/blob/5b1fc21c57a5b58042dc0cf32bf6fa26a18c8463/project_plans/isolated-dev-stacks/research/architecture.md#L189-L214) designed a
*different* multi-instance problem (one Go+`next dev` pair for an opt-in interactive dev mode) and
explicitly rejected folding it into the existing `playwright.config.ts`/`global-setup.ts`, writing:

> "`playwright.config.ts`'s `use.baseURL` and the global-setup's server lifecycle are both
> single-server-shaped by construction ... branching all of that on a mode flag inside one
> file/one global-setup is more invasive than standing up a second small config."

**The same structural fact is true here, and more severely so.** That project's rejected
alternative only had to touch `global-setup.ts` and one derived `baseURL` value. Lifting the
single-server constraint for *true in-process multi-worker parallelism* additionally requires
flipping `playwright.config.ts`'s `fullyParallel: false` → `true` and `workers: 1` → `N`
([`playwright.config.ts:53,59`](https://github.com/tstapler/stapler-squad/blob/0b0ae53ea20e2df7afd2166119a59098229dd478/tests/e2e/playwright.config.ts#L53)) — settings that apply to **the whole
config file**, i.e. to all 10 `projects` entries (`visual-matrix`/`-cyberpunk77`/`-wh40k`/`-clean`,
`chromium`, `desktop-firefox`, `desktop-webkit`, `mobile-chromium`, `chromium-dom`, optional
`webkit-mobile`), not just to the specs a new mode is meant to cover. Mutating the existing file
would change the concurrency model for every spec and every local `npm test` run on day one, with
no way to stage it incrementally within one file. Standing up a **separate config +
worker-scoped-fixture module** (e.g. `playwright.multiworker.config.ts` reusing `TestServer` the
same way the dev-stacks plan's `playwright.dev-mode.config.ts` was meant to reuse it) keeps
`playwright.config.ts`'s default `workers: 1` behavior — and therefore every existing CI shard job
and every developer's local `npx playwright test` — completely unchanged while the new mode is
proven out, exactly mirroring that precedent's stated rationale. The precedent's second reason
(avoid slowing down the common path with an unwanted cold start) doesn't transfer directly — this
change aims to *speed up* the same default gate, not add an opt-in slow path — but the first reason
(file is single-server-shaped by construction; branching it is more invasive than a parallel file)
transfers directly and, given the larger blast radius here, argues *more* strongly for the same
separate-file approach, not less.

## 4. What job-level sharding already proved, and what it didn't

This is the single most useful piece of evidence this research turned up, and it's specific to
*this* codebase, not a generic Playwright fact: **the 10-way GH Actions matrix already runs 10
independently-seeded `TestServer` instances concurrently** (one per `e2e-next-shard`/`e2e-bun-shard`
matrix job), each running its own `go run ./tests/demo/seed` (6 demo sessions), its own
`go run ./tests/e2e/seed-hidden` (1 hidden session), and its own 3 live tmux-backed sessions named
`e2e-review-1..3` — and the suite passes against the recorded baseline today. That means:

- **Already de-risked:** no spec in the 193-file suite currently depends on a *globally unique*
  session title, count, or ID across the whole run — if one did, it would already be failing
  differently shard-to-shard, since every shard's `e2e-review-1` is a *different* session on a
  *different* server. The bounded grep below corroborates this: only 2 files
  (`hidden-session-view.spec.ts`, `login-static-assets.spec.ts`) reference the seeded-session
  naming scheme at all, and both are scoped to their own file's server.
- **Not yet proven:** resource contention from running N `TestServer` processes (each spawning a
  real `stapler-squad` binary, a private tmux server, and up to 3 additional tmux-backed bash
  sessions) **on one shared CI runner** rather than N separate runners. Job-level sharding gives
  each shard a dedicated `ubuntu-latest` VM (4 vCPU / 16 GB per GitHub's published spec) for free;
  in-process multi-worker would put N of these full server+tmux stacks on *one* such VM
  simultaneously. Nothing in the 193 specs or in `TestServer` answers whether N=2, 4, or 8 of these
  stacks fit comfortably in one runner's CPU/RAM/disk budget — that is an empirical CI question,
  not a code-reading one, and is exactly the kind of "expect iterative, CI-cycle-expensive
  validation" risk the requirements doc's Rabbit Holes section already flagged for this item.

## 5. Bounded audit of the 193 specs (sample methodology, not exhaustive retriage)

Per the requirements doc's explicit warning against "rubber-stamping everything" or a shallow pass,
this was a targeted grep for the specific risk categories identified above (own-managed shared
resources, direct `TEST_SERVER_DIR`/`TESTDIR` reads, seeded-session-name coupling, file-scoped
`beforeAll`/`afterAll`) — not a read of all 193 files.

| Risk category | Files flagged | % of 193 |
|---|---|---|
| Reads `TEST_SERVER_TESTDIR`/`TEST_SERVER_DIR` directly (breaks under per-worker servers as-is) | 2 (`connection-count-indicator.spec.ts`, `launcher-presets.spec.ts`) | ~1% |
| References the global seeded-session naming scheme | 2 (`hidden-session-view.spec.ts`, `login-static-assets.spec.ts`) | ~1% |
| Manages an own additional shared/network resource (`createServer`/`net.connect`/`sshd`/hardcoded-port patterns) | 8, of which only 1 (`remote-workspaces.spec.ts`) is a confirmed real external resource (sshd) on inspection — the other 7 are false-positive keyword hits (e.g. "hardcoded" in a comment) | ~4% (≤1 confirmed) |
| File-scoped `beforeAll`/`afterAll` (state at file granularity — safe under worker-per-file assignment, same granularity job-level sharding already uses) | 54 | 28% |

**Verified (cheap, this pass):** all 193 files were grep-scanned for these four patterns — that
part is exhaustive, not a sample. **Not verified without deeper reading:** whether any of the
remaining ~179 files (193 minus the ~14 flagged above, allowing for overlap) make a *subtler*
cross-file assumption invisible to keyword search — e.g. relying on timing/ordering rather than
named state. Given §4's finding that job-level sharding already runs 10 independently-seeded
servers concurrently without failure, the realistic remaining exposure from this category is small,
but "small" is an inference from indirect evidence, not a file-by-file proof — closing it fully
would mean reading all 193 files, which this research pass deliberately did not do (and the
requirements doc explicitly warns against as its own failure mode).

## 6. Is 10-way sharding well-tuned, or does fixed overhead now dominate?

Pulled real timing from a recent `main` CI run (`gh run view 40184788547`, 2026-10-25, all 20 shard
jobs `success`) via `gh api .../actions/jobs/<id>` for step-level timestamps:

**Shard-to-shard duration variance (same run, same 10-way split):**

| | fastest shard | slowest shard | spread |
|---|---|---|---|
| `e2e-next-shard` (10 shards) | 15m49s (#6) | 31m36s (#10) | ~2.0x |
| `e2e-bun-shard` (10 shards) | 15m48s (#6) | 34m4s (#8) | ~2.2x |

**Fixed per-job overhead (checkout → "Install Playwright + test dependencies" step, cache-hit
case), from step-level timestamps on shard #6/#8:**

| Job | Fixed CI-setup overhead | Of which |
|---|---|---|
| `e2e-next-shard` #6 | 2m52s | Go setup (12s) + artifact download+`go build` (1m33s) + Bun/Node setup (19s) + Playwright-browser cache+install (45s) |
| `e2e-bun-shard` #8 | 3m24s | Go/Bun/Node/buf setup (~40s) + **`make stapler-squad` full rebuild** (2m14s) + Playwright-browser cache+install (31s) |

Plus, **inside** every shard's single "Run e2e shard" step (not broken out as a separate CI step,
but visible in the raw log), `global-setup.ts` pays its own per-shard cost every time: on shard #6,
11:05:36.05 → 11:05:55.97 (≈20s: demo-session seed 1.7s, hidden-session seed 1.6s, then an
**explicit, hardcoded 12-second `setTimeout`** at [`test-server.ts:302`](https://github.com/tstapler/stapler-squad/blob/c1596445bcea3d50520ab3708f17c19a100eb6be/tests/e2e/helpers/test-server.ts#L302) to
let the review-queue poller mark the 3 live sessions idle) before the first test even starts.

**Reading this data together:**

- Fixed CI-job overhead is real (~3-3.5 min) but is roughly **10-20% of the slowest shard's total
  time**, not the dominant cost.
- The embedded `global-setup` seeding cost (~20s/shard) is smaller still in wall-clock terms, but
  it is paid identically on **every one of the 20 shard jobs this run** (≈400s / 6.7 compute-minutes
  of Actions billing, run in parallel so it doesn't extend wall-clock, but it is pure duplicated
  work — the same seed data, regenerated 20 times, discarded 20 times).
- **The dominant driver of the slowest-shard tail is shard imbalance, not fixed overhead**: a
  >2x spread between the fastest and slowest shard in the *same* 10-way split, on the *same* run.
  Playwright's built-in `--shard=N/M` splits by file in a fixed round-robin order; it has no notion
  of each file's historical runtime, so a shard that happens to draw several slow files (e.g. the
  visual-regression/background-activity-style specs with multi-second waits) runs far longer than
  one that draws several fast files, regardless of shard count. The wall-clock story in the
  requirements doc ("~25-40 min/shard") is consistent with this: that range *is* the imbalance,
  not a uniform per-shard duration.
- This means the **highest-leverage, lowest-risk unexplored lever is duration-aware shard
  rebalancing** (bin-packing spec files into 10 buckets by last-run duration, read from the
  existing blob-report JSON that `e2e-next`/`e2e-bun` already merge, rather than Playwright's
  default file-order split) — strictly cheaper and safer than the server-per-worker
  rearchitecture, and it directly attacks the thing the data shows is actually dominant (the >2x
  tail), not a close second. This is a finding for the plan phase, separate from this item's
  disposition.
- On raw shard-count tuning: going from 10→fewer, larger shards would pay the ~3-3.5 min fixed
  overhead fewer times in total Actions-minutes, but would *not* shrink wall-clock unless combined
  with better balancing (fewer/larger shards without balancing just makes the imbalance tail
  worse, not better). Going to >10 shards shrinks the achievable per-shard test count but doesn't
  reduce the fixed per-job overhead's proportion — more jobs, same ~3min tax each. Neither
  direction, alone, is free; balancing is the one lever with no symmetric downside identified here.
- GH Actions concurrency: this research didn't find evidence it's binding (consistent with the
  requirements doc's own note that prior work in this session already checked this at 8-10
  shards); confirming the ceiling for a specific higher shard count would need a live
  `gh api /rate_limit`-adjacent check or an actual run at that count, not something inferable from
  one run's job list.

## 7. Disposition: Isolate via seam

**Lifting the single-shared-server constraint for true in-process multi-worker parallelism is
"Isolate via seam" — design the extension point now, defer implementation.**

Not **Refactor-first**: the cost/risk evidence above doesn't support doing this now.
- §6 shows the fixed-overhead and imbalance problems it would partially address are smaller,
  cheaper-to-fix problems than advertised, and a specific cheaper fix (duration-aware rebalancing)
  attacks the *actual* dominant cost (shard imbalance, >2x spread) more directly and with far less
  risk.
- §4 shows the *correctness* risk (per-spec state assumptions) is substantially, but not fully,
  de-risked by the already-running 20-job matrix — "substantially" based on a bounded grep (§5,
  ~1-4% of files flagged, not a full read), not a full audit.
- §4 and §3 both show the *resource-contention* risk (N server+tmux stacks sharing one CI runner's
  CPU/RAM instead of N dedicated runners) is genuinely unverified and can only be resolved
  empirically, via the same iterative, multi-CI-cycle validation the #1165→#1166 saga already cost
  this session once this quarter — the requirements doc flags this explicitly as a likely
  wall-clock blocker once scoped, which is exactly what this research scoped it as.
- §3 shows the blast radius of touching the existing single config is larger here (whole-file
  `fullyParallel`/`workers` flip affecting all 10 projects) than the already-rejected precedent
  case, reinforcing rather than weakening that precedent's conclusion.

Not **Extend as-is** either, because unlike a generic "leave it alone," this research did find a
concrete, cheap-to-write extension point (§2: a worker-scoped `baseURL`-overriding fixture reusing
`TestServer` unchanged, in a new sibling config file) and two specific call sites that would need to
change (§1.2: `connection-count-indicator.spec.ts`, `launcher-presets.spec.ts`). Writing that seam
down now (even just as a documented design, not code) means a future follow-up doesn't re-derive
§1-§4 from scratch, and the two spec files are a known, bounded starting point rather than an
unknown one.

**Recommended near-term action instead:** pursue duration-aware shard rebalancing (§6) and the
other already-in-scope levers (overhead caching, Bun-specific install speedups) as this project's
actual implementation, and record the N-server design from §1-§3 as an explicit follow-up story —
per the requirements doc's own Risk Control section, staged behind its own separate PR, never
conflated with the safer wins.
