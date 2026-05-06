// Command norawexec runs the norawexec static analysis pass as a standalone tool.
//
// Usage:
//
//	norawexec ./session/...
//
// The tool reports direct calls to os/exec.Command and os/exec.CommandContext
// outside of the approved executor wrapper packages. These calls lack WaitDelay,
// which causes zombie process accumulation when contexts expire and grandchildren
// hold pipes open. Use safeexec.CommandContext() instead.
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/tstapler/stapler-squad/tools/lint/norawexec"
)

func main() {
	singlechecker.Main(norawexec.Analyzer)
}
