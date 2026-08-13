package scrollback

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePlainJSONL writes N storedEntry lines to path (uncompressed).
func writePlainJSONL(t *testing.T, path string, count int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 1; i <= count; i++ {
		require.NoError(t, enc.Encode(storedEntry{
			Timestamp: int64(i * 1000),
			Sequence:  uint64(i),
			Data:      "data",
		}))
	}
}

// writeGzipJSONL writes N storedEntry lines to path (gzip-compressed).
func writeGzipJSONL(t *testing.T, path string, count int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	gw := gzip.NewWriter(f)
	enc := json.NewEncoder(gw)
	for i := 1; i <= count; i++ {
		require.NoError(t, enc.Encode(storedEntry{
			Timestamp: int64(i * 1000),
			Sequence:  uint64(i),
			Data:      "data",
		}))
	}
	require.NoError(t, gw.Close())
}

// countLines counts the number of non-empty JSON lines in a plain JSONL file.
func countLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			n++
		}
	}
	require.NoError(t, sc.Err())
	return n
}

func TestForkScrollback_SubsetCopied(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scrollback.jsonl")
	dst := filepath.Join(dir, "fork", "scrollback.jsonl")

	writePlainJSONL(t, src, 10)

	require.NoError(t, ForkScrollback(src, 5, dst))

	assert.Equal(t, 5, countLines(t, dst))

	// Verify first and last entry sequence numbers.
	f, _ := os.Open(dst)
	defer f.Close()
	sc := bufio.NewScanner(f)
	var entries []storedEntry
	for sc.Scan() {
		var e storedEntry
		require.NoError(t, json.Unmarshal(sc.Bytes(), &e))
		entries = append(entries, e)
	}
	assert.EqualValues(t, 1, entries[0].Sequence)
	assert.EqualValues(t, 5, entries[4].Sequence)
}

func TestForkScrollback_UpToSeqZero_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scrollback.jsonl")
	dst := filepath.Join(dir, "fork", "scrollback.jsonl")

	writePlainJSONL(t, src, 5)

	require.NoError(t, ForkScrollback(src, 0, dst))

	// upToSeq=0 means condition `entry.Sequence > 0` is true for all entries
	// (sequences start at 1), so nothing is copied.
	assert.Equal(t, 0, countLines(t, dst))
}

func TestForkScrollback_UpToSeqExceedsMax_AllCopied(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scrollback.jsonl")
	dst := filepath.Join(dir, "fork", "scrollback.jsonl")

	writePlainJSONL(t, src, 7)

	require.NoError(t, ForkScrollback(src, 9999, dst))

	assert.Equal(t, 7, countLines(t, dst))
}

func TestForkScrollback_MissingSrc_CreatesEmptyDst(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "fork", "scrollback.jsonl")

	require.NoError(t, ForkScrollback(filepath.Join(dir, "nonexistent.jsonl"), 10, dst))

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.EqualValues(t, 0, info.Size())
}

func TestForkScrollback_GzipSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scrollback.jsonl.gz")
	dst := filepath.Join(dir, "fork", "scrollback.jsonl")

	writeGzipJSONL(t, src, 8)

	require.NoError(t, ForkScrollback(src, 4, dst))

	assert.Equal(t, 4, countLines(t, dst))
}
