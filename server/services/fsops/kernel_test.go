package fsops

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/fsopstest"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestOSFS_ShouldRoundTripFilesThroughTheSeam(t *testing.T) {
	t.Parallel()
	var seam FSOps = OSFS{}
	dir := filepath.Join(t.TempDir(), "a", "b")
	require.NoError(t, seam.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, "f.txt")

	require.NoError(t, seam.WriteFileSync(p, []byte("one"), 0o600))
	assert.Error(t, seam.WriteFileSync(p, []byte("two"), 0o600), "WriteFileSync creates exclusively")
	got, err := seam.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "one", string(got))
	info, err := seam.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, int64(3), info.Size())

	ap, err := seam.OpenAppend(p, 0o600)
	require.NoError(t, err)
	_, err = ap.Write([]byte("+more"))
	require.NoError(t, err)
	require.NoError(t, ap.Sync())
	require.NoError(t, ap.Close())
	got, _ = seam.ReadFile(p)
	assert.Equal(t, "one+more", string(got))

	moved := filepath.Join(dir, "g.txt")
	require.NoError(t, seam.Rename(p, moved))
	matches, err := seam.Glob(filepath.Join(dir, "*.txt"))
	require.NoError(t, err)
	assert.Equal(t, []string{moved}, matches)
	require.NoError(t, seam.Remove(moved))
	_, err = seam.Stat(moved)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestOSFS_ShouldReportWriteFailure_When_DirectoryMissing(t *testing.T) {
	t.Parallel()
	err := OSFS{}.WriteFileSync(filepath.Join(t.TempDir(), "missing", "f"), []byte("x"), 0o600)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestWriteSettingsAtomic_ShouldWriteIndentedJSONWithMode0600(t *testing.T) {
	t.Parallel()
	claudeDir := filepath.Join(t.TempDir(), ".claude")
	path := filepath.Join(claudeDir, "settings.json")
	unlock := LockSettingsPath(path)
	defer unlock()

	err := WriteSettingsAtomic(path, claudeDir, map[string]json.RawMessage{"a": json.RawMessage(`1`)})

	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"a\": 1\n}", string(got))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	entries, _ := os.ReadDir(claudeDir)
	assert.Len(t, entries, 1, "no temp file left behind")
}

func TestWriteSettingsAtomic_ShouldFailAndLeaveNoTemp_When_Unwritable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notDir := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(notDir, nil, 0o600))

	err := WriteSettingsAtomic(filepath.Join(notDir, "s.json"), notDir, map[string]json.RawMessage{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create dir")

	claudeDir := filepath.Join(dir, "d")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	target := filepath.Join(claudeDir, "target")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "child"), 0o755))
	err = WriteSettingsAtomic(target, claudeDir, map[string]json.RawMessage{})
	require.Error(t, err, "rename onto a non-empty directory fails")
	assert.Contains(t, err.Error(), "rename")
	entries, _ := os.ReadDir(claudeDir)
	assert.Len(t, entries, 1, "the failed temp file was removed")

	err = WriteSettingsAtomic(target, claudeDir, map[string]json.RawMessage{"bad": json.RawMessage(`{`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal settings")
}

func TestInternalIOError_ShouldHideCauseFromClient(t *testing.T) {
	t.Parallel()
	err := InternalIOError("read", "dir/f", errors.New("read /home/u/secret: boom"), "failed to read file")
	assert.Equal(t, connect.CodeInternal, err.Code())
	assert.Equal(t, "internal: failed to read file", err.Error())
}

func TestOpenRootError_ShouldMapNotExistToNotFound_And_OtherToInternal(t *testing.T) {
	t.Parallel()
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(OpenRootError(os.ErrNotExist)))
	err := OpenRootError(errors.New("denied by /secret/path"))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "/secret/path")
}

type stubWorkspaces struct {
	ws  domain.Workspace
	err error
	got string
}

func (s *stubWorkspaces) GetWorkspace(id string) (domain.Workspace, error) {
	s.got = id
	return s.ws, s.err
}

func TestResolveFileRequest_ShouldPickRootPerMode(t *testing.T) {
	t.Parallel()
	dir, _ := fsopstest.NewResolverFixture(t)
	prov := &stubWorkspaces{ws: domain.Workspace{ActiveDir: dir, ExistingDir: dir}}

	rr, err := ResolveFileRequest(context.Background(), prov, FileRef{SessionID: "s1", Path: "src/a.go"}, AccessRead)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rr.Root.Close() })
	assert.Equal(t, ResolvedPath("src/a.go"), rr.Resolved)
	assert.False(t, rr.ViaLink)
	assert.Equal(t, "s1", prov.got)
	assert.Equal(t, dir, rr.Root.Dir())

	rr2, err := ResolveFileRequest(context.Background(), prov, FileRef{SessionID: "s1"}, AccessWrite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rr2.Root.Close() })
	assert.Equal(t, ResolvedPath("."), rr2.Resolved, "an empty path names the root")

	rr3, err := ResolveFileRequest(context.Background(), prov, FileRef{SessionID: "s1", Path: "AGENTS.md"}, AccessRead)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rr3.Root.Close() })
	assert.True(t, rr3.ViaLink)
}

func TestResolveFileRequest_ShouldRefuse_When_RequestIsUnusable(t *testing.T) {
	t.Parallel()
	dir, _ := fsopstest.NewResolverFixture(t)
	ctx := context.Background()
	ok := &stubWorkspaces{ws: domain.Workspace{ActiveDir: dir, ExistingDir: dir}}

	_, err := ResolveFileRequest(ctx, ok, FileRef{Path: "a"}, AccessRead)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "session id required")

	_, err = ResolveFileRequest(ctx, &stubWorkspaces{err: errors.New("no such session")}, FileRef{SessionID: "s"}, AccessRead)
	assert.EqualError(t, err, "no such session")

	gone := &stubWorkspaces{ws: domain.Workspace{ActiveDir: filepath.Join(dir, "gone"), ExistingDir: dir}}
	_, err = ResolveFileRequest(ctx, gone, FileRef{SessionID: "s"}, AccessWrite)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.ErrorIs(t, err, ErrWorktreeMissing)

	_, err = ResolveFileRequest(ctx, &stubWorkspaces{}, FileRef{SessionID: "s"}, AccessRead)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "no working directory")

	missing := &stubWorkspaces{ws: domain.Workspace{ExistingDir: filepath.Join(dir, "nope")}}
	_, err = ResolveFileRequest(ctx, missing, FileRef{SessionID: "s"}, AccessRead)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	_, err = ResolveFileRequest(ctx, ok, FileRef{SessionID: "s", Path: "leak"}, AccessRead)
	var denied *SymlinkDeniedError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, "leak", denied.Link)
	assert.Contains(t, denied.Error(), `"leak"`)
}

func TestWorktreeRoot_ShouldExposeConfinedReadAccess(t *testing.T) {
	t.Parallel()
	dir, _ := fsopstest.NewResolverFixture(t)
	root, err := OpenWorktreeRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })

	entries, err := root.ReadDir("src")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "a.go", entries[0].Name())
	_, err = root.ReadDir("missing")
	assert.Error(t, err)
	info, err := root.Stat("AGENTS.md")
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "Stat follows an in-tree link")
	info, err = root.Lstat("AGENTS.md")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "Lstat does not")
	_, err = root.Stat("leak")
	assert.Error(t, err, "os.Root refuses a link leaving the root")
	_, err = fsReadFile(root, "../outside")
	assert.Error(t, err)
	got, err := fsReadFile(root, "src/a.go")
	require.NoError(t, err)
	assert.Equal(t, "package a\n", got)

	_, err = OpenWorktreeRoot(filepath.Join(dir, "nope"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func fsReadFile(root *WorktreeRoot, name string) (string, error) {
	b, err := fs.ReadFile(root.FS(), name)
	return string(b), err
}

func TestRefPathOrDot_ShouldDefaultEmptyToDot(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ".", RefPathOrDot(FileRef{}))
	assert.Equal(t, "a/b", RefPathOrDot(FileRef{Path: "a/b"}))
}

func TestDupPTYFile_ShouldReturnIndependentDescriptor(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(p, []byte("abc"), 0o600))
	f, err := os.Open(p)
	require.NoError(t, err)
	dup, err := DupPTYFile(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	buf := make([]byte, 3)
	n, err := dup.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "abc", string(buf[:n]), "the dup outlives the original")
	require.NoError(t, dup.Close())

	closed, _ := os.Open(p)
	_ = closed.Close()
	_, err = DupPTYFile(closed)
	assert.ErrorContains(t, err, "failed to duplicate PTY fd")
}

func TestExecSyscall_ShouldReturnError_When_ExecutableMissing(t *testing.T) {
	t.Parallel()
	err := ExecSyscall(filepath.Join(t.TempDir(), "no-such-binary"), nil, nil)
	assert.ErrorIs(t, err, syscall.ENOENT)
}

func TestIsKnownTextFile_ShouldMatchExtensionAndBareBasename(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]bool{
		"a/b.GO": true, "README.md": true, "dir/Makefile": true, "Dockerfile": true,
		".gitignore": true, "image.png": false, "noext": false, "a.unknownext": false,
	} {
		assert.Equal(t, want, IsKnownTextFile(path), path)
	}
}

func TestSniffContent_ShouldClassifyTextBinaryAndNulBytes(t *testing.T) {
	t.Parallel()
	ct, bin := SniffContent(strings.NewReader("hello\n"), false)
	assert.Equal(t, "text/plain; charset=utf-8", ct)
	assert.False(t, bin)

	_, bin = SniffContent(strings.NewReader("hel\x00lo"), false)
	assert.True(t, bin, "a NUL in the first 8000 bytes overrides a text MIME")

	_, bin = SniffContent(strings.NewReader("hel\x00lo"), true)
	assert.False(t, bin, "known-text files skip the scan")

	_, bin = SniffContent(strings.NewReader("\x89PNG\r\n\x1a\n"), false)
	assert.True(t, bin)

	long := strings.Repeat("a", 600) + "\x00"
	_, bin = SniffContent(strings.NewReader(long), false)
	assert.True(t, bin, "a NUL past the 512-byte sniff window is still found")
}

func TestReadTextCapped_ShouldCapAtTruncateSize(t *testing.T) {
	t.Parallel()
	buf, truncated, err := ReadTextCapped(strings.NewReader("hello"), 5)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf))
	assert.False(t, truncated)

	big := strings.Repeat("x", TruncateSize+10)
	buf, truncated, err = ReadTextCapped(strings.NewReader(big), int64(len(big)))
	require.NoError(t, err)
	assert.Len(t, buf, TruncateSize)
	assert.True(t, truncated)

	exact := strings.Repeat("y", TruncateSize)
	buf, truncated, err = ReadTextCapped(strings.NewReader(exact), TruncateSize)
	require.NoError(t, err)
	assert.Len(t, buf, TruncateSize)
	assert.False(t, truncated, "a file exactly at the cap is not truncated")
	buf, truncated, err = ReadTextCapped(strings.NewReader(exact+"z"), TruncateSize+1)
	require.NoError(t, err)
	assert.Len(t, buf, TruncateSize)
	assert.True(t, truncated, "one byte over the cap is truncated")

	_, _, err = ReadTextCapped(errSeeker{}, 5)
	assert.ErrorContains(t, err, "seek")
	_, _, err = ReadTextCapped(errReader{}, 5)
	assert.ErrorContains(t, err, "read")
}

type errSeeker struct{}

func (errSeeker) Read([]byte) (int, error)       { return 0, nil }
func (errSeeker) Seek(int64, int) (int64, error) { return 0, errors.New("seek boom") }

type errReader struct{}

func (errReader) Seek(int64, int) (int64, error) { return 0, nil }
func (errReader) Read([]byte) (int, error)       { return 0, errors.New("read boom") }

func TestReadFull_ShouldSwallowEOF_And_SurfaceOtherErrors(t *testing.T) {
	t.Parallel()
	buf := make([]byte, 8)
	n, err := ReadFull(strings.NewReader("abc"), buf)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = ReadFull(errReader{}, buf)
	assert.Equal(t, 0, n)
	assert.EqualError(t, err, "read boom")
}

func TestKnownTextExtensions_ShouldKeepTheAllowlist(t *testing.T) {
	t.Parallel()
	assert.Len(t, KnownTextExtensions, 73)
	for _, ext := range []string{".go", ".md", ".R", ".Dockerfile", ".lock", ".patch"} {
		assert.True(t, KnownTextExtensions[ext], ext)
	}
	assert.False(t, KnownTextExtensions[".png"])
}

func TestRecordMetrics_ShouldNotPanic_For_EveryOutcomeShape(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	assert.NotPanics(t, func() {
		RecordWriteMetrics(ctx, WriteOutcomeOK, time.Millisecond)
		RecordSearchMetrics(ctx, SearchOutcomeOK, time.Millisecond, 0)
		RecordSearchMetrics(ctx, SearchOutcomeOK, time.Millisecond, time.Millisecond)
		RecordRangeMetrics(ctx, "ok", time.Millisecond)
		RecordStatMetrics(ctx, nil, time.Millisecond)
	})
}

func TestStatOutcome_ShouldMapConnectCodesToClosedLabels(t *testing.T) {
	t.Parallel()
	for want, err := range map[string]error{
		"ok":      nil,
		"invalid": connect.NewError(connect.CodeNotFound, nil),
		"denied":  connect.NewError(connect.CodePermissionDenied, nil),
		"refused": connect.NewError(connect.CodeFailedPrecondition, nil),
		"error":   errors.New("boom"),
	} {
		assert.Equal(t, want, statOutcome(err))
	}
	assert.Equal(t, "invalid", statOutcome(connect.NewError(connect.CodeInvalidArgument, nil)))
}

func TestNoopFilesTelemetry_ShouldAcceptEveryEvent(t *testing.T) {
	t.Parallel()
	var tel FilesTelemetry = NoopFilesTelemetry{}
	ctx := context.Background()
	assert.NotPanics(t, func() {
		tel.RecordWrite(ctx, "s", WriteOutcomeEvent{})
		tel.RecordSearch(ctx, "s", SearchOutcomeEvent{})
		tel.RecordSearchFirstResult(ctx, "s", SearchFirstResultEvent{})
		tel.RecordPatch(ctx, "s", PatchOutcomeEvent{})
	})
}

func TestFilesTelemetry_ShouldRecordSearchFirstResultAndScaleBuckets(t *testing.T) {
	t.Parallel()
	p := &fsopstest.RecordingProvider{}
	tel := NewFilesTelemetry(p)
	tel.RecordSearchFirstResult(context.Background(), "s1", SearchFirstResultEvent{Scale: BucketScale(5), Duration: 3 * time.Millisecond})
	require.Len(t, p.Events, 1)
	assert.Equal(t, "files.server.search_first_result", p.Events[0].EventName)
	assert.Equal(t, "s1", p.Events[0].SessionID)
	require.NotNil(t, p.Events[0].DurationMs)
	assert.Equal(t, int64(3), *p.Events[0].DurationMs)
	assert.Equal(t, map[string]string{"scale": string(BucketScale(5))}, p.Events[0].Labels)

	seen := map[ScaleBucket]bool{}
	for _, n := range []int{0, 1, 99, 100, 999, 1000, 9999, 10000, 100000, 1000000} {
		seen[BucketScale(n)] = true
	}
	assert.Greater(t, len(seen), 2, "the scale buckets separate small and large trees")
}

func TestMemBackend_ShouldServeFilesLinksAndFaultHooks(t *testing.T) {
	t.Parallel()
	m := NewMemBackend()
	assert.ErrorIs(t, errOf(m.OpenRoot("/missing")), os.ErrNotExist)
	m.AddFile("/w", "dir/a.txt", []byte("hello"), time.Unix(5, 0))
	m.AddSymlink("/w", "ln", "dir/a.txt")
	m.MkParents("/w", "empty/sub")
	m.EnsureTree("/e")
	root, err := m.OpenRoot("/w")
	require.NoError(t, err)

	target, err := root.Readlink("ln")
	require.NoError(t, err)
	assert.Equal(t, "dir/a.txt", target)
	_, err = root.Readlink("dir/a.txt")
	assert.Error(t, err, "not a symlink")
	_, err = root.Readlink("nope")
	assert.ErrorIs(t, err, os.ErrNotExist)

	info, err := root.Lstat("dir/a.txt")
	require.NoError(t, err)
	assert.Equal(t, "a.txt", info.Name())
	assert.Equal(t, int64(5), info.Size())
	assert.Equal(t, time.Unix(5, 0), info.ModTime())
	assert.Nil(t, info.Sys())
	dirInfo, err := root.Lstat("empty/sub")
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	_, err = root.Open("dir")
	assert.Error(t, err, "directories do not open")
	_, err = root.Open("nope")
	assert.ErrorIs(t, err, os.ErrNotExist)

	var reads []int
	m.SetOnRead(func(n int) { reads = append(reads, n) })
	m.ShortRead = 2
	h, err := root.Open("dir/a.txt")
	require.NoError(t, err)
	buf := make([]byte, 5)
	n, err := h.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "short reads clip to the injected size")
	assert.Equal(t, []int{5}, reads)
	st, err := h.Stat()
	require.NoError(t, err)
	assert.Equal(t, "a.txt", st.Name())
	_, err = h.Seek(0, 0)
	require.NoError(t, err)
	n, err = h.ReadAt(buf, 0)
	assert.Equal(t, 5, n)
	assert.NoError(t, err)
	require.NoError(t, h.Close())
	require.NoError(t, root.Close())

	m.SetOnRead(nil)
	m.ShortRead = 0
	require.NoError(t, root.Replace("dir/a.txt", []byte("bye"), nil))
	require.NoError(t, root.Replace("new/b.txt", []byte("n"), nil))
	boom := errors.New("pre-rename veto")
	assert.ErrorIs(t, root.Replace("dir/a.txt", []byte("no"), func() error { return boom }), boom)
	assert.Error(t, root.Replace("empty", []byte("x"), nil), "a directory is not replaceable")
	h, err = root.Open("dir/a.txt")
	require.NoError(t, err)
	b := make([]byte, 8)
	n, _ = h.Read(b)
	assert.Equal(t, "bye", string(b[:n]), "the vetoed replace left the bytes alone")
}

func errOf(_ FileRoot, err error) error { return err }

func TestOsRootBackend_ShouldReplaceThroughTheConfinedRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("old"), 0o640))
	root, err := OsRootBackend{}.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })

	h, err := root.Open("f.txt")
	require.NoError(t, err)
	_ = h.Close()
	_, err = root.Open("missing.txt")
	require.Error(t, err)

	require.NoError(t, root.Replace("f.txt", []byte("new"), nil))
	got, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	assert.Equal(t, "new", string(got))
	info, _ := os.Stat(filepath.Join(dir, "f.txt"))
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "the replace keeps the mode")

	boom := errors.New("veto")
	assert.ErrorIs(t, root.Replace("f.txt", []byte("zzz"), func() error { return boom }), boom)
	got, _ = os.ReadFile(filepath.Join(dir, "f.txt"))
	assert.Equal(t, "new", string(got))

	_, err = OsRootBackend{}.OpenRoot(filepath.Join(dir, "nope"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestIsAtomicTempName_ShouldRequireLeadingDotAndMarker(t *testing.T) {
	t.Parallel()
	assert.True(t, IsAtomicTempName(".a.txt.ssq-tmp-abc"))
	assert.False(t, IsAtomicTempName("a.txt.ssq-tmp-abc"), "a visible file that merely contains the marker is not a temp")
	assert.False(t, IsAtomicTempName(".a.txt"))
}

func TestWorktreeOnDisk_ShouldBeFalse_When_ActiveDirIsEmptyOrFellBack(t *testing.T) {
	t.Parallel()
	assert.True(t, WorktreeOnDisk(domain.Workspace{ActiveDir: "/w", ExistingDir: "/w"}))
	assert.False(t, WorktreeOnDisk(domain.Workspace{ActiveDir: "/w", ExistingDir: "/repo"}), "cleaned-up worktree fell back to the repo root")
	assert.False(t, WorktreeOnDisk(domain.Workspace{}), "two empty directories are not an on-disk worktree")
}
