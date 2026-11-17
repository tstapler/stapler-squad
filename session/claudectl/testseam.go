package claudectl

import (
	"context"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
	"github.com/tstapler/stapler-squad/session/ptyio"
)

// Test seams for tests outside this package (session, server/services) that
// need an unstarted controller over an in-memory pane. Not for production use.

// NewBareForTest returns an unstarted controller with no detectors or pane.
func NewBareForTest(sessionName string, inst InstanceContext) *ClaudeController {
	return &ClaudeController{sessionName: sessionName, instance: inst}
}

// SetPTYAccessForTest installs pa as the controller's pane without Start().
func (cc *ClaudeController) SetPTYAccessForTest(pa *ptyio.PTYAccess) { cc.ptyAccess.Store(pa) }

// SetDetectorsForTest installs the status and idle detectors without Start().
func (cc *ClaudeController) SetDetectorsForTest(sd *detection.StatusDetector, id *detection.IdleDetector) {
	cc.statusDetector.Store(sd)
	cc.idleDetector.Store(id)
}

// SetRateLimitHandlerForTest installs h as the rate-limit consumer.
func (cc *ClaudeController) SetRateLimitHandlerForTest(h *ratelimit.PTYConsumer) {
	cc.rateLimitHandler.Store(h)
}

// SetSessionNameForTest overrides the session name set at construction.
func (cc *ClaudeController) SetSessionNameForTest(name string) { cc.sessionName = name }

// MarkRunningForTest gives the controller a non-nil lifecycle context so it
// reports started, without spawning the background goroutines Start() would.
func (cc *ClaudeController) MarkRunningForTest(ctx context.Context) {
	cc.lifecycle.Write(func(l *controllerLifecycle) { l.ctx = ctx })
}

// StatusListenersForTest returns a copy of the registered status-change listeners.
func (cc *ClaudeController) StatusListenersForTest() []StatusChangeListener {
	var out []StatusChangeListener
	cc.listeners.Read(func(ls []StatusChangeListener) {
		out = append(out, ls...)
	})
	return out
}
