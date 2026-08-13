// Package warren is a lightweight lifecycle coordinator for Go applications.
//
// It provides phased startup, tracked goroutine management, validated setter
// injection, typed test overrides, and component health checks. It is
// designed to sit alongside explicit constructor-based dependency wiring —
// not to replace it.
//
// # Philosophy
//
// Warren is NOT a service registry. It never holds references to your services
// at runtime, resolves dependencies by reflection, or require you to annotate
// your structs. You write normal constructor functions; warren coordinates when
// they run and what happens when the process shuts down.
//
// The three rules:
//
//  1. Never pass *App to a service method — only to constructors and phase functions.
//  2. Every background goroutine must be registered with Go() so Stop() can wait for it.
//  3. Every required post-construction setter must be registered with Wire so Validate() can catch omissions.
//
// # Quick start
//
//	app := warren.New()
//
//	app.Phase("core", func(ctx context.Context, a *warren.App) error {
//	    cfg := config.Load()
//	    repo, err := db.Open(cfg)
//	    if err != nil {
//	        return err
//	    }
//	    a.OnStop("db", func(ctx context.Context) error { return repo.Close() })
//	    a.Health("db", repo.Ping)
//	    return nil
//	})
//
//	app.Phase("runtime", func(ctx context.Context, a *warren.App) error {
//	    a.Go("poller", func(ctx context.Context) {
//	        for {
//	            select {
//	            case <-ctx.Done():
//	                return
//	            case <-time.After(5 * time.Second):
//	                poll()
//	            }
//	        }
//	    })
//	    return nil
//	})
//
//	if err := app.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # Components
//
// App — lifecycle coordinator, see [App].
//
// GoroutineGroup — standalone goroutine tracking for use inside services, see [GoroutineGroup].
//
// Binding — typed overridable component slot for test injection, see [Binding].
//
// Wire — validates that all required post-construction setters were called, see [Wire] and [Set].
//
// HealthReport — aggregate health status, see [App.Check].
package warren
