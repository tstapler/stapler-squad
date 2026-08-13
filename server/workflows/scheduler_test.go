package workflows

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// --- FireNow integration (model resolution wiring) ---

// fakeSessionService captures the CreateSessionRequest passed to it and
// returns a synthetic session so FireNow's happy path completes.
type fakeSessionService struct {
	lastReq *sessionv1.CreateSessionRequest
}

func (f *fakeSessionService) CreateSession(_ context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	f.lastReq = req.Msg
	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: &sessionv1.Session{Id: "fake-session-id"},
	}), nil
}

func newTestScheduler(t *testing.T) (*Scheduler, *fakeSessionService, session.WorkflowRepository) {
	t.Helper()
	_, wfRepo := newTestInfra(t)
	fakeSvc := &fakeSessionService{}
	return NewScheduler(wfRepo, fakeSvc, nil), fakeSvc, wfRepo
}

func TestFireNow_FamilyAlias_ExpectResolvedToConcreteModelInProgram(t *testing.T) {
	sched, fakeSvc, wfRepo := newTestScheduler(t)
	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug: "sonnet-family-wf", Name: "sonnet-family-wf",
		Command: "echo hi", TargetDirectory: "/tmp", Model: "family:sonnet",
	})
	require.NoError(t, err)

	_, err = sched.FireNow(context.Background(), wf, "")
	require.NoError(t, err)
	require.NotNil(t, fakeSvc.lastReq)
	assert.Equal(t, "claude --model "+DefaultModelFamilies()["sonnet"], fakeSvc.lastReq.Program)
}

func TestFireNow_ExistingLiteralModelID_ExpectUnchangedProgramString(t *testing.T) {
	sched, fakeSvc, wfRepo := newTestScheduler(t)
	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug: "literal-model-wf", Name: "literal-model-wf",
		Command: "echo hi", TargetDirectory: "/tmp", Model: "claude-sonnet-4-6",
	})
	require.NoError(t, err)

	_, err = sched.FireNow(context.Background(), wf, "")
	require.NoError(t, err)
	require.NotNil(t, fakeSvc.lastReq)
	assert.Equal(t, "claude --model claude-sonnet-4-6", fakeSvc.lastReq.Program)
}

func TestFireNow_UnknownFamilyAlias_ExpectErrorAndNoSessionCreated(t *testing.T) {
	sched, fakeSvc, wfRepo := newTestScheduler(t)
	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug: "unknown-family-wf", Name: "unknown-family-wf",
		Command: "echo hi", TargetDirectory: "/tmp", Model: "family:retired-name",
	})
	require.NoError(t, err)

	_, err = sched.FireNow(context.Background(), wf, "")
	require.Error(t, err)
	assert.Nil(t, fakeSvc.lastReq, "no session should be created when model resolution fails")
}

func TestFireNow_OverrideFile_ExpectNewLatestPickedUpWithoutCodeChange(t *testing.T) {
	sched, fakeSvc, wfRepo := newTestScheduler(t)
	path := filepath.Join(t.TempDir(), "overrides.json")
	overrideBytes, err := json.Marshal(map[string]string{"sonnet": "claude-sonnet-9999-override"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, overrideBytes, 0o600))

	families, err := LoadModelFamilyOverride(path)
	require.NoError(t, err)
	sched.SetModelFamilies(families)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug: "sonnet-override-wf", Name: "sonnet-override-wf",
		Command: "echo hi", TargetDirectory: "/tmp", Model: "family:sonnet",
	})
	require.NoError(t, err)

	_, err = sched.FireNow(context.Background(), wf, "")
	require.NoError(t, err)
	require.NotNil(t, fakeSvc.lastReq)
	assert.Equal(t, "claude --model claude-sonnet-9999-override", fakeSvc.lastReq.Program)
}
