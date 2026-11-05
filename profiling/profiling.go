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
	// Registers /debug/pprof/delta_{heap,block,mutex} on http.DefaultServeMux:
	// per-interval profiles, so a scraper that sums scrapes over a time window
	// (Pyroscope) does not re-count lifetime totals on every scrape.
	_ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// Config holds profiling configuration
type Config struct {
	Enabled      bool
	HTTPPort     int
	BlockProfile bool
	MutexProfile bool
	// MutexFraction samples on average 1/N contended mutex events; 0 means
	// DefaultMutexFraction. 1 records every event, which is measurably costly.
	MutexFraction int
	// BlockRateNs records every blocking event of at least this many ns and a
	// proportional sample of shorter ones; 0 means DefaultBlockRateNs.
	BlockRateNs  int
	CPUProfile   bool
	TraceEnabled bool
	TraceFile    string
}

// Defaults chosen to keep lock profiling cheap enough to leave on in the
// always-profiled service (see docs/observability/README.md,
// "Sampling cost", for the measured overhead of rate 1 versus these).
const (
	DefaultMutexFraction = 10
	DefaultBlockRateNs   = 1000
)

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// StartProfiling enables runtime profiling based on config
func StartProfiling(cfg Config) (func(), error) {
	if !cfg.Enabled {
		return func() {}, nil
	}

	var cleanupFuncs []func()

	// Enable block profiling (shows where goroutines block)
	if cfg.BlockProfile {
		rate := orDefault(cfg.BlockRateNs, DefaultBlockRateNs)
		runtime.SetBlockProfileRate(rate)
		log.Info("Block profiling enabled", "rate_ns", rate)
		cleanupFuncs = append(cleanupFuncs, func() {
			runtime.SetBlockProfileRate(0)
		})
	}

	// Enable mutex profiling (shows lock contention)
	if cfg.MutexProfile {
		fraction := orDefault(cfg.MutexFraction, DefaultMutexFraction)
		runtime.SetMutexProfileFraction(fraction)
		log.Info("Mutex profiling enabled", "fraction", fraction)
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
		// #nosec G304 -- traceFile is either the fixed /tmp/stapler-squad-trace-<pid>.out
		// default above or cfg.TraceFile, which is only ever set to "" by main.go (a
		// compile-time constant); not derived from RPC/network/user request input.
		f, err := os.Create(traceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create trace file: %w", err)
		}
		if err := trace.Start(f); err != nil {
			_ = f.Close() // trace never started, so there's nothing buffered to lose
			return nil, fmt.Errorf("failed to start trace: %w", err)
		}
		log.Info("Execution trace enabled", "file", traceFile)
		log.Info("View with: go tool trace <file>", "file", traceFile)
		cleanupFuncs = append(cleanupFuncs, func() {
			trace.Stop()
			if err := f.Close(); err != nil {
				log.Warn("Failed to close trace file; trace data may be incomplete", "file", traceFile, "err", err)
			}
			log.Info("Trace saved", "file", traceFile)
		})
	}

	// Start HTTP profiling server
	if cfg.HTTPPort > 0 {
		addr := fmt.Sprintf("localhost:%d", cfg.HTTPPort)
		mux := newProfilingMux()

		srv := &http.Server{Addr: addr, Handler: mux}

		go func() {
			log.Info("Profiling server started", "url", fmt.Sprintf("http://%s/debug/pprof/", addr))
			log.Info("  - Goroutines", "url", fmt.Sprintf("http://%s/debug/pprof/goroutine?debug=1", addr))
			log.Info("  - Heap", "url", fmt.Sprintf("http://%s/debug/pprof/heap", addr))
			log.Info("  - Block", "url", fmt.Sprintf("http://%s/debug/pprof/block?debug=1", addr))
			log.Info("  - Mutex", "url", fmt.Sprintf("http://%s/debug/pprof/mutex?debug=1", addr))
			log.Info("  - CPU", "cmd", fmt.Sprintf("curl http://%s/debug/pprof/profile?seconds=30 > cpu.prof", addr))
			log.Info("  - Fork pressure", "url", fmt.Sprintf("http://%s/debug/fork-pressure", addr))
			log.Info("  - Blob cache", "url", fmt.Sprintf("http://%s/debug/blob-cache", addr))
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Error("Profiling server error", "err", err)
			}
		}()

		cleanupFuncs = append(cleanupFuncs, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				log.Error("Error shutting down profiling server", "err", err)
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

// newProfilingMux serves /debug/pprof/* (cumulative and delta_* variants,
// registered on http.DefaultServeMux by the pprof blank imports) plus the
// app's own debug endpoints.
func newProfilingMux() *http.ServeMux {
	mux := http.NewServeMux()
	// pprof handlers live on http.DefaultServeMux; forward /debug/pprof/ there.
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

	// Blob-cache effectiveness endpoint — see unfinished.BlobCacheStatsSnapshot.
	// Registered before the scanner exists (this server starts early in
	// main.go); safe because the reader lookup only happens per-request.
	mux.HandleFunc("/debug/blob-cache", func(w http.ResponseWriter, r *http.Request) {
		s := unfinished.BlobCacheStatsSnapshot()
		total := s.Hits + s.Misses
		var hitRate float64
		if total > 0 {
			hitRate = float64(s.Hits) / float64(total)
		}
		// repo_cache_* fields root-cause a low hit_rate: blobCache lives inside
		// *cachedRepo, so if repo_cache_evictions tracks repo_cache_cold_opens
		// closely, repo evictions (not a low same-blob-revisit workload) are
		// wiping the blob cache before it can warm up. See RepoCacheStats' doc
		// comment.
		rc := unfinished.RepoCacheStatsSnapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits":                    s.Hits,
			"misses":                  s.Misses,
			"hit_rate":                hitRate,
			"estimated_time_saved_ms": s.EstimatedTimeSaved.Milliseconds(),
			"repo_cache_current_size": rc.CurrentSize,
			"repo_cache_cold_opens":   rc.ColdOpens,
			"repo_cache_evictions":    rc.Evictions,
		})
	})
	return mux
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

			// Alert on goroutine leak
			if count > 100 {
				log.Error("high goroutine count detected", "count", count)
			}
		}
	}
}
