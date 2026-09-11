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

  ### Phase 0a — If `:6060` returns "cpu profiling already in use"

  Go's CPU profiler is a single process-wide lock — if the local observability stack
  (`~/dotfiles/stapler-scripts/observability/`, see the `observability-grafana-dashboards`
  skill) is running, its Grafana Alloy container continuously scrapes `:6060`'s CPU profile
  on its own schedule, which holds that lock more or less permanently. `go tool pprof
  http://localhost:6060/debug/pprof/profile` will then fail every time, indefinitely — this
  is not a stuck/leaked lock to work around by restarting the app (verified: restarting via
  `launchctl kickstart -k` produces a fresh PID that hits the same "already in use" within
  seconds, because Alloy immediately reconnects to the new process).

  Check whether the stack is up before assuming the app itself is broken:
  ```bash
  docker ps | grep -E "observability-(alloy|pyroscope|grafana)-1"
  ```

  If it's running, pull the CPU/heap/goroutine data from Pyroscope's query API instead of
  `:6060` directly — this is real production data (Alloy has been scraping continuously),
  which is *better* for judging a fix's live steady-state impact than a single manual
  `-seconds=10` snapshot would be anyway:
  ```bash
  # Confirm stapler-squad is a known service in Pyroscope
  curl -s -X POST http://localhost:4040/querier.v1.QuerierService/LabelValues \
    -H "Content-Type: application/json" -d '{"name":"service_name"}'

  # Pull a merged CPU profile over a time window (epoch millis) as pprof-proto-shaped JSON
  NOW=$(date +%s)000; FROM=$(( $(date +%s) - 1800 ))000
  curl -s -X POST http://localhost:4040/querier.v1.QuerierService/SelectMergeProfile \
    -H "Content-Type: application/json" \
    -d "{\"profileTypeID\":\"process_cpu:samples:count:cpu:nanoseconds\",\"labelSelector\":\"{service_name=\\\"stapler-squad\\\"}\",\"start\":$FROM,\"end\":$NOW}" \
    -o /tmp/pyro_profile.json
  ```

  The response is a real `pprof.Profile` proto (fields: `function`, `location`, `mapping`,
  `sample`, `stringTable`) encoded as protojson — **every int64 field is a JSON string**
  (`"id": "1"`, not `1`), and proto3 omits any field at its zero value, so a function/location
  with id/name index 0 has the key missing entirely, not present-as-zero. Write a small script
  rather than reaching for `jq` alone: build `function_id → name` from `stringTable`, walk each
  `location`'s `line[].functionId` to get a location's function chain, then for each `sample`
  sum `value[0]` into both a flat map (leaf function only) and a cum map (every function
  appearing anywhere in that sample's `locationId` chain) — this reproduces `go tool pprof
  -top`'s flat/cum% columns. Divide by the total summed `value[0]` across all samples for the
  percentages. `sampleType`/`periodType` are themselves index-into-`stringTable` pairs, not
  literal strings — decode those the same way if you need to report units.

  Prefer Grafana's own UI (`http://localhost:48300`, the `pprof-profiles` dashboard, default
  creds `admin`/`admin`) over the raw API when a human will be looking at the result — it
  renders an actual flamegraph. Use the raw `QuerierService` API above only when the agent
  itself needs the numbers to reason about (as in this workflow).

  ### Phase 0b — Over-time trend analysis (always run this, not just on lock failure)

  A single `-seconds=N` snapshot only tells you what's hot *right now*. Alloy has been
  continuously scraping all five profile types (`process_cpu`, `memory` alloc+inuse,
  `mutex`, `block`, `goroutine`) into Pyroscope since the process started — use that history
  to tell "growing/regressing" apart from "always been like this" before proposing a fix.
  The `ssq-pprof-profiles` Grafana dashboard already plots CPU/heap/goroutine as
  `timeseries` panels; the API calls below are for when the agent itself needs the numbers
  (trend %, is-it-a-leak) rather than a human looking at a graph.

  ```bash
  # Discover what's actually being collected before assuming a profile type name
  curl -s -X POST http://localhost:4040/querier.v1.QuerierService/LabelValues \
    -H "Content-Type: application/json" -d '{"name":"__profile_type__"}'

  # Pull a bucketed time series for one profile type over a window (epoch millis, step in seconds)
  NOW=$(date +%s)000; FROM=$(( $(date +%s) - 75600 ))000   # 21h window
  curl -s -X POST http://localhost:4040/querier.v1.QuerierService/SelectSeries \
    -H "Content-Type: application/json" \
    -d "{\"profileTypeID\":\"memory:inuse_space:bytes:space:bytes\",\"labelSelector\":\"{service_name=\\\"stapler-squad\\\"}\",\"start\":$FROM,\"end\":$NOW,\"step\":900,\"aggregation\":\"TIME_SERIES_AGGREGATION_TYPE_AVERAGE\"}"
  ```

  **Gotcha — `aggregation` defaults to summing every scrape inside each bucket, which is
  correct for counter-type profiles (`process_cpu` ns, `mutex`/`block` delay ns — these
  represent "time spent during this interval") but wrong for gauge-type profiles
  (`memory:inuse_*`, `goroutine:*count*` — instantaneous snapshots).** Verified in practice:
  a 15-minute bucket with the default (sum) aggregation reported "258GB in-use heap" for a
  local dev process — nonsense, and off by roughly the scrape count per bucket (Alloy scrapes
  far more often than the bucket width). The same query with
  `"aggregation":"TIME_SERIES_AGGREGATION_TYPE_AVERAGE"` reported a believable ~750MB–5.6GB.
  **Always pass `aggregation: AVERAGE` for `memory:inuse_*` and `goroutine:*` series; leave the
  default (sum) for `process_cpu`, `mutex:delay`, `block:delay`, and `memory:alloc_*`.**

  To turn the series into a trend verdict without eyeballing a chart, split it into thirds and
  compare means (first-third vs. last-third) — a script, not `jq` alone, since every numeric
  field in the protojson response is a **string** (`"value": 749815066.05` renders fine, but
  `"timestamp":"1789034682000"` is a quoted int64 — don't `int()` cast without stripping quotes
  first if you hand-roll this in a shell one-liner):

  ```python
  import json
  d = json.load(open("/tmp/series.json"))
  vals = [float(p["value"]) for p in d["series"][0]["points"]]
  n = len(vals); k = max(1, n // 3)
  first, last = sum(vals[:k]) / k, sum(vals[-k:]) / k
  print(f"delta% first-third→last-third: {(last - first) / first * 100:.1f}%")
  ```

  A goroutine-count series that's flat across the window rules out a leak more convincingly
  than one live snapshot ever could. A `memory:inuse_*` series that keeps climbing bucket over
  bucket (not just noisy) is the actual leak signal — a high-but-flat average is not.
  CPU/mutex/block trending down alongside less real work happening (fewer active dev sessions,
  quieter time of day) is expected and not a finding; only flag a trend that's the *opposite*
  of what usage would predict, or a specific function's cum% climbing across snapshots taken
  hours apart (see Phase 2's "verify before trusting" rule — the same discipline applies to
  trend data, not just single-snapshot top-N).

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
  | `GoGitVCSReader.diffShortstatUncached` (via `unfinished.Scanner.worker`→`scanRepo`/`scanWorktree`) | `session/unfinished/gogit_vcs_reader.go:1365` | CPU: 17.6% flat / 18.6% cum of a live 8s `-seconds` sample (2026-09-10), almost entirely `runtime.tryDeferToSpanScan` (GC mark-assist billed to the allocating goroutine) | intrinsic cost of go-git's pure-Go diff/tree-walk on every background scan poll, not a leak — see the architecture note in Phase 2b below before micro-optimizing further; the 2026-08 alloc-focused fix (HEAD tree hash caching, `4cd1d384a`) cut allocation *volume* but not this function's CPU share |

  (2026-05-02 rows — hot-path `DebugLog.Printf` in `instance_status.go`/`review_queue_poller.go`/`control_mode.go`/`connectrpc_websocket.go`, and the ent `Get`-before-update in `ent_repository.go`/`storage.go` — verified fixed on 2026-07-13: no `DebugLog` calls remain in those files, mutex total dropped to ~1.4ms cum, and storage.go carries a "pre-fix: this loop re-queried..." comment. Removed per the prune rule above.)

  (2026-07-13 row — `diffShortstatUncached` racy-clean re-hash + untracked-file walk in `session/unfinished/gogit_vcs_reader.go:850,877,926`, ~397GB cum/66% of all allocations — VERIFIED absent from the 2026-08-06 fresh `-alloc_space` top-N output: no longer appears at all, confirming the underlying fix shipped. Removed per the prune rule above.)

  ---

  ## Phase 2b — Big-swing architectural CPU efficiency

  Run this after the per-line ranking, not instead of it. A pooled buffer or a cached hash map
  is a *tactical* fix — it makes one function cheaper per call. This phase asks a different
  question: is the *shape* of the system generating more calls, more scan passes, or more
  redundant work than the feature actually requires, such that no amount of per-call
  micro-optimization inside that function will move the needle?

  Signals worth treating as "architecture, not a line fix":
  - **A single function's `cum%` in a fresh CPU/alloc sample is large *and* the fix history
    shows a prior tactical fix already reduced its allocation volume without reducing its
    CPU/call-count share** — the remaining cost is inherent to *what* the function does
    (e.g. a pure-Go diff/tree-walk), not *how* it's coded. Reducing bytes-per-call already
    happened; the next lever is calling it less often or replacing the algorithm.
  - **A background poller/scanner's total cost scales with `(number of tracked repos/sessions)
    × (poll frequency)`** rather than with actual user-visible events. Check the ticker
    interval and worker-pool size (`grep -n "NewTicker\|numWorkers" <package>/*.go`) against
    how many repos/sessions are realistically open — if the ticker is a "backstop" behind an
    fsnotify/event-driven primary path (as `session/unfinished/watcher.go`'s 60s ticker and
    `scanner.go`'s fsnotify-driven trigger channel already are for this repo), the architecture
    question is whether the backstop interval or worker count can widen further, not whether
    the per-poll function is fast.
  - **CPU time attributed to `runtime.gcBgMarkWorker`/`tryDeferToSpanScan`/`scanSpan` under a
    `-focus` filter for your own package** means GC mark-assist work is being billed to your
    allocating goroutines — the fix is fewer/smaller allocations upstream (back to Phase 1's
    allocs profile), or in the extreme, whether the workload should allocate at all (e.g. a
    pure-Go library doing tree/diff construction in Go heap vs. shelling out to native `git`,
    which does the same work in a separate process with its own GC-free C allocator — a real
    architectural trade-off: process-spawn overhead and a second binary dependency, versus
    GC pressure on the main process. Don't take this trade without measuring both sides.)

  For each candidate, write one paragraph answering: *what would change if this were redesigned
  from scratch knowing today's actual scale*, then decide whether that redesign is worth a
  `[PerfFix-Arch-N]` proposal (below) or is over-engineering for the current scale. Not every
  large `cum%` number justifies an architectural rewrite — say explicitly when the tactical fix
  (pool, cache, cheaper check before the expensive path) is sufficient and a rewrite is not
  worth the risk.

  ### Template

  ```
  ### [PerfFix-Arch-N] Short title

  **Current shape**: what triggers this work today, how often, at what scale
  **Cost signal**: profile + trend data from Phase 0b/2 that shows this is structural, not a one-line fix
  **Proposed redesign**: the architectural change (e.g. event-driven only, no ticker; replace library X with Y; batch N calls into 1)
  **Trade-off**: what the redesign costs (complexity, a new dependency, weaker consistency, etc.)
  **Estimated impact**: low / medium / high — and whether it's worth it at current scale
  ```

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

  ### General Go hot-path checklist — consult before writing a fix, not after

  Go's runtime is managed, so on this codebase's hot paths (background scanners, streaming
  RPC handlers) the dominant cost is almost always GC pressure from heap allocations, not raw
  instruction count — this matches what Phase 2b already says about `tryDeferToSpanScan`/
  `gcBgMarkWorker` showing up under `-focus`. Check these before proposing a fix, and prefer
  whichever is provably true on `-benchmem` output over guessing:

  - **Preallocate slices/maps with a known or estimated capacity** (`make([]T, 0, n)`) instead
    of letting `append` grow them — repeated growth reallocates and copies. Already the pattern
    this codebase's fixed hotspots follow (e.g. `metas := make([]indexMeta, 0, len(idx.Entries))`
    in `diffShortstatUncached`).
  - **Pool short-lived, frequently-recreated buffers/objects with `sync.Pool`** rather than
    allocating fresh per call — this is exactly PerfFix-1/2/3 in the known-hotspots table
    (pooled `bufio.Scanner` buffers).
  - **Avoid `[]byte`↔`string` conversions in a loop** — each non-elided conversion copies.
    `strings.Builder`/direct `[]byte` manipulation avoids it.
  - **Verify escape analysis on a suspected hot path** with
    `go build -gcflags="-m" ./path/to/pkg 2>&1 | grep "escapes to heap"` — confirms a value you
    expected to stay on the stack didn't, before proposing a fix for it. Don't skip this and
    guess; the compiler's answer is authoritative and free to check.
  - **Avoid interface calls / reflection inside a genuinely hot inner loop** (dynamic dispatch
    blocks inlining) — but weigh this against this codebase's existing interface seams
    (`VCSReader`, `Determiner`) which are deliberate architecture, not accidental hot-path cost;
    don't propose collapsing an interface used once per scan cycle just because interfaces have
    a theoretical cost. Reserve this for a loop that runs thousands of times per call, not per
    top-level call itself.
  - **`sync/atomic` over `sync.Mutex` for simple counters/flags** — already this codebase's
    convention (see `golang-concurrency` skill); only worth a fresh proposal if a *new* mutex
    is found guarding a single counter.
  - **Struct field ordering (largest→smallest) to reduce padding** is a real but usually
    low-impact win here — don't propose it unless a `heap` profile already shows the struct
    itself (not what it points to) as a meaningful fraction of resident bytes; reordering a
    struct nobody profiled as hot is churn, not a fix.
  - **Profile-Guided Optimization (PGO)**: if a fix's expected win is "the compiler should
    inline/devirtualize this better, but I can't point at one line," that's what PGO is
    for — drop a real profile in as `default.pprof` at the module root and rebuild; the
    compiler uses it automatically. This is a fallback for diffuse, whole-program wins, not
    a substitute for fixing a located hotspot from Phase 2.

  **Always confirm the theory against `-benchmem` before shipping the fix** — write the
  benchmark first (this codebase's convention: `<Thing>_bench_test.go`, `b.ReportAllocs()`,
  compare variants as `b.Run` subtests), run it before and after, and quote the allocs/op and
  ns/op delta in the fix's PR description. A plausible-sounding allocation theory that doesn't
  move `-benchmem`'s numbers is not done — see `BenchmarkDiffShortstat_GoGitVsNative` and
  `BenchmarkScanWorktree_DuplicateClassificationWork` in
  `session/unfinished/gogit_vcs_reader_shellout_bench_test.go` for the pattern: compare the
  current implementation against a candidate replacement across a realistic size range, not
  just one data point.

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
  Hand each `[PerfFix-N]` block to **`/go:optimize`**, which walks its own decision tree
  (atomic shadow, RWMutex, TTL cache, direct SQL update, etc.) to pick and implement the
  matching pattern plus its enforcement test — pass it the profile type, file:line, cycles,
  and one-sentence description from the block.
  Do **not** add a CLAUDE.md note unless every other enforcement level is unreachable.

  ---

  ## Phase 5 — Browser / React Profiling

  Run this phase in parallel with or after Go profiling. The app runs at `http://localhost:8543`.
  Playwright is available at `tests/e2e/node_modules/.bin/playwright` (or via the `playwright` MCP
  server's `browser_*` tools if that path isn't populated in this environment).

  **Load the `browser-profiling` skill for this phase** — it is the canonical reference for
  everything below (triage table, `<Profiler>` usage, memory-leak workflow, fix verification) plus
  a section this file doesn't duplicate: analyzing a captured/downloaded trace (`.json`/`.json.gz`)
  programmatically with Perfetto's `trace_processor` — the `go tool pprof -top` equivalent for
  Chrome traces. Reach for that instead of `grep`/`jq`/`json.load`-ing a large trace file by hand;
  a real session trace routinely runs several hundred MB and a naive parse is slow, easy to get
  wrong (wrong pid/thread, double-counted nested slices), and floods context if the raw output
  lands in a conversation.

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
# macOS (launchd): launchctl list com.stapler-squad | grep -A1 '"Program" ='

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

If any of the above returns "cpu profiling already in use" instead of data, the local
observability stack's Grafana Alloy container is continuously scraping `:6060` — see
Phase 0a above for pulling the same data from Pyroscope's query API instead (`:4040`) or
viewing it as a flamegraph in Grafana (`:48300`, `pprof-profiles` dashboard) rather than
fighting the lock (a service restart does not free it — Alloy just reconnects).

Once you have ranked `[PerfFix-N]` findings, hand each one to **`/go:optimize`** to
implement the fix and its enforcement test — this command only produces proposals.

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
