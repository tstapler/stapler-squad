package services

import (
	"sort"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

// wsUnaryAllowed is the server-side deny-by-default table of procedures the
// unary WebSocket bridge will dispatch (reads only). It mirrors the client's
// wsEligible column in web-app/src/lib/transport/__fixtures__/ws-unary-allowlist.json;
// the client policy is never the only gate. Keys are full procedure paths.
var wsUnaryAllowed = map[string]struct{}{
	sessionv1connect.SessionServiceGetVCSStatusProcedure:                 {},
	sessionv1connect.SessionServiceGetSessionDiffProcedure:               {},
	sessionv1connect.SessionServiceListFilesProcedure:                    {},
	sessionv1connect.SessionServiceGetFileContentProcedure:               {},
	sessionv1connect.SessionServiceStatFileProcedure:                     {},
	sessionv1connect.SessionServiceGetFileTransportCapabilitiesProcedure: {},
	sessionv1connect.SessionServiceListShellsProcedure:                   {},
	sessionv1connect.SessionServiceListWorkflowsProcedure:                {},
	sessionv1connect.SessionServiceListProgramsConfigProcedure:           {},
	sessionv1connect.SessionServiceListPendingApprovalsProcedure:         {},
	sessionv1connect.SessionServiceGetNotificationHistoryProcedure:       {},
	sessionv1connect.SessionServiceGetTmuxVersionStatusProcedure:         {},
	sessionv1connect.SessionServiceGetSessionDefaultsProcedure:           {},
	sessionv1connect.SessionServiceListProjectsProcedure:                 {},
	sessionv1connect.SessionServiceGetReviewQueueProcedure:               {},
	sessionv1connect.SessionServiceGetSessionProcedure:                   {},
	sessionv1connect.BacklogServiceListStuckBacklogItemsProcedure:        {},
}

// wsUnaryClientHeaders is the REQUEST meta.headers allowlist (ADR-001); any other
// key fails closed with invalid_argument.
var wsUnaryClientHeaders = []string{"Connect-Protocol-Version"}

// WsUnaryAllowedMethods returns the allowed procedures as full paths
// ("/session.v1.SessionService/ListFiles"), sorted.
func WsUnaryAllowedMethods() []string {
	out := make([]string, 0, len(wsUnaryAllowed))
	for m := range wsUnaryAllowed {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func wsUnaryMethodAllowed(method string) bool {
	_, ok := wsUnaryAllowed[method]
	return ok
}
