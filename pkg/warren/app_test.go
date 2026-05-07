package warren_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/warren"
)

// ── Phase ordering ────────────────────────────────────────────────────────────

func TestApp_PhasesRunInOrder(t *testing.T) {
	var order []string
	app := warren.New()
	app.Phase("alpha", func(_ context.Context, _ *warren.App) error {
		order = append(order, "alpha")
		return nil
	})
	app.Phase("beta", func(_ context.Context, _ *warren.App) error {
		order = append(order, "beta")
		return nil
	})
	app.Phase("gamma", func(_ context.Context, _ *warren.App) error {
		order = append(order, "gamma")
		return nil
	})

	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(context.Background()) //nolint:errcheck

	want := []string{"alpha", "beta", "gamma"}
	for i, got := range order {
		if got != want[i] {
			t.Errorf("phase[%d]: got %q, want %q", i, got, want[i])
		}
	}
}

func TestApp_PhaseErrorStopsExecution(t *testing.T) {
	sentinel := errors.New("phase-b failed")
	var gammaRan bool

	app := warren.New()
	app.Phase("alpha", func(_ context.Context, _ *warren.App) error { return nil })
	app.Phase("beta", func(_ context.Context, _ *warren.App) error { return sentinel })
	app.Phase("gamma", func(_ context.Context, _ *warren.App) error {
		gammaRan = true
		return nil
	})

	err := app.Start(context.Background())
	if err == nil {
		t.Fatal("expected error from phase beta, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error does not wrap sentinel: %v", err)
	}
	if gammaRan {
		t.Error("gamma phase ran after beta failed")
	}
}

func TestApp_StartCalledTwiceReturnsError(t *testing.T) {
	app := warren.New()
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(context.Background()) //nolint:errcheck

	if err := app.Start(context.Background()); err == nil {
		t.Error("expected error on second Start(), got nil")
	}
}

func TestApp_PhaseAfterStartPanics(t *testing.T) {
	app := warren.New()
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(context.Background()) //nolint:errcheck

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling Phase() after Start()")
		}
	}()
	app.Phase("late", func(_ context.Context, _ *warren.App) error { return nil })
}

// ── Goroutine tracking ────────────────────────────────────────────────────────

func TestApp_GoTracksGoroutine(t *testing.T) {
	app := warren.New()
	started := make(chan struct{})

	app.Phase("start", func(_ context.Context, a *warren.App) error {
		a.Go("worker", func(ctx context.Context) {
			close(started)
			<-ctx.Done()
		})
		return nil
	})

	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not start within 1s")
	}

	active := app.Active()
	if active["worker"] != 1 {
		t.Errorf("Active()[\"worker\"] = %d, want 1", active["worker"])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := app.Stop(ctx); err != nil {
		t.Errorf("Stop returned unexpected error: %v", err)
	}

	if len(app.Active()) != 0 {
		t.Errorf("goroutines still active after Stop: %v", app.Active())
	}
}

func TestApp_GoBeforeStartPanics(t *testing.T) {
	app := warren.New()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling Go() before Start()")
		}
	}()
	app.Go("too-early", func(_ context.Context) {})
}

func TestApp_GoLeakDetected(t *testing.T) {
	app := warren.New()
	app.ShutdownTimeout = 100 * time.Millisecond

	app.Phase("run", func(_ context.Context, a *warren.App) error {
		a.Go("leaky", func(ctx context.Context) {
			// Ignores context cancellation — intentional leak for test.
			time.Sleep(10 * time.Second)
		})
		return nil
	})

	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := app.Stop(ctx)
	if err == nil {
		t.Fatal("expected goroutine leak error, got nil")
	}
	if !containsString(err.Error(), "leaky") {
		t.Errorf("error %q does not mention goroutine name %q", err.Error(), "leaky")
	}
}

// ── Stop ordering ─────────────────────────────────────────────────────────────

func TestApp_StopRunsInReverseOrder(t *testing.T) {
	var order []string
	app := warren.New()
	app.OnStop("first", func(_ context.Context) error {
		order = append(order, "first")
		return nil
	})
	app.OnStop("second", func(_ context.Context) error {
		order = append(order, "second")
		return nil
	})
	app.OnStop("third", func(_ context.Context) error {
		order = append(order, "third")
		return nil
	})

	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"third", "second", "first"}
	for i, got := range order {
		if got != want[i] {
			t.Errorf("stop[%d]: got %q, want %q", i, got, want[i])
		}
	}
}

func TestApp_StopCollectsAllErrors(t *testing.T) {
	app := warren.New()
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	app.OnStop("a", func(_ context.Context) error { return errors.New("err-a") })
	app.OnStop("b", func(_ context.Context) error { return nil })
	app.OnStop("c", func(_ context.Context) error { return errors.New("err-c") })

	err := app.Stop(context.Background())
	if err == nil {
		t.Fatal("expected errors from stop functions, got nil")
	}
	if !containsString(err.Error(), "err-a") {
		t.Errorf("error %q missing err-a", err.Error())
	}
	if !containsString(err.Error(), "err-c") {
		t.Errorf("error %q missing err-c", err.Error())
	}
}

// ── Health checks ─────────────────────────────────────────────────────────────

func TestApp_HealthAggregatesChecks(t *testing.T) {
	app := warren.New()
	app.Health("db", func() error { return nil })
	app.Health("cache", func() error { return fmt.Errorf("cache unavailable") })
	app.Health("queue", func() error { return nil })

	report := app.Check()

	if report.Healthy {
		t.Error("expected Healthy=false when a check fails")
	}
	if len(report.Checks) != 3 {
		t.Fatalf("expected 3 check results, got %d", len(report.Checks))
	}
	if !report.Checks[0].Healthy {
		t.Error("db check should be healthy")
	}
	if report.Checks[1].Healthy {
		t.Error("cache check should be unhealthy")
	}
	if report.Checks[1].Err == nil {
		t.Error("cache check result should have non-nil Err")
	}
}

func TestApp_HealthAllPassing(t *testing.T) {
	app := warren.New()
	app.Health("db", func() error { return nil })
	app.Health("queue", func() error { return nil })

	if r := app.Check(); !r.Healthy {
		t.Error("expected Healthy=true when all checks pass")
	}
}

// ── Run convenience wrapper ───────────────────────────────────────────────────

func TestApp_RunStartsAndStopsOnContextCancel(t *testing.T) {
	var phaseRan atomic.Bool
	var stopRan atomic.Bool

	app := warren.New()
	app.Phase("work", func(_ context.Context, a *warren.App) error {
		phaseRan.Store(true)
		a.OnStop("cleanup", func(_ context.Context) error {
			stopRan.Store(true)
			return nil
		})
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	// Give Run() time to start, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after context cancel")
	}

	if !phaseRan.Load() {
		t.Error("phase did not run")
	}
	if !stopRan.Load() {
		t.Error("stop hook did not run")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
