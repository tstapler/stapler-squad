package scanstate

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statFile(t *testing.T, path string) (*os.File, os.FileInfo) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	fi, err := f.Stat()
	require.NoError(t, err)
	return f, fi
}

func TestScanComplete_HoldsBackUnterminatedTail(t *testing.T) {
	t.Parallel()
	var lines []string
	consumed, tail, err := ScanComplete(bytes.NewReader([]byte("a\nbb\n\nccc")), 1024, func(l []byte) { lines = append(lines, string(l)) })
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "bb", ""}, lines)
	assert.Equal(t, int64(6), consumed)
	assert.Equal(t, "ccc", string(tail))
}

func TestScanComplete_OversizedLineStopsAfterEarlierLines(t *testing.T) {
	t.Parallel()
	data := append([]byte("ok\n"), append(bytes.Repeat([]byte("x"), 200<<10), '\n')...)
	var lines []string
	consumed, _, err := ScanComplete(bytes.NewReader(data), 64<<10, func(l []byte) { lines = append(lines, string(l)) })
	require.True(t, errors.Is(err, bufio.ErrTooLong))
	assert.Equal(t, []string{"ok"}, lines)
	assert.Equal(t, int64(3), consumed)
}

func TestPosition_ResumableDetectsShrinkRewriteAndAcceptsGrowth(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "f.jsonl")
	orig := bytes.Repeat([]byte("line one\n"), 2000) // > 2 fingerprint windows
	require.NoError(t, os.WriteFile(path, orig, 0o600))
	f, fi := statFile(t, path)
	pos, err := Capture(f, fi, int64(len(orig)))
	require.NoError(t, err)
	assert.True(t, pos.Unchanged(fi))

	// Appended: resumable, not unchanged.
	require.NoError(t, os.WriteFile(path, append(append([]byte{}, orig...), "more\n"...), 0o600))
	f2, fi2 := statFile(t, path)
	assert.False(t, pos.Unchanged(fi2))
	assert.True(t, pos.Resumable(f2, fi2.Size()))

	// Shrunk below the offset.
	require.NoError(t, os.WriteFile(path, orig[:100], 0o600))
	f3, fi3 := statFile(t, path)
	assert.False(t, pos.Resumable(f3, fi3.Size()))

	// Replaced by a larger file with different content: only the hash can tell.
	other := bytes.Repeat([]byte("LINE TWO\n"), 3000)
	require.NoError(t, os.WriteFile(path, other, 0o600))
	f4, fi4 := statFile(t, path)
	assert.Greater(t, fi4.Size(), pos.Offset)
	assert.False(t, pos.Resumable(f4, fi4.Size()))
}
