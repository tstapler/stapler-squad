package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// A renamed tymux Instance must restore its own daemon session after a restart:
// the daemon session id is persisted and handed to the new backend, so the
// title-derived label (which changed) is not consulted.
func TestTymuxSessionID_PersistedAndRestoredAcrossRenameAndRestart(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	orig := newTymuxSessionFn
	defer func() { newTymuxSessionFn = orig }()

	live := &fakeTymuxManagerForSetCommand{sessionID: "daemon-session-42"}
	inst := &Instance{Title: "before-rename", Path: t.TempDir(), Program: "claude", Status: Paused, Backend: BackendTymux}
	inst.processManager = NewTymuxBackend(live)
	require.NoError(t, inst.Rename("after-rename"))

	data := inst.ToInstanceData()
	assert.Equal(t, "daemon-session-42", data.TymuxSessionID)
	assert.Equal(t, "after-rename", data.Title)

	restored := &fakeTymuxManagerForSetCommand{}
	newTymuxSessionFn = func() tymux.TymuxManager { return restored }
	_, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)
	assert.Equal(t, "daemon-session-42", restored.gotStoredID)
	assert.Equal(t, "after-rename", restored.gotLabel)
}

// Records saved before the id was persisted carry none; restore must not hand
// the backend a bogus id.
func TestTymuxSessionID_OldRecordWithoutIDRestoresWithEmptyStoredID(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	orig := newTymuxSessionFn
	defer func() { newTymuxSessionFn = orig }()
	restored := &fakeTymuxManagerForSetCommand{}
	newTymuxSessionFn = func() tymux.TymuxManager { return restored }

	data := InstanceData{Title: "legacy", Path: t.TempDir(), Program: "claude", Status: Paused, Backend: BackendTymux}
	_, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)
	assert.Empty(t, restored.gotStoredID)
}

func TestEntRepository_PersistsTymuxSessionID(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	data := createTestSession("tymux-id")
	data.TymuxSessionID = "daemon-1"
	require.NoError(t, repo.Create(ctx, data))
	got, err := repo.Get(ctx, data.Title)
	require.NoError(t, err)
	assert.Equal(t, "daemon-1", got.TymuxSessionID)

	data.TymuxSessionID = "daemon-2" // recovery created a new daemon session
	require.NoError(t, repo.Update(ctx, data))
	got, err = repo.Get(ctx, data.Title)
	require.NoError(t, err)
	assert.Equal(t, "daemon-2", got.TymuxSessionID)
}

func TestEntRepository_ClearsTymuxSessionID(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	data := createTestSession("tymux-id-clear")
	data.TymuxSessionID = "daemon-1"
	require.NoError(t, repo.Create(ctx, data))
	data.TymuxSessionID = "" // backend switched to tmux, or the session was killed
	require.NoError(t, repo.Update(ctx, data))
	got, err := repo.Get(ctx, data.Title)
	require.NoError(t, err)
	assert.Empty(t, got.TymuxSessionID)
}

func TestToInstanceData_TmuxSwitchedInstanceDropsTymuxSessionID(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	inst := &Instance{Title: "switched", Path: t.TempDir(), Program: "claude", Status: Paused, Backend: BackendTmux}
	inst.processManager = newTmuxBackendFromOpts(ProcessManagerOptions{})
	assert.Empty(t, inst.ToInstanceData().TymuxSessionID)
}

// A deferred (bulk-load or shadow) instance only builds the backend object; it
// must never Start or Restore, because that would claim the real session's id
// in the process-wide registry and strand it if the shadow is dropped.
func TestFromInstanceDataDeferred_NeverTouchesDaemonSession(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	orig := newTymuxSessionFn
	defer func() { newTymuxSessionFn = orig }()
	var fakes []*fakeTymuxManagerForSetCommand
	newTymuxSessionFn = func() tymux.TymuxManager {
		f := &fakeTymuxManagerForSetCommand{}
		fakes = append(fakes, f)
		return f
	}
	data := InstanceData{Title: "twin", Path: t.TempDir(), Program: "claude", Status: Active, Backend: BackendTymux, TymuxSessionID: "daemon-7"}

	_, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)
	_, err = FromInstanceDataDeferred(data)
	require.NoError(t, err)

	require.Len(t, fakes, 2)
	for _, f := range fakes {
		assert.Zero(t, f.startCalls+f.restoreCalls, "deferred construction must not start or restore")
		assert.Equal(t, "daemon-7", f.gotStoredID, "the real restore later adopts the same id")
	}
}
