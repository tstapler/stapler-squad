package svctest

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// Main is the shared hermetic TestMain body of the server/services test binaries: it returns the exit code.
// hermeticSetup runs after the Claude CLI is isolated and before anything else, so a package can point its own
// package-level seams (for example its DNS lookup) at the hermetic fakes.
func Main(m *testing.M, hermeticSetup func()) int {
	netGuard := envtest.DenyNonLoopbackNetwork()
	restoreClaudeCLI, err := envtest.IsolateClaudeCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		return 1
	}
	hermeticSetup()
	headless.DefaultCapabilitySelfCheck = headless.NewPassedCapabilitySelfCheckForTesting()
	restore := envtest.ClearAmbientGitHubTokenEnv()
	restoreState := envtest.ClearAmbientStaplerSquadStateEnv()
	RecordSessionDriverStarts()
	ReapLeakedTmuxTestServers()
	StartTmuxTestServerWatchdog()
	// Before any test's goleak.IgnoreCurrent() baseline: see the function's doc comment.
	if err := session.WarmTestEntRepositoryTemplate(); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: warm test ent repository template: %v\n", err)
		return 1
	}
	code := m.Run()
	// Checked even when tests failed, so a leak is named rather than hidden behind them.
	if n := AwaitNoSessionDrivers(5 * time.Second); n > 0 {
		fmt.Fprintf(os.Stderr, "TestMain: %d SessionDriver goroutine(s) outlived their tests; "+
			"stop them with SetupForkTestFixture or StopSessionDriverOnCleanup (see driver_leak.go)\n", n)
		recs := LeakedSessionDriverStarts()
		for _, rec := range recs {
			fmt.Fprintf(os.Stderr, "leaked driver: %s\n", rec)
		}
		if len(recs) < n { // e.g. a retry continuation whose original start record reads as stopped
			for _, st := range LiveSessionDriverStacks() {
				fmt.Fprintf(os.Stderr, "live driver goroutine: %s\n", st)
			}
		}
		code = 1
	}
	restoreState()
	restore()
	restoreClaudeCLI()
	if report := netGuard.Report(); report != "" {
		fmt.Fprint(os.Stderr, report)
		code = 1
	}
	return code
}
