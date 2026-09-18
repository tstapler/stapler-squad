//go:build windows

package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsTimeoutKill_should_AlwaysReturnTrue_When_OnWindows documents that
// isTimeoutKill has no signal to inspect on Windows (TerminateProcess, not
// SIGKILL) and always defers to the ctx.Err() check callers already perform —
// see timeout_executor_windows.go's doc comment.
func TestIsTimeoutKill_should_AlwaysReturnTrue_When_OnWindows(t *testing.T) {
	assert.True(t, isTimeoutKill(nil))
	assert.True(t, isTimeoutKill(context.DeadlineExceeded))
	assert.True(t, isTimeoutKill(errors.New("some other exit error")))
}
