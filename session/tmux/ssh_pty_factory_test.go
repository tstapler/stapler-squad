package tmux

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestSSHPtyFactory_StartPty_ReadWriteResizeClose is Task 4.4.1b's unit test:
// RequestPty + Start against a real in-process test sshd, exercising every
// PtySession method (Write, Read, Resize, Close) the raw-PTY-attach path
// (session.Instance.GetPTYSession, Task 4.4.1d) depends on.
func TestSSHPtyFactory_StartPty_ReadWriteResizeClose(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "pty-factory", srv.Addr, cfg)
	factory := NewSSHPtyFactory(runner)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// testSSHHandler's "cat" verb echoes stdin back to stdout -- see
	// ssh_test_server_test.go -- so a write must produce a matching read.
	sess, err := factory.StartPty(ctx, &pty.Winsize{Cols: 80, Rows: 24}, "", "cat")
	if err != nil {
		t.Fatalf("StartPty() error = %v", err)
	}
	defer func() {
		if err := sess.Close(); err != nil {
			t.Logf("Close() error = %v", err)
		}
	}()

	if err := sess.Resize(120, 40); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	const payload = "hello over ssh pty\n"
	if _, err := sess.Write([]byte(payload)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// A real PTY's line discipline (which RequestPty causes the test server
	// to allocate) is free to translate the trailing '\n' to '\r' or '\r\n'
	// on echo -- exactly the genuine PTY semantics this factory exists to
	// provide (as opposed to a plain non-PTY exec channel, which would pass
	// bytes through unmodified). Only the payload's non-newline content is
	// asserted verbatim; the line terminator is checked loosely. A single
	// Read (rather than io.ReadFull to an exact byte count) is used since
	// the exact echoed length depends on that translation.
	buf := make([]byte, 64)
	type readResult struct {
		n   int
		err error
	}
	readDone := make(chan readResult, 1)
	go func() {
		n, err := sess.Read(buf)
		readDone <- readResult{n: n, err: err}
	}()

	var got string
	select {
	case res := <-readDone:
		if res.err != nil && res.err.Error() != "EOF" {
			t.Fatalf("Read() error = %v", res.err)
		}
		got = strings.TrimRight(string(buf[:res.n]), "\r\n")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the echoed payload to be read back")
	}

	want := strings.TrimRight(payload, "\n")
	if got != want {
		t.Fatalf("echoed payload = %q, want %q", got, want)
	}
}

// TestSSHPtyFactory_StartPty_RemotePtyFactoryInterface locks in that
// SSHPtyFactory satisfies RemotePtyFactory (session/tmux/pty.go) -- the
// interface session.Instance.GetPTYSession (Task 4.4.1d) is written against.
func TestSSHPtyFactory_StartPty_RemotePtyFactoryInterface(t *testing.T) {
	var _ RemotePtyFactory = (*SSHPtyFactory)(nil)
}
