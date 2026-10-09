package native

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A not-yet-created path must canonicalize to the same string it has once created,
// even when an ancestor is a symlink (macOS /var -> /private/var).
func TestCanonicalizeWorktreePath_should_ResolveExistingAncestor_When_PathDoesNotExist(t *testing.T) {
	t.Parallel()
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))
	realResolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)

	missing := filepath.Join(link, "a", "b", "wt")
	before := CanonicalizeWorktreePath(missing)
	assert.Equal(t, filepath.Join(realResolved, "a", "b", "wt"), before)

	require.NoError(t, os.MkdirAll(missing, 0o755))
	assert.Equal(t, before, CanonicalizeWorktreePath(missing))
}

func TestCanonicalizeWorktreePath_should_ReturnInput_When_EmptyOrRootless(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", CanonicalizeWorktreePath(""))
	assert.Equal(t, filepath.Clean("no-such-rel-dir/x"), CanonicalizeWorktreePath("no-such-rel-dir/x"))
}
