# Build vs. Buy: e2e-test-parallelization

Research question: should further speedup of `tests/e2e/` come from building custom
infrastructure, or from adopting existing tools/services? Four areas assessed below, each with
Pros / Cons / Verdict.

## Baseline (what's already in use)

Confirmed by reading `origin/main`'s `.github/workflows/build.yml` (current local `main` is
behind `origin/main`; `git branch --contains` shows the sharding commit `5a7279bdb` ("ci(e2e):
shard e2e-next/e2e-bun and make e2e-next report-only (#1166)") lives on `origin/main`, not yet on
the checked-out branch):

- `e2e-next-shard`/`e2e-bun-shard` use Playwright's **native** `--shard=${{ matrix.shard }}/10`
  across a 10-way GitHub Actions matrix (`build.yml:351,496`), each shard capped at a 40m soft /
  50m hard budget.
- `e2e-next`/`e2e-bun` merge jobs (`build.yml:423,590`) download every shard's blob report and run
  Playwright's own `npx playwright merge-reports` to produce one JSON/HTML report, which the
  baseline-classification comparator (Stories 7.10/7.11) then reads.
- This is already exactly the two built-in primitives Playwright documents for CI sharding: the
  `--shard` flag and the `merge-reports` CLI
  ([playwright.dev/docs/test-sharding](https://playwright.dev/docs/test-sharding)). There is no
  unused built-in Playwright CI feature being left on the table.

## 1. Existing OSS tooling for shard-count/balancing beyond --shard + merge-reports

**What exists:**

- Playwright's own `--shard` balances **by test count** when `fullyParallel: true`, or **by file**
  when it's `false` ([playwright.dev/docs/test-sharding](https://playwright.dev/docs/test-sharding),
  verified via WebFetch) — never by historical wall-clock time. This repo's
  `tests/e2e/playwright.config.ts` sets `fullyParallel: false` / `workers: 1` (lines 53, 58), so
  today's 10-way split balances by **file**, not even by test count — the weakest of the three
  balancing granularities Playwright supports, and a plausible reason shard times vary.
- Several timing-aware shard planners exist, none from Microsoft:
  - **NSXBet/test-orchestrator** (formerly referenced as `nsxbet/playwright-orchestrator`) —
    "Intelligent Playwright test distribution across CI shards using timing data." VERIFIED via
    GitHub API (`api.github.com/repos/NSXBet/test-orchestrator`): created 2026-01-23, last pushed
    2026-10-07, **4 stars, 0 forks, 2 open issues**. Actively pushed but objectively not an
    established/well-adopted project — a single small org's internal tool, not a de facto
    standard.
  - A Currents.dev blog post describes a bin-packing (Longest-Processing-Time-First) shard planner
    using Playwright's `--test-list` flag (v1.56+) to assign whole spec files to shards by observed
    duration — a pattern, not a packaged tool; Currents' own product is a paid SaaS dashboard
    (pricing aggregators report $40–$1170+/mo, unverified against Currents' own site) layered on
    top of Playwright, not a CI action.
  - Blog-documented DIY patterns (record durations via the reporter API, output a GitHub Actions
    matrix sized off a test-count heuristic) are reported in multiple sources but have no single
    canonical, maintained implementation. One cited "~1 shard per 2–3 min of total runtime"
    heuristic is explicitly flagged by its own source as a rule of thumb, not an official
    recommendation.
- No GitHub Marketplace action from Microsoft or a major vendor does "calculate Playwright shard
  count/assignment from historical timing" as a packaged, versioned product — every citable
  example is a blog post, a single-org tool, or a paid dashboard product's internal feature.

**Pros of adopting one (e.g. the Currents pattern or test-orchestrator):**
- Could fix uneven shard wall-clock times (the requirements doc's "is per-shard fixed overhead now
  the bigger lever" question) directly, without this project writing its own bin-packer.
- `--test-list` (Playwright 1.56+) is itself a stable, documented primitive worth checking this
  repo's Playwright version against before building a custom file-to-shard mapping.

**Cons:**
- `test-orchestrator` is too immature (4 stars, single external org, no track record) to adopt as
  a dependency in CI for a personal project — a bug in someone else's 9-month-old tool breaking the
  main CI gate is a worse failure mode than the status quo's uneven-but-correct shards.
- All timing-based approaches need a durable store of historical per-spec timings to bin-pack
  against; this repo doesn't have one today, and building it (e.g. persisting Playwright JSON
  report durations across runs) is itself new infrastructure, whether or not a third-party
  balancer consumes it.
- The uneven-shard problem may be secondary to the **fixed per-shard overhead** the requirements
  doc separately flags (repeated Go/Bun/Playwright-browser setup, `make stapler-squad` rebuild per
  shard) — a timing-aware balancer doesn't touch that at all.

**Verdict: Not recommended as a dependency; the *pattern* (switch `fullyParallel: true` so
`--shard` balances by test, not file, before reaching for a third-party balancer) is cheap, native,
and worth trying first — Viable.** A custom timing-based bin-packer, if ever justified, should stay
a small in-repo script (model: Currents' documented LPT-first approach) rather than adopting
`test-orchestrator` or paying for Currents.

## 2. Managed/hosted e2e runners (BrowserStack, Sauce Labs, Playwright's own cloud, etc.)

**What exists:**
- Microsoft's own Playwright Testing (Azure-hosted browsers) is metered/custom-quoted (one
  aggregator cites ~$0.06/test-minute, unverified against Microsoft's own pricing page) — built for
  scaling *browser* concurrency, not for solving the shared-backend-server problem; it still talks
  to whatever backend URL the test config points at, so it does nothing to let N workers get their
  own isolated `stapler-squad` server.
- BrowserStack and Sauce Labs are device/browser clouds (cross-browser/mobile matrix testing), not
  backend-orchestration services. BrowserStack's open-source program is real (press materials from
  2018, unverified as still current, describe free full-product access for qualifying OSS projects)
  and TestingBot explicitly markets a free-for-OSS tier. None of these services spin up or isolate
  *this repo's own Go binary* per shard — that's still 100% this project's responsibility whether
  or not browsers run locally or in a vendor's cloud.
- None of the three vendors researched offer "per-worker isolated backend environment" as a
  product feature; that's an application-specific concept (a stateful Go server + SQLite/file-based
  test dir), not something a browser-automation vendor's infrastructure would ever model.

**Pros:**
- Free-for-OSS tiers exist and cost nothing to this personal project, if adopted.
- Could offload browser-concurrency scaling so this repo's own GitHub Actions runners aren't the
  bottleneck for *browser* execution.

**Cons:**
- Doesn't address the actual bottleneck named in the requirements doc at all: the
  single-shared-server architecture. Every one of these services still needs *this project's*
  server running somewhere reachable by the browser session; outsourcing the browser doesn't lift
  the "one `stapler-squad` process per job" constraint.
- Adds an external-service dependency (network reachability from a vendor's cloud back to wherever
  the test server runs, auth/API-key management, a new paid-tier risk if free-tier eligibility ever
  lapses) to a personal project whose current CI already runs entirely inside GitHub Actions with
  no external accounts.
- Disproportionate complexity for the stated problem: this is a wall-clock problem solvable with
  more/better-balanced GitHub Actions jobs the project already has unlimited (practically) access
  to as a public repo, not a cross-browser-device-matrix problem.

**Verdict: Not recommended.** Correctly anticipated by the task brief — a poor fit for a personal
OSS project's CI, and more importantly doesn't solve the problem that's actually unsolved
(per-worker server isolation). Noted for completeness only.

## 3. LLM-generated custom N-server orchestration vs. adapting the existing TestServer class

**What Playwright itself offers:** confirmed via WebFetch of
[playwright.dev's parallelism/worker docs](https://playwright.dev/docs/test-parallel) — Playwright
exposes exactly one relevant built-in primitive: **worker-scoped fixtures** (`{ scope: 'worker' }`)
combined with `test.info().workerIndex` / `parallelIndex`. The docs' own canonical example is a
`dbUserName` fixture that creates a worker-unique database user via `workerIndex` and tears it down
after that worker's tests finish — structurally identical to what "one isolated `stapler-squad`
server per worker" would need, just for a DB user instead of a whole server process. Playwright
does **not** ship a built-in "spawn N isolated server processes, one per worker" utility — the docs
explicitly state a per-worker server pattern isn't documented and would need "a custom worker-scoped
fixture that derives its port from the worker index," confirming the task brief's working hypothesis.

**What this repo already has:** `tests/e2e/helpers/test-server.ts`'s `TestServer` class
(read directly) is already shaped for this almost by accident — its constructor takes
`port`/`testDir`/`buildPath` as `Partial<TestServerConfig>`, allocates a free port via
`findFreePort()` when `port === 0`, derives a **per-instance tmux socket name scoped to its own
port** (`tmuxSocket(): \`ssq-e2e-${this.config.port}\``), and isolates its own test data directory
and `STAPLER_SQUAD_INSTANCE`/`STAPLER_SQUAD_TMUX_SOCKET` env vars per instance. The only thing
standing between this class and "N isolated servers, one per worker" is that `global-setup.ts`
today instantiates exactly **one** `TestServer` and publishes its URL/dir via unindexed
`process.env.TEST_SERVER_URL`/`TEST_SERVER_TESTDIR` for every worker to share — a global-setup
limitation, not a `TestServer` limitation.

**Pros of adapting `TestServer` (vs. hand-rolling new orchestration):**
- `TestServer` already solves port allocation, process spawn/teardown (SIGTERM→timeout→SIGKILL),
  tmux-socket isolation, and test-dir isolation — the exact primitives N-server orchestration needs
  — and they're already tested-in-production by every current e2e run.
- Minimal new surface: a worker-scoped fixture that does
  `new TestServer({ port: 0, testDir: \`...-${workerIndex}\` })`, `start()`, publish
  `workerInfo.workerIndex`-keyed env/fixture values, `stop()` on teardown — following the docs'
  own `dbUserName` recipe almost verbatim, substituting a full server spawn for a DB user grant.
- Confirmed safe building block: `remote-workspaces.spec.ts`'s existing per-file `beforeAll`/
  `afterAll` dedicated-sshd pattern (noted in `global-setup.ts`'s own comments) already proves this
  codebase tolerates per-test/per-file supplementary process lifecycles alongside the shared global
  server — the worker-scoped case is the same shape at a coarser grain.

**Cons / what's still genuinely bespoke:**
- No library "solves" this in general — it's inherently glue code that must know this specific
  repo's binary path, env vars (`STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_TMUX_SOCKET`,
  `STAPLER_SQUAD_TEST_DIR`), and readiness probe (`/health`). Confirmed by both the Playwright docs
  search (no built-in) and the absence of any third-party "per-worker server supervisor for
  Playwright" package found in research for area 1.
- The requirements doc's own Rabbit Hole flags the real risk correctly: some specs may rely on
  cross-test state continuity within one shared server lifetime (the TOFU-dialog-skip workaround
  for `chromium-dom` cited in requirements.md is a documented example of such shared-server
  coupling) — adapting `TestServer` to run N copies doesn't by itself prove every spec is
  worker-independent; that's a correctness audit `TestServer`'s shape can't shortcut.
- Enabling true `fullyParallel: true` with N servers compounds with finding #1's
  `fullyParallel: false` discovery — flipping both at once increases the blast radius of a single
  change; the plan phase should treat them as separable levers (file-balanced-but-false→true
  shard-count balancing can be tried first, cheaply, before the much larger N-server effort).

**Verdict: Recommended, if the shared-server constraint is lifted at all** — adapt/extend
`TestServer` + a worker-scoped fixture, not bespoke orchestration from scratch. Building something
new here would be re-deriving work `TestServer` already does correctly. This still doesn't make the
rearchitecture low-risk (see requirements.md's own soak/validation-window caveat) — it only
narrows the *implementation* risk, not the *correctness-audit* risk of per-spec shared-state
assumptions.

## 4. Forking/adapting the isolated-dev-stacks dual-process design

Read in full: `project_plans/isolated-dev-stacks/research/architecture.md` (§3 "Process
orchestration architecture", §4 "Playwright architecture"). That research (2026-07-03, for an
unrelated opt-in dev-mode feature pairing a Go binary with `next dev`) independently arrived at
the same architecture this question is asking about:

- §3 explicitly models its launcher on `tests/e2e/helpers/test-server.ts`'s `TestServer` shape
  ("allocate port(s) → spawn child with env → wait-for-ready → own teardown") and rejects both
  `session/`'s tmux/git-worktree machinery (wrong layer — built for long-lived interactive agent
  sessions, not short-lived dev processes) and a docker-compose-style declarative layer (new
  infrastructure disproportionate to the problem) — the same "glue code, not a library" conclusion
  area 3 above reaches independently.
- §4 explicitly identifies the *same* constraint this project is investigating: "`playwright.config.ts`
  is already `fullyParallel: false`, `workers: 1`, against exactly one global server" and
  concludes a **separate, opt-in Playwright config + global-setup** (not branching the existing
  single-server config on a mode flag) is the right shape for its own dual-process need, with the
  same env-var-inheritance mechanism (`process.env.TEST_SERVER_URL` set in global-setup, inherited
  by spawned workers) this project's `global-setup.ts` already relies on (per that file's own
  comment, line ~15-16).

**Pros of treating that research as reusable precedent:**
- It's a second, independently-derived confirmation that `TestServer`'s shape (not a new library,
  not session/'s tmux machinery) is the right level of abstraction for "spawn and supervise
  isolated local server processes for test/dev purposes" in this codebase — cross-checks area 3's
  conclusion from a completely different feature's research.
- The "worker-indexed env vars Playwright's fixture system can read per-worker" idea named directly
  in this project's own requirements.md (Open Questions) is exactly the mechanism §4 already
  prototyped conceptually for a different pairing (frontend dev port + backend port instead of
  worker index + backend port) — the substitution is mechanical: swap "instance name" for
  "`workerIndex`"/`parallelIndex`.
- No code to literally fork — `isolated-dev-stacks` hasn't shipped a dual-process launcher yet (it's
  still a research doc, not merged code per that project's own plan.md status), so there's no
  shared module to extract today. But the *design document itself* is directly reusable context: an
  implementer of N-server e2e workers can lift §3/§4's architecture almost as-is, substituting
  `workerIndex` for the dev-mode launcher's "instance name."

**Cons:**
- The two use cases differ in one load-bearing way: isolated-dev-stacks' launcher supervises a
  *long-lived, single* dual-process pair started once per developer session; the e2e case needs *N*
  short-lived instances started/torn down per CI job run, matched to Playwright's own worker
  lifecycle (`workerIndex` restarts on worker crash, `parallelIndex` doesn't — WebFetch of
  playwright.dev confirmed this exact distinction) — the e2e fixture needs to be written against
  Playwright's worker-fixture API directly, not against isolated-dev-stacks' plain
  `child_process.spawn` supervisor API, so the "fork" is conceptual/architectural, not a literal
  code import.
- isolated-dev-stacks' own frontend-port problem (CORS origin allowlist, `NEXT_PUBLIC_API_URL`
  compile-time inlining) doesn't apply here — the e2e suite already runs against the Go binary's
  static-exported bundle (single origin), so none of §1/§2's frontend-specific findings transfer.
  Only §3/§4's *process-orchestration shape* is reusable, not the full document.

**Verdict: Recommended as a design precedent to cite and adapt, not literal code to fork.** Use
`architecture.md` §3/§4's "small supervisor shaped like `TestServer`, worker/instance-indexed env
vars, separate opt-in config rather than branching the default one" conclusion directly in the plan
phase's design for N-server e2e workers — it independently validates area 3's TestServer-adaptation
recommendation and pre-answers the "separate config vs. branch the existing one" design question
the same way requirements.md's own Rabbit Hole worries about.

## Summary table

| Option | Verdict |
|---|---|
| 1a. Switch `fullyParallel: true` so native `--shard` balances by test, not file | Viable — try first, free, no new dependency |
| 1b. Adopt a third-party timing-based shard balancer (e.g. NSXBet/test-orchestrator) | Not recommended — 4-star, 9-month-old single-org tool; too immature to depend on in the main gate |
| 1c. Build a small in-repo timing-based bin-packer if file/count balancing proves insufficient | Viable — only after 1a is tried and found wanting |
| 2. Managed/hosted e2e runner (BrowserStack/Sauce Labs/Playwright Cloud) | Not recommended — doesn't address the shared-server constraint at all; poor fit for a personal OSS project's CI |
| 3. Bespoke N-server orchestration from scratch | Not recommended — re-derives what `TestServer` already does |
| 3′. Adapt `TestServer` + a Playwright worker-scoped fixture for N isolated servers | Recommended, if the shared-server constraint is lifted at all — but still needs the correctness audit for cross-test state coupling (e.g. the TOFU/`chromium-dom` case) before it's safe |
| 4. Fork/adapt isolated-dev-stacks' dual-process design | Recommended as architectural precedent (not literal code) — independently confirms the TestServer-based approach and the "separate opt-in config" shape |
