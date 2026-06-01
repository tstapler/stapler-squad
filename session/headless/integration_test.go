//go:build integration

package headless

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPool_RealClaude_SimplePrompt calls the real claude binary with a trivial prompt.
// Requires CLAUDE_INTEGRATION_TESTS=true and claude in PATH.
func TestPool_RealClaude_SimplePrompt(t *testing.T) {
	pool, err := NewPool(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2})
	require.NoError(t, err, "NewPool should succeed when claude is in PATH")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := pool.CallBlocking(ctx, FeatureKeyCustom, "", "Say hello in exactly 3 words.")
	require.NoError(t, err)
	assert.NotEmpty(t, result, "result should be non-empty")
	t.Logf("claude response: %q", result)
}

// TestPool_RealClaude_SessionResumption verifies that the second call uses --resume.
func TestPool_RealClaude_SessionResumption(t *testing.T) {
	// Wrap a real pool to inspect args.
	realPool, err := NewPool(PoolConfig{MaxCallsPerSession: 10, MaxConcurrentSessions: 1})
	require.NoError(t, err)

	// Replace runner with a tracking wrapper.
	var capturedArgs [][]string
	originalRunner := realPool.runner.(*ProcessRunner)
	trackRunner := &argsCapturingRunner{inner: originalRunner, captured: &capturedArgs}
	realPool.runner = trackRunner

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err = realPool.CallBlocking(ctx, "integration-test", "", "Say 'first call'")
	require.NoError(t, err, "first call should succeed")

	_, err = realPool.CallBlocking(ctx, "integration-test", "", "Say 'second call'")
	require.NoError(t, err, "second call should succeed")

	require.Len(t, capturedArgs, 2, "should have captured 2 calls")

	// First call: should have --output-format json.
	found := false
	for i, a := range capturedArgs[0] {
		if a == "--output-format" && i+1 < len(capturedArgs[0]) && capturedArgs[0][i+1] == "json" {
			found = true
			break
		}
	}
	assert.True(t, found, "first call should use --output-format json; got: %v", capturedArgs[0])

	// Second call: should have --resume.
	foundResume := false
	for _, a := range capturedArgs[1] {
		if a == "--resume" {
			foundResume = true
			break
		}
	}
	assert.True(t, foundResume, "second call should use --resume; got: %v", capturedArgs[1])
}

// argsCapturingRunner wraps a real runner and records args.
type argsCapturingRunner struct {
	inner    *ProcessRunner
	captured *[][]string
}

func (r *argsCapturingRunner) Run(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, func() error, error) {
	argsCopy := make([]string, len(args))
	for i, a := range args {
		argsCopy[i] = a
	}
	*r.captured = append(*r.captured, argsCopy)
	return r.inner.Run(ctx, args, stdin)
}
