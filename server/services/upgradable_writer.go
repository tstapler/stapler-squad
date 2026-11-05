package services

import (
	"context"
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
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
	writer     TerminalWriter
	upgradable bool
	upgrading  bool
	flipped    bool
	onUpgrade  func()
	lateResize func(cols, rows int)
	lastCols   int
	lastRows   int
	done       chan struct{} // closed after the upgrade callback has run
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
	late := u.lateResize
	flipped := u.flipped
	u.mu.Unlock()
	if flipped && late != nil {
		late(cols, rows)
	}
}

// LastDroppedResize returns the latest resize recorded by NoteDroppedResize.
func (u *upgradableWriter) LastDroppedResize() (cols, rows int, ok bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastCols, u.lastRows, u.lastCols > 0 && u.lastRows > 0
}

// SetOnUpgrade registers fn, run once (outside the lock) when the attach becomes
// read-write; it runs immediately if that already happened.
func (u *upgradableWriter) SetOnUpgrade(fn func()) {
	u.mu.Lock()
	u.onUpgrade = fn
	already := u.flipped && !u.upgrading
	u.mu.Unlock()
	if already {
		u.finishUpgrade(fn)
	}
}

// Writer returns the current capability, nil while the attach is read-only
// (including while another caller is mid-upgrade).
func (u *upgradableWriter) Writer() TerminalWriter {
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
	fn := u.onUpgrade
	if fn == nil {
		u.writer = writer // SetOnUpgrade runs the callback later
		defer u.mu.Unlock()
		return writer
	}
	u.upgrading = true
	u.mu.Unlock()
	log.Info("[WebSocket] read-only attach upgrading to read-write", "session", u.instance.GetTitle())
	fn()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.writer = writer
	u.upgrading = false
	u.closeDone()
	return writer
}

func (u *upgradableWriter) finishUpgrade(fn func()) {
	fn()
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
	bus       *events.EventBus // nil: upgrade only when a client frame next asks for the writer
}

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
	var voteMu sync.Mutex // serializes votes so the last recorded size wins
	voteLatest := func() {
		voteMu.Lock()
		defer voteMu.Unlock()
		if cols, rows, ok := w.writer.LastDroppedResize(); ok {
			voteSize(connCtx, w.hub, w.id, cols, rows)
			return
		}
		voteHandshakeSize(connCtx, w.hub, w.id, w.handshake)
	}
	w.writer.mu.Lock()
	w.writer.lateResize = func(int, int) { voteLatest() }
	w.writer.mu.Unlock()
	w.writer.SetOnUpgrade(func() {
		w.hub.UpgradeSubscriber(w.id, streamhub.SubscriberCapability{
			CanResize: true,
			CanWrite:  w.instance.Snapshot().Permissions.CanSendCommand,
		})
		voteLatest()
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
