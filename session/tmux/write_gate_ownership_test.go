package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/diagnose"
)

func TestSessionIdentity_Matches_BothFieldsAgree(t *testing.T) {
	id := SessionIdentity{SessionUUID: "abc", TmuxOwnerMarker: "abc"}

	ok, reason := id.Matches("abc")
	if !ok {
		t.Fatalf("expected match, got mismatch with reason %q", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason on match, got %q", reason)
	}
}

func TestSessionIdentity_Matches_SessionUUIDMismatch(t *testing.T) {
	id := SessionIdentity{SessionUUID: "other", TmuxOwnerMarker: "s1"}

	ok, reason := id.Matches("s1")
	if ok {
		t.Fatal("expected mismatch, got match")
	}
	if reason != diagnose.SafetyGateReasonIdentityMismatchInstance {
		t.Errorf("reason = %q, want %q", reason, diagnose.SafetyGateReasonIdentityMismatchInstance)
	}
}

func TestSessionIdentity_Matches_TmuxOwnerMarkerMismatch(t *testing.T) {
	id := SessionIdentity{SessionUUID: "s1", TmuxOwnerMarker: "s2"}

	ok, reason := id.Matches("s1")
	if ok {
		t.Fatal("expected mismatch, got match")
	}
	if reason != diagnose.SafetyGateReasonIdentityMismatchTmuxMarker {
		t.Errorf("reason = %q, want %q", reason, diagnose.SafetyGateReasonIdentityMismatchTmuxMarker)
	}
}

func TestSessionIdentity_Matches_BothFieldsMismatch_ReportsUUIDFirst(t *testing.T) {
	id := SessionIdentity{SessionUUID: "other", TmuxOwnerMarker: "also-other"}

	ok, reason := id.Matches("s1")
	if ok {
		t.Fatal("expected mismatch, got match")
	}
	if reason != diagnose.SafetyGateReasonIdentityMismatchInstance {
		t.Errorf("reason = %q, want %q (UUID check runs first)", reason, diagnose.SafetyGateReasonIdentityMismatchInstance)
	}
}

// newFakeTmuxShowEnvironment writes a fake tmux script (mirroring
// session/orphan_sweep_test.go's fake-tmux harness) that logs every
// invocation's argv to a temp file and, for any `show-environment` call,
// prints outputLine and exits with exitCode. Any other subcommand exits 1 --
// readSessionOwnerMarker never issues one, so this is a deliberate guard
// against a future accidental dependency on some other tmux call.
func newFakeTmuxShowEnvironment(t *testing.T, outputLine string, exitCode int) (fakeTmux, logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "argv.log")
	fakeTmux = filepath.Join(dir, "tmux")

	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> "%s"
case "$*" in
  *show-environment*) echo '%s'; exit %d ;;
esac
exit 1
`, logPath, outputLine, exitCode)
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	return fakeTmux, logPath
}

func TestReadSessionOwnerMarker_ReturnsMarker_WhenPaneEnvVarSet(t *testing.T) {
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=9f2c-work", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	marker, err := readSessionOwnerMarker(context.Background(), "", "stapler-e6c2a88e-work")
	require.NoError(t, err)
	require.Equal(t, "9f2c-work", marker)
}

func TestReadSessionOwnerMarker_TargetsRequestedPaneAndVariable(t *testing.T) {
	fakeTmux, logPath := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=abc", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	_, err := readSessionOwnerMarker(context.Background(), "", "stapler-e6c2a88e-work")
	require.NoError(t, err)

	logBytes, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	logStr := string(logBytes)
	require.Contains(t, logStr, "show-environment")
	require.Contains(t, logStr, "stapler-e6c2a88e-work")
	require.Contains(t, logStr, "STAPLER_SESSION_UUID")
}

func TestReadSessionOwnerMarker_ReturnsError_WhenPaneGone(t *testing.T) {
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "can't find pane", 1)
	t.Setenv("TMUX_BIN", fakeTmux)

	marker, err := readSessionOwnerMarker(context.Background(), "", "stapler-gone")
	require.Error(t, err)
	require.Empty(t, marker)
}

func TestReadSessionOwnerMarker_ReturnsError_WhenMarkerRemovedFromEnvironment(t *testing.T) {
	// tmux(1): a variable removed from the environment is reported prefixed
	// with "-" (e.g. a session whose pane env was explicitly unset) rather
	// than by a nonzero exit -- readSessionOwnerMarker must treat this as
	// absent, not as a literal marker value of "-STAPLER_SESSION_UUID".
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "-STAPLER_SESSION_UUID", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	marker, err := readSessionOwnerMarker(context.Background(), "", "stapler-removed")
	require.Error(t, err)
	require.Empty(t, marker)
}

func TestReadSessionOwnerMarker_ReturnsError_WhenOutputMalformed(t *testing.T) {
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "GARBAGE", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	marker, err := readSessionOwnerMarker(context.Background(), "", "stapler-malformed")
	require.Error(t, err)
	require.Empty(t, marker)
}

func TestReadSessionOwnerMarker_ReturnsError_WhenPaneNameEmpty(t *testing.T) {
	marker, err := readSessionOwnerMarker(context.Background(), "", "")
	require.Error(t, err)
	require.Empty(t, marker)
}

// fakeInstanceIdentitySnapshot satisfies InstanceIdentitySnapshot without
// depending on *session.Instance -- see that interface's doc comment for why
// session/tmux cannot import package session (import cycle).
type fakeInstanceIdentitySnapshot struct {
	uuid string
}

func (f fakeInstanceIdentitySnapshot) SnapshotSessionUUID() string { return f.uuid }

func TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnMatchedIdentity_WhenSnapshotAndTmuxMarkerBothMatchExpectedUUID(t *testing.T) {
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=9f2c-work", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := fakeInstanceIdentitySnapshot{uuid: "9f2c-work"}
	identity, err := verifyIdentityImmediatelyBeforeWrite(context.Background(), inst, "", "stapler-e6c2a88e-work", "9f2c-work")

	require.NoError(t, err)
	require.Equal(t, SessionIdentity{SessionUUID: "9f2c-work", TmuxOwnerMarker: "9f2c-work"}, identity)
}

func TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnTmuxMarkerMismatchReason_WhenPaneMarkerDiffersFromSnapshot(t *testing.T) {
	// The exact ce71ad1a cross-session-misdelivery class this facade exists
	// to close: the Instance object still believes it's the right session,
	// but the pane underneath it has been reused by someone else.
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=different-session", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := fakeInstanceIdentitySnapshot{uuid: "9f2c-work"}
	identity, err := verifyIdentityImmediatelyBeforeWrite(context.Background(), inst, "", "stapler-e6c2a88e-work", "9f2c-work")

	require.Error(t, err)
	require.Equal(t, SessionIdentity{}, identity)
	require.True(t, errors.Is(err, ErrIdentityMismatchTmuxMarker))
	require.False(t, errors.Is(err, ErrIdentityMismatchInstance))
}

func TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnInstanceMismatchReason_WhenSnapshotUUIDDiffersFromExpected(t *testing.T) {
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=9f2c-work", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := fakeInstanceIdentitySnapshot{uuid: "stale-uuid"}
	identity, err := verifyIdentityImmediatelyBeforeWrite(context.Background(), inst, "", "stapler-e6c2a88e-work", "9f2c-work")

	require.Error(t, err)
	require.Equal(t, SessionIdentity{}, identity)
	require.True(t, errors.Is(err, ErrIdentityMismatchInstance))
	require.False(t, errors.Is(err, ErrIdentityMismatchTmuxMarker))
}

func TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnTmuxMarkerMismatchReason_WhenMarkerReadFails(t *testing.T) {
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "no such pane", 1)
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := fakeInstanceIdentitySnapshot{uuid: "9f2c-work"}
	identity, err := verifyIdentityImmediatelyBeforeWrite(context.Background(), inst, "", "stapler-gone", "9f2c-work")

	require.Error(t, err)
	require.Equal(t, SessionIdentity{}, identity)
	require.True(t, errors.Is(err, ErrIdentityMismatchTmuxMarker), "a read failure is ambiguity on the tmux-marker side, same as an explicit mismatch")

	var mismatchErr *IdentityMismatchError
	require.True(t, errors.As(err, &mismatchErr))
	require.Error(t, mismatchErr.Cause, "the underlying read failure should be preserved for diagnostics")
}

func TestVerifyIdentityImmediatelyBeforeWrite_ShouldBeIdempotent_WhenCalledTwiceInSuccession(t *testing.T) {
	// Story 3.4.1's NudgeGate pipeline calls this facade once as its own
	// identity check, and the write call site calls it again immediately
	// before the write -- both calls must be safe, read-only, and agree.
	fakeTmux, _ := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=9f2c-work", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := fakeInstanceIdentitySnapshot{uuid: "9f2c-work"}
	identity1, err1 := verifyIdentityImmediatelyBeforeWrite(context.Background(), inst, "", "stapler-e6c2a88e-work", "9f2c-work")
	identity2, err2 := verifyIdentityImmediatelyBeforeWrite(context.Background(), inst, "", "stapler-e6c2a88e-work", "9f2c-work")

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, identity1, identity2)
}
