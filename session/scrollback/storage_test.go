package scrollback

import (
	"os"
	"path/filepath"
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
