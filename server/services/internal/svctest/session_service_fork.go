package svctest

import (
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
)

// addInstanceToPoller registers a session instance directly with the poller so
// that findInstance() can resolve it by title (the normal production path).
func AddInstanceToPoller(poller *session.ReviewQueuePoller, inst *instance.Instance) {
	poller.SetInstances(append(poller.GetInstances(), inst))
}
