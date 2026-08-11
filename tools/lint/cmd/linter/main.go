// Command linter runs all stapler-squad custom lint passes as a single binary.
//
// Usage:
//
//	linter ./...
//	linter -norawexec ./...          # run only norawexec pass
//	linter -hotpolllog ./session/... # run only hotpolllog pass
//
// Passes:
//   - hotpolllog: detects DebugLog/InfoLog calls inside select-case of for loops
//   - nocommandpattern: requires //nolint:commandpattern comment on CommandPattern fields
//   - norawexec: detects direct os/exec.Command calls outside approved wrapper packages
//   - tmuxsocketscope: detects tmux command construction that bypasses socket resolution
//   - silenttransition: detects TransitionBacklogItemStatus/UpdateItemSessionEnded
//     calls whose error is only logged, never surfaced or propagated
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/tstapler/stapler-squad/tools/lint/hotpolllog"
	"github.com/tstapler/stapler-squad/tools/lint/nocommandpattern"
	"github.com/tstapler/stapler-squad/tools/lint/norawexec"
	"github.com/tstapler/stapler-squad/tools/lint/silenttransition"
	"github.com/tstapler/stapler-squad/tools/lint/tmuxsocketscope"
)

func main() {
	multichecker.Main(
		hotpolllog.Analyzer,
		nocommandpattern.Analyzer,
		norawexec.Analyzer,
		silenttransition.Analyzer,
		tmuxsocketscope.Analyzer,
	)
}
