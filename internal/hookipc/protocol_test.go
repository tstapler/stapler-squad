package hookipc

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

func TestDecodeEnvelope_should_ReturnClassificationEnvelope_When_VersionAndIdentityMatch(t *testing.T) {
	endpoint := HookEndpoint{
		SocketPath:          "/tmp/hook.sock",
		InstanceFingerprint: Fingerprint("/state/manual-a"),
		ProtocolVersion:     CurrentProtocolVersion,
	}
	requestID, stable, err := NewRequestID("session-1", ToolUseID("toolu_123"))
	if err != nil {
		t.Fatal(err)
	}
	if !stable {
		t.Fatal("tool_use_id should produce a stable request id")
	}
	envelope := ClassificationEnvelope{
		ProtocolVersion:     CurrentProtocolVersion,
		InstanceFingerprint: endpoint.InstanceFingerprint,
		RequestID:           requestID,
		Payload: classifier.PermissionRequestPayload{
			SessionID:     "session-1",
			ToolUseID:     "toolu_123",
			HookEventName: "PreToolUse",
			ToolName:      "Bash",
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ClassificationEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(endpoint); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if decoded.Payload.ToolUseID != "toolu_123" {
		t.Fatalf("ToolUseID = %q, want toolu_123", decoded.Payload.ToolUseID)
	}
	repeated, repeatedStable, err := NewRequestID("session-1", ToolUseID(decoded.Payload.ToolUseID))
	if err != nil {
		t.Fatal(err)
	}
	if !repeatedStable || repeated != requestID {
		t.Fatalf("stable request ID changed: %q != %q", repeated, requestID)
	}
}

func TestDecodeEnvelope_should_RejectRequest_When_VersionOrFingerprintMismatches(t *testing.T) {
	endpoint := HookEndpoint{
		InstanceFingerprint: Fingerprint("/state/manual-a"),
		ProtocolVersion:     CurrentProtocolVersion,
	}
	base := ClassificationEnvelope{
		ProtocolVersion:     CurrentProtocolVersion,
		InstanceFingerprint: endpoint.InstanceFingerprint,
		RequestID:           "request-1",
		Payload: classifier.PermissionRequestPayload{
			HookEventName: "PreToolUse",
			ToolName:      "Bash",
		},
	}

	wrongVersion := base
	wrongVersion.ProtocolVersion++
	if err := wrongVersion.Validate(endpoint); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatalf("wrong version error = %v, want ErrProtocolMismatch", err)
	}
	wrongInstance := base
	wrongInstance.InstanceFingerprint = Fingerprint("/state/production")
	if err := wrongInstance.Validate(endpoint); !errors.Is(err, ErrInstanceMismatch) {
		t.Fatalf("wrong instance error = %v, want ErrInstanceMismatch", err)
	}
}

func TestNewRequestID_should_NotDeduplicate_When_ToolUseIDIsMissing(t *testing.T) {
	first, firstStable, err := NewRequestID("session-1", "")
	if err != nil {
		t.Fatal(err)
	}
	second, secondStable, err := NewRequestID("session-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if firstStable || secondStable {
		t.Fatal("missing tool_use_id unexpectedly produced a stable id")
	}
	if first == second {
		t.Fatalf("random request IDs collided: %q", first)
	}
}
