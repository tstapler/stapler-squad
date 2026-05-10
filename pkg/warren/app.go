package warren

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DefaultShutdownTimeout is the time Stop() waits for goroutines to exit
// before reporting them as leaks.
const DefaultShutdownTimeout = 30 * time.Second

// App is the application lifecycle coordinator. It manages phased startup,
// background goroutines, ordered shutdown, and component health checks.
//
// App is NOT a service registry. Never pass *App to service methods — it
// belongs only in the wiring layer (Phase functions and constructors called
// from Phase functions).
//
// The typical lifecycle is:
//
//  1. Declare phases with Phase()
//  2. Call Run() (or Start() + Stop()) from main
//  3. Inside each Phase fn: construct components, register goroutines with Go(),
//     register cleanup with OnStop(), register checks with Health()
type App struct {
	mu      sync.Mutex
	phases  []phaseEntry
	stopFns []stopEntry // in registration order; Stop() calls in reverse
	checks  []healthEntry

	goroutines      *GoroutineGroup // nil until Start() initialises it
	ShutdownTimeout time.Duration

	started bool
}

type phaseEntry struct {
	name string
	fn   func(ctx context.Context, app *App) error
}

type stopEntry struct {
	name string
	fn   func(ctx context.Context) error
}

type healthEntry struct {
	name string
	fn   func() error
}

// New creates an App with DefaultShutdownTimeout.
func New() *App {
	return &App{ShutdownTimeout: DefaultShutdownTimeout}
}

// Phase registers a named startup phase. Phases execute sequentially in
// registration order when Start() is called.
//
// fn receives the root context and the App itself. Inside fn:
//   - construct your components
//   - call a.Go() to register background goroutines
//   - call a.OnStop() to register cleanup hooks
//   - call a.Health() to register health checks
//
// Panics if called after Start().
func (a *App) Phase(name string, fn func(ctx context.Context, app *App) error) *App {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		panic("warren: Phase() called after Start() — declare all phases before starting the app")
	}
	a.phases = append(a.phases, phaseEntry{name: name, fn: fn})
	return a
}

// Go registers a named background goroutine that is tracked by the App.
// fn must return when ctx is cancelled. Stop() waits for all registered
// goroutines to exit before running stop hooks.
//
// Go may only be called from within a Phase function (i.e. after Start() has
// initialised the goroutine group). Panics if called before Start().
//
// Multiple goroutines may share a name; Active() reports counts per name.
func (a *App) Go(name string, fn func(ctx context.Context)) {
	a.mu.Lock()
	g := a.goroutines
	a.mu.Unlock()
	if g == nil {
		panic("warren: Go() must be called from within a Phase function — the goroutine group is not yet initialised")
	}
	g.Go(name, fn)
}

// OnStop registers a named cleanup function. Stop() calls registered functions
// in reverse registration order so that components shut down in the opposite
// order they started.
//
// OnStop may be called before or after Start().
func (a *App) OnStop(name string, fn func(ctx context.Context) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopFns = append(a.stopFns, stopEntry{name: name, fn: fn})
}

// Health registers a named health check function. Check() runs all registered
// functions and returns an aggregate HealthReport.
func (a *App) Health(name string, fn func() error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks = append(a.checks, healthEntry{name: name, fn: fn})
}

// Start runs all registered phases sequentially using ctx as the lifecycle
// context. ctx is also the parent context for all goroutines registered with Go().
//
// Returns on the first phase error. Phases are not retried.
// Returns an error if called more than once.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return fmt.Errorf("warren: Start() called more than once")
	}
	a.started = true
	a.goroutines = NewGoroutineGroup(ctx)
	phases := append([]phaseEntry(nil), a.phases...)
	a.mu.Unlock()

	for _, p := range phases {
		if err := p.fn(ctx, a); err != nil {
			return fmt.Errorf("warren: phase %q: %w", p.name, err)
		}
	}
	return nil
}

// Stop shuts down the application:
//  1. Cancels the goroutine context so all registered goroutines receive a
//     done signal.
//  2. Waits up to ShutdownTimeout for goroutines to exit. Any still running
//     are reported as leaks.
//  3. Calls all OnStop functions in reverse registration order, using ctx
//     (typically a short-deadline context) as the cleanup context.
//
// Returns a *MultiError if any step produced errors, including goroutine leaks.
func (a *App) Stop(ctx context.Context) error {
	a.mu.Lock()
	g := a.goroutines
	stopFns := append([]stopEntry(nil), a.stopFns...)
	a.mu.Unlock()

	var errs []error

	if g != nil {
		if leaked := g.Wait(a.ShutdownTimeout); len(leaked) > 0 {
			errs = append(errs, fmt.Errorf("%s", leakReport(leaked, a.ShutdownTimeout)))
		}
	}

	for i := len(stopFns) - 1; i >= 0; i-- {
		sf := stopFns[i]
		if err := sf.fn(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop %q: %w", sf.name, err))
		}
	}

	return multiError(errs)
}

// Run is a convenience wrapper that calls Start(ctx), blocks until ctx is
// done, then calls Stop() with a fresh context bounded by ShutdownTimeout.
//
// This is the typical entry point from main():
//
//	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer cancel()
//	if err := app.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
func (a *App) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), a.ShutdownTimeout)
	defer cancel()
	return a.Stop(stopCtx)
}

// Check runs all registered health checks and returns an aggregate report.
// Checks run sequentially. A failed check does not prevent subsequent checks
// from running.
func (a *App) Check() HealthReport {
	a.mu.Lock()
	checks := append([]healthEntry(nil), a.checks...)
	a.mu.Unlock()

	report := HealthReport{Checks: make([]CheckResult, 0, len(checks))}
	allHealthy := true
	for _, c := range checks {
		start := time.Now()
		err := c.fn()
		result := CheckResult{
			Name:    c.name,
			Healthy: err == nil,
			Err:     err,
			Latency: time.Since(start),
		}
		if err != nil {
			allHealthy = false
		}
		report.Checks = append(report.Checks, result)
	}
	report.Healthy = allHealthy
	return report
}

// Active returns the names and counts of currently running goroutines.
// Returns nil if Start() has not been called.
func (a *App) Active() map[string]int {
	a.mu.Lock()
	g := a.goroutines
	a.mu.Unlock()
	if g == nil {
		return nil
	}
	return g.Active()
}
