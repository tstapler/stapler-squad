package services

import (
	"bytes"
	"context"
	"hash/crc32"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const rangeTestDir = "/w"

// rangeOpsOver builds FileOps over mem with files:range-read on; the analysis
// cache is closed with the test.
func rangeOpsOver(t *testing.T, mem *memBackend) *FileOps {
	t.Helper()
	ops := NewFileOps(FilePolicy{
		Workspace: &fakeWorkspaceProvider{existingDir: rangeTestDir},
		Flags:     &stubFilesFlags{rangeRead: true},
	}, mem)
	t.Cleanup(ops.analysis.Close)
	return ops
}

func linesReq(path string, anchor int64, dir sessionv1.RangeDir, want int32, maxBytes int64) *sessionv1.ReadFileRangeRequest {
	return &sessionv1.ReadFileRangeRequest{
		SessionId: "test-session", Path: path, MaxBytes: maxBytes,
		Anchor: &sessionv1.ReadFileRangeRequest_Lines{Lines: &sessionv1.LineAnchor{AnchorLine: anchor, Dir: dir, WantLines: want}},
	}
}

func bytesReq(path string, offset, length int64, dir sessionv1.RangeDir) *sessionv1.ReadFileRangeRequest {
	return &sessionv1.ReadFileRangeRequest{
		SessionId: "test-session", Path: path,
		Anchor: &sessionv1.ReadFileRangeRequest_Bytes{Bytes: &sessionv1.ByteAnchor{Offset: offset, Length: length, Dir: dir}},
	}
}

// waitReady blocks until the background analysis for path is ready.
func waitReady(t *testing.T, ops *FileOps, path string) {
	t.Helper()
	fr, err := ops.readFileFacts("test-session", path)
	require.NoError(t, err)
	_, err = ops.analysis.Await(context.Background(), fr.Src.Key, ops.analysisRunnerFor(fr.Src))
	require.NoError(t, err)
}

func castagnoli(b []byte) uint32 { return crc32.Checksum(b, crc32.MakeTable(crc32.Castagnoli)) }

func TestReadRange_ShouldReturnLineAlignedLfWindowWithRawOffsets_When_CrlfBomFile(t *testing.T) {
	t.Parallel()
	mem := newMemBackend()
	var doc bytes.Buffer
	doc.Write(utf8BOM)
	for i := 1; i <= 10_000; i++ {
		doc.WriteString("row " + strconv.Itoa(i) + "\r\n")
	}
	raw := doc.Bytes()
	mem.AddFile(rangeTestDir, "crlf.txt", raw, readCorpusMTime)
	ops := rangeOpsOver(t, mem)
	ctx := context.Background()

	first, err := ops.ReadRange(ctx, linesReq("crlf.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 3, 0))
	require.NoError(t, err)
	assert.Equal(t, "row 1\nrow 2\nrow 3\n", first.Content, "LF-normalized, BOM stripped")
	assert.True(t, first.HasBom)
	assert.True(t, first.Bof)
	assert.False(t, first.Eof)
	assert.Equal(t, int64(0), first.StartByte)
	rawLen := int64(len(utf8BOM) + len("row 1\r\nrow 2\r\nrow 3\r\n"))
	assert.Equal(t, rawLen, first.EndByte, "offsets are raw: the BOM and CRs count")
	assert.Equal(t, castagnoli(raw[:rawLen]), first.ChunkCrc32C)
	assert.Equal(t, []int64{1, 3}, []int64{first.StartLine, first.EndLine})
	assert.Equal(t, sessionv1.LineEnding_LINE_ENDING_CRLF, first.EolClass)
	assert.False(t, first.Torn)
	assert.Equal(t, rangeValidator(mustInfo(t, ops, "crlf.txt")), first.Validator)

	waitReady(t, ops, "crlf.txt")
	mid, err := ops.ReadRange(ctx, linesReq("crlf.txt", 5000, sessionv1.RangeDir_RANGE_DIR_AROUND, 10, 0))
	require.NoError(t, err)
	assert.False(t, mid.HasBom, "only the first window sets has_bom")
	assert.Equal(t, int64(4995), mid.StartLine)
	assert.Equal(t, int64(5004), mid.EndLine)
	assert.True(t, strings.HasPrefix(mid.Content, "row 4995\nrow 4996\n"))
	assert.True(t, mid.LineCountKnown)
	assert.Equal(t, int64(10_000), mid.LineCount)
	assert.Equal(t, analysisStateReady, mid.Analysis)
	assert.Equal(t, castagnoli(raw[mid.StartByte:mid.EndByte]), mid.ChunkCrc32C)
	assert.Equal(t, NewFileVersionToken(int64(len(raw)), readCorpusMTime.UnixNano(), raw).String(), mid.VersionToken)

	huge, err := ops.ReadRange(ctx, linesReq("crlf.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 100_000, 100<<20))
	require.NoError(t, err)
	assert.Equal(t, int64(maxRangeLines), huge.EndLine-huge.StartLine+1, "want_lines clamped to 4,000")
	big := bytes.Repeat([]byte("0123456789\n"), 200_000)
	mem.AddFile(rangeTestDir, "wide.txt", big, readCorpusMTime)
	clamped, err := ops.ReadRange(ctx, linesReq("wide.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 4000, 100<<20))
	require.NoError(t, err)
	assert.LessOrEqual(t, clamped.EndByte-clamped.StartByte, int64(maxRangeBytes), "max_bytes clamped to 1 MiB")
}

func mustInfo(t *testing.T, ops *FileOps, path string) os.FileInfo {
	t.Helper()
	rr, err := ops.resolve(FileRef{SessionID: "test-session", Path: path}, AccessRead)
	require.NoError(t, err)
	defer func() { _ = rr.Root.Close() }()
	info, err := rr.Root.Lstat(rr.Resolved)
	require.NoError(t, err)
	return info
}

func TestReadRange_ShouldReturnWindowAroundMillionthLine_When_TwoMillionLineFile(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	for i := 1; i <= 2_000_000; i++ {
		buf.WriteString("L")
		buf.WriteString(strconv.Itoa(i))
		buf.WriteByte('\n')
	}
	mem := newMemBackend()
	mem.AddFile(rangeTestDir, "big.log", buf.Bytes(), readCorpusMTime)
	ops := rangeOpsOver(t, mem)
	waitReady(t, ops, "big.log")
	resp, err := ops.ReadRange(context.Background(), linesReq("big.log", 1_000_000, sessionv1.RangeDir_RANGE_DIR_AROUND, 400, 128<<10))
	require.NoError(t, err)
	assert.Equal(t, int64(999_800), resp.StartLine)
	assert.Equal(t, int64(1_000_199), resp.EndLine)
	assert.True(t, strings.HasPrefix(resp.Content, "L999800\n"))
	assert.False(t, resp.Bof)
	assert.False(t, resp.Eof)
	assert.True(t, resp.LineCountKnown)
	assert.Equal(t, int64(2_000_000), resp.LineCount)
	assert.False(t, resp.Torn)
	assert.LessOrEqual(t, resp.EndByte-resp.StartByte, int64(128<<10))
}

func TestReadRange_ShouldSnapByteAnchorToRuneStart_And_SetEofOnLastWindow(t *testing.T) {
	t.Parallel()
	mem := newMemBackend()
	rune4 := "\U0001F600" // F0 9F 98 80
	mem.AddFile(rangeTestDir, "emoji.txt", []byte("ab"+rune4+"cd\n"), readCorpusMTime)
	late := append([]byte(strings.Repeat("a\n", 5000)), 'x', 0, 'y', '\n') // NUL at 10,001, after 8 KB
	mem.AddFile(rangeTestDir, "late_nul.txt", late, readCorpusMTime)
	mem.AddFile(rangeTestDir, "latin1.txt", []byte("caf\xE9\nok\n"), readCorpusMTime)
	mem.AddFile(rangeTestDir, "long.txt", []byte(strings.Repeat("z", 300*1024)+"\nshort\n"), readCorpusMTime)
	ops := rangeOpsOver(t, mem)
	ctx := context.Background()

	for inside := int64(3); inside <= 5; inside++ {
		resp, err := ops.ReadRange(ctx, bytesReq("emoji.txt", inside, 100, sessionv1.RangeDir_RANGE_DIR_FORWARD))
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.StartByte, "anchor %d snaps back to the rune start", inside)
		assert.Equal(t, rune4+"cd\n", resp.Content, "window text begins at the rune")
		assert.True(t, resp.Eof, "last window sets eof")
	}
	tail, err := ops.ReadRange(ctx, bytesReq("emoji.txt", 7, 100, sessionv1.RangeDir_RANGE_DIR_FORWARD))
	require.NoError(t, err)
	assert.True(t, tail.Eof)
	assert.Equal(t, int64(7), tail.StartByte, "ASCII anchor does not move")

	nul, err := ops.ReadRange(ctx, bytesReq("late_nul.txt", 9990, 100, sessionv1.RangeDir_RANGE_DIR_FORWARD))
	require.NoError(t, err)
	require.NotNil(t, nul.ContainsNulAt)
	assert.Equal(t, int64(10_001), *nul.ContainsNulAt)
	assert.False(t, nul.IsBinary, "NUL after the first 8 KB is not binary")

	bad, err := ops.ReadRange(ctx, linesReq("latin1.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0))
	require.NoError(t, err)
	assert.True(t, bad.NonUtf8)

	long, err := ops.ReadRange(ctx, linesReq("long.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0))
	require.NoError(t, err)
	assert.True(t, long.LongLine, "a 300 KiB line switches to byte-window mode")
	assert.True(t, long.PartialLastLine)
	assert.NotEqual(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE, long.ReadOnlyReason)
	assert.LessOrEqual(t, long.EndByte-long.StartByte, int64(defaultRangeBytes))
}

func TestReadRange_ShouldSetTorn_When_FileChangesBetweenFstats_And_ReturnWholeFile_When_PreferFullIfLe(t *testing.T) {
	t.Parallel()
	mem := newMemBackend()
	small := []byte(strings.Repeat("line\n", 60_000))     // 300 KB
	medium := []byte(strings.Repeat("line\n", 1_000_000)) // 5 MB
	mem.AddFile(rangeTestDir, "small.txt", small, readCorpusMTime)
	mem.AddFile(rangeTestDir, "medium.txt", medium, readCorpusMTime)
	ops := rangeOpsOver(t, mem)
	ctx := context.Background()

	full := linesReq("small.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0)
	full.PreferFullIfLe = 1 << 20
	resp, err := ops.ReadRange(ctx, full)
	require.NoError(t, err)
	assert.Equal(t, string(small), resp.Content, "whole file in one response")
	assert.True(t, resp.Bof && resp.Eof)
	assert.Equal(t, NewFileVersionToken(int64(len(small)), readCorpusMTime.UnixNano(), small).String(), resp.VersionToken, "normal token, hash inline")

	over := linesReq("medium.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0)
	over.PreferFullIfLe = 1 << 20
	resp, err = ops.ReadRange(ctx, over)
	require.NoError(t, err)
	assert.Equal(t, int64(10), resp.EndLine, "first window only")
	assert.False(t, resp.Eof)
	assert.True(t, strings.HasSuffix(resp.VersionToken, ":-"), "pending analysis keeps the - token")
	assert.Equal(t, analysisStatePending, resp.Analysis)

	var once atomic.Bool
	mem.SetOnRead(func(n int) {
		if n == 512 && once.CompareAndSwap(false, true) { // the sniff read: an agent rewrites the file between the two fstats
			mem.AddFile(rangeTestDir, "small.txt", append(small, 'x'), readCorpusMTime.Add(time.Second))
		}
	})
	torn, err := ops.ReadRange(ctx, linesReq("small.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0))
	require.NoError(t, err)
	assert.True(t, torn.Torn)

	mem.SetOnRead(nil)
	stale := linesReq("small.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 10, 0)
	stale.IfValidator = resp.Validator
	fresh, err := ops.ReadRange(ctx, stale)
	require.NoError(t, err)
	assert.NotEqual(t, resp.Validator, fresh.Validator, "a stale if_validator is answered with the new one")
	assert.False(t, fresh.Torn)
}

func TestReadRange_ShouldRefuseAnchorlessRequest_And_UnindexedFarLine(t *testing.T) {
	t.Parallel()
	mem := newMemBackend()
	mem.AddFile(rangeTestDir, "a.txt", []byte(strings.Repeat("x\n", 100)), readCorpusMTime)
	ops := rangeOpsOver(t, mem)
	_, err := ops.ReadRange(context.Background(), &sessionv1.ReadFileRangeRequest{SessionId: "test-session", Path: "a.txt"})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	gate := make(chan struct{})
	mem.SetOnRead(func(n int) {
		if n == analyzeChunkBytes {
			<-gate
		}
	})
	_, err = ops.ReadRange(context.Background(), linesReq("a.txt", 50, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "index still pending after the bounded wait")
	close(gate)
}
