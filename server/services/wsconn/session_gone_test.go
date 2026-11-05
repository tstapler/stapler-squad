package wsconn

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
)

func TestLogThrottle_AdmitsOncePerIntervalPerKey(t *testing.T) {
	th := newLogThrottle(30 * time.Second)
	t0 := time.Unix(1000, 0)
	assert.True(t, th.allow("a", t0))
	assert.False(t, th.allow("a", t0.Add(29*time.Second)))
	assert.True(t, th.allow("b", t0.Add(time.Second)), "keys are independent")
	assert.True(t, th.allow("a", t0.Add(30*time.Second)))
}

func TestLogThrottle_PrunesExpiredEntries(t *testing.T) {
	th := newLogThrottle(time.Second)
	t0 := time.Unix(2000, 0)
	for i := 0; i < 2*logThrottleSweepAt; i++ {
		th.allow(fmt.Sprintf("s%d", i), t0)
	}
	th.allow("late", t0.Add(time.Minute))
	th.mu.Lock()
	defer th.mu.Unlock()
	assert.LessOrEqual(t, len(th.last), 1, "expired keys must be swept")
}

func TestDefinitivelyGone_RequiresBackendToAgree(t *testing.T) {
	notFound := connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
	assert.True(t, definitivelyGone(notFound, func() bool { return false }))
	assert.False(t, definitivelyGone(notFound, func() bool { return true }), "backend still alive: paint blank, keep the stream")
	assert.False(t, definitivelyGone(errors.New("boom"), func() bool { return false }))
}
