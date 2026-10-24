package server

import (
	"net/http"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
)

// probeProcedurePath is the full request path of ProbeProgram as seen by the
// outer handler, before the mux strips the "/api" prefix.
const probeProcedurePath = "/api" + sessionv1connect.SessionServiceProbeProgramProcedure

// nudgeProcedurePath is the full request path of the write-capable
// NudgeSessionForPR, guarded like ProbeProgram on the unauthenticated listener.
const nudgeProcedurePath = "/api" + sessionv1connect.GitHubUserServiceNudgeSessionForPRProcedure

// pruneProcedurePath is the full request path of PruneHiddenSessionNotifications,
// guarded whole (dry run and apply) with the rebinding profile.
const pruneProcedurePath = "/api" + sessionv1connect.SessionServicePruneHiddenSessionNotificationsProcedure

// updateFlagProcedurePath is the full request path of UpdateFeatureFlag, which
// flips protections (read-only guards, the delivery gate), guarded like prune.
const updateFlagProcedurePath = "/api" + sessionv1connect.SessionServiceUpdateFeatureFlagProcedure

// replyProcedurePath is the full request path of ReplyToPendingQuestion, the
// audited write into a hidden session's question dialog, guarded with the
// rebinding profile (the in-handler local-caller gate adds the peer check).
const replyProcedurePath = "/api" + sessionv1connect.SessionServiceReplyToPendingQuestionProcedure

// writeFileProcedurePath is the full request path of WriteFile, the save primitive of the
// Files tab: an unauthenticated :8543 listener must give it the POST-only and
// Origin/rebinding verdict like every other write RPC (ADR-031).
const writeFileProcedurePath = "/api" + sessionv1connect.SessionServiceWriteFileProcedure

// guardedProcedures is the LocalWriteGuard set. A new member names its profile;
// ProbeProgram and the nudge keep the original probe verdict byte for byte.
var guardedProcedures = map[string]middleware.GuardProfile{
	probeProcedurePath: middleware.ProfileProbe,
	nudgeProcedurePath: middleware.ProfileProbe,
	pruneProcedurePath: middleware.ProfileRebinding,

	updateFlagProcedurePath: middleware.ProfileRebinding,
	replyProcedurePath:      middleware.ProfileRebinding,

	writeFileProcedurePath: middleware.ProfileRebinding,

	// Config writes that change engine or gate behavior on the unauthenticated
	// local listener (Task 2.1.1e). Reads (Get*/List*) stay unguarded.
	"/api" + sessionv1connect.QuotaServiceUpdateQuotaConfigProcedure:       middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceCreateStageProcedure:           middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceUpdateStageProcedure:           middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceDeleteStageProcedure:           middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceCreateStageTransitionProcedure: middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceUpdateStageTransitionProcedure: middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceDeleteStageTransitionProcedure: middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceCreateTransitionGateProcedure:  middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceUpdateTransitionGateProcedure:  middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceDeleteTransitionGateProcedure:  middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceCreatePipelineModeProcedure:    middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceUpdatePipelineModeProcedure:    middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceDeletePipelineModeProcedure:    middleware.ProfileRebinding,
	"/api" + sessionv1connect.BacklogServiceRecordGateApprovalProcedure:    middleware.ProfileRebinding,
}

// writeFileBodyLimit caps a WriteFile body: 1 MiB of content grows to about 1.34 MiB as
// base64 JSON, plus the path, token and framing (ADR-031 "Request-size cap").
const writeFileBodyLimit = 1536 * 1024

// fileReadRequestBodyLimit caps the small request messages of the read-side file RPCs, inflated size included.
const fileReadRequestBodyLimit = 256 * 1024

// withFileBodyLimits wraps the session Connect handler so both listeners share the
// WriteFile body cap. It sits around the handler, not beside guardedProcedures: that map
// feeds only localChainWith, so a limit registered there would leave :8444 unbounded.
// The path is the one the handler sees (the mux has already stripped "/api").
func withFileBodyLimits(h http.Handler) http.Handler {
	h = middleware.BodyLimit(sessionv1connect.SessionServiceWriteFileProcedure, writeFileBodyLimit)(h)
	for _, procedure := range []string{
		sessionv1connect.SessionServiceReadFileRangeProcedure,
		sessionv1connect.SessionServiceStatFileProcedure,
		sessionv1connect.SessionServiceGetFileTransportCapabilitiesProcedure,
	} {
		h = middleware.BodyLimit(procedure, fileReadRequestBodyLimit)(h)
	}
	return h
}

// localChain is the :8543 middleware chain (inside otelhttp):
// Logging -> CORS -> Compress -> HostGuard -> [auth | ProbeGuard] -> mux.
// HostGuard covers every route (and WebSocket upgrade), so a rebinding or
// reverse-proxy Host never reaches a handler.
// The listener has no auth unless authMiddleware is set, so ProbeGuard is the
// boundary for the RPCs that execute a program or write to a session's terminal
// (ProbeProgram, NudgeSessionForPR); with auth, auth is the boundary.
func (s *Server) localChain() http.Handler {
	return s.localChainWith(guardedProcedures)
}

// localChainWith builds localChain over an explicit guard set so tests can
// exercise a profile before a real procedure registers in it.
func (s *Server) localChainWith(procedures map[string]middleware.GuardProfile) http.Handler {
	inner := http.Handler(s)
	if s.authMiddleware != nil {
		inner = s.authMiddleware(inner)
	} else {
		inner = middleware.LocalWriteGuard(procedures, middleware.LocalWriteGuardConfig{
			Probe:     s.probeGuardConfig(),
			Rebinding: s.LocalWriteVerdictConfig(),
		})(inner)
	}
	inner = middleware.HostGuard(s.hostGuardConfig())(inner)
	return s.stampRequest(services.ListenerLocal, s.requiresAuth)(
		middleware.Logging(middleware.CORSWithOrigins(s.origins)(middleware.Compress(inner))))
}
