package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// rangeRefusalPaths are the Story 2.1.4 confinement matrix paths: each must be
// refused by ReadFileRange with the code GetFileContent gives.
var rangeRefusalPaths = []string{ //nolint:gochecknoglobals
	"leak", "leak_rel", "dir_leak", "../outside", ".git/config", "gitlink", "linkdir",
	"loop1", "dangling", "missing.go", "docs",
}

func codeOf(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}

// runRangeConformance adds the Story 1.6.2 read cases to FileOpsConformance. ops
// must have files:range-read on through a *stubFilesFlags.
func runRangeConformance(t *testing.T, ops *FileOps) {
	t.Helper()
	ctx := context.Background()
	read := func(path string) *sessionv1.ReadFileRangeResponse {
		resp, err := ops.ReadRange(ctx, linesReq(path, 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 400, 0))
		require.NoError(t, err, path)
		return resp
	}

	t.Run("range: BOF and EOF on a small file", func(t *testing.T) {
		resp := read("plain.txt")
		assert.True(t, resp.Bof && resp.Eof)
		assert.Equal(t, "a\nb\n", resp.Content)
		assert.Equal(t, []int64{1, 2}, []int64{resp.StartLine, resp.EndLine})
		empty := read("empty.txt")
		assert.True(t, empty.Bof && empty.Eof)
		assert.Zero(t, empty.EndByte)
	})

	t.Run("range: UTF-8 boundary snaps back", func(t *testing.T) {
		for anchor, want := range map[int64]int64{1: 0, 2: 0, 3: 3, 4: 3, 5: 3} {
			resp, err := ops.ReadRange(ctx, bytesReq("big.txt", anchor, 64, sessionv1.RangeDir_RANGE_DIR_FORWARD))
			require.NoError(t, err)
			assert.Equal(t, want, resp.StartByte, "anchor %d", anchor)
			assert.False(t, resp.NonUtf8)
		}
	})

	t.Run("range: CRLF and BOM keep raw offsets", func(t *testing.T) {
		resp := read("docs/a.md")
		assert.True(t, resp.HasBom)
		assert.Equal(t, int64(120), resp.EndByte)
		assert.NotContains(t, resp.Content, "\r")
		assert.Equal(t, sessionv1.LineEnding_LINE_ENDING_CRLF, resp.EolClass)
	})

	t.Run("range: binary, non-UTF-8 and long line", func(t *testing.T) {
		assert.True(t, read("bin.dat").IsBinary)
		assert.True(t, read("nul_late.dat").IsBinary, "NUL inside the first 8 KB")
		assert.True(t, read("latin1.txt").NonUtf8)
		long := read("huge.txt")
		assert.True(t, long.LongLine)
		assert.True(t, long.PartialLastLine)
		assert.LessOrEqual(t, long.EndByte-long.StartByte, int64(defaultRangeBytes))
	})

	t.Run("range: stale validator is answered with the current one", func(t *testing.T) {
		req := linesReq("plain.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0)
		req.IfValidator = "1:2:3"
		resp, err := ops.ReadRange(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Validator)
		assert.NotEqual(t, "1:2:3", resp.Validator)
	})

	t.Run("range: confinement corpus is refused exactly like GetFileContent", func(t *testing.T) {
		for _, rel := range rangeRefusalPaths {
			_, getErr := ops.GetContent(ctx, "test-session", rel)
			_, rangeErr := ops.ReadRange(ctx, linesReq(rel, 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0))
			require.Error(t, getErr, rel)
			assert.Equal(t, codeOf(getErr), codeOf(rangeErr), rel)
		}
	})

	t.Run("range: capability honesty for READ_LINES and READ_BYTES", func(t *testing.T) {
		flags, ok := ops.policy.Flags.(*stubFilesFlags)
		require.True(t, ok)
		defer func() { flags.rangeRead = true }()
		for _, on := range []bool{true, false} {
			flags.rangeRead = on
			advertised := map[sessionv1.FileOp]bool{}
			for _, op := range ops.Capabilities(ctx).Ops {
				advertised[op] = true
			}
			for _, op := range []sessionv1.FileOp{sessionv1.FileOp_FILE_OP_READ_LINES, sessionv1.FileOp_FILE_OP_READ_BYTES} {
				assert.Equal(t, on, advertised[op], "%s advertised=%v", op, on)
			}
			_, linesErr := ops.ReadRange(ctx, linesReq("plain.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0))
			_, bytesErr := ops.ReadRange(ctx, bytesReq("plain.txt", 0, 5, sessionv1.RangeDir_RANGE_DIR_FORWARD))
			if on {
				assert.NoError(t, linesErr)
				assert.NoError(t, bytesErr)
			} else {
				assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(linesErr))
				assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(bytesErr))
			}
		}
	})
}

func TestReadRange_ShouldRefuseExactlyLikeGetFileContent_When_FlagOffHiddenSymlinkEscapeGitOrPausedWorktree(t *testing.T) {
	dir := buildReadCorpus(t)
	mem := memFromDir(t, dir)
	mem.SetOnRead(func(int) { t.Error("a refused range call must read nothing") })
	flags := &stubFilesFlags{rangeRead: false}
	ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: flags}, strictOver{mem})
	t.Cleanup(ops.analysis.Close)
	ctx := context.Background()

	for _, rel := range append([]string{"plain.txt"}, rangeRefusalPaths...) {
		_, err := ops.ReadRange(ctx, linesReq(rel, 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0))
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "flag off refuses %s before touching the backend", rel)
	}

	flags.rangeRead = true
	for _, rel := range rangeRefusalPaths {
		_, getErr := ops.GetContent(ctx, "test-session", rel)
		_, rangeErr := ops.ReadRange(ctx, linesReq(rel, 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0))
		assert.Equal(t, codeOf(getErr), codeOf(rangeErr), rel)
		if rel == "leak" || rel == "gitlink" || rel == ".git/config" {
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(rangeErr), rel)
		}
	}
}

func TestReadRange_ShouldFollowReadPolicy_When_WorktreeIsGoneFromDisk(t *testing.T) {
	dir := buildReadCorpus(t)
	provider := pausedWorkspace{existing: dir}
	ops := NewFileOps(FilePolicy{Workspace: provider, Flags: &stubFilesFlags{rangeRead: true}}, osRootBackend{})
	t.Cleanup(ops.analysis.Close)
	ctx := context.Background()
	got, getErr := ops.GetContent(ctx, "test-session", "plain.txt")
	rng, rangeErr := ops.ReadRange(ctx, linesReq("plain.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0))
	require.NoError(t, getErr, "reads of a paused session fall back to ExistingDir")
	require.NoError(t, rangeErr)
	assert.Equal(t, got.Content, rng.Content)
}

// pausedWorkspace reports an ActiveDir that is gone, so ExistingDir fell back to the repo root.
type pausedWorkspace struct{ existing string }

func (p pausedWorkspace) GetWorkspace(string) (session.Workspace, error) {
	return session.Workspace{ActiveDir: p.existing + "-gone", ExistingDir: p.existing}, nil
}

func TestReadRange_ShouldReturnIdenticalResponses_When_OsRootAndMemBackends(t *testing.T) {
	dir := buildReadCorpus(t)
	newOps := func(b FileBackend) *FileOps {
		ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: &stubFilesFlags{rangeRead: true}}, b)
		t.Cleanup(ops.analysis.Close)
		return ops
	}
	osOps, memOps := newOps(osRootBackend{}), newOps(memFromDir(t, dir))
	reqs := []*sessionv1.ReadFileRangeRequest{
		linesReq("plain.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 400, 0),
		linesReq("docs/a.md", 3, sessionv1.RangeDir_RANGE_DIR_AROUND, 2, 0),
		linesReq("mixed.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 400, 0),
		linesReq("huge.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 400, 0),
		bytesReq("big.txt", 1000, 99, sessionv1.RangeDir_RANGE_DIR_AROUND),
		bytesReq("latin1.txt", 0, 100, sessionv1.RangeDir_RANGE_DIR_FORWARD),
	}
	for _, req := range reqs {
		want, err := osOps.ReadRange(context.Background(), req)
		require.NoError(t, err)
		got, err := memOps.ReadRange(context.Background(), req)
		require.NoError(t, err)
		want.Validator, got.Validator = "", "" // the inode part is backend-specific
		want.CapsEpoch, got.CapsEpoch = 0, 0
		assert.True(t, proto.Equal(want, got), "%s differs between backends", req.Path)
	}
}
