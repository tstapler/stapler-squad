# Stack Research — Go DI Patterns and warren API

## 1. Go Conventions: Splitting Large Model Files

### Same-Package File Splitting (The Go Way)

Go allows a type's methods to span any number of files within the same package. The rule is simple: all files in a directory share one package namespace. There is no concept of "partial classes" — Go achieves the same result by convention.

**Standard pattern in large Go codebases:**

```
session/
  instance.go             ← struct definition + constructors + core lifecycle
  instance_status.go      ← status machine methods
  instance_tmux.go        ← tmux interaction methods
  instance_worktree.go    ← git worktree methods
  instance_tags.go        ← tag management
  instance_serialization.go ← ToInstanceData / FromInstanceData
```

This is already partially in use: `instance_status.go` (222 lines) and `instance_workspace.go` (472+ lines) exist in `session/`. The split is working — those files compile and tests pass.

**Evidence from well-known codebases:**
- **Kubernetes**: `pkg/kubelet/kubelet.go` (struct + core lifecycle) + `kubelet_pods.go`, `kubelet_node.go`, `kubelet_volumes.go` — each file groups related methods.
- **Docker/Moby**: `daemon/daemon.go` (core struct) + `daemon_unix.go`, `daemon_windows.go`, `daemon_network.go`, etc.
- **HashiCorp Vault**: `vault/core.go` + `vault/core_metrics.go`, `vault/core_backend.go` — domain-focused splits within same package.

### When to Split a File vs Extract a Sub-Package

| Criterion | Same-Package File Split | New Sub-Package |
|---|---|---|
| Type still needs access to private fields | Yes — use file split | No — impossible |
| Domain cohesion is loose | Yes — group related methods | Yes |
| Callers need to import just one piece | No | Yes |
| Circular import risk | N/A | High — check carefully |
| Goal is readability/reviewability | Yes | Overkill |

**Key insight**: The requirements explicitly prohibit sub-package extraction (`session/` splits only). Same-package file splitting is the correct pattern here. Go methods are not bound to the file where the struct is declared — `func (i *Instance) Foo()` is valid in any `.go` file in `package session`.

### Go File-Ordering Rules

Within a package:
- **`init()` execution order**: files are processed alphabetically; `init()` in `instance_status.go` runs before `init()` in `instance_tmux.go`. The `session/` package has no `init()` functions in any `instance*.go` file (only `session/detection/ratelimit/detector.go` and `session/ent/` generated code have `init()`).
- **Method resolution**: no ordering dependency — methods on a type defined in file A can be called by methods in file B.
- **Package-level variables**: evaluated before `init()` in alphabetical file order. None of the instance files use package-level vars that depend on each other.

**Conclusion**: The file split carries zero init-order risk for `instance.go`.

---

## 2. pkg/warren API Surface

### Wire (`pkg/warren/wire.go`)

```go
type Wire struct {
    component string
    entries   []wireEntry
}

func NewWire(component string) *Wire
func Set[T comparable](w *Wire, name string, setter func(T), value T)
func SetAlways[T any](w *Wire, name string, setter func(T), value T)
func (w *Wire) Require(name string) *Wire
func (w *Wire) Mark(name string)
func (w *Wire) Validate() error      // returns descriptive error listing missing setters
func (w *Wire) MustValidate()        // panics on validation failure
func (w *Wire) Applied() int
func (w *Wire) Total() int
```

**Key behaviors:**
- `Set` skips the call and records an error entry if `value` is the zero value (nil for pointers/interfaces). `Validate()` then surfaces all skipped setters.
- `SetAlways` unconditionally calls the setter — use for bool/int params.
- `Require` + `Mark` pattern for conditional setters (e.g., inside an `if` block).
- Error message format: `"warren: <component> wiring incomplete — unapplied setters: <name> (<reason>), ..."`.

### Binding (`pkg/warren/binding.go`)

```go
type Binding[T any] struct { ... }

func NewBinding[T any](name string) *Binding[T]
func (b *Binding[T]) Set(v T)
func (b *Binding[T]) Get() (T, bool)
func (b *Binding[T]) Must() T           // panics if not set
func (b *Binding[T]) Override(t testing.TB, v T)  // test-only, auto-restored
func (b *Binding[T]) Name() string
func (b *Binding[T]) IsSet() bool
```

Binding is a concurrency-safe slot for a single globally-wired value. `Override` is test-only (takes `testing.TB`), ensuring it cannot be called in production code.

### App (`pkg/warren/app.go` — inferred from app_test.go)

```go
type App struct { ShutdownTimeout time.Duration; ... }

func New() *App
func (a *App) Phase(name string, fn func(context.Context, *App) error)
func (a *App) Go(name string, fn func(context.Context))
func (a *App) OnStop(name string, fn func(context.Context) error)
func (a *App) Health(name string, fn func() error)
func (a *App) Check() HealthReport
func (a *App) Start(ctx context.Context) error
func (a *App) Stop(ctx context.Context) error
func (a *App) Run(ctx context.Context) error
func (a *App) Active() map[string]int
```

### Current Usage Gap

`warren.Wire` and `warren.Set` are used **only in `main.go`** for the top-level App phase wiring, and defined/tested in `pkg/warren/`. They are NOT used inside `BuildServiceDeps` or `BuildRuntimeDeps` in `server/dependencies.go`, which is the core gap G1 aims to fix.

### How to Apply Wire to dependencies.go

The pattern for `BuildServiceDeps`:
```go
func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) {
    // ... construction ...

    w := warren.NewWire("ServiceDeps")
    warren.Set(w, "StatusManager",        core.SessionService.SetStatusManager,        statusManager)
    warren.Set(w, "ReviewQueuePoller",    core.SessionService.SetReviewQueuePoller,    reviewQueuePoller)
    warren.Set(w, "ApprovalProvider",     reviewQueuePoller.SetApprovalProvider,       core.ApprovalStore)
    if err := w.Validate(); err != nil {
        return nil, err
    }

    return &ServiceDeps{...}, nil
}
```

---

## 3. Setter Injection Validation Patterns in Go (Without DI Frameworks)

Without a framework, Go codebases use these patterns:

1. **Constructor injection** (preferred): pass all deps to `New*()`. Validates at compile time. Not viable here without large-scale refactor.
2. **Validation function**: after setting, call a `func (s *Svc) Validate() error` that checks for nil fields.
3. **Warren Wire** (this project's pattern): record each `Set*` call; `Validate()` reports all missing at once — better than panic on first nil access.
4. **`sync.Once` + sentinel**: set deps in a `Once`-protected setup function that returns error if any dep is nil.

Warren's approach has a key advantage over rolling a manual nil-check: it reports ALL missing setters in one error, not just the first one hit at runtime.
