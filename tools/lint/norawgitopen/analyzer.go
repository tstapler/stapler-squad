// Package norawgitopen defines a go/analysis pass that detects direct calls to
// go-git's git.PlainOpen or git.PlainOpenWithOptions outside of
// session/git.OpenRepo, the approved wrapper.
//
// Background: neither call enables EnableDotGitCommonDir by default. Without it,
// go-git silently resolves objects/refs for a linked worktree (`git worktree add`)
// against the wrong gitdir — not an error, a real-but-wrong result (e.g. HEAD
// resolving to a stale SHA from before the worktree was created). This was found
// duplicated, and wrong, at every one of 15+ call sites across session/git and
// session/repo_path.go. session/git.OpenRepo is the single source of truth for the
// correct options; every repo open must go through it.
package norawgitopen

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the norawgitopen check.
var Analyzer = &analysis.Analyzer{
	Name:     "norawgitopen",
	Doc:      "detects direct calls to go-git's PlainOpen/PlainOpenWithOptions outside session/git.OpenRepo; use OpenRepo() instead so EnableDotGitCommonDir is always set for correct linked-worktree resolution",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// exemptPackageSuffixes are the package path suffixes allowed to call
// git.PlainOpen/PlainOpenWithOptions directly because they independently
// reproduce the commondir-resolution logic by hand (gogitstore does this
// deliberately, to share a SharedObjectStore across worktrees — see its Open
// doc comment). session/git itself is deliberately NOT exempted here: it's
// where OpenRepo lives, and it's also where every original unfiltered call
// site lived — a package-wide exemption there would mean this analyzer could
// never re-catch the exact bug it exists to prevent. OpenRepo's own call is
// exempted individually via a //nolint:norawgitopen comment instead.
var exemptPackageSuffixes = []string{
	"/session/unfinished/gogitstore",
}

func run(pass *analysis.Pass) (interface{}, error) {
	pkgPath := pass.Pkg.Path()
	for _, suffix := range exemptPackageSuffixes {
		if strings.HasSuffix(pkgPath, suffix) {
			return nil, nil
		}
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		name, ok := rawGitOpenCallName(call, pass)
		if !ok {
			return
		}
		if nolintcomment.Contains(pass, call.Pos(), "norawgitopen") {
			return
		}
		pass.Reportf(call.Pos(),
			"direct call to git.%s — use session/git.OpenRepo() so EnableDotGitCommonDir is always set for correct linked-worktree resolution; add //nolint:norawgitopen with a justification if this genuinely cannot use the wrapper",
			name)
	})

	return nil, nil
}

// rawGitOpenCallName returns ("PlainOpen"|"PlainOpenWithOptions", true) when call
// resolves (via type info) to that function in github.com/go-git/go-git/v5.
func rawGitOpenCallName(call *ast.CallExpr, pass *analysis.Pass) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	name := sel.Sel.Name
	if name != "PlainOpen" && name != "PlainOpenWithOptions" {
		return "", false
	}
	obj, ok := pass.TypesInfo.Uses[sel.Sel]
	if !ok {
		return "", false
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return "", false
	}
	if fn.Pkg() == nil || fn.Pkg().Path() != "github.com/go-git/go-git/v5" {
		return "", false
	}
	return name, true
}
