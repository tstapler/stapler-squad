package session

import (
	"time"
)

// instanceOption mutates a started test Instance; now is the builder's single clock reading.
type instanceOption func(inst *Instance, now time.Time)

// startedInstance is a started, Running session with UpdatedAt = now. Determine reads the
// wall clock directly, so timestamps are taken at construction, immediately before use.
func startedInstance(title string, opts ...instanceOption) *Instance {
	now := time.Now()
	inst := &Instance{Title: title, UUID: "uuid-" + title, Status: Running}
	inst.started.Store(true)
	inst.UpdatedAt = now
	for _, o := range opts {
		o(inst, now)
	}
	inst.SyncAtomicTimestamps()
	return inst
}

// withAges sets UpdatedAt and LastMeaningfulOutput as offsets before now.
func withAges(updatedAgo, outputAgo time.Duration) instanceOption {
	return func(inst *Instance, now time.Time) {
		inst.UpdatedAt = now.Add(-updatedAgo)
		inst.LastMeaningfulOutput = now.Add(-outputAgo)
	}
}

func withCreatedAgo(d time.Duration) instanceOption {
	return func(inst *Instance, now time.Time) { inst.CreatedAt = now.Add(-d) }
}

// checkSession evaluates one session with the poller's current flag options.
func (rqp *ReviewQueuePoller) checkSession(inst *Instance, paneActivity map[string]time.Time) {
	opts, ruleOpts := rqp.currentOptions()
	rqp.evaluateSession(inst, paneActivity, opts, ruleOpts)
}
