package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// leasedCtxFake is an InstanceContext whose lease comes from a real Instance;
// only WriteToPTY is exercised.
type leasedCtxFake struct {
	InstanceContext
	inst     *Instance
	entered  chan struct{} // closed when the accessor starts waiting for the lease
	released atomic.Bool   // set by the test just before the holder releases
	mu       sync.Mutex
	wrote    [][]byte
	// wroteBeforeRelease and wroteWithoutLease record contract violations.
	wroteBeforeRelease, wroteWithoutLease bool
}

func (f *leasedCtxFake) AcquireTerminalWriteLease(ctx context.Context, writer string, maxWait time.Duration) (*HeldLease, error) {
	close(f.entered)
	return f.inst.AcquireTerminalWriteLease(ctx, writer, maxWait)
}

func (f *leasedCtxFake) WriteToPTY(data []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wroteBeforeRelease = f.wroteBeforeRelease || !f.released.Load()
	if l, free := f.inst.TryTerminalWriteLease(LeaseWriterOther); free {
		l.Release()
		f.wroteWithoutLease = true
	}
	f.wrote = append(f.wrote, data)
	return len(data), nil
}

func TestLeasedSessionAccessor_ShouldWaitForTheLeaseThenWriteUnderItAndRelease_WhenAnotherWriterHoldsItBriefly(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "ratelimit")
	fake := &leasedCtxFake{inst: inst, entered: make(chan struct{})}
	holder, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)

	done := make(chan error, 1)
	go func() {
		_, err := leasedSessionAccessor{fake}.WriteToPTY([]byte("go\n"))
		done <- err
	}()
	<-fake.entered
	fake.released.Store(true)
	holder.Release()

	require.NoError(t, <-done)
	assert.Len(t, fake.wrote, 1)
	assert.False(t, fake.wroteBeforeRelease, "nothing is typed while another writer holds the pane")
	assert.False(t, fake.wroteWithoutLease, "the recovery input is typed while the wrapper holds the lease")
	AssertLeaseFree(t, inst)
}

func TestLeasedSessionAccessor_ShouldWriteDirectly_WhenTheContextCannotAcquireALease(t *testing.T) {
	t.Parallel()
	fake := &noLeaseCtxFake{}
	_, err := leasedSessionAccessor{fake}.WriteToPTY([]byte("x"))
	require.NoError(t, err)
	assert.Equal(t, 1, fake.writes)
}

type noLeaseCtxFake struct {
	InstanceContext
	writes int
}

func (f *noLeaseCtxFake) WriteToPTY(d []byte) (int, error) { f.writes++; return len(d), nil }
