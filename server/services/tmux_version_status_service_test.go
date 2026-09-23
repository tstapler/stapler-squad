package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestGetTmuxVersionStatus_ReturnsEmpty_When_NoMismatchDetected(t *testing.T) {
	svc := newCreateTestService(t, createTestStorage(t))

	resp, err := svc.GetTmuxVersionStatus(context.Background(), connect.NewRequest(&sessionv1.GetTmuxVersionStatusRequest{}))
	if err != nil {
		t.Fatalf("GetTmuxVersionStatus: %v", err)
	}
	if len(resp.Msg.Mismatches) != 0 {
		t.Errorf("Mismatches = %v, want empty when this test binary's process has never detected one", resp.Msg.Mismatches)
	}
}

func TestRestartTmuxServer_ReturnsErrServerDownAsSuccess_When_NoServerRunning(t *testing.T) {
	svc := newCreateTestService(t, createTestStorage(t))

	// A socket with no running server: tmux.RestartTmuxServer treats
	// "already gone" as success (nothing left to kill), not an error --
	// mirrors the idempotent-restart semantics the RPC promises callers.
	resp, err := svc.RestartTmuxServer(context.Background(), connect.NewRequest(&sessionv1.RestartTmuxServerRequest{
		ServerSocket: "definitely_not_a_running_tmux_server_" + t.Name(),
	}))
	if err != nil {
		t.Fatalf("RestartTmuxServer: %v", err)
	}
	if resp.Msg.SessionsAffected != 0 {
		t.Errorf("SessionsAffected = %d, want 0 for a socket with no running server", resp.Msg.SessionsAffected)
	}
}
