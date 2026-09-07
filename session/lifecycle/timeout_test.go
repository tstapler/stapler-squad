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

func TestIsBenignTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"net.Error timeout", fmt.Errorf("read: %w", fakeTimeoutNetError{}), true},
		{"os.ErrDeadlineExceeded", fmt.Errorf("read: %w", os.ErrDeadlineExceeded), true},
		{"io.ErrUnexpectedEOF", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), true},
		{"genuine disconnect", errors.New("connection reset by peer"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lifecycle.IsBenignTimeout(tt.err); got != tt.want {
				t.Errorf("IsBenignTimeout(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
