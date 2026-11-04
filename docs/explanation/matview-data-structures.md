# Data structures for the materialized VCS status view

Why the event-fed materialized view ([ADR-034, PR #1309](https://github.com/tstapler/stapler-squad/pull/1309)) uses the data structures it does, and where it should use something else. Written against its first implementation, [PR #1312](https://github.com/tstapler/stapler-squad/pull/1312) (`internal/matview`, head `24db6edb`).

Owner question: *"for the materialized view can you evaluate all of the datastructures that we need to implement it? or should?"*

## Short answer

**Should we use libraries for these structures? Almost never.** At this scale (tens of live keys, at most 512 pinned and 256 unpinned entries, event bursts of low hundreds per second) every structure is fast enough in its simplest form, so the criteria that decide are correctness under concurrency and simplicity, not ns/op. Of ten structures, zero need a new third-party dependency (no new module; one indirect-to-direct promotion, `xxhash`), and two library candidates fail on semantics, not speed:

- `golang.org/x/sync/semaphore` adds nothing: the view does its own ordering and only needs `TryAcquire`/`Release`, which the existing channel semaphore already provides (its `Acquire` waiters are FIFO, so it could not do the ordering itself).
- `hashicorp/golang-lru/v2` has no pin or veto API; pins would have to be emulated with `Remove` on pin and `Add` on unpin, which is just the `container/list` design with a new `require` line and an MPL-2.0 licence.

The hand-rolled core in #1312 is the right shape: one mutex over `map[K]*entry`, per-key `time.AfterFunc` debounce, per-subscriber latest-wins map. The review found **four correctness issues** (M1-M3 and S1), each reproduced by a throwaway test, plus an unreproduced panic-safety gap (S2); see [Review of #1312](#appendix-b-review-of-1312). None is a data-structure choice; all are lifecycle and accounting bugs.

Labels used below: **VERIFIED** means a command was run and its output is quoted; **INFERRED** means reasoned from code or docs without running it.

## Appendix A: evaluation, structure by structure

### Method and environment

All benchmarks are throwaway files placed inside the #1312 package (so they can reach unexported types) and are not committed; the source is attached to the PR as a comment. Each command ran three times and the table shows the median.

- Host: Apple M5 Max, 18 CPUs, `go1.26.6 darwin/arm64`. The machine was heavily loaded by endpoint security software: load average 135 to 165 during the runs (`uptime`). Absolute numbers are inflated and noisy; treat ratios as the signal and order-of-magnitude as the claim.
- Scale used: 40 keys of ~120 bytes (`repo|activeDir` shape), 14 repo roots, 768 entries for eviction (256 unpinned + 512 pinned), 8/64/512 queued items for the queue.
- `-cpu 1,4,8` was used only for the read path (the one structure with contended readers). Everything else is driven by one goroutine (the FSEvents callback, the pump, one subscriber), so parallel runs would measure nothing real.

```sh
cd internal/matview   # on the #1312 branch, with zz_bench_test.go added
go test -run xxx -bench StoreGet -benchtime=200ms -cpu 1,4,8 -count=3 .
go test -run xxx -bench 'Debounce|QueuePick|EvictOverCap|SubscriberConflate|ETag|Ingest' -benchtime=100ms -cpu 1 -count=3 .
go test -run TestMemoryFootprint -v -count=3 .
```

### Decision table

| # | Structure | #1312 chose | Decision | Why (one line) |
|---|---|---|---|---|
| 1 | key to entry store | `sync.Mutex` + `map`, entries hold a `Versioned` value | **Keep.** Optional later: `xsync.Map` for the read path only | One lock is needed anyway for timers, queue and pins; 140 ns contended is irrelevant at tens of reads per second |
| 2 | per-key debounce / max-wait | `time.AfterFunc` per key, `Stop`+re-arm per event, generation guard | **Keep** (not a heap, not a wheel) | 259 ns per event, 2 allocs; 300 events/s costs about 0.008% of a core |
| 3 | priority queue + concurrency bound | `map` scan per dequeue, `Limiter` interface, 50 ms retry poll | **Replace the scan with a 3-bucket FIFO** (nice-to-have); keep the `Limiter` seam | Scan is O(n) per pick: 2.2 us/pick at n=512 vs 13 ns/pick for buckets |
| 4 | eviction with pins | collect all evictable entries on every new-key insert, `sort.Slice` when over cap | **Keep the policy; replace the sort with an O(n) oldest-pick when evicting one entry, or a `container/list` LRU** (nice-to-have). No library | 33 us per insert at cap under the global lock; `golang-lru` has no pin |
| 5 | change feed with conflation | per-subscriber `map[K]Update` + 1-slot notify channel | **Keep.** Revision cursor/ring buffer rejected | Latest-wins per key is exactly the requirement; memory is bounded by `MaxKeysPerSub` |
| 6 | in-flight dedup | own `running`/`rerun`/`waiters` state, no singleflight | **Keep own coalescing, but add cancel-on-last-waiter-leaves** (must-fix) | singleflight cannot express "started after my request" nor cancel on last leaver |
| 7 | event ingestion | not in #1312 (belongs to the change source) | **Map set + parent-walk lookup + reuse `classifyGitEvent`.** No bloom/trie/ring buffer | 94 ns per event, 0 allocs; the kernel already flags overflow |
| 8 | materialized value | `Versioned[V]` held by value, `V` is whatever the caller returns | **Keep; document that `V` is immutable.** No string interning | Copy-on-write is free if readers never mutate; payload is unique strings |
| 9 | stale-tier tracking | `ComputedAt` compared lazily at read | **Keep.** No expiry wheel | Zero background cost; a wheel would add timers to avoid one `time.Sub` per read |
| 10 | backstop scheduling | not in #1312 | **One self-rearming jittered `AfterFunc` per watched repo** | Go's runtime already keeps timers in per-P heaps; a custom scheduler heap is a second copy of it |
| + | ETag | `ETag func(V) string` supplied by caller | **Keep the seam; use xxhash for the in-view change detector, HMAC only for the wire validator** | 1.1 us vs 0.1 us at 2 KiB, 46 us vs 6 us at 128 KiB |

### 1. Key to entry store

Requirement: read-mostly `Get`, entries created lazily, lifecycle (timer, queue slot, pin count, waiters) must change atomically with the map.

| Option | Concurrency | ns/op at `-cpu 1 / 4 / 8` | allocs | Notes |
|---|---|---|---|---|
| `Mutex` + `map` (#1312, `lastRead` written under the lock) | one lock, readers serialize | 41 / 123 / 135 | 0 | simplest; lifecycle fields share the lock |
| `RWMutex` + `map` + `atomic.Pointer` | readers share, writers block all | 44 / 66 / 48 | 0 | still takes the lock on the hit |
| `sync.Map` + `atomic.Pointer` | lock-free reads | 47 / 24 / 20 | 0 | `any` boxing on `Store`; no atomic compute+insert |
| `xsync.Map` + `atomic.Pointer` | lock-free reads | 39 / 24 / 16 | 0 | already in `go.mod` (v4.5.0); has `Compute` for atomic create |
| real `View.Get` hit | as row 1 | 54 / 124 / 141 | 0 | includes `Clock.Now()` |

**Decision: keep the mutex.** VERIFIED: contention costs about 100 ns per read at 8 goroutines, 7 to 8x slower than lock-free, and still 0 allocations. INFERRED from the ADR: after the push stream is on, `Get` is called for cold misses, user refetches, and the flag-off fallback, tens per second at most, so the cost is 0.014% of a core at 1,000 reads/s (140 ns x 1,000). A lock-free read path would also not remove the mutex, because `Invalidate`, the pump and `Subscribe` must mutate timer, queue and pin state together with the map. If profiling ever shows contention (`/golang-profiling`, mutex profile; the repo exports Go mutex wait time since #1290), the least invasive change is an `atomic.Pointer` to the current `Versioned` per entry plus `xsync.Map` for lookup, keeping the mutex for writers. That matches the `InstanceSnapshot` convention in `.claude/rules/instance-lock-free-reads.md`. Not needed now.

### 2. Per-key debounce with max-wait

| Option | Event-path cost | Goroutines | Leak risk | Notes |
|---|---|---|---|---|
| `AfterFunc` per key, `Stop`+new timer per event (#1312) | 259 ns, 160 B, 2 allocs | 0 idle; one per firing | none while `Stop` is called on drop/close (it is: `dropLocked`, `Close`) | generation guard makes a stale fire a no-op |
| persistent `Timer.Reset` per key | 24 ns, 0 allocs | 0 idle | `Reset` cannot stop a callback that has already started, so it needs the same generation guard (INFERRED from the `time.Timer.Reset` contract) | 10x cheaper, no real gain at this rate |
| single min-heap + one timer | 9.5 ns for `heap.Fix` only | 1 | one more goroutine and wakeup logic to get wrong | the cost of arming the single timer is not in this number |
| atomic dirty stamp + shared ticker | 0.6 ns | 1 | ticker wakes when idle unless stopped | trades exactness (quiet window rounds up to a tick) for CPU |
| hierarchical timer wheel | O(1) | 1 | a library or ~200 lines | built for 100k+ timers |

**Decision: keep per-key `AfterFunc`.** VERIFIED: 259 ns per event. At the ADR's worst case of 300 events per second that is 78 us per second, 0.008% of a core, and 48 KB/s of short-lived garbage (160 B x 300). A wheel or heap pays off at tens of thousands of timers; here there are at most a few hundred and the Go runtime already keeps them in per-P heaps. Injectable `Clock` is already what makes tests deterministic (`FakeClock.Advance` is O(timers), fine for tests). The max-wait cap is implemented by shrinking the delay to `firstDirty + MaxWait` (`armDebounceLocked`), which is correct and needs no extra structure.

### 3. Priority queue with bounded concurrency

Requirement: order is *waiters, then pinned, then oldest*; at most `maxConcurrentVCSComputations = 4` shared with the request path; cancellable.

| Option | Cost to enqueue n then drain n (`-cpu 1`) | Notes |
|---|---|---|
| `map` scan for best on each dequeue (#1312) | n=8: 456 ns, n=64: 30 us, n=512: 1,133 us | O(n^2) for a full drain; 0 allocs at n=8, 3 from n=64 |
| `container/heap` with `Fix` on priority change | n=8: 237 ns, n=64: 3.5 us, n=512: 41 us | needs `idx` bookkeeping; priority changes (a `Get` bumps a queued key) must call `heap.Fix` |
| 3 bucket FIFOs (waiter / pinned / other) | n=8: 154 ns, n=64: 1.1 us, n=512: 6.6 us | a bump is a move between buckets; oldest-first is free per bucket |
| `x/sync/semaphore.Weighted` | n/a | `TryAcquire`/`Release` would satisfy `Limiter`, but add nothing over the channel semaphore; its blocking `Acquire` is FIFO, so it cannot order the queue |

**Decision: bucketed FIFOs are best; the scan is acceptable.** VERIFIED: per pick the scan costs about 2.2 us at n=512 (1,133 us / 512), against about 13 ns for buckets (enqueue plus drain including slice growth, so not an alloc-free pick; the scan allocates only from n=64 up). A dequeue happens only when a compute finishes, and a git compute takes tens of milliseconds to seconds, so 2 us is noise. The reason to change it anyway is that `pumpLocked` selects the best entry *before* calling `TryAcquire` and runs on every enqueue, so while the four slots are saturated every debounce fire pays a full scan under the global lock for nothing (see [Review](#appendix-b-review-of-1312), delta N2). Keep the `Limiter` interface: `vcsFlights.slots` in `server/services/vcs_status_flight.go` is a `chan struct{}` and adapts to `TryAcquire`/`Release` in a few lines. The 50 ms retry timer is a poll, but a bounded one that only runs while saturated; a release hook on the shared limiter would remove it and is not worth the coupling.

Starvation note (INFERRED from `better`): under sustained pinned load an unpinned stale-while-revalidate refresh can wait behind pinned entries indefinitely. Bound the damage with the existing `FreshTTL`/`StaleTTL` (such an entry is served stale for at most 60 s and then becomes a waiter, which outranks pinned). No aging is needed.

### 4. Eviction with pin counts

| Option | Pin support | Cost per new key at 768 entries | Notes |
|---|---|---|---|
| collect all evictable, `sort.Slice` by `lastRead` when over cap (#1312) | yes (`pins == 0`) | 33 us, 14 allocs at cap (256 unpinned + 512 pinned), under the global lock | the O(n) collect runs on every new-key insert; the sort only when over cap, which is the steady state once the cache is full |
| `container/list` LRU of unpinned entries only | yes (pinned entries leave the list) | 181 ns, 3 allocs | `Remove` on pin, `PushFront` on unpin |
| `hashicorp/golang-lru/v2` v2.0.7 | **no** (emulate with Remove/Add) | n/a | see below |
| CLOCK | yes, with a reference bit | O(1) amortized | more code than `container/list` for no gain at 256 |

**Decision: keep the policy, no library.** VERIFIED API check (`go doc -all github.com/hashicorp/golang-lru/v2 Cache`): the methods are `Add/Get/Peek/Contains/Remove/RemoveOldest/Resize...`; `NewWithEvict`'s callback runs after eviction and cannot veto it, so a pinned entry cannot be protected in place; the workaround (`Remove` on pin, `Add` on unpin) is the `container/list` design plus a dependency. It resolves only as a test-graph transitive module (`go list -m -json`: `v2.0.7`, 2023-09-21, `"Indirect": true`) and has no line in `go.mod` (`grep lru go.mod` is empty), so adopting it adds a new `require`, under MPL-2.0 (`LICENSE` header). At 33 us per insert, 100 new keys per second is 0.33% of a core; the cheap fix is an O(n) min-by-`lastRead` pick when evicting one entry, which drops the sort and allocations (delta N3). Move to `container/list` only if the count of unpinned entries grows past a few thousand.

Accounting for the per-stream cap of 64 and per-process cap of 512 is a counter, not a structure. The structure that matters is what the counter counts (distinct entries or subscriptions); #1312 counts subscriptions, which is the bug in delta M3.

### 5. Change feed with conflation

| Option | Memory per subscriber | Slow-subscriber policy | Resume | Cost |
|---|---|---|---|---|
| per-subscriber `map[K]Update` + 1-slot notify (#1312) | bounded by `MaxKeysPerSub` (64) | latest wins per key, others skipped, no unbounded queue | via ETag compare at the stream layer | `put` 24 ns, 0 allocs; 32-key burst + `take` 2.7 us, 1 alloc (the result slice) |
| slot array + dirty bitmap, reused output buffer | fixed | same | same | 32-key burst + drain 323 ns, 0 allocs, but needs key-to-slot indexing and a 64-bit mask per subscriber |
| append-only log + per-subscriber revision cursor | shared log, grows with churn | must truncate; slow cursor loses entries unless the log is bounded, then needs a resync path | native (`since rev`) | more machinery to bound than the map |
| broadcast to bounded per-subscriber channel | fixed capacity | drop-oldest or block; both lose "latest per key" unless coalesced anyway | no | would reimplement the map |

**Decision: keep the map.** VERIFIED: 2.7 us per 32-key burst with one allocation is invisible at the rates in play (one burst per debounce window per stream), and memory is bounded by construction: `unsafe.Sizeof(Update[string,*T]) = 88` bytes, so 64 pending entries are about 5.6 KB of values plus map overhead. Allocated (not retained) bytes while filling a 64-key subscriber measured about 27 KB (`TestMemoryFootprint`, `TotalAlloc` delta, which includes map-growth garbage, so it is an upper bound on retained size). The bitmap variant is 8x faster and alloc-free, and still not worth its extra indexing code. A revision-cursor log is the right structure when subscribers must *resume without a snapshot*; the ADR resumes with `known_etags` plus a fresh snapshot instead, so the map is sufficient.

Global revision and ETag: `Rev` is a single `uint64` bumped only when the ETag changes (correct under `v.mu`; INFERRED: it is a view-wide counter, so per-key gaps are expected and clients must compare, not count). For the ETag, two jobs are being conflated:

| Job | Needs | Best fit | 2 KiB / 16 KiB / 128 KiB |
|---|---|---|---|
| "did the value change?" inside the view (`publishLocked`) | a fast, non-forgeable-irrelevant digest | `xxhash` (`cespare/xxhash/v2`, already an indirect dependency, `go.mod` line 131) | 101 ns / 752 ns / 6.1 us |
| validator sent to clients (`If-None-Match`) | unguessable per process | existing `conditional.TagMessage` HMAC-SHA256 | 1.1 us / 6.2 us / 46 us, 7 allocs |

Both costs are per *compute* (which already costs tens of milliseconds of git), not per read. The HMAC is acceptable everywhere at these sizes. Pick one: if `View.ETag` is the wire validator, reuse `conditional`; if it is only a change detector, a plain hash is enough. Avoid hashing the same payload twice (once in the view, once in the RPC handler).

### 6. In-flight dedup

`golang.org/x/sync/singleflight` (`go doc`): `DoChan` "will not be closed" channel, takes no `context`, and `Forget` is the only escape hatch. It cannot (a) make a late caller wait for a compute that started *after* its request (the `WaitFresh` and `since` semantics), or (b) cancel the work when the last waiter leaves. `vcsFlights` in `server/services/vcs_status_flight.go` already exists for exactly those two gaps: its `do` takes `notBefore`, refcounts `waiters`, and calls `cancel()` when the last leaves (`leave`).

**Decision: keep the view's own coalescing** (`running`, `rerun`, `waiters`, one pending compute per key), because the view also has to coalesce *events* and *debounce* with the same state. But it must carry over the lifetime rule from #1264, which #1312 does not (delta M1). Use a per-entry `context.WithCancel` and cancel it when `len(waiters) == 0 && pins == 0` at the moment the last waiter abandons.

### 7. Event ingestion from FSEvents/fsnotify bursts

| Concern | Option | Cost | Decision |
|---|---|---|---|
| path to repo key | linear `HasPrefix` over 14 roots | 241 ns, 5 allocs (the allocs are `r+"/"` in the benchmark, avoidable) | no |
| path to repo key | walk parents, `map[string]` lookup | 94 ns, 0 allocs | **yes** |
| dedup to a dirty flag | `map[key]struct{}` set or the entry's own `firstDirty` | O(1) | **yes**, the view's per-key timer already is the dedup |
| noise filter (`*.lock`, `.watchman-cookie*`, `next-index-*`, `.git/objects/`) | string tests | included above | **share the name rules of `classifyGitEvent`** (`session/unfinished/scanner_gitwatch_events.go:183-194`: `.lock`, `next-index-`, `.watchman-cookie`), but it takes only a file name directly under `.git`; the `.git/objects/` and full-path handling must be new, and the 94 ns figure is the benchmark's own filter, not this function |
| overflow detection | ring buffer of events | n/a | **no**: `Event.Flags.NeedsRescan()` (`internal/fsevents/types.go:40`) is the kernel's own overflow signal |
| path set structure | bloom filter / trie | n/a | **no**: 14 roots, not 100k |

VERIFIED: 94 ns per event for lookup plus filter, 0 allocs; at 300 events/s that is 28 us/s. A trie or bloom filter would only matter if the filter had to match thousands of path patterns. The ADR already forbids a per-consumer watcher; the remaining design point is that overflow flags must reach `Invalidate(key, ReasonOverflow)` for every key on that stream, which is a loop over at most the keys of one stream.

### 8. The materialized value

- **Snapshot struct.** `entry.cur` is a `Versioned[V]` copied under the lock; `Get` and subscribers receive that copy. If `V` is a pointer (the proto `*VCSStatus`), the copy shares the pointee between the view, every `Get` caller and every subscriber. That is copy-on-write with structural sharing for free, **provided nobody mutates it**. INFERRED risk: an RPC handler that sets `status_as_of`, `etag` or `not_modified` on the returned proto mutates shared state and races. The package doc should state that `V` must be treated as immutable and handlers must `proto.Clone` (or build a new message) before stamping fields.
- **Size.** VERIFIED `unsafe.Sizeof`: `entry` 264 B, `Versioned[*T]` 64 B, `Update` 88 B, so the structural overhead of 768 entries is about 200 KB. The payload dominates and was not measured here (the generated proto types are not present in a clean checkout, and the commit list cap is the ADR's, not the view's). Payload size is UNVERIFIED.
- **Interning.** Not recommended. INFERRED: the strings in a status are file paths and commit subjects, unique per entry; the only repeated values (branch name, remote) are a handful per entry. Interning adds a global map and lifetime problem to save bytes that were never measured as a problem.

### 9. Stale-tier tracking (15 s fresh, 60 s stale-while-revalidate)

Per-entry `ComputedAt` evaluated lazily in `serveLocked` is the right design: no timers, no wheel, O(1) per read, and `watched()` entries skip the check entirely. A time-bucketed expiry wheel would add background work to avoid a single `time.Sub` per read. **Keep.** The one subtlety, already handled: an unchanged recompute updates `ComputedAt` without bumping `Rev` (`publishLocked`), so `wait_for_fresh` must compare `ComputedAt`, as the ADR says.

### 10. Backstop reconcile scheduling

Not in #1312 (`ReasonBackstop` exists, nothing emits it). Options: per-repo jittered ticker (one `AfterFunc` that re-arms itself with `base + rand jitter`), or one scheduler goroutine with a min-heap of next-due times. **Decision: per-repo self-rearming `AfterFunc`, owned by the change source, not the view.** There are tens of watched repos; Go's runtime already is a timer heap, so a custom heap duplicates it and adds a goroutine whose lifecycle must be managed. The jitter (`hash(repo)` offset plus per-cycle random) is what prevents a thundering herd, not the container. It belongs in the `ChangeSource` so the view stays free of repo-specific policy; the sleep/resume wall-clock-jump check (ADR section 4) is a separate small watcher that calls `Invalidate(k, ReasonResume)` for every key.

## Appendix B: review of #1312

Everything below is against head `24db6edb`. Probes are throwaway tests (`zz_probe_test.go`) added locally and never pushed; each ran with `go test -run Probe -v -count=1 ./internal/matview`. The existing suite also passes under `-race -count=3` and `go vet` (VERIFIED).

### Must-fix

| ID | Finding | Evidence | Fix |
|---|---|---|---|
| M1 | **Compute is not cancelled when the last waiter leaves.** `run` derives its context from `context.Background()` with a 30 s timeout ([view.go:487](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L487)); `abandon` ([view.go:293-316](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L293-L316)) only removes the waiter. #1264's rule (a cancelled caller must not keep burning CPU) is lost for unpinned keys. | `TestProbeWaiterLeavesCancelsCompute`: "compute ctx cancelled after sole waiter left: **false**" | Per-run `context.WithCancel`; in `abandon`, if no waiters remain and `pins == 0`, cancel it. A cancelled run must not publish an error as the entry's value. |
| M2 | **A cold `Get` during the first compute triggers a second compute.** `Get` applies `WaitFresh` ordering (`since` must precede `startSeq`) to a plain miss, so a compute already running for a `Subscribe` does not satisfy it and `requestLocked` sets `rerun` ([view.go:339-346](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L339-L346)). Doubles git work for the common "open N panes" cold start. | `TestProbeColdGetDuringFirstCompute`: "Get still waiting after first compute finished (needs a second compute); calls=2" | Root cause is two things: the waiter's `since`, and `enqueueLocked`/`requestLocked` setting `rerun` whenever the entry is running. For `!e.has && !opt.WaitFresh`, register with `since = 0` **and** skip setting `rerun` (a scratch test of `since = 0` alone still ran 2 computes). The same `rerun` root cause also duplicates computes for `Subscribe` during a running cold `Get`, and for repeated stale-tier `Get`s during a refresh (reviewer scratch tests: 2 and 3 computes); fix `enqueueLocked`, not only `Get`. |
| M3 | **`MaxPinned` counts subscriptions, not entries.** `v.pinned++` runs per (subscriber, key) ([subscribe.go:59-66](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/subscribe.go#L59-L66)). The ADR's subscribe-new-then-release-old reopen overlap therefore doubles the count, and a few tabs can hit 512 with far fewer distinct entries, rejecting keys that should be pinned. | `TestProbePinnedDoubleCount`: `MaxPinned=4`, 3 distinct keys; second overlapping stream rejected=`[b c]` | Increment `v.pinned` only on `e.pins` 0 to 1 and decrement on 1 to 0 (and in `dropLocked`, subtract 1 if `e.pins > 0`). The cap check at subscribe.go:59 must also be skipped for an entry that already has `pins > 0` (peek `v.entries[k]` before `entryLocked`), else a reopen of already-pinned keys is still rejected at the cap. Keep `MaxKeysPerSub` per stream. |

### Should-fix (cheap, low risk)

| ID | Finding | Fix |
|---|---|---|
| S1 | **Events recompute entries nobody watches or waits on.** `markDirtyLocked` schedules a compute for any known key; an unpinned, unread entry kept for up to 10 min is recomputed on every burst. Probe: "computes before events=1 after 5 FS bursts on unpinned, unread entry=6". This contradicts the ADR's "zero git work when nothing changes" for the build-burst case. | For `pins == 0 && len(waiters) == 0`, set a `dirty` flag instead of scheduling. Both `Get` (which would otherwise serve the dirty value as fresh for up to 15 s) and `Subscribe` (whose `e.has` path pushes `e.cur` and makes the entry `watched()`, skipping the TTL check forever) must check the flag and enqueue a recompute. |
| S2 | **`cfg.ETag(val)` runs outside `safeCompute`** ([view.go:499](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L499)). A panic there is unrecovered on a bare goroutine, kills the process, and leaks the limiter slot. | Compute the ETag inside the `recover` wrapper. |
| S3 | `V` mutability contract is undocumented (see section 8). | Package doc line. |

### Nice-to-have

| ID | Finding | Evidence |
|---|---|---|
| N1 | Read path takes the global mutex (41 / 123 / 135 ns at `-cpu 1/4/8`); an `atomic.Pointer` per entry would be 16-24 ns. | Section 1; not worth it at current read rates. |
| N2 | `pumpLocked` scans for `best` before `TryAcquire`, so a saturated pump scans on every enqueue ([view.go:450-470](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L450-L470)). 2.2 us per pick at n=512. Acquire first, then select; or use 3 bucket FIFOs. | Section 3. |
| N3 | `entryLocked` runs `evictOverCapLocked(1)` on every new key: an O(n) collect always, plus a sort once at cap (the steady state): 33 us at 768 entries under the global lock ([view.go:162-177](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L162-L177)). At cap, pick the single oldest by an O(n) scan (or keep a `container/list` LRU of unpinned entries). | Section 4. |
| N4 | `run` starts a helper goroutine per compute only to cancel on `v.done` ([view.go:488-494](https://github.com/tstapler/stapler-squad/blob/24db6edbb9139a933a6ea2c4e5723c2094450e80/internal/matview/view.go#L488-L494)). Derive compute contexts from one view-level base context cancelled in `Close`; this folds into M1's per-run cancel. | Section 6. |
| N5 | `take()` allocates a result slice per wake and map iteration order is random, so keys within a batch arrive unordered. Fine for correctness; sort by `Rev` if clients render in arrival order. | Section 5. |

Also noted, not probed: `Remove`/eviction do not stop an in-flight `run` for the dropped key, so a re-created entry can compute concurrently with the old one (minor duplicate compute).

Kept as is (no delta): per-key `AfterFunc` debounce with a generation guard, the max-wait computation in `armDebounceLocked`, the `Limiter` seam, `FakeClock`, the latest-wins subscriber, lazy tier evaluation, and the waiters > pinned > oldest ordering.

### Not covered by #1312, so not reviewed

Change sources, backstop, `known_etags` resume, the sleep/resume clock-jump check, and the stream layer. They are specified in ADR-034 sections 3 to 5; decisions for them are in sections 7, 9 and 10 above.

## What would change these decisions

- Hundreds of live keys instead of tens, or `GetVCSStatus`-style reads in the tens of thousands per second: revisit section 1 (lock-free read path) and section 4 (`container/list` LRU).
- More than a few thousand pending computes: switch the queue to a heap or buckets (section 3).
- Subscribers that must resume without a snapshot: add the revision-cursor log (section 5).
