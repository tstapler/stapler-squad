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

  If it returns HTML, capture all four profiles:

  ```bash
  # Goroutine states (qualitative: what are all goroutines doing right now?)
  curl -s "http://localhost:6060/debug/pprof/goroutine?debug=2" > /tmp/goroutines.txt

  # Mutex contention (quantitative: which mutexes are hot?)
  curl -s "http://localhost:6060/debug/pprof/mutex?debug=1" > /tmp/mutex.txt

  # Scheduler blocking (quantitative: which goroutines block the scheduler longest?)
  curl -s "http://localhost:6060/debug/pprof/block?debug=1" > /tmp/block.txt

  # In-use heap allocations (what is alive right now?)
  curl -s "http://localhost:6060/debug/pprof/heap?debug=1" > /tmp/heap.txt

  # Allocation rate (what allocates most often, even if short-lived?)
  curl -s "http://localhost:6060/debug/pprof/allocs?debug=1" > /tmp/allocs.txt
  ```

  If the server is not running with `--profile`, restart it:
  ```bash
  make restart-web PROFILE_FLAGS="--profile"
  ```

  ---

  ## Phase 1 — Read the Profiles

  ### How to interpret each profile

  **mutex** — the most actionable for latency.
  - Format: `<cycles> <count> @ <addrs>`
  - `cycles` = total CPU cycles spent waiting on this lock (higher → more contention)
  - `count` = how many times goroutines waited (high count at low cycles = many short waits; low count at high cycles = long waits)
  - Look for: your own packages (`github.com/tstapler/stapler-squad`) in the stack, especially inside loops or hot-path handlers.
  - Red flag: `log.(*Logger).output` in the stack — stdlib log holds a mutex per write; any hot-path debug `Printf` call serializes every goroutine that hits it.

  **block** — scheduler delays from channel/select operations.
  - Same format as mutex.
  - High `cycles` on `runtime.selectgo` inside event loops is normal (timer fires). Abnormally high `count` (>10K) on a per-connection goroutine is a sign of excessive goroutine wake-ups.
  - Red flag: >10K blocks on a `streamVia*` or `handleClient` goroutine with a short lifetime.

  **allocs** — allocation rate (lifetime may be short).
  - Format: `<in-use-count>: <in-use-bytes> [<total-count>: <total-bytes>]`
  - Second pair `[total-count: total-bytes]` is the rate metric — even if objects are freed quickly, allocating millions of them adds GC pressure.
  - Red flag: proto `Marshal`/`Unmarshal` allocating on every streaming frame, or ORM queries returning full rows when only one field is needed.

  **heap** — live allocations at snapshot time.
  - Same format; first pair is the rate metric here.
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

  Extract the top-5 stacks from mutex and block profiles, filtering to stapler-squad frames:

  ```bash
  grep -E "^[0-9]+ [0-9]+ @|#.*github.com/tstapler" /tmp/mutex.txt | head -60
  grep -E "^[0-9]+ [0-9]+ @|#.*github.com/tstapler" /tmp/block.txt | head -60
  grep -E "^[0-9]+: [0-9]+ \[[0-9]+:|#.*github.com/tstapler" /tmp/allocs.txt | head -60
  ```

  Fill in this table (sort by cycles × count for mutex; by count for block):

  | Rank | Profile | Location | cycles | count | Root cause hypothesis |
  |------|---------|----------|--------|-------|-----------------------|
  | 1 | mutex | file:line | ... | ... | ... |
  | 2 | block | file:line | ... | ... | ... |
  | … | … | … | … | … | … |

  ### Known recurring hotspots in this codebase (as of 2026-05-02 profiling session)

  | Issue | Location | Profile signal | Impact |
  |-------|----------|----------------|--------|
  | `log.DebugLog.Printf` in hot poll loop | `session/instance_status.go:78` (`GetStatus`) | mutex: 2.2B cycles, 5094 events | Every review queue tick serializes on log mutex |
  | `log.DebugLog.Printf` in content cache hot path | `session/review_queue_poller.go:557,574,581` | mutex: 1.4B cycles, 2607 events | Same pattern — no `DebugLog != nil` guard |
  | `log.DebugLog.Printf` on every `%output` event | `session/tmux/control_mode.go:331` | mutex: 2.7B cycles, 94 events | tmux output path — fires on every terminal byte |
  | `log.DebugLog.Printf` inside streaming send loop | `server/services/connectrpc_websocket.go:629` | block: 23T cycles, 26437 events | Per-frame log call in WebSocket stream goroutine |
  | `EntRepository.Get` before every field update | `session/ent_repository.go:622` via `storage.go:285` | allocs: full row read per update | Should be a direct `UPDATE … WHERE id=?` |

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

# perf:make-it-faster

Connect to the live pprof endpoint, read all five profiles, rank hotspots by CPU cycles
and allocation rate, produce numbered fix proposals with enforcement stubs, and verify
each proposal would have caught the regression via the Reflect & Fix ladder.

## Quick start

```bash
# Server must be running with --profile
make restart-web PROFILE_FLAGS="--profile"

# Capture all profiles in one shot
for p in goroutine mutex block heap allocs; do
  curl -s "http://localhost:6060/debug/pprof/${p}?debug=1" > /tmp/ss-${p}.txt
done

# Then invoke this command — it reads the files and does the rest
```

## Profile quick-reference

| Profile | Primary metric | What to look for |
|---------|---------------|-----------------|
| `mutex` | cycles waiting for a lock | stdlib `log.Printf` in hot paths; RWMutex on read-heavy paths |
| `block` | cycles blocked in select/chan | abnormally high `count` on per-connection goroutines |
| `allocs` | total-bytes column `[N: X]` | proto Marshal per frame, ORM full-row reads |
| `heap` | in-use objects | large objects without pool; compress encoder per request |
| `goroutine` | goroutine count and state | leaks (`[sleep, X minutes]`), lock storms (`[semacquire]`) |

## Enforcement ladder

```
1. Compile time  → type / interface change
2. Lint rule     → golangci-lint custom check or existing rule
3. Benchmark     → AllocsPerRun or ns/op regression gate
4. Unit test     → asserts pre-fix code fails
5. CLAUDE.md     → last resort only
```

## Known hotspots (as of 2026-05-02)

| Location | Profile | Cycles | Count | Fix direction |
|----------|---------|--------|-------|---------------|
| `session/instance_status.go:78` | mutex | 2.2B | 5094 | remove debug Printf from GetStatus hot path |
| `session/review_queue_poller.go:557` | mutex | 1.4B | 2607 | gate behind `DebugLog != nil` or remove |
| `session/tmux/control_mode.go:331` | mutex | 2.7B | 94 | remove Printf from %output hot path |
| `server/services/connectrpc_websocket.go:629` | block | 23T | 26437 | remove per-frame debug log from stream goroutine |
| `session/ent_repository.go:622` via `storage.go:285` | allocs | — | — | direct UPDATE instead of Get + update |
