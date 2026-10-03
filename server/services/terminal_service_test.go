package services

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/sendkeysguard"
)

// TestServicesPackage_NoDirectSendKeysPlusEnterConcatenation is the
// server/services instance of the BUG-031 structural guard (see
// sendkeysguard's doc comment and session/pane_submit_test.go's sibling
// test) — TerminalService.WriteToSession and SessionService.steerInstance
// both used to build the single-write pattern this guards against.
func TestServicesPackage_NoDirectSendKeysPlusEnterConcatenation(t *testing.T) {
	t.Parallel()
	sendkeysguard.CheckNoSingleWriteEnterConcatenation(t, ".")
}

// TestSubmitErrToConnectError_MapsErrSubmitNotConfirmedToAborted is the
// server/services counterpart to server/mcp's TestSubmitErrResult_* — a
// swallowed submit (session.ErrSubmitNotConfirmed) must surface as a
// distinguishable connect error code, not CodeInternal.
func TestSubmitErrToConnectError_MapsErrSubmitNotConfirmedToAborted(t *testing.T) {
	err := submitErrToConnectError(session.ErrSubmitNotConfirmed)
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Errorf("expected CodeAborted, got %v", connect.CodeOf(err))
	}
}

// TestSubmitErrToConnectError_MapsDeadlineExceededToCodeDeadlineExceeded
// proves the timeout path is distinguishable from both a swallowed submit
// and a generic failure.
func TestSubmitErrToConnectError_MapsDeadlineExceededToCodeDeadlineExceeded(t *testing.T) {
	err := submitErrToConnectError(context.DeadlineExceeded)
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Errorf("expected CodeDeadlineExceeded, got %v", connect.CodeOf(err))
	}
}

// TestSubmitErrToConnectError_MapsOtherErrorsToCodeInternal verifies the
// fallback case doesn't silently masquerade as one of the two distinct codes.
func TestSubmitErrToConnectError_MapsOtherErrorsToCodeInternal(t *testing.T) {
	err := submitErrToConnectError(fmt.Errorf("pty closed"))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}
