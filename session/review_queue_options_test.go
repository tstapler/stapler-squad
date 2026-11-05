package session

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
)

// optsRecordingDeterminer captures the options each Determine call receives.
type optsRecordingDeterminer struct {
	mu   sync.Mutex
	seen []DeterminerOptions
}

func (d *optsRecordingDeterminer) Determine(_ *Instance, _ string, _ InstanceStatusInfo, _ detection.TerminalDetector, opts DeterminerOptions) DetectionResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, opts)
	return DetectionResult{Action: DetectionActionSkip}
}

func TestDeterminerOptions_Should_ReadResolverOncePerCycle_When_ThreeSessionsPolled(t *testing.T) {
	t.Parallel()
	rqp := NewReviewQueuePoller(NewReviewQueue(), NewInstanceStatusManager(), nil)
	rec := &optsRecordingDeterminer{}
	rqp.statusDeterminer = rec

	var reads atomic.Int32
	rqp.SetOptionsSource(func() (DeterminerOptions, RuleOptions) {
		reads.Add(1)
		return DeterminerOptions{ProseQuestion: true}, RuleOptions{}
	})

	instances := make([]*Instance, 3)
	for i := range instances {
		instances[i] = makeStaleInstance(rqp, fmt.Sprintf("opts-session-%d", i))
	}
	rqp.SetInstances(instances)

	rqp.checkSessions()

	if got := reads.Load(); got != 1 {
		t.Errorf("resolver reads per cycle = %d, want 1", got)
	}
	if len(rec.seen) != 3 {
		t.Fatalf("Determine calls = %d, want 3", len(rec.seen))
	}
	for _, o := range rec.seen {
		if !o.ProseQuestion {
			t.Errorf("session got zero options, want the cycle's options: %+v", o)
		}
	}
}

func TestDeterminerOptions_Should_BeZeroValue_When_NoSourceWired(t *testing.T) {
	t.Parallel()
	var src FlagOptionsSource
	d, r := src.resolve()
	if d != (DeterminerOptions{}) || r != (RuleOptions{}) {
		t.Errorf("nil source resolved to %+v %+v, want zero values", d, r)
	}
}

func TestRuleOptions_Should_ReadAckBypassFlagOncePerCycleAndNotEnterDeterminer_When_ThreeSessionsPolled(t *testing.T) {
	t.Parallel()
	rqp := NewReviewQueuePoller(NewReviewQueue(), NewInstanceStatusManager(), nil)
	rec := &optsRecordingDeterminer{}
	rqp.statusDeterminer = rec

	var reads atomic.Int32
	rqp.SetOptionsSource(func() (DeterminerOptions, RuleOptions) {
		reads.Add(1)
		return DeterminerOptions{}, RuleOptions{AckBypassFix: true}
	})
	instances := make([]*Instance, 3)
	for i := range instances {
		instances[i] = makeStaleInstance(rqp, fmt.Sprintf("rule-opts-session-%d", i))
	}
	rqp.SetInstances(instances)

	rqp.checkSessions()

	if got := reads.Load(); got != 1 {
		t.Errorf("resolver reads per cycle = %d, want 1", got)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, o := range rec.seen {
		if o != (DeterminerOptions{}) {
			t.Errorf("determiner saw %+v, want zero options (AckBypassFix is a RuleOptions flag)", o)
		}
	}
}
