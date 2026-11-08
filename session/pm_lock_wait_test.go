package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

func pmLockWaitTotal() uint64 {
	var n uint64
	for _, s := range termlat.Default.Report() {
		if s.Stage == termlat.StagePMLockWait {
			n += s.Total
		}
	}
	return n
}

func TestInstancePM_should_RecordTheWaitOnlyWhenPmMuIsContended(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "pm-wait"}
	inst.processManager = &fakeSendKeysProcessManager{}
	before := pmLockWaitTotal()

	inst.pm()
	assert.Equal(t, before, pmLockWaitTotal(), "an uncontended call must record nothing")

	inst.pmMu.Lock()
	done := make(chan struct{})
	go func() { inst.pm(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	inst.pmMu.Unlock()
	<-done

	assert.Greater(t, pmLockWaitTotal(), before)
}
