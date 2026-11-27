package autonomysvc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// canRestart is the last check before an unattended write or driver start.
func TestCanRestart_ShouldRefuse_When_AnyOfItsConditionsFails(t *testing.T) {
	t.Parallel()
	live := func() *instance.Instance {
		return &instance.Instance{Title: "r", UUID: "r-uuid", Status: domain.Active, AutonomousMode: true}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	assert.True(t, canRestart(context.Background(), live()))
	assert.False(t, canRestart(cancelled, live()), "a requested stop")

	archived := live()
	at := time.Now()
	archived.ArchivedAt = &at
	assert.False(t, canRestart(context.Background(), archived), "an archived session")

	paused := live()
	paused.Status = domain.Paused
	assert.False(t, canRestart(context.Background(), paused), "a session that is not active")

	manual := live()
	manual.AutonomousMode = false
	assert.False(t, canRestart(context.Background(), manual), "autonomy turned off")

	retrying := live()
	retrying.SetRetryProgress(func(rs *domain.RetryState) { rs.NextRetryAt = time.Now().Add(time.Hour) })
	assert.False(t, canRestart(context.Background(), retrying), "a pending automated retry")
}
