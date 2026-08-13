package mcp

import (
	"regexp"
	"testing"
)

func TestErrorCodeFormat(t *testing.T) {
	re := regexp.MustCompile(`^[A-Z_]+$`)
	codes := []string{
		ErrSessionNotFound,
		ErrInvalidArgument,
		ErrInternalError,
		ErrConfirmationRequired,
		ErrInvalidStatusTrans,
		ErrSessionNotRunning,
		ErrRateLimitExceeded,
		ErrSessionStartupTimeout,
		ErrInvalidPath,
		ErrPTYWriteTimeout,
	}
	for _, code := range codes {
		if !re.MatchString(code) {
			t.Errorf("error code %q does not match ^[A-Z_]+$", code)
		}
	}
}
