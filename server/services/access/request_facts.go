package access

import (
	"context"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/server/middleware"
)

// SteerRequestFacts are the per-request facts an audit line records.
type SteerRequestFacts struct {
	Refusal   LocalWriteRefusal
	UserAgent string
}

// RequestFactsOf records the request's raw facts without evaluating a verdict.
func RequestFactsOf(ctx context.Context, req connect.AnyRequest) SteerRequestFacts {
	peer := req.Peer().Addr
	f := LocalWriteRefusal{Peer: peer, PeerLoopback: middleware.PeerIsLoopback(peer)}
	if rec, ok := RequestRecordFrom(ctx); ok {
		f.Listener, f.AuthMode, f.Host, f.Origin, f.Proxied = rec.Listener, rec.AuthMode, rec.Host, rec.Origin, rec.Proxied
	}
	return SteerRequestFacts{Refusal: f, UserAgent: req.Header().Get("User-Agent")}
}
