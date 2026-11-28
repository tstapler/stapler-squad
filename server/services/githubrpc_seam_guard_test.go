package services

import (
	"go/ast"
	"strings"
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

// seamUses lists the seams decl refers to, in a function or at package level.
func seamUses(decl ast.Decl) []string {
	var used []string
	ast.Inspect(decl, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if _, seam := githubrpcTestSeams[sel.Sel.Name]; seam {
				used = append(used, sel.Sel.Name)
			}
		}
		return true
	})
	return used
}

func TestGitHubRPCTestSeams_should_HaveNoProductionCaller(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	covered := map[string]bool{}
	var callers []string
	walkModuleFiles(t, moduleRoot(t), func(rel string, f *ast.File) {
		top, _, nested := strings.Cut(rel, "/")
		if !nested {
			top = "root package"
		}
		covered[top] = true
		for _, decl := range f.Decls {
			site := rel + " package level"
			if fd, ok := decl.(*ast.FuncDecl); ok {
				site = rel + " " + funcDisplayName(fd)
				declared[site] = true
			}
			for _, name := range seamUses(decl) {
				callers = append(callers, site+" uses "+name)
			}
		}
	})

	assert.Empty(t, callers)
	for _, top := range []string{"root package", "server", "cmd"} {
		assert.True(t, covered[top], "the walk parsed no file under %q", top)
	}
	for name, site := range githubrpcTestSeams {
		assert.True(t, declared[site], "%s is no longer at %q; update this guard", name, site)
	}
}
