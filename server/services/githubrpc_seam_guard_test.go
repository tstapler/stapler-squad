package services

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
)

// githubrpcTestSeams are exported only for tests in other packages.
// SetPRSnapshotSource decides which sessions count as linked to a PR, so no
// production code may call it. A reference counts, so a method value is caught.
var githubrpcTestSeams = map[string]string{
	"SetPRSnapshotSource": "server/services/githubrpc/github_user_nudge.go GitHubUserService.SetPRSnapshotSource",
	"NudgeWiringForTest":  "server/services/githubrpc/wiring.go GitHubUserService.NudgeWiringForTest",
}

func TestGitHubRPCTestSeams_should_HaveNoProductionCaller(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	var callers []string
	walkModuleFuncs(t, moduleRoot(t), func(rel string, fd *ast.FuncDecl) {
		site := rel + " " + funcDisplayName(fd)
		declared[site] = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if _, seam := githubrpcTestSeams[sel.Sel.Name]; seam {
					callers = append(callers, site+" uses "+sel.Sel.Name)
				}
			}
			return true
		})
	})

	assert.Empty(t, callers)
	for name, site := range githubrpcTestSeams {
		assert.True(t, declared[site], "%s is no longer at %q; update this guard", name, site)
	}
}
