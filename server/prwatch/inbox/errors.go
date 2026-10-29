package inbox

import (
	"errors"
	"fmt"
)

// Callers branch on these with errors.Is. None of them ever carries the
// bearer token.
var (
	// ErrUnauthorized is a 401: the token was rejected.
	ErrUnauthorized = errors.New("inbox rejected the bearer token")
	// ErrTransport covers refused connections, timeouts and unexpected redirects.
	ErrTransport = errors.New("inbox unreachable")
	// Err5xx is a server-side failure, including non-JSON proxy error pages.
	Err5xx = errors.New("inbox server error")
	// ErrDecode means a 2xx response was malformed, truncated or oversized.
	ErrDecode = errors.New("inbox response could not be decoded")
	// ErrRejected is any other non-2xx answer (400, 403, 404 outside Watch).
	ErrRejected = errors.New("inbox rejected the request")
	// ErrNotFound is a 404 outside POST /v1/watch (for example an ack of an
	// item that is already gone). It also matches ErrRejected.
	ErrNotFound = fmt.Errorf("%w: not found", ErrRejected)
	// ErrWatchUnsupported is a 404 on POST /v1/watch: an older inbox, so the
	// caller falls back to Subscribe and records coverage as unknown.
	ErrWatchUnsupported = errors.New("inbox does not support /v1/watch")
)
