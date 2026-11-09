package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// terminalLatencyReport is the /actuator/terminal-latency body: rolling (5 min) percentiles per
// stage, backend (tmux/tymux) and listener (local/remote), server- and browser-measured alike.
type terminalLatencyReport struct {
	Note     string          `json:"note"`
	Alerting []termlat.Stats `json:"alerting"`
	Stages   []termlat.Stats `json:"stages"`
	// Resync counts resync requests and client-side skips by reason and backend since start.
	Resync []termlat.ResyncCount `json:"resync"`
	// Drops counts output frames dropped on a full queue by cause and backend since start.
	Drops []termlat.DropCount `json:"drops"`
}

const terminalLatencyNote = "ms for duration stages, counts for *_depth stages; p50/p95/p99 over the last 5 minutes, max since start. " +
	"echo_* and ttfp_* are browser-measured; the rest are server-side. " +
	"alerting lists stages whose p95 is over its limit (echo_paint: " + termlat.EnvEchoP95Ms + ", default 150 ms; input_total: 100 ms)."

// handleActuatorTerminalLatency serves the rolling terminal latency report. JSON by default;
// ?format=text for a terminal. Read-only, aggregate data with no session identifiers.
func handleActuatorTerminalLatency(rec *termlat.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rep := terminalLatencyReport{Note: terminalLatencyNote, Alerting: rec.Alerting(), Stages: rec.Report(), Resync: rec.ResyncCounts(), Drops: rec.DropCounts()}
		if rep.Alerting == nil {
			rep.Alerting = []termlat.Stats{}
		}
		if r.URL.Query().Get("format") == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(formatTerminalLatency(rep)))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	}
}

func formatTerminalLatency(rep terminalLatencyReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %-7s %-8s %6s %9s %9s %9s %9s\n", "stage", "backend", "listener", "n", "p50", "p95", "p99", "max")
	for _, s := range rep.Stages {
		fmt.Fprintf(&b, "%-20s %-7s %-8s %6d %9.1f %9.1f %9.1f %9.1f\n", s.Stage, s.Backend, s.Listener, s.Count, s.P50, s.P95, s.P99, s.Max)
	}
	for _, r := range rep.Resync {
		fmt.Fprintf(&b, "resync %-20s %-7s %8d\n", r.Reason, r.Backend, r.Total)
	}
	for _, d := range rep.Drops {
		fmt.Fprintf(&b, "drop %-20s %-7s %8d\n", d.Cause, d.Backend, d.Total)
	}
	for _, s := range rep.Alerting {
		fmt.Fprintf(&b, "ALERT: %s %s/%s p95=%.1f\n", s.Stage, s.Backend, s.Listener, s.P95)
	}
	return b.String()
}
