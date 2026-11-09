package tymux

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/internal/qos"
	"github.com/tstapler/stapler-squad/log"
)

// capPaneActivityHint is the GetServerInfo capability for tymuxd's SetPaneActivity RPC.
const capPaneActivityHint = "pane.activity_hint"

// paneActivitySender performs the SetPaneActivity RPC. It exists as a seam because the generated
// client pinned in go.mod predates the RPC (tymux#86): the real sender lives in
// pane_activity_adapter.go behind the tymux_pane_activity build tag, to be made unconditional
// when the pin moves to a release that has the RPC on selecting.Transport. Until then
// newPaneActivitySender stays nil and SetPaneActivity is a logged-once no-op.
type paneActivitySender interface {
	sendPaneActivity(ctx context.Context, paneID string, activity qos.Activity) error
}

// newPaneActivitySender builds a sender over the session's transport, or returns nil when the
// transport cannot carry the RPC. Set by the adapter file's init.
var newPaneActivitySender func(rpcTransport) paneActivitySender

// activityHintState mirrors the lifecycle capability cache: capUnknown until a definite
// answer, then capSupported or capUnsupported for the session's lifetime (a failed
// GetServerInfo is not cached).
type activityHintState struct {
	capability atomic.Int32
	loggedOnce atomic.Bool
	sender     atomic.Pointer[paneActivitySender]
	// lastSent is the last hint the daemon accepted (a qos.Activity), so shutdown only has to
	// clear panes that are actually boosted or demoted.
	lastSent atomic.Int32
}

// SetPaneActivity tells tymuxd how latency-sensitive this session's pane is right now. It is an
// advisory optimization, so every shortfall (no client support, older daemon, no pane yet, RPC
// failure) returns nil; callers must never treat a missing hint as an error.
func (s *tymuxGRPCSession) SetPaneActivity(ctx context.Context, activity qos.Activity) error {
	if !s.activityHintAvailable(ctx) {
		return nil
	}
	s.mu.RLock()
	paneID := s.paneID
	s.mu.RUnlock()
	if paneID == "" {
		return nil
	}
	err := (*s.activity.sender.Load()).sendPaneActivity(ctx, paneID, activity)
	if connect.CodeOf(err) == connect.CodeUnimplemented {
		s.activity.capability.Store(capUnsupported)
		s.logActivityHintOnce("daemon rejected SetPaneActivity as unimplemented")
		return nil
	}
	if err == nil {
		s.activity.lastSent.Store(int32(activity))
	}
	return err
}

// resetActivityTimeout bounds each best-effort reset RPC at startup and shutdown.
const resetActivityTimeout = 2 * time.Second

// resetPaneActivityAsync clears whatever hint a previous squad left on this pane: the daemon
// keeps hints until it restarts, so a squad that died while a pane was Background would
// otherwise leave it demoted. Fire-and-forget; a no-op on daemons without the capability.
func (s *tymuxGRPCSession) resetPaneActivityAsync() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), resetActivityTimeout)
		defer cancel()
		_ = s.SetPaneActivity(ctx, qos.ActivityNormal)
	}()
}

// ResetPaneActivities returns every hinted pane to Normal. Call it on shutdown, after the hubs
// have stopped sending, so a stopped squad never leaves a pane boosted or demoted.
func ResetPaneActivities(ctx context.Context) {
	claimedSessions.Range(func(_, v any) bool {
		s, ok := v.(*tymuxGRPCSession)
		if !ok || qos.Activity(s.activity.lastSent.Load()) == qos.ActivityNormal {
			return true
		}
		_ = s.SetPaneActivity(ctx, qos.ActivityNormal)
		return true
	})
}

// activityHintAvailable is true when a sender exists and the daemon advertises the capability.
func (s *tymuxGRPCSession) activityHintAvailable(ctx context.Context) bool {
	switch s.activity.capability.Load() {
	case capUnsupported:
		return false
	case capSupported:
		return s.activity.sender.Load() != nil
	}
	sender := s.activity.sender.Load()
	if sender == nil && newPaneActivitySender != nil {
		if built := newPaneActivitySender(s.transport); built != nil {
			s.activity.sender.CompareAndSwap(nil, &built)
			sender = s.activity.sender.Load()
		}
	}
	if sender == nil {
		s.activity.capability.Store(capUnsupported)
		s.logActivityHintOnce("tymux client has no SetPaneActivity (pin predates tymux#86)")
		return false
	}
	info, err := s.transport.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	if connect.CodeOf(err) == connect.CodeUnimplemented {
		s.activity.capability.Store(capUnsupported)
		s.logActivityHintOnce("daemon has no GetServerInfo")
		return false
	}
	if err != nil {
		return false
	}
	if !slices.Contains(info.Msg.GetCapabilities(), capPaneActivityHint) {
		s.activity.capability.Store(capUnsupported)
		s.logActivityHintOnce("daemon does not advertise " + capPaneActivityHint)
		return false
	}
	s.activity.capability.Store(capSupported)
	return true
}

func (s *tymuxGRPCSession) logActivityHintOnce(reason string) {
	if s.activity.loggedOnce.CompareAndSwap(false, true) {
		log.Info("tymux: pane activity hints disabled for this session", "reason", reason)
	}
}
