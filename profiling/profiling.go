package profiling

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/trace"
	"time"

	"claude-squad/log"
)

// Config holds profiling configuration
type Config struct {
	Enabled       bool
	HTTPPort      int
	BlockProfile  bool
	MutexProfile  bool
	CPUProfile    bool
	TraceEnabled  bool
	TraceFile     string
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
			traceFile = fmt.Sprintf("/tmp/claude-squad-trace-%d.out", os.Getpid())
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
		srv := &http.Server{Addr: addr}

		go func() {
			log.InfoLog.Printf("Profiling server started on http://%s/debug/pprof/", addr)
			log.InfoLog.Printf("  - Goroutines: http://%s/debug/pprof/goroutine?debug=1", addr)
			log.InfoLog.Printf("  - Heap:       http://%s/debug/pprof/heap", addr)
			log.InfoLog.Printf("  - Block:      http://%s/debug/pprof/block?debug=1", addr)
			log.InfoLog.Printf("  - Mutex:      http://%s/debug/pprof/mutex?debug=1", addr)
			log.InfoLog.Printf("  - CPU:        curl http://%s/debug/pprof/profile?seconds=30 > cpu.prof", addr)
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.ErrorLog.Printf("Profiling server error: %v", err)
			}
		}()

		cleanupFuncs = append(cleanupFuncs, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(ctx)
		})
	}

	// Return cleanup function
	return func() {
		for _, cleanup := range cleanupFuncs {
			cleanup()
		}
	}, nil
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
