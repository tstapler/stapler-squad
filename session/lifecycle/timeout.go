package lifecycle

import (
	"errors"
	"io"
	"net"
	"os"
)

// IsBenignTimeout reports whether err is an expected poll-timeout rather
// than a real disconnect: a net.Error with Timeout() true, os.ErrDeadlineExceeded,
// or io.ErrUnexpectedEOF (a partial read before the deadline fired). Ports
// exactly the three typed checks session/external_streamer.go used inline,
// deliberately without a strings.Contains fallback — a classifier that
// falls through to matching error text silently breaks if a wrapped
// library changes its message.
func IsBenignTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
}
