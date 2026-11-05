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

// upgradableWriter holds a stream's pane-write capability. A tymux attach that
// was read-only only because the instance was not Active (any non-Active status)
// upgrades once, one-way, when it becomes Active. Only streamViaHub upgrades;
// archived and hidden attaches never do.
type upgradableWriter struct {
	handler  *ConnectRPCWebSocketHandler
	instance *session.Instance

	mu         sync.Mutex
	writer     TerminalWriter
	upgradable bool
	flipped    bool
	onUpgrade  func()
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
func (u *upgradableWriter) NoteDroppedResize(cols, rows int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastCols, u.lastRows = cols, rows
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
	already := u.flipped
	u.mu.Unlock()
	if already {
		u.runUpgradeCallback(fn)
	}
}

// Writer returns the current capability, nil while the attach is read-only.
func (u *upgradableWriter) Writer() TerminalWriter {
	u.mu.Lock()
	if u.writer == nil && u.upgradable {
		writer, upgradable := u.handler.writerDecision(u.instance)
		u.upgradable = upgradable
		if writer != nil {
			u.writer = writer
			u.flipped = true
			fn := u.onUpgrade
			u.mu.Unlock()
			log.Info("[WebSocket] read-only attach upgraded to read-write", "session", u.instance.GetTitle())
			if fn != nil {
				u.runUpgradeCallback(fn)
			}
			u.mu.Lock()
		}
	}
	defer u.mu.Unlock()
	return u.writer
}

func (u *upgradableWriter) runUpgradeCallback(fn func()) {
	fn()
	u.mu.Lock()
	defer u.mu.Unlock()
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

// wireWriterUpgrade upgrades the hub subscriber's capability and casts its resize
// vote (latest dropped resize, else the handshake size) when the instance becomes
// Active, observed from session-updated events so no client frame is needed.
func (h *ConnectRPCWebSocketHandler) wireWriterUpgrade(connCtx context.Context, w writerUpgrade) {
	w.writer.SetOnUpgrade(func() {
		w.hub.UpgradeSubscriber(w.id, streamhub.SubscriberCapability{
			CanResize: true,
			CanWrite:  w.instance.Snapshot().Permissions.CanSendCommand,
		})
		cols, rows, ok := w.writer.LastDroppedResize()
		if !ok {
			voteHandshakeSize(connCtx, w.hub, w.id, w.handshake)
			return
		}
		voteSize(connCtx, w.hub, w.id, cols, rows)
	})
	if w.bus == nil || !w.writer.stillUpgradable() {
		_ = w.writer.Writer() // covers a flip that happened before SetOnUpgrade
		return
	}
	ch, subID := w.bus.Subscribe(connCtx)
	go func() {
		defer w.bus.Unsubscribe(subID)
		// Check after subscribing so a flip between the attach and Subscribe is not missed.
		if w.writer.Writer() != nil {
			return
		}
		w.upgradeOnSessionUpdates(connCtx, ch)
	}()
}

func (w writerUpgrade) upgradeOnSessionUpdates(connCtx context.Context, ch <-chan *events.Event) {
	for {
		select {
		case <-connCtx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if ev.Type != events.EventSessionUpdated || ev.Session == nil || ev.Session.UUID != w.instance.UUID {
				continue
			}
			if w.writer.Writer() != nil || !w.writer.stillUpgradable() {
				return
			}
		}
	}
}
