package server

import (
	"fmt"
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/envtest"
)

// TestMain denies non-loopback dials through http.DefaultTransport and
// scrubs ambient GitHub tokens, so the suite never reaches real hosts with a
// developer's credentials. A blocked dial fails the run (see envtest.NetGuard).
func TestMain(m *testing.M) {
	guard := envtest.DenyNonLoopbackNetwork()
	restore := envtest.ClearAmbientGitHubTokenEnv()
	code := m.Run()
	restore()
	if report := guard.Report(); report != "" {
		fmt.Fprint(os.Stderr, report)
		code = 1
	}
	os.Exit(code)
}
