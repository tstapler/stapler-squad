package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// T-WL-11
func TestTerminalWriteLeaseFlag_ShouldSerializeWhenOnAndReturnANonExclusiveLeaseWhenOffAndFailClosedOnAnUnreadableConfig(t *testing.T) {
	t.Parallel()
	var nilFlag *LeaseFlag
	assert.True(t, nilFlag.Enabled(), "an unreadable config (no flag value) keeps the lease on")
	assert.True(t, (&LeaseFlag{}).Enabled(), "default on")

	flag := &LeaseFlag{}
	inst := &Instance{Title: "flag", UUID: sessiontest.FakeLeaseOwner}
	inst.SetLeaseFlag(flag)

	a, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)
	_, ok = inst.TryTerminalWriteLease(LeaseWriterMCP)
	assert.False(t, ok, "on: exclusive")
	a.Release()

	flag.SetEnabled(false)
	b, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)
	c, ok := inst.TryTerminalWriteLease(LeaseWriterMCP)
	require.True(t, ok, "off: non-exclusive, today's behavior")
	d, err := inst.AcquireTerminalWriteLease(context.Background(), LeaseWriterMCP, 0)
	require.NoError(t, err)

	// The capability types keep working with the flag off; a zero value is still refused.
	pane := sessiontest.NewFakePaneSubmitter()
	pane.Updates = []bool{true}
	require.ErrorIs(t, SubmitDriverContent(context.Background(), pane, &HeldLease{}, "x", time.Millisecond, time.Millisecond), ErrNoLease)
	require.NoError(t, SendKeysWithTimeout(context.Background(), pane, b, "x", time.Second))
	c.Release()
	d.Release()

	flag.SetEnabled(true)
	AssertLeaseFree(t, inst)
	e, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)
	_, ok = inst.TryTerminalWriteLease(LeaseWriterMCP)
	assert.False(t, ok, "back on: exclusive again")
	e.Release()
}
