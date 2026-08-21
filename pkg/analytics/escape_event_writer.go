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
	SessionID       string
	Stage           Stage
	SequenceType    string // "CSI", "OSC", "DCS", etc.
	SequenceSubtype string // e.g. "SGR", "cursor-up", "clipboard"
	ByteLen         int
	PayloadHash     string // SHA-256 hex prefix, empty if redacted
	RawBytes        []byte // nil unless capture_level=full
	Mangled         bool
	MangleType      string // "truncated", "mutated", "stripped"
	WallTime        time.Time
	SessionSeq      int64 // cumulative PTY byte offset at start of chunk
}

// EscapeEventWriter is the interface for persisting escape events.
type EscapeEventWriter interface {
	WriteEscapeEvent(ctx context.Context, event EscapeEventRecord)
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
