package services

import (
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// addHibernatedThenForceActive persists inst with Status=Hibernated (whose reload path
// unconditionally sets started=true, unlike Active/Stopped which defer Start() to an async
// loop that never runs in these tests — see fromInstanceData's branches), reloads it, force-
// flips Status back to Active without touching the started field, and registers it with the
// poller. This yields an Active + started=true instance backed by a real (but not yet
// running) tmux-capable processManager, so an Active-branch program switch performs a real
// restart — mirroring TestResumeCrashedSession_TransitionsCrashedToActive's real-tmux
// pattern elsewhere in this file (including its cleanup convention).
func addHibernatedThenForceActive(t *testing.T, fix *forkTestFixture, title, path string) *session.Instance {
	t.Helper()

	inst := &session.Instance{
		Title:     title,
		Path:      path,
		Status:    session.Hibernated,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == title {
			li.ForceStatus(session.Active)
			addInstanceToPoller(fix.poller, li)
			return li
		}
	}
	t.Fatalf("addHibernatedThenForceActive: could not find %q after reload", title)
	return nil
}

// findInstanceByTitle returns the instance with the given title from loaded, failing the
// test if it isn't present.
func findInstanceByTitle(t *testing.T, loaded []*session.Instance, title string) *session.Instance {
	t.Helper()
	for _, inst := range loaded {
		if inst.Title == title {
			return inst
		}
	}
	t.Fatalf("findInstanceByTitle: could not find %q", title)
	return nil
}

// --------------------------------------------------------------------------
// UpdateSession – program branch (RPC-handler level)
// --------------------------------------------------------------------------

// TestUpdateSession_ProgramUpdate_ActiveSession_Restarts verifies that changing the
// program on an Active session performs a real restart (as opposed to Stopped, which skips
// it — see the sibling NoRestart test below) and returns the updated program in the
// response. Uses a real tmux session, cleaned up afterward — same pattern as
// TestResumeCrashedSession_TransitionsCrashedToActive elsewhere in this file.
func TestUpdateSession_ProgramUpdate_ActiveSession_Restarts(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := addHibernatedThenForceActive(t, fix, "active-program-session", t.TempDir())
	t.Cleanup(func() { _ = inst.KillSession() })

	newProgram := "aider"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "active-program-session",
		Program: &newProgram,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "aider", resp.Msg.Session.Program)
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_ACTIVE, resp.Msg.Session.Status)

	loaded, loadErr := fix.storage.LoadInstances()
	require.NoError(t, loadErr)
	found := findInstanceByTitle(t, loaded, "active-program-session")
	assert.Equal(t, "aider", found.Program, "program change must persist")
}

// TestUpdateSession_ProgramUpdate_ActiveSession_RestartFailure_ReturnsInternal verifies
// that when Instance.SwitchProgram's restart attempt fails, UpdateSession translates that
// into connect.CodeInternal (session_service.go's switchErr branch) rather than a 200 with
// a stale response — the one branch this refactor touches that TestUpdateSession_
// ProgramUpdate_ActiveSession_Restarts (the success path) doesn't exercise. A working
// directory that doesn't exist makes the real tmux `new-session -c <dir>` deterministically
// fail without needing a fake processManager.
func TestUpdateSession_ProgramUpdate_ActiveSession_RestartFailure_ReturnsInternal(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := addHibernatedThenForceActive(t, fix, "active-restart-failure-session", "/nonexistent/path/does-not-exist-ssq-test")
	t.Cleanup(func() { _ = inst.KillSession() })

	newProgram := "aider"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "active-restart-failure-session",
		Program: &newProgram,
	}))
	require.Error(t, err, "restart against a nonexistent working directory must fail")

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())

	// Per SwitchProgram's contract, persist runs before the restart attempt, so the
	// program change is durable even though the restart itself failed.
	loaded, loadErr := fix.storage.LoadInstances()
	require.NoError(t, loadErr)
	found := findInstanceByTitle(t, loaded, "active-restart-failure-session")
	assert.Equal(t, "aider", found.Program, "program change must persist despite the restart failure")
}

// TestUpdateSession_ProgramUpdate_StoppedSession_NoRestart verifies that changing the
// program on a Stopped (non-Active) session persists the new value without attempting a
// restart, and the request succeeds.
func TestUpdateSession_ProgramUpdate_StoppedSession_NoRestart(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:     "stopped-program-session",
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == "stopped-program-session" {
			addInstanceToPoller(fix.poller, li)
			break
		}
	}

	newProgram := "aider"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "stopped-program-session",
		Program: &newProgram,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "aider", resp.Msg.Session.Program)

	reloaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := findInstanceByTitle(t, reloaded, "stopped-program-session")
	assert.Equal(t, "aider", found.Program, "program change must be persisted")
}

// TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault verifies that an empty
// program string ("System default") resolves to the configured default program rather
// than being stored as "" or silently dropped.
func TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:     "default-resolve-session",
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "aider",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == "default-resolve-session" {
			addInstanceToPoller(fix.poller, li)
			break
		}
	}

	empty := ""
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "default-resolve-session",
		Program: &empty,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	defaultProgram := config.LoadConfig().DefaultProgram
	assert.NotEmpty(t, defaultProgram, "test assumption: a default program must be configured")
	assert.Equal(t, defaultProgram, resp.Msg.Session.Program,
		"empty string must resolve to the configured default program")
}

// TestUpdateSession_ProgramUpdate_SameValue_NoOp verifies that requesting the program the
// session already has is a no-op: no restart, no "program" field in updatedFields, and no
// event published.
func TestUpdateSession_ProgramUpdate_SameValue_NoOp(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "same-value-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := fix.bus.Subscribe(ctx)

	sameProgram := "claude"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "same-value-session",
		Program: &sameProgram,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "claude", resp.Msg.Session.Program)

	select {
	case evt := <-ch:
		t.Fatalf("no-op program update must not publish an event, got %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

// TestUpdateSession_ProgramAndOtherField_SinglePublish is a regression guard for the
// double-publish risk called out in research/pitfalls.md §2: a single UpdateSession
// request that changes both program and another field (title) must publish exactly one
// SessionUpdated event covering both fields, not two separate events.
func TestUpdateSession_ProgramAndOtherField_SinglePublish(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "combo-program-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := fix.bus.Subscribe(ctx)

	newProgram := "aider"
	newTitle := "combo-program-session-renamed"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "combo-program-session",
		Program: &newProgram,
		Title:   &newTitle,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	var collected []*events.Event
collectLoop:
	for {
		select {
		case evt := <-ch:
			collected = append(collected, evt)
		case <-time.After(200 * time.Millisecond):
			break collectLoop
		}
	}

	require.Len(t, collected, 1, "exactly one SessionUpdated event must be published for a combined program+title update")
	assert.ElementsMatch(t, []string{"program", "title"}, collected[0].UpdatedFields)
}

// --------------------------------------------------------------------------
// UpdateSessionProgram (capacity-monitor auto-fallback path)
// --------------------------------------------------------------------------

// TestUpdateSessionProgram_RealInstance_SwitchesAndPersists exercises
// SessionService.UpdateSessionProgram directly against a real SessionService/Instance
// (not capacity_monitor_test.go's mockSessionSwitcher), verifying the program change
// persists to storage.
func TestUpdateSessionProgram_RealInstance_SwitchesAndPersists(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "auto-fallback-session")

	err := fix.svc.UpdateSessionProgram(context.Background(), "auto-fallback-session", "aider")
	require.NoError(t, err)

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := findInstanceByTitle(t, loaded, "auto-fallback-session")
	assert.Equal(t, "aider", found.Program, "program change must be persisted")
}

// TestUpdateSessionProgram_NotFound verifies the not-found error path is a plain error
// (not a panic or a connect error type), matching the SessionSwitcher interface's plain
// `error` return.
func TestUpdateSessionProgram_NotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	err := fix.svc.UpdateSessionProgram(context.Background(), "nonexistent-id", "claude")
	require.Error(t, err)

	var connectErr *connect.Error
	assert.False(t, errors.As(err, &connectErr), "UpdateSessionProgram must return a plain error, not a connect.Error")
}

// TestUpdateSessionProgram_PublishesPlainEvent_WhenStatusManagerNil verifies that
// UpdateSessionProgram still publishes a plain SessionUpdated event (no detection fields)
// when no statusManager is wired — parity with pre-refactor behavior for the common case
// (capacity monitor fires before any controller-status wiring is relevant).
func TestUpdateSessionProgram_PublishesPlainEvent_WhenStatusManagerNil(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "plain-event-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := fix.bus.Subscribe(ctx)

	err := fix.svc.UpdateSessionProgram(context.Background(), "plain-event-session", "aider")
	require.NoError(t, err)

	select {
	case evt := <-ch:
		assert.Equal(t, events.EventSessionUpdated, evt.Type)
		assert.Equal(t, []string{"program"}, evt.UpdatedFields)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SessionUpdated event")
	}
}

// TestUpdateSessionProgram_PublishesPlainEvent_WhenControllerNotRegistered verifies that
// UpdateSessionProgram still publishes a plain SessionUpdated event when a statusManager
// is wired but no controller is registered for the instance (IsControllerActive=false) —
// the other half of AC0/AC1's "controller inactive" parity requirement.
func TestUpdateSessionProgram_PublishesPlainEvent_WhenControllerNotRegistered(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	fix.svc.SetStatusManager(session.NewInstanceStatusManager())

	addPausedSession(t, fix, "no-controller-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := fix.bus.Subscribe(ctx)

	err := fix.svc.UpdateSessionProgram(context.Background(), "no-controller-session", "aider")
	require.NoError(t, err)

	select {
	case evt := <-ch:
		assert.Equal(t, events.EventSessionUpdated, evt.Type)
		assert.Equal(t, []string{"program"}, evt.UpdatedFields)
		assert.Empty(t, evt.DetectedContext, "no controller registered means no detection context")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SessionUpdated event")
	}
}
