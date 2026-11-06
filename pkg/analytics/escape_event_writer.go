package analytics

import (
	"context"
	"sync"
	"time"
)

// Stage identifies which pipeline stage observed a sequence.
type Stage string

const (
	StagePTYRead   Stage = "pty_read"
	StageTransport Stage = "transport"
	StageBrowser   Stage = "browser"
)

// EscapeEventRecord holds a single observed escape sequence event.
type EscapeEventRecord struct {
	SessionID         string
	ProjectPath       string
	Stage             Stage
	SequenceType      string // "CSI", "OSC", "DCS", etc.
	SequenceSubtype   string // e.g. "SGR", "cursor-up", "clipboard"
	SequenceSignature string // privacy-safe normalized command, e.g. "CSI:?25h" or "OSC:52"
	ByteLen           int
	PayloadHash       string // SHA-256 hex prefix, empty if redacted
	RawBytes          []byte // nil unless capture_level=full
	Mangled           bool
	MangleType        string // "truncated", "mutated", "stripped"
	WallTime          time.Time
	SessionSeq        int64 // cumulative PTY byte offset at start of chunk
}

// EscapeEventWriter is the interface for persisting escape events.
type EscapeEventWriter interface {
	WriteEscapeEvent(ctx context.Context, event EscapeEventRecord)
}

// EscapeEventWriterStats is implemented by writers that expose capture loss.
type EscapeEventWriterStats interface {
	DroppedCount() int64
}

// NoopEscapeEventWriter discards all events (used when capture_level=off).
type NoopEscapeEventWriter struct{}

func (n NoopEscapeEventWriter) WriteEscapeEvent(_ context.Context, _ EscapeEventRecord) {}

// Global escape event writer singleton.
var (
	globalEscapeWriter   EscapeEventWriter = NoopEscapeEventWriter{}
	globalEscapeWriterMu sync.RWMutex
)

// SetGlobalEscapeWriter replaces the process-wide escape event writer.
// Called once at server startup after the batch writer is created.
func SetGlobalEscapeWriter(w EscapeEventWriter) {
	globalEscapeWriterMu.Lock()
	defer globalEscapeWriterMu.Unlock()
	if w == nil {
		globalEscapeWriter = NoopEscapeEventWriter{}
	} else {
		globalEscapeWriter = w
	}
}

// GetGlobalEscapeWriter returns the process-wide escape event writer.
func GetGlobalEscapeWriter() EscapeEventWriter {
	globalEscapeWriterMu.RLock()
	defer globalEscapeWriterMu.RUnlock()
	return globalEscapeWriter
}

// GetGlobalEscapeWriterDroppedCount reports events lost to writer backpressure.
func GetGlobalEscapeWriterDroppedCount() int64 {
	writer := GetGlobalEscapeWriter()
	stats, ok := writer.(EscapeEventWriterStats)
	if !ok {
		return 0
	}
	return stats.DroppedCount()
}
