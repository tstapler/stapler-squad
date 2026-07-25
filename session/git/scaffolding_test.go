package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScaffoldingExcludePatterns_MatchGitignore asserts every pattern in
// ScaffoldingExcludePatterns is also declared in the repo's .gitignore, so the
// two can't silently drift apart the way backlogExcludePatterns and .gitignore
// did before this test existed. Patterns are compared with leading/trailing "/"
// stripped, since the two files use slightly different (but equivalent) glob
// forms for the same path (e.g. "web-app/.next/" here vs "/web-app/.next" in
// .gitignore).
//
// This only checks the Go-slice -> .gitignore direction. The reverse (every
// .gitignore line also appears in the slice) isn't asserted: .gitignore has
// hundreds of entries unrelated to worktree scaffolding (node_modules, build
// artifacts, etc.) and there's no way to distinguish "belongs in the shared
// exclude list" from "unrelated ignore rule" without re-deriving the list
// itself — the direction that actually matters for correctness (a pattern the
// guard excludes but .gitignore doesn't know about) is the one covered here.
func TestScaffoldingExcludePatterns_MatchGitignore(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".gitignore"))
	require.NoError(t, err, "must be able to read the repo's .gitignore from session/git/")

	lines := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines[strings.Trim(line, "/")] = true
	}

	for _, p := range ScaffoldingExcludePatterns {
		norm := strings.Trim(p, "/")
		assert.True(t, lines[norm], "pattern %q (normalized %q) is in ScaffoldingExcludePatterns but not in .gitignore — add it so ignore rules match the shared exclude list", p, norm)
	}
}
