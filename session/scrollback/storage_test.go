package scrollback

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileScrollbackStorage_Write_RejectsSessionIDEscapingBasePath verifies
// getFilePath's containment check: a sessionID crafted to traverse outside
// basePath must cause Write to fail with an error rather than writing a
// scrollback file outside the intended directory. sessionID reaches here
// unsanitized from RPC/MCP fields (see getFilePath's doc comment), so a
// caller-supplied ".." segment is the realistic attack shape.
func TestFileScrollbackStorage_Write_RejectsSessionIDEscapingBasePath(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "none", 0)

	const maliciousSessionID = "../../etc/passwd"
	entries := []ScrollbackEntry{{Timestamp: time.Now(), Data: []byte("payload"), Sequence: 1}}

	err := storage.Write(maliciousSessionID, entries)
	require.Error(t, err, "Write must reject a sessionID that escapes basePath")

	// Confirm nothing was written outside basePath at the escaped location.
	escapedPath := filepath.Join(filepath.Dir(basePath), "etc", "passwd", "scrollback.jsonl")
	_, statErr := os.Stat(escapedPath)
	assert.True(t, os.IsNotExist(statErr), "no file should have been created at the escaped path %s", escapedPath)
}

// TestFileScrollbackStorage_Write_LegitimateSessionID_Succeeds is the control
// case: a normal sessionID must still be writable and readable under
// basePath.
func TestFileScrollbackStorage_Write_LegitimateSessionID_Succeeds(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "none", 0)

	const sessionID = "legit-session-123"
	entries := []ScrollbackEntry{{Timestamp: time.Now(), Data: []byte("hello world"), Sequence: 1}}

	err := storage.Write(sessionID, entries)
	require.NoError(t, err)

	wantPath := filepath.Join(basePath, sessionID, "scrollback.jsonl")
	_, statErr := os.Stat(wantPath)
	require.NoError(t, statErr, "scrollback file should exist at %s", wantPath)

	got, err := storage.Read(sessionID, 0, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hello world", string(got[0].Data))
}

// TestFileScrollbackStorage_Read_RejectsSessionIDEscapingBasePath mirrors the
// Write case for the read path -- Read must also refuse to resolve a path
// outside basePath rather than silently returning no rows.
func TestFileScrollbackStorage_Read_RejectsSessionIDEscapingBasePath(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "none", 0)

	_, err := storage.Read("../../etc/passwd", 0, 10)
	require.Error(t, err, "Read must reject a sessionID that escapes basePath")
}

// failingWriteCloser creates the real temp file (so leak checks are meaningful) but
// fails every Write, standing in for a full disk or broken pipe.
type failingWriteCloser struct{ f *os.File }

func (w failingWriteCloser) Write([]byte) (int, error) { return 0, syscall.EPIPE }
func (w failingWriteCloser) Close() error              { return w.f.Close() }

// TestFileScrollbackStorage_Truncate_AbortsOnCompressorCloseFailure covers the fix where
// a failed compressor Close() during Truncate aborts before the os.Rename that would
// otherwise replace the original file with truncated/corrupt data. It targets zstd
// because klauspost/compress's zstd Writer buffers all Write() calls and only flushes on
// Close(), so every Encode() succeeds and only the final Close() observes the write
// failure. The failure is injected through openTempFile; an earlier version raced a
// FIFO reader's close() against Truncate's write, which lost under whole-package load.
func TestFileScrollbackStorage_Truncate_AbortsOnCompressorCloseFailure(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "zstd", 3)

	const sessionID = "truncate-close-failure"
	var entries []ScrollbackEntry
	for i := 0; i < 50; i++ {
		entries = append(entries, ScrollbackEntry{
			Timestamp: time.Now(),
			Data:      []byte("some scrollback line of text, padded to make the file non-trivially sized"),
			Sequence:  uint64(i),
		})
	}
	require.NoError(t, storage.Write(sessionID, entries))

	filePath, err := storage.getFilePath(sessionID)
	require.NoError(t, err)
	originalContent, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.NotEmpty(t, originalContent)

	storage.openTempFile = func(path string) (io.WriteCloser, error) {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if openErr != nil {
			return nil, openErr
		}
		return failingWriteCloser{f}, nil
	}

	// Halving the real compressed size keeps keepBytes below the stat.Size() early
	// return and above the keepCount == 0 case (no Encode, nothing for Close to flush).
	keepBytes := int64(len(originalContent)) / 2
	require.Greater(t, keepBytes, int64(0))

	err = storage.Truncate(sessionID, keepBytes)
	require.Error(t, err, "Truncate must return an error when the zstd writer's Close() fails")
	assert.Contains(t, err.Error(), "failed to close zstd writer")

	afterContent, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)
	assert.Equal(t, originalContent, afterContent, "original scrollback file must be unchanged after a failed Truncate")

	_, statErr := os.Stat(filePath + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "temp path should have been removed even though Truncate failed")
}
