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
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported analysis.Analyzer for the nocommandpattern check.
var Analyzer = &analysis.Analyzer{
	Name:     "nocommandpattern",
	Doc:      "requires CommandPattern fields in Rule struct literals to carry a //nolint:commandpattern justification comment; prefer Criteria (AST-based) over regex matching",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

func run(pass *analysis.Pass) (interface{}, error) {
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
			if !hasJustificationComment(pass, kv.Pos()) {
				pass.Reportf(kv.Pos(),
					"CommandPattern set without a //nolint:commandpattern justification comment; prefer Criteria (AST-based) matching, or document why Criteria cannot express this match")
			}
		}
	})

	return nil, nil
}

// hasJustificationComment returns true if a comment that starts with
// "//nolint:commandpattern" (no space between // and nolint) appears on
// the same line as pos or the immediately preceding line.
//
// We require the nolint directive to start at the beginning of the comment
// text (after //) to avoid matching "//nolint:commandpattern" mentioned
// inside analysistest // want annotations.
func hasJustificationComment(pass *analysis.Pass, pos token.Pos) bool {
	fset := pass.Fset
	file := fset.File(pos)
	if file == nil {
		return false
	}
	targetLine := file.Line(pos)

	for _, f := range pass.Files {
		if fset.File(f.Pos()) != file {
			continue
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				commentLine := file.Line(c.Pos())
				if commentLine == targetLine || commentLine == targetLine-1 {
					// Match //nolint:commandpattern at the start of the comment,
					// not merely anywhere inside the text (guards against want annotations).
					text := strings.TrimPrefix(c.Text, "//")
					if strings.HasPrefix(strings.TrimSpace(text), "nolint:commandpattern") {
						return true
					}
				}
			}
		}
	}
	return false
}

var _ token.Pos // keep token import used
