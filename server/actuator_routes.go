package server

import (
	"net/http"

	"github.com/tstapler/stapler-squad/internal/termlat"
	"github.com/tstapler/stapler-squad/server/actuator"
	"github.com/tstapler/stapler-squad/server/prwatchhost"
	"github.com/tstapler/stapler-squad/server/services/access"
)

// registerActuatorRoutes wires a small actuator-style surface (inspired by
// Spring Boot's /actuator/health, /actuator/metrics): a machine-checkable
// health rollup and a flat metrics snapshot, always available on the main
// server (unlike /debug/blob-cache and friends, which only exist when the
// process is started with --profile). Both read the exact same snapshot
// functions the OTel instruments use (session/unfinished.RegisterMetrics,
// tmux.ForkPressureSnapshot) — there is one source of truth per metric, this
// just gives it a second, always-on export shape for curl/uptime checks.
func (s *Server) registerActuatorRoutes() {
	for _, e := range s.actuatorEndpoints() {
		s.mux.Handle(e.Path, e.Handler)
	}
}

// actuatorEndpoint is one row of the actuator route table.
type actuatorEndpoint struct {
	Path    string
	Handler http.Handler
}

// actuatorEndpoints is the single list of actuator routes: a new endpoint is a
// handler file plus one row here. Row order is registration order. Every row is
// unconditional: a missing dependency is answered by the handler (503, "not
// running"), never by leaving the route unregistered.
func (s *Server) actuatorEndpoints() []actuatorEndpoint {
	return []actuatorEndpoint{
		{"/actuator/health", http.HandlerFunc(actuator.Health)},
		{"/actuator/metrics", http.HandlerFunc(actuator.Metrics)},
		{"/actuator/db", http.HandlerFunc(actuator.DB)},
		{"/actuator/git-backend", http.HandlerFunc(actuator.GitBackend)},
		{"/actuator/workflow-engine", http.HandlerFunc(s.handleActuatorWorkflowEngine)},
		{"/actuator/nudge-outcomes", http.HandlerFunc(actuator.NudgeOutcomes)},
		{"/actuator/unfinished-scanner", actuator.UnfinishedScanner(s.unfinishedScanner)},
		{"/actuator/inbox-pr-watch", actuator.InboxPRWatch(prwatchhost.CurrentStatusProvider)},
		{"/actuator/ws-unary", actuator.WsUnary(s.wsUnaryBridge.Load)},
		{"/actuator/terminal-mux", actuator.TerminalMux(s.terminalMuxBridge.Load)},
		{"/actuator/terminal-input", actuator.TerminalInput(access.TerminalInputReport)},
		{"/actuator/terminal-latency", actuator.TerminalLatency(termlat.Default)},
	}
}

// SetWorkflowEngineSoakSource wires the actuator's soak data.
func (s *Server) SetWorkflowEngineSoakSource(src *actuator.WorkflowEngineSoakSource) {
	s.workflowEngineSoak.Store(src)
}

func (s *Server) handleActuatorWorkflowEngine(w http.ResponseWriter, r *http.Request) {
	actuator.ServeWorkflowEngineSoak(w, r, s.workflowEngineSoak.Load())
}
