package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// setupUWSFixtureWithWatchDirWatcher mirrors setupUWSFixture but wires a real
// WatchDirWatcher, exercising the same production path UpdateUnfinishedWorkConfig
// uses to add/remove watch dirs live.
func setupUWSFixtureWithWatchDirWatcher(t *testing.T) (svc *UnfinishedWorkService, scanner *unfinished.Scanner, cleanup func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "uws-watchdir-test-*")
	require.NoError(t, err)

	stateStore, err := unfinished.NewStateStore(filepath.Join(tmpDir, "unfinished-state.json"))
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	scanner = unfinished.NewScanner(bus, stateStore)
	watchDirWatcher := unfinished.NewWatchDirWatcher(scanner, stateStore)

	storage := createTestStorage(t)
	svc = NewUnfinishedWorkService(scanner, stateStore, bus, storage, watchDirWatcher)

	cleanup = func() {
		bus.Close()
		os.RemoveAll(tmpDir)
	}
	return
}

func makeWatchDirRepo(t *testing.T, parent, name string) string {
	t.Helper()
	repoPath := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
	return repoPath
}

// TestUpdateUnfinishedWorkConfig_AddingWatchDirRegistersItsRepos proves the
// applyWatchDirChanges "dir added" branch actually reaches the scanner.
func TestUpdateUnfinishedWorkConfig_AddingWatchDirRegistersItsRepos(t *testing.T) {
	svc, scanner, cleanup := setupUWSFixtureWithWatchDirWatcher(t)
	defer cleanup()

	root := t.TempDir()
	repoPath := makeWatchDirRepo(t, root, "my-repo")

	_, err := svc.UpdateUnfinishedWorkConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
		Config: &sessionv1.UnfinishedWorkConfig{WatchDirs: []string{root}},
	}))
	require.NoError(t, err)

	require.True(t, scanner.IsTracked(repoPath), "repo under newly-added watch dir should be registered")
}

// TestUpdateUnfinishedWorkConfig_RemovingWatchDirUnregistersItsRepos proves the
// applyWatchDirChanges "dir removed" branch actually reaches the scanner.
func TestUpdateUnfinishedWorkConfig_RemovingWatchDirUnregistersItsRepos(t *testing.T) {
	svc, scanner, cleanup := setupUWSFixtureWithWatchDirWatcher(t)
	defer cleanup()

	root := t.TempDir()
	repoPath := makeWatchDirRepo(t, root, "my-repo")

	ctx := context.Background()
	_, err := svc.UpdateUnfinishedWorkConfig(ctx, connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
		Config: &sessionv1.UnfinishedWorkConfig{WatchDirs: []string{root}},
	}))
	require.NoError(t, err)
	require.True(t, scanner.IsTracked(repoPath), "setup: repo should be tracked before removal")

	_, err = svc.UpdateUnfinishedWorkConfig(ctx, connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
		Config: &sessionv1.UnfinishedWorkConfig{WatchDirs: nil},
	}))
	require.NoError(t, err)

	require.False(t, scanner.IsTracked(repoPath), "repo should be unregistered after its watch dir is removed")
}

// TestUpdateUnfinishedWorkConfig_RemovingWatchDirDoesNotDropPinnedRepo is the
// regression test for the pinned-repo/watch-dir ordering bug: a repo that is
// both pinned and discovered under a watch dir must stay tracked after the
// watch dir is removed, since Scanner has no per-source refcounting and would
// otherwise leave it untracked until some other source happens to re-add it.
func TestUpdateUnfinishedWorkConfig_RemovingWatchDirDoesNotDropPinnedRepo(t *testing.T) {
	svc, scanner, cleanup := setupUWSFixtureWithWatchDirWatcher(t)
	defer cleanup()

	root := t.TempDir()
	repoPath := makeWatchDirRepo(t, root, "pinned-and-watched-repo")

	ctx := context.Background()
	_, err := svc.UpdateUnfinishedWorkConfig(ctx, connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
		Config: &sessionv1.UnfinishedWorkConfig{
			WatchDirs:   []string{root},
			PinnedRepos: []string{repoPath},
		},
	}))
	require.NoError(t, err)
	require.True(t, scanner.IsTracked(repoPath), "setup: repo should be tracked before watch dir removal")

	_, err = svc.UpdateUnfinishedWorkConfig(ctx, connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
		Config: &sessionv1.UnfinishedWorkConfig{
			WatchDirs:   nil,
			PinnedRepos: []string{repoPath},
		},
	}))
	require.NoError(t, err)

	require.True(t, scanner.IsTracked(repoPath),
		"repo still pinned should remain tracked after its watch dir is removed")
}

// TestUpdateUnfinishedWorkConfig_NilWatchDirWatcherIsNoOp proves the
// applyWatchDirChanges nil-watchDirWatcher branch doesn't panic or error when
// the feature is wired up with a nil watcher (e.g. disabled deployments).
func TestUpdateUnfinishedWorkConfig_NilWatchDirWatcherIsNoOp(t *testing.T) {
	svc, cleanup := setupUWSFixture(t) // wires watchDirWatcher: nil, see setupUWSFixture
	defer cleanup()

	_, err := svc.UpdateUnfinishedWorkConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
		Config: &sessionv1.UnfinishedWorkConfig{WatchDirs: []string{t.TempDir()}},
	}))
	require.NoError(t, err)
}
