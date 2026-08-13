# Go Double-Checked Locking — Return the Locally-Computed Value

In the pattern `read-lock → cache miss → compute → write-lock → conditional store`, always return the locally-computed value, not the cache slot.

**Wrong:**
```go
g.mu.Lock()
if cacheExpired { g.cache = computed }
g.mu.Unlock()
return g.cache, nil  // ← returns what's IN the slot, not what THIS goroutine computed
```

**Right:**
```go
g.mu.Lock()
if cacheExpired { g.cache = computed }
g.mu.Unlock()
return computed, nil  // ← always return locally-computed value
```

## Why

After a lost write race, another goroutine's result is in `g.cache`. Re-reading the slot returns that foreign result, which may contradict the current goroutine's own computation and violates the caller's expectation of a consistent view. The canonical implementation of this pattern is in `session/git/worktree_git.go` (`IsDirty` method).
