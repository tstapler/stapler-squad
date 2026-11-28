//go:build !windows

package sshtransport

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/testutil/tmuxreap"
)

func TestMain(m *testing.M) {
	tmuxreap.ReapLeakedTestServers()
	tmuxreap.StartTestServerWatchdog(os.Getpid())
	os.Exit(m.Run())
}
