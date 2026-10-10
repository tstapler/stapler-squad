package session

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

// ptyWritePM routes SendKeys to a real PTY master and makes Close close one end
// of the pair, modelling TmuxSession.Close (master) and the kill of the attach
// process (slave).
type ptyWritePM struct {
	*stuckDialogProcessManager
	master, slave *os.File
	closeSlave    bool
	closeOnce     sync.Once
}

func (p *ptyWritePM) SendKeys(keys string) (int, error) { return p.master.WriteString(keys) }

func (p *ptyWritePM) Close() (err error) {
	p.closeOnce.Do(func() {
		if p.closeSlave {
			err = p.slave.Close()
			return
		}
		err = p.master.Close()
	})
	return err
}

// T-WL-07: a write wedged on a real PTY whose reader stopped keeps the lease,
// and Pause and Delete complete while it is wedged (they are not acquirers).
//
// Settled finding (plan assumption was INFERRED, now VERIFIED the other way):
// closing the master or the slave does NOT wake a blocked master write on this
// kernel; only a slave reader does. So the lease is released only when the
// blocked goroutine's write returns (here: after the test drains the slave).
func TestWedgedWrite_ShouldKeepTheLeaseAndPauseAndDeleteShouldCompleteWhileItIsWedged_WhenARealPtyPairStopsBeingRead(t *testing.T) {
	for _, tc := range []struct {
		name       string
		closeSlave bool
	}{{"close master", false}, {"close slave", true}} {
		t.Run(tc.name, func(t *testing.T) {
			master, slave, err := pty.Open()
			require.NoError(t, err)
			_, err = term.MakeRaw(int(slave.Fd()))
			require.NoError(t, err)

			pm := &ptyWritePM{
				stuckDialogProcessManager: &stuckDialogProcessManager{dialogText: "idle"},
				master:                    master, slave: slave, closeSlave: tc.closeSlave,
			}
			inst := &Instance{Title: "wedged-" + tc.name, Status: Active, processManager: pm, Permissions: GetManagedPermissions()}
			inst.started.Store(true)
			inst.SetLeaseFlag(&LeaseFlag{})

			lease, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
			require.True(t, ok)
			err = SendKeysWithTimeout(context.Background(), inst, lease, strings.Repeat("x", 4<<20), 100*time.Millisecond)
			require.ErrorIs(t, err, context.DeadlineExceeded, "the write must wedge on the unread PTY")

			_, ok = inst.TryTerminalWriteLease(LeaseWriterDriver)
			require.False(t, ok, "no forced release: the wedged write still holds the lease")

			// Pause (which closes the PTY) completes while the write is wedged.
			paused := make(chan error, 1)
			go func() { paused <- inst.Pause() }()
			select {
			case err := <-paused:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Fatal("Pause queued behind the wedged write")
			}

			// Delete completes too.
			deleted := make(chan error, 1)
			go func() { deleted <- inst.Destroy() }()
			select {
			case err := <-deleted:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Fatal("Destroy queued behind the wedged write")
			}

			// Observed, not asserted: whether the closes above woke the write.
			if l, free := inst.TryTerminalWriteLease(LeaseWriterOther); free {
				l.Release()
				t.Logf("%s: the PTY close woke the blocked write", tc.name)
			} else {
				t.Logf("%s: the PTY close has not woken the blocked write (a standalone 2s probe of the same pair never saw it wake; only a slave reader does)", tc.name)
			}

			// A slave reader is what ends a blocked master write: attach one
			// (re-opening the slave by name when the pair's slave was closed)
			// so the writer goroutine joins and the lease is released.
			reader, err := os.OpenFile(slave.Name(), os.O_RDWR|syscall.O_NOCTTY, 0)
			require.NoError(t, err)
			go func() { _, _ = io.Copy(io.Discard, reader) }()
			t.Cleanup(func() { _ = reader.Close() })
			waitLeaseFree(t, inst)
		})
	}
}
