package workspacepeers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakeTmux installs a TMUX_BIN script answering list-sessions and show-environment.
// It sets an env var, so callers must not run in parallel.
func fakeTmux(t *testing.T, script string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmux")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700)) // #nosec G306 -- test script must be executable
	t.Setenv("TMUX_BIN", path)
}

const fakeTmuxScript = `
case "$*" in
  *list-sessions*) printf 'staplersquad_one\nother_session\nstaplersquad_two\nstaplersquad_noenv\n\n' ;;
  *"show-environment -t staplersquad_one"*) echo 'STAPLER_SESSION_UUID=uuid-one' ;;
  *"show-environment -t staplersquad_two"*) echo 'STAPLER_SESSION_UUID=uuid-two' ;;
  *"show-environment -t staplersquad_noenv"*) echo 'STAPLER_SESSION_UUID=' ;;
  *"show-environment -t other_session"*) echo 'STAPLER_SESSION_UUID=uuid-foreign' ;;
  *) exit 1 ;;
esac
`

func TestLiveTmuxSessionUUIDs_should_ReturnUUIDsOfPrefixedSessionsOnly(t *testing.T) {
	fakeTmux(t, fakeTmuxScript)

	got := LiveTmuxSessionUUIDs(context.Background())

	assert.Equal(t, map[string]struct{}{"uuid-one": {}, "uuid-two": {}}, got)
}

func TestLiveTmuxSessionUUIDs_should_SkipSessionWhoseEnvironmentLookupFails(t *testing.T) {
	fakeTmux(t, `
case "$*" in
  *list-sessions*) printf 'staplersquad_ok\nstaplersquad_bad\n' ;;
  *"show-environment -t staplersquad_ok"*) echo 'STAPLER_SESSION_UUID=uuid-ok' ;;
  *) exit 1 ;;
esac
`)

	assert.Equal(t, map[string]struct{}{"uuid-ok": {}}, LiveTmuxSessionUUIDs(context.Background()))
}

func TestLiveTmuxSessionUUIDs_should_ReturnEmptyNonNilSet_When_TmuxFails(t *testing.T) {
	fakeTmux(t, "exit 1\n")

	got := LiveTmuxSessionUUIDs(context.Background())

	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestBlockForPath_should_ResolveWorktreeToMainRepoWorkspace(t *testing.T) {
	t.Parallel()
	main := t.TempDir()
	wt := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+main+"/.git/worktrees/feature\n"), 0o600))
	peer := domain.InstanceData{UUID: "peer", Title: "main-checkout-peer", Path: main, Status: domain.Active}

	block := New(fakeStore{data: []domain.InstanceData{peer}}).BlockForPath(context.Background(), wt)

	assert.Contains(t, block, "main-checkout-peer", "a worktree shares its main repo's workspace key")
}

func TestBlockForPath_should_NotMatchSiblingWorktreeByPath_When_PathIsNotAWorktree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	other := domain.InstanceData{UUID: "peer", Title: "elsewhere", Path: t.TempDir(), Status: domain.Active}

	assert.Equal(t, "", New(fakeStore{data: []domain.InstanceData{other}}).BlockForPath(context.Background(), dir))
}
