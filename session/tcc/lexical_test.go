package tcc

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLexicalPolicy_DefaultPathsAndUnknown(t *testing.T) {
	t.Parallel()
	classify := LexicalPolicy("/Users/test")
	for _, p := range []string{"/Users/test/code/repo", "/Users/test/.stapler-squad/worktrees/a", "/Users/test", "/Users"} {
		require.False(t, classify(p).Protected, p)
	}
	for _, p := range []string{"/Users/test/Documents/repo", "/Users/test/Desktop/repo", "/Users/test/code-other/repo", "/Volumes/unknown/repo", "/network/repo"} {
		require.True(t, classify(p).Protected, p)
	}
	require.Equal(t, "/Users/test/Documents", classify("/Users/test/Documents/repo").Root)
	require.True(t, classify("/Users/test/Code/repo").Protected)
	require.True(t, classify("/Users/test/.stapler-squad-other/repo").Protected)
	aliases := LexicalPolicy("/var/home/test")
	require.False(t, aliases("/private/var/home/test/code/repo").Protected)
	require.Equal(t, aliases("/var/home/test/Documents/repo"), aliases("/private/var/home/test/Documents/repo"))
	require.True(t, aliases("/variable/home/test/code/repo").Protected)
}
