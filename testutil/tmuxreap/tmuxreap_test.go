//go:build !windows

package tmuxreap

import "testing"

// Regression coverage for the leaked-test-tmux-server class of bug: a reaper
// silently failed to recognize "test-isolated-<pid>" sockets (the name
// testSocketOnce in session/tmux generates, shared by every package's
// tests), so orphaned isolated servers from a SIGKILLed test binary
// accumulated indefinitely instead of being reaped on the next run.

func TestIsTestSocketName_MatchesSharedIsolatedSocketPrefix(t *testing.T) {
	if !isTestSocketName("test-isolated-239479") {
		t.Fatal("isTestSocketName(\"test-isolated-239479\") = false, want true — " +
			"the shared per-process isolated socket name (session/tmux testSocketOnce) must be reapable")
	}
}

func TestIsTestSocketName_StillMatchesUnderscorePrefixes(t *testing.T) {
	if !isTestSocketName("test_recovery_1234") {
		t.Fatal("isTestSocketName(\"test_recovery_1234\") = false, want true")
	}
	if isTestSocketName("staplersquad_keepalive") {
		t.Fatal("isTestSocketName(\"staplersquad_keepalive\") = true, want false — must never match a real session")
	}
}

// Regression coverage for BUG-105: testutil.CreateIsolatedTmuxServer and
// session/session_creation_test.go's testTmuxSocket helper both embed an
// arbitrary Go test name between "test_" and the owning PID
// ("test_<TestName>_<pid>[_<n>]"), so no fixed literal prefix can name every
// case — the allowlist must match on the bare "test_" prefix instead.
func TestIsTestSocketName_MatchesGenericTestNamePrefix(t *testing.T) {
	cases := []string{
		"test_TestSessionCreationWithWorktree_67798_1",
		"test_TestFromInstanceData_ActiveSession_DetectsAlreadyRunningTmux_67798",
	}
	for _, name := range cases {
		if !isTestSocketName(name) {
			t.Errorf("isTestSocketName(%q) = false, want true", name)
		}
	}
}

func TestExtractTestSocketPID_HyphenDelimited(t *testing.T) {
	pid, ok := extractTestSocketPID("test-isolated-239479")
	if !ok || pid != 239479 {
		t.Fatalf("extractTestSocketPID(\"test-isolated-239479\") = (%d, %v), want (239479, true)", pid, ok)
	}
}

func TestExtractTestSocketPID_UnderscoreDelimited(t *testing.T) {
	pid, ok := extractTestSocketPID("test_recovery_239479_1")
	if !ok || pid != 239479 {
		t.Fatalf("extractTestSocketPID(\"test_recovery_239479_1\") = (%d, %v), want (239479, true)", pid, ok)
	}
}

func TestExtractTestSocketPID_RejectsOutOfRangeNumbers(t *testing.T) {
	// A component larger than pidMax (e.g. a nanosecond timestamp) must never
	// be mistaken for a PID.
	if _, ok := extractTestSocketPID("test-isolated-99999999999"); ok {
		t.Fatal("extractTestSocketPID accepted a number outside the valid PID range")
	}
}
