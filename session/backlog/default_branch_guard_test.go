package backlog

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowedBounceMainBranchUses lists the only production uses of
// domain.BounceMainBranch left in backlog code, keyed by file, with the reason.
// Everything else must call git.ResolveDefaultBranchName(repoPath): a repo's
// default branch is per-repo state (kibitzer is "master"), not a constant.
var allowedBounceMainBranchUses = map[string]string{ //nolint:gochecknoglobals
	"backlog_lifecycle_pr.go": "closeIfSupersededByMain: closes a live PR as superseded; switching would newly activate that on master repos",
}

// TestBacklogCode_should_NotHardcodeDefaultBranch fails when backlog production
// code names the default branch with a "main" literal or domain.BounceMainBranch
// outside the allowlist.
func TestBacklogCode_should_NotHardcodeDefaultBranch(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var violations []string
	bounceUses := map[string]int{}

	walkErr := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "blktest" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if msg, kind := hardcodedBranchUse(fset, n); msg != "" {
				_, allowed := allowedBounceMainBranchUses[filepath.Base(path)]
				if kind == "bounce" && allowed {
					bounceUses[filepath.Base(path)]++
				} else {
					violations = append(violations, msg)
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, walkErr)

	assert.Empty(t, violations, "name the default branch via git.ResolveDefaultBranchName(repoPath), not a literal")
	assert.Equal(t, 1, bounceUses["backlog_lifecycle_pr.go"], "allowlisted BounceMainBranch use count changed; update the allowlist")
}

// hardcodedBranchUse reports a "main" string literal ("literal") or a
// BounceMainBranch selector ("bounce") at n; msg is empty for any other node.
func hardcodedBranchUse(fset *token.FileSet, n ast.Node) (msg, kind string) {
	switch x := n.(type) {
	case *ast.BasicLit:
		if v, err := strconv.Unquote(x.Value); x.Kind == token.STRING && err == nil && v == "main" {
			return fset.Position(x.Pos()).String() + `: "main" literal`, "literal"
		}
	case *ast.SelectorExpr:
		if x.Sel.Name == "BounceMainBranch" {
			return fset.Position(x.Pos()).String() + ": BounceMainBranch", "bounce"
		}
	}
	return "", ""
}
