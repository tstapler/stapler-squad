package scrollback

import (
	"fmt"
	"runtime"
	"testing"
)

// BenchmarkCircularBuffer_ConcurrentReadWrite measures throughput under concurrent
// read and write load — GOMAXPROCS readers and 1 writer simultaneously.
// This simulates the production pattern: one terminal poller writing, multiple
// WebSocket clients reading the scrollback.
func BenchmarkCircularBuffer_ConcurrentReadWrite(b *testing.B) {
	const bufSize = 10000
	data := []byte("benchmark terminal output line with some content here")

	buffer := NewCircularBuffer(bufSize)

	// Pre-fill the buffer to simulate a non-empty state
	for i := 0; i < bufSize/2; i++ {
		buffer.Append([]byte(fmt.Sprintf("prefill line %04d", i)))
	}

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Mix of reads and writes to simulate concurrent access.
			// The Go testing framework distributes iterations across goroutines,
			// so this creates a realistic GOMAXPROCS-way concurrent load.
			buffer.Append(data)
			_ = buffer.GetLastN(100)
		}
	})
}

// BenchmarkCircularBuffer_BurstAppend measures throughput for rapid sequential writes
// without interleaved reads — e.g., ingesting a burst of terminal output.
// Complements BenchmarkCircularBufferAppend by testing the full-buffer eviction path.
func BenchmarkCircularBuffer_BurstAppend(b *testing.B) {
	const bufSize = 1000
	const burstSize = 1000

	// Pre-fill to trigger eviction on every append
	buffer := NewCircularBuffer(bufSize)
	for i := 0; i < bufSize; i++ {
		buffer.Append([]byte(fmt.Sprintf("fill line %04d", i)))
	}

	data := []byte("burst append benchmark data with typical terminal output content")

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Burst of burstSize appends — all trigger eviction since buffer is full
		for j := 0; j < burstSize; j++ {
			buffer.Append(data)
		}
	}

	b.SetBytes(int64(burstSize * len(data)))
}

// BenchmarkCircularBuffer_GetLastN_LargeBuffer measures retrieval latency from a
// large buffer — simulating a client that reconnects and fetches recent history.
// Complements BenchmarkCircularBufferGetLastN (which uses n=100 on a full buffer).
func BenchmarkCircularBuffer_GetLastN_LargeBuffer(b *testing.B) {
	const bufSize = 100000
	const getN = 1000

	buffer := NewCircularBuffer(bufSize)
	for i := 0; i < bufSize; i++ {
		buffer.Append([]byte(fmt.Sprintf("history line %06d with some terminal content", i)))
	}

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entries := buffer.GetLastN(getN)
		if len(entries) != getN {
			b.Fatalf("expected %d entries, got %d", getN, len(entries))
		}
	}
}

// BenchmarkCircularBuffer_GetRange_Sequential measures GetRange performance when
// scanning forward through the buffer — the pattern used by streaming subscribers
// catching up after a reconnect.
func BenchmarkCircularBuffer_GetRange_Sequential(b *testing.B) {
	const bufSize = 10000
	const pageSize = 500

	buffer := NewCircularBuffer(bufSize)
	for i := 0; i < bufSize; i++ {
		buffer.Append([]byte(fmt.Sprintf("streaming line %05d", i)))
	}

	// Start from the oldest sequence
	startSeq := buffer.GetOldestSequence()

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Get a page of entries starting from a known sequence
		entries := buffer.GetRange(startSeq, pageSize)
		if len(entries) == 0 {
			b.Fatal("GetRange returned empty slice")
		}
	}
}
