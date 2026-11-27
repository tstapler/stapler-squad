package streamhub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/qos"
)

// The window is between a pump arming its retry and releasing ownership, too narrow to hit
// through the public API, so the busy pump is simulated.
func TestCloseQoS_should_MakeABusyPumpLoopAgain_When_ItsKickIsCoalesced(t *testing.T) {
	t.Parallel()
	h := &StreamHub{
		sessionName:  "s",
		qos:          qos.NewTracker(time.Now, qos.InteractiveWindow),
		activitySink: func(qos.Activity) error { return nil },
	}
	h.activity.pending.Store(true)

	h.closeQoS()

	require.True(t, h.activity.force.Load(), "without force the pump exits and the final Normal is lost")
}
