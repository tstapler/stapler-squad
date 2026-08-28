# Go Concurrency Patterns

## Double-Checked Locking — Always Return the Locally-Computed Value

In the pattern `read-lock → cache miss → compute → write-lock → conditional store`, always return the locally-computed value, not the cache slot. Re-reading the slot after a lost write race returns another goroutine's observation, which may contradict the current goroutine's computation.

```go
// WRONG: returns g.cache (another goroutine may have stored a different value)
g.mu.Lock()
if cacheExpired { g.cache = computed }
g.mu.Unlock()
return g.cache, nil

// CORRECT: always return locally-computed value
g.mu.Lock()
if cacheExpired { g.cache = computed }
g.mu.Unlock()
return computed, nil
```
