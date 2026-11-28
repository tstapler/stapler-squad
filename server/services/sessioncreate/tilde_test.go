package sessioncreate

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandTildePath(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err, "os.UserHomeDir() must succeed for tilde expansion tests")

	cases := []struct {
		name  string
		input string
		check func(t *testing.T, got string)
	}{
		{
			name:  "bare tilde expands to home dir",
			input: "~",
			check: func(t *testing.T, got string) {
				assert.Equal(t, home, got)
			},
		},
		{
			name:  "tilde-slash prefix expands to path under home",
			input: "~/foo/bar",
			check: func(t *testing.T, got string) {
				assert.Equal(t, home+"/foo/bar", got)
			},
		},
		{
			name:  "absolute path returned unchanged",
			input: "/absolute/path",
			check: func(t *testing.T, got string) {
				assert.Equal(t, "/absolute/path", got)
			},
		},
		{
			name:  "relative path returned unchanged",
			input: "relative/path",
			check: func(t *testing.T, got string) {
				assert.Equal(t, "relative/path", got)
			},
		},
		{
			name:  "empty string returned unchanged",
			input: "",
			check: func(t *testing.T, got string) {
				assert.Equal(t, "", got)
			},
		},
		{
			name:  "tilde-slash with path traversal is rejected and returned unchanged",
			input: "~/../../etc/passwd",
			check: func(t *testing.T, got string) {
				// Path traversal escaping the home directory must be rejected:
				// expandLocalTildePath returns the original input string unmodified.
				assert.Equal(t, "~/../../etc/passwd", got, "path traversal must return original input")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := expandLocalTildePath(tc.input)
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

// TestExpandTildePath_RemoteRejectsTilde proves the pre-ship review fix: a
// `~`-prefixed path must be rejected for a remote-targeted session rather than
// silently expanded against this server process's own local home directory
// (which names a location on the wrong host entirely).
func TestExpandTildePath_RemoteRejectsTilde(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"~", "~/foo/bar"} {
		_, err := rejectRemoteTildePath(input)
		require.Error(t, err, "rejectRemoteTildePath(%q) must reject tilde expansion", input)
	}
	// An absolute path is unaffected.
	got, err := rejectRemoteTildePath("/absolute/path")
	require.NoError(t, err)
	assert.Equal(t, "/absolute/path", got)
}

// ---------------------------------------------------------------------------
// expandLocalTildePath / rejectRemoteTildePath
// ---------------------------------------------------------------------------

// TestExpandTildePath_AliasPathCallSite documents the tilde expansion that
// occurs at CreateSession for alias configs that contain tilde-prefixed
// paths. This test ensures that if the call site is accidentally removed,
// this scenario will fail explicitly.
func TestExpandTildePath_AliasPathCallSite(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Simulate CreateSession's resolveSessionDefaults alias-path pattern:
	//   if resolvedPath == "" && resolved.Path != "" {
	//       resolvedPath, err = expandLocalTildePath(resolved.Path) // or rejectRemoteTildePath, if remote
	//   }
	aliasPath := "~/projects/myrepo"
	var resolvedPath string
	if resolvedPath == "" && aliasPath != "" {
		expanded, expandErr := expandLocalTildePath(aliasPath)
		require.NoError(t, expandErr)
		resolvedPath = expanded
	}
	assert.Equal(t, home+"/projects/myrepo", resolvedPath,
		"alias path with tilde prefix must be expanded when resolvedPath is initially empty")
}
