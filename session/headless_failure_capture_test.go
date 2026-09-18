package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteHeadlessFailureCapture_should_WriteFullRawOutput_When_UnderMaxBytes
// guards the core fix: previously a headless triage/review parse failure only
// ever logged a ~200-byte preview of the raw LLM output — the full text was
// discarded. This asserts the full raw text survives to disk, not a preview.
func TestWriteHeadlessFailureCapture_should_WriteFullRawOutput_When_UnderMaxBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := strings.Repeat("x", 500) + "\nnot valid json at all"

	path, err := WriteHeadlessFailureCapture(dir, "session-abc", raw, DefaultHeadlessFailureCaptureMaxBytes)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, raw, string(content), "the full raw output must be persisted, not a truncated preview")
	assert.True(t, filepath.IsAbs(path), "returned path should be absolute so it can be opened directly")
}

// TestWriteHeadlessFailureCapture_should_KeepTail_When_ExceedsMaxBytes covers the
// truncation direction: for a parse failure the interesting content (the malformed
// JSON, an error message) is almost always near the END of a long LLM response, so
// truncation must drop the head and keep the tail — the opposite would silently
// discard exactly the bytes a human needs to diagnose the failure.
func TestWriteHeadlessFailureCapture_should_KeepTail_When_ExceedsMaxBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := strings.Repeat("A", 100) + strings.Repeat("B", 100)

	path, err := WriteHeadlessFailureCapture(dir, "session-xyz", raw, 100)
	require.NoError(t, err)

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.True(t, strings.HasSuffix(string(content), strings.Repeat("B", 100)), "must keep the tail (most recent output)")
	assert.False(t, strings.Contains(string(content), strings.Repeat("A", 100)), "must drop the head once truncated")
	assert.Contains(t, string(content), headlessFailureCaptureTruncationMarker)
}

// TestWriteHeadlessFailureCapture_should_ReturnNoop_When_RawIsEmpty asserts an empty
// capture is a deliberate no-op (nothing useful to persist), not an error.
func TestWriteHeadlessFailureCapture_should_ReturnNoop_When_RawIsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := WriteHeadlessFailureCapture(dir, "session-empty", "", DefaultHeadlessFailureCaptureMaxBytes)
	require.NoError(t, err)
	assert.Empty(t, path)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "no file should be written for an empty capture")
}

// TestWriteHeadlessFailureCapture_should_CreateDir_When_MissingAndPersistAcrossReads
// asserts the capture dir is created on demand (mirroring TriageArtifactDirOrDefault's
// MkdirAll precedent) and that the file survives being read multiple times (durable,
// not cleaned up like WriteReviewTranscriptFile's ephemeral repo-checkout file).
func TestWriteHeadlessFailureCapture_should_CreateDir_When_MissingAndPersistAcrossReads(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "headless-failures")

	path, err := WriteHeadlessFailureCapture(dir, "session-durable", "some raw output", DefaultHeadlessFailureCaptureMaxBytes)
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		_, statErr := os.Stat(path)
		require.NoError(t, statErr, "capture file must still exist on repeated reads (no cleanup)")
	}
}
