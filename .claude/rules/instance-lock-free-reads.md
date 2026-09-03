---
globs:
  - "session/instance*.go"
---

# Read Instance Fields via Snapshot(), Not the Raw Field

Mutable `*Instance` fields (`i.Path`, `i.Branch`, etc.) are written under `i.mu.Lock()` by the actor setters in `session/instance_actor_setters.go` and republished to `i.snapshot` (an `atomic.Pointer[InstanceSnapshot]`) on every mutation. Reading the raw field directly, without going through `Snapshot()` or an `i.mu.RLock()`-guarded accessor, races with those writes.

**Wrong:**
```go
func (i *Instance) GetEffectiveRootDir() string {
	if i.gitManager.HasWorktree() {
		if p := i.gitManager.GetWorktreePath(); p != "" {
			return p
		}
	}
	return i.Path // unguarded read races with setGitHubResolutionLocked's i.mu.Lock() write
}
```

**Right:**
```go
func (i *Instance) GetEffectiveRootDir() string {
	if i.gitManager.HasWorktree() {
		if p := i.gitManager.GetWorktreePath(); p != "" {
			return p
		}
	}
	return i.GetPath()
}

// GetPath returns the instance's repository root path via the lock-free
// published Snapshot() rather than the bare i.Path field.
func (i *Instance) GetPath() string {
	return i.Snapshot().Path
}
```

If the field isn't in `InstanceSnapshot` (`session/instance_snapshot.go`) yet, add it there first (it's the single authoritative field list per that file's header comment) rather than reaching for an ad hoc `i.mu.RLock()`. A raw `i.mu.RLock()`-guarded read is only appropriate for state genuinely excluded from the snapshot (manager/dependency objects — see the exclusion list in `instance_snapshot.go`'s doc comment).

## Why

`session/instance_actor_setters.go`'s `setGitHubResolutionLocked` (and its siblings) mutate fields like `i.Path` under `i.mu.Lock()` from a background goroutine — e.g. deferred GitHub URL resolution completing after `CreateSession` has already returned. `GetEffectiveRootDir`, `GetWorkingDirectory`, and `Workspace` in `session/instance_worktree.go` read `i.Path` directly with no lock, which `go test -race` caught as a genuine data race: `TestCreateSession_GitHubURLResolution_NotBoundByRequestContext`'s background resolution goroutine wrote `i.Path` concurrently with a parallel test's cleanup path reading it via `GetEffectiveRootDir`, failing CI (`server/services` package, MCP Integration Tests workflow).

The project already has the fix built in: every mutator publishes an updated `InstanceSnapshot` via `i.snapshot.Store(...)` (see `session/instance_snapshot.go`'s "IAC Epic 2" lock-free-reader conversion), so `Snapshot()` gives race-free reads with no lock contention at all — cheaper than `i.mu.RLock()`, not just safer. Bypassing it and touching the raw field reintroduces the exact class of bug that conversion existed to eliminate.
