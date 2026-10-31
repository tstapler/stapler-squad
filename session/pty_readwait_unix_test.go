//go:build unix

package session

import (
	"testing"
	"time"
)

func TestWaitReadable(t *testing.T) {
	t.Parallel()
	master, slave := openSilentPTY(t)

	if ready, err := waitReadable(master, time.Millisecond); err != nil || ready {
		t.Fatalf("silent PTY: ready=%v err=%v, want false,nil", ready, err)
	}
	// poll(2) is only used for fds beyond FD_SETSIZE, so check its idle result here.
	if rc, err := master.SyscallConn(); err == nil {
		_ = rc.Control(func(fd uintptr) {
			if ready, err := pollReadable(int(fd), time.Millisecond); err != nil || ready {
				t.Errorf("poll on silent PTY: ready=%v err=%v, want false,nil", ready, err)
			}
		})
	}
	if _, err := slave.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	// Both wait primitives (select for low fds, poll for fds beyond FD_SETSIZE)
	// must observe the pending byte; the 5s bound only fails a hang.
	rc, err := master.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Control(func(fd uintptr) {
		for name, wait := range map[string]func(int, time.Duration) (bool, error){"select": selectReadable, "poll": pollReadable} {
			if ready, err := wait(int(fd), 5*time.Second); err != nil || !ready {
				t.Errorf("%s: ready=%v err=%v, want true,nil", name, ready, err)
			}
		}
	})
	if ready, err := waitReadable(master, 5*time.Second); err != nil || !ready {
		t.Fatalf("pending byte: ready=%v err=%v", ready, err)
	}
}

func TestWaitReadable_ClosedFile(t *testing.T) {
	t.Parallel()
	master, _ := openSilentPTY(t)
	_ = master.Close()
	// The caller falls through to Read on any error, which reports the closed
	// file through the stream's "file already closed" EOF mapping.
	if _, err := waitReadable(master, time.Millisecond); err == nil {
		t.Fatal("waitReadable on a closed file returned no error")
	}
}

func TestWaitReadable_HangupIsReadable(t *testing.T) {
	t.Parallel()
	master, slave := openSilentPTY(t)
	_ = slave.Close()
	if ready, err := waitReadable(master, 5*time.Second); err != nil || !ready {
		t.Fatalf("hung-up PTY: ready=%v err=%v, want true,nil so Read can report EOF", ready, err)
	}
}
