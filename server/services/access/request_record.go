package access

import (
	"context"
	"net/http"

	"github.com/tstapler/stapler-squad/server/middleware"
)

// Listener names and auth modes stamped on a RequestRecord.
const (
	ListenerLocal    = "local"
	ListenerRemote   = "remote"
	AuthModeNone     = "none"
	AuthModeRequired = "required"
)

// RequestRecord holds the raw facts about an inbound request that later
// policy (the delivery-gate audit, LocalWriteGuard) reads. It carries no verdict.
type RequestRecord struct {
	Listener string
	AuthMode string
	Host     string
	Origin   string
	Proxied  bool
}

type requestRecordKey struct{}

// WithRequestRecord stamps a RequestRecord on the request context. AuthMode is
// "required" only when requiresAuth is true.
func WithRequestRecord(listener string, requiresAuth bool) func(http.Handler) http.Handler {
	mode := AuthModeNone
	if requiresAuth {
		mode = AuthModeRequired
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := RequestRecord{
				Listener: listener,
				AuthMode: mode,
				Host:     r.Host,
				Origin:   r.Header.Get("Origin"),
				Proxied:  middleware.HasProxyHeader(r.Header),
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestRecordKey{}, rec)))
		})
	}
}

// RequestRecordFrom returns the stamped record; ok is false when no chain stamped it.
func RequestRecordFrom(ctx context.Context) (RequestRecord, bool) {
	rec, ok := ctx.Value(requestRecordKey{}).(RequestRecord)
	return rec, ok
}
