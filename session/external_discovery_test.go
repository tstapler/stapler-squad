package session

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryWithDelay_SuccessFirstAttempt verifies that the function returns nil
// immediately when fn succeeds on the first call.
func TestRetryWithDelay_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := retryWithDelay(3, 1*time.Millisecond, func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "fn should be called exactly once on first-attempt success")
}

// TestRetryWithDelay_SuccessAfterRetry verifies that retryWithDelay retries and
// returns nil when fn eventually succeeds within maxAttempts.
func TestRetryWithDelay_SuccessAfterRetry(t *testing.T) {
	calls := 0
	transientErr := errors.New("transient error")
	err := retryWithDelay(3, 1*time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return transientErr
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls, "fn should be called 3 times before success")
}

// TestRetryWithDelay_AllAttemptsExhausted verifies that retryWithDelay returns
// the last error when all maxAttempts are exhausted.
func TestRetryWithDelay_AllAttemptsExhausted(t *testing.T) {
	permanentErr := errors.New("permanent error")
	calls := 0
	err := retryWithDelay(3, 1*time.Millisecond, func() error {
		calls++
		return permanentErr
	})
	require.Error(t, err)
	assert.Equal(t, permanentErr, err)
	assert.Equal(t, 3, calls, "fn should be attempted exactly maxAttempts times")
}

// TestRetryWithDelay_ConnectionRefusedSkipsRetry verifies that a connection-
// refused error causes retryWithDelay to return immediately without retrying,
// since a refused connection indicates a stale socket that won't recover.
func TestRetryWithDelay_ConnectionRefusedSkipsRetry(t *testing.T) {
	connRefusedErr := errors.New("dial unix /tmp/dead.sock: connect: connection refused")
	calls := 0
	err := retryWithDelay(3, 1*time.Millisecond, func() error {
		calls++
		return connRefusedErr
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "connection refused should skip retries — stale socket")
}

// TestRetryWithDelay_NoSuchFileSkipsRetry verifies that "no such file or directory"
// errors (missing socket) are also treated as permanent and skip retries.
func TestRetryWithDelay_NoSuchFileSkipsRetry(t *testing.T) {
	noFileErr := errors.New("stat /tmp/missing.sock: no such file or directory")
	calls := 0
	err := retryWithDelay(3, 1*time.Millisecond, func() error {
		calls++
		return noFileErr
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "no such file error should skip retries — missing socket")
}

// TestIsConnectionRefused verifies the helper correctly classifies errors.
func TestIsConnectionRefused(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"uppercase mixed", errors.New("dial: Connection Refused"), true},
		{"no such file", errors.New("no such file or directory"), true},
		{"no such socket", errors.New("no such socket"), true},
		{"unrelated error", errors.New("timeout"), false},
		{"transient io error", errors.New("read: broken pipe"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isConnectionRefused(tc.err))
		})
	}
}
