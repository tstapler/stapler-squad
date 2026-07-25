# Pitfalls & Risks: perf-mutex-hotspots-2026-07

Date: 2026-07-01
Researcher: Claude Code (Sonnet 4.6)

---

## Codebase State Before Refactoring

Before addressing singleflight/TTL-cache pitfalls, note that **two of the three stated hotspots are already implemented** in the current codebase:

| Hotspot | Status | File |
|---|---|---|
| CircularBuffer → sync.RWMutex | **Already done** | `session/circular_buffer.go:20` uses `sync.RWMutex` |
| GitWorktree.IsDirty → 5s TTL cache | **Already done** (15s TTL) | `session/git/worktree_git.go` — `IsDirtyWithHint` has full RLock fast path + Write-lock slow path + thundering-herd guard |
| GoGitVCSReader AheadBehind/DiffShortstat/HasUncommitted → singleflight | **Not yet done** | `session/unfinished/gogit_vcs_reader.go` uses TTL caches via `sync.Map` but no singleflight |

The requirements are therefore partially pre-implemented. The remaining work is narrowly scoped to adding `singleflight.Group` to `GoGitVCSReader`.

---

## 1. singleflight Pitfalls

### 1.1 Panic propagation — **critical**

`singleflight.Do` catches panics inside the user function, converts them to a `PanicError` value, and re-panics in **all waiting goroutines**, not just the one that panicked. A single `nil` pointer dereference in a go-git operation will cascade to every goroutine waiting on the same key.

**Defense:** Wrap the `Do` body with a `recover` that converts panics to errors before returning:

```go
g.sfGroup.Do(cacheKey, func() (interface{}, error) {
    defer func() {
        if r := recover(); r != nil {
            // convert to error so the panic doesn't rebroadcast
        }
    }()
    return g.aheadBehindUncached(worktreePath, base)
})
```

The `golang.org/x/sync/singleflight` package (v0.20.0 in this repo) has not changed this behaviour in Go 1.25.

### 1.2 Negative cache / error sharing

`singleflight.Do` shares both results **and errors**. If the first caller gets a transient error (e.g. `go-git` packfile reader temporarily busy), all waiting callers receive the same error. The TTL on the result cache does not apply to errors, so they are not cached at the `sync.Map` layer — but every in-flight waiter is still poisoned in that round.

**Defence:** Do not store error results in the TTL cache (current code does this correctly already: `DiffShortstat` only calls `diffStatCache.Store` when `err == nil`). For singleflight, the same logic applies: only cache on success.

### 1.3 Key collision between logical operations

`GoGitVCSReader` uses separate `sync.Map` caches for `AheadBehind` and `CommitMessages` keyed by `worktreePath + "\x00" + base`. If a single `singleflight.Group` is shared across all three methods (`AheadBehind`, `DiffShortstat`, `HasUncommitted`), a key clash is possible: `DiffShortstat` uses only `worktreePath` as key, while `AheadBehind` uses `worktreePath + "\x00" + base`. These are distinct but a mis-scoped group could conflate them.

**Defence:** Use either:
- Separate `singleflight.Group` fields per method, or
- A single group with a method-type prefix in the key (e.g. `"ab:" + worktreePath + "\x00" + base`).

A single shared group with prefixed keys is simpler and avoids struct bloat.

### 1.4 Context cancellation via DoChan

`DoChan` returns a channel, so the caller can select against a context cancellation. However, the underlying work **continues running** even if the caller context is cancelled — singleflight does not propagate cancellation to the in-flight function. For VCS operations that take 50–500 ms, this means cancelled HTTP requests (e.g. a session status poll that the user navigated away from) still consume CPU and hold `entry.mu`.

**Defence:** For this codebase the scanner uses polling goroutines without per-call contexts, so DoChan is not needed. Use `Do` (synchronous) for all three methods. If context propagation is needed in future, consider `errgroup.Group` instead.

### 1.5 Forget for cache invalidation

`singleflight.Group.Forget(key)` removes a key from the in-flight map so the next `Do` call starts fresh work. This is needed if a result is known to be stale before the TTL expires (e.g. `InvalidateDirtyCache` equivalent for the VCS reader). In the current codebase `GoGitVCSReader` has no explicit invalidation path — the TTL is the only eviction mechanism — so `Forget` is not currently required. However, if a future feature calls the reader immediately after a commit lands (e.g. post-push status refresh), `Forget` will be needed to prevent serving a stale singleflight result.

---

## 2. sync.RWMutex Upgrade Pitfalls

> Note: `CircularBuffer` already uses `sync.RWMutex`. This section documents pitfalls to verify the existing implementation avoids and to guide any future RWMutex additions.

### 2.1 Write starvation under heavy read load

`sync.RWMutex` in Go uses a reader-preferring strategy: new readers can acquire RLock even while a writer is waiting, preventing the writer from ever proceeding if the read rate is high enough. In the `CircularBuffer` case, `Write()` holds the full write lock while `GetRecent()` / `GetAll()` hold read locks. If streaming output causes continuous reads, a background `Write()` call could starve.

**Current status:** The existing implementation correctly uses `Lock()` for `Write()` / `Clear()` / disk operations and `RLock()` for `GetRecent()` / `GetAll()` / `Len()` / `TotalBytesWritten()` / `WriteTo()`. The write path (PTY output) is a single producer, so starvation risk is low in practice.

### 2.2 Incorrect RLock/RUnlock pairing

`RUnlock` on a mutex that was not RLocked causes a fatal panic at runtime (`sync: RUnlock of unlocked RWMutex`). Unlike `defer mu.Unlock()` where the symmetric pair is obvious, conditional early-return code can mis-pair unlock calls.

**Observed risk in codebase:** `gogit_vcs_reader.go` uses non-deferred `entry.mu.Lock()` / `entry.mu.Unlock()` pairs with multiple early-return branches (e.g. `HasUncommitted` lines 281–354). Each error path manually calls `entry.mu.Unlock()` before returning. If singleflight is added and the Do body wraps these methods, the unlock calls must be audited to ensure no double-unlock or missing-unlock across the singleflight boundary.

### 2.3 defer ordering with multiple defers

When `defer mu.RUnlock()` is used alongside other deferred calls, execution order is LIFO. If a deferred `iter.Close()` calls back into code that tries to acquire the same mutex, a deadlock results. In `gogit_vcs_reader.go`, `iter.Close()` is called explicitly (not deferred) before `entry.mu.Unlock()` to avoid this — correct practice that must be preserved if code is refactored.

### 2.4 copylock vet check

`sync.RWMutex` must not be copied after first use. `go vet` catches this via the `copylocks` analyser. `GitWorktree` embeds `sync.RWMutex` as a value field (`isDirtyCacheMu sync.RWMutex`). If `GitWorktree` is ever assigned by value (not pointer), the vet check will fire. Current code uses `*GitWorktree` throughout — this is correct and must be preserved.

`GoGitVCSReader` contains `sync.Map` fields which have the same copylock constraint. Both structs are passed by pointer everywhere in the current codebase.

### 2.5 RLock overhead under no-contention

`sync.RWMutex.RLock()` is more expensive than `sync.Mutex.Lock()` when there is **no contention**, because it must do an atomic add to the reader count. The benefit (multiple concurrent readers) only materialises when the critical section is long enough to justify the overhead. For `CircularBuffer.Len()` (returns a single int), the critical section is ~1 ns; the RLock overhead (~4 ns on M-series Apple Silicon) is 4× the work. However, this is only relevant at extremely high call rates — the upgrade is still correct given that `GetRecent` / `GetAll` / `WriteTo` have meaningful critical sections.

---

## 3. TTL Cache Pitfalls

> Note: `IsDirty` in `session/git/worktree_git.go` already implements the pattern correctly with a 15s TTL. The pitfalls below document what the existing code gets right and what to watch for in new TTL caches.

### 3.1 Thundering herd on expiry — the core risk

A plain TTL cache without singleflight does **not** solve the thundering herd: all goroutines that observe an expired entry simultaneously proceed to recompute, each acquiring `entry.mu`, serialising the recomputation but wasting N−1 subprocess invocations. The existing `IsDirtyWithHint` avoids this by running git subprocess *outside* the lock and then storing under a write lock with a conditional check (so only the first writer stores, subsequent writers discard their result). This is correct for IsDirty because the subprocess result is cheap and idempotent.

For `GoGitVCSReader` methods, the computation holds `entry.mu` for the duration (go-git packfile reader is not goroutine-safe), so all callers queue up anyway — the thundering herd becomes a serialised stampede. singleflight eliminates redundant work entirely.

### 3.2 time.Now() overhead in hot paths

`time.Now()` on Linux performs a `clock_gettime(CLOCK_REALTIME)` vDSO call (~15–25 ns). In `DiffShortstat`, `AheadBehind`, and `CommitMessages`, `time.Now()` is called once per cache check and once per cache store. At the scanner's 30s cycle rate with ~4 workers × N repos, this is negligible. However, if the scanner rate were to increase or the methods called from a tight loop, calling `time.Now()` twice per cache hit (check + store) could accumulate. The current code calls it once on the hot path (`time.Now().Before(e.expiry)`) — acceptable.

### 3.3 Cache key heap escape

In `gogit_vcs_reader.go`, the `aheadBehindCache` key is built with string concatenation: `worktreePath + "\x00" + base`. This allocates a new string on every call. At high call rates (scanner polling every 30s across 100 repos), this is ~100 allocations/30s — negligible. However, if singleflight is added, the same key string is passed to both the TTL cache check and the singleflight `Do` — one allocation can serve both by storing the key in a local variable.

### 3.4 sync.Map delete vs lazy expiry cleanup

The current `gogit_vcs_reader.go` TTL caches use `sync.Map.Store` to overwrite expired entries rather than deleting them. Expired entries are re-checked on next access and overwritten if stale. This is a lazy-expiry approach: memory is not freed until the key is accessed again. For per-worktree-path caches with bounded key sets (one per active session), this is acceptable. The `repoCache` has an explicit eviction mechanism (`pruneRepoCache`), but the `diffStatCache`, `aheadBehindCache`, and `commitMessagesCache` do not — they rely on the bounded key space to prevent unbounded growth.

**Risk:** If sessions are deleted and their worktree paths are removed, stale entries persist in the caches until the GC reclaims them (strings are not pinned). This is fine for correctness but means dead sessions contribute to `sync.Map` memory for up to 30 minutes. No action required unless memory pressure is observed.

---

## 4. Go 1.25 Specific

### 4.1 singleflight — no behaviour changes in Go 1.25

`golang.org/x/sync v0.20.0` (used in this repo) is not a standard library package; it tracks its own release schedule independently of the Go toolchain. There are no announced behaviour changes to `singleflight` in Go 1.25. The panic-propagation behaviour (`PanicError` + `PanicValue`) has been stable since Go 1.13. `DoChan` channel semantics are unchanged.

### 4.2 sync.RWMutex — no behaviour changes in Go 1.25

`sync.RWMutex` internals were last significantly changed in Go 1.19 (starvation prevention). Go 1.25 release notes do not document changes to `sync.RWMutex`. The copylock vet check behaviour is unchanged.

### 4.3 sync.Map — minor allocation improvements in recent Go versions

Go 1.20 and later include incremental improvements to `sync.Map` internal implementation (reduced hot-path allocations for the `Load` fast path). Go 1.25 has no announced breaking changes to `sync.Map`. The `LoadOrStore` + `Range` + `Delete` APIs used in `gogit_vcs_reader.go` behave identically.

### 4.4 go-git packfile reader goroutine safety — unchanged

The `go-git/go-git/v5` library's packfile reader is not goroutine-safe. This is a documented constraint of the library (not a Go runtime concern) and has not changed. The per-repo `cachedRepo.mu sync.Mutex` must remain in place **inside** any singleflight `Do` body — singleflight serialises callers with the same key but different keys (different repos) run concurrently and still require per-repo locking.

---

## Summary Risk Matrix

| Risk | Severity | Likelihood | Mitigation |
|---|---|---|---|
| Panic in singleflight Do rebroadcasts to all waiters | HIGH | LOW (go-git panics are rare) | `recover` inside Do body |
| Single singleflight group key collision | MEDIUM | MEDIUM (if implemented naively) | Prefix keys by method name |
| Double-unlock if refactoring non-deferred unlock chains | HIGH | MEDIUM | Audit all `entry.mu.Unlock()` call sites before adding singleflight |
| Stale `IsDirty` cache after session commit (already exists) | MEDIUM | MEDIUM | `InvalidateDirtyCache()` already called in `PushChanges` / `CommitChanges` |
| sync.Map dead-entry leak for deleted sessions | LOW | HIGH (always happens) | Acceptable; bounded by session count |
| RWMutex write starvation in CircularBuffer | LOW | LOW (single writer) | No action needed |
