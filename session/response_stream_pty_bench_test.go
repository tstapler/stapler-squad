package session

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// BenchmarkResponseStream_FloodedPTY streams a child's flood of output through
// the real read loop on a real PTY master (the deadline-unsupported file type
// on darwin) until EOF; MB/s and allocs/op show the cost of the read path.
func BenchmarkResponseStream_FloodedPTY(b *testing.B) {
	const floodBytes = 64 << 20
	b.SetBytes(floodBytes)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmd := safeexec.CommandContext(context.Background(), "head", "-c", strconv.Itoa(floodBytes), "/dev/zero")
		master, err := pty.Start(cmd)
		if err != nil {
			b.Skipf("cannot start a PTY child: %v", err)
		}
		rs := NewResponseStream("bench-flood", NewPTYAccess("bench-flood", master, NewCircularBuffer(64*1024)))
		eof := make(chan struct{})
		rs.OnEOF = func() { close(eof) }
		if err := rs.Start(context.Background()); err != nil {
			b.Fatalf("Start: %v", err)
		}
		select {
		case <-eof:
		case <-time.After(60 * time.Second):
			b.Fatal("flood did not reach EOF")
		}
		_ = cmd.Wait()
		_ = master.Close()
	}
}
