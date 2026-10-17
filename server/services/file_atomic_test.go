package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func atomicRoot(t testing.TB) (string, *WorktreeRoot) {
	t.Helper()
	dir := t.TempDir()
	root, err := OpenWorktreeRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	return dir, root
}

func tempNames(t testing.TB, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if isAtomicTempName(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestAtomicReplace_ShouldPreserveModeAndLeaveNoTemp_When_RunSh0755(t *testing.T) {
	dir, root := atomicRoot(t)
	target := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o755))
	require.NoError(t, os.Chmod(target, 0o755)) // defeat the umask

	require.NoError(t, atomicReplace(root, "run.sh", []byte("new")))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	assert.Empty(t, tempNames(t, dir))
}

func TestAtomicReplace_ShouldCreateNewFileWithDefaultMode_And_WorkInSubdirectory(t *testing.T) {
	dir, root := atomicRoot(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))

	require.NoError(t, atomicReplace(root, "sub/new.txt", []byte("hi")))

	info, err := os.Stat(filepath.Join(dir, "sub", "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	assert.Empty(t, tempNames(t, filepath.Join(dir, "sub")))
}

func TestAtomicReplace_ShouldRefuseSymlinkOrDirectoryTarget(t *testing.T) {
	dir, root := atomicRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink("real", filepath.Join(dir, "link")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "d"), 0o755))

	assert.Error(t, atomicReplace(root, "link", []byte("y")), "a write must never be redirected through a link")
	assert.Error(t, atomicReplace(root, "d", []byte("y")))
	got, _ := os.ReadFile(filepath.Join(dir, "real"))
	assert.Equal(t, "x", string(got))
}

func TestAtomicReplace_ShouldNotWriteOutsideRoot_When_ParentIsEscapingSymlink(t *testing.T) {
	outer := t.TempDir()
	dir := filepath.Join(outer, "repo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(outer, "outside"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outer, "outside"), filepath.Join(dir, "esc")))
	root, err := OpenWorktreeRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	assert.Error(t, atomicReplace(root, "esc/f.txt", []byte("pwn")))
	entries, _ := os.ReadDir(filepath.Join(outer, "outside"))
	assert.Empty(t, entries, "nothing may land outside the root, temp files included")
}

func TestAtomicReplace_ShouldLeaveOriginalIntactAndRemoveTemp_When_WriteFailsAfterTempCreate(t *testing.T) {
	dir, root := atomicRoot(t)
	target := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o644))
	w := newAtomicWriter(root)
	var sawTemp atomic.Bool
	w.writeAll = func(f *os.File, _ []byte) error {
		sawTemp.Store(len(tempNames(t, dir)) == 1) // the temp exists when the failure happens
		return errors.New("no space left on device")
	}

	err := w.replace("f.txt", []byte("replacement"))

	require.ErrorContains(t, err, "no space left")
	assert.True(t, sawTemp.Load())
	got, _ := os.ReadFile(target)
	assert.Equal(t, "original", string(got))
	assert.Empty(t, tempNames(t, dir))
}

func TestAtomicReplace_ShouldSweepOnlyStaleTemps_When_WriteToDirectory(t *testing.T) {
	dir, root := atomicRoot(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "other"), 0o755))
	now := time.Now()
	mk := func(rel string, age time.Duration) {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		require.NoError(t, os.Chtimes(p, now.Add(-age), now.Add(-age)))
	}
	mk(".run.sh.ssq-tmp-stale", 15*time.Minute)
	mk(".run.sh.ssq-tmp-young", time.Minute)
	mk("other/.x.ssq-tmp-stale", 15*time.Minute)
	mk("keep.txt", 24*time.Hour)

	require.NoError(t, atomicReplace(root, "run.sh", []byte("new")))

	assert.Equal(t, []string{".run.sh.ssq-tmp-young"}, tempNames(t, dir), "stale swept, young left")
	assert.Len(t, tempNames(t, filepath.Join(dir, "other")), 1, "only the written directory is swept, no recursion")
	_, err := os.Stat(filepath.Join(dir, "keep.txt"))
	assert.NoError(t, err)
}

func TestAtomicReplace_ShouldSerializeSamePath_And_NeverInterleave_When_TwoGoroutinesSave(t *testing.T) {
	dir, root := atomicRoot(t)
	inputs := [2]string{strings.Repeat("A", 1<<16), strings.Repeat("B", 1<<16)}
	var inside, overlaps atomic.Int64
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				unlock := pathLocks.Lock(pathKey{ActiveDir: dir, Path: "f.txt"})
				if inside.Add(1) > 1 {
					overlaps.Add(1)
				}
				assert.NoError(t, atomicReplace(root, "f.txt", []byte(inputs[i])))
				inside.Add(-1)
				unlock()
			}
		}()
	}
	wg.Wait()

	assert.Zero(t, overlaps.Load(), "writers overlapped inside the per-path lock")
	got, err := os.ReadFile(filepath.Join(dir, "f.txt"))
	require.NoError(t, err)
	assert.Contains(t, inputs[:], string(got), "final content is exactly one input, never interleaved")
	assert.Empty(t, tempNames(t, dir))
}

func TestPathMutexRegistry_ShouldNotSerializeAcrossSessions_And_ShouldReleaseIdleEntries(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	reg := &pathMutexRegistry{}
	a := pathKey{ActiveDir: "/wt/a", Path: "README.md"}
	b := pathKey{ActiveDir: "/wt/b", Path: "README.md"}

	unlockA := reg.Lock(a)
	acquiredB := make(chan func(), 1)
	go func() { acquiredB <- reg.Lock(b) }()
	select {
	case unlockB := <-acquiredB:
		unlockB()
	case <-time.After(5 * time.Second):
		t.Fatal("the same relative path in another worktree must not block")
	}

	sameKey := make(chan func(), 1)
	go func() { sameKey <- reg.Lock(a) }()
	require.Eventually(t, func() bool { reg.mu.Lock(); defer reg.mu.Unlock(); return reg.entries[a].refs == 2 }, 5*time.Second, time.Millisecond)
	select {
	case <-sameKey:
		t.Fatal("same key must block while held")
	default:
	}
	unlockA()
	(<-sameKey)()

	assert.Zero(t, reg.size(), "idle entries are removed")
	for i := range 100 {
		reg.Lock(pathKey{ActiveDir: "/wt", Path: ResolvedPath(fmt.Sprintf("f%d", i))})()
	}
	assert.Zero(t, reg.size(), "the registry cannot grow without bound")
}

func TestWithoutAtomicTemps_ShouldHideTempsWithoutMutatingInput(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.txt", ".a.txt.ssq-tmp-123", "b.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, n), nil, 0o644))
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	n := len(entries)

	got := withoutAtomicTemps(entries)

	require.Len(t, got, 2)
	assert.Equal(t, "a.txt", got[0].Name())
	assert.Equal(t, "b.txt", got[1].Name())
	assert.Len(t, entries, n)
	same := withoutAtomicTemps(got)
	assert.Equal(t, got, same)
}

func TestContentSearch_ShouldNotMatchAtomicTempFiles(t *testing.T) {
	_, root := writeTree(t, map[string]string{"a.txt": "needle\n", ".a.txt.ssq-tmp-abc": "needle\n"})
	got, _ := runSearch(t, root, literalOpts("needle"))
	assert.Equal(t, []string{"a.txt"}, matchedPaths(got))
}

func TestFileServiceListFiles_ShouldHideAtomicTempFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".a.txt.ssq-tmp-dead"), []byte("x"), 0o644))
	svc := NewFileService(&fakeWorkspaceProvider{existingDir: dir})

	resp, err := svc.ListFiles(t.Context(), connect.NewRequest(&sessionv1.ListFilesRequest{SessionId: "test-session", IncludeIgnored: true}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Files, 1)
	assert.Equal(t, "a.txt", resp.Msg.Files[0].Name)
}
