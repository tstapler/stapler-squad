//go:build !windows

package tymux

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/log"
)

// tymuxPTYAdapter fakes a PTY master fd with a Unix-domain socketpair: the
// external half goes to ProcessManager callers for Read/Write, the internal
// half is what the two pump goroutines below bridge to the fanout (output)
// and sendOnStream (input).
type tymuxPTYAdapter struct {
	external *os.File
	internal *os.File
	fanout   *ClientFanout
	subID    string

	closeOnce sync.Once
}

// newTymuxPTYAdapter subscribes to fanout and starts the two pump
// goroutines. The caller owns calling close() exactly once.
func newTymuxPTYAdapter(fanout *ClientFanout, send func(*v1.AttachRequest) error) (*tymuxPTYAdapter, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("tymux: socketpair for PTY adapter: %w", err)
	}
	// A blocking fd isn't registered with the runtime's netpoller, so
	// os.NewFile would return a non-pollable *os.File (SetReadDeadline
	// unsupported) and every Read/Write would park a whole OS thread —
	// non-blocking mode is what makes these behave like the real PTY
	// master fd TmuxSession.GetPTY() hands back.
	if err := syscall.SetNonblock(fds[0], true); err != nil {
		return nil, fmt.Errorf("tymux: set nonblocking on PTY adapter socketpair: %w", err)
	}
	if err := syscall.SetNonblock(fds[1], true); err != nil {
		return nil, fmt.Errorf("tymux: set nonblocking on PTY adapter socketpair: %w", err)
	}
	internal := os.NewFile(uintptr(fds[0]), "tymux-pty-internal")
	external := os.NewFile(uintptr(fds[1]), "tymux-pty")

	subID, outCh := fanout.Subscribe()

	a := &tymuxPTYAdapter{external: external, internal: internal, fanout: fanout, subID: subID}

	go a.pumpOutput(outCh)
	go a.pumpInput(send)

	return a, nil
}

// pumpOutput copies fanout broadcasts into the internal socket half until
// outCh closes (Unsubscribe) or the external half is closed by the caller.
func (a *tymuxPTYAdapter) pumpOutput(outCh chan []byte) {
	for data := range outCh {
		if _, err := a.internal.Write(data); err != nil {
			return
		}
	}
}

// pumpInput forwards bytes written to the external half onto the standing
// stream via send (sendOnStream). Exits on the next Read error after
// close() closes the internal half — except mid-flight in send() itself,
// which close() cannot interrupt (same limitation sendOnStream's other
// callers already have).
func (a *tymuxPTYAdapter) pumpInput(send func(*v1.AttachRequest) error) {
	buf := make([]byte, 4096)
	for {
		n, err := a.internal.Read(buf)
		if n > 0 {
			input := make([]byte, n)
			copy(input, buf[:n])
			if sendErr := send(&v1.AttachRequest{
				Payload: &v1.AttachRequest_Input{Input: input},
			}); sendErr != nil {
				log.Warn("tymux: PTY adapter: failed to forward input on standing stream", "err", sendErr)
			}
		}
		if err != nil {
			return
		}
	}
}

// file returns the external *os.File handed to ProcessManager callers — a
// method, not a bare field read, so the fieldless windows stand-in can
// satisfy the same call from session.go's platform-agnostic GetPTY().
func (a *tymuxPTYAdapter) file() *os.File { return a.external }

// close unsubscribes from the fanout and closes both socket halves. Safe to
// call more than once. Unblocks pumpOutput and a pumpInput parked in Read;
// a pumpInput mid-send() only exits once that call returns (see its doc).
func (a *tymuxPTYAdapter) close() {
	a.closeOnce.Do(func() {
		a.fanout.Unsubscribe(a.subID)
		_ = a.internal.Close()
		_ = a.external.Close()
	})
}
