# Stack Research: e2e-test-parallelization

Agent 1 (Stack). Scope: Bun/Playwright versions and their parallelism-relevant
features, GitHub Actions concurrency ceilings, and unused Bun-specific CI
speedups. Does not cover the shared-server architecture question itself
(that's the architecture research track) except where a stack fact bears on
it directly.

## Note on repo state during this research

The local checkout's `main` branch (`92448321592b`) is **behind**
`origin/main` (`d901621ea...` at research time) — it predates PR #1166
(`5a7279bdb`, "ci(e2e): shard e2e-next/e2e-bun and make e2e-next
report-only"), which the requirements doc describes as already merged. The
10-way shard jobs are real and live, just not in this on-disk copy. All
`build.yml` findings below are read from `origin/main` via
`git show origin/main:.github/workflows/build.yml`, not the stale local file.
A later research/plan agent updating this workspace should `git pull`/rebase
onto `origin/main` before editing `build.yml` for real.

## Bun version and relevant features

- Pinned via `.bun-version` at repo root: **`1.4.3`**. `web-app/package.json`
  mirrors this in `"packageManager": "bun@1.4.3"` and the `preinstall` guard
  script that rejects non-Bun package managers.
- `.github/actions/setup-bun/action.yml` installs this exact pin via
  `oven-sh/setup-bun@0c5077e51419868618aeaa5fe8019c62421857d6` (pinned to the
  `v2.2.0` tag's SHA) with `bun-version-file: .bun-version`, then caches
  `~/.bun/install/cache` keyed on a hash of every `bun.lock` in the repo
  (`web-app`, `tests/e2e`, `website`, `tools/*`, `tools/scanner/frontend`).
  This cache is already shared by every job that uses the composite action,
  including `e2e-next-shard`/`e2e-bun-shard`.
- **Bun 1.4** (shipped 2026-08-20, per Bun's own blog and the Appwrite
  changelog covering it) added an **opt-in global virtual store** for the
  isolated linker, advertised as up to 7x faster installs on a **warm CI
  cache with a large dependency tree** (lockfile present, package cache
  warm, `node_modules` deleted between runs — the exact CI shape this repo's
  shard jobs have). It is off by default; turning it on needs both:
  ```toml
  # bunfig.toml
  [install]
  linker = "isolated"
  globalStore = true
  ```
  or per-invocation: `BUN_INSTALL_GLOBAL_STORE=1 bun install --linker isolated`.
  (Sources: [bun.com/docs/pm/global-store](https://bun.com/docs/pm/global-store), [bun.com/docs/pm/isolated-installs](https://bun.com/docs/pm/isolated-installs); Bun's own docs disagree across mirrored pages on whether the global store defaults on or off for fresh `configVersion` lockfiles, so this needs confirming against the installed `1.4.3` behavior empirically, not assumed from the docs.)
  Caveat for this repo: `tests/e2e/`'s `devDependencies` tree is small (7
  packages) — the 7x figure is explicitly a large-dependency-tree number, so
  the win here is likely much smaller in absolute terms than it would be for
  `web-app/`'s much larger tree. Worth a quick empirical `bun install --linker
  isolated` timing comparison before committing CI time to it, not assuming
  the headline multiplier transfers.
- Bun 1.3.x (prior to 1.4) had already shipped cheaper installs relevant
  here: streaming tarball extraction cut peak memory ~17x, and the isolated
  linker took a peer-heavy monorepo install from 20.5s to 2.4s (per
  releases.sh's changelog aggregation) — this repo is already on a version
  that includes those.
- **`oven-sh/setup-bun` itself has no built-in dependency-cache input** — it
  only optionally caches the downloaded Bun *executable* (the old `v1`
  `no-cache` input), not `bun install`'s package cache. This repo's existing
  `actions/cache` step wrapping `~/.bun/install/cache` is the standard
  pattern (third-party actions like `antongolub/action-setup-bun` wrap the
  same idea with a `cache:` input, but `oven-sh/setup-bun` does not) — so
  the current setup-bun composite action is already doing the right thing
  here; there's no missed built-in flag to turn on.
- `bun install --frozen-lockfile` runs independently in **every** shard job
  (10x for `e2e-next-shard`, 10x for `e2e-bun-shard`, plus once each in the
  roll-up `e2e-next`/`e2e-bun` jobs) — the Bun *executable* and its install
  cache are shared via the composite action's cache, but each job still pays
  the `bun install` CLI's own resolve/link work against a cold
  `node_modules/`. This is the concrete lever the global-store feature above
  would apply to, 22 times per CI run (10+10+1+1, times 2 if you count
  `e2e-video.yml`'s separate `bun install --frozen-lockfile` too).
- `bunx`/`npx`: the repo's e2e jobs consistently invoke Playwright via
  `npx playwright ...`, never `bunx playwright ...`, despite Bun being
  installed in every one of these jobs already (`actions/setup-node` is
  *also* installed in parallel, specifically because Jest/Next/ESLint
  binaries have a Node shebang — see `setup-bun/action.yml`'s header
  comment). `bunx` and `npx` both just resolve and exec the same installed
  `node_modules/.bin/playwright`; there is no meaningful install-speed or
  spawn-speed difference once the package is already installed via `bun
  install` — `bunx`'s speed edge over `npx` is in *implicit* package
  fetch-and-run (no local install), which doesn't apply here since
  Playwright is already a committed `devDependency`. **Conclusion: switching
  `npx` → `bunx` in these jobs is cosmetic, not a performance lever** — don't
  recommend it as a win.
- Bun's own `--parallel` / sharding flags belong to `bun test` (its native
  test runner), which is explicitly out of scope per the closed
  Jest-replacement decision and is not used for e2e specs regardless
  (Playwright owns that role, per the requirements doc). No further
  `bun test` research was done, per the standing instruction.

## Playwright version and parallelism features

- `tests/e2e/package.json` pins `"@playwright/test": "^1.40.0"` — a loose
  caret range. The **actual resolved/installed version**, per
  `tests/e2e/bun.lock`, is **`1.56.1`** (`playwright`, `playwright-core`, and
  `@playwright/test` all resolve to `1.56.1`). So the repo is not really "on
  1.40" — it's on whatever `^1.40.0` resolves to today, which bumps forward
  silently on every `bun install --frozen-lockfile` run against an
  unregenerated lockfile... except the lockfile itself pins the resolution,
  so in practice CI gets exactly `1.56.1` until someone bumps the lockfile.
  Worth flagging: the `package.json` range (`^1.40.0`) is purely cosmetic
  documentation at this point — the lockfile, not the range, is the source
  of truth for what CI runs.
- Features already in use in `build.yml`'s `e2e-next-shard`/`e2e-bun-shard`
  jobs (confirmed by reading `origin/main`'s current file, not the stale
  local one — see note above):
  - `--shard=${{ matrix.shard }}/10` — file-level sharding across a 10-way
    GitHub Actions matrix, `fail-fast: false`.
  - `--reporter=list,blob` per shard, then a separate roll-up job
    (`e2e-next`/`e2e-bun`) downloads all shards' blob artifacts and runs
    `npx playwright merge-reports --reporter=json ./all-blobs` to produce one
    unified JSON report that `scripts/e2e-baseline-compare.ts` consumes.
  - `workers: 1` / `fullyParallel: false` in `playwright.config.ts` — no
    in-process parallelism at all within a shard, by design (the
    single-shared-backend-server constraint; that's the architecture track's
    question, not reproduced in full here).
  - 3 browser engines installed per shard (`npx playwright install chromium
    firefox webkit --with-deps`) even though most projects in
    `playwright.config.ts` are Chromium-only — Firefox/WebKit are only used
    by a handful of `continue-on-error`/informational projects
    (`webkit-mobile`, the Story-1.3.5c chord-spec cross-browser checks). This
    is a real, measurable per-shard install-time cost (3x the browser
    download/install vs. installing only `chromium`) paid on **every** shard
    (20 shard jobs/run) for coverage that's informational on all but one
    project. Confirming the `continue-on-error` scope and whether
    Firefox/WebKit installation could be isolated to a single non-sharded
    job (mirroring `e2e-video.yml`'s `chromium`-only install) is a candidate
    for the plan phase, not decided here.
  - **Playwright shards by file count, evenly dividing the number of test
    files (or tests, depending on version) across `--shard=N/M`** — not by
    historical duration. I found no evidence in official docs or current
    third-party guides of a built-in duration-aware/self-balancing shard
    splitter in Playwright itself (searched explicitly for this). Teams that
    want duration-balanced shards build it themselves from blob-report timing
    data (exactly what Playwright's JSON/blob report would supply) feeding a
    custom shard-assignment script — Playwright does not do this
    automatically. If shard-count tuning becomes a plan-phase recommendation,
    rebalancing by recorded per-spec duration (the repo already produces this
    data via the blob reports it merges) is a build-it-yourself lever, not a
    flag to flip.
- **No newer Playwright version unlocks a materially different sharding or
  parallelism primitive for this repo's use case.** `--shard`, the blob
  reporter, and `merge-reports` all predate 1.40 and are unchanged in
  capability through the versions I could confirm (1.56 installed; latest
  stable found via search was in the 1.60-1.62 line, per
  [currents.dev](https://currents.dev/posts/pw-1.60.0) (1.60.0, dated
  2026-05-11) and [testdino.com](https://testdino.com/playwright-releases)
  (1.61, WebAuthn/passkey support, Web Storage API — both unrelated to CI
  parallelism); I could not reach the official 1.62.x changelog directly, so
  treat "1.62.1 is latest stable" as **UNVERIFIED**, sourced only from a
  third-party release tracker, not playwright.dev itself.) Nothing found in
  any of these lines bears on job-level sharding, worker counts, or report
  merging — the relevant primitives this repo already uses were stable well
  before 1.56. **Recommendation: no version bump is needed to pursue any of
  the sharding/overhead levers in scope for this project; if a bump happens
  for unrelated reasons (security, bug fixes), it's free to take but isn't
  itself a lever.**
- `testInfo.parallelIndex`/`workerIndex` (stable Playwright API, available at
  1.56 and long before) is the mechanism a true in-process multi-worker
  design would need to assign each worker its own backend server
  port/testdir if the single-shared-server constraint were lifted — relevant
  context for whichever research track (architecture) evaluates that
  question, noted here only because it's a stack-version fact: the API
  exists and has existed for years, so a version bump is not a blocker for
  that redesign either, only the redesign's own implementation/validation
  cost is.

## GitHub Actions concurrency ceiling

- GitHub's own docs (`docs.github.com/en/actions/reference/limits` — the
  current canonical page, "About billing for GitHub Actions") list
  **concurrent-job limits by billing plan, not by repo visibility**:

  | Plan | Concurrent jobs (standard runners) | Concurrent macOS jobs |
  |---|---|---|
  | Free | 20 | 5 |
  | Pro | 40 | 5 |
  | Team | 60 | 5 |
  | Enterprise Cloud | 500 | 50 |

  Jobs beyond the cap are queued, not rejected/failed.
- `tstapler/stapler-squad` is a personal/public repo, which the docs confirm
  gets **free, uncapped Actions *minutes*** on standard GitHub-hosted
  runners (no monthly minute quota, unlike private repos on Free/Pro/Team) —
  but **I found no explicit statement that public repos get a higher
  *concurrency* cap than the plan table above.** The limits table has no
  public/private column; "free minutes" and "concurrent job slots" are
  separate axes, and only the first is confirmed more generous for public
  repos. Treat "public repos get more concurrent jobs" as **UNVERIFIED** —
  the safer working assumption for planning is the plan-tier number (almost
  certainly 20, Free, unless the account carries a paid plan — not checked
  here, out of scope for a stack-tech question).
- Concrete implication for shard-count planning: this project's two e2e
  jobs together already run **20 shard jobs per CI run** (10 `e2e-next-shard`
  + 10 `e2e-bun-shard`), which on its own would exceed a 20-job *total*
  concurrency cap if literally nothing else in `build.yml` needs a runner at
  the same time — and `build.yml` has ~15 other jobs (`detect-changes`,
  `prepare`, `web-build-smoke-next/bun`, `built-assets-guard`,
  `web-build-gate`, `storybook-build`, `web-supply-chain`, `test-affected`,
  `test-tmux`, `test-delivery-guards`, `test-oracle`, `test-rest`, `test`,
  `pty-race-regression`, `integration-coverage`, `build`, `install-check`,
  `benchmark-gate`) that can run concurrently with the shards. If the
  account is on GitHub Free (20 concurrent jobs total, across *all*
  workflows in the account, not just this one), **pushing shard count higher
  than 10-per-job (20 total across both jobs) risks real queuing**, not
  because of a "public repo exemption" that doesn't exist, but because of the
  flat 20-job Free-plan ceiling colliding with everything else this repo's
  CI already runs concurrently. The requirements doc's claim that "this
  session's own prior work found this not to be a binding constraint at
  8-10 shards" is plausible only if either (a) the account is on a paid
  plan, or (b) other `build.yml` jobs weren't queued at the same wall-clock
  moment as the shards in that observation. **Recommend the plan phase
  verify the account's actual GitHub plan** (`gh api
  /user/settings/billing/actions` or the billing UI) **before recommending
  any shard-count increase past 10**, rather than relying on the
  not-binding-at-8-10 anecdote alone — it may have been true by coincidence
  of scheduling, not headroom.

## Summary of unused-but-available Bun/Playwright CI levers

1. **Bun 1.4's opt-in global virtual store** (`linker = "isolated"` +
   `globalStore = true`) — untried, plausible small win on warm CI cache,
   `tests/e2e`'s own dependency tree is small so the win is likely modest;
   worth a quick empirical timing test, not a default recommendation.
2. **Firefox/WebKit install cost paid on every one of 20 shard jobs** for
   coverage that's `continue-on-error`/informational on all but the gating
   Chromium project — a real, unexamined cost; isolating those browsers to
   a single dedicated job (as `e2e-video.yml` already does, Chromium-only)
   is a concrete candidate.
3. **`npx` vs `bunx`** — confirmed not a real lever once the package is
   already `bun install`-ed; don't recommend this.
4. **Duration-aware shard rebalancing** — not a Playwright built-in; would
   be a custom script consuming the blob-report timing data the repo
   already generates, if shard-count tuning research finds current shards
   unevenly loaded.
5. **No Playwright version bump is needed** to access any sharding/worker
   primitive relevant to this project — `--shard`, blob reporter, and
   `merge-reports` are all already in use and have been stable since well
   before the currently-resolved 1.56.1.
6. **GitHub Actions concurrency ceiling is a plan-tier fact (20/40/60/500),
   not a public-repo exemption** — verify the account's actual billing plan
   before recommending shard counts above 10, since the flat cap is shared
   across every concurrently-running job in the whole account, and
   `build.yml` already runs ~15 other jobs alongside the e2e shards.

## Sources

- [bun.com/docs/pm/global-store](https://bun.com/docs/pm/global-store) — Bun global virtual store docs (install speed, config)
- [bun.com/docs/pm/isolated-installs](https://bun.com/docs/pm/isolated-installs) — isolated linker docs
- [bun.com/blog/bun-v1.4](https://bun.com/blog/bun-v1.4.md) — Bun 1.4 announcement (2026-08-20)
- [appwrite.io/blog/post/announcing-bun-1-4-runtime](https://appwrite.io/blog/post/announcing-bun-1-4-runtime) — third-party Bun 1.4 benchmark detail (7x warm-CI install claim, build-time comparison)
- [releases.sh/bun/overview.md](https://releases.sh/bun/overview.md) — Bun 1.3.x install-performance changelog aggregation
- `oven-sh/setup-bun` v2 action (read via this repo's `.github/actions/setup-bun/action.yml` pin) — no built-in dependency-cache input confirmed by its README/docsearch mirror
- [currents.dev/posts/pw-1.60.0](https://currents.dev/posts/pw-1.60.0) — Playwright 1.60.0 release note (2026-05-11)
- [testdino.com/playwright-releases](https://testdino.com/playwright-releases) — Playwright 1.61 release summary (WebAuthn/passkey, Web Storage API)
- [playwright.dev/docs/test-sharding](https://playwright.dev/docs/test-sharding) — official sharding/blob-report/merge-reports docs
- [docs.github.com/en/actions/reference/limits](https://docs.github.com/en/actions/reference/limits) — GitHub Actions concurrent-job limits by plan
- [docs.github.com/en/billing/concepts/product-billing/github-actions](https://docs.github.com/en/billing/concepts/product-billing/github-actions) — public-repo free-minutes confirmation
- Repo files read directly: `.bun-version`, `web-app/package.json`, `tests/e2e/package.json`, `tests/e2e/bun.lock`, `.github/actions/setup-bun/action.yml`, `.github/workflows/e2e-video.yml`, `tests/e2e/playwright.config.ts`, `.github/workflows/build.yml` at `origin/main` (commit `5a7279bdb` and later — **not** the stale local `main`)
