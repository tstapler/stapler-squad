# Architecture Research: fix-benchmark-ci-failures

## 1. Tier 1 vs Tier 2 Steps — What Tier 2 is Missing

**`go-tier1` steps (complete):**
1. checkout
2. Set up Go
3. Install buf (`buf-Linux-x86_64 v1.41.0`)
4. Generate protobuf code — uses `buf.gen.go-only.yaml` (avoids web node_modules dep)
5. Generate ent ORM code — `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
6. Run Tier 1 benchmarks (specific regex, specific packages: `./server/events ./server/terminal ./session/scrollback ./session ...`)
7. Restore/save baseline to cache

**`go-tier2` steps (incomplete):**
1. checkout
2. Set up Go
3. Run full benchmark suite (`go test -bench=. -run='^$' -benchmem -count=8 -timeout=30m ./...`)
4. Save baseline to cache

**Missing from Tier 2 (root cause #2):**
- `Install buf` — not present
- `Generate protobuf code` — not present (must use `buf.gen.go-only.yaml` since no bun/node is installed)
- `Generate ent ORM code` — not present

Without the buf + ent generate steps, `go test ./...` on ent-dependent packages produces compile errors (missing generated files in `session/ent/*.go`), causing those packages' benchmarks to be silently skipped or the job to fail outright.

## 2. Terminal-Throughput Spec — Timing Constants and Failure Risk

File: `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`

- `TOTAL_RUNS = 10`, `WARMUP_RUNS = 2` (first 2 discarded; 8 counted)
- `PRESET = 'PAYLOAD_100KB'` (100KB payload each run)
- `test.setTimeout(300_000)` — 5-minute total test timeout
- `waitForTestCompletion(page, 30_000)` — called once per run; allows 30s for each of the 10 runs
- Helper `STATUS_CHANGE_TIMEOUT_MS = 5000` (in `helpers.ts`) — time for status to transition from `'idle'` after click

**Timing math:** 10 runs × 30s/run = 300s maximum. The global 5-minute timeout exactly matches this worst case — there is no slack. On slower Ubuntu 26 runners, any single run taking >30s causes a `TimeoutError` on the `waitForTestCompletion` call and fails the whole test before all results are written, so `benchmark-results.json` is not produced and the subsequent assertion step fails.

**Assertions that can fail on Ubuntu 26:**
1. `waitForTestCompletion(page, 30_000)` times out if one render run takes >30s
2. The subsequent "Assert benchmark data was produced" step fails if `benchmark-results.json` is absent
3. `setupStressTestPage`'s `expect(page.getByTestId('test-status')).toHaveText('idle', { timeout: 10000 })` could also time out under heavy load

## 3. Playwright Config Files and Which Jobs Use Them

| Config file | Used by job | Runner installs from |
|---|---|---|
| `web-app/playwright.config.ts` | `frontend-throughput` | `cd web-app && npx playwright install --with-deps chromium` |
| `web-app/playwright.config.ts` | `e2e-latency` | `cd web-app && npx playwright install --with-deps chromium` |
| `tests/e2e/playwright.config.ts` | `e2e-perf` | `cd tests/e2e && npx playwright install --with-deps chromium` |
| `website/playwright.config.ts` | (not used by any benchmark job) | — |

**All three Playwright-using jobs** (`frontend-throughput`, `e2e-latency`, `e2e-perf`) specify `runs-on: ubuntu-latest`, which now resolves to Ubuntu 26. All three need the pin to `ubuntu-24.04`.

The `go-tier1` and `go-tier2` jobs also use `ubuntu-latest` but do not invoke Playwright; pinning them is optional but advisable for reproducibility.

## 4. The `e2e-perf` Config vs `web-app` Config

`tests/e2e/playwright.config.ts` is structurally different from `web-app/playwright.config.ts`:
- It defines a `perf` project (registered only when `--project=perf` or `PW_PERF=1` is set), with `testMatch: '**/perf/**/*.spec.ts'`, `timeout: 600000` (10 min), `retries: 0`, `actionTimeout: 90000`, `navigationTimeout: 180000`.
- The job in `benchmark.yml` calls `npx playwright test --project=perf` from `tests/e2e/` — so this config is the operative one for the `e2e-perf` job.
- The `web-app/playwright.config.ts` is the operative config for `frontend-throughput` and `e2e-latency`.

## 5. Infrastructure Helper Changes Needed

For `frontend-throughput` timing fix, the target is `waitForTestCompletion(page, 30_000)` in the **spec itself** (not helpers.ts). The helpers.ts default is `60_000` for a different signature — the benchmark spec explicitly overrides it with `30_000`. To fix the Ubuntu 26 timeout, the spec call should be raised (e.g. to `60_000`) and/or `test.setTimeout` should be raised from `300_000` to `600_000`.

No changes needed in `helpers.ts` itself for the timeout fix — the constant `STATUS_CHANGE_TIMEOUT_MS = 5000` governs only the initial click-to-status-change transition, which is not the failing assertion.

## Summary of All Files That Need Changes

1. `.github/workflows/benchmark.yml`
   - `go-tier2` job: add `Install buf`, `Generate protobuf code` (with `buf.gen.go-only.yaml`), and `Generate ent ORM code` steps before `Run full benchmark suite`
   - `frontend-throughput`, `e2e-latency`, `e2e-perf`, and optionally `go-tier1`/`go-tier2`: change `runs-on: ubuntu-latest` to `runs-on: ubuntu-24.04`

2. `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`
   - Raise `waitForTestCompletion(page, 30_000)` to `waitForTestCompletion(page, 60_000)` (or higher)
   - Raise `test.setTimeout(300_000)` to `test.setTimeout(600_000)` to maintain slack

No changes needed to `tests/e2e/playwright.config.ts`, `web-app/playwright.config.ts`, or `helpers.ts`.
