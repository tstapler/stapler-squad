//go:build unix

package session

import (
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fdSetSize is how many fd numbers a select(2) FdSet can hold.
var fdSetSize = len(unix.FdSet{}.Bits) * int(unsafe.Sizeof(unix.FdSet{}.Bits[0])) * 8

// waitReadable blocks for at most timeout until f has data to read or has hung
// up, and reports whether a Read would now return without blocking.
//
// It exists for PTY masters the Go runtime cannot poll (darwin: "file type does
// not support deadline"), where Read cannot be interrupted. Waiting with a
// short timeout lets the caller re-check cancellation between waits.
//
// The fd is reached through RawConn.Control, which (unlike File.Fd) leaves it
// in blocking mode so concurrent Write calls keep their blocking semantics,
// and holds a reference so a concurrent Close cannot recycle the fd number
// under the wait.
func waitReadable(f *os.File, timeout time.Duration) (bool, error) {
	rc, err := f.SyscallConn()
	if err != nil {
		return false, closedErr(err)
	}
	ready := false
	var waitErr error
	if err := rc.Control(func(fd uintptr) {
		// No FIONREAD fast path: on a darwin PTY master it reports the slave's
		// unread *input* queue, not pending output, so it can claim "ready" and
		// send the caller into an uninterruptible Read.
		ready, waitErr = waitFD(int(fd), timeout)
	}); err != nil {
		return false, closedErr(err)
	}
	if errors.Is(waitErr, unix.EBADF) {
		// The fd died under a still-open os.File; report it like a closed file so
		// the caller's Read surfaces it through the EOF mapping (poll(2) already
		// reaches the same outcome via POLLNVAL).
		return false, closedErr(waitErr)
	}
	return ready, waitErr
}

// waitFD waits for fd to become readable. select(2) is about twice as fast as
// poll(2) on darwin for this one-fd wait (measured: BenchmarkResponseStream_FloodedPTY),
// but cannot take fds beyond its FdSet, which a server with many sessions can
// exceed, so those use poll(2).
func waitFD(fd int, timeout time.Duration) (bool, error) {
	if fd < fdSetSize {
		return selectReadable(fd, timeout)
	}
	return pollReadable(fd, timeout)
}

func selectReadable(fd int, timeout time.Duration) (bool, error) {
	var set unix.FdSet
	set.Set(fd)
	tv := unix.NsecToTimeval(int64(timeout))
	n, err := unix.Select(fd+1, &set, nil, nil, &tv)
	return n > 0, ignoreEINTR(err)
}

func pollReadable(fd int, timeout time.Duration) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	ms := int((timeout + time.Millisecond - 1) / time.Millisecond) // round up: 0 would busy-spin
	n, err := unix.Poll(fds, ms)
	return n > 0, ignoreEINTR(err)
}

// ignoreEINTR turns an interrupted wait into "not ready, no error": the caller
// loops and re-checks cancellation anyway.
func ignoreEINTR(err error) error {
	if errors.Is(err, unix.EINTR) {
		return nil
	}
	return err
}

// closedErr marks an error from SyscallConn/Control (only returned for a closed
// file) so callers can tell it from a failure of the wait itself.
func closedErr(err error) error { return fmt.Errorf("%w: %v", os.ErrClosed, err) }
