package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrelateCandidate_ReturnsResolvedPIDExact_When_DetectFindsOneMatchByPID(t *testing.T) {
	tmpHome := t.TempDir()
	uuid := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	projectDir := "-Users-alice-myproject"
	historyPath := filepath.Join(tmpHome, ".claude", "projects", projectDir, uuid+".jsonl")

	inspector := &mockProcessInspector{
		files: []string{historyPath},
	}
	detector := NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)

	candidate := ExternalSessionCandidate{
		SourceKind: MuxDiscovered,
		Path:       "/Users/alice/myproject",
		PID:        1234,
	}

	result, err := CorrelateCandidate(detector, candidate)
	require.NoError(t, err)
	assert.Equal(t, CorrelationResolved, result.Kind)
	assert.Equal(t, uuid, result.UUID)
	assert.Equal(t, ConfidencePIDExact, result.Confidence)
	assert.Empty(t, result.Candidates)
}

func TestCorrelateCandidate_ReturnsAmbiguous_When_PathDetectionFindsTwoCandidates(t *testing.T) {
	tmpHome := t.TempDir()
	projectPath := "/Users/alice/ambiguousproject"
	projectDir := ClaudeProjectDirName(projectPath)
	dir := filepath.Join(tmpHome, ".claude", "projects", projectDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	uuid1 := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	uuid2 := "11111111-2222-3333-4444-555555555555"
	require.NoError(t, os.WriteFile(filepath.Join(dir, uuid1+".jsonl"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, uuid2+".jsonl"), []byte("{}"), 0o644))

	// No PID supplied (dead/unmanaged process with no open Claude fd), so
	// CorrelateCandidate must fall back to path detection.
	inspector := &mockProcessInspector{files: nil}
	detector := NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)

	candidate := ExternalSessionCandidate{
		SourceKind: MuxDiscovered,
		Path:       projectPath,
		PID:        0,
	}

	result, err := CorrelateCandidate(detector, candidate)
	require.NoError(t, err)
	assert.Equal(t, CorrelationAmbiguous, result.Kind)
	assert.Empty(t, result.UUID)
	assert.Equal(t, ConfidenceNone, result.Confidence)
	assert.Len(t, result.Candidates, 2)
}

func TestCorrelateCandidate_ReturnsNotFound_When_NeitherPIDNorPathMatch(t *testing.T) {
	tmpHome := t.TempDir()
	inspector := &mockProcessInspector{files: nil}
	detector := NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)

	candidate := ExternalSessionCandidate{
		SourceKind: MuxDiscovered,
		Path:       "/Users/alice/nonexistentproject",
		PID:        0,
	}

	result, err := CorrelateCandidate(detector, candidate)
	require.NoError(t, err)
	assert.Equal(t, CorrelationNotFound, result.Kind)
	assert.Empty(t, result.UUID)
	assert.Equal(t, ConfidenceNone, result.Confidence)
	assert.Empty(t, result.Candidates)
}
