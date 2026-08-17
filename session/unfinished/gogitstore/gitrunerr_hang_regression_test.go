// gitrunerr_hang_regression_test.go turns the hang described in
// gogitstore_test.go's gitCommandTimeout doc comment into an executable, CI-
// enforced check. Before that fix (gitCommandTimeout + safeexec.CommandContextPG),
// a git subprocess that never exited and ignored SIGTERM would block
// gitRunErr's cmd.CombinedOutput() forever, past go test's own -timeout,
// which only panics the test binary and never signals the child process
// tree. That fix was previously proven only by a one-time manual repro
// recorded in a commit message; this test proves it on every run instead.
package gogitstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGitRunErr_HungSigtermIgnoringGitSubprocess_ReturnsWithinTimeout puts a
// fake `git` on PATH that ignores SIGTERM and never exits on its own, then
// asserts gitRunErr still returns an error within a bounded time rather than
// hanging past it.
func TestGitRunErr_HungSigtermIgnoringGitSubprocess_ReturnsWithinTimeout(t *testing.T) {
	fakeGitDir := t.TempDir()
	// trap '' TERM sets SIG_IGN, which (unlike a custom handler) survives
	// exec — so the sleep that replaces this shell process still ignores
	// SIGTERM too. Only SIGKILL (safeexec's escalation after its grace
	// period) can end it.
	const fakeGitScript = "#!/bin/sh\ntrap '' TERM\nexec sleep 3600\n"
	fakeGitPath := filepath.Join(fakeGitDir, "git")
	if err := os.WriteFile(fakeGitPath, []byte(fakeGitScript), 0o755); err != nil {
		t.Fatalf("failed to write fake git script: %v", err)
	}

	t.Setenv("PATH", fakeGitDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	repoDir := t.TempDir()

	// gitCommandTimeout bounds gitRunErr's own wait; safeexec's SIGKILL
	// escalation (a further grace period after SIGTERM, see
	// safeexec.sigkillGrace) is what actually ends the hung subprocess once
	// that timeout fires, since the fake git ignores SIGTERM. margin is
	// generous slack over both, not a tight bound, since this only needs to
	// prove "bounded", not measure the exact duration.
	const margin = 30 * time.Second
	overallBound := gitCommandTimeout + margin

	done := make(chan error, 1)
	go func() {
		// "status" is deliberately not a retryable command (see
		// gitCommandIsRetryable) so gitRunErr attempts exactly once instead
		// of retrying up to 5x30s.
		done <- gitRunErr(t.Logf, repoDir, "status")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected gitRunErr to fail against a hung fake git binary, got nil error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected a timeout error, got: %v", err)
		}
	case <-time.After(overallBound):
		t.Fatalf("gitRunErr did not return within %s of a hung, SIGTERM-ignoring git subprocess", overallBound)
	}
}
