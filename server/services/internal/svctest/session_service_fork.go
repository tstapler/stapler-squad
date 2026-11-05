package svctest

import (
	"github.com/tstapler/stapler-squad/session"
)

// addInstanceToPoller registers a session instance directly with the poller so
// that findInstance() can resolve it by title (the normal production path).
func AddInstanceToPoller(poller *session.ReviewQueuePoller, inst *session.Instance) {
	poller.SetInstances(append(poller.GetInstances(), inst))
}
