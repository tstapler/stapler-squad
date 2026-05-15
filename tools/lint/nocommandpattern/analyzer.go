// Package nocommandpattern defines a go/analysis pass that detects Rule struct
// literals where the CommandPattern field is set without a justification comment.
//
// In pkg/classifier/classifier.go, approval rules should use the Criteria
// (AST-based) field for command matching rather than CommandPattern (regex).
// CommandPattern is a fallback only for cases where Criteria cannot express
// the match (e.g., matching specific flag values or redirection targets).
//
// Any Rule literal that sets CommandPattern must have a comment containing
// "nolint:commandpattern" on the same line or the immediately preceding line,
// explaining why Criteria cannot be used instead.
//
// This enforces the "prefer AST-based matching over regex" principle
// structurally so every regex usage is explicitly justified.
package nocommandpattern

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the nocommandpattern check.
var Analyzer = &analysis.Analyzer{
	Name:     "nocommandpattern",
	Doc:      "requires CommandPattern fields in Rule struct literals to carry a //nolint:commandpattern justification comment; prefer Criteria (AST-based) over regex matching",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// onlyPackage restricts the check to packages whose import path contains this
// substring. CommandPattern is a pattern defined only in pkg/classifier; there
// is no value checking unrelated packages that happen to have a field with the
// same name.
const onlyPackage = "pkg/classifier"

func run(pass *analysis.Pass) (interface{}, error) {
	if !strings.Contains(pass.Pkg.Path(), onlyPackage) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CompositeLit)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		lit := n.(*ast.CompositeLit)

		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ident, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Name != "CommandPattern" {
				continue
			}

			// CommandPattern is set. Require a justification comment.
			if !nolintcomment.Contains(pass, kv.Pos(), "commandpattern") {
				pass.Reportf(kv.Pos(),
					"CommandPattern set without a //nolint:commandpattern justification comment; prefer Criteria (AST-based) matching, or document why Criteria cannot express this match")
			}
		}
	})

	return nil, nil
}

