package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// Story 5.2 unary guards: a hidden (background) session is read-only for the UI
// write RPCs; a visible one is unchanged.

func requireReadOnlyRefusal(t *testing.T, err error, what string) {
	t.Helper()
	require.Error(t, err, what)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), what)
	assert.Contains(t, err.Error(), "read-only", what)
}

// T-RO-14
func TestWriteToSession_ShouldReturnFailedPreconditionWhenHiddenAndSucceedWhenVisible(t *testing.T) {
	e := newSteerEnv(t)
	hidden := e.addSession("wts-hidden", true, "")
	visible := e.addSession("wts-visible", false, "")
	ts := newLeaseTerminalService(t, hidden.inst)
	ts.poller.SetInstances([]*session.Instance{hidden.inst, visible.inst})
	ts.SetGuardsFlag(e.fix.svc.guards)

	_, err := ts.WriteToSession(context.Background(), writeToSessionReq("wts-hidden"))
	requireReadOnlyRefusal(t, err, "hidden")
	assert.Empty(t, hidden.pm.writes())
	session.AssertLeaseFree(t, hidden.inst)

	_, err = ts.WriteToSession(context.Background(), writeToSessionReq("wts-visible"))
	require.NoError(t, err)
	assert.Equal(t, []string{"ls", session.EnterKeySequence}, visible.pm.writes())
}

// T-RO-13
func TestRestartSessionAndShell_ShouldReturnFailedPrecondition_WhenHiddenAndInternalCallersUnaffected(t *testing.T) {
	fix := setupShellsFixture(t)
	hidden := &session.Instance{
		Title: "restart-hidden", Path: "/tmp/test", Status: session.Paused, Program: "claude",
		Hidden: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(hidden))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == "restart-hidden" {
			addInstanceToPoller(fix.poller, li)
		}
	}
	require.True(t, fix.svc.FindLiveInstance("restart-hidden").Snapshot().Hidden)

	_, err = fix.svc.RestartSession(context.Background(), connect.NewRequest(&sessionv1.RestartSessionRequest{Id: "restart-hidden"}))
	requireReadOnlyRefusal(t, err, "RestartSession")

	_, err = fix.svc.RestartShell(context.Background(), connect.NewRequest(&sessionv1.RestartShellRequest{SessionId: "restart-hidden", ShellId: "sh-1"}))
	requireReadOnlyRefusal(t, err, "RestartShell")
}

// T-RO-40
func TestSwitchWorkspace_ShouldReturnFailedPreconditionWithZeroWritesZeroCheckpointsAndZeroInFlightEntriesForAHiddenTargetForEveryType_WhenDirectoryRevisionOrWorktreeSwitchIsRequested(t *testing.T) {
	e := newSteerEnv(t)
	hidden := e.addSession("switch-hidden", true, "")
	// Paused, so the visible switch takes no checkpoint through the fake pane.
	e.addSession("switch-visible", false, "", func(i *session.Instance) { i.Status = session.Paused })

	types := map[string]sessionv1.WorkspaceSwitchType{
		"directory": sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_DIRECTORY,
		"revision":  sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_REVISION,
		"worktree":  sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_WORKTREE,
	}
	for name, st := range types {
		// A pre-existing in-flight entry would answer Unavailable if the guard ran
		// after the in-flight store; the guard must come first.
		e.fix.svc.workspaceSvc.inFlightSwitches.Store("switch-hidden", true)
		_, err := e.fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
			Id: "switch-hidden", Target: "/elsewhere", SwitchType: st,
		}))
		requireReadOnlyRefusal(t, err, name)
		_, stillThere := e.fix.svc.workspaceSvc.inFlightSwitches.Load("switch-hidden")
		assert.True(t, stillThere, "%s: the guard must not touch the in-flight map", name)
		e.fix.svc.workspaceSvc.inFlightSwitches.Delete("switch-hidden")
	}
	assert.Empty(t, hidden.pm.writes(), "no cd marker typed into the pane")
	assert.Empty(t, hidden.inst.Checkpoints, "no pre-switch checkpoint")

	resp, err := e.fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id: "switch-visible", Target: "/elsewhere", SwitchType: sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_DIRECTORY,
	}))
	if err != nil {
		assert.NotContains(t, err.Error(), "read-only", "a visible target is not refused by the guard")
	} else {
		require.NotNil(t, resp)
	}
}

// T-RO-47
func TestUpdateSession_ShouldReturnFailedPreconditionWithZeroRestartsZeroMarkersAndNoMutation_WhenAHiddenActiveTargetChangesAutoApproveOrProgram(t *testing.T) {
	e := newSteerEnv(t)
	hidden := e.addSession("restart-fields", true, "")
	hidden.inst.AutoApprove = false
	title := "renamed"
	yes, no := true, false
	aider := "aider"

	cases := map[string]*sessionv1.UpdateSessionRequest{
		"auto_approve":         {Id: "restart-fields", AutoApprove: &yes},
		"program":              {Id: "restart-fields", Program: &aider},
		"auto_approve + title": {Id: "restart-fields", AutoApprove: &yes, Title: &title},
		"autonomous_mode":      {Id: "restart-fields", AutonomousMode: &yes},
	}
	for name, r := range cases {
		_, err := e.fix.svc.UpdateSession(context.Background(), connect.NewRequest(r))
		require.Error(t, err, name)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), name)
	}
	assert.Equal(t, "restart-fields", hidden.inst.Title, "the whole request is rejected")
	assert.False(t, hidden.inst.AutoApprove)
	assert.Equal(t, "claude", hidden.inst.Program)
	assert.Empty(t, hidden.pm.writes(), "no restart marker typed into the pane")

	// An unchanged value restarts nothing, so it is not refused.
	_, err := e.fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id: "restart-fields", AutoApprove: &no,
	}))
	require.NoError(t, err)
}

// T-RO-32
func TestGuardsFlag_ShouldBeAnInjectedInterfaceNotABoolAndFailClosed_WhenConfigUnreadable(t *testing.T) {
	hidden := session.NewStartedInstanceForTest(t, "flag-hidden", &steerRecordingPM{})
	hidden.Hidden = true
	visible := session.NewStartedInstanceForTest(t, "flag-visible", &steerRecordingPM{})

	assert.Equal(t, TerminalReadOnly, AccessForUnary(nil, nil))
	assert.Equal(t, TerminalReadOnly, AccessForUnary(hidden, nil), "no flag injected: guards on")
	var typedNil *UnaryGuardsFlag
	assert.Equal(t, TerminalReadOnly, AccessForUnary(hidden, typedNil), "a nil flag value is on")
	assert.Equal(t, TerminalReadWrite, AccessForUnary(visible, nil))

	flag := &UnaryGuardsFlag{}
	assert.True(t, flag.GuardsEnabled(), "the zero value is on")
	assert.Equal(t, TerminalReadOnly, AccessForUnary(hidden, flag))
	flag.SetEnabled(false)
	assert.Equal(t, TerminalReadWrite, AccessForUnary(hidden, flag), "off lets a hidden target through")
	flag.SetEnabled(true)
	assert.Equal(t, TerminalReadOnly, AccessForUnary(hidden, flag))
}
