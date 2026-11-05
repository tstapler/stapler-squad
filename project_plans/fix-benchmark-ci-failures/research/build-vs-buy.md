# Build-vs-Buy: Fix Benchmark CI Failures

Research date: 2026-11-04

Source files reviewed:
- `.github/workflows/benchmark.yml` (full)
- `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`
- `web-app/tests/e2e/terminal-stress/helpers.ts`

---

## Problem 1: Ubuntu 26 breaks `playwright install --with-deps`

Three jobs (`frontend-throughput`, `e2e-latency`, `e2e-perf`) all use `runs-on: ubuntu-latest`
and call `npx playwright install --with-deps chromium`. The `--with-deps` flag shells out to
`apt-get` for system-level browser dependencies (libglib, libnss, libatk, etc.). When GitHub
Actions moved `ubuntu-latest` to Ubuntu 26, some packages in the standard apt repository changed
names or became unavailable, causing the step to fail (or stall — the existing retry loop already
has a comment noting 29-minute stalls against Ubuntu archive mirrors).

### Option A — Pin to `ubuntu-24.04`

Change each affected job's `runs-on: ubuntu-latest` to `runs-on: ubuntu-24.04`.

**Pros:**
- One-line change per job; zero risk of test behavioral changes.
- Ubuntu 24.04 LTS is supported on GitHub-hosted runners through at least 2028; Playwright
  explicitly lists 24.04 as a supported platform.
- The retry loop added for the stall issue (`timeout 8m npx playwright install`) remains useful
  on 24.04 for transient mirror flakes.

**Cons:**
- Creates a maintenance note: revisit when GHA deprecates the `ubuntu-24.04` label (years away,
  and GitHub announces these months in advance).
- Does not eliminate the underlying fragility of `--with-deps` apt resolution.

**Risk:** Very low.

### Option B — Upgrade Playwright to latest (Ubuntu 26 support)

Upgrade `@playwright/test` and ensure the pinned `npx playwright install` version matches.

**Pros:**
- If the new version natively supports Ubuntu 26, the `ubuntu-latest` label can stay.
- Keeps pace with upstream browser versions.

**Cons:**
- Playwright's Node API surface changes between minors; upgrades frequently break locator
  patterns, `waitForSelector` semantics, or `--reporter=json` output format, requiring test
  edits.
- Ubuntu 26 Playwright support must be verified; it was not confirmed stable as of research date.
- Upgrade is a separate PR with its own test cycle, delaying the CI fix.

**Risk:** Medium. Introduces new failure modes unrelated to the problem being fixed.

### Option C — Use `mcr.microsoft.com/playwright` Docker image

Add a `container: mcr.microsoft.com/playwright:v1.x-jammy` block to each affected job.

**Pros:**
- Browsers are pre-installed; `--with-deps` is unnecessary; completely OS-agnostic.
- Most robust long-term: no dependency on host runner's apt repositories.

**Cons:**
- Restructures all three jobs significantly. The `container:` block changes how `actions/setup-go`,
  `actions/setup-node`, and `./.github/actions/setup-bun` work (they may need explicit path
  overrides in containers).
- The `e2e-latency` and `e2e-perf` jobs build and run a Go binary inside the container — Go
  toolchain setup inside a Playwright container requires either a multi-stage approach or
  installing Go manually.
- Image pull time (1–2 GB) on every run adds latency.
- Image version must be pinned and bumped alongside Playwright version — two things to keep in sync.

**Risk:** Medium. The job restructuring is non-trivial and introduces new failure modes during
the migration.

### Recommendation: Option A

Pin to `ubuntu-24.04`. The fix is a one-liner, Ubuntu 24.04 has years of GHA support remaining,
and the existing retry loop already handles transient apt-mirror flakiness on 24.04. Options B
and C solve a broader problem but carry disproportionate migration risk for a CI fix.

---

## Problem 2: `go-tier2` silently skips benchmarks needing generated code

The `go-tier2` job (lines 141–178 of `benchmark.yml`) runs:

```yaml
- name: Run full benchmark suite
  run: |
    go test \
      -bench=. \
      -run='^$' \
      -benchmem \
      -count=8 \
      -timeout=30m \
      ./... | tee tier2-bench.txt
```

It has no `buf generate` or `ent generate` steps. Packages under `session/ent/` require the ent
ORM generated files; packages importing `gen/proto/...` require `buf generate` output. When those
files are absent, `go test ./...` reports `[build failed]` for those packages and continues to
the next — no job error, just silent omission from `tier2-bench.txt`.

### Option A — Add `buf generate` + `ent generate` to Tier 2 (match Tier 1)

Copy the three steps from `go-tier1` into `go-tier2`:

```yaml
- name: Install buf
  run: |
    curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.41.0/buf-Linux-x86_64" \
      -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf

- name: Generate protobuf code
  run: buf generate proto --template buf.gen.go-only.yaml

- name: Generate ent ORM code
  run: go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

**Pros:**
- Correct fix. Tier 2 becomes a true full-suite run.
- Steps are already proven in Tier 1; no guesswork.
- Adds ~2–3 minutes to a job with a 50-minute budget.
- Keeps the `--feature sql/upsert` flag that the CLAUDE.md documents as critical.

**Cons:**
- Minor: adds curl download (buf binary) on every run; could be cached, but is fast enough to
  not matter.

**Risk:** Very low.

### Option B — Suppress build errors with `grep -v "build failed"`

Pipe `go test ./...` through `grep -v "[build failed]"`.

**Pros:**
- Zero extra steps.

**Cons:**
- Packages that fail to build produce **no benchmark output at all** — grep suppresses the error
  message but can't manufacture results. The same benchmarks remain silently absent.
- Masks genuine build breakage on main (a real compile error in a non-generated package would
  be hidden).
- Anti-pattern: CI should surface failures, not filter them.

**Risk:** High fragility. Reject.

### Option C — Use specific package paths in Tier 2

Replace `./...` with an explicit list of packages that don't require generated code.

**Pros:**
- No code-gen step required.

**Cons:**
- Defeats Tier 2's design purpose: catching benchmarks in packages not covered by Tier 1's
  targeted regex. The list requires ongoing maintenance as packages are added.
- Benchmarks in generated-code packages (session/ent and proto-dependent packages) remain
  permanently absent from the suite.
- If a future benchmark lands in a generated-code package, it silently stays missing.

**Risk:** Medium — ongoing maintenance burden and permanent blind spot.

### Recommendation: Option A

Add the three code-gen steps from Tier 1 verbatim. It is the only option that makes Tier 2 do
what its name and comments claim: run the full benchmark suite.

---

## Problem 3: `frontend-throughput` per-run timeout

The spec file sets `test.setTimeout(300_000)` (5 minutes total for all 10 runs), but each
individual `waitForTestCompletion` call uses a 30-second timeout:

```ts
// terminal-throughput.spec.ts, line 64
await waitForTestCompletion(page, 30_000);
```

The `waitForTestCompletion` helper's signature (helpers.ts, line 132) has a **default** of 60s:

```ts
export async function waitForTestCompletion(page: Page, timeoutMs: number = 60000): Promise<void>
```

The spec unnecessarily tightens it to 30s. On slower CI runners (canvas renderer, no GPU,
shared GHA hosts), a 100 KB payload render can take >30s, causing a mid-run timeout before
any results are written, which then fails the "Assert benchmark data was produced" gate.

### Option A — Increase `waitForTestCompletion` timeout from 30s to 60s

Change line 64 of `terminal-throughput.spec.ts`:

```ts
// Before
await waitForTestCompletion(page, 30_000);
// After
await waitForTestCompletion(page, 60_000);
```

**Pros:**
- Directly targets the failure mode: the per-run rendering timeout.
- Aligns with the helper's documented default (60s).
- Keeps all 10 runs (8 measured after 2 warmup), preserving statistical confidence.
- 10 runs × 60s worst case = 600s, but test.setTimeout is 300s — if 60s each is insufficient
  on a truly slow runner, the overall test timeout will catch it cleanly rather than the first
  run timing out mid-benchmark. Consider whether test.setTimeout also needs raising to 600s to
  accommodate 60s × 10 runs; at 5–15s per run on a reasonable CI runner, 300s is likely still
  fine, but 600s adds safety.

**Cons:**
- If the CI runner is pathologically slow, 60s may still be insufficient; a second round of
  tuning could be needed.

**Risk:** Very low.

### Option B — Reduce TOTAL_RUNS from 10 to 6

Change `TOTAL_RUNS = 10` to `TOTAL_RUNS = 6` in the spec.

**Pros:**
- Each run gets more time within the fixed 300s test budget.
- Fewer runs complete faster.

**Cons:**
- Reduces post-warmup samples from 8 to 4 (6 - 2 warmup). The spec targets `<20% CV` for
  stability — 4 samples makes meeting that target less reliable.
- Changes the `extra` field reported to benchdiff (`runs=4` vs `runs=8`), breaking
  cross-run baseline comparisons if the history mixes 10-run and 6-run baselines.
- Does not fix the root cause: the per-run timeout is still 30s. With 6 runs and 30s each,
  a slow render on run 3 still times out.

**Risk:** Medium — degrades benchmark quality without fixing the actual timeout.

### Option C — Add `--timeout` to the playwright CLI invocation

Add `--timeout=300000` to the `npx playwright test ...` command in `benchmark.yml`.

**Pros:**
- Explicit and visible in the workflow file.

**Cons:**
- `test.setTimeout(300_000)` inside the spec already overrides any default timeout for this
  test. Adding `--timeout` to the CLI sets the same value that is already in effect.
- Critically: `--timeout` / `test.setTimeout` controls the **overall test timeout**, not each
  individual `expect(...).toHaveText(...)` call inside it. The `waitForTestCompletion(page, 30_000)`
  failure comes from an `expect` with its own `{ timeout: 30000 }` argument — a different
  timeout, unaffected by `--timeout` / `test.setTimeout`.
- This option does not fix the actual failure.

**Risk:** Low — but also zero impact on the problem.

### Recommendation: Option A

Raise the `waitForTestCompletion` argument from 30s to 60s. It directly targets the failure
(per-run rendering timeout exceeding the expect timeout), matches the helper's documented
default, and preserves all 10 runs. If the test.setTimeout of 300s turns out tight with 60s
per run, raise it to 600s in the same change.

---

## Summary of Recommendations

| Problem | Recommended Fix | Effort |
|---|---|---|
| Ubuntu 26 / `--with-deps` | Pin `runs-on: ubuntu-24.04` on `frontend-throughput`, `e2e-latency`, `e2e-perf` | 3 one-line changes |
| Tier 2 silent skips | Add `buf generate` + `ent generate` steps to `go-tier2` (copy from `go-tier1`) | Copy 3 existing steps |
| Per-run rendering timeout | Raise `waitForTestCompletion(page, 30_000)` to `waitForTestCompletion(page, 60_000)` in spec | 1 one-line change |
