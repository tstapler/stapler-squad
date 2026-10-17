package services

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const writeSessionID = "test-session"

// writeWorkspaces is the WorkspaceProvider and WriteGuard of a write rig.
type writeWorkspaces struct {
	mu        sync.Mutex
	ws        session.Workspace
	guardErr  error
	checks    int
	gotWrites int // spy: stays zero, a guard has no way to start or revive a session
}

func (w *writeWorkspaces) GetWorkspace(id string) (session.Workspace, error) {
	if id != writeSessionID {
		return session.Workspace{}, connect.NewError(connect.CodeNotFound, errors.New("no such session"))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ws, nil
}

func (w *writeWorkspaces) CheckWriteAccess(string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.checks++
	return w.guardErr
}

type spyInvalidator struct {
	mu   sync.Mutex
	dirs []string
}

func (s *spyInvalidator) InvalidateVCSStatus(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirs = append(s.dirs, dir)
}

type writeRig struct {
	t     *testing.T
	dir   string
	prov  *writeWorkspaces
	flags *stubFilesFlags
	tel   *recordingTelemetry
	inv   *spyInvalidator
	ops   *FileOps
	w     *FileWriter
}

func newWriteRig(t *testing.T) *writeRig {
	t.Helper()
	dir := t.TempDir()
	prov := &writeWorkspaces{ws: onDisk(dir)}
	flags := &stubFilesFlags{editing: true}
	tel := &recordingTelemetry{}
	inv := &spyInvalidator{}
	ops := NewFileOps(FilePolicy{Workspace: prov, Flags: flags, Telemetry: tel, Guard: prov}, osRootBackend{})
	return &writeRig{t: t, dir: dir, prov: prov, flags: flags, tel: tel, inv: inv, ops: ops, w: NewFileWriter(ops, inv)}
}

func (r *writeRig) put(rel string, data []byte) string {
	r.t.Helper()
	p := filepath.Join(r.dir, rel)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(r.t, os.WriteFile(p, data, 0o644))
	return p
}

func (r *writeRig) disk(rel string) []byte {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, rel))
	require.NoError(r.t, err)
	return b
}

// token reads rel through the real read path, the way a client obtains its token.
func (r *writeRig) token(rel string) string {
	r.t.Helper()
	resp, err := r.ops.GetContent(context.Background(), writeSessionID, rel)
	require.NoError(r.t, err)
	return resp.VersionToken
}

func (r *writeRig) write(rel, content, token string) (*sessionv1.WriteFileResponse, error) {
	r.t.Helper()
	resp, err := r.w.WriteFile(context.Background(), connect.NewRequest(&sessionv1.WriteFileRequest{
		SessionId: writeSessionID, Path: rel, Content: []byte(content), ExpectedToken: token,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func reasonOf(t *testing.T, err error) sessionv1.ReadOnlyReason {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	for _, d := range ce.Details() {
		v, verr := d.Value()
		require.NoError(t, verr)
		if rd, ok := v.(*sessionv1.ReadOnlyReasonDetail); ok {
			return rd.Reason
		}
	}
	return sessionv1.ReadOnlyReason_READ_ONLY_REASON_UNSPECIFIED
}

func requireRefusal(t *testing.T, err error, code connect.Code, reason sessionv1.ReadOnlyReason) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, code, connect.CodeOf(err), err.Error())
	assert.Equal(t, reason, reasonOf(t, err), err.Error())
}

func captureSlog(t *testing.T) *strings.Builder {
	t.Helper()
	var logs strings.Builder
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return &logs
}

func TestWriteFile_ShouldReturnSavedWithNewToken_When_ValidTokenAndFlagOn(t *testing.T) {
	logs := captureSlog(t)
	r := newWriteRig(t)
	r.put("a.go", []byte("package a\r\n\r\nvar X = 1\r\n"))
	r.put("bom.txt", append(append([]byte{}, utf8BOM...), "x\r\n"...))
	t1 := r.token("a.go")

	resp, err := r.write("a.go", "package a\n\nvar X = 2\n", t1)

	require.NoError(t, err)
	saved := resp.GetSaved()
	require.NotNil(t, saved)
	assert.NotEqual(t, t1, saved.Token)
	assert.Equal(t, "package a\r\n\r\nvar X = 2\r\n", string(r.disk("a.go")), "file's own CRLF re-applied")
	assert.Equal(t, r.token("a.go"), saved.Token, "response token names the bytes now on disk")
	assert.Contains(t, logs.String(), "outcome=ok")
	assert.Contains(t, logs.String(), "bytes=")
	assert.NotContains(t, logs.String(), "var X = 2")

	resp, err = r.write("bom.txt", "y\n", r.token("bom.txt"))
	require.NoError(t, err)
	require.NotNil(t, resp.GetSaved())
	assert.Equal(t, append(append([]byte{}, utf8BOM...), "y\r\n"...), r.disk("bom.txt"), "BOM re-applied")
	assert.Equal(t, []string{r.dir}, r.inv.dirs[:1], "a save evicts the VCS cache for the active dir")
}

func TestWriteFile_ShouldReturnConflictFullText_When_AgentRewroteFile(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	t1 := r.token("a.go")
	r.put("a.go", append(append([]byte{}, utf8BOM...), "theirs\r\nline\r\n"...)) // agent rewrite, BOM + CRLF
	t3 := r.token("a.go")

	resp, err := r.write("a.go", "mine\n", t1)

	require.NoError(t, err)
	full := resp.GetConflict().GetFullText()
	require.NotNil(t, full)
	assert.Equal(t, "theirs\nline\n", full.Content, "LF-normalized, BOM stripped")
	assert.Equal(t, t3, full.Token)
	assert.Equal(t, append(append([]byte{}, utf8BOM...), "theirs\r\nline\r\n"...), r.disk("a.go"), "file unchanged")
}

func TestWriteFile_ShouldReturnConflictUnavailable_When_DiskGrewPastCapOrBecameBinaryNonUtf8MixedEolOrDeleted(t *testing.T) {
	cases := []struct {
		name string
		data []byte // nil: delete the file
		want sessionv1.ReadOnlyReason
	}{
		{"too large", []byte(strings.Repeat("a", truncateSize+1)), sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE},
		{"binary", []byte{'a', 0, 'b'}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY},
		{"non-utf8", []byte("caf\xE9\n"), sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8},
		{"mixed eol", []byte("a\r\nb\nc\n"), sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL},
		{"deleted", nil, sessionv1.ReadOnlyReason_READ_ONLY_REASON_DELETED},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newWriteRig(t)
			p := r.put("a.go", []byte("one\n"))
			t1 := r.token("a.go")
			if tc.data == nil {
				require.NoError(t, os.Remove(p))
			} else {
				require.NoError(t, os.WriteFile(p, tc.data, 0o644))
			}

			resp, err := r.write("a.go", "mine\n", t1)

			require.NoError(t, err, "never a Connect error")
			un := resp.GetConflict().GetUnavailable()
			require.NotNil(t, un)
			assert.Equal(t, tc.want, un.Reason)
			assert.Nil(t, resp.GetConflict().GetFullText(), "no content field")
			if tc.data != nil {
				assert.Equal(t, tc.data, r.disk("a.go"), "file unchanged")
			}
		})
	}
}

func TestWriteFile_ShouldReturnConflict_When_SameSizeRewriteWithPinnedMtime(t *testing.T) {
	r := newWriteRig(t)
	p := r.put("a.go", []byte("aaaa\n"))
	pinned := time.Unix(1_700_000_000, 0)
	require.NoError(t, os.Chtimes(p, pinned, pinned))
	t1 := r.token("a.go")
	require.NoError(t, os.WriteFile(p, []byte("bbbb\n"), 0o644))
	require.NoError(t, os.Chtimes(p, pinned, pinned))

	resp, err := r.write("a.go", "mine\n", t1)

	require.NoError(t, err)
	require.NotNil(t, resp.GetConflict().GetFullText(), "size+mtime equality never accepts without hashing")
	assert.Equal(t, "bbbb\n", string(r.disk("a.go")))
}

func TestWriteFile_ShouldReturnSaved_When_MtimeTouchedForwardWithIdenticalBytes(t *testing.T) {
	r := newWriteRig(t)
	p := r.put("a.go", []byte("same\n"))
	t1 := r.token("a.go")
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(p, later, later))

	resp, err := r.write("a.go", "mine\n", t1)

	require.NoError(t, err)
	saved := resp.GetSaved()
	require.NotNil(t, saved, "an mtime-only change is not a conflict")
	assert.NotEqual(t, t1, saved.Token)
	assert.Equal(t, "mine\n", string(r.disk("a.go")))
}

func TestWriteFile_ShouldAbortWithConflict_When_PreRenameHookChangesBytes_And_ProceedWhenOnlyMtimeChanges(t *testing.T) {
	t.Run("bytes changed before rename aborts", func(t *testing.T) {
		r := newWriteRig(t)
		p := r.put("a.go", []byte("one\n"))
		t1 := r.token("a.go")
		r.w.preRenameHook = func() { require.NoError(t, os.WriteFile(p, []byte("agent\n"), 0o644)) }

		resp, err := r.write("a.go", "mine\n", t1)

		require.NoError(t, err)
		full := resp.GetConflict().GetFullText()
		require.NotNil(t, full)
		assert.Equal(t, "agent\n", full.Content)
		assert.Equal(t, "agent\n", string(r.disk("a.go")), "the agent's bytes survive")
		assert.Empty(t, tempNames(t, r.dir), "temp removed")
	})
	t.Run("mtime-only change re-hashes equal and proceeds", func(t *testing.T) {
		r := newWriteRig(t)
		p := r.put("a.go", []byte("one\n"))
		t1 := r.token("a.go")
		r.w.preRenameHook = func() {
			later := time.Now().Add(time.Hour)
			require.NoError(t, os.Chtimes(p, later, later))
		}

		resp, err := r.write("a.go", "mine\n", t1)

		require.NoError(t, err)
		require.NotNil(t, resp.GetSaved())
		assert.Equal(t, "mine\n", string(r.disk("a.go")))
	})
}

func TestWriteFile_ShouldReturnFailedPreconditionWorktreeMissing_When_ActiveDirDiffersFromExistingDir(t *testing.T) {
	r := newWriteRig(t)
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "a.go"), []byte("one\n"), 0o644))
	// A paused worktree session: its (deleted) worktree is the active dir, the repo root is the fallback.
	r.prov.ws = session.Workspace{
		RepoRoot: repoRoot, WorktreeDir: filepath.Join(repoRoot, "gone"),
		ActiveDir: filepath.Join(repoRoot, "gone"), ExistingDir: repoRoot,
	}

	_, err := r.write("a.go", "mine\n", "v1:4:1:"+strings.Repeat("0", 64))

	requireRefusal(t, err, connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING)
	assert.Equal(t, "one\n", string(mustRead(t, filepath.Join(repoRoot, "a.go"))), "repo-root file untouched")
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}

func TestWriteFile_ShouldSave_When_DirectoryModeSessionWorktreeDirEmpty(t *testing.T) {
	r := newWriteRig(t)
	r.prov.ws = session.Workspace{RepoRoot: r.dir, WorktreeDir: "", ActiveDir: r.dir, ExistingDir: r.dir}
	r.put("a.go", []byte("one\n"))

	resp, err := r.write("a.go", "mine\n", r.token("a.go"))

	require.NoError(t, err)
	require.NotNil(t, resp.GetSaved())
	assert.Equal(t, "mine\n", string(r.disk("a.go")))
}

func TestWriteFile_ShouldReturnPermissionDeniedHiddenSession_When_SessionHidden(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	tok := r.token("a.go")
	r.prov.guardErr = hiddenSessionRefusal()

	_, err := r.write("a.go", "mine\n", tok)

	requireRefusal(t, err, connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_HIDDEN_SESSION)
	assert.Equal(t, "one\n", string(r.disk("a.go")))

	// The real guard over a real hidden Instance refuses; with the live guards flag off it allows.
	inst := hiddenInstance(t, true)
	ws := &WorkspaceService{liveFinder: staticFinder{inst}}
	requireRefusal(t, ws.CheckWriteAccess(inst.Title), connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_HIDDEN_SESSION)
	off := &UnaryGuardsFlag{}
	off.SetEnabled(false)
	ws.SetGuardsFlag(off)
	assert.NoError(t, ws.CheckWriteAccess(inst.Title))
	assert.NoError(t, (&WorkspaceService{liveFinder: staticFinder{hiddenInstance(t, false)}}).CheckWriteAccess(inst.Title))
}

func TestWriteFile_ShouldRefuseArchivedSession_And_NeverReviveIt(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	tok := r.token("a.go")
	r.prov.guardErr = archivedSessionRefusal()

	_, err := r.write("a.go", "mine\n", tok)

	requireRefusal(t, err, connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING)
	assert.Equal(t, "one\n", string(r.disk("a.go")))
	assert.Zero(t, r.prov.gotWrites, "the writer's only session access is the read-only guard")

	// The real guard, over a real archived Instance: refuses and leaves its state alone.
	inst := newArchivedInstanceForTest(t)
	ws := &WorkspaceService{liveFinder: staticFinder{inst}}
	before := inst.Snapshot().ArchivedAt
	gerr := ws.CheckWriteAccess(inst.Title)
	requireRefusal(t, gerr, connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING)
	assert.Equal(t, before, inst.Snapshot().ArchivedAt, "ArchivedAt unchanged")
	assert.True(t, inst.IsArchived())
}

func TestWriteFile_ShouldReturnFailedPreconditionFlagOffAndWriteNothing_When_FlagOff(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	tok := r.token("a.go")
	r.flags.editing = false

	_, err := r.write("a.go", "mine\n", tok)

	requireRefusal(t, err, connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_FLAG_OFF)
	assert.Equal(t, "one\n", string(r.disk("a.go")))
	assert.Zero(t, r.prov.checks, "flag is checked before any session lookup")
}

func TestWriteFile_ShouldReturnInvalidArgument_When_ContentHasCrNulOrTokenHashDash(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	good := r.token("a.go")
	r.put("huge.txt", []byte(strings.Repeat("a", truncateSize+10)))
	r.put("bin.dat", []byte{1, 2, 0, 3})
	r.put("latin1.txt", []byte("caf\xE9\n"))
	r.put("mixed.txt", []byte("a\r\nb\nc\n"))
	r.put("lone_cr.txt", []byte("a\nb\rc\n"))
	require.NoError(t, os.MkdirAll(filepath.Join(r.dir, ".git", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(r.dir, ".git", "hooks", "pre-commit"), []byte("x"), 0o755))
	require.NoError(t, os.Symlink("a.go", filepath.Join(r.dir, "link.go")))
	dashToken := func(rel string) string {
		info, err := os.Stat(filepath.Join(r.dir, rel))
		require.NoError(t, err)
		return NonEditableToken(info.Size(), info.ModTime().UnixNano()).String()
	}

	table := []struct {
		name, path, content, token string
		code                       connect.Code
		reason                     sessionv1.ReadOnlyReason
	}{
		{"unparseable token", "a.go", "x\n", "not-a-token", connect.CodeInvalidArgument, 0},
		{"NUL in content", "a.go", "a\x00b", good, connect.CodeInvalidArgument, 0},
		{"CR in content", "a.go", "a\r\nb", good, connect.CodeInvalidArgument, 0},
		{"dash token on a plain truncated read", "a.go", "x\n", dashToken("a.go"), connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TRUNCATED},
		{"dash token on a file over the cap", "huge.txt", "x\n", dashToken("huge.txt"), connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE},
		{"binary target", "bin.dat", "x\n", dashToken("bin.dat"), connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY},
		{"non-UTF-8 target", "latin1.txt", "x\n", dashToken("latin1.txt"), connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8},
		{"mixed EOL target", "mixed.txt", "x\n", dashToken("mixed.txt"), connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL},
		{"lone CR target", "lone_cr.txt", "x\n", dashToken("lone_cr.txt"), connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL},
		{"content over 1 MiB", "a.go", strings.Repeat("a", truncateSize+1), good, connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE},
		{"symlink leaf", "link.go", "x\n", good, connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK},
		{".git/hooks/pre-commit", ".git/hooks/pre-commit", "x\n", good, connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_GIT_INTERNAL},
		{"path escape", "../x", "x\n", good, connect.CodeInvalidArgument, 0},
		{"empty path", "", "x\n", good, connect.CodeInvalidArgument, 0},
		{"directory", ".", "x\n", good, connect.CodeInvalidArgument, 0},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			before := treeDigest(t, r.dir)
			_, err := r.write(tc.path, tc.content, tc.token)
			requireRefusal(t, err, tc.code, tc.reason)
			assert.Equal(t, before, treeDigest(t, r.dir), "nothing written")
		})
	}
}

func TestWriteFile_ShouldInvalidateVcsStatus_When_SavedChange(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	tok := r.token("a.go")

	_, err := r.write("a.go", "stale\n", "v1:1:1:"+strings.Repeat("0", 64)) // conflict: no save
	require.NoError(t, err)
	assert.Empty(t, r.inv.dirs, "a conflict changed nothing")

	_, err = r.write("a.go", "mine\n", tok)
	require.NoError(t, err)
	assert.Equal(t, []string{r.dir}, r.inv.dirs, "exactly one eviction through the port")
}

func TestWriteFile_ShouldLogOutcomeWithoutContent_When_OkConflictDeniedDisabledError(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\n"))
	secret := "SECRETCONTENT"
	cases := []struct {
		name    string
		prepare func() string // returns the token to send
		outcome string
	}{
		{"ok", func() string { return r.token("a.go") }, "outcome=ok"},
		{"conflict", func() string { return "v1:1:1:" + strings.Repeat("0", 64) }, "outcome=conflict_rejected"},
		{"denied", func() string { r.prov.guardErr = hiddenSessionRefusal(); return r.token("a.go") }, "outcome=denied"},
		{"disabled", func() string { r.prov.guardErr = nil; r.flags.editing = false; return "x" }, "outcome=disabled"},
		{"error", func() string { r.flags.editing = true; return "x" }, "outcome=rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := tc.prepare()
			logs := captureSlog(t)
			_, _ = r.write("a.go", secret+"\n", tok)
			out := logs.String()
			assert.Contains(t, out, tc.outcome)
			assert.Contains(t, out, "session="+writeSessionID)
			assert.Contains(t, out, "bytes=")
			assert.Contains(t, out, "a.go")
			assert.NotContains(t, out, secret)
			assert.Equal(t, 1, strings.Count(out, "file write"), "one line per call")
		})
	}
}

func TestWriteFile_ShouldLogSessionPathBytesOutcomeIncludingConflictRejected_NoContents(t *testing.T) {
	logs := captureSlog(t)
	r := newWriteRig(t)
	r.put("a.go", []byte("THEIRSTEXT\n"))

	_, err := r.write("a.go", "MINETEXT\n", "v1:1:1:"+strings.Repeat("0", 64))

	require.NoError(t, err)
	out := logs.String()
	for _, want := range []string{"session=" + writeSessionID, "path=a.go", "bytes=9", "outcome=conflict_rejected"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "THEIRSTEXT")
	assert.NotContains(t, out, "MINETEXT")
}

func TestWriteFile_ShouldRecordOneTelemetryEventPerOutcomeWithoutPath_When_OkConflictDeniedDisabledError(t *testing.T) {
	r := newWriteRig(t)
	r.put("a.go", []byte("one\r\n"))

	_, err := r.write("a.go", "mine\n", r.token("a.go")) // ok
	require.NoError(t, err)
	_, err = r.write("a.go", "mine\n", "v1:1:1:"+strings.Repeat("0", 64)) // conflict
	require.NoError(t, err)
	r.prov.guardErr = hiddenSessionRefusal()
	_, _ = r.write("a.go", "mine\n", r.token("a.go")) // denied
	r.prov.guardErr = nil
	r.flags.editing = false
	_, _ = r.write("a.go", "mine\n", "x") // disabled
	r.flags.editing = true
	_, _ = r.write("a.go", "mine\n", "garbage") // rejected: unparseable token
	r.flags.editing = true
	_, _ = r.write("missing-session-file-dir/../../x", "m\n", "x") // rejected: escape

	r.tel.mu.Lock()
	defer r.tel.mu.Unlock()
	require.Len(t, r.tel.writes, 6, "exactly one RecordWrite per call")
	got := make([]WriteOutcome, len(r.tel.writes))
	for i, w := range r.tel.writes {
		got[i] = w.Outcome
	}
	assert.Equal(t, []WriteOutcome{
		WriteOutcomeOK, WriteOutcomeConflictRejected, WriteOutcomeDenied, WriteOutcomeDisabled,
		WriteOutcomeRejected, WriteOutcomeRejected,
	}, got)
	first := r.tel.writes[0]
	assert.Equal(t, WriteReasonNone, first.Reason)
	assert.Equal(t, BucketSize(int64(len("mine\n"))), first.Size)
	assert.Equal(t, EOLCRLF, first.EOL)
	assert.Equal(t, BOMNo, first.BOM)
	assert.Equal(t, WriteReasonHiddenSession, r.tel.writes[2].Reason)
	assert.Equal(t, WriteReasonFlagOff, r.tel.writes[3].Reason)
	assert.Equal(t, WriteReasonBadToken, r.tel.writes[4].Reason)
}

func TestWriteFile_ShouldNotAlterResponse_When_TelemetryRecorderIsSlow(t *testing.T) {
	r := newWriteRig(t)
	r.ops.policy.Telemetry = NewFilesTelemetry(&recordingProvider{err: errors.New("db down")})
	r.put("a.go", []byte("one\n"))

	resp, err := r.write("a.go", "mine\n", r.token("a.go"))

	require.NoError(t, err)
	require.NotNil(t, resp.GetSaved())
}

// staticFinder is a LiveInstanceFinder over one instance (any id resolves to it).
type staticFinder struct{ inst *session.Instance }

func (f staticFinder) FindLiveInstance(string) *session.Instance { return f.inst }

func newArchivedInstanceForTest(t *testing.T) *session.Instance {
	t.Helper()
	inst := hiddenInstance(t, false)
	now := time.Now()
	inst.SetArchivedAt(&now)
	return inst
}

func TestWorkspaceService_ShouldEvictVcsStatusCache_When_InvalidateVcsStatusCalledBySave(t *testing.T) {
	ws := &WorkspaceService{}
	ws.vcsStatusCache.Store("/work/a", vcsStatusCacheEntry{cachedAt: time.Now()})
	ws.vcsStatusCache.Store("/work/b", vcsStatusCacheEntry{cachedAt: time.Now()})

	ws.InvalidateVCSStatus("/work/a")

	_, aCached := ws.vcsStatusCache.Load("/work/a")
	_, bCached := ws.vcsStatusCache.Load("/work/b")
	assert.False(t, aCached, "keyed like GetVCSStatus: the entry for the saved session's dir is gone")
	assert.True(t, bCached, "other sessions' entries stay")

	// A save reaches the cache through the real FileService wiring: WorkspaceService is the StatusInvalidator.
	r := newWriteRig(t)
	ws.vcsStatusCache.Store(r.dir, vcsStatusCacheEntry{cachedAt: time.Now()})
	r.w = NewFileWriter(r.ops, ws)
	r.put("a.go", []byte("one\n"))
	_, err := r.write("a.go", "mine\n", r.token("a.go"))
	require.NoError(t, err)
	_, still := ws.vcsStatusCache.Load(r.dir)
	assert.False(t, still, "the next GetVCSStatus recomputes instead of waiting out the 15 s TTL")
}
