package services

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/instance"
)

const (
	clearTerminalOutcomeNotFound = "not_found"
	clearTerminalOutcomeReadOnly = "read_only"
	clearTerminalOutcomeCanceled = "canceled"
	maxClearTerminalBatch        = 100
)

// +api: session:clear-terminal-status-if-live
// ClearTerminalStatusIfLive hot-attaches sessions that are terminal in the DB
// but whose backing process is still alive, sharing session.HotRestoreIfLive
// with boot reconcile. A single-session request that cannot be cleared fails
// with FailedPrecondition; batches report per-session outcomes.
func (s *SessionService) ClearTerminalStatusIfLive(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearTerminalStatusIfLiveRequest],
) (*connect.Response[sessionv1.ClearTerminalStatusIfLiveResponse], error) {
	ids := req.Msg.GetSessionIds()
	if len(ids) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_ids is required"))
	}
	if len(ids) > maxClearTerminalBatch {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at most %d session_ids per request", maxClearTerminalBatch))
	}
	results := make([]*sessionv1.ClearTerminalStatusIfLiveResult, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_ids must not contain empty ids"))
		}
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			results = append(results, &sessionv1.ClearTerminalStatusIfLiveResult{
				SessionId: id, Outcome: clearTerminalOutcomeCanceled, Message: "request canceled before this session was processed",
			})
			continue
		}
		results = append(results, s.clearTerminalStatusOne(id, func(inst *instance.Instance) func() bool {
			return inst.IsLiveOrStoredSessionLive
		}))
	}
	log.Info("audit: ClearTerminalStatusIfLive batch", "requested", len(ids), "summary", summarizeClearTerminal(results))
	if len(results) == 1 && results[0].Outcome != string(instance.HotRestoreCleared) {
		r := results[0]
		code := connect.CodeFailedPrecondition
		switch r.Outcome {
		case clearTerminalOutcomeNotFound:
			code = connect.CodeNotFound
		case string(instance.HotRestoreError):
			code = connect.CodeInternal
		}
		return nil, connect.NewError(code, fmt.Errorf("%s: %s", r.Outcome, r.Message))
	}
	return connect.NewResponse(&sessionv1.ClearTerminalStatusIfLiveResponse{Results: results}), nil
}

// clearTerminalStatusOne handles one id; liveProbe is the test seam for liveness.
func (s *SessionService) clearTerminalStatusOne(id string, liveProbe func(*instance.Instance) func() bool) *sessionv1.ClearTerminalStatusIfLiveResult {
	res := &sessionv1.ClearTerminalStatusIfLiveResult{SessionId: id}
	// Managed instances only: findInstance's external-discovery fallback would let
	// an external session reach Start(false).
	inst := findInstanceByID(s.allInstances(), id)
	if inst == nil {
		res.Outcome, res.Message = clearTerminalOutcomeNotFound, "session not found"
		log.Info("audit: ClearTerminalStatusIfLive", "session", id, "outcome", res.Outcome)
		return res
	}
	// Same hidden/background-session guard RestartSession uses: a revival is a UI write.
	if _, err := AccessForUnary(inst, s.guards).Writer(nil); err != nil {
		res.Outcome, res.Message = clearTerminalOutcomeReadOnly, "session is a background session and is read-only"
		log.Info("audit: ClearTerminalStatusIfLive", "session", inst.Title, "uuid", inst.UUID, "outcome", res.Outcome)
		return res
	}
	statusBefore := inst.GetLifecycleStatus()
	outcome, err := instance.HotRestoreIfLive(inst, liveProbe(inst))
	res.Outcome = string(outcome)
	switch outcome {
	case instance.HotRestoreCleared:
		res.Message = fmt.Sprintf("cleared %s; attached to live process", statusBefore)
		s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))
	case instance.HotRestoreNotLive:
		res.Message = "backing process is not alive; session left untouched"
	case instance.HotRestoreArchived:
		res.Message = "session is archived; left untouched"
	case instance.HotRestoreBusy:
		res.Message = "a restart is already in flight; try again shortly"
	case instance.HotRestoreNotTerminal:
		res.Message = fmt.Sprintf("status %s is not terminal", statusBefore)
	default:
		res.Message = "hot restore failed"
		if err != nil {
			res.Message = err.Error()
		}
	}
	log.Info("audit: ClearTerminalStatusIfLive", "session", inst.Title, "uuid", inst.UUID,
		"status_before", statusBefore.String(), "outcome", res.Outcome, "err", err)
	return res
}

// summarizeClearTerminal counts results per outcome for the per-call audit line.
func summarizeClearTerminal(results []*sessionv1.ClearTerminalStatusIfLiveResult) map[string]int {
	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Outcome]++
	}
	return counts
}
