# Go Concurrency Audit — stapler-squad

**Usage**: Audit and fix Go concurrency anti-patterns in the stapler-squad codebase.
**Reference**: `/go:parallelism` for primitive-selection decision tree.

---

## Known Anti-Patterns Fixed in This Codebase

### 1. `map[string]*T + RWMutex` → `xsync.MapOf[string, T]`

Lock-free concurrent maps eliminate contention on high-read-rate hot paths.

**Fixed examples:**
- `session/instance_status.go` — `InstanceStatusManager.controllers` was `map[string]*ClaudeController + RWMutex`; replaced with `xsync.MapOf[string, *ClaudeController]`
- `session/review_queue_poller.go` — `pollerContentProvider.cache` replaced 4 separate `map[string]*` fields with `xsync.MapOf[string, contentCacheEntry]` (comment: "xsync.MapOf replaces 4 map[string]* + RWMutex — lock-free reads across sessions")

**Pattern to look for:**
```go
// BAD
type Foo struct {
    mu    sync.RWMutex
    items map[string]*Bar
}
// GOOD
type Foo struct {
    items *xsync.MapOf[string, *Bar]
}
```

Import: `github.com/puzpuzpuz/xsync/v4`

---

### 2. Atomic Shadow for Lock-Free Debounce

When a field is written infrequently (under lock) but read on a hot path, add an `atomic.Int64` shadow to allow fast-path rejection without acquiring the lock.

**Fixed example:**
- `session/detection/idle.go` — `IdleDetector.lastActivityNs atomic.Int64` shadows `lastActivity time.Time`
  ```go
  // lastActivityNs is a unix-nano shadow of lastActivity for lock-free debounce in RecordActivity.
  // All writers hold mu; this atomic allows RecordActivity to skip the lock on fast-path no-ops.
  lastActivityNs atomic.Int64
  ```
  Used in `RecordActivity()` to skip `mu.Lock()` when the call is a no-op:
  ```go
  nowNs := id.timeNow().UnixNano()
  if nowNs-id.lastActivityNs.Load() < int64(minActivityInterval) {
      return // skip lock, not enough time elapsed
  }
  id.mu.Lock()
  defer id.mu.Unlock()
  id.lastActivityNs.Store(nowNs)
  id.lastActivity = id.timeNow()
  ```

---

### 3. Double-Fetch Reduction — Combine Lock Acquisitions

When two consecutive read-lock acquisitions read the same struct, combine them into one lock-hold to save a round-trip through murmur3 hash + cache lookup.

**Fixed example:**
- `session/claude_controller.go` — `GetStatusAndIdleInfo()` replaces two separate calls (`GetCurrentStatus()` + `GetIdleStateInfo()`):
  ```go
  // Single cache read covers both status and idle entries.
  cc.cache.Read(func(c cacheState) {
      if h == c.status.tailHash { statusHit = true; ... }
      if h == c.idle.tailHash   { idleHit = true; ... }
  })
  ```
  Comment: "Saves one GetRecentHash (murmur3 over 4KB) and one cache.Read on every poll tick compared to calling GetCurrentStatus + GetIdleStateInfo separately."

---

### 4. Double-Checked Locking — Return Locally-Computed Value

See `.claude/docs/concurrency-patterns.md`. Canonical implementation:
- `session/git/worktree_git.go` — `GitWorktree.IsDirtyWithHint()`

Always return the locally-computed value after the write-lock section, not the cache field. Re-reading the cache slot after a lost write race returns a foreign goroutine's result.

---

### 5. Cache-Line Padding for Atomic + Mutex Adjacency

When a `sync.RWMutex` and an `atomic.Pointer` live in the same struct, separate them with `[64]byte` padding to prevent false sharing (Go issue #67764).

**Fixed example:**
- `session/claude_controller.go` — `ClaudeController`:
  ```go
  lifecycle Locked[controllerLifecycle]
  _         [64]byte // cache-line padding: prevents lifecycle.mu invalidating adjacent atomic slots
  ptyAccess atomic.Pointer[PTYAccess]
  ```

---

## Grep Recipes for Finding Remaining Candidates

```bash
# Find map+mutex candidates (potential xsync.MapOf opportunities)
grep -rn "sync\.RWMutex\|deadlock\.RWMutex" --include="*.go" session/ | \
  xargs -I{} sh -c 'file=$(echo "{}" | cut -d: -f1); grep -l "map\[" "$file" 2>/dev/null' | sort -u

# Find structs where lock is defined AFTER the fields it protects (annotation-unfriendly)
grep -rn -A 15 "^type .* struct {" --include="*.go" session/ | \
  grep -B 5 "sync\.RWMutex\|sync\.Mutex" | grep -v "^--$"

# Find high-frequency methods that acquire a lock (candidate for double-fetch reduction)
grep -n "\.mu\.RLock()\|\.mu\.Lock()" session/*.go | awk -F: '{print $1}' | sort | uniq -c | sort -rn | head -10

# Find deadlock.Mutex or deadlock.RWMutex structs (incompatible with checklocks annotations)
grep -rn "deadlock\.RWMutex\|deadlock\.Mutex" --include="*.go" session/ | grep -v "_test.go"

# Find atomic.Int64 / atomic.Int32 candidates (fields that could shadow a mutex-protected field)
grep -rn "atomic\.Int64\|atomic\.Int32\|atomic\.Uint64" --include="*.go" session/ | grep -v "_test.go"
```

---

## Already-Fixed Locations (Skip in Future Audits)

| File | What was fixed |
|---|---|
| `session/instance_status.go` | `InstanceStatusManager` — map+RWMutex → xsync.MapOf |
| `session/review_queue_poller.go` | `pollerContentProvider` — 4 maps+RWMutex → xsync.MapOf |
| `session/detection/idle.go` | `IdleDetector.lastActivityNs` — atomic shadow for lock-free debounce |
| `session/claude_controller.go` | `GetStatusAndIdleInfo()` — combined two lock acquisitions into one |
| `session/claude_controller.go` | Cache-line padding between `lifecycle.mu` and `ptyAccess atomic.Pointer` |
| `session/git/worktree_git.go` | `IsDirtyWithHint()` — correct double-checked locking (return locally-computed) |
| `session/git/worktree.go` | `GitWorktree.isDirtyCache*` — `// +checklocks:isDirtyCacheMu` annotations |
| `session/locked.go` | `Locked[T].val` — `// +checklocks:mu` annotation |
| `session/circular_buffer.go` | `CircularBuffer` mutable fields — `// +checklocks:mu` annotations |
| `session/detection/approval.go` | `ApprovalDetector` fields — `// +checklocks:mu` annotations |
| `session/scrollback/buffer.go` | `CircularBuffer` mutable fields — `// +checklocks:mutex` annotations |

---

## checklocks Annotation Guide

This codebase uses `gvisor.dev/gvisor/tools/checklocks` to enforce mutex discipline.

**Annotation syntax** — inline field comment, must be the only content after `//`:
```go
type Foo struct {
    mu   sync.RWMutex
    bar  string // +checklocks:mu
    baz  int    // +checklocks:mu
}
```

**Constraints:**
- Only works with `sync.Mutex` and `sync.RWMutex`. Does NOT work with `deadlock.Mutex` / `deadlock.RWMutex` (false positives).
- `maxSize`/`size` style immutable fields should NOT be annotated — they're set in constructors and never mutated.

**Run the check:**
```bash
make checklocks
# or directly:
checklocks -inferred=false -atomic=false ./session/git/... ./session/detection/...
```

The `-inferred=false` flag only reports violations of explicit annotations (not suggestions), giving a clean CI gate while allowing progressive adoption.

---

## PR Review Checklist for Lock-Touching Changes

- [ ] New map fields guarded by a mutex → consider `xsync.MapOf` instead
- [ ] New struct fields alongside an existing `sync.RWMutex` → add `// +checklocks:mu` annotation; run `make checklocks`
- [ ] Any new `deadlock.Mutex`/`deadlock.RWMutex` — ensure these don't appear in packages covered by `make checklocks` (false positives)
- [ ] Double-checked locking: return locally-computed value, not cache slot (see rule file)
- [ ] Hot-path function acquires a lock on every call → check if an atomic shadow can enable a fast-path no-op
- [ ] Mutex and atomic.Pointer in same struct → `[64]byte` padding between them if the struct is on a hot read path
- [ ] New `// Callers MUST hold X` comment → this is a candidate for a checklocks function annotation (`// +checklocks:mu` on the func)
