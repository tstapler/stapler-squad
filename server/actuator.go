package server

import (
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// highGoroutineCountThreshold mirrors profiling.MonitorGoroutines' existing
// alert threshold — reused here as a health signal rather than invented
// fresh, so "degraded" in /actuator/health means the same thing an operator
// already sees logged as a warning.
const highGoroutineCountThreshold = 100

// registerActuatorRoutes wires a small actuator-style surface (inspired by
// Spring Boot's /actuator/health, /actuator/metrics): a machine-checkable
// health rollup and a flat metrics snapshot, always available on the main
// server (unlike /debug/blob-cache and friends, which only exist when the
// process is started with --profile). Both read the exact same snapshot
// functions the OTel instruments use (session/unfinished.RegisterMetrics,
// tmux.ForkPressureSnapshot) — there is one source of truth per metric, this
// just gives it a second, always-on export shape for curl/uptime checks.
func (s *Server) registerActuatorRoutes() {
	s.mux.HandleFunc("/actuator/health", handleActuatorHealth)
	s.mux.HandleFunc("/actuator/metrics", handleActuatorMetrics)
}

type actuatorComponent struct {
	Status string `json:"status"`
	Detail any    `json:"detail,omitempty"`
}

func handleActuatorHealth(w http.ResponseWriter, _ *http.Request) {
	fp := tmux.ForkPressureSnapshot()
	goroutines := runtime.NumGoroutine()

	components := map[string]actuatorComponent{
		"fork_pressure": {
			Status: forkPressureHealthStatus(fp.Level),
			Detail: fp.Level.String(),
		},
		"runtime": {
			Status: goroutineHealthStatus(goroutines),
			Detail: map[string]any{"goroutines": goroutines},
		},
	}

	overall := "ok"
	httpStatus := http.StatusOK
	for _, c := range components {
		switch c.Status {
		case "critical":
			overall = "critical"
			httpStatus = http.StatusServiceUnavailable
		case "degraded":
			if overall == "ok" {
				overall = "degraded"
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     overall,
		"components": components,
	})
}

func forkPressureHealthStatus(level tmux.ForkPressureLevel) string {
	switch level {
	case tmux.ForkPressureCritical:
		return "critical"
	case tmux.ForkPressureWarning:
		return "degraded"
	default:
		return "ok"
	}
}

func goroutineHealthStatus(count int) string {
	if count > highGoroutineCountThreshold {
		return "degraded"
	}
	return "ok"
}

func handleActuatorMetrics(w http.ResponseWriter, _ *http.Request) {
	blobCache := unfinished.BlobCacheStatsSnapshot()
	blobCacheTotal := blobCache.Hits + blobCache.Misses
	var blobCacheHitRate float64
	if blobCacheTotal > 0 {
		blobCacheHitRate = float64(blobCache.Hits) / float64(blobCacheTotal)
	}

	fp := tmux.ForkPressureSnapshot()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"blob_cache": map[string]any{
			"hits":                    blobCache.Hits,
			"misses":                  blobCache.Misses,
			"hit_rate":                blobCacheHitRate,
			"estimated_time_saved_ms": blobCache.EstimatedTimeSaved.Milliseconds(),
		},
		"fork_pressure": map[string]any{
			"level":              fp.Level.String(),
			"spawns_in_window":   fp.SpawnsInWindow,
			"failures_in_window": fp.FailuresInWindow,
			"zombies_in_window":  fp.ZombiesInWindow,
			"total_spawns":       fp.TotalSpawns,
			"total_failures":     fp.TotalFailures,
			"total_zombies":      fp.TotalZombies,
		},
		"runtime": map[string]any{
			"goroutines":    runtime.NumGoroutine(),
			"heap_inuse_mb": float64(m.HeapInuse) / 1024 / 1024,
			"heap_alloc_mb": float64(m.HeapAlloc) / 1024 / 1024,
		},
	})
}
