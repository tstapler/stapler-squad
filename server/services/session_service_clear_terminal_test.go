package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

func newClearTerminalService(insts ...*instance.Instance) *SessionService {
	poller := svctest.NewEmptyPoller()
	poller.SetInstances(insts)
	return &SessionService{ReviewQueuePoller: poller, eventBus: events.NewEventBus(8)}
}

func probeOf(live bool, probed *bool) func(*instance.Instance) func() bool {
	return func(*instance.Instance) func() bool {
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
		inst      *instance.Instance
		live      bool
		want      string
		wantProbe bool
		want0     domain.Status
	}{
		{"not live is untouched", &instance.Instance{Title: "x", Status: domain.PermanentlyFailed}, false, "not_live", true, domain.PermanentlyFailed},
		{"archived is rejected without probing", &instance.Instance{Title: "x", Status: domain.Failed, ArchivedAt: &now}, true, "archived", false, domain.Failed},
		{"active is not terminal", &instance.Instance{Title: "x", Status: domain.Active}, true, "not_terminal", false, domain.Active},
		// No process manager is wired, so the hot attach itself fails: proves the
		// live seam reached Start, and the prior terminal status is restored.
		{"live reaches hot attach and failure restores status", &instance.Instance{Title: "x", Status: domain.Failed}, true, "error", true, domain.Failed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newClearTerminalService(tc.inst)
			probed := false
			res := svc.clearTerminalStatusOne("x", probeOf(tc.live, &probed))
			assert.Equal(t, tc.want, res.Outcome, res.Message)
			assert.Equal(t, tc.wantProbe, probed)
			assert.Equal(t, tc.want0, tc.inst.GetLifecycleStatus())
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
	svc := newClearTerminalService(&instance.Instance{Title: "dead", Status: domain.PermanentlyFailed})

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

func clearReq(ids ...string) *connect.Request[sessionv1.ClearTerminalStatusIfLiveRequest] {
	return connect.NewRequest(&sessionv1.ClearTerminalStatusIfLiveRequest{SessionIds: ids})
}

func TestClearTerminalStatusIfLive_ClearedPublishesSessionUpdated(t *testing.T) {
	t.Parallel()
	inst := instance.NewHotRestorableInstanceForTest(t, "alive", domain.PermanentlyFailed)
	svc := newClearTerminalService(inst)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch, _ := svc.eventBus.Subscribe(ctx)

	resp, err := svc.ClearTerminalStatusIfLive(ctx, clearReq("alive"))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 1)
	assert.Equal(t, "cleared", resp.Msg.Results[0].Outcome, resp.Msg.Results[0].Message)
	assert.Equal(t, domain.Active, inst.GetLifecycleStatus())

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventSessionUpdated, ev.Type)
		assert.Contains(t, ev.UpdatedFields, "status")
		assert.Same(t, inst, ev.Session)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a SessionUpdated event after a cleared outcome")
	}
}

func TestClearTerminalStatusIfLive_BatchCapBoundary(t *testing.T) {
	t.Parallel()
	svc := newClearTerminalService()
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("id-%d", i)
		}
		return out
	}
	resp, err := svc.ClearTerminalStatusIfLive(t.Context(), clearReq(ids(100)...))
	require.NoError(t, err, "exactly the cap is accepted")
	assert.Len(t, resp.Msg.Results, 100)

	_, err = svc.ClearTerminalStatusIfLive(t.Context(), clearReq(ids(101)...))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestClearTerminalStatusIfLive_RejectsBlankIDs(t *testing.T) {
	t.Parallel()
	svc := newClearTerminalService(&instance.Instance{Title: "x", Status: domain.Failed})
	for _, bad := range []string{"", "   "} {
		_, err := svc.ClearTerminalStatusIfLive(t.Context(), clearReq("x", bad))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "id %q", bad)
	}
}

func TestClearTerminalStatusIfLive_SingleIDErrorMapping(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, tc := range []struct {
		name string
		inst *instance.Instance
		want connect.Code
	}{
		// IsLiveOrStoredSessionLive is false for a bare instance: not_live.
		{"not_live", &instance.Instance{Title: "x", Status: domain.PermanentlyFailed}, connect.CodeFailedPrecondition},
		{"archived", &instance.Instance{Title: "x", Status: domain.Failed, ArchivedAt: &now}, connect.CodeFailedPrecondition},
		{"not_terminal", &instance.Instance{Title: "x", Status: domain.Active}, connect.CodeFailedPrecondition},
		{"read_only", hiddenFailedInstance(t), connect.CodeFailedPrecondition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newClearTerminalService(tc.inst).ClearTerminalStatusIfLive(t.Context(), clearReq("x"))
			require.Error(t, err)
			assert.Equal(t, tc.want, connect.CodeOf(err))
			assert.Contains(t, err.Error(), tc.name)
		})
	}
}

func hiddenFailedInstance(t *testing.T) *instance.Instance {
	t.Helper()
	inst := instance.NewHotRestorableInstanceForTest(t, "x", domain.Failed)
	instance.SetHiddenForTest(t, inst, true)
	return inst
}

func TestClearTerminalStatusIfLive_HiddenSessionIsReadOnly(t *testing.T) {
	t.Parallel()
	inst := hiddenFailedInstance(t)
	svc := newClearTerminalService(inst)
	resp, err := svc.ClearTerminalStatusIfLive(t.Context(), clearReq("x", "missing"))
	require.NoError(t, err)
	assert.Equal(t, "read_only", resp.Msg.Results[0].Outcome)
	assert.Equal(t, domain.Failed, inst.GetLifecycleStatus(), "a hidden session is never revived")
}

func TestClearTerminalStatusIfLive_ExternalDiscoveryOnlySessionIsNotFound(t *testing.T) {
	t.Parallel()
	// Not in the poller: findInstance's external-discovery fallback must not apply.
	svc := newClearTerminalService()
	res := svc.clearTerminalStatusOne("ext", probeOf(true, new(bool)))
	assert.Equal(t, "not_found", res.Outcome)
}

func TestClearTerminalStatusIfLive_DuplicateIDsClearOnce(t *testing.T) {
	t.Parallel()
	inst := instance.NewHotRestorableInstanceForTest(t, "dup", domain.Failed)
	svc := newClearTerminalService(inst)
	resp, err := svc.ClearTerminalStatusIfLive(t.Context(), clearReq("dup", "dup"))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 2)
	assert.Equal(t, "cleared", resp.Msg.Results[0].Outcome)
	assert.Equal(t, "not_terminal", resp.Msg.Results[1].Outcome)
}

func TestClearTerminalStatusIfLive_CanceledContextStopsTheBatch(t *testing.T) {
	t.Parallel()
	inst := instance.NewHotRestorableInstanceForTest(t, "late", domain.Failed)
	svc := newClearTerminalService(inst)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	resp, err := svc.ClearTerminalStatusIfLive(ctx, clearReq("late", "other"))
	require.NoError(t, err)
	for _, r := range resp.Msg.Results {
		assert.Equal(t, "canceled", r.Outcome)
	}
	assert.Equal(t, domain.Failed, inst.GetLifecycleStatus())
}
