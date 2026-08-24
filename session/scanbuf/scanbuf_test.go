package scanbuf

import (
	"bufio"
	"bytes"
	"testing"
)

func TestGetPut_ReturnsBufferAtInitialSize_NotMaxTokenSize(t *testing.T) {
	t.Parallel()

	bufPtr := Get()
	defer Put(bufPtr)

	if len(*bufPtr) != initialBufSize {
		t.Errorf("len(*bufPtr) = %d, want %d (initialBufSize)", len(*bufPtr), initialBufSize)
	}
	if len(*bufPtr) == MaxTokenSize {
		t.Error("pooled buffer is sized at MaxTokenSize — defeats the point of lazy growth")
	}
}

func TestScan_GrowsPastInitialBuffer_ForOversizedLine(t *testing.T) {
	t.Parallel()

	line := bytes.Repeat([]byte("a"), initialBufSize*2)
	data := append(append([]byte{}, line...), '\n')

	bufPtr := Get()
	defer Put(bufPtr)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(*bufPtr, MaxTokenSize)

	if !scanner.Scan() {
		t.Fatalf("Scan() = false, want true; err = %v", scanner.Err())
	}
	if got := len(scanner.Bytes()); got != len(line) {
		t.Errorf("scanned line length = %d, want %d", got, len(line))
	}
	if len(*bufPtr) != initialBufSize {
		t.Errorf("pooled buffer grew in place: len(*bufPtr) = %d, want %d", len(*bufPtr), initialBufSize)
	}
}

// BenchmarkScanBuf_TypicalLine guards against the pool regressing back to
// allocating a full MaxTokenSize buffer per Get() — see PerfFix-1: pprof
// showed 120MB/39.44% of resident heap pinned by worst-case-sized pooled
// buffers under concurrent scans of small JSONL lines.
func BenchmarkScanBuf_TypicalLine(b *testing.B) {
	line := bytes.Repeat([]byte("x"), 200)
	data := append(append([]byte{}, line...), '\n')

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bufPtr := Get()
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(*bufPtr, MaxTokenSize)
		for scanner.Scan() {
			_ = scanner.Bytes()
		}
		if len(*bufPtr) != initialBufSize {
			b.Fatalf("buffer grew for a typical-sized line: len = %d", len(*bufPtr))
		}
		Put(bufPtr)
	}
}
