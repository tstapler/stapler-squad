package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func atArgs() map[string]any {
	return map[string]any{"trigger_type": "at", "message": "check the build", "delay_minutes": float64(30)}
}

func TestScheduleWake_should_RecordTheVerifiedCallerAsOwner_And_IgnoreSessionIdArgument(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.backend.generation = 2
	args := atArgs()
	args["session_id"] = "another-session"

	m := r.call(as("owner-a"), toolScheduleWake, args)

	require.True(t, m["success"].(bool), "%v", m)
	calls := r.backend.scheduleCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "owner-a", calls[0].Owner)
	assert.Equal(t, 2, calls[0].Generation, "the chain generation comes from the server, not the caller")
	assert.Equal(t, domain.WakeActionSteerText, calls[0].Action.Kind)
	assert.Equal(t, domain.AtSpec{At: wakeToolsNow.Add(30 * 60e9)}, calls[0].Spec)
}

func TestScheduleWake_should_ReturnFlagOff_When_ToolOrTriggerScopeIsOff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		global    bool
		scopes    map[string]bool
		wantScope string
	}{
		{"global off", false, nil, "tool:schedule_wake"},
		{"tool scope off under global on", true, map[string]bool{"tool:schedule_wake": false}, "tool:schedule_wake"},
		{"trigger scope off under global on", true, map[string]bool{"trigger:at": false}, "trigger:at"},
		{"tool and trigger on under global off", false, map[string]bool{"tool:schedule_wake": true, "trigger:at": true}, ""},
		{"tool on but dispatcher not running under global off", false, map[string]bool{"tool:schedule_wake": true}, "tool:schedule_wake"},
		{"tool and another trigger on: this type stays off", false, map[string]bool{"tool:schedule_wake": true, "trigger:cron": true}, "trigger:at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newToolRig(t)
			r.cfg = &config.Config{
				FeatureFlags:      map[string]bool{config.WakeTriggersFlag: tt.global},
				FeatureFlagScopes: map[string]map[string]bool{config.WakeTriggersFlag: tt.scopes},
			}
			m := r.call(as("owner-a"), toolScheduleWake, atArgs())
			if tt.wantScope == "" {
				assert.True(t, m["success"].(bool), "%v", m)
				return
			}
			assert.Equal(t, CodeFlagOff, resultCode(m))
			assert.Equal(t, tt.wantScope, resultErrField(m, "reason"))
			assert.Empty(t, r.backend.scheduleCalls())
		})
	}
}

func TestScheduleWake_should_MapDispatcherRefusalsToTypedResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantCode   string
		wantReason string
		wantLimit  string
	}{
		{"cap", &wakes.CapExceededError{Limit: wakes.LimitPendingPerOwner, Detail: "x"}, CodeCapExceeded, "", wakes.LimitPendingPerOwner},
		{"no predicate", wakes.ValidateSpec(domain.BacklogEventSpec{ItemID: "i", Kind: "status_transition"}), CodeNoReconcilePredicate, "", ""},
		{"event stream", wakes.ErrEventStreamUnavailable, CodeUnavailable, "event_stream", ""},
		{"source", errors.Join(wakes.ErrUnavailable, errors.New("no poller")), CodeUnavailable, "source", ""},
		{"invalid", wakes.ErrInvalid, ErrInvalidArgument, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newToolRig(t)
			r.backend.scheduleErr = func(int, wakes.ScheduleRequest) error { return tt.err }
			m := r.call(as("owner-a"), toolScheduleWake, atArgs())
			assert.False(t, m["success"].(bool))
			assert.Equal(t, tt.wantCode, resultCode(m))
			assert.Equal(t, tt.wantReason, resultErrField(m, "reason"))
			assert.Equal(t, tt.wantLimit, resultErrField(m, "limit"))
		})
	}
}

func TestScheduleWake_should_ReturnUnavailable_When_NoDispatcherInThisProcess(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.deps = nil
	m := r.call(as("owner-a"), toolScheduleWake, atArgs())
	assert.Equal(t, CodeUnavailable, resultCode(m))
}

func TestScheduleWake_should_RejectMalformedArguments(t *testing.T) {
	t.Parallel()
	tests := map[string]map[string]any{
		"unknown trigger type": {"trigger_type": "nope", "message": "m"},
		"at in the past":       {"trigger_type": "at", "message": "m", "at": "2026-10-19T11:00:00Z"},
		"at without a time":    {"trigger_type": "at", "message": "m"},
		"empty message":        {"trigger_type": "at", "message": " ", "delay_minutes": float64(5)},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newToolRig(t)
			m := r.call(as("owner-a"), toolScheduleWake, args)
			assert.Equal(t, ErrInvalidArgument, resultCode(m))
			assert.Empty(t, r.backend.scheduleCalls())
		})
	}
}

func TestScheduleWake_should_BuildEachTriggerSpecFromItsArguments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args map[string]any
		want domain.WakeSpec
	}{
		{map[string]any{"trigger_type": "cron", "cron": "0 * * * *"}, domain.CronSpec{Expr: "0 * * * *"}},
		{map[string]any{"trigger_type": "backlog_event", "item_id": "i1", "kind": "verdict_recorded"}, domain.BacklogEventSpec{ItemID: "i1", Kind: "verdict_recorded"}},
		{map[string]any{"trigger_type": "pr_event", "repo": "o/r", "number": float64(9), "kind": "merged"}, domain.PREventSpec{Repo: "o/r", Number: 9, Kind: "merged"}},
		{map[string]any{"trigger_type": "ci_event", "repo": "o/r", "number": float64(9), "kind": "success"}, domain.CIEventSpec{Repo: "o/r", Number: 9, Kind: "success"}},
		{map[string]any{"trigger_type": "session_state", "target_session": "p1", "state": "idle"}, domain.SessionStateSpec{SessionUUID: "p1", State: "idle"}},
		{map[string]any{"trigger_type": "guidance_answered", "guidance_request_id": "g1"}, domain.GuidanceAnsweredSpec{GuidanceRequestID: "g1"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.want.TriggerType()), func(t *testing.T) {
			t.Parallel()
			r := newToolRig(t)
			tt.args["message"] = "wake me"
			m := r.call(as("owner-a"), toolScheduleWake, tt.args)
			require.True(t, m["success"].(bool), "%v", m)
			require.Len(t, r.backend.scheduleCalls(), 1)
			assert.Equal(t, tt.want, r.backend.scheduleCalls()[0].Spec)
		})
	}
}

func TestListWakes_And_CancelWake_should_ScopeToTheVerifiedCaller(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	mine := &session.WakeTriggerData{ID: "w-mine", OwnerSessionUUID: "owner-a", Type: domain.WakeTriggerAt, Spec: domain.AtSpec{At: wakeToolsNow}, Status: domain.WakeStatusPending}
	done := &session.WakeTriggerData{ID: "w-done", OwnerSessionUUID: "owner-a", Type: domain.WakeTriggerAt, Spec: domain.AtSpec{At: wakeToolsNow}, Status: domain.WakeStatusDelivered}
	theirs := &session.WakeTriggerData{ID: "w-theirs", OwnerSessionUUID: "owner-b", Type: domain.WakeTriggerAt, Spec: domain.AtSpec{At: wakeToolsNow}, Status: domain.WakeStatusPending}
	r.backend.live = []*session.WakeTriggerData{mine, done, theirs}

	list := r.call(as("owner-a"), toolListWakes, map[string]any{})
	require.True(t, list["success"].(bool))
	assert.Len(t, list["wakes"], 1, "live wakes of the caller only")
	all := r.call(as("owner-a"), toolListWakes, map[string]any{"include_ended": true})
	assert.Len(t, all["wakes"], 2)

	ok := r.call(as("owner-a"), toolCancelWake, map[string]any{"wake_id": "w-mine"})
	require.True(t, ok["success"].(bool), "%v", ok)
	assert.Equal(t, true, ok["cancelled"])

	r.backend.cancelErr = wakes.ErrWakeNotOwner
	forbidden := r.call(as("owner-a"), toolCancelWake, map[string]any{"wake_id": "w-theirs"})
	assert.Equal(t, CodeForbidden, resultCode(forbidden))
	r.backend.cancelErr = nil
	missing := r.call(as("owner-a"), toolCancelWake, map[string]any{"wake_id": "nope"})
	assert.Equal(t, ErrItemNotFound, resultCode(missing))
}

func TestWakeTools_should_ReachARealDispatcher_And_ReportItsRefusals(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	d := wakes.NewDispatcher(wakes.Config{
		Store: repo, Steerer: noSteerer{}, Admission: denyAdmission{}, Owners: fakeOwnerLookup{},
		Limits: wakes.LimitSettings{PendingPerOwner: 1},
	})
	r := newToolRig(t)
	r.deps = &WakeToolDeps{Backend: d}

	first := r.call(as("owner-a"), toolScheduleWake, atArgs())
	require.True(t, first["success"].(bool), "%v", first)
	second := r.call(as("owner-a"), toolScheduleWake, map[string]any{"trigger_type": "at", "message": "another", "delay_minutes": float64(40)})
	assert.Equal(t, CodeCapExceeded, resultCode(second))
	assert.Equal(t, wakes.LimitPendingPerOwner, resultErrField(second, "limit"))

	noPred := r.call(as("owner-b"), toolScheduleWake, map[string]any{
		"trigger_type": "backlog_event", "message": "m", "item_id": "i", "kind": "status_transition",
	})
	// With no event bus the event-stream refusal comes first; either way it is typed and writes nothing.
	assert.Contains(t, []string{CodeUnavailable, CodeNoReconcilePredicate}, resultCode(noPred))

	list := r.call(as("owner-a"), toolListWakes, map[string]any{})
	assert.Len(t, list["wakes"], 1)
	id := list["wakes"].([]any)[0].(map[string]any)["id"].(string)
	cancelled := r.call(as("owner-a"), toolCancelWake, map[string]any{"wake_id": id})
	assert.Equal(t, true, cancelled["cancelled"])
	foreign := r.call(as("owner-b"), toolCancelWake, map[string]any{"wake_id": id})
	assert.Equal(t, CodeForbidden, resultCode(foreign))
}

type noSteerer struct{}

func (noSteerer) IsReadyForSteer(string) bool { return false }
func (noSteerer) WriteWake(context.Context, string, string) (wakes.WriteOutcome, error) {
	return wakes.WriteNotAttempted, nil
}

type denyAdmission struct{}

func (denyAdmission) Admit(context.Context) (bool, error) { return false, nil }
