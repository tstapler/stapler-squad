package sessioncreate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestResolveSessionType_ExplicitDirectory(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
	}
	got, err := resolveSessionType(msg, "")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeDirectory, got)
}

func TestResolveSessionType_ExplicitNewWorktree(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
	}
	got, err := resolveSessionType(msg, "my-branch")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeNewWorktree, got)
}

func TestResolveSessionType_ExplicitExistingWorktree(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE,
		ExistingWorktree: "/some/worktree",
	}
	got, err := resolveSessionType(msg, "")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeExistingWorktree, got)
}

func TestResolveSessionType_UnspecifiedDefaultsToDirectory(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
	}
	got, err := resolveSessionType(msg, "")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeDirectory, got)
}

func TestResolveSessionType_UnspecifiedBranchInfersNewWorktree(t *testing.T) {
	t.Parallel()
	// Backward-compat: a resolved branch with no explicit session_type → new_worktree.
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
	}
	got, err := resolveSessionType(msg, "feat/my-feature")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeNewWorktree, got)
}

func TestResolveSessionType_UnspecifiedExistingWorktreeInfersExistingWorktree(t *testing.T) {
	t.Parallel()
	// Backward-compat: ExistingWorktree field present → existing_worktree (takes priority over branch).
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
		ExistingWorktree: "/path/to/worktree",
	}
	got, err := resolveSessionType(msg, "feat/branch")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeExistingWorktree, got)
}

func TestResolveSessionType_OneOff_ReturnsSessionTypeOneOff(t *testing.T) {
	t.Parallel()
	// SESSION_TYPE_ONE_OFF maps to SessionTypeOneOff (caller converts to directory after path gen).
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
	}
	got, err := resolveSessionType(msg, "some-branch")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionTypeOneOff, got)
}

func TestResolveSessionType_should_ReturnError_When_SessionTypeUnrecognized(t *testing.T) {
	t.Parallel()
	// An unrecognized proto enum value must fail loudly instead of silently
	// downgrading to SessionTypeDirectory (worktree-envvars-hijack Epic 3.1).
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType(999),
	}
	got, err := resolveSessionType(msg, "")
	require.Error(t, err)
	assert.Equal(t, domain.SessionType(""), got)
}

// TestRequiresExplicitPath covers requiresExplicitPath's exemption branches,
// extracted from CreateSession's inline path-validation check. The "autonomous
// mode is exempt" case below is a regression test for a bug where autonomous
// sessions created via the omnibar (SessionType=DIRECTORY, AutonomousMode=true,
// Path="") were rejected with "path is required": the guard only exempted
// SESSION_TYPE_ONE_OFF, not AutonomousMode, even though the omnibar always sends
// an empty path for autonomous sessions and relies on the server to generate a
// scratch directory (mirroring one-off). This used to be verified via a full
// CreateSession -> tmux -> SessionDriver round trip (destroyCreatedSession ->
// JoinSessionDriver); see BUG-099 for why that made it reliably flaky.
func TestRequiresExplicitPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  *sessionv1.CreateSessionRequest
		want bool
	}{
		{
			name: "directory session with empty path requires one",
			msg: &sessionv1.CreateSessionRequest{
				SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
				Path:        "",
			},
			want: true,
		},
		{
			name: "directory session with a path does not require one",
			msg: &sessionv1.CreateSessionRequest{
				SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
				Path:        "/some/path",
			},
			want: false,
		},
		{
			name: "one-off is exempt regardless of path",
			msg: &sessionv1.CreateSessionRequest{
				SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
				Path:        "",
			},
			want: false,
		},
		{
			name: "autonomous mode is exempt",
			msg: &sessionv1.CreateSessionRequest{
				SessionType:    sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
				AutonomousMode: true,
				Path:           "",
			},
			want: false,
		},
		{
			name: "alias name is exempt",
			msg: &sessionv1.CreateSessionRequest{
				SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
				AliasName:   "my-alias",
				Path:        "",
			},
			want: false,
		},
		{
			name: "new project is exempt",
			msg: &sessionv1.CreateSessionRequest{
				SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
				Path:        "",
			},
			want: false,
		},
		{
			name: "restart from session id is exempt",
			msg: &sessionv1.CreateSessionRequest{
				SessionType:          sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
				RestartFromSessionId: "abc123",
				Path:                 "",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, requiresExplicitPath(tt.msg))
		})
	}
}

// TestNeedsGeneratedOneOffPath covers needsGeneratedOneOffPath's branches,
// extracted from CreateSession's inline directory-generation check, plus a
// generateOneOffPath call proving the generated directory actually lands on
// disk. The "autonomous with empty resolved path generates" case is a
// regression test: like one-off sessions, an autonomous session with an empty
// path must get a generated scratch directory rather than being rejected before
// directory-generation logic runs. This used to be verified via a full
// CreateSession -> tmux -> SessionDriver round trip (destroyCreatedSession ->
// JoinSessionDriver); see BUG-099 for why that made it reliably flaky (the
// driver goroutine never exited within any timeout tried).
func TestNeedsGeneratedOneOffPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		msg          *sessionv1.CreateSessionRequest
		resolvedPath string
		want         bool
	}{
		{
			name:         "one-off always generates, even with a resolved path",
			msg:          &sessionv1.CreateSessionRequest{SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF},
			resolvedPath: "/already/resolved",
			want:         true,
		},
		{
			name:         "autonomous with empty resolved path generates",
			msg:          &sessionv1.CreateSessionRequest{AutonomousMode: true},
			resolvedPath: "",
			want:         true,
		},
		{
			name:         "autonomous with an explicit resolved path does not generate",
			msg:          &sessionv1.CreateSessionRequest{AutonomousMode: true},
			resolvedPath: "/explicit/path",
			want:         false,
		},
		{
			name:         "non-autonomous directory session with empty path does not generate",
			msg:          &sessionv1.CreateSessionRequest{SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY},
			resolvedPath: "",
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, needsGeneratedOneOffPath(tt.msg, tt.resolvedPath))
		})
	}
}

func TestGenerateOneOffPath(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	cfg := &config.Config{OneOffBaseDir: baseDir}

	first, err := generateOneOffPath(cfg)
	require.NoError(t, err)
	second, err := generateOneOffPath(cfg)
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "successive calls should generate distinct directories")
	for _, p := range []string{first, second} {
		info, statErr := os.Stat(p)
		require.NoError(t, statErr)
		assert.True(t, info.IsDir())
		assert.Equal(t, baseDir, filepath.Dir(p))
	}
}

func TestGenerateOneOffPath_BadBaseDir_ReturnsError(t *testing.T) {
	t.Parallel()
	// Point OneOffBaseDir at a file (not a directory) so the directory cannot be created.
	baseDirParent := t.TempDir()
	baseDirAsFile := filepath.Join(baseDirParent, "oneoff")
	require.NoError(t, os.WriteFile(baseDirAsFile, []byte("not a directory"), 0o644))

	cfg := &config.Config{OneOffBaseDir: filepath.Join(baseDirAsFile, "nested")}

	_, err := generateOneOffPath(cfg)
	require.Error(t, err)
}

// TestCreateSessionTimeout_ComfortablyAboveGitCloneBound guards against the
// constant silently regressing below the documented worst case (git clone up
// to ~120s) — see the comment on CreateSessionTimeout.
func TestCreateSessionTimeout_ComfortablyAboveGitCloneBound(t *testing.T) {
	t.Parallel()
	assert.Greater(t, CreateSessionTimeout, 120*time.Second)
}
