package profiling

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/trace"
	"time"

	pyroscope "github.com/grafana/pyroscope-go"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Config holds profiling configuration
type Config struct {
	Enabled      bool
	HTTPPort     int
	BlockProfile bool
	MutexProfile bool
	CPUProfile   bool
	TraceEnabled bool
	TraceFile    string
}

// StartProfiling enables runtime profiling based on config
func StartProfiling(cfg Config) (func(), error) {
	if !cfg.Enabled {
		return func() {}, nil
	}

	var cleanupFuncs []func()

	// Enable block profiling (shows where goroutines block)
	if cfg.BlockProfile {
		runtime.SetBlockProfileRate(1)
		log.InfoLog.Printf("Block profiling enabled")
		cleanupFuncs = append(cleanupFuncs, func() {
			runtime.SetBlockProfileRate(0)
		})
	}

	// Enable mutex profiling (shows lock contention)
	if cfg.MutexProfile {
		runtime.SetMutexProfileFraction(1)
		log.InfoLog.Printf("Mutex profiling enabled")
		cleanupFuncs = append(cleanupFuncs, func() {
			runtime.SetMutexProfileFraction(0)
		})
	}

	// Start execution trace (detailed goroutine execution tracking)
	if cfg.TraceEnabled {
		traceFile := cfg.TraceFile
		if traceFile == "" {
			traceFile = fmt.Sprintf("/tmp/stapler-squad-trace-%d.out", os.Getpid())
		}
		f, err := os.Create(traceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create trace file: %w", err)
		}
		if err := trace.Start(f); err != nil {
			f.Close()
			return nil, fmt.Errorf("failed to start trace: %w", err)
		}
		log.InfoLog.Printf("Execution trace enabled: %s", traceFile)
		log.InfoLog.Printf("View with: go tool trace %s", traceFile)
		cleanupFuncs = append(cleanupFuncs, func() {
			trace.Stop()
			f.Close()
			log.InfoLog.Printf("Trace saved to: %s", traceFile)
		})
	}

	// Start HTTP profiling server
	if cfg.HTTPPort > 0 {
		addr := fmt.Sprintf("localhost:%d", cfg.HTTPPort)
		mux := http.NewServeMux()
		// pprof handlers are registered on http.DefaultServeMux by the blank import;
		// forward all /debug/pprof/ requests there.
		mux.Handle("/debug/pprof/", http.DefaultServeMux)

		// Fork pressure metrics endpoint — no pprof dependency, zero-cost to query.
		mux.HandleFunc("/debug/fork-pressure", func(w http.ResponseWriter, r *http.Request) {
			s := tmux.ForkPressureSnapshot()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"level":              s.Level.String(),
				"window_seconds":     int(s.WindowDuration.Seconds()),
				"spawns_in_window":   s.SpawnsInWindow,
				"failures_in_window": s.FailuresInWindow,
				"zombies_in_window":  s.ZombiesInWindow,
				"total_spawns":       s.TotalSpawns,
				"total_failures":     s.TotalFailures,
				"total_zombies":      s.TotalZombies,
				"last_alert_at":      s.LastAlertAt,
				"thresholds": map[string]any{
					"failure_alert": 5,
					"spawn_warn":    60,
					"zombie_alert":  3,
				},
			})
		})

		srv := &http.Server{Addr: addr, Handler: mux}

		go func() {
			log.InfoLog.Printf("Profiling server started on http://%s/debug/pprof/", addr)
			log.InfoLog.Printf("  - Goroutines:    http://%s/debug/pprof/goroutine?debug=1", addr)
			log.InfoLog.Printf("  - Heap:          http://%s/debug/pprof/heap", addr)
			log.InfoLog.Printf("  - Block:         http://%s/debug/pprof/block?debug=1", addr)
			log.InfoLog.Printf("  - Mutex:         http://%s/debug/pprof/mutex?debug=1", addr)
			log.InfoLog.Printf("  - CPU:           curl http://%s/debug/pprof/profile?seconds=30 > cpu.prof", addr)
			log.InfoLog.Printf("  - Fork pressure: http://%s/debug/fork-pressure", addr)
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.ErrorLog.Printf("Profiling server error: %v", err)
			}
		}()

		cleanupFuncs = append(cleanupFuncs, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				log.ErrorLog.Printf("Error shutting down profiling server: %v", err)
			}
		})
	}

	// Return cleanup function
	return func() {
		for _, cleanup := range cleanupFuncs {
			cleanup()
		}
	}, nil
}

// StartContinuousProfiling starts Pyroscope continuous profiling if serverAddr is non-empty.
// Returns a stop function (call on shutdown) and any initialization error.
// When serverAddr is empty, returns a no-op stop function and nil error.
func StartContinuousProfiling(appName, serverAddr string) (func(), error) {
	if serverAddr == "" {
		return func() {}, nil
	}
	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: appName,
		ServerAddress:   serverAddr,
		Logger:          nil,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileGoroutines,
		},
	})
	if err != nil {
		return func() {}, fmt.Errorf("pyroscope: %w", err)
	}
	return func() { _ = profiler.Stop() }, nil
}

// PrintGoroutineStacks prints all goroutine stacks to logs
// Useful for debugging hangs
func PrintGoroutineStacks() {
	buf := make([]byte, 1<<20) // 1MB buffer
	stacklen := runtime.Stack(buf, true)
	log.InfoLog.Printf("=== Goroutine Stacks ===\n%s", buf[:stacklen])
}

// MonitorGoroutines periodically logs goroutine counts
func MonitorGoroutines(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count := runtime.NumGoroutine()
			log.InfoLog.Printf("Active goroutines: %d", count)

			// Alert on goroutine leak
			if count > 100 {
				log.ErrorLog.Printf("⚠️  High goroutine count detected: %d", count)
			}
		}
	}
}
