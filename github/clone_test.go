package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsSubPath covers the containment helper reused by
// FindExistingClone/GetOrCloneRepository to reject an owner/repo pair whose
// joined clonePath escapes DefaultCloneBase -- including the classic
// prefix-check-without-separator bug class ("/tmp/foo" vs "/tmp/foobar" must
// NOT be treated as "foobar is under foo").
func TestIsSubPath(t *testing.T) {
	t.Run("legitimate subpath", func(t *testing.T) {
		assert.True(t, isSubPath("/tmp/foo", "/tmp/foo/bar/baz"))
	})

	t.Run("exact match", func(t *testing.T) {
		assert.True(t, isSubPath("/tmp/foo", "/tmp/foo"))
	})

	t.Run("dot-dot escape attempt", func(t *testing.T) {
		assert.False(t, isSubPath("/tmp/foo", "/tmp/foo/../../etc/passwd"))
	})

	t.Run("sibling directory sharing a string prefix", func(t *testing.T) {
		// base="/tmp/foo", candidate="/tmp/foobar" -- foobar is NOT under foo,
		// even though the raw strings share a prefix.
		assert.False(t, isSubPath("/tmp/foo", "/tmp/foobar"))
	})
}
