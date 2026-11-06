package hookipc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClient_should_RespectTotalDeadline_When_HandlerBlocks(t *testing.T) {
	endpoint := testEndpoint(t)
	server, err := NewServer(endpoint, func(ctx context.Context, request ClassificationEnvelope) (ClassificationReply, error) {
		<-ctx.Done()
		return ClassificationReply{}, ctx.Err()
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

	client, err := NewClient(endpoint, WithClientTimeout(20*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = client.Classify(context.Background(), validEnvelope(endpoint, "request-timeout"))
	if err == nil {
		t.Fatal("Classify succeeded while handler was blocked")
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("deadline took %s, want bounded failure", elapsed)
	}
}

func TestClassificationReplyValidate_should_RejectReply_When_RequestIDMismatches(t *testing.T) {
	endpoint := testEndpoint(t)
	reply := ClassificationReply{
		ProtocolVersion:     endpoint.ProtocolVersion,
		InstanceFingerprint: endpoint.InstanceFingerprint,
		RequestID:           "other-request",
		Source:              DecisionSourcePrimary,
	}
	if err := reply.Validate(endpoint, "expected-request"); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Validate error = %v, want ErrInvalidEnvelope", err)
	}
}
