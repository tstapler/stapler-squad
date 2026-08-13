---
description: Connect to the running stapler-squad pprof endpoint, interpret the profiles, identify the top performance bottlenecks, propose concrete improvements ranked by impact, and codify each fix with a test or lint rule so regressions cannot silently reappear.
prompt: |
  # perf:make-it-faster — Profiling → Proposal → Enforcement

  You are performing a live performance audit of the running stapler-squad process.
  Work through four phases in order and produce concrete, actionable output.

  ---

  ## Phase 0 — Connect and Capture

  The server must be running with `--profile` to expose pprof. Check first:

  ```bash
  curl -s http://localhost:6060/debug/pprof/ | head -5
  ```

  **Context budget: do NOT curl mutex/block/heap/allocs to raw text files.** Those endpoints
  return address-only stacks (100K+ lines for allocs/heap on a long-running process) that must
  be manually symbolized and aggregated — that parsing work (grep/awk/python over raw pprof
  text) is expensive in tokens and error-prone (cycles/counts misaligning with the wrong stack
  frame). `go tool pprof` does the fetch, symbolization, and aggregation in one step and prints
  only the ranked top-N, typically 20-30 lines total. Find the binary once per session:

  ```bash
  systemctl --user show stapler-squad -p ExecStart | grep -oP '(?<=path=)\S+'
  ```

  Then, for mutex/block/allocs/heap, go straight to ranked output instead of a raw capture:

  ```bash
  cd /path/to/stapler-squad  # repo root containing the binary above

  # Mutex contention — output is pre-converted to ms/%, no manual cycle math needed
  go tool pprof -top -nodecount=15 ./stapler-squad http://localhost:6060/debug/pprof/mutex

  # Scheduler blocking
  go tool pprof -top -nodecount=15 ./stapler-squad http://localhost:6060/debug/pprof/block

  # Allocation rate, filtered to your own packages, in MB
  go tool pprof -top -alloc_space -unit=mb -nodecount=20 \
    -focus='github.com/tstapler/stapler-squad' ./stapler-squad http://localhost:6060/debug/pprof/allocs

  # Live heap (in-use, not cumulative)
  go tool pprof -top -inuse_space -unit=mb -nodecount=20 \
    -focus='github.com/tstapler/stapler-squad' ./stapler-squad http://localhost:6060/debug/pprof/heap
  ```

  Once the top function is identified, get its line-level breakdown directly instead of grepping
  raw stacks for it — this is the fastest way to find the exact allocating/blocking line:

  ```bash
  go tool pprof -list='FunctionName$' -alloc_space -unit=mb ./stapler-squad http://localhost:6060/debug/pprof/allocs
  ```

  Goroutine states are the one profile still worth a raw capture, since the qualitative
  state-count grep below only needs debug=2's per-goroutine header lines, not symbolized stacks:

  ```bash
  curl -s "http://localhost:6060/debug/pprof/goroutine?debug=2" > /tmp/goroutines.txt
  grep "^goroutine" /tmp/goroutines.txt | sed 's/goroutine [0-9]* //' | sort | uniq -c | sort -rn
  ```

  If the server is not running with `--profile`, restart it:
  ```bash
  make restart-web PROFILE_FLAGS="--profile"
  ```

  ---

  ## Phase 1 — Read the Profiles

  ### How to interpret each profile

  All four are read via `go tool pprof -top` per Phase 0 — the flat/cum % columns already do the
  ranking, so treat "high in the `-top` output" as the signal rather than re-deriving it from raw
  cycles/byte counts.

  **mutex** — the most actionable for latency.
  - `-top` output columns are `flat flat% sum% cum cum%`, already in wall-time (ms), not raw cycles.
  - Look for: your own packages (`github.com/tstapler/stapler-squad`) in the stack, especially inside loops or hot-path handlers.
  - Red flag: `log.(*Logger).output` in the stack — stdlib log holds a mutex per write; any hot-path debug `Printf` call serializes every goroutine that hits it.

  **block** — scheduler delays from channel/select operations.
  - Same column format as mutex.
  - High `cum` on `runtime.selectgo` inside long-lived event loops (per-session control loops, streaming goroutines) is normal — it's just the goroutine idling on its next event, not contention. Judge it by `count` from the raw profile instead: abnormally high `count` (>10K) on a goroutine with a *short* lifetime is the actual signal.
  - Red flag: >10K blocks on a `streamVia*` or `handleClient` goroutine with a short lifetime.

  **allocs** — allocation rate (lifetime may be short). Use `-alloc_space` (total bytes ever allocated) to catch high-churn code, not just what's currently live.
  - Red flag: proto `Marshal`/`Unmarshal` allocating on every streaming frame, or ORM queries returning full rows when only one field is needed.
  - Red flag: a background poller/scanner whose `-focus`'d cumulative % is disproportionate to its purpose (e.g. a single function accounting for >20% of all allocations app-wide) — that's a sign it's redoing full-cost work on every poll instead of caching or narrowing scope.

  **heap** — live allocations at snapshot time. Use `-inuse_space`.
  - Red flag: compression encoder `blockEnc.init` without a `sync.Pool` — should show pool-resident objects, not fresh allocations.

  **goroutines** — qualitative health check.
  - Count goroutines by state with:
    ```bash
    grep "^goroutine" /tmp/goroutines.txt | sed 's/goroutine [0-9]* //' | sort | uniq -c | sort -rn
    ```
  - Normal states: `[select]`, `[chan receive]`, `[IO wait]`
  - Red flags: many goroutines in `[semacquire]` (lock contention) or `[sleep, X minutes]` (goroutine leak)

  ---

  ## Phase 2 — Rank Bottlenecks

  The `-top -focus='github.com/tstapler/stapler-squad'` commands from Phase 0 already rank
  candidates by weight — this phase is about turning the top 3-5 lines of each into a table, not
  re-extracting from raw text. For each candidate that clears a red flag from Phase 1, drill into
  its exact line with `-list='<FuncName>$'` (Phase 0) and copy the `file:line` and metric it shows.

  Fill in this table (mutex/block ranked by `-top`'s cum ms; allocs/heap by cum MB or cum%):

  | Rank | Profile | Location | Metric | Root cause hypothesis |
  |------|---------|----------|--------|-----------------------|
  | 1 | mutex | file:line | X ms | ... |
  | 2 | allocs | file:line | X MB cum, Y% of total | ... |
  | … | … | … | … | … |

  ### Known recurring hotspots — verify before trusting, prune what's gone

  This table is a memory aid, not ground truth — it goes stale the moment a listed fix ships.
  Before using an entry to prioritize work, re-check it against this run's fresh `-top` output;
  delete rows that no longer appear as a top-N candidate, and add any new top-N finding whose
  cumulative share is large enough to be worth remembering next time (rule of thumb: >5% of
  total allocation, or a mutex/block entry inside the top 5 by cum ms).

  | Issue | Location | Profile signal | Impact |
  |-------|----------|----------------|--------|
  | `findConversationFilePath` fresh 1MB scanner buffer per file during walk | `session/history.go:360` | allocs: 36.05% cum (2026-08-06) | see PerfFix-1 below — pool the `bufio.Scanner` buffer instead of allocating fresh per file |
  | `ArtifactExtractor.scanFile` fresh 10MB scanner buffer per call | `session/artifacts/scan.go:37` | allocs: 18.56%/20.96% cum (2026-08-06) | see PerfFix-2 below — shared pool with `tokens/parser.go` |
  | `Parser.ParseReader` fresh 10MB scanner buffer per call | `session/tokens/parser.go:73` | allocs: 17.49%/20.46% cum (2026-08-06) | see PerfFix-3 below — shared pool with `artifacts/scan.go` |

  (2026-05-02 rows — hot-path `DebugLog.Printf` in `instance_status.go`/`review_queue_poller.go`/`control_mode.go`/`connectrpc_websocket.go`, and the ent `Get`-before-update in `ent_repository.go`/`storage.go` — verified fixed on 2026-07-13: no `DebugLog` calls remain in those files, mutex total dropped to ~1.4ms cum, and storage.go carries a "pre-fix: this loop re-queried..." comment. Removed per the prune rule above.)

  (2026-07-13 row — `diffShortstatUncached` racy-clean re-hash + untracked-file walk in `session/unfinished/gogit_vcs_reader.go:850,877,926`, ~397GB cum/66% of all allocations — VERIFIED absent from the 2026-08-06 fresh `-alloc_space` top-N output: no longer appears at all, confirming the underlying fix shipped. Removed per the prune rule above.)

  ---

  ## Phase 3 — Propose Improvements

  For each bottleneck, propose a concrete fix at the **earliest achievable enforcement level**:

  ```
  1. Compile time  → type change, interface constraint
  2. Lint rule     → custom golangci-lint rule, existing staticcheck rule
  3. Benchmark     → must regress detectably if the fix is reverted
  4. Unit test     → asserts correct behavior before/after
  5. CLAUDE.md     → only when 1–4 are genuinely unreachable
  ```

  ### Template for each proposal

  ```
  ### [PerfFix-N] Short title

  **Profile signal**: mutex / block / allocs — file:line — X cycles, Y events
  **Root cause**: one sentence
  **Fix**: what to change and where
  **Enforcement**: lint rule name / benchmark name / test name that would have caught it
  **Estimated impact**: low / medium / high — why
  ```

  ---

  ## Phase 4 — Codify (Reflect & Fix)

  Apply the Reflect & Fix framework to every fix you propose.

  For **mutex contention from hot-path logging**:
  - Category: **Semantic/Intent** — the debug log is syntactically valid but semantically wrong in a tight loop
  - Enforcement: lint rule that flags `log.DebugLog.Printf` calls not guarded by `if log.DebugLog != nil` inside functions whose names match `*poll*`, `*check*`, `*stream*`, `*handle*`
  - Write the rule in `buildSrc/` or as a golangci-lint custom check; add a test that fires on the bad pattern and is silent on the guarded form
  - Add to `.golangci.yml` under `custom-gcl` or `revive` rules

  For **allocation-per-frame in streaming paths**:
  - Category: **Integration Gap** — proto allocation per frame is correct in isolation but adds up at stream throughput
  - Enforcement: benchmark `BenchmarkStreamViaControlMode` that asserts `allocs/op == 0` for the hot path (use `testing.AllocsPerRun`)
  - Must fail before the fix (pooled protos not yet introduced) and pass after

  For **read-before-write in ORM updates**:
  - Category: **API Contract Gap** — the update method's interface doesn't signal that it does a read first
  - Enforcement: integration test `TestUpdateFieldInRepo_UsesDirectUpdate` that counts SQL statements and asserts `SELECT` count == 0 for a field update

  ### Verification table

  | Fix | Enforcement | Pre-fix behaviour | Verdict |
  |----|------------|------------------|---------|
  | Remove hot-path `DebugLog.Printf` | lint rule | fires on pre-fix code ✓ | catches it |
  | Pool proto in stream loop | `BenchmarkStream_AllocsPerOp` | allocs > 0 ✓ | catches it |
  | Direct SQL update | `TestUpdateFieldInRepo_NoSelect` | sees SELECT ✓ | catches it |

  ---

  ## Output Format

  Produce:
  1. The filled-in Phase 2 ranking table
  2. One `### [PerfFix-N]` block per proposed fix (minimum 3, maximum 10)
  3. The Phase 4 verification table
  4. A prioritised "what to tackle first" recommendation (2–3 sentences)

  Do **not** implement the fixes — this command produces proposals for agent hand-off.
  Do **not** add a CLAUDE.md note unless every other enforcement level is unreachable.

  ---

  ## Phase 5 — Browser / React Profiling

  Run this phase in parallel with or after Go profiling. The app runs at `http://localhost:8543`.
  Playwright is available at `tests/e2e/node_modules/.bin/playwright`.

  ### 5a — Capture numeric baseline via Playwright

  Write and run `/tmp/ss-browser-baseline.js`:

  ```javascript
  const { chromium } = require('/Users/tylerstapler/IdeaProjects/stapler-squad/tests/e2e/node_modules/playwright-core');

  async function captureBaseline(label, scenarioFn) {
    const browser = await chromium.launch();
    const page = await browser.newPage();

    await page.addInitScript(() => {
      window.__perfData__ = { longTasks: [] };
      new PerformanceObserver(list => {
        list.getEntries().forEach(e => window.__perfData__.longTasks.push({
          duration: e.duration, startTime: e.startTime
        }));
      }).observe({ entryTypes: ['longtask'] });
    });

    await browser.startTracing(page, {
      path: `/tmp/trace-${label}.json`,
      screenshots: false,
      categories: ['devtools.timeline', 'v8', 'blink.user_timing', 'disabled-by-default-v8.cpu_profiler'],
    });

    const before = await page.metrics();
    await page.goto('http://localhost:8543', { waitUntil: 'networkidle' });
    await scenarioFn(page);
    const after = await page.metrics();
    await browser.stopTracing();

    const longTasks = await page.evaluate(() => window.__perfData__.longTasks);
    console.log(`\n=== ${label} ===`);
    console.log({
      scriptDuration:  (after.ScriptDuration  - before.ScriptDuration).toFixed(3) + 's',
      layoutCount:      after.LayoutCount      - before.LayoutCount,
      recalcStyleCount: after.RecalcStyleCount - before.RecalcStyleCount,
      heapGrowthMB:    ((after.JSHeapUsedSize  - before.JSHeapUsedSize) / 1024 / 1024).toFixed(2) + 'MB',
      nodes:            after.Nodes            - before.Nodes,
    });
    console.log(`Long tasks (>50ms): ${longTasks.length}`, longTasks.map(t => Math.round(t.duration) + 'ms'));
    console.log(`Trace saved: /tmp/trace-${label}.json`);
    await browser.close();
  }

  captureBaseline('initial-load', async (page) => {
    await page.waitForSelector('body');
    await page.waitForTimeout(1000);
  }).then(() =>
  captureBaseline('session-list-scroll', async (page) => {
    await page.waitForSelector('body');
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press('ArrowDown');
      await page.waitForTimeout(100);
    }
  })).catch(console.error);
  ```

  ```bash
  node /tmp/ss-browser-baseline.js
  ```

  ### 5b — Interpret results

  **Long tasks (>50ms)**: each one blocks user input and shows up as red-flagged bars in the Performance panel.
  Load `/tmp/trace-initial-load.json` into Chrome DevTools → Performance tab for the flamechart.

  **Key metrics to flag**:
  | Metric | Warning threshold | Critical threshold |
  |--------|------------------|--------------------|
  | `scriptDuration` on initial load | > 0.5s | > 1.0s |
  | `layoutCount` per interaction | > 10 | > 50 |
  | `heapGrowthMB` after 10 interactions | > 5MB | > 20MB |
  | Long task count on load | > 3 | > 10 |
  | Single long task duration | > 100ms | > 500ms |

  ### 5c — React-specific checks

  Add a temporary `<Profiler>` wrapper in the dev build around the sessions list:

  ```tsx
  import { Profiler, type ProfilerOnRenderCallback } from 'react';

  const onRender: ProfilerOnRenderCallback = (id, phase, actualDuration, baseDuration) => {
    if (actualDuration > 16)
      console.warn(`[Profiler] ${id} (${phase}): ${actualDuration.toFixed(1)}ms  ratio: ${(actualDuration/baseDuration).toFixed(2)}`);
  };

  <Profiler id="SessionList" onRender={onRender}>
    <SessionList />
  </Profiler>
  ```

  Key ratio: `actualDuration / baseDuration` → near 1.0 = memoization absent; near 0.1 = working.

  ### 5d — Bundle size check

  ```bash
  cd web-app && npm run build 2>/dev/null | tail -20
  # Then inspect the largest chunks
  ls -lah web-app/.next/static/chunks/*.js 2>/dev/null | sort -k5 -rh | head -10
  # or for Vite/CRA:
  ls -lah web-app/dist/assets/*.js 2>/dev/null | sort -k5 -rh | head -10
  ```

  ### 5e — Browser fix proposals (same template as Phase 3)

  For each browser bottleneck found, produce a `### [PerfFix-Browser-N]` block:
  - **Signal**: metric name + value
  - **Root cause**: one sentence
  - **Fix**: what component/hook to change
  - **Enforcement**: Jest/RTL test or Playwright perf assertion that would catch regression

---

# perf:make-it-faster

Connect to the live pprof endpoint, read all five profiles, rank hotspots by CPU cycles
and allocation rate, produce numbered fix proposals with enforcement stubs, and verify
each proposal would have caught the regression via the Reflect & Fix ladder.

## Quick start

```bash
# Server must be running with --profile
make restart-web PROFILE_FLAGS="--profile"

# Locate the binary once (needed for go tool pprof symbolization)
systemctl --user show stapler-squad -p ExecStart | grep -oP '(?<=path=)\S+'

# Ranked, symbolized, pre-aggregated — no raw-text capture or manual parsing needed
BIN=./stapler-squad  # path from above
go tool pprof -top -nodecount=15 $BIN http://localhost:6060/debug/pprof/mutex
go tool pprof -top -nodecount=15 $BIN http://localhost:6060/debug/pprof/block
go tool pprof -top -alloc_space -unit=mb -focus='github.com/tstapler/stapler-squad' -nodecount=20 $BIN http://localhost:6060/debug/pprof/allocs
go tool pprof -top -inuse_space -unit=mb -focus='github.com/tstapler/stapler-squad' -nodecount=20 $BIN http://localhost:6060/debug/pprof/heap

# Goroutine states are the one profile still worth a raw capture + grep (compact output)
curl -s "http://localhost:6060/debug/pprof/goroutine?debug=2" > /tmp/ss-goroutine.txt
grep "^goroutine" /tmp/ss-goroutine.txt | sed 's/goroutine [0-9]* //' | sort | uniq -c | sort -rn

# Then drill into the top hit's exact line:
go tool pprof -list='FunctionName$' -alloc_space -unit=mb $BIN http://localhost:6060/debug/pprof/allocs
```

Avoid `curl ... ?debug=1 > /tmp/ss-<profile>.txt` for mutex/block/heap/allocs — those files run
100K+ lines on a long-running process and force manual grep/awk/python symbolization that
`go tool pprof` already does. Only goroutine is worth a raw capture, for the state-count grep above.

## Profile quick-reference

| Profile | Primary metric (via `go tool pprof -top`) | What to look for |
|---------|---------------|-----------------|
| `mutex` | cum ms waiting for a lock | stdlib `log.Printf` in hot paths; RWMutex on read-heavy paths |
| `block` | cum ms blocked in select/chan | abnormally high raw `count` on short-lived per-connection goroutines |
| `allocs` | cum MB (`-alloc_space`) | proto Marshal per frame, ORM full-row reads, a poller redoing full-cost work every tick |
| `heap` | cum MB (`-inuse_space`) | large objects without pool; compress encoder per request |
| `goroutine` | goroutine count and state | leaks (`[sleep, X minutes]`), lock storms (`[semacquire]`) |

## Enforcement ladder

```
1. Compile time  → type / interface change
2. Lint rule     → golangci-lint custom check or existing rule
3. Benchmark     → AllocsPerRun or ns/op regression gate
4. Unit test     → asserts pre-fix code fails
5. CLAUDE.md     → last resort only
```

## Browser quick-reference

```bash
# Run browser baseline (app must be on localhost:8543)
node /tmp/ss-browser-baseline.js

# Inspect bundle chunks by size
ls -lah web-app/.next/static/chunks/*.js 2>/dev/null | sort -k5 -rh | head -10

# JS coverage (unused code)
# Run captureBaseline with page.coverage.startJSCoverage() — see browser-profiling skill
```

| Signal | Tool | Where to look |
|--------|------|---------------|
| Long tasks on load | Playwright `page.metrics()` + `PerformanceObserver longtask` | > 3 tasks or > 100ms each |
| React re-render cascade | `<Profiler onRender>` | `actualDuration / baseDuration` near 1.0 |
| Layout thrashing | Performance panel → "Forced reflow" | `layoutCount` > 10 per interaction |
| Memory leak | Playwright heap delta across 10 cycles | > 5MB growth |
| Oversized bundle | `source-map-explorer` or `.next/static/chunks` | chunk > 500KB unparsed |

## Known hotspots — prune stale rows each session, don't just append

Re-check every row against this run's fresh `go tool pprof -top` output before relying on it —
a shipped fix makes a row wrong, not just outdated. Delete rows that no longer show up in the
top-N; only add a row when its cumulative share is large enough to matter next time (>5% of
total allocation, or top-5 by cum ms for mutex/block).

| Location | Profile | Signal (as of session date) | Fix direction |
|----------|---------|--------|---------------|
| `session/history.go:360` (`findConversationFilePath`) | allocs | 36.05% cum (2026-08-06) | pool the 1MB `bufio.Scanner` buffer instead of allocating fresh per file during `filepath.Walk` |
| `session/artifacts/scan.go:37` (`scanFile`) | allocs | 18.56%/20.96% cum (2026-08-06) | pool the 10MB scanner buffer, share helper with `tokens/parser.go` |
| `session/tokens/parser.go:73` (`ParseReader`) | allocs | 17.49%/20.46% cum (2026-08-06) | pool the 10MB scanner buffer, share helper with `artifacts/scan.go` |

(2026-05-02 rows for `instance_status.go`/`review_queue_poller.go`/`control_mode.go`/`connectrpc_websocket.go` hot-path `DebugLog.Printf` calls and the `ent_repository.go`/`storage.go` Get-before-update — verified fixed on 2026-07-13 and pruned: no `DebugLog` calls remain in those files, mutex total is ~1.4ms cum, storage.go now carries a "pre-fix: this loop re-queried..." comment.)

(2026-07-13 row for `session/unfinished/gogit_vcs_reader.go:850,877,926` racy-clean re-hash + untracked-file walk, ~397GB cum/66% of allocations — VERIFIED absent from the 2026-08-06 fresh `-alloc_space` top-N output, pruned per the rule above.)
