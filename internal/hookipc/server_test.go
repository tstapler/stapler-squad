package hookipc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/testutil/socket"
)

func TestServer_should_ReturnExactPreToolUseReply_When_ClientUsesProtocolV1(t *testing.T) {
	endpoint := testEndpoint(t)
	server, err := NewServer(endpoint, func(_ context.Context, request ClassificationEnvelope) (ClassificationReply, error) {
		return ClassificationReply{
			ProtocolVersion:     CurrentProtocolVersion,
			InstanceFingerprint: endpoint.InstanceFingerprint,
			RequestID:           request.RequestID,
			Source:              DecisionSourcePrimary,
			Output: &HookOutput{HookSpecificOutput: HookSpecificOutput{
				HookEventName:            "PreToolUse",
				PermissionDecision:       "allow",
				PermissionDecisionReason: "seed-safe-git: read-only command",
			}},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Close(ctx)
	})

	client, err := NewClient(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := client.Classify(context.Background(), validEnvelope(endpoint, "request-1"))
	if errors.Is(err, syscall.EPERM) {
		t.Skipf("sandbox forbids Unix-socket connect: %v", err)
	}
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if reply.Output == nil || reply.Output.HookSpecificOutput.PermissionDecision != "allow" {
		t.Fatalf("reply = %+v, want allow output", reply)
	}
}

func TestServer_should_RejectRequestBeforeClassification_When_InstanceFingerprintMismatches(t *testing.T) {
	endpoint := testEndpoint(t)
	var calls atomic.Int64
	server, err := NewServer(endpoint, func(_ context.Context, request ClassificationEnvelope) (ClassificationReply, error) {
		calls.Add(1)
		return ClassificationReply{}, errors.New("must not run")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Close(ctx)
	})

	clientEndpoint := endpoint
	clientEndpoint.InstanceFingerprint = Fingerprint("another-instance")
	client, err := NewClient(clientEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	envelope := validEnvelope(clientEndpoint, "request-1")
	if _, err := client.Classify(context.Background(), envelope); err == nil {
		t.Fatal("Classify succeeded for wrong instance")
	}
	if calls.Load() != 0 {
		t.Fatalf("handler calls = %d, want 0", calls.Load())
	}
}

func TestServer_should_NotUnlinkLiveSocket_When_SecondServerStarts(t *testing.T) {
	endpoint := testEndpoint(t)
	handler := func(_ context.Context, request ClassificationEnvelope) (ClassificationReply, error) {
		return ClassificationReply{
			ProtocolVersion:     CurrentProtocolVersion,
			InstanceFingerprint: endpoint.InstanceFingerprint,
			RequestID:           request.RequestID,
			Source:              DecisionSourcePrimary,
		}, nil
	}
	first, err := NewServer(endpoint, handler)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = first.Close(ctx)
	})

	second, err := NewServer(endpoint, handler)
	if err != nil {
		t.Fatal(err)
	}
	secondStartErr := second.Start()
	if !errors.Is(secondStartErr, ErrEndpointInUse) && !errors.Is(secondStartErr, syscall.EPERM) {
		t.Fatalf("second Start error = %v, want endpoint-in-use or sandbox permission error", secondStartErr)
	}
	if _, err := os.Lstat(endpoint.SocketPath); err != nil {
		t.Fatalf("second server removed first server socket: %v", err)
	}
	if errors.Is(secondStartErr, syscall.EPERM) {
		t.Skipf("sandbox forbids Unix-socket connect: %v", secondStartErr)
	}
	client, err := NewClient(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Classify(context.Background(), validEnvelope(endpoint, "still-live")); err != nil {
		t.Fatalf("first server no longer reachable: %v", err)
	}
}

func testEndpoint(t *testing.T) HookEndpoint {
	t.Helper()
	dir := socket.ShortTempSocketDir(t)
	return HookEndpoint{
		SocketPath:          filepath.Join(dir, "hook.sock"),
		InstanceFingerprint: Fingerprint(dir),
		ProtocolVersion:     CurrentProtocolVersion,
	}
}

func validEnvelope(endpoint HookEndpoint, requestID HookRequestID) ClassificationEnvelope {
	return ClassificationEnvelope{
		ProtocolVersion:     endpoint.ProtocolVersion,
		InstanceFingerprint: endpoint.InstanceFingerprint,
		RequestID:           requestID,
		Payload: classifier.PermissionRequestPayload{
			HookEventName: "PreToolUse",
			ToolName:      "Bash",
			ToolInput:     map[string]interface{}{"command": "git status"},
		},
	}
}

func TestServer_should_RemoveStaleSocket_When_NoListenerOwnsIt(t *testing.T) {
	endpoint := testEndpoint(t)
	if err := os.WriteFile(endpoint.SocketPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(endpoint, func(_ context.Context, request ClassificationEnvelope) (ClassificationReply, error) {
		return ClassificationReply{
			ProtocolVersion:     CurrentProtocolVersion,
			InstanceFingerprint: endpoint.InstanceFingerprint,
			RequestID:           request.RequestID,
			Source:              DecisionSourceDefer,
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("Start with stale socket: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
