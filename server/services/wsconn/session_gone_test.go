package wsconn

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tymux"
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

func TestLogCurrentPaneFailure_PaneNotRunningIsQuietAndNonFatal(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("failed to capture fresh pane content: %w",
		fmt.Errorf("%w: %w", tymux.ErrPaneNotRunning, connect.NewError(connect.CodeFailedPrecondition, errors.New("pane is not running"))))

	assert.NoError(t, logCurrentPaneFailure("mid-stream current pane request", "pane-quiet-test", "r1", err))
	assert.False(t, goneLogThrottle.allow("pane-not-running:pane-quiet-test", time.Now()), "first failure must have claimed the throttle slot")
}

func TestPaneNotLiveReply(t *testing.T) {
	req := &sessionv1.CurrentPaneRequest{ResyncId: "r1"}
	out := paneNotLiveReply(req, fmt.Errorf("%w: %w", tymux.ErrPaneNotRunning, connect.NewError(connect.CodeFailedPrecondition, errors.New("pane is not running"))))
	if out == nil || out.GetResyncId() != "r1" || len(out.GetData()) != 0 {
		t.Fatalf("want empty reply echoing resync id, got %v", out)
	}
	if paneNotLiveReply(req, connect.NewError(connect.CodeInternal, errors.New("boom"))) != nil {
		t.Fatal("non-FailedPrecondition must not get a synthetic reply")
	}
}
