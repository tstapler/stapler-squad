package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
)

// T-WL-11
func TestTerminalWriteLeaseFlag_ShouldSerializeWhenOnAndReturnANonExclusiveLeaseWhenOffAndFailClosedOnAnUnreadableConfig(t *testing.T) {
	t.Parallel()
	var nilFlag *instance.LeaseFlag
	assert.True(t, nilFlag.Enabled(), "an unreadable config (no flag value) keeps the lease on")
	assert.True(t, (&instance.LeaseFlag{}).Enabled(), "default on")

	flag := &instance.LeaseFlag{}
	inst := &instance.Instance{Title: "flag", UUID: fixturetest.FakeLeaseOwner}
	inst.SetLeaseFlag(flag)

	a, ok := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)
	require.True(t, ok)
	_, ok = inst.TryTerminalWriteLease(instance.LeaseWriterMCP)
	assert.False(t, ok, "on: exclusive")
	a.Release()

	flag.SetEnabled(false)
	b, ok := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)
	require.True(t, ok)
	c, ok := inst.TryTerminalWriteLease(instance.LeaseWriterMCP)
	require.True(t, ok, "off: non-exclusive, today's behavior")
	d, err := inst.AcquireTerminalWriteLease(context.Background(), instance.LeaseWriterMCP, 0)
	require.NoError(t, err)

	// The capability types keep working with the flag off; a zero value is still refused.
	pane := fixturetest.NewFakePaneSubmitter()
	pane.Updates = []bool{true}
	require.ErrorIs(t, instance.SubmitDriverContent(context.Background(), pane, &instance.HeldLease{}, "x", time.Millisecond, time.Millisecond), instance.ErrNoLease)
	require.NoError(t, instance.SendKeysWithTimeout(context.Background(), pane, b, "x", time.Second))
	c.Release()
	d.Release()

	flag.SetEnabled(true)
	instance.AssertLeaseFree(t, inst)
	e, ok := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)
	require.True(t, ok)
	_, ok = inst.TryTerminalWriteLease(instance.LeaseWriterMCP)
	assert.False(t, ok, "back on: exclusive again")
	e.Release()
}
