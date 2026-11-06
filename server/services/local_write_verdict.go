package services

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/middleware"
)

// LocalWriteProfile selects which verdict an in-handler check applies.
type LocalWriteProfile int

const (
	// LocalWriteRebinding is the rebinding gate used by the steer and Prune.
	LocalWriteRebinding LocalWriteProfile = iota
	// LocalWriteLocalCaller is the rebinding gate plus the local-caller gate (Reply).
	LocalWriteLocalCaller
)

// LocalWriteRefusal is a refusal, with the facts audit lines record. Reason is
// "" when the request was allowed.
type LocalWriteRefusal struct {
	Reason       string
	Listener     string
	AuthMode     string
	Host         string
	Origin       string
	Peer         string
	PeerLoopback bool
	Proxied      bool
}

// Allowed reports whether the verdict allowed the request.
func (r LocalWriteRefusal) Allowed() bool { return r.Reason == "" }

type verdictConfigKey struct{}

// WithLocalWriteVerdictConfig makes the rebinding gate's live inputs available
// to handlers behind the chain, next to the request record.
func WithLocalWriteVerdictConfig(cfg middleware.VerdictConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), verdictConfigKey{}, cfg)))
		})
	}
}

// EvaluateLocalWrite runs the in-handler verdict for the request in ctx. It
// always returns the facts an audit line records; err is a PermissionDenied
// Connect error when the request is refused. It fails closed when no chain
// stamped a request record or no verdict config was wired.
func EvaluateLocalWrite(ctx context.Context, peerAddr string, profile LocalWriteProfile) (LocalWriteRefusal, error) {
	rec, ok := RequestRecordFrom(ctx)
	out := LocalWriteRefusal{
		Listener: rec.Listener, AuthMode: rec.AuthMode, Host: rec.Host, Origin: rec.Origin,
		Peer: peerAddr, PeerLoopback: middleware.PeerIsLoopback(peerAddr), Proxied: rec.Proxied,
	}
	cfg, haveCfg := ctx.Value(verdictConfigKey{}).(middleware.VerdictConfig)
	switch {
	case !ok:
		out.Reason = "no_request_record"
	case !haveCfg:
		out.Reason = "verdict_not_configured"
	default:
		facts := middleware.CallerFacts{
			Host: rec.Host, Origin: rec.Origin, PeerAddr: peerAddr,
			Proxied: rec.Proxied, AuthRequired: rec.AuthMode == AuthModeRequired,
		}
		out.Reason = middleware.RebindingVerdict(facts, cfg)
		if out.Reason == "" && profile == LocalWriteLocalCaller {
			out.Reason = middleware.LocalCallerVerdict(facts)
		}
	}
	if out.Allowed() {
		return out, nil
	}
	log.Warn("local-write request refused", "reason", out.Reason, "listener", out.Listener,
		"host", out.Host, "origin", out.Origin, "peer", out.Peer, "proxied", out.Proxied)
	msg := "this request is only accepted from a verified local address: " + out.Reason
	if out.Reason == "host_not_allowed" {
		msg = "the Host " + strconv.Quote(out.Host) + " is not a loopback name, a verified hostname of this machine or one of its addresses"
	}
	return out, connect.NewError(connect.CodePermissionDenied, errors.New(msg))
}
