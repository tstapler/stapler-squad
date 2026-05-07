// Command nocommandpattern runs the nocommandpattern static analysis pass as a
// standalone tool.
//
// Usage:
//
//	go run ./tools/lint/nocommandpattern/cmd/nocommandpattern ./pkg/classifier/...
//
// The tool reports any Rule struct literal that sets the CommandPattern field
// without a //nolint:commandpattern justification comment explaining why the
// Criteria (AST-based) approach cannot express the same match.
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/tstapler/stapler-squad/tools/lint/nocommandpattern"
)

func main() {
	singlechecker.Main(nocommandpattern.Analyzer)
}
