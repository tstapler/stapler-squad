package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"time"
)

// instanceOption mutates a started test Instance; now is the builder's single clock reading.
type instanceOption func(inst *instance.Instance, now time.Time)

// startedInstance is a started, Running session with UpdatedAt = now. Determine reads the
// wall clock directly, so timestamps are taken at construction, immediately before use.
func startedInstance(title string, opts ...instanceOption) *instance.Instance {
	now := time.Now()
	inst := &instance.Instance{Title: title, UUID: "uuid-" + title, Status: domain.Active}
	inst.MarkStarted()
	inst.UpdatedAt = now
	for _, o := range opts {
		o(inst, now)
	}
	inst.SyncAtomicTimestamps()
	return inst
}

// withAges sets UpdatedAt and LastMeaningfulOutput as offsets before now.
func withAges(updatedAgo, outputAgo time.Duration) instanceOption {
	return func(inst *instance.Instance, now time.Time) {
		inst.UpdatedAt = now.Add(-updatedAgo)
		inst.LastMeaningfulOutput = now.Add(-outputAgo)
	}
}

func withCreatedAgo(d time.Duration) instanceOption {
	return func(inst *instance.Instance, now time.Time) { inst.CreatedAt = now.Add(-d) }
}

// checkSession evaluates one session with the poller's current flag options.
func (rqp *ReviewQueuePoller) checkSession(inst *instance.Instance, paneActivity map[string]time.Time) {
	opts, ruleOpts := rqp.currentOptions()
	rqp.evaluateSession(inst, paneActivity, opts, ruleOpts)
}
