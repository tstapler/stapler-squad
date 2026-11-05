package services

import (
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// upgradableWriter holds a stream's pane-write capability and re-evaluates it
// when the attach was read-only only because a tymux instance was not Active
// yet (e.g. opened while PermanentlyFailed, then retried). Archived or hidden
// attaches never upgrade.
type upgradableWriter struct {
	handler  *ConnectRPCWebSocketHandler
	instance *session.Instance

	mu         sync.Mutex
	writer     TerminalWriter
	upgradable bool
	onUpgrade  func()
}

func (h *ConnectRPCWebSocketHandler) newUpgradableWriter(instance *session.Instance) *upgradableWriter {
	writer, upgradable := h.writerDecision(instance)
	return &upgradableWriter{handler: h, instance: instance, writer: writer, upgradable: upgradable}
}

// SetOnUpgrade registers fn, called once when a read-only attach becomes read-write.
func (u *upgradableWriter) SetOnUpgrade(fn func()) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.onUpgrade = fn
}

// Writer returns the current capability, nil while the attach is read-only.
func (u *upgradableWriter) Writer() TerminalWriter {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.writer != nil || !u.upgradable {
		return u.writer
	}
	writer, upgradable := u.handler.writerDecision(u.instance)
	u.upgradable = upgradable
	if writer == nil {
		return nil
	}
	u.writer = writer
	log.Info("[WebSocket] read-only attach upgraded to read-write", "session", u.instance.GetTitle())
	if u.onUpgrade != nil {
		u.onUpgrade()
	}
	return u.writer
}
