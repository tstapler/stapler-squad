package protocol

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	kgzip "github.com/klauspost/compress/gzip"
)

func TestCompressEnvelopeIfLarge_should_ReturnPayloadUnmodified_When_AtOrUnderThreshold(t *testing.T) {
	payload := []byte("small payload")

	tests := []struct {
		name      string
		threshold int
	}{
		{"under threshold", len(payload) + 1},
		{"exactly at threshold", len(payload)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, wasCompressed, err := CompressEnvelopeIfLarge(payload, tt.threshold)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wasCompressed {
				t.Fatal("expected wasCompressed=false, got true")
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("expected payload to be returned unmodified: got %v, want %v", got, payload)
			}
		})
	}
}

func TestCompressEnvelopeIfLarge_should_GzipCompressPayload_When_ExceedsThreshold(t *testing.T) {
	payload := []byte(strings.Repeat("terminal resync payload data ", 100))

	got, wasCompressed, err := CompressEnvelopeIfLarge(payload, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wasCompressed {
		t.Fatal("expected wasCompressed=true, got false")
	}
	if len(got) >= len(payload) {
		t.Fatalf("expected compressed payload to be smaller: got %d bytes, original %d bytes", len(got), len(payload))
	}

	// Verify the result is a well-formed gzip stream.
	r, err := kgzip.NewReader(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("compressed output is not valid gzip: %v", err)
	}
	defer r.Close()
	decoded, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read gzip stream: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatal("decoded gzip stream does not match original payload")
	}
}

// TestCompressEnvelopeIfLarge_should_RoundTripExactly_When_DecompressedByStandardGzipReader
// covers AC6a's integration-level round trip (validation.md maps this to
// handleCurrentPaneRequest_should_RoundTripCompressedTerminalOutput_When_...), simulated
// entirely in Go: connectrpc_websocket.go's shared envelope-writing helper (Task 3.2.1.2)
// doesn't exist yet (Epic 3.2 hasn't landed), so the real call site can't be exercised. This
// test instead compresses via CompressEnvelopeIfLarge (the server side) and decompresses via
// the standard library's gzip reader (standing in for the client's DecompressionStream, which
// consumes the same standard gzip format — see decompressGzipPayload in
// web-app/src/lib/transport/websocket-transport.ts), asserting the round-tripped bytes match
// the pre-compression payload exactly.
func TestCompressEnvelopeIfLarge_should_RoundTripExactly_When_DecompressedByStandardGzipReader(t *testing.T) {
	payloads := [][]byte{
		[]byte(strings.Repeat("A", 5000)),
		[]byte(strings.Repeat("terminal output with \x1b[31mANSI\x1b[0m escapes\n", 200)),
		bytes.Repeat([]byte{0x00, 0xFF, 0x7F, 0x80}, 2000),
	}

	for _, payload := range payloads {
		compressed, wasCompressed, err := CompressEnvelopeIfLarge(payload, 100)
		if err != nil {
			t.Fatalf("compress failed: %v", err)
		}
		if !wasCompressed {
			t.Fatal("expected payload above threshold to be compressed")
		}

		r, err := kgzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatalf("failed to open gzip reader: %v", err)
		}
		roundTripped, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatalf("failed to decompress: %v", err)
		}

		if !bytes.Equal(roundTripped, payload) {
			t.Fatalf("round-tripped payload does not match original: got %d bytes, want %d bytes", len(roundTripped), len(payload))
		}
	}
}

func TestCompressEnvelopeIfLarge_should_BeSafeForConcurrentUse_When_PoolIsSharedAcrossGoroutines(t *testing.T) {
	payload := []byte(strings.Repeat("concurrent compression payload ", 200))

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			compressed, wasCompressed, err := CompressEnvelopeIfLarge(payload, 10)
			if err != nil {
				errs <- err
				return
			}
			if !wasCompressed {
				errs <- io.ErrUnexpectedEOF
				return
			}
			r, err := kgzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				errs <- err
				return
			}
			defer r.Close()
			decoded, err := io.ReadAll(r)
			if err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(decoded, payload) {
				errs <- io.ErrShortBuffer
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent compression failure: %v", err)
	}
}

// BenchmarkCompressEnvelopeIfLarge_ConcurrentBursts measures CPU/latency overhead of gzip
// compression under concurrent load (Task 5.1.1.3). Run backgrounded per
// .claude/docs/benchmarks.md convention:
//
//	go test -bench=BenchmarkCompressEnvelopeIfLarge_ConcurrentBursts -benchmem ./server/protocol -timeout=5m &
func BenchmarkCompressEnvelopeIfLarge_ConcurrentBursts(b *testing.B) {
	payload := bytes.Repeat([]byte("terminal resync payload burst data "), 500) // ~18KB

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := CompressEnvelopeIfLarge(payload, 1024)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCompressEnvelopeIfLarge_Sequential provides a single-goroutine baseline to compare
// against the concurrent-burst benchmark above.
//
//	go test -bench=BenchmarkCompressEnvelopeIfLarge_Sequential -benchmem ./server/protocol -timeout=5m &
func BenchmarkCompressEnvelopeIfLarge_Sequential(b *testing.B) {
	payload := bytes.Repeat([]byte("terminal resync payload burst data "), 500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := CompressEnvelopeIfLarge(payload, 1024); err != nil {
			b.Fatal(err)
		}
	}
}
