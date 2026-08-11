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

// --- ResolveModel ---

func TestResolveModel_KnownFamily_ExpectConcreteID(t *testing.T) {
	families := DefaultModelFamilies()
	resolved, err := ResolveModel(families, "family:sonnet")
	require.NoError(t, err)
	assert.Equal(t, families["sonnet"], resolved)
}

func TestResolveModel_LiteralModelID_ExpectPassThroughUnchanged(t *testing.T) {
	families := DefaultModelFamilies()
	resolved, err := ResolveModel(families, "claude-opus-4-8")
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-8", resolved)
}

func TestResolveModel_Empty_ExpectPassThroughUnchanged(t *testing.T) {
	resolved, err := ResolveModel(DefaultModelFamilies(), "")
	require.NoError(t, err)
	assert.Equal(t, "", resolved)
}

func TestResolveModel_BareFamilyNameWithoutPrefix_ExpectNotResolved(t *testing.T) {
	// A pre-existing workflow that historically stored the literal string
	// "sonnet" (before family aliases existed) must not be silently
	// reinterpreted as the "sonnet" family.
	resolved, err := ResolveModel(DefaultModelFamilies(), "sonnet")
	require.NoError(t, err)
	assert.Equal(t, "sonnet", resolved)
}

func TestResolveModel_UnknownFamily_ExpectError(t *testing.T) {
	_, err := ResolveModel(DefaultModelFamilies(), "family:retired-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retired-name")
}

// --- LoadModelFamilyOverride ---

func TestLoadModelFamilyOverride_ValidJSON_ExpectMergedOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"sonnet":"claude-sonnet-9000"}`), 0o600))

	families, err := LoadModelFamilyOverride(path)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-9000", families["sonnet"])
	// Unrelated defaults survive untouched.
	assert.Equal(t, DefaultModelFamilies()["opus"], families["opus"])
}

func TestLoadModelFamilyOverride_MissingFile_ExpectError(t *testing.T) {
	_, err := LoadModelFamilyOverride(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestLoadModelFamilyOverride_MalformedJSON_ExpectError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(path, []byte(`{not valid json`), 0o600))

	_, err := LoadModelFamilyOverride(path)
	require.Error(t, err)
}

// --- ValidateModel ---

func TestValidateModel_Empty_ExpectValid(t *testing.T) {
	assert.NoError(t, ValidateModel(""))
}

func TestValidateModel_ValidLiteralID_ExpectValid(t *testing.T) {
	assert.NoError(t, ValidateModel("claude-sonnet-4-6"))
}

func TestValidateModel_ValidFamilyAlias_ExpectValid(t *testing.T) {
	assert.NoError(t, ValidateModel("family:sonnet"))
}

func TestValidateModel_EmbeddedSpace_ExpectRejected(t *testing.T) {
	assert.Error(t, ValidateModel("family: sonnet"))
}

func TestValidateModel_ShellMetacharacters_ExpectRejected(t *testing.T) {
	for _, bad := range []string{"claude-sonnet-4-6; rm -rf /", "$(whoami)", "model`id`", "a|b", "a&&b"} {
		assert.Error(t, ValidateModel(bad), "expected %q to be rejected", bad)
	}
}

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
