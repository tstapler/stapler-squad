package services

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

const clearTerminalOutcomeNotFound = "not_found"

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
	results := make([]*sessionv1.ClearTerminalStatusIfLiveResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, s.clearTerminalStatusOne(id, func(inst *session.Instance) func() bool {
			return inst.IsLiveOrStoredSessionLive
		}))
	}
	if len(results) == 1 && results[0].Outcome != string(session.HotRestoreCleared) {
		r := results[0]
		code := connect.CodeFailedPrecondition
		switch r.Outcome {
		case clearTerminalOutcomeNotFound:
			code = connect.CodeNotFound
		case string(session.HotRestoreError):
			code = connect.CodeInternal
		}
		return nil, connect.NewError(code, fmt.Errorf("%s: %s", r.Outcome, r.Message))
	}
	return connect.NewResponse(&sessionv1.ClearTerminalStatusIfLiveResponse{Results: results}), nil
}

// clearTerminalStatusOne handles one id; liveProbe is the test seam for liveness.
func (s *SessionService) clearTerminalStatusOne(id string, liveProbe func(*session.Instance) func() bool) *sessionv1.ClearTerminalStatusIfLiveResult {
	res := &sessionv1.ClearTerminalStatusIfLiveResult{SessionId: id}
	inst := findInstanceByID(s.allInstances(), id)
	if inst == nil {
		inst = s.findInstance(id)
	}
	if inst == nil {
		res.Outcome, res.Message = clearTerminalOutcomeNotFound, "session not found"
		log.Info("audit: ClearTerminalStatusIfLive", "session", id, "outcome", res.Outcome)
		return res
	}
	statusBefore := inst.GetLifecycleStatus()
	outcome, err := session.HotRestoreIfLive(inst, liveProbe(inst))
	res.Outcome = string(outcome)
	switch outcome {
	case session.HotRestoreCleared:
		res.Message = fmt.Sprintf("cleared %s; attached to live process", statusBefore)
		s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))
	case session.HotRestoreNotLive:
		res.Message = "backing process is not alive; session left untouched"
	case session.HotRestoreArchived:
		res.Message = "session is archived; left untouched"
	case session.HotRestoreNotTerminal:
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
