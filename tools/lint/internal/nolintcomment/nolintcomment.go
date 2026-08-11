// Package nolintcomment provides a shared helper for detecting //nolint directives.
package nolintcomment

import (
	"go/token"
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
