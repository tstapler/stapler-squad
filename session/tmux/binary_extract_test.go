package tmux

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractBinary_WritesExecutable(t *testing.T) {
	dir := t.TempDir()

	path, err := extractBinary(dir, []byte("v1"))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(got))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100, "extracted file must be executable")
}

func TestExtractBinary_SameContentIsNotRewritten(t *testing.T) {
	dir := t.TempDir()

	first, err := extractBinary(dir, []byte("v1"))
	require.NoError(t, err)
	before, err := os.Stat(first)
	require.NoError(t, err)

	second, err := extractBinary(dir, []byte("v1"))
	require.NoError(t, err)
	after, err := os.Stat(second)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.True(t, os.SameFile(before, after), "unchanged content must keep the same inode")
}

// A newer version must land beside the old file, never over it: an old tmux
// server may still be running from the old path (--tmux-keep-server).
func TestExtractBinary_NewContentLeavesOldFileUntouched(t *testing.T) {
	dir := t.TempDir()

	oldPath, err := extractBinary(dir, []byte("v1"))
	require.NoError(t, err)
	oldBefore, err := os.Stat(oldPath)
	require.NoError(t, err)

	// Same length as v1: a length-only staleness check would wrongly skip this.
	newPath, err := extractBinary(dir, []byte("v2"))
	require.NoError(t, err)

	assert.NotEqual(t, oldPath, newPath)
	oldAfter, err := os.Stat(oldPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(oldBefore, oldAfter))
	oldBytes, err := os.ReadFile(oldPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(oldBytes))
	newBytes, err := os.ReadFile(newPath)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(newBytes))
}

func TestExtractBinary_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()

	_, err := extractBinary(dir, []byte("v1"))
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}
