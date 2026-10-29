package server

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/gitwiring"
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
	s.mux.HandleFunc("/actuator/db", handleActuatorDB)
	s.mux.HandleFunc("/actuator/git-backend", handleActuatorGitBackend)
	s.mux.HandleFunc("/actuator/workflow-engine", s.handleActuatorWorkflowEngine)
	s.mux.HandleFunc("/actuator/nudge-outcomes", handleActuatorNudgeOutcomes)
	s.mux.HandleFunc("/actuator/unfinished-scanner", handleActuatorUnfinishedScanner(s.unfinishedScanner))
	s.mux.HandleFunc("/actuator/inbox-pr-watch", handleActuatorInboxPRWatch)
	s.mux.HandleFunc("/actuator/ws-unary", handleActuatorWsUnary(s.wsUnaryBridge.Load))
}

// handleActuatorInboxPRWatch serves the PR watch status snapshot (D14). JSON by default;
// ?format=text for a terminal. With no provider it answers "not running" (never 404).
func handleActuatorInboxPRWatch(w http.ResponseWriter, r *http.Request) {
	provider := currentPRWatchStatusProvider()
	text := r.URL.Query().Get("format") == "text"
	if text {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	if provider == nil {
		if text {
			_, _ = w.Write([]byte(prwatch.NotRunningText()))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": false})
		return
	}
	st := provider()
	if text {
		host, _ := os.Hostname()
		_, _ = w.Write([]byte(prwatch.RenderText(st, time.Now(), host)))
		return
	}
	_ = json.NewEncoder(w).Encode(st)
}

// handleActuatorDB serves per-pool sql.DBStats and query counters (session/dbobs).
// With the single-connection pool, wait_count/wait_duration_ms rising is DB queueing.
func handleActuatorDB(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"pools": dbobs.Snapshot()})
}

// handleActuatorNudgeOutcomes serves the durable nudge_outcome_total counters
// and the time of the first record (Story 4.1.2).
func handleActuatorNudgeOutcomes(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session.DefaultNudgeOutcomes().Snapshot())
}

// handleActuatorWsUnary serves the unary WebSocket bridge counters as JSON. The bridge is
// resolved per request because it may be built after the routes are registered.
func handleActuatorWsUnary(bridge func() *services.UnaryWsBridge) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bridge().Report())
	}
}

// handleActuatorUnfinishedScanner serves the unfinished-work scanner's counters as JSON, read
// from atomics, so soak decisions need neither OpenTelemetry nor a profile
// (docs/how-to/tune-unfinished-scanner-caches.md).
func handleActuatorUnfinishedScanner(s *unfinished.Scanner) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(unfinished.BuildReport(s))
	}
}

// handleActuatorGitBackend serves the git backend shadow-soak counters and the mechanical flip
// verdict (docs/how-to/flip-git-backend-cohort.md). JSON by default; ?format=text for a terminal.
func handleActuatorGitBackend(w http.ResponseWriter, r *http.Request) {
	writeGitBackendSoak(w, r, gitwiring.SoakStatus(r.Context()))
}

func writeGitBackendSoak(w http.ResponseWriter, r *http.Request, report gitwiring.SoakReport) {
	w.Header().Set("X-Git-Soak-Ready", strconv.FormatBool(report.Ready()))
	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(report.Text()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

type actuatorComponent struct {
	Status string `json:"status"`
	Detail any    `json:"detail,omitempty"`
}

func handleActuatorHealth(w http.ResponseWriter, _ *http.Request) {
	writeActuatorHealth(w, tmux.ForkPressureSnapshot().Level, runtime.NumGoroutine())
}

// writeActuatorHealth takes the process-global inputs as parameters so tests can
// pin them instead of depending on package-wide goroutine count or fork pressure.
func writeActuatorHealth(w http.ResponseWriter, forkLevel tmux.ForkPressureLevel, goroutines int) {
	components := map[string]actuatorComponent{
		"fork_pressure": {
			Status: forkPressureHealthStatus(forkLevel),
			Detail: forkLevel.String(),
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
		"workflow_engine_bypass_total": session.EngineBypassSnapshot(),
		"db":                           dbobs.Snapshot(),
		"runtime": map[string]any{
			"goroutines":    runtime.NumGoroutine(),
			"heap_inuse_mb": float64(m.HeapInuse) / 1024 / 1024,
			"heap_alloc_mb": float64(m.HeapAlloc) / 1024 / 1024,
		},
	})
}
