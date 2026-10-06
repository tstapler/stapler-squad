//go:build !windows && !linux

package safeexec

import (
	"os"
	"testing"
)

// TestMain here mirrors safeexec_pdeathsig_linux_test.go's Linux-only
// TestMain: that file is tagged "linux" and doesn't compile on Darwin, so a
// non-Linux TestMain is needed to dispatch the sigkill re-exec helper. Go
// permits only one TestMain per test binary per platform, hence the split.
func TestMain(m *testing.M) {
	if os.Getenv(sigkillHelperEnvVar) == "1" {
		runSigkillHelperProcess()
		return
	}
	os.Exit(m.Run())
}
