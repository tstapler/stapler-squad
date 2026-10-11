// Package norawgitcli defines a go/analysis pass that forbids new git CLI call
// sites outside the sanctioned CLI backend and its Runner wiring.
//
// Background: the go-git fork plan (project_plans/go-git-fork-full-git-replacement,
// Story 1.1.4) routes every git operation through session/git/backend so one
// router can choose between the CLI and go-git implementations. A caller that
// shells out to git directly bypasses that router, so migration could silently
// regress. This analyzer blocks new sites; existing ones carry
// //nolint:norawgitcli // migrating, <ticket> and are ratcheted down by
// TestBaselineCount.
//
// Flagged (non-test files only; tests build git fixtures legitimately):
//   - os/exec or executor/safeexec Command/CommandContext/CommandContextPG
//     whose command-name argument is the constant "git"
//   - a Run(ctx, dir, "git", ...) call on a command runner (tmux.CommandRunner)
//   - any call to a function or method named runGitCommand
//
// Limits (documented, not detected): function-value aliases (f := exec.Command),
// non-constant command names, wrapper helpers that take the name as a parameter,
// exec.Cmd struct literals, and indirection through sh -c / env.
//
// Sanctioned packages: session/git/backend/cli and session/gitwiring, plus the
// test-fixture helper packages that build throwaway repos (testutil/gitfixture,
// session/git/internal/gittest).
package norawgitcli

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported analysis.Analyzer for the norawgitcli check.
var Analyzer = &analysis.Analyzer{
	Name:     "norawgitcli",
	Doc:      "forbids git CLI call sites (exec/safeexec constructors, runner.Run(..., \"git\", ...), runGitCommand) outside session/git/backend/cli and session/gitwiring; route git through session/git/backend instead",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// sanctionedPackageSuffixes are the packages allowed to invoke git directly:
// the CLI backend, the Runner implementation wired into it, and test-fixture builders.
var sanctionedPackages = []string{
	modulePath + "/session/git/backend/cli",
	modulePath + "/session/gitwiring",
	modulePath + "/testutil/gitfixture",
	modulePath + "/session/git/internal/gittest",
}

const modulePath = "github.com/tstapler/stapler-squad"

const directive = "nolint:norawgitcli"

// execPackageSuffixes are the packages whose Command* constructors spawn a process.
var execPackageSuffixes = []string{"os/exec", "/executor/safeexec"}

func run(pass *analysis.Pass) (interface{}, error) {
	pkgPath := pass.Pkg.Path()
	for _, p := range sanctionedPackages {
		if pkgPath == p || strings.HasPrefix(pkgPath, p+"/") {
			return nil, nil
		}
	}

	// Directive comments by file and line; used entries are removed so the
	// leftovers are stale (they suppress nothing) and get reported.
	directives := map[string]map[int]token.Pos{}
	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(c.Text, "//")), directive) {
					pos := pass.Fset.Position(c.Pos())
					if directives[pos.Filename] == nil {
						directives[pos.Filename] = map[int]token.Pos{}
					}
					directives[pos.Filename][pos.Line] = c.Pos()
				}
			}
		}
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}
		kind, ok := rawGitCall(call, pass)
		if !ok {
			return
		}
		// A directive on the call's line or the line above suppresses it.
		for _, line := range []int{pos.Line, pos.Line - 1} {
			if _, found := directives[pos.Filename][line]; found {
				delete(directives[pos.Filename], line)
				return
			}
		}
		pass.Reportf(call.Pos(),
			"%s invokes the git CLI directly — route it through session/git/backend (CLI calls belong only in session/git/backend/cli and session/gitwiring); if this is existing code mid-migration add //nolint:norawgitcli // migrating, <ticket>",
			kind)
	})

	for file, lines := range directives {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, pos := range lines {
			pass.Reportf(pos, "stale //%s: no git CLI call on this line or the next; delete it (and lower baselineCount in baseline_test.go)", directive)
		}
	}
	return nil, nil
}

// rawGitCall reports a human-readable kind when call is a forbidden git CLI invocation.
func rawGitCall(call *ast.CallExpr, pass *analysis.Pass) (string, bool) {
	fn := calledFunc(call, pass)
	if fn == nil {
		return "", false
	}
	if fn.Name() == "runGitCommand" && fn.Pkg() != nil && isPackage(fn.Pkg(), "/session/git") {
		return "runGitCommand", true
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return "", false
	}
	switch {
	case isExecConstructor(fn):
		// The command name is the first string parameter (Command: 0, CommandContext: 1).
		idx := firstStringParam(sig)
		if idx >= 0 && isGitArg(call, idx, pass) {
			return fn.Pkg().Name() + "." + fn.Name(), true
		}
	case fn.Name() == "Run" && sig.Recv() != nil && fn.Pkg() != nil && isPackage(fn.Pkg(), "/session/tmux"):
		// tmux.CommandRunner shape: Run(ctx, dir, name string, args ...string).
		if sig.Params().Len() == 4 && sig.Variadic() && isStringParam(sig, 1) && isStringParam(sig, 2) &&
			isGitArg(call, 2, pass) {
			return "runner.Run", true
		}
	}
	return "", false
}

func calledFunc(call *ast.CallExpr, pass *analysis.Pass) *types.Func {
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return nil
	}
	fn, _ := pass.TypesInfo.Uses[id].(*types.Func)
	return fn
}

func isExecConstructor(fn *types.Func) bool {
	if fn.Pkg() == nil || !strings.HasPrefix(fn.Name(), "Command") {
		return false
	}
	for _, s := range execPackageSuffixes {
		if strings.HasSuffix(fn.Pkg().Path(), s) {
			return true
		}
	}
	return false
}

func isPackage(p *types.Package, suffix string) bool {
	return strings.HasSuffix(p.Path(), suffix)
}

func firstStringParam(sig *types.Signature) int {
	for i := 0; i < sig.Params().Len(); i++ {
		if isStringParam(sig, i) {
			return i
		}
	}
	return -1
}

func isStringParam(sig *types.Signature, i int) bool {
	b, ok := sig.Params().At(i).Type().Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// isGitArg reports whether argument idx is a compile-time constant naming git.
func isGitArg(call *ast.CallExpr, idx int, pass *analysis.Pass) bool {
	if idx >= len(call.Args) {
		return false
	}
	tv, ok := pass.TypesInfo.Types[call.Args[idx]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return false
	}
	base := strings.ToLower(path.Base(strings.TrimSpace(constant.StringVal(tv.Value))))
	return base == "git" || base == "git.exe"
}
