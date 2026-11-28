package services

// Delegators for the RPCs about the host this server runs on: the capture tap,
// TCC enrichment, the tmux server and the global CLI hooks.

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/antigravityexport"
	"github.com/tstapler/stapler-squad/server/services/capturetap"
	"github.com/tstapler/stapler-squad/server/services/hookinstall"
	"github.com/tstapler/stapler-squad/server/services/tccrpc"
	"github.com/tstapler/stapler-squad/server/services/tmuxadmin"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
)

// SetCaptureTap turns the terminal-stream capture tap on or off.
func (s *SessionService) SetCaptureTap(ctx context.Context, req *connect.Request[sessionv1.SetCaptureTapRequest]) (*connect.Response[sessionv1.SetCaptureTapResponse], error) {
	return capturetap.Service{Registry: s.tapRegistry}.SetCaptureTap(ctx, req)
}

// GetCaptureTap reports the capture tap's state.
func (s *SessionService) GetCaptureTap(ctx context.Context, req *connect.Request[sessionv1.GetCaptureTapRequest]) (*connect.Response[sessionv1.GetCaptureTapResponse], error) {
	return capturetap.Service{Registry: s.tapRegistry}.GetCaptureTap(ctx, req)
}

// SetTCCEnrichment wires the startup-owned gate before serving requests.
func (s *SessionService) SetTCCEnrichment(gate *tcc.ConsentGate, enricher *instance.WorktreeEnricher, enabled bool) {
	s.tccEnrichment = tccrpc.NewState(gate, enricher, enabled)
}

// GetTCCEnrichmentStatus reports the enrichment consent state per protected root.
func (s *SessionService) GetTCCEnrichmentStatus(ctx context.Context, req *connect.Request[sessionv1.GetTCCEnrichmentStatusRequest]) (*connect.Response[sessionv1.GetTCCEnrichmentStatusResponse], error) {
	var live func() []*instance.Instance
	if s.ReviewQueuePoller != nil {
		live = s.ReviewQueuePoller.GetInstances
	}
	return s.tccEnrichment.GetTCCEnrichmentStatus(ctx, req, live)
}

// ContinueTCCEnrichment records consent for a protected root and resumes its probes.
func (s *SessionService) ContinueTCCEnrichment(ctx context.Context, req *connect.Request[sessionv1.ContinueTCCEnrichmentRequest]) (*connect.Response[sessionv1.ContinueTCCEnrichmentResponse], error) {
	return s.tccEnrichment.ContinueTCCEnrichment(ctx, req)
}

// GetTmuxVersionStatus reports detected tmux client/server version mismatches.
func (s *SessionService) GetTmuxVersionStatus(ctx context.Context, req *connect.Request[sessionv1.GetTmuxVersionStatusRequest]) (*connect.Response[sessionv1.GetTmuxVersionStatusResponse], error) {
	return tmuxadmin.GetTmuxVersionStatus(ctx, req)
}

// RestartTmuxServer kills the tmux server on a socket. DESTRUCTIVE: see tmuxadmin.RestartTmuxServer.
func (s *SessionService) RestartTmuxServer(ctx context.Context, req *connect.Request[sessionv1.RestartTmuxServerRequest]) (*connect.Response[sessionv1.RestartTmuxServerResponse], error) {
	return tmuxadmin.RestartTmuxServer(ctx, req)
}

// GetHookStatus reports whether the global CLI hooks are installed.
func (s *SessionService) GetHookStatus(ctx context.Context, req *connect.Request[sessionv1.GetHookStatusRequest]) (*connect.Response[sessionv1.GetHookStatusResponse], error) {
	return s.hookInstaller().GetHookStatus(ctx, req)
}

// InstallHooks installs the requested global CLI hooks.
func (s *SessionService) InstallHooks(ctx context.Context, req *connect.Request[sessionv1.InstallHooksRequest]) (*connect.Response[sessionv1.InstallHooksResponse], error) {
	return s.hookInstaller().InstallHooks(ctx, req)
}

func (s *SessionService) hookInstaller() hookinstall.Service {
	st, ok := s.storage.(*session.Storage)
	if !ok {
		return hookinstall.Service{}
	}
	return hookinstall.Service{ExportAntigravityRules: func(ctx context.Context) error {
		return antigravityexport.ExportAntigravityRulesFromDB(ctx, st)
	}}
}
