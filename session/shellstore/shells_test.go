package shellstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	entshell "github.com/tstapler/stapler-squad/session/ent/shell"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

func newShells(t testing.TB, titles ...string) *Shells {
	t.Helper()
	client := entdbtest.NewClient(t)
	for _, title := range titles {
		_, err := client.Session.Create().SetTitle(title).SetPath("/tmp/x").SetStatus(0).SetProgram("claude").Save(context.Background())
		require.NoError(t, err)
	}
	return New(client)
}

func shell(id string, order int) domain.ShellData {
	return domain.ShellData{ID: id, Name: id, Command: "bash", TmuxSessionName: "tmux-" + id, OrderIndex: order}
}

func TestShells_should_ListInOrderIndexOrderScopedToSession_When_Created(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newShells(t, "a", "b")
	require.NoError(t, s.CreateShell(ctx, "a", shell("a2", 2)))
	require.NoError(t, s.CreateShell(ctx, "a", shell("a1", 1)))
	require.NoError(t, s.CreateShell(ctx, "b", shell("b1", 0)))

	got, err := s.ListShells(ctx, "a")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "a1", got[0].ID)
	assert.Equal(t, "a2", got[1].ID)
	assert.Equal(t, domain.ShellStatusRunning, got[0].Status)
}

func TestShells_should_StoreWorkingDirOnlyWhenNonEmpty_When_Created(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newShells(t, "a")
	withDir := shell("w", 0)
	withDir.WorkingDir = "/work"
	require.NoError(t, s.CreateShell(ctx, "a", withDir))
	require.NoError(t, s.CreateShell(ctx, "a", shell("n", 1)))

	got, err := s.ListShells(ctx, "a")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "/work", got[0].WorkingDir)
	assert.Empty(t, got[1].WorkingDir)
	nulls, err := s.client.Shell.Query().Where(entshell.WorkingDirIsNil()).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, nulls, "an empty working dir is stored as NULL, not \"\"")
}

func TestShells_should_ErrorWithSessionName_When_SessionMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newShells(t)
	err := s.CreateShell(ctx, "ghost", shell("x", 0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CreateShell: session 'ghost' not found")
	_, err = s.ListShells(ctx, "ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ListShells: session 'ghost' not found")
}

func TestShells_should_RecordExitCodeAndStopTimeOnlyWhenNotRunning_When_StatusUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newShells(t, "a")
	require.NoError(t, s.CreateShell(ctx, "a", shell("x", 0)))

	code := 3
	require.NoError(t, s.UpdateShellStatus(ctx, "x", domain.ShellStatusRunning, nil))
	row, err := s.client.Shell.Get(ctx, "x")
	require.NoError(t, err)
	assert.Nil(t, row.StoppedAt, "a running shell has no stopped_at")
	assert.Nil(t, row.ExitCode)

	require.NoError(t, s.UpdateShellStatus(ctx, "x", "stopped", &code))
	row, err = s.client.Shell.Get(ctx, "x")
	require.NoError(t, err)
	assert.Equal(t, "stopped", row.Status)
	require.NotNil(t, row.ExitCode)
	assert.Equal(t, 3, *row.ExitCode)
	assert.NotNil(t, row.StoppedAt, "a non-running status stamps stopped_at")
}

func TestShells_should_RemoveRowAndWrapError_When_Deleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newShells(t, "a")
	require.NoError(t, s.CreateShell(ctx, "a", shell("x", 0)))
	require.NoError(t, s.DeleteShell(ctx, "x"))
	got, err := s.ListShells(ctx, "a")
	require.NoError(t, err)
	assert.Empty(t, got)

	err = s.DeleteShell(ctx, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DeleteShell:")
	err = s.UpdateShellStatus(ctx, "x", "stopped", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UpdateShellStatus:")
}
