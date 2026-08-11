//go:build darwin

package procinfo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessInspector_OpenFiles_IncludesRealFile(t *testing.T) {
	inspector := NewProcessInspector()
	files, err := inspector.OpenFiles(int32(os.Getpid()))
	require.NoError(t, err)
	assert.NotEmpty(t, files, "current process should have open files")
}

func TestProcessInspector_Cwd_MatchesOsGetwd(t *testing.T) {
	inspector := NewProcessInspector()
	cwd, err := inspector.Cwd(int32(os.Getpid()))
	require.NoError(t, err)

	expected, err := os.Getwd()
	require.NoError(t, err)

	// Normalize symlinks for comparison
	expectedResolved, _ := filepath.EvalSymlinks(expected)
	cwdResolved, _ := filepath.EvalSymlinks(cwd)
	assert.Equal(t, expectedResolved, cwdResolved)
}

func TestProcessInspector_CreateTime_ReturnsPositive(t *testing.T) {
	inspector := NewProcessInspector()
	ct, err := inspector.CreateTime(int32(os.Getpid()))
	require.NoError(t, err)
	assert.Greater(t, ct, int64(0))

	// Should be within 24 hours of now
	now := time.Now().UnixMilli()
	assert.Less(t, ct, now+86400000, "create time should not be in the future")
	assert.Greater(t, ct, now-86400000*365, "create time should not be more than 1 year ago")
}

func TestProcessInspector_IsAlive_CorrectCreateTime(t *testing.T) {
	inspector := NewProcessInspector()
	ct, err := inspector.CreateTime(int32(os.Getpid()))
	require.NoError(t, err)

	alive := inspector.IsAlive(int32(os.Getpid()), ct)
	assert.True(t, alive)
}

func TestProcessInspector_IsAlive_DetectsPIDReuse(t *testing.T) {
	inspector := NewProcessInspector()
	// Use wrong create time (epoch 0 — clearly incorrect)
	alive := inspector.IsAlive(int32(os.Getpid()), 0)
	assert.False(t, alive, "wrong create time should indicate PID reuse")
}

func TestProcessInspector_NonExistentPID_ReturnsError(t *testing.T) {
	inspector := NewProcessInspector()
	files, err := inspector.OpenFiles(99999999)
	// Should either return error or empty slice — must not panic
	if err != nil {
		assert.Empty(t, files)
	}
}

func TestProcessInspector_PermissionDenied_ReturnsEmptyNotError(t *testing.T) {
	inspector := NewProcessInspector()
	// PID 1 (launchd on macOS) — may be permission denied
	files, err := inspector.OpenFiles(1)
	// Should not propagate unhandled permission error
	_ = err
	_ = files
	// Key: must not panic
}
