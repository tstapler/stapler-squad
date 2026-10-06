//go:build !windows

package services

import (
	"os"

	"github.com/tstapler/stapler-squad/testutil/tmuxreap"
)

// reapLeakedTmuxTestServers and startTmuxTestServerWatchdog mirror
// session/tmux/testmain_test.go's TestMain: this package's fixtures spin up
// real tmux test servers (CreateSession/StartSessionDriver/RestoreWithWorkDir),
// and under repeated -count runs a server left behind by an earlier iteration
// or a killed test binary would otherwise accumulate and never get reaped.
func reapLeakedTmuxTestServers() {
	tmuxreap.ReapLeakedTestServers()
}

func startTmuxTestServerWatchdog() {
	tmuxreap.StartTestServerWatchdog(os.Getpid())
}
