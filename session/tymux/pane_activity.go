package tymux

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/internal/qos"
	"github.com/tstapler/stapler-squad/log"
)

// capPaneActivityHint is the GetServerInfo capability for tymuxd's SetPaneActivity RPC.
const capPaneActivityHint = "pane.activity_hint"

// paneActivitySender performs the SetPaneActivity RPC. It is a seam so tests can fake the wire;
// the real sender is built from the session transport in pane_activity_adapter.go.
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

	// mu serializes sends so a re-send after reattach, the startup reset and a hub hint cannot
	// reorder on the wire. requested is the latest hint a caller asked for (delivered or not) and
	// seq counts those requests; both are guarded by mu.
	mu        sync.Mutex
	requested qos.Activity
	seq       uint64

	// resends counts in-flight stream-open resends so tests can wait for them to settle.
	resends sync.WaitGroup
}

// errPaneActivityPending marks a hint that was not delivered but may be later (no pane yet).
// Permanent shortfalls (no client support, older daemon) return nil: retrying cannot help.
var errPaneActivityPending = errors.New("tymux: pane activity hint not delivered yet")

// SetPaneActivity tells tymuxd how latency-sensitive this session's pane is right now. It is an
// advisory optimization. A nil return means the hint is settled: delivered, or impossible for the
// lifetime of this session (no client support, older daemon). A non-nil return means it was not
// delivered but a retry may succeed (no pane yet, RPC failure), so callers should retry; they must
// never treat it as fatal.
func (s *tymuxGRPCSession) SetPaneActivity(ctx context.Context, activity qos.Activity) error {
	s.activity.mu.Lock()
	defer s.activity.mu.Unlock()
	s.activity.requested = activity
	s.activity.seq++
	return s.sendActivityLocked(ctx, activity)
}

// sendActivityLocked performs one hint RPC; the caller holds activity.mu.
func (s *tymuxGRPCSession) sendActivityLocked(ctx context.Context, activity qos.Activity) error {
	if available, err := s.activityHintAvailable(ctx); !available {
		return err
	}
	s.mu.RLock()
	paneID := s.paneID
	s.mu.RUnlock()
	if paneID == "" {
		return errPaneActivityPending
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
// otherwise leave it demoted. Fire-and-forget; a no-op on daemons without the capability. It
// yields to any hub hint requested after this call, so a late reset never overwrites a newer hint.
func (s *tymuxGRPCSession) resetPaneActivityAsync() {
	s.activity.mu.Lock()
	seen := s.activity.seq
	s.activity.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), resetActivityTimeout)
		defer cancel()
		s.resetPaneActivity(ctx, seen)
	}()
}

func (s *tymuxGRPCSession) resetPaneActivity(ctx context.Context, seenSeq uint64) {
	s.activity.mu.Lock()
	defer s.activity.mu.Unlock()
	if s.activity.seq != seenSeq || s.activity.requested != qos.ActivityNormal {
		return
	}
	_ = s.sendActivityLocked(ctx, qos.ActivityNormal)
}

// resendPaneActivityAsync re-asserts the last requested non-Normal hint once an Attach stream is
// open. tymuxd holds a hint that has no open stream for only a few seconds, so a hint sent before
// the stream existed (or across a reconnect) would otherwise wait for the hub's next refresh.
func (s *tymuxGRPCSession) resendPaneActivityAsync() {
	s.activity.resends.Add(1)
	go func() {
		defer s.activity.resends.Done()
		ctx, cancel := context.WithTimeout(context.Background(), resetActivityTimeout)
		defer cancel()
		s.resendPaneActivity(ctx)
	}()
}

func (s *tymuxGRPCSession) resendPaneActivity(ctx context.Context) {
	s.activity.mu.Lock()
	defer s.activity.mu.Unlock()
	if want := s.activity.requested; want != qos.ActivityNormal {
		_ = s.sendActivityLocked(ctx, want)
	}
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
// When false, a nil error means permanently unavailable and a non-nil error means retry later.
func (s *tymuxGRPCSession) activityHintAvailable(ctx context.Context) (bool, error) {
	switch s.activity.capability.Load() {
	case capUnsupported:
		return false, nil
	case capSupported:
		return s.activity.sender.Load() != nil, nil
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
		s.logActivityHintOnce("transport cannot send SetPaneActivity")
		return false, nil
	}
	info, err := s.transport.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	if connect.CodeOf(err) == connect.CodeUnimplemented {
		s.activity.capability.Store(capUnsupported)
		s.logActivityHintOnce("daemon has no GetServerInfo")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !slices.Contains(info.Msg.GetCapabilities(), capPaneActivityHint) {
		s.activity.capability.Store(capUnsupported)
		s.logActivityHintOnce("daemon does not advertise " + capPaneActivityHint)
		return false, nil
	}
	s.activity.capability.Store(capSupported)
	return true, nil
}

func (s *tymuxGRPCSession) logActivityHintOnce(reason string) {
	if s.activity.loggedOnce.CompareAndSwap(false, true) {
		log.Info("tymux: pane activity hints disabled for this session", "reason", reason)
	}
}
