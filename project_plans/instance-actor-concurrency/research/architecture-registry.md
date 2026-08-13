# Architecture: `Registry` + `LiveInstance` (extends `architecture.md`)

Research/design only. No application files modified. Extends `research/architecture.md` (actor/
command/snapshot design, still valid) with the `Registry`/`LiveInstance` split from `ADR-031` and
`requirements.md` R2.11-R2.17, replacing superseded R2.10 and closing `adversarial-review.md`'s
two blockers (§2 partial `Stop()`-after-use; §3 `daemon.go`'s duplicate-actor hazard and
`loadInstancesWithWiring`'s hot-path leak).

Grounded in: `server/dependencies.go` (130-945), `session/review_queue_poller.go` (152-191,
848-883), `session/storage.go` (17-137, 271-289), `session/instance_terminal.go` (33-45),
`server/services/session_service.go` (335-364, 427-436, 1618-1710, 1819-1834),
`server/services/workspace_service.go` (27-86), `server/mcp/tools_lifecycle.go` (320-399),
`server/services/terminal_websocket.go` (39-70), `session/health.go` (46-90),
`session/hibernation_sweeper.go` (201-256), `daemon/daemon.go` (22-126, 288-330),
`decisions/ADR-029-actor-shutdown-context-cancelfunc.md`, `server/server.go` (51,173,387-400,711).

---

## 1. `Registry` type design

```go
// session/registry.go (new file)

// Registry owns the sessionID -> live actor mapping + refcounts. mu guards map
// membership only — NOT the per-field stateMutex this migration eliminates.
type Registry struct {
    storage *Storage // read-only: FindInstanceDataByID / ListInstanceIDs only
    mu      sync.Mutex
    entries map[string]*registryEntry
}

type registryEntry struct {
    instance *LiveInstance
    refcount int
}

func NewRegistry(storage *Storage) *Registry {
    return &Registry{storage: storage, entries: make(map[string]*registryEntry)}
}
```

**`sync.Mutex`, not `RWMutex`.** The reader-contention argument motivating this migration
(`RWMutex.RLock()`'s shared atomic increment contending across cores) applies to high-frequency,
read-dominant per-field access — exactly what `stateMutex` was. This lock guards a map lookup +
int increment held for microseconds and is *not* read-dominant: every `Acquire`/`release()` is a
write, so there's no read-only population to justify `RWMutex`'s split-lock overhead (per
`go-concurrency`: `RWMutex` only earns its keep when reads dominate writes by a wide margin and
hold time is non-trivial — neither holds).

**`Acquire`/`release()`** — three outcomes (not-in-storage error / construct-and-spawn /
refcount++):

```go
// Acquire returns the live handle for sessionID, constructing+spawning its actor on
// first access. release() must be called exactly once; teardown happens at refcount
// zero (R2.14).
func (r *Registry) Acquire(sessionID string) (*LiveInstance, func(), error) {
    r.mu.Lock()
    if e, ok := r.entries[sessionID]; ok {
        e.refcount++
        r.mu.Unlock()
        return e.instance, r.releaseFunc(sessionID), nil
    }
    r.mu.Unlock() // release before I/O — construction must not block unrelated Acquires

    data, err := r.storage.FindInstanceDataByID(sessionID)
    if err != nil { return nil, nil, fmt.Errorf("registry: acquire %q: %w", sessionID, err) }
    if data == nil { return nil, nil, ErrSessionNotFound } // outcome 1: not in storage
    live, err := newLiveInstance(data, r.storage) // outcome 2: construct + spawn actor
    if err != nil { return nil, nil, fmt.Errorf("registry: construct %q: %w", sessionID, err) }

    // Re-check under lock: a concurrent Acquire may have won the race already.
    // Never let two live actors exist for one ID — adopt the winner's, discard ours.
    r.mu.Lock()
    defer r.mu.Unlock()
    if e, ok := r.entries[sessionID]; ok {
        live.stopActor()
        e.refcount++
        return e.instance, r.releaseFunc(sessionID), nil
    }
    r.entries[sessionID] = &registryEntry{instance: live, refcount: 1}
    return live, r.releaseFunc(sessionID), nil
}

func (r *Registry) releaseFunc(id string) func() { return func() { r.release(id) } }

func (r *Registry) release(sessionID string) {
    r.mu.Lock()
    e, ok := r.entries[sessionID]
    if !ok { r.mu.Unlock(); log.Warn("registry: release for unknown sessionID", "id", sessionID); return }
    e.refcount--
    if e.refcount > 0 { r.mu.Unlock(); return }
    delete(r.entries, sessionID)
    r.mu.Unlock()
    e.instance.stopActor() // outside the lock — may do teardown I/O
}
```

The double-checked re-lock makes `daemon.go`'s duplicate-actor hazard (`adversarial-review.md`
§3b) structurally impossible even under concurrent `Acquire` for the same ID: whichever
construction finishes second discovers an existing entry and discards its own actor via
`stopActor()` instead of registering a second one. `ErrSessionNotFound` is a sentinel so callers
`errors.Is` it to distinguish "doesn't exist in storage" from "exists, no live actor yet."

---

## 2. Where `Registry` is constructed and injected

**Recommendation: `BuildServiceDeps`** (`server/dependencies.go:322-353`), not `BuildCoreDeps` or
`BuildRuntimeDeps`. `Registry`'s only dependency is `*Storage`, available at end of Phase 1 — but
`BuildServiceDeps`'s documented scope (244-250), "management components that depend on CoreDeps,"
is where `StatusManager`/`ReviewQueuePoller`/`PRStatusPoller` already live for this reason;
`Registry` is one more such manager. `BuildRuntimeDeps` (424-945) is where `LoadInstances()` runs
today (438) and consumers get wired (471-484) — too late; `Registry` must exist *before* Phase 3
rewrites Step 5. `BuildCoreDeps`'s membership is identity/data/event plumbing (`SessionService`,
`Storage`, `EventBus`, `ReviewQueue`, `ApprovalStore`, `ErrorRegistry`), not lifecycle managers.

```go
type ServiceDeps struct {
    *CoreDeps
    StatusManager     *session.InstanceStatusManager
    ReviewQueuePoller *session.ReviewQueuePoller
    PRStatusPoller    *session.PRStatusPoller
    Registry          *session.Registry // NEW
}

func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) {
    // ... existing statusManager/reviewQueuePoller/prStatusPoller construction unchanged ...
    registry := session.NewRegistry(core.Storage)
    warren.Set(w, "Registry", core.SessionService.SetRegistry, registry) // NEW, alongside existing Set() calls
    // ... w.Validate(), return &ServiceDeps{..., Registry: registry}, unchanged shape otherwise ...
}
```

`RuntimeDeps` embeds `*ServiceDeps` (358), so `rt.Registry` is already available with no new
field; add `Registry: rt.Registry` to `ToServerDeps()` (90-123) so `NewServerWithDeps` can wire it
into `WorkspaceService.SetLiveFinder`-style calls and `daemon.Daemon`'s constructor — existing
`warren.Set` convention, no new DI idiom (ADR-031's "no new architectural idiom" consequence).

**`BuildRuntimeDeps` Step 5 rewrite (sketch):** startup still enumerates every persisted session
once, via `Acquire` instead of raw `FromInstanceData`, so the first `LiveInstance` per session is
created through the same path every other caller uses:

```go
dataList, err := storage.LoadInstanceData() // metadata only, no Start() side effect
instances := make([]*session.LiveInstance, 0, len(dataList))
releases := make([]func(), 0, len(dataList))
for _, data := range dataList {
    live, release, err := svc.Registry.Acquire(data.GetStableID())
    if err != nil { log.Warn("startup: acquire failed", "session", data.Title, "err", err); continue }
    instances = append(instances, live)
    releases = append(releases, release) // held for process lifetime, see §6
}
```

---

## 3. Relationship to `ReviewQueuePoller`'s `SetInstances`/`AddInstance`/`GetInstances`

**Recommendation: coexist — `ReviewQueuePoller` becomes a long-lived `Registry` consumer, not a
competing construction path.** It never constructs an `*Instance` today (152-167 only accept
already-built instances) — it's a subscriber list with its own bookkeeping, not a second smart
constructor. Route its 3 *mutating* entry points through `Registry`, leaving read-only ones alone:

```go
type ReviewQueuePoller struct {
    // ... unchanged fields ...
    registry *Registry
    releases map[string]func() // mirrors rqp.instances membership
}

// AddInstance now acquires by ID (externalDiscovery.OnSessionAdded's call-site change, dependencies.go:633-647).
func (rqp *ReviewQueuePoller) AddInstance(sessionID string) error {
    live, release, err := rqp.registry.Acquire(sessionID)
    if err != nil { return err }
    rqp.mu.Lock()
    defer rqp.mu.Unlock()
    live.GitManager().PrimeDirtyCacheJitter() // unchanged behavior, line 165 today
    rqp.instances = append(rqp.instances, live)
    rqp.releases[sessionID] = release
    return nil
}

// RemoveInstance additionally releases — tears down the actor once refcount hits zero.
func (rqp *ReviewQueuePoller) RemoveInstance(instanceTitle string) {
    rqp.mu.Lock()
    // ... existing filter loop unchanged ...
    if release, ok := rqp.releases[removedTitle]; ok {
        delete(rqp.releases, removedTitle)
        defer release() // outside rqp.mu
    }
    rqp.mu.Unlock()
    rqp.contentProvider.EvictInstance(evictKey)
}
```

`GetInstances()`/`FindInstance()` (877-883, 848-860) stay unchanged — already borrowed references,
same pattern as `FindLiveInstance` (`session_service.go:431`).

**Migration plan (no big-bang rewrite):** (1) change only the 3 mutating entry points and their
call sites in `dependencies.go` (438, 483, 633-659); (2) read-only consumers — `WatchSessions`
(`session_service.go:1874`), `ListSessions`/`GetSession` (887, 1000), `annotateUserPRCache`
(`dependencies.go:953`) — need zero changes beyond the mechanical rename the migration already
forces; (3) closes §3b's `daemon.go:292` finding without a bespoke fix beyond §5.7 (dedup returns
the existing actor on a double-call, never a second one); (4) closes §3a's
`loadInstancesWithWiring` finding (335-364, ~10 sites when `s.reviewQueuePoller` is nil) by
converting its fallback to a per-ID `Acquire` (Group B, R2.16) — no siblings to leak;
`ListSessions`/`GetSession` use `RegistryInspector.List()` (§4) instead.

---

## 4. Interface for testability (R2.17)

**Minimal interface most consumers need — one method:**
```go
// InstanceAcquirer lets tests fake Acquire without a real tmux/git backend.
type InstanceAcquirer interface { Acquire(sessionID string) (*LiveInstance, func(), error) }
```

**Separate interface for enumeration/admin callers** (ISP — a single-handle caller should never
be handed `List`/`Count`):
```go
// RegistryInspector: read-only enumeration for admin/debug/bulk-listing callers
// (ListSessions/WatchSessions's initial snapshot). Instances are borrowed — callers
// must not hold them past the call without a real Acquire.
type RegistryInspector interface {
    List() []*LiveInstance
    Count() int
}
```

`*Registry` satisfies both; nothing forces combining them, matching `WorkspaceService`'s existing
1-method `LiveInstanceFinder` (`workspace_service.go:32-34`) and `StatusProvider`/`ContentProvider`
(`review_queue_poller.go:19,26`).

**`TryAcquire`? Not needed.** Every cataloged call site either already blocks on `LoadInstances()`
today or is a background poller iterating synchronously; `FindLiveInstance` already covers "tell
me if it's live, else nil" via `nil`-returning lookup. A non-blocking variant has no consumer.

---

## 5. Call-site mapping: `Acquire`/`release()` vs `InstanceData`

### 5.1 `workspace_service.go:67-86` (`findInstanceFast`) — Group B (before: whole-list scan, siblings discarded)

```go
func (ws *WorkspaceService) findInstanceFast(id string) (*session.LiveInstance, func(), error) {
    live, release, err := ws.registry.Acquire(id) // ws.registry: session.InstanceAcquirer
    if errors.Is(err, session.ErrSessionNotFound) {
        return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", id))
    }
    if err != nil { return nil, nil, connect.NewError(connect.CodeInternal, err) }
    return live, release, nil // callers (SwitchWorkspace, GetVCSStatus) defer release()
}
```

### 5.2 `session_service.go:1618-1673` (`HibernateSession` RPC) — Group B

Before: `LoadInstances()`→scan→`Hibernate()`→`instances[idx]=instance; SaveInstances(instances)`
(re-saves the whole list; siblings never `Stop()`'d — §2's blocker).
```go
live, release, err := s.registry.Acquire(req.Msg.Id)
if errors.Is(err, session.ErrSessionNotFound) { return nil, connect.NewError(connect.CodeNotFound, ...) }
if err != nil { return nil, connect.NewError(connect.CodeInternal, err) }
defer release()

s.stopAndDeregisterDriver(live.Title)
live.SetHibernateReason(orDefault(req.Msg.Reason, "manual"))
if err := live.Hibernate(ctx); err != nil { return nil, connect.NewError(connect.CodeFailedPrecondition, err) }
s.removeFromAllPollers(live.Title)
// No whole-list SaveInstances — single-instance save becomes storage.SaveInstance(live.ToInstanceData()).
s.eventBus.Publish(events.NewSessionUpdatedEvent(live, []string{"status"}))
```
Closes §2's blocker at the root: `Acquire` only touches the requested ID, so there's no "load N,
keep 1" shape to leak siblings from.

### 5.3 `tools_lifecycle.go:320-354` (`stop_session`) and `:374-399` (`updateSession`) — Group B

Both today: whole-list scan, mutate one, discard siblings unstopped. `updateSession` also has a
two-branch `FindLiveInstance`-then-`LoadInstances`-fallback shape `Acquire` collapses into one call.

`stop_session`:
```go
live, release, err := lh.registry.Acquire(sessionID)
if err != nil { /* not-found/internal mapping */ }
defer release() // fires after Destroy() below (LIFO) — see §6's ordering discussion
if live.Status() != session.Paused && !live.Started() {
    _ = live.Start(false) // best-effort hydration, log-and-continue on error
}
if err := live.Destroy(); err != nil { log.Warn("mcp destroy had errors", "session", sessionID, "err", err) }
lh.svc.RemoveFromAllPollers(live.Title)
lh.store.DeleteInstance(live.Title)
```
`updateSession` (same `Acquire`/`defer release()` preamble, then):
```go
live.Title = newTitle       // field mutation until architecture.md's actor migration lands —
live.SetTags(newTags)       // Registry and the actor/command migration are independent axes.
live.Category = newCategory // Single-instance save, not a full-list re-save.
```

### 5.4 `terminal_websocket.go:39-70` (`HandleWebSocket`) — handed to a long-lived stream (before:
whole-list scan, `instance` held for the connection's lifetime, siblings never released)
```go
sessionID := r.URL.Query().Get("session_id")
live, release, err := h.registry.Acquire(sessionID)
if errors.Is(err, session.ErrSessionNotFound) { http.Error(w, "Session not found", http.StatusNotFound); return }
if err != nil { http.Error(w, "Failed to acquire session", http.StatusInternalServerError); return }
defer release() // fires when the handler returns, i.e. when the WS connection closes
conn, err := upgrader.Upgrade(w, r, nil)
// ... existing streaming loop, using `live` instead of `instance` ...
```

### 5.5 / 5.6 `health.go:46-90` and `hibernation_sweeper.go:201-256` — full, ticker-driven
sweeps, NOT single-lookup sites (§2's second sub-finding; before: whole-list load, nothing ever
released, unbounded over process uptime). `Registry` gains one batch helper (sugar on the concrete
type, not part of §4's minimal interfaces) so every sweep caller gets symmetric release for free:
```go
// AcquireAll acquires every session known to Storage; returns one release func for all.
func (r *Registry) AcquireAll() ([]*LiveInstance, func(), error) {
    ids, err := r.storage.ListInstanceIDs() // metadata-only enumeration
    if err != nil { return nil, nil, err }
    var live []*LiveInstance
    var releases []func()
    for _, id := range ids {
        l, release, err := r.Acquire(id)
        if err != nil { log.Warn("AcquireAll: skipping", "id", id, "err", err); continue }
        live = append(live, l)
        releases = append(releases, release)
    }
    return live, func() { for _, release := range releases { release() } }, nil
}
```
`health.go`: `live, releaseAll, err := h.registry.AcquireAll(); defer releaseAll()` replaces
`LoadInstances()` at line 47, loop unchanged. `hibernation_sweeper.go`: same substitution, only in
the `s.liveProvider == nil` branch (211) — `!= nil` already uses borrowed `GetInstances()` refs.
Generically closes §2's second sub-finding: `AcquireAll`'s closure is the fix, reusable anywhere.

### 5.7 `daemon/daemon.go:288-330` (`detectAndAddNewSessions`) — the duplicate-actor hazard (before:
reconstructs **every** persisted session every tick, discards already-tracked matches — leaking one
actor per tracked session per tick and racing the live one in `currentInstances`, §3b)
```go
func detectAndAddNewSessions(currentInstances *[]*session.LiveInstance, releases *[]func(), registry *session.Registry) error {
    dataList, err := registry.Storage().LoadInstanceData() // metadata only, no Start()
    if err != nil { return fmt.Errorf("failed to load instance data: %w", err) }
    existingTitles := make(map[string]bool)
    for _, instance := range *currentInstances { existingTitles[instance.Title] = true }
    for _, data := range dataList {
        if existingTitles[data.Title] || !data.Started() { continue } // never Acquire a tracked session
        live, release, err := registry.Acquire(data.GetStableID())
        if err != nil { log.Warn("acquire failed", "session", data.Title, "err", err); continue }
        live.AutoYes = true
        *currentInstances = append(*currentInstances, live)
        *releases = append(*releases, release)
    }
    return nil
}
```
This is §3b's own recommended fix ("skip reconstructing already-tracked titles using the existing
`existingTitles` map") — no `FromInstanceData`/`Start()` ever runs for a tracked session, so the
two-actors-same-tmux-target race is eliminated by construction; even a buggy filter can't produce
a second actor, since `Acquire`'s dedup (§1) still catches it.

---

## 6. Shutdown/drain

Two triggers, matching ADR-029's pattern (primary: session deletion; secondary: `shutdownHooks`
safety net) — `Registry` sits between `release()` and actual teardown, adding no third mechanism.

**Trigger 1 — refcount reaches zero** (§1). Per R2.14 this should rarely fire "for real" for a
still-alive session, since the long-lived owner (registry-backed poller/startup path, §2/§3) holds
a reference for its whole lifetime — refcount really hits zero once `removeFromAllPollers`
(`session_service.go:1819-1834`) releases that hold, i.e. session deletion.

**Trigger 2 — full server shutdown:**
```go
// Registry.Shutdown force-stops every actor regardless of refcount. Registered as
// one more shutdownHooks entry (server.go:387,393,400 already do this for
// UserPRCache/WorkflowScheduler/BacklogService).
func (r *Registry) Shutdown() {
    r.mu.Lock()
    entries := r.entries
    r.entries = make(map[string]*registryEntry) // no late Acquire reuses a stopped actor
    r.mu.Unlock()
    for _, e := range entries { e.instance.stopActor() }
}
// srv.shutdownHooks = append(srv.shutdownHooks, deps.Registry.Shutdown)
```
Extends ADR-029's "stop every live actor on shutdown" task, no second idiom. Runs before
`s.httpServer.Shutdown(ctx)` (`server.go:718`), same ordering ADR-029 requires.

**Ordering.** Extends, doesn't change, `architecture.md` §6's rule ("Storage must remove the
instance before calling `Close()`"): `release()` reaching zero must happen *after*
`storage.DeleteInstance()` runs, so no racing `Acquire` can resurrect a handle for a
soon-to-vanish ID. `defer release()` placed before `storage.DeleteInstance()` in the same
function provides this for free (`defer`s run LIFO) — `Registry` guarantees only one goroutine is
ever mid-construction for a given ID, which is what that ordering rule needs under concurrency.
`snapshot.Load()` keeps serving reads after `stopActor()` exactly as `architecture.md` §6
designed — `Registry` only changes *who decides when* teardown fires, not the teardown itself.
