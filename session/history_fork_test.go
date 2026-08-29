package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeConvJSONL writes N minimal JSON lines to path.
func writeFakeConvJSONL(t *testing.T, path string, count int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	for i := 1; i <= count; i++ {
		line, _ := json.Marshal(map[string]int{"line": i})
		_, err = fmt.Fprintf(f, "%s\n", line)
		require.NoError(t, err)
	}
}

// countJSONLLines counts non-empty lines in a JSONL file.
func countJSONLLines(t *testing.T, path string) int {
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

func TestForkClaudeConversation_SubsetCopied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "conv.jsonl")
	dstDir := filepath.Join(dir, "fork")

	writeFakeConvJSONL(t, srcPath, 5)

	newUUID, err := ForkClaudeConversation(srcPath, 3, dstDir)

	require.NoError(t, err)
	assert.NotEmpty(t, newUUID)

	dstPath := filepath.Join(dstDir, newUUID+".jsonl")
	assert.Equal(t, 3, countJSONLLines(t, dstPath))
}

func TestForkClaudeConversation_ZeroLineCount_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "conv.jsonl")
	dstDir := filepath.Join(dir, "fork")

	writeFakeConvJSONL(t, srcPath, 5)

	newUUID, err := ForkClaudeConversation(srcPath, 0, dstDir)

	require.NoError(t, err)
	assert.NotEmpty(t, newUUID)

	dstPath := filepath.Join(dstDir, newUUID+".jsonl")
	assert.Equal(t, 0, countJSONLLines(t, dstPath))
}

func TestForkClaudeConversation_LineCountExceedsMax_AllCopied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "conv.jsonl")
	dstDir := filepath.Join(dir, "fork")

	writeFakeConvJSONL(t, srcPath, 4)

	newUUID, err := ForkClaudeConversation(srcPath, 9999, dstDir)

	require.NoError(t, err)
	dstPath := filepath.Join(dstDir, newUUID+".jsonl")
	assert.Equal(t, 4, countJSONLLines(t, dstPath))
}

func TestForkClaudeConversation_MissingSrc_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dstDir := filepath.Join(dir, "fork")

	_, err := ForkClaudeConversation(filepath.Join(dir, "nonexistent.jsonl"), 5, dstDir)

	require.Error(t, err)
}

func TestForkClaudeConversation_OutputFilenameMatchesUUID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "conv.jsonl")
	dstDir := filepath.Join(dir, "fork")

	writeFakeConvJSONL(t, srcPath, 3)

	newUUID, err := ForkClaudeConversation(srcPath, 3, dstDir)

	require.NoError(t, err)
	expectedPath := filepath.Join(dstDir, newUUID+".jsonl")
	_, statErr := os.Stat(expectedPath)
	assert.NoError(t, statErr, "output file should exist at %s", expectedPath)
}

func TestForkClaudeConversation_ReturnsDifferentUUIDs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "conv.jsonl")
	dstDir := filepath.Join(dir, "fork")

	writeFakeConvJSONL(t, srcPath, 2)

	uuid1, err := ForkClaudeConversation(srcPath, 2, dstDir)
	require.NoError(t, err)

	uuid2, err := ForkClaudeConversation(srcPath, 2, dstDir)
	require.NoError(t, err)

	assert.NotEqual(t, uuid1, uuid2, "each fork should produce a unique UUID")
}

// TestForkClaudeConversation_ConcurrentCalls_ProduceValidOutput is a concurrent-load
// sanity check, not a regression test for the tmp-filename write race: dstPath is
// already unique per call via a fresh UUID generated before any I/O, so concurrent
// calls here never share a destination path and this cannot fail under the old
// dstPath+".tmp" fixed-suffix code either. It verifies that concurrent calls with
// distinct destinations each still produce valid, non-corrupt output, mirroring
// config_test.go's TestSaveConfig_ConcurrentWritesToSamePath pattern in shape only.
func TestForkClaudeConversation_ConcurrentCalls_ProduceValidOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "conv.jsonl")
	dstDir := filepath.Join(dir, "fork")
	writeFakeConvJSONL(t, srcPath, 3)

	const n = 20
	var wg sync.WaitGroup
	uuids := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uuids[i], errs[i] = ForkClaudeConversation(srcPath, 3, dstDir)
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for i, err := range errs {
		require.NoError(t, err, "ForkClaudeConversation must not error under concurrent callers targeting the same dstDir")
		assert.False(t, seen[uuids[i]], "each concurrent call must produce a distinct UUID")
		seen[uuids[i]] = true

		outPath := filepath.Join(dstDir, uuids[i]+".jsonl")
		data, readErr := os.ReadFile(outPath)
		require.NoError(t, readErr, "output file for uuid %s must exist", uuids[i])
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			assert.True(t, json.Valid(line), "output for uuid %s must be valid JSON per line, not torn/corrupt: %s", uuids[i], data)
		}
	}
}
