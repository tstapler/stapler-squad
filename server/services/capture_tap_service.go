package services

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

const (
	// maxCaptureTapSessionIDs bounds one request's session list.
	maxCaptureTapSessionIDs = 256
	// maxCaptureTapSessionIDLen bounds one session id, in bytes. Below the 255-byte
	// file-name limit so the hash suffix, ".jsonl" and ".old" still fit.
	maxCaptureTapSessionIDLen = 200
)

type requestHostKey struct{}

// WithRequestHost makes the request's Host header available to handlers behind
// ConnectRPC, which does not expose it. Wrap the SessionService handler with it.
func WithRequestHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestHostKey{}, r.Host)))
	})
}

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

// SetCaptureTap turns the terminal-stream capture tap on or off at runtime.
// Loopback requests only; the output directory is never caller-supplied.
// +api: SetCaptureTap
func (s *SessionService) SetCaptureTap(
	ctx context.Context,
	req *connect.Request[sessionv1.SetCaptureTapRequest],
) (*connect.Response[sessionv1.SetCaptureTapResponse], error) {
	if err := requireLoopbackCaller(req.Peer().Addr, requestHost(ctx), req.Header()); err != nil {
		return nil, err
	}
	msg := req.Msg
	if msg.GetEnabled() && msg.GetTtlSeconds() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ttl_seconds must not be negative"))
	}
	if len(msg.GetSessionIds()) > maxCaptureTapSessionIDs {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("too many session_ids"))
	}
	for _, id := range msg.GetSessionIds() {
		if err := validateCaptureTapSessionID(id); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	ttl := time.Duration(msg.GetTtlSeconds()) * time.Second
	st, err := s.captureTapRegistry().Set(msg.GetEnabled(), msg.GetSessionIds(), ttl)
	if err != nil {
		if errors.Is(err, streamhub.ErrTapAllEnabled) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	log.Warn("[SetCaptureTap] capture tap changed", "peer", req.Peer().Addr, "enabled", msg.GetEnabled(),
		"sessions", msg.GetSessionIds(), "scope", captureTapScope(st))
	return connect.NewResponse(&sessionv1.SetCaptureTapResponse{State: captureTapState(st)}), nil
}

// GetCaptureTap reports the capture tap's state. Loopback requests only.
// +api: GetCaptureTap
func (s *SessionService) GetCaptureTap(
	ctx context.Context,
	req *connect.Request[sessionv1.GetCaptureTapRequest],
) (*connect.Response[sessionv1.GetCaptureTapResponse], error) {
	if err := requireLoopbackCaller(req.Peer().Addr, requestHost(ctx), req.Header()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionv1.GetCaptureTapResponse{State: captureTapState(s.captureTapRegistry().Status())}), nil
}

func (s *SessionService) captureTapRegistry() *streamhub.TapRegistry {
	if s.tapRegistry != nil {
		return s.tapRegistry
	}
	return streamhub.DefaultTapRegistry()
}

func requestHost(ctx context.Context) string {
	host, _ := ctx.Value(requestHostKey{}).(string)
	return host
}

func validateCaptureTapSessionID(id string) error {
	switch {
	case id == "":
		return errors.New("session_ids must not contain empty names")
	case len(id) > maxCaptureTapSessionIDLen:
		return errors.New("session_ids entries must be at most 256 bytes")
	case !utf8.ValidString(id) || strings.ContainsFunc(id, unicode.IsControl):
		return errors.New("session_ids must not contain control characters or invalid UTF-8")
	}
	return nil
}

// requireLoopbackCaller fails closed. The socket peer must be loopback, the
// Host must be a loopback name (a local reverse proxy or tunnel forwards the
// public Host), no proxy header may be present, and a browser Origin must be
// same-origin with the Host. An absent Origin is a non-browser client.
func requireLoopbackCaller(peerAddr, host string, h http.Header) error {
	deny := func(reason string) error {
		log.Warn("[CaptureTap] rejected non-loopback request", "peer", peerAddr, "reason", reason)
		return connect.NewError(connect.CodePermissionDenied,
			errors.New("the capture tap can only be controlled from loopback (127.0.0.1 or ::1) without proxy headers: "+reason))
	}
	if !middleware.PeerIsLoopback(peerAddr) {
		return deny("peer address is not loopback")
	}
	if !hostIsLoopback(host) {
		return deny("Host is not a loopback name")
	}
	if name := middleware.ProxyHeaderName(h); name != "" {
		return deny("request carries " + name)
	}
	for _, origin := range h.Values("Origin") {
		if u, perr := url.Parse(origin); perr != nil || u.Host == "" || !strings.EqualFold(u.Host, host) {
			return deny("Origin is not same-origin with Host")
		}
	}
	return nil
}

func hostIsLoopback(host string) bool {
	// A trailing ':' (empty port) and userinfo are never sent by a real client;
	// url.Parse would silently accept both, so reject them explicitly.
	if host == "" || strings.HasSuffix(host, ":") {
		return false
	}
	u, err := url.Parse("//" + host)
	return err == nil && u.User == nil && u.Hostname() != "" && middleware.IsLoopbackHostname(u.Hostname())
}

func captureTapScope(st streamhub.TapStatus) string {
	switch {
	case st.AllEnabled:
		return "all"
	case len(st.SessionIDs) > 0:
		return "sessions"
	default:
		return "off"
	}
}

func captureTapState(st streamhub.TapStatus) *sessionv1.CaptureTapState {
	out := &sessionv1.CaptureTapState{
		Enabled:    st.Enabled,
		Scope:      captureTapScope(st),
		SessionIds: st.SessionIDs,
		Dir:        st.Dir,
	}
	if !st.ExpiresAt.IsZero() {
		out.TtlExpiresAt = timestamppb.New(st.ExpiresAt)
	}
	for _, ss := range st.Sessions {
		out.Sessions = append(out.Sessions, &sessionv1.CaptureTapSessionStatus{
			Name: ss.Name, Path: ss.Path, Enabled: ss.Enabled,
			BytesWritten: ss.BytesWritten, Capped: ss.Capped, Failed: ss.Failed, Rotated: ss.Rotated,
		})
	}
	return out
}
