package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/workspacepeers"
)

// recordingGoalStore is the second GoalStore implementation: it records every
// call's arguments and returns canned results, so the tests can pin what the
// handlers pass the store and reach error branches real Ent cannot produce.
type recordingGoalStore struct {
	setCalls    []setGoalCall
	setRes      *instance.SessionGoalData
	setErr      error
	getCalls    []string
	getRes      *instance.SessionGoalData
	getErr      error
	updateCalls []updateTaskCall
	updateRes   *instance.SessionGoalData
	updateErr   error
	peerCalls   []peerCall
	peerRes     []workspacepeers.WorkspacePeer
	peerErr     error
}

type setGoalCall struct {
	uuid, goal, status, setBy, workspaceKey string
	tasks                                   []instance.TaskNode
}

type updateTaskCall struct{ uuid, taskID, status string }

type peerCall struct{ workspaceKey, exclude string }

func (f *recordingGoalStore) SetSessionGoal(_ context.Context, uuid, goal, status string, tasks []instance.TaskNode, setBy, workspaceKey string) (*instance.SessionGoalData, error) {
	f.setCalls = append(f.setCalls, setGoalCall{uuid, goal, status, setBy, workspaceKey, tasks})
	return f.setRes, f.setErr
}

func (f *recordingGoalStore) GetSessionGoal(_ context.Context, uuid string) (*instance.SessionGoalData, error) {
	f.getCalls = append(f.getCalls, uuid)
	return f.getRes, f.getErr
}

func (f *recordingGoalStore) UpdateSessionTaskStatus(_ context.Context, uuid, taskID, status string) (*instance.SessionGoalData, error) {
	f.updateCalls = append(f.updateCalls, updateTaskCall{uuid, taskID, status})
	return f.updateRes, f.updateErr
}

func (f *recordingGoalStore) ListWorkspacePeers(_ context.Context, workspaceKey, exclude string) ([]workspacepeers.WorkspacePeer, error) {
	f.peerCalls = append(f.peerCalls, peerCall{workspaceKey, exclude})
	return f.peerRes, f.peerErr
}

var _ GoalStore = (*recordingGoalStore)(nil)

func fakeGoalHandlers(inst *instance.Instance, fake *recordingGoalStore) *goalHandlers {
	return &goalHandlers{
		storage:  fake,
		peers:    fake,
		store:    &stubStore{instances: []*instance.Instance{inst}},
		eventBus: events.NewEventBus(5),
	}
}

func errorCode(t *testing.T, m map[string]interface{}) string {
	t.Helper()
	require.False(t, m["success"].(bool))
	return m["error"].(map[string]interface{})["code"].(string)
}

func TestGoalStorePort_should_PassResolvedArgsToStore_When_SettingGoal(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("goal-port-set", "uuid-set")
	inst.GitHubOwner, inst.GitHubRepo = "acme", "widgets"
	fake := &recordingGoalStore{setRes: &instance.SessionGoalData{Goal: "stored goal", Status: "blocked", Tasks: []instance.TaskNode{{ID: "t1"}, {ID: "t2"}}}}
	h := fakeGoalHandlers(inst, fake)

	res, err := h.setSessionGoal(WithSessionUUID(context.Background(), "uuid-set"), makeToolReq(map[string]interface{}{
		"goal":   "ship it",
		"status": "working",
		"tasks":  []interface{}{map[string]interface{}{"id": "t1", "title": "one", "status": "pending"}},
	}))
	require.NoError(t, err)

	require.Len(t, fake.setCalls, 1)
	c := fake.setCalls[0]
	assert.Equal(t, "uuid-set", c.uuid)
	assert.Equal(t, "ship it", c.goal)
	assert.Equal(t, "working", c.status)
	assert.Equal(t, "uuid-set", c.setBy)
	assert.Equal(t, inst.WorkspaceKey(), c.workspaceKey)
	assert.NotEmpty(t, c.workspaceKey)
	require.Len(t, c.tasks, 1)
	assert.Equal(t, "t1", c.tasks[0].ID)

	m := parseResult(t, res)
	assert.True(t, m["success"].(bool))
	assert.Equal(t, "stored goal", m["goal"], "result echoes the store's value, not the request")
	assert.Equal(t, "blocked", m["status"])
	assert.EqualValues(t, 2, m["tasks_total"])
}

func TestGoalStorePort_should_ReturnInternalError_When_StoreSetFails(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("goal-port-set-err", "uuid-set-err")
	fake := &recordingGoalStore{setErr: errors.New("db down")}
	h := fakeGoalHandlers(inst, fake)

	res, err := h.setSessionGoal(WithSessionUUID(context.Background(), "uuid-set-err"), makeToolReq(map[string]interface{}{"goal": "g"}))
	require.NoError(t, err)

	m := parseResult(t, res)
	assert.Equal(t, ErrInternalError, errorCode(t, m))
	assert.Contains(t, m["error"].(map[string]interface{})["message"], "db down")
}

func TestGoalStorePort_should_LookUpByInstanceUUID_When_GettingGoal(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("goal-port-get", "uuid-get")
	fake := &recordingGoalStore{getRes: &instance.SessionGoalData{SessionUUID: "uuid-get", Goal: "the goal", Status: "idle"}}
	h := fakeGoalHandlers(inst, fake)

	res, err := h.getSessionGoal(context.Background(), makeToolReq(map[string]interface{}{"session_id": "goal-port-get"}))
	require.NoError(t, err)

	assert.Equal(t, []string{"uuid-get"}, fake.getCalls, "title is resolved to the UUID before the store call")
	assert.Equal(t, "the goal", parseResult(t, res)["goal"])
}

func TestGoalStorePort_should_MapStoreErrors_When_GettingGoal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"not found", domain.ErrNotFound, ErrItemNotFound},
		{"wrapped not found", errors.Join(errors.New("ctx"), domain.ErrNotFound), ErrItemNotFound},
		{"other", errors.New("boom"), ErrInternalError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := makeTestInstance("goal-port-get-err", "uuid-get-err")
			h := fakeGoalHandlers(inst, &recordingGoalStore{getErr: tc.err})

			res, err := h.getSessionGoal(context.Background(), makeToolReq(map[string]interface{}{"session_id": "goal-port-get-err"}))
			require.NoError(t, err)
			assert.Equal(t, tc.code, errorCode(t, parseResult(t, res)))
		})
	}
}

func updateReq() map[string]interface{} {
	return map[string]interface{}{"task_id": "task-1", "status": "done"}
}

func TestGoalStorePort_should_CheckThenUpdateForCaller_When_UpdatingTask(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("goal-port-upd", "uuid-upd")
	fake := &recordingGoalStore{
		getRes:    &instance.SessionGoalData{SessionUUID: "uuid-upd"},
		updateRes: &instance.SessionGoalData{SessionUUID: "uuid-upd", Tasks: []instance.TaskNode{{ID: "task-1", Status: instance.TaskStatusDone}, {ID: "task-2", Status: instance.TaskStatusPending}}},
	}
	h := fakeGoalHandlers(inst, fake)

	res, err := h.updateSessionTask(WithSessionUUID(context.Background(), "uuid-upd"), makeToolReq(updateReq()))
	require.NoError(t, err)

	assert.Equal(t, []string{"uuid-upd"}, fake.getCalls)
	assert.Equal(t, []updateTaskCall{{"uuid-upd", "task-1", "done"}}, fake.updateCalls)
	m := parseResult(t, res)
	assert.True(t, m["success"].(bool))
	assert.EqualValues(t, 1, m["tasks_done"])
	assert.EqualValues(t, 2, m["tasks_total"])
}

func TestGoalStorePort_should_RejectWithoutUpdating_When_GoalCheckFails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		get  *instance.SessionGoalData
		err  error
		code string
	}{
		{"no goal", nil, domain.ErrNotFound, ErrItemNotFound},
		{"store error", nil, errors.New("boom"), ErrInternalError},
		{"goal owned by another session", &instance.SessionGoalData{SessionUUID: "someone-else"}, nil, ErrPermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := makeTestInstance("goal-port-upd-chk", "uuid-chk")
			fake := &recordingGoalStore{getRes: tc.get, getErr: tc.err}
			h := fakeGoalHandlers(inst, fake)

			res, err := h.updateSessionTask(WithSessionUUID(context.Background(), "uuid-chk"), makeToolReq(updateReq()))
			require.NoError(t, err)
			assert.Equal(t, tc.code, errorCode(t, parseResult(t, res)))
			assert.Empty(t, fake.updateCalls, "a failed ownership check must not reach the write")
		})
	}
}

func TestGoalStorePort_should_MapStoreErrors_When_UpdateFails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		code string
		msg  string
	}{
		{"goal gone", domain.ErrNotFound, ErrItemNotFound, "no goal set"},
		{"task not found by message", errors.New("task \"task-1\" not found"), ErrItemNotFound, "not found in goal tree"},
		{"other", errors.New("boom"), ErrInternalError, "failed to update"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := makeTestInstance("goal-port-upd-err", "uuid-uerr")
			fake := &recordingGoalStore{getRes: &instance.SessionGoalData{SessionUUID: "uuid-uerr"}, updateErr: tc.err}
			h := fakeGoalHandlers(inst, fake)

			res, err := h.updateSessionTask(WithSessionUUID(context.Background(), "uuid-uerr"), makeToolReq(updateReq()))
			require.NoError(t, err)
			m := parseResult(t, res)
			assert.Equal(t, tc.code, errorCode(t, m))
			assert.Contains(t, m["error"].(map[string]interface{})["message"], tc.msg)
		})
	}
}

func TestGoalStorePort_should_ScopeToCallersWorkspaceAndMapPeers_When_ListingPeers(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("goal-port-peers", "uuid-peers")
	inst.GitHubOwner, inst.GitHubRepo = "acme", "widgets"
	fake := &recordingGoalStore{peerRes: []workspacepeers.WorkspacePeer{
		{SessionUUID: "p1", Title: "peer one", Branch: "b", Path: "/p", Status: domain.Paused, Goal: &instance.SessionGoalData{Goal: "peer goal", Status: "working"}},
		{SessionUUID: "p2", Title: "peer two"},
	}}
	h := fakeGoalHandlers(inst, fake)

	res, err := h.listWorkspacePeers(WithSessionUUID(context.Background(), "uuid-peers"), makeToolReq(nil))
	require.NoError(t, err)

	assert.Equal(t, []peerCall{{inst.WorkspaceKey(), "uuid-peers"}}, fake.peerCalls)
	peers := parseResult(t, res)["peers"].([]interface{})
	require.Len(t, peers, 2)
	first := peers[0].(map[string]interface{})
	assert.Equal(t, "peer one", first["title"])
	assert.Equal(t, "peer goal", first["goal_text"])
	assert.Equal(t, "working", first["goal_status"])
	assert.NotContains(t, peers[1].(map[string]interface{}), "goal_text", "a goal-less peer has no goal fields")
}

func TestGoalStorePort_should_ReturnInternalError_When_PeerListFails(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("goal-port-peers-err", "uuid-perr")
	inst.GitHubOwner, inst.GitHubRepo = "acme", "widgets"
	h := fakeGoalHandlers(inst, &recordingGoalStore{peerErr: errors.New("boom")})

	res, err := h.listWorkspacePeers(WithSessionUUID(context.Background(), "uuid-perr"), makeToolReq(nil))
	require.NoError(t, err)
	assert.Equal(t, ErrInternalError, errorCode(t, parseResult(t, res)))
}
