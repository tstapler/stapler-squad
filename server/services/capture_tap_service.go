package services

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

// maxCaptureTapSessionIDs bounds one request's session list.
const maxCaptureTapSessionIDs = 256

// proxyHeaders mark a request that passed through a reverse proxy or tunnel. A
// local proxy connects from loopback, so the socket alone cannot tell a proxied
// remote caller from a local one.
var proxyHeaders = []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Via"}

// +api: capture-tap:set
// SetCaptureTap turns the terminal-stream capture tap on or off at runtime.
// Loopback requests only; the output directory is never caller-supplied.
func (s *SessionService) SetCaptureTap(
	_ context.Context,
	req *connect.Request[sessionv1.SetCaptureTapRequest],
) (*connect.Response[sessionv1.SetCaptureTapResponse], error) {
	if err := requireLoopbackCaller(req.Peer().Addr, req.Header()); err != nil {
		return nil, err
	}
	msg := req.Msg
	if msg.GetTtlSeconds() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ttl_seconds must not be negative"))
	}
	if len(msg.GetSessionIds()) > maxCaptureTapSessionIDs {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("too many session_ids"))
	}
	for _, id := range msg.GetSessionIds() {
		if id == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_ids must not contain empty names"))
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

// +api: capture-tap:get
// GetCaptureTap reports the capture tap's state. Loopback requests only.
func (s *SessionService) GetCaptureTap(
	_ context.Context,
	req *connect.Request[sessionv1.GetCaptureTapRequest],
) (*connect.Response[sessionv1.GetCaptureTapResponse], error) {
	if err := requireLoopbackCaller(req.Peer().Addr, req.Header()); err != nil {
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

// requireLoopbackCaller fails closed: the socket peer must be a loopback
// address, no proxy header may be present, and a browser Origin must be
// loopback too.
func requireLoopbackCaller(peerAddr string, h interface{ Values(string) []string }) error {
	deny := func(reason string) error {
		log.Warn("[CaptureTap] rejected non-loopback request", "peer", peerAddr, "reason", reason)
		return connect.NewError(connect.CodePermissionDenied,
			errors.New("the capture tap can only be controlled from loopback (127.0.0.1 or ::1) without proxy headers: "+reason))
	}
	ap, err := netip.ParseAddrPort(peerAddr)
	if err != nil || !ap.Addr().Unmap().IsLoopback() {
		return deny("peer address is not loopback")
	}
	for _, name := range proxyHeaders {
		if len(h.Values(name)) > 0 {
			return deny("request carries " + name)
		}
	}
	for _, origin := range h.Values("Origin") {
		u, perr := url.Parse(origin)
		if perr != nil || !middleware.IsLoopbackHostname(u.Hostname()) {
			return deny("Origin is not loopback")
		}
	}
	return nil
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
			BytesWritten: ss.BytesWritten, Capped: ss.Capped, Failed: ss.Failed,
		})
	}
	return out
}
