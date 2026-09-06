package lifecycle_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session/lifecycle"
)

// fakeTimeoutNetError is a minimal net.Error whose Timeout() always returns
// true, for exercising IsBenignTimeout's errors.As branch without a real
// network round trip.
type fakeTimeoutNetError struct{}

func (fakeTimeoutNetError) Error() string   { return "fake: i/o timeout" }
func (fakeTimeoutNetError) Timeout() bool   { return true }
func (fakeTimeoutNetError) Temporary() bool { return true }

func TestIsBenignTimeout_NetErrorTimeout_ReturnsTrue(t *testing.T) {
	err := fmt.Errorf("read: %w", fakeTimeoutNetError{})
	if !lifecycle.IsBenignTimeout(err) {
		t.Errorf("IsBenignTimeout(%v) = false, want true", err)
	}
}

func TestIsBenignTimeout_DeadlineExceeded_ReturnsTrue(t *testing.T) {
	err := fmt.Errorf("read: %w", os.ErrDeadlineExceeded)
	if !lifecycle.IsBenignTimeout(err) {
		t.Errorf("IsBenignTimeout(%v) = false, want true", err)
	}
}

func TestIsBenignTimeout_UnexpectedEOF_ReturnsTrue(t *testing.T) {
	err := fmt.Errorf("read: %w", io.ErrUnexpectedEOF)
	if !lifecycle.IsBenignTimeout(err) {
		t.Errorf("IsBenignTimeout(%v) = false, want true", err)
	}
}

func TestIsBenignTimeout_GenuineDisconnectError_ReturnsFalse(t *testing.T) {
	err := errors.New("connection reset by peer")
	if lifecycle.IsBenignTimeout(err) {
		t.Errorf("IsBenignTimeout(%v) = true, want false", err)
	}
}
