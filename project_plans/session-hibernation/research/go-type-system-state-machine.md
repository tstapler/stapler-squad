# Go Type System Patterns for a Maintainable State Machine

Research for the session-hibernation / state machine redesign epic.
Current codebase: Go 1.25.0, ent ORM (status stored as `field.Int("status")`), ConnectRPC, concurrent access via `deadlock.RWMutex`.

---

## Context: Current Implementation

`session/state_machine.go` defines:

```go
type Status int      // iota constants: Running, Ready, Loading, Paused, NeedsApproval, Creating, Stopped
var allowedTransitions = map[Status][]Status{ ... }
func CanTransition(from, to Status) bool { ... }
```

`session/instance_state.go` provides:

```go
func (i *Instance) transitionTo(s Status) error   // must hold stateMutex
func (i *Instance) setStatus(s Status)            // no-lock, inner
```

The ent schema stores status as `field.Int("status")` — the `Status` int value is written directly to SQLite.
`GetEffectiveStatus()` overlays `DetectedStatus` from the terminal controller on top of the lifecycle status at read time. Sub-status is already not persisted; only the lifecycle enum is stored.

Target states from requirements: `Creating`, `Active`, `Paused`, `Stopped`, `Hibernated` (5 states, down from 7).

---

## Technique 1: Exhaustive Switches via Unexported Interface

### How it works

```go
type Status interface{ isStatus() }

type statusCreating  struct{}; func (statusCreating)  isStatus() {}
type statusActive    struct{}; func (statusActive)    isStatus() {}
type statusPaused    struct{}; func (statusPaused)    isStatus() {}
type statusStopped   struct{}; func (statusStopped)   isStatus() {}
type statusHibernated struct{}; func (statusHibernated) isStatus() {}

var (
    Creating   Status = statusCreating{}
    Active     Status = statusActive{}
    Paused     Status = statusPaused{}
    Stopped    Status = statusStopped{}
    Hibernated Status = statusHibernated{}
)
```

Because `isStatus()` is unexported, no type outside the package can implement `Status`. Only the five values above exist.

**Can the compiler force exhaustiveness?** No — Go does not have sealed interfaces or match exhaustiveness checks. A `switch s.(type)` that omits a case compiles fine. The only tooling workaround is `exhaustive` linter (github.com/nishanths/exhaustive), which works on both `const`-type enums and interface type-switches when annotated with `//nolint` markers.

**Workarounds for exhaustiveness:**
1. `exhaustive` linter with `--default-signifies-exhaustive=false` — flags missing cases at CI time.
2. Encode the switch as a `visitor` / `Matcher` function:
   ```go
   type StatusVisitor struct {
       OnCreating   func()
       OnActive     func()
       OnPaused     func()
       OnStopped    func()
       OnHibernated func()
   }
   func MatchStatus(s Status, v StatusVisitor) {
       switch s.(type) {
       case statusCreating:   v.OnCreating()
       case statusActive:     v.OnActive()
       case statusPaused:     v.OnPaused()
       case statusStopped:    v.OnStopped()
       case statusHibernated: v.OnHibernated()
       }
   }
   ```
   Callers that miss a field get a zero-value no-op, not a compile error — this is only a UX improvement.
3. Code-gen a compile-time check using `var _ = [1]struct{}{}[someExpr]` tricks — very fragile.

**Interaction with ent ORM (int storage):**
The ent schema uses `field.Int("status")`. With the interface approach, `Status` has no natural integer representation. You must maintain a manual mapping:

```go
func StatusToInt(s Status) int { switch s.(type) { ... } }
func StatusFromInt(n int) (Status, error) { switch n { ... } }
```

This is boilerplate that duplicates the iota ordering exactly where iota was eliminating it.

**Interaction with JSON serialization:**
Same problem. `encoding/json` cannot marshal/unmarshal an interface value without custom `MarshalJSON`/`UnmarshalJSON` that call the int-mapping functions.

**Verdict: Avoid.** The "sealed interface" trick adds substantial serialization boilerplate while not actually providing compile-time exhaustiveness (only linter-time). It does not compose naturally with ent's int column or JSON/proto wire formats.

---

## Technique 2: Typed String/Int Enums with a `Valid()` Method (Current Approach)

### How it works

```go
type Status int

const (
    Creating   Status = iota
    Active
    Paused
    Stopped
    Hibernated
)

func (s Status) Valid() bool {
    return s >= Creating && s <= Hibernated
}
```

**Current approach pros:**
- Direct int storage in ent — no mapping layer.
- JSON serialization via `String()` / `UnmarshalJSON` is straightforward.
- Concurrent-safe by value (int copies are atomic on all supported platforms).
- `Status.String()` gives human-readable logging.
- The `exhaustive` linter works with `type Status int` + `const ( ... )` out of the box.

**Current approach cons:**
- Zero value (`0 = Running` currently, `0 = Creating` after rename) is significant — the zero value of the type is a real state, not a sentinel. Reordering iota values is a breaking DB migration.
- No per-transition metadata (guards, actions) in the type definition; that lives separately in `allowedTransitions`.
- Compiler does not enforce exhaustive switches; a `default:` case silently swallows new states.

**Adding compile-time guards without changing the type:**
1. Add `exhaustive` to the lint toolchain (already recommended in `.claude/docs/nil-safety.md` style; same pattern).
2. Omit `default:` from all status switches and let `exhaustive` flag missing cases.
3. Add a compile-time count check using a blank array:
   ```go
   const _statusCount = 5
   var _ = [_statusCount]struct{}{}[Hibernated] // compile error if Hibernated != 4
   ```
   This is fragile but catches iota reordering.

**Verdict: Recommended as the base type.** The typed-int enum is the right storage primitive because it maps directly to the ent `field.Int("status")` column. Layer exhaustiveness via the `exhaustive` linter rather than changing the type.

---

## Technique 3: Functional Options + Builder Pattern for Transitions

### How it works

Instead of a flat map, transitions are declared as first-class values:

```go
type TransitionDef struct {
    From  Status
    To    Status
    Guard func(ctx context.Context, i *Instance) error
    After func(ctx context.Context, i *Instance) // side effect after transition
}

var transitions = []TransitionDef{
    {From: Creating, To: Active,     Guard: guardCreatingDone},
    {From: Creating, To: Stopped,    Guard: nil},
    {From: Active,   To: Paused,     Guard: guardHasWorktree, After: afterPause},
    {From: Active,   To: Stopped,    Guard: nil},
    {From: Active,   To: Hibernated, Guard: guardIsIdle,      After: afterHibernate},
    {From: Paused,   To: Active,     Guard: nil,              After: afterResume},
    {From: Paused,   To: Stopped,    Guard: nil},
    {From: Stopped,  To: Active,     Guard: guardColdRestore, After: afterColdStart},
    {From: Hibernated, To: Active,   Guard: nil,              After: afterWakeResume},
    {From: Hibernated, To: Stopped,  Guard: nil},
}
```

A helper builds a lookup map at init time for O(1) lookup:

```go
type transitionKey struct{ From, To Status }
var transitionIndex map[transitionKey]TransitionDef

func init() {
    transitionIndex = make(map[transitionKey]TransitionDef, len(transitions))
    for _, t := range transitions {
        transitionIndex[transitionKey{t.From, t.To}] = t
    }
}

func (i *Instance) transitionTo(ctx context.Context, to Status) error {
    key := transitionKey{i.Status, to}
    def, ok := transitionIndex[key]
    if !ok {
        return ErrInvalidTransition{From: i.Status, To: to}
    }
    if def.Guard != nil {
        if err := def.Guard(ctx, i); err != nil {
            return err
        }
    }
    i.Status = to
    if def.After != nil {
        def.After(ctx, i) // runs while lock still held
    }
    return nil
}
```

**Comparison to current map-based approach:**

| Aspect | Current map | TransitionDef slice |
|---|---|---|
| Per-transition guards | Not supported | First-class |
| Per-transition after-hooks | Not supported | First-class |
| Readability | Concise | Verbose but self-documenting |
| Adding a new transition | One slice append | One slice append |
| Guard logic lives | Scattered in callers | Co-located with transition |
| Context propagation to guards | Not possible | Straightforward |

The current code dispatches guards _before_ calling `transitionTo` — e.g., `Approve()` checks `NeedsApproval` before calling `transitionTo(Running)`. The `TransitionDef` approach inlines those checks, removing call-site discipline requirements.

**Fit for this codebase:** High. The five new states have meaningfully different preconditions: hibernation needs an idle check + scrollback snapshot; cold restore needs worktree validation; pause needs worktree deletion. Encoding these as `Guard` functions on the `TransitionDef` is cleaner than the current pattern of per-method pre-checks scattered across `instance_state.go`.

**Verdict: Recommended for the transition layer.** Keep `type Status int` but replace the `map[Status][]Status` with a `[]TransitionDef` slice. The `transitionTo` method becomes the single choreography point: validate guard, apply state, run after-hook.

---

## Technique 4: Third-Party `statemachine` Packages

### Packages evaluated

#### `looplab/fsm` (github.com/looplab/fsm)

- States and events defined as `string` — not typed. `fsm.FSM.State()` returns `string`.
- Transitions are declared as `[]fsm.EventDesc`, each `EventDesc` has `Name string`, `Src []string`, `Dst string`.
- Callbacks via string-keyed map `map[string]func(*Event)`.
- **Typed states**: No. All state names are untyped strings. A typo in a state name silently produces undefined behavior.
- **ent ORM integration**: Requires a custom `int ↔ string` mapping layer — adds boilerplate equivalent to what we already have.
- **Maintenance burden**: Active project, ~3k GitHub stars. API is stable but the string-typed states are a fundamental mismatch with this codebase's goals.
- **Verdict: Avoid.** String-typed states provide no safety over a hand-rolled solution, and add an import dependency for no architectural gain.

#### `qmuntal/stateless` (github.com/qmuntal/stateless)

- Inspired by .NET Stateless library.
- States are `interface{}` (any). You _can_ pass `Status` int values as states.
- Trigger/event system is also `interface{}`.
- Supports entry/exit/transition actions, guard conditions, substates (hierarchical states).
- **Typed states**: Partially. Since states are `interface{}`, you use your own typed enum values, but the library's internal storage is `interface{}` maps — no exhaustiveness checking at any level.
- **ent ORM integration**: Better than looplab/fsm because you supply your own int-typed state constants. No string mapping needed.
- **Substates**: Has a hierarchical state concept that could model `(Active, Processing)` etc., but requires careful setup.
- **Maintenance burden**: Active, ~1k GitHub stars. Smaller community. More complex API than needed.
- **Verdict: Use for subset / Avoid.** `stateless` is the closest match to this project's needs, but its `interface{}`-based state storage means you lose type safety at the boundaries and gain only the guard/callback dispatch that `TransitionDef` provides more transparently.

#### `dyrector-io/stapled`

- As of research date, this is a much smaller project focused on the dyrector.io product. Not a general-purpose FSM library. Limited documentation.
- **Verdict: Avoid.** Not general-purpose; insufficient ecosystem adoption.

### Overall assessment of third-party FSM packages

None of the evaluated packages provide:
1. Compile-time-typed states (not string or interface{})
2. Native ent ORM int storage integration
3. A significantly reduced amount of code vs. rolling our own

The "rolling our own" path (`TransitionDef` + `transitionIndex` map) is ~60 lines of code and produces a more readable, type-safe result than any of these packages. The maintenance burden is trivially lower because there is no external dependency to upgrade.

**Verdict: Roll our own.** Use `TransitionDef` slice + init-time index map.

---

## Technique 5: State-Tagged Method Names

### How it works

Instead of a central `transitionTo` dispatcher, every operation encodes its precondition in its method name:

```go
func (i *Instance) hibernateFromActive(ctx context.Context) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    if i.Status != Active {
        return ErrInvalidTransition{From: i.Status, To: Hibernated}
    }
    // hibernation logic inline
    i.Status = Hibernated
    return nil
}

func (i *Instance) resumeFromHibernated(ctx context.Context) error { ... }
func (i *Instance) pauseFromActive(ctx context.Context) error { ... }
```

**Pros:**
- The precondition is visible in the method name — no lookup in `state_machine.go`.
- Each method can have its own signature (different parameters, return types).
- No central dispatch function to maintain.

**Cons:**
- The FSM graph becomes implicit. To understand all valid transitions, you must grep for methods with this naming convention.
- Adding a new state requires auditing all methods to check if they handle the new state — no centralized table to update.
- No systematic way to enforce that every (`from`, `to`) pair has a method, nor that no method implements an invalid pair.
- The current codebase already has named methods (`Approve()`, `Deny()`, `MarkNeedsApproval()`) — but these call `transitionTo`, maintaining the central table. Removing `transitionTo` loses the single source of truth.

**Verdict: Use for the public API layer only.** Exported methods like `Hibernate()`, `Pause()`, `Resume()`, `Stop()` should be the public surface. Internally they should call `transitionTo` (or the `TransitionDef`-aware equivalent) rather than directly manipulating `i.Status`. This gives the best of both: descriptive public methods AND a single auditable transition table.

---

## Technique 6: Compile-Time Exhaustiveness with Generics (Go 1.21+)

### How it works

Go 1.25.0 is in use (confirmed in `go.mod`). The relevant generics-based pattern:

```go
// Attempt to create a generic "match" function that forces exhaustiveness
type StatusCase[T any] struct {
    Creating   func() T
    Active     func() T
    Paused     func() T
    Stopped    func() T
    Hibernated func() T
}

func MatchStatus[T any](s Status, c StatusCase[T]) T {
    switch s {
    case Creating:   return c.Creating()
    case Active:     return c.Active()
    case Paused:     return c.Paused()
    case Stopped:    return c.Stopped()
    case Hibernated: return c.Hibernated()
    default:
        panic(fmt.Sprintf("unhandled Status %v", s))
    }
}
```

**Can this force compile-time exhaustiveness?** Partially. If a caller sets `StatusCase[string]{Creating: ..., Active: ...}` and omits `Paused`, the struct literal compiles — Go does not require all fields of a struct to be set. The missing fields get the zero value (`nil` for `func()`), causing a nil dereference at runtime.

To force compile-time errors you would need a variadic-parameter or builder API that the compiler checks — not possible with Go's struct literal semantics.

**What generics actually help with:** A `StatusCase[T]` struct is useful as a pattern for exhaustive _code generation_, not compiler enforcement. You can write a `go generate` step that produces a `Status.Match(c StatusCase[T]) T` function and grep the codebase for callers that don't set all fields (using AST analysis or `exhaustruct` linter).

**The `exhaustruct` linter** (github.com/GaijinEntertainment/go-exhaustruct) flags struct literals with unset fields. Combining `StatusCase[T]` with `exhaustruct` gives near-compile-time exhaustiveness at lint time.

**Fit for this codebase:** Moderate. Go 1.25 supports this pattern. The main cost is the `StatusCase[T]` struct needs to be updated whenever a state is added — but that is exactly what we want. Using `exhaustruct` in CI makes omissions a lint failure.

**Verdict: Use for subset.** Adopt the `StatusCase[T]` pattern (or the `exhaustive` linter on `switch` statements) as a CI-time guard. Do not use it as the primary dispatch mechanism — it adds verbosity. Use it for high-value switch sites (the proto adapter, the frontend label mapper) where silent omissions have user-visible consequences.

---

## Technique 7: Sub-Status Without New Lifecycle States

### The problem

The requirements define `(Active, Processing)`, `(Active, Idle)`, `(Active, NeedsApproval)` tuples. Three modeling options:

### Option A: Tuple field on Instance

```go
type FullStatus struct {
    Lifecycle Status
    Sub       SubStatus // only meaningful when Lifecycle == Active
}
```

**Pros:** Single field lookup; easy to add to proto as a pair of fields.
**Cons:** `Sub` must be cleared on every transition out of `Active`; `Sub` must never be set when `Lifecycle != Active`. These invariants require discipline across all call sites.
**Fit:** Medium. Adds a DB column (if persisted) or a memory field (if not).

### Option B: Separate ephemeral field, never stored in DB

```go
type Instance struct {
    Status    Status    // persisted as ent field.Int("status")
    SubStatus SubStatus // NOT in ent schema; derived at read time
}
```

`SubStatus` is populated by `GetEffectiveStatus()` equivalent — derived from `DetectedStatus` by the status manager each time the instance is read/streamed.

**Pros:** DB schema unchanged. `SubStatus` is never stale because it is always recomputed. Matches the existing `GetEffectiveStatus()` pattern exactly.
**Cons:** In-memory `SubStatus` on `Instance` could get out of sync with the status manager if code reads `i.SubStatus` directly instead of calling `GetEffectiveStatus()`. Discipline required.
**Fit:** High. This is what the requirements specify: "Sub-status is NOT stored in the database — it is always derived at read time from the detection layer."

### Option C: Keep sub-status entirely out of Instance, compute at API boundary

Sub-status is computed only in `instance_adapter.go` when building the proto `Session` message for API responses:

```go
func toProtoSession(i *Instance, mgr StatusManager) *sessionv1.Session {
    s := &sessionv1.Session{ ... }
    s.SubStatus = subStatusFromDetected(mgr.GetStatus(i))
    return s
}
```

`Instance` itself never has a `SubStatus` field.

**Pros:** Zero risk of `Instance.SubStatus` staleness. Clean separation: `Instance` is the mutable process state; the proto message is the read projection.
**Cons:** Sub-status is unavailable to Go-internal consumers (e.g., review queue poller) without re-deriving it from the status manager. But the review queue poller already works with `DetectedStatus` directly — it does not need `SubStatus`.
**Fit:** High. This is the cleanest architectural boundary.

### Recommendation for Sub-Status

Use **Option C** (API-boundary-only derivation) as the canonical approach:
- `Instance.Status` holds only the 5 lifecycle states, stored as `field.Int("status")` in ent.
- `SubStatus` appears only in the proto `Session` message, populated in the adapter layer.
- Internal Go code that needs fine-grained status (review queue, approval automation) continues to use `detection.DetectedStatus` directly from the status manager — no `SubStatus` type needed in the `session` package.

If Option C proves insufficient (e.g., a new feature needs to store sub-status for filtering), promote to **Option B** — add a `SubStatus` field to `Instance` that is populated by the status manager but never written to ent. A `field.Enum("sub_status").Optional()` ent field can be added later without disrupting the current design.

---

## Final Recommendation

### Adopt this stack for the state machine redesign:

**1. Base type: `type Status int` with named iota constants (Technique 2)**

Keep the existing approach. It is the only type that maps naturally to `field.Int("status")` in ent without a conversion layer. Update the constants to the 5 new states: `Creating=0`, `Active=1`, `Paused=2`, `Stopped=3`, `Hibernated=4`. Do not reuse existing integer values for renamed states — write a migration that maps old ints to new ints.

**2. Transition layer: `[]TransitionDef` with guard + after-hook (Technique 3)**

Replace `map[Status][]Status` with a `[]TransitionDef` slice. The `TransitionDef` struct carries an optional `Guard func(context.Context, *Instance) error` and optional `After func(context.Context, *Instance)`. `transitionTo` becomes the single choreography point that runs guard, updates status, and fires the after-hook.

**3. Exhaustiveness: `exhaustive` linter at CI (Technique 2 + 6)**

Add `exhaustive` to the lint pipeline (same step as `nilaway`). Configure it to flag `switch` statements on `Status` that lack a `default:` or that omit a case. For high-value structural dispatch (proto adapter, label mapper), use the `StatusCase[T]` + `exhaustruct` pattern to catch omissions at lint time.

**4. Public API surface: state-tagged method names calling `transitionTo` internally (Technique 5)**

Exported methods: `Hibernate(ctx)`, `Pause(ctx)`, `Resume(ctx)`, `Stop(ctx)`, `FinishCreating(ctx)`. These are descriptive and discoverable. Internally each calls `transitionTo(ctx, target)`, which delegates to the `TransitionDef` table. Never expose `transitionTo` as public.

**5. Sub-status: Option C — derived at API boundary only (Technique 7)**

Do not add `SubStatus` to `Instance` or ent. Compute it in `instance_adapter.go` from the status manager's `DetectedStatus`. Update `GetEffectiveStatus()` to return `Status` only (no sub-status blending); add a separate `GetSubStatus()` at the adapter level.

---

### Recommended Code Sketch

```go
// state_machine.go

package session

import "context"

// Status is the lifecycle state of a session, persisted as an int in the ent schema.
// DO NOT reorder — integer values are stored in the database.
type Status int

const (
    Creating   Status = 0 // async init in progress
    Active     Status = 1 // AI process alive and running
    Paused     Status = 2 // worktree removed, branch preserved
    Stopped    Status = 3 // explicitly stopped, cold-restore possible
    Hibernated Status = 4 // process killed to save memory, scrollback checkpointed
)

// TransitionDef describes a valid state transition and its optional guard / after-hook.
type TransitionDef struct {
    From  Status
    To    Status
    // Guard runs before the transition; returning an error aborts it.
    Guard func(ctx context.Context, i *Instance) error
    // After runs immediately after status is updated, while the lock is held.
    After func(ctx context.Context, i *Instance)
}

var transitions = []TransitionDef{
    {From: Creating,   To: Active},
    {From: Creating,   To: Stopped},
    {From: Active,     To: Paused,     Guard: guardWorktreePresent, After: afterPause},
    {From: Active,     To: Stopped},
    {From: Active,     To: Hibernated, Guard: guardIsIdle,          After: afterHibernate},
    {From: Paused,     To: Active,                                  After: afterResume},
    {From: Paused,     To: Stopped},
    {From: Stopped,    To: Active,     Guard: guardWorktreePresent, After: afterColdStart},
    {From: Hibernated, To: Active,                                  After: afterWakeResume},
    {From: Hibernated, To: Stopped},
}

type transitionKey struct{ From, To Status }

var transitionIndex map[transitionKey]TransitionDef

func init() {
    transitionIndex = make(map[transitionKey]TransitionDef, len(transitions))
    for _, t := range transitions {
        transitionIndex[transitionKey{t.From, t.To}] = t
    }
}

// CanTransition reports whether a From→To transition is valid.
func CanTransition(from, to Status) bool {
    _, ok := transitionIndex[transitionKey{from, to}]
    return ok
}

// instance_state.go

// transitionTo validates and executes a state transition.
// Must be called with i.stateMutex held.
func (i *Instance) transitionTo(ctx context.Context, to Status) error {
    key := transitionKey{i.Status, to}
    def, ok := transitionIndex[key]
    if !ok {
        return ErrInvalidTransition{From: i.Status, To: to}
    }
    if def.Guard != nil {
        if err := def.Guard(ctx, i); err != nil {
            return err
        }
    }
    i.Status = to
    if def.After != nil {
        def.After(ctx, i)
    }
    return nil
}

// Public methods — state-tagged names, call transitionTo internally:

func (i *Instance) Hibernate(ctx context.Context) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    return i.transitionTo(ctx, Hibernated)
}

func (i *Instance) Pause(ctx context.Context) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    return i.transitionTo(ctx, Paused)
}

func (i *Instance) Resume(ctx context.Context) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    return i.transitionTo(ctx, Active)
}

func (i *Instance) Stop(ctx context.Context) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    return i.transitionTo(ctx, Stopped)
}

func (i *Instance) FinishCreating(ctx context.Context) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    return i.transitionTo(ctx, Active)
}
```

**DB migration note:** The existing integer values for `Paused` (`= 3` in the current iota where `Running=0, Ready=1, Loading=2, Paused=3`) must be remapped. A SQLite migration:

```sql
UPDATE sessions SET status = 4 WHERE status = 3; -- old Stopped(6) → new Stopped(3)
UPDATE sessions SET status = 2 WHERE status = 3; -- old Paused(3) → new Paused(2)
-- etc. — run in ent migration
```

The ent `WithGlobalUniqueID` + versioned migration approach in this repo handles this. Plan the migration carefully — verify with `make test` against a copy of production data.

---

## Summary Table

| Technique | Verdict | Why |
|---|---|---|
| Unexported interface seal | Avoid | Serialization boilerplate, no true compile enforcement |
| Typed int enum + `Valid()` | **Recommended (base type)** | Direct ent int storage, simple JSON, `exhaustive` linter covers gaps |
| `TransitionDef` slice | **Recommended (transition layer)** | Per-transition guards + after-hooks; single source of truth |
| `looplab/fsm` | Avoid | String-typed states, no ent integration |
| `qmuntal/stateless` | Avoid | interface{} states, more complex than needed |
| `dyrector-io/stapled` | Avoid | Not a general-purpose library |
| State-tagged method names | **Recommended (public API only)** | Descriptive surface; internal still uses `transitionTo` |
| Generics `StatusCase[T]` | Use for subset | CI exhaustiveness guard on high-value switches |
| Sub-status tuple on Instance | Avoid as primary | DB/staleness complexity; API-boundary derivation is cleaner |
| Sub-status at API boundary | **Recommended** | Zero schema change; derived from existing detection layer |
