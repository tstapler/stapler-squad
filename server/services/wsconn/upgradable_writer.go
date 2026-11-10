package wsconn

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/streamhub"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// upgradableWriter holds a stream's pane-write capability. An attach that is
// read-only for a recoverable reason (session.TerminalInputBlock: a tymux
// instance that is not Active yet, or archived) upgrades once, one-way, when the
// block clears, so the stream agrees with Session.terminal_read_only. Only
// streamViaHub upgrades; hidden attaches never do.
//
// The new writer is exposed only after the upgrade callback has finished, so
// no input is forwarded before the hub subscriber's capability is upgraded.
type upgradableWriter struct {
	handler  *ConnectRPCWebSocketHandler
	instance *session.Instance

	mu         sync.Mutex
	writer     services.TerminalWriter
	upgradable bool
	upgrading  bool
	flipped    bool
	onUpgrade  func(services.TerminalWriter)
	granted    services.TerminalWriter // the writer being granted, valid once flipped
	lateResize func(writer services.TerminalWriter, cols, rows int)
	lastCols   int
	lastRows   int
	done       chan struct{}  // closed after the upgrade callback has run
	votes      sync.WaitGroup // in-flight asynchronous resize votes
}

func (h *ConnectRPCWebSocketHandler) newUpgradableWriter(instance *session.Instance) *upgradableWriter {
	writer, upgradable := h.writerDecision(instance)
	return &upgradableWriter{handler: h, instance: instance, writer: writer, upgradable: upgradable, done: make(chan struct{})}
}

// Upgraded is closed once a read-only attach has become read-write and its
// upgrade callback has finished.
func (u *upgradableWriter) Upgraded() <-chan struct{} { return u.done }

// NoteDroppedResize remembers the latest resize frame dropped while read-only.
// If the upgrade already started (the read loop saw "read-only" just before it),
// the resize is voted now instead of being lost.
func (u *upgradableWriter) NoteDroppedResize(cols, rows int) {
	u.mu.Lock()
	u.lastCols, u.lastRows = cols, rows
	late, granted := u.lateResize, u.granted
	u.mu.Unlock()
	if granted != nil && late != nil {
		late(granted, cols, rows)
	}
}

// WaitVotes blocks until every resize vote started so far has finished (tests, shutdown).
func (u *upgradableWriter) WaitVotes() { u.votes.Wait() }

// LastDroppedResize returns the latest resize recorded by NoteDroppedResize.
func (u *upgradableWriter) LastDroppedResize() (cols, rows int, ok bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastCols, u.lastRows, u.lastCols > 0 && u.lastRows > 0
}

// SetOnUpgrade registers fn, run once (outside the lock) when the attach becomes
// read-write; it runs immediately if that already happened.
func (u *upgradableWriter) SetOnUpgrade(fn func(services.TerminalWriter)) {
	u.mu.Lock()
	u.onUpgrade = fn
	granted := u.granted
	already := u.flipped && !u.upgrading
	u.mu.Unlock()
	if already {
		u.finishUpgrade(fn, granted)
	}
}

// Writer returns the current capability, nil while the attach is read-only
// (including while another caller is mid-upgrade).
func (u *upgradableWriter) Writer() services.TerminalWriter {
	u.mu.Lock()
	if u.writer != nil || !u.upgradable || u.upgrading {
		defer u.mu.Unlock()
		return u.writer
	}
	writer, upgradable := u.handler.writerDecision(u.instance)
	u.upgradable = upgradable
	if writer == nil {
		defer u.mu.Unlock()
		return nil
	}
	u.flipped = true
	u.granted = writer
	fn := u.onUpgrade
	if fn == nil {
		u.writer = writer // SetOnUpgrade runs the callback later
		defer u.mu.Unlock()
		return writer
	}
	u.upgrading = true
	u.mu.Unlock()
	log.Info("[WebSocket] read-only attach upgrading to read-write", "session", u.instance.GetTitle())
	fn(writer)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.writer = writer
	u.upgrading = false
	u.closeDone()
	return writer
}

func (u *upgradableWriter) finishUpgrade(fn func(services.TerminalWriter), granted services.TerminalWriter) {
	fn(granted)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.closeDone()
}

func (u *upgradableWriter) closeDone() {
	select {
	case <-u.done:
	default:
		close(u.done)
	}
}

func (u *upgradableWriter) stillUpgradable() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.writer == nil && u.upgradable
}

// writerUpgrade is everything wireWriterUpgrade needs to upgrade one hub subscriber.
type writerUpgrade struct {
	hub       *streamhub.StreamHub
	id        streamhub.SubscriberID
	writer    *upgradableWriter
	instance  *session.Instance
	handshake *sessionv1.CurrentPaneRequest
	bus       *events.EventBus                                                          // nil: upgrade only when a client frame next asks for the writer
	vote      func(ctx context.Context, writer services.TerminalWriter, cols, rows int) // nil: cast it on hub
}

// resizeVoteTimeout bounds one post-upgrade resize vote (SetWindowSize + capture).
const resizeVoteTimeout = 5 * time.Second

// attachUpgradableSubscriber attaches the stream to hub and arranges for a
// read-only attach to upgrade when the instance's block clears. The returned
// channel closes when the event-bus watcher has released its subscription.
func (h *ConnectRPCWebSocketHandler) attachUpgradableSubscriber(connCtx context.Context, hub *streamhub.StreamHub, a hubSubscriberAttachment) (*upgradableWriter, streamhub.SubscriberID, <-chan struct{}) {
	upWriter := h.newUpgradableWriter(a.instance)
	a.writer = upWriter.Writer()
	subscriberID := h.attachHubSubscriber(connCtx, hub, a)
	var bus *events.EventBus
	if h.sessionService != nil {
		bus = h.sessionService.GetEventBus()
	}
	watchDone := h.wireWriterUpgrade(connCtx, writerUpgrade{
		hub: hub, id: subscriberID, writer: upWriter, instance: a.instance, handshake: a.currentPaneReq, bus: bus,
	})
	return upWriter, subscriberID, watchDone
}

// wireWriterUpgrade upgrades the hub subscriber's capability and casts its resize
// vote (latest dropped resize, else the handshake size) when the instance's block
// clears, observed from session events so no client frame is needed. The returned
// channel closes when the bus watcher (if any) has unsubscribed.
func (h *ConnectRPCWebSocketHandler) wireWriterUpgrade(connCtx context.Context, w writerUpgrade) <-chan struct{} {
	castVote := w.vote
	if castVote == nil {
		castVote = func(ctx context.Context, writer services.TerminalWriter, cols, rows int) {
			voteSize(ctx, w.hub, w.id, writer, cols, rows)
		}
	}
	var voteMu sync.Mutex // serializes votes so the last recorded size wins
	// voteLatest runs asynchronously: a hung tmux resize must not block the input read
	// loop or keep the writer hidden. Each run votes the latest size recorded when it runs.
	voteLatest := func(writer services.TerminalWriter) {
		w.writer.votes.Add(1)
		go func() {
			defer w.writer.votes.Done()
			voteMu.Lock()
			defer voteMu.Unlock()
			ctx, cancel := context.WithTimeout(connCtx, resizeVoteTimeout)
			defer cancel()
			cols, rows, ok := w.writer.LastDroppedResize()
			if !ok {
				cols, rows, ok = handshakeSize(w.handshake)
			}
			if !ok {
				log.Warn("[streamViaHub] no size known for the post-upgrade resize vote, layout may be incorrect")
				return
			}
			castVote(ctx, writer, cols, rows)
		}()
	}
	w.writer.mu.Lock()
	w.writer.lateResize = func(writer services.TerminalWriter, _, _ int) { voteLatest(writer) }
	w.writer.mu.Unlock()
	w.writer.SetOnUpgrade(func(writer services.TerminalWriter) {
		w.hub.UpgradeSubscriber(w.id, streamhub.SubscriberCapability{
			CanResize: true,
			CanWrite:  w.instance.Snapshot().Permissions.CanSendCommand,
		})
		voteLatest(writer)
	})
	watchDone := make(chan struct{})
	if w.bus == nil || !w.writer.stillUpgradable() {
		_ = w.writer.Writer() // covers a flip that happened before SetOnUpgrade
		close(watchDone)
		return watchDone
	}
	ch, subID := w.bus.Subscribe(connCtx)
	go func() {
		defer close(watchDone)
		defer w.bus.Unsubscribe(subID)
		// Check after subscribing so a flip between the attach and Subscribe is not missed.
		if w.writer.Writer() != nil {
			return
		}
		w.upgradeOnSessionEvents(connCtx, ch)
	}()
	return watchDone
}

func (w writerUpgrade) upgradeOnSessionEvents(connCtx context.Context, ch <-chan *events.Event) {
	for {
		select {
		case <-connCtx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if !isSessionChangeFor(ev, w.instance) {
				continue
			}
			if w.writer.Writer() != nil || !w.writer.stillUpgradable() {
				return
			}
		}
	}
}

func isSessionChangeFor(ev *events.Event, instance *session.Instance) bool {
	if ev.Type != events.EventSessionUpdated && ev.Type != events.EventSessionArchived {
		return false
	}
	return ev.Session != nil && ev.Session.UUID == instance.UUID
}
