package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/headless"
)

// firstCallJSONHS returns a valid first-call JSON response for headless service tests.
func firstCallJSONHS(sessionID, result string) string {
	return `{"session_id":"` + sessionID + `","result":"` + result + `","cost_usd":0.001}`
}

// newHeadlessTestServer creates an in-process HTTP test server for HeadlessService.
func newHeadlessTestServer(t *testing.T, pool *headless.Pool) (*httptest.Server, sessionv1connect.HeadlessServiceClient) {
	t.Helper()
	svc := NewHeadlessService(pool)
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewHeadlessServiceHandler(svc)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := sessionv1connect.NewHeadlessServiceClient(
		srv.Client(),
		srv.URL,
	)
	return srv, client
}

// TestHeadlessService_RunHeadlessCall_StreamsChunks verifies streaming of chunks.
func TestHeadlessService_RunHeadlessCall_StreamsChunks(t *testing.T) {
	runner := headless.NewFakeRunner(firstCallJSONHS("s1", "hello from LLM"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	_, client := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
		FeatureKey: "custom",
		UserPrompt: "say hello",
	}))
	require.NoError(t, err)
	defer stream.Close()

	var chunks []*sessionv1.RunHeadlessCallResponse
	for stream.Receive() {
		chunks = append(chunks, stream.Msg())
	}
	require.NoError(t, stream.Err())

	// At least one text chunk + done chunk.
	require.NotEmpty(t, chunks)
	found := false
	for _, c := range chunks {
		if c.Text != "" {
			found = true
		}
	}
	assert.True(t, found, "at least one chunk should have non-empty text")
}

// TestHeadlessService_RunHeadlessCall_InvalidFeatureKey_ReturnsInvalidArgument verifies key validation.
func TestHeadlessService_RunHeadlessCall_InvalidFeatureKey_ReturnsInvalidArgument(t *testing.T) {
	runner := headless.NewFakeRunner()
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	_, client := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, badKey := range []string{"", "unknown-key", "REVIEW", " "} {
		stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
			FeatureKey: badKey,
			UserPrompt: "prompt",
		}))
		if err == nil {
			// Some servers return error on first Receive.
			for stream.Receive() {
			}
			err = stream.Err()
			stream.Close()
		}
		require.Error(t, err, "expected error for bad key %q", badKey)
		var connectErr *connect.Error
		if assert.ErrorAs(t, err, &connectErr) {
			assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code(), "bad key %q", badKey)
		}
	}
}

// TestHeadlessService_RunHeadlessCall_AllowedFeatureKeys verifies all allowed keys.
func TestHeadlessService_RunHeadlessCall_AllowedFeatureKeys(t *testing.T) {
	allowedKeys := []string{"review", "summarize", "acceptance-criteria", "pr-description", "commit-message", "custom"}

	runner := headless.NewFakeRunner(
		firstCallJSONHS("s1", "ok"),
		firstCallJSONHS("s2", "ok"),
		firstCallJSONHS("s3", "ok"),
		firstCallJSONHS("s4", "ok"),
		firstCallJSONHS("s5", "ok"),
		firstCallJSONHS("s6", "ok"),
	)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)
	_, clientWithPool := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, key := range allowedKeys {
		stream, err := clientWithPool.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
			FeatureKey: key,
			UserPrompt: "prompt",
		}))
		if err != nil {
			t.Logf("allowed key %q returned error at stream creation: %v (may be expected if pool exhausted)", key, err)
			continue
		}
		for stream.Receive() {
		}
		if streamErr := stream.Err(); streamErr != nil {
			// Check it's not CodeInvalidArgument.
			var connectErr *connect.Error
			if assert.ErrorAs(t, streamErr, &connectErr) {
				assert.NotEqual(t, connect.CodeInvalidArgument, connectErr.Code(),
					"key %q should be allowed but got InvalidArgument", key)
			}
		}
		stream.Close()
	}
}

// TestHeadlessService_RunHeadlessCall_PoolNil_ReturnsUnavailable verifies nil pool handling.
func TestHeadlessService_RunHeadlessCall_PoolNil_ReturnsUnavailable(t *testing.T) {
	_, client := newHeadlessTestServer(t, nil) // nil pool

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
		FeatureKey: "custom",
		UserPrompt: "prompt",
	}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
		stream.Close()
	}
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
	}
}

// TestHeadlessService_RunHeadlessCall_ZeroTimeoutSeconds_UsesDefault verifies that
// TimeoutSeconds=0 is accepted (uses default) and completes without error.
func TestHeadlessService_RunHeadlessCall_ZeroTimeoutSeconds_UsesDefault(t *testing.T) {
	runner := headless.NewFakeRunner(firstCallJSONHS("s1", "ok"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)
	_, client := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
		FeatureKey:     "custom",
		UserPrompt:     "prompt",
		TimeoutSeconds: 0, // service applies DefaultCallTimeout
	}))
	require.NoError(t, err)
	defer stream.Close()

	for stream.Receive() {
	}
	assert.NoError(t, stream.Err())
}

// TestHeadlessService_RunHeadlessCall_ContextCancel_StopsSubprocess verifies cancellation.
func TestHeadlessService_RunHeadlessCall_ContextCancel_StopsSubprocess(t *testing.T) {
	runner := headless.NewFakeRunner(firstCallJSONHS("s1", "ok"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)
	_, client := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
		FeatureKey: "custom",
		UserPrompt: "prompt",
	}))
	if err != nil {
		// Error at stream creation (pre-cancelled context) is OK.
		return
	}
	defer stream.Close()
	// Drain — should complete without hanging.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for stream.Receive() {
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not complete after context cancellation within 5s")
	}
}

// TestHeadlessService_RunHeadlessCall_SystemPromptOverLimit_ReturnsInvalidArgument
// verifies that system_prompt exceeding maxPromptBytes is rejected.
func TestHeadlessService_RunHeadlessCall_SystemPromptOverLimit_ReturnsInvalidArgument(t *testing.T) {
	runner := headless.NewFakeRunner()
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)
	_, client := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
		FeatureKey:   "custom",
		UserPrompt:   "prompt",
		SystemPrompt: strings.Repeat("x", 100_001),
	}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
		stream.Close()
	}
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	}
}

// TestHeadlessService_RunHeadlessCall_UserPromptOverLimit_ReturnsInvalidArgument
// verifies that user_prompt exceeding maxPromptBytes is rejected.
func TestHeadlessService_RunHeadlessCall_UserPromptOverLimit_ReturnsInvalidArgument(t *testing.T) {
	runner := headless.NewFakeRunner()
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)
	_, client := newHeadlessTestServer(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
		FeatureKey: "custom",
		UserPrompt: strings.Repeat("y", 100_001),
	}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
		stream.Close()
	}
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	}
}
