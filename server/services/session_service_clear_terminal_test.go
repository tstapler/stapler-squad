package services

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

func newClearTerminalService(insts ...*session.Instance) *SessionService {
	poller := newEmptyPoller()
	poller.SetInstances(insts)
	return &SessionService{reviewQueuePoller: poller, eventBus: events.NewEventBus(8)}
}

func probeOf(live bool, probed *bool) func(*session.Instance) func() bool {
	return func(*session.Instance) func() bool {
		return func() bool {
			*probed = true
			return live
		}
	}
}

func TestClearTerminalStatusOne_Outcomes(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, tc := range []struct {
		name      string
		inst      *session.Instance
		live      bool
		want      string
		wantProbe bool
		want0     session.Status
	}{
		{"not live is untouched", &session.Instance{Title: "x", Status: session.PermanentlyFailed}, false, "not_live", true, session.PermanentlyFailed},
		{"archived is rejected without probing", &session.Instance{Title: "x", Status: session.Failed, ArchivedAt: &now}, true, "archived", false, session.Failed},
		{"active is not terminal", &session.Instance{Title: "x", Status: session.Active}, true, "not_terminal", false, session.Active},
		// No process manager is wired, so the hot attach itself fails: proves the
		// live seam reached Start rather than short-circuiting.
		{"live reaches hot attach", &session.Instance{Title: "x", Status: session.Failed}, true, "error", true, session.Creating},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newClearTerminalService(tc.inst)
			probed := false
			res := svc.clearTerminalStatusOne("x", probeOf(tc.live, &probed))
			assert.Equal(t, tc.want, res.Outcome, res.Message)
			assert.Equal(t, tc.wantProbe, probed)
			if tc.want != "error" {
				assert.Equal(t, tc.want0, tc.inst.GetLifecycleStatus())
			}
		})
	}
}

func TestClearTerminalStatusOne_UnknownSession(t *testing.T) {
	t.Parallel()
	probed := false
	res := newClearTerminalService().clearTerminalStatusOne("nope", probeOf(true, &probed))
	assert.Equal(t, "not_found", res.Outcome)
	assert.False(t, probed)
}

func TestClearTerminalStatusIfLive_RPCValidation(t *testing.T) {
	t.Parallel()
	svc := newClearTerminalService(&session.Instance{Title: "dead", Status: session.PermanentlyFailed})

	_, err := svc.ClearTerminalStatusIfLive(t.Context(), connect.NewRequest(&sessionv1.ClearTerminalStatusIfLiveRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = svc.ClearTerminalStatusIfLive(t.Context(), connect.NewRequest(&sessionv1.ClearTerminalStatusIfLiveRequest{SessionIds: []string{"missing"}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	// Batch: per-session outcomes, no error even when nothing clears.
	resp, err := svc.ClearTerminalStatusIfLive(t.Context(), connect.NewRequest(&sessionv1.ClearTerminalStatusIfLiveRequest{SessionIds: []string{"missing", "dead"}}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 2)
	assert.Equal(t, "not_found", resp.Msg.Results[0].Outcome)
	assert.NotEqual(t, "cleared", resp.Msg.Results[1].Outcome)
}
