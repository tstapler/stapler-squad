// Package nolintcomment provides a shared helper for detecting //nolint directives.
package nolintcomment

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Contains returns true if a //nolint comment containing linterName appears on
// the same line as pos or the immediately preceding line. The comment text must
// start with "nolint" (after stripping "//" and whitespace) to avoid false
// positives on analysistest want annotations that quote nolint directives inside
// expected error messages. linterName may appear anywhere within the directive,
// so //nolint:forbidigo,norawexec correctly suppresses the norawexec check.
func Contains(pass *analysis.Pass, pos token.Pos, linterName string) bool {
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
					text := strings.TrimPrefix(c.Text, "//")
					trimmed := strings.TrimSpace(text)
					if strings.HasPrefix(trimmed, "nolint") && strings.Contains(trimmed, linterName) {
						return true
					}
				}
			}
		}
	}
	return false
}

// Result is the outcome of looking up a //nolint:<name> directive for a site.
type Result int

const (
	None    Result = iota // no directive covers the site
	Missing               // a directive covers the site but gives no reason
	OK                    // a directive with a reason covers the site
)

// Index maps each line of a file to the earliest AST node start on it, so a
// trailing comment can be told apart from a standalone one.
type Index struct {
	file  *token.File
	f     *ast.File
	first map[int]token.Pos
}

// NewIndex builds an Index for f.
func NewIndex(fset *token.FileSet, f *ast.File) *Index {
	file := fset.File(f.Pos())
	first := map[int]token.Pos{}
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, isFile := n.(*ast.File); isFile {
			return true
		}
		l := file.Line(n.Pos())
		if p, ok := first[l]; !ok || n.Pos() < p {
			first[l] = n.Pos()
		}
		return true
	})
	return &Index{file: file, f: f, first: first}
}

// Lookup finds //nolint:<list> <reason> naming linterName on pos's line, or on a
// standalone comment line directly above. A trailing comment (one with code
// before it on its line) never covers the next line. A directive with no reason
// yields Missing.
func (x *Index) Lookup(pos token.Pos, linterName string) Result {
	line := x.file.Line(pos)
	result := None
	for _, cg := range x.f.Comments {
		for _, c := range cg.List {
			cl := x.file.Line(c.Pos())
			p, hasNode := x.first[cl]
			prevStandalone := cl == line-1 && !(hasNode && p < c.Pos())
			if cl != line && !prevStandalone {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, "nolint:") {
				continue
			}
			list, reason, _ := strings.Cut(strings.TrimPrefix(text, "nolint:"), " ")
			if !slices.Contains(strings.Split(list, ","), linterName) {
				continue
			}
			if strings.TrimSpace(reason) != "" {
				return OK
			}
			result = Missing
		}
	}
	return result
}

// Lookup finds the //nolint:<linterName> directive covering pos in pass's files
// (same line, or a standalone comment line above) and reports whether it gives a reason.
func Lookup(pass *analysis.Pass, pos token.Pos, linterName string) Result {
	file := pass.Fset.File(pos)
	for _, f := range pass.Files {
		if pass.Fset.File(f.Pos()) == file {
			return NewIndex(pass.Fset, f).Lookup(pos, linterName)
		}
	}
	return None
}
