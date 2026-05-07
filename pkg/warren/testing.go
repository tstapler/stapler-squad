package warren

import (
	"context"
	"testing"
	"time"
)

// TestApp creates an App for use in tests. It sets a short ShutdownTimeout
// (2 seconds) to make goroutine leak detection fast. Stop() is registered
// via t.Cleanup and called automatically when the test ends.
//
// Usage:
//
//	func TestMyComponent(t *testing.T) {
//	    app := warren.TestApp(t)
//	    app.Phase("setup", func(ctx context.Context, a *warren.App) error {
//	        // construct components with test doubles
//	        return nil
//	    })
//	    if err := app.Start(context.Background()); err != nil {
//	        t.Fatal(err)
//	    }
//	    // test body — Stop() is called automatically via t.Cleanup
//	}
func TestApp(t testing.TB) *App {
	t.Helper()
	app := &App{ShutdownTimeout: 2 * time.Second}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := app.Stop(ctx); err != nil {
			t.Logf("warren.TestApp cleanup: Stop() returned error: %v", err)
		}
	})
	return app
}
