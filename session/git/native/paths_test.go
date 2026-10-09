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

// "lnk/../y" must resolve physically (through the symlink), as git records it,
// not lexically to a sibling of lnk.
func TestCanonicalizeWorktreePath_should_ResolveSymlinkPhysically_When_PathContainsDotDot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target", "deep")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "target", "y"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "y"), 0o755))
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Symlink(target, link))

	want, err := filepath.EvalSymlinks(filepath.Join(root, "target", "y"))
	require.NoError(t, err)
	assert.Equal(t, want, CanonicalizeWorktreePath(link+"/../y"))
}
