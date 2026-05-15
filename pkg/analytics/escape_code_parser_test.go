package analytics

import (
	"context"
	"strings"
	"testing"
)

// spyWriter is a test implementation of EscapeEventWriter that records events.
type spyWriter struct {
	events []EscapeEventRecord
}

func (s *spyWriter) WriteEscapeEvent(_ context.Context, event EscapeEventRecord) {
	s.events = append(s.events, event)
}

func TestParseCSISequences(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	tests := []struct {
		name     string
		input    []byte
		wantCat  EscapeCategory
		wantDesc string
	}{
		{
			name:     "cursor up",
			input:    []byte("\x1b[A"),
			wantCat:  CategoryCursor,
			wantDesc: "Cursor Up",
		},
		{
			name:     "cursor position",
			input:    []byte("\x1b[10;20H"),
			wantCat:  CategoryCursor,
			wantDesc: "Cursor Position (10;20)",
		},
		{
			name:     "SGR reset",
			input:    []byte("\x1b[0m"),
			wantCat:  CategorySGR,
			wantDesc: "Reset Attributes",
		},
		{
			name:     "SGR red foreground",
			input:    []byte("\x1b[31m"),
			wantCat:  CategorySGR,
			wantDesc: "Foreground Red",
		},
		{
			name:     "erase display",
			input:    []byte("\x1b[2J"),
			wantCat:  CategoryErase,
			wantDesc: "Erase All",
		},
		{
			name:     "erase line",
			input:    []byte("\x1b[K"),
			wantCat:  CategoryErase,
			wantDesc: "Erase to End of Line",
		},
		{
			name:     "scroll region",
			input:    []byte("\x1b[1;24r"),
			wantCat:  CategoryScroll,
			wantDesc: "Set Scroll Region (1;24)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Clear()
			parser.Parse(tt.input, 0)

			entries := store.GetAll()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}

			if entries[0].Category != tt.wantCat {
				t.Errorf("category = %v, want %v", entries[0].Category, tt.wantCat)
			}
			if entries[0].HumanReadable != tt.wantDesc {
				t.Errorf("description = %q, want %q", entries[0].HumanReadable, tt.wantDesc)
			}
		})
	}
}

func TestParseDECPrivateModes(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	tests := []struct {
		name     string
		input    []byte
		wantDesc string
	}{
		{
			name:     "enable cursor",
			input:    []byte("\x1b[?25h"),
			wantDesc: "Enable Cursor Visibility (DECTCEM)",
		},
		{
			name:     "disable cursor",
			input:    []byte("\x1b[?25l"),
			wantDesc: "Disable Cursor Visibility (DECTCEM)",
		},
		{
			name:     "enable alternate screen",
			input:    []byte("\x1b[?1049h"),
			wantDesc: "Enable Alternate Screen Buffer with Cursor Save",
		},
		{
			name:     "disable alternate screen",
			input:    []byte("\x1b[?1049l"),
			wantDesc: "Disable Alternate Screen Buffer with Cursor Save",
		},
		{
			name:     "enable bracketed paste",
			input:    []byte("\x1b[?2004h"),
			wantDesc: "Enable Bracketed Paste Mode",
		},
		{
			name:     "enable sync update",
			input:    []byte("\x1b[?2026h"),
			wantDesc: "Enable Synchronous Update Mode",
		},
		{
			name:     "enable mouse tracking",
			input:    []byte("\x1b[?1000h"),
			wantDesc: "Enable X11 Mouse Reporting (Normal)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Clear()
			parser.Parse(tt.input, 0)

			entries := store.GetAll()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}

			if entries[0].Category != CategoryDECPriv {
				t.Errorf("category = %v, want %v", entries[0].Category, CategoryDECPriv)
			}
			if entries[0].HumanReadable != tt.wantDesc {
				t.Errorf("description = %q, want %q", entries[0].HumanReadable, tt.wantDesc)
			}
		})
	}
}

func TestParseOSCSequences(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	tests := []struct {
		name     string
		input    []byte
		wantDesc string
	}{
		{
			name:     "set window title BEL",
			input:    []byte("\x1b]0;Test Title\x07"),
			wantDesc: "OSC: Set Icon Name and Window Title",
		},
		{
			name:     "set window title ST",
			input:    []byte("\x1b]2;Test Title\x1b\\"),
			wantDesc: "OSC: Set Window Title",
		},
		{
			name:     "hyperlink",
			input:    []byte("\x1b]8;;https://example.com\x07"),
			wantDesc: "OSC: Hyperlink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Clear()
			parser.Parse(tt.input, 0)

			entries := store.GetAll()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}

			if entries[0].Category != CategoryOSC {
				t.Errorf("category = %v, want %v", entries[0].Category, CategoryOSC)
			}
			if entries[0].HumanReadable != tt.wantDesc {
				t.Errorf("description = %q, want %q", entries[0].HumanReadable, tt.wantDesc)
			}
		})
	}
}

func TestParseSimpleEscapes(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	tests := []struct {
		name     string
		input    []byte
		wantDesc string
	}{
		{
			name:     "save cursor",
			input:    []byte("\x1b7"),
			wantDesc: "Save Cursor (DECSC)",
		},
		{
			name:     "restore cursor",
			input:    []byte("\x1b8"),
			wantDesc: "Restore Cursor (DECRC)",
		},
		{
			name:     "reverse index",
			input:    []byte("\x1bM"),
			wantDesc: "Reverse Index (RI) - Cursor up, scroll if at top",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Clear()
			parser.Parse(tt.input, 0)

			entries := store.GetAll()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}

			if entries[0].Category != CategorySimple {
				t.Errorf("category = %v, want %v", entries[0].Category, CategorySimple)
			}
			if entries[0].HumanReadable != tt.wantDesc {
				t.Errorf("description = %q, want %q", entries[0].HumanReadable, tt.wantDesc)
			}
		})
	}
}

func TestParseMixedContent(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	// Mix of text and escape sequences
	input := []byte("Hello \x1b[31mRed\x1b[0m World\x1b[A")
	parser.Parse(input, 0)

	entries := store.GetAll()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify categories
	categories := make(map[EscapeCategory]int)
	for _, e := range entries {
		categories[e.Category]++
	}

	if categories[CategorySGR] != 2 {
		t.Errorf("expected 2 SGR entries, got %d", categories[CategorySGR])
	}
	if categories[CategoryCursor] != 1 {
		t.Errorf("expected 1 Cursor entry, got %d", categories[CategoryCursor])
	}
}

func TestParsePartialSequences(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	// Send partial escape sequence
	parser.Parse([]byte("Hello \x1b[31"), 0)

	// Should have no complete entries yet
	entries := store.GetAll()
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries (partial), got %d", len(entries))
	}

	// Complete the sequence
	parser.Parse([]byte("m World"), 0)

	entries = store.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after completion, got %d", len(entries))
	}

	if entries[0].HumanReadable != "Foreground Red" {
		t.Errorf("description = %q, want %q", entries[0].HumanReadable, "Foreground Red")
	}
}

func TestParserDisabled(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	// Parser disabled by default

	parser.Parse([]byte("\x1b[31m"), 0)

	entries := store.GetAll()
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries (parser disabled), got %d", len(entries))
	}
}

func TestStoreDisabled(t *testing.T) {
	store := NewEscapeCodeStore()
	// Store disabled by default
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	parser.Parse([]byte("\x1b[31m"), 0)

	entries := store.GetAll()
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries (store disabled), got %d", len(entries))
	}
}

func TestStoreStats(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	// Parse various sequences
	parser.Parse([]byte("\x1b[31m\x1b[32m\x1b[0m"), 0) // 3 SGR
	parser.Parse([]byte("\x1b[A\x1b[B"), 0)            // 2 Cursor
	parser.Parse([]byte("\x1b[?25h"), 0)               // 1 DECPriv

	stats := store.GetStats()

	if stats.UniqueCodes != 6 {
		t.Errorf("UniqueCodes = %d, want 6", stats.UniqueCodes)
	}
	if stats.TotalCodes != 6 {
		t.Errorf("TotalCodes = %d, want 6", stats.TotalCodes)
	}
	if stats.CategoryCounts[CategorySGR] != 3 {
		t.Errorf("SGR count = %d, want 3", stats.CategoryCounts[CategorySGR])
	}
	if stats.CategoryCounts[CategoryCursor] != 2 {
		t.Errorf("Cursor count = %d, want 2", stats.CategoryCounts[CategoryCursor])
	}
	if stats.CategoryCounts[CategoryDECPriv] != 1 {
		t.Errorf("DECPriv count = %d, want 1", stats.CategoryCounts[CategoryDECPriv])
	}
}

// newParserWithSpy creates a parser+store with a spy writer at the given capture level.
func newParserWithSpy(captureLevel string, redactOSC bool, samplingRate float64) (*EscapeCodeParser, *EscapeCodeStore, *spyWriter) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)
	spy := &spyWriter{}
	parser.SetEventWriter(spy, captureLevel, redactOSC, samplingRate)
	return parser, store, spy
}

func TestEventWriterOSC52Redaction(t *testing.T) {
	parser, _, spy := newParserWithSpy("summary", true, 1.0)
	// OSC 52 clipboard set: ESC ] 52 ; <base64> BEL
	parser.Parse([]byte("\x1b]52;c;aGVsbG8=\x07"), 0)

	if len(spy.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spy.events))
	}
	ev := spy.events[0]
	if ev.PayloadHash != "" {
		t.Errorf("PayloadHash should be empty for OSC 52, got %q", ev.PayloadHash)
	}
	if ev.RawBytes != nil {
		t.Errorf("RawBytes should be nil for OSC 52, got %v", ev.RawBytes)
	}
	if ev.SequenceSubtype != "clipboard" {
		t.Errorf("SequenceSubtype should be 'clipboard', got %q", ev.SequenceSubtype)
	}
}

func TestEventWriterOSC0Redaction(t *testing.T) {
	parser, _, spy := newParserWithSpy("summary", true, 1.0)
	// OSC 0 sets icon name and window title
	parser.Parse([]byte("\x1b]0;My Window\x07"), 0)

	if len(spy.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spy.events))
	}
	ev := spy.events[0]
	if ev.PayloadHash != "" {
		t.Errorf("PayloadHash should be empty for OSC 0 with redaction, got %q", ev.PayloadHash)
	}
	if ev.RawBytes != nil {
		t.Errorf("RawBytes should be nil for OSC 0 with redaction, got %v", ev.RawBytes)
	}
}

func TestEventWriterCaptureLevelFull(t *testing.T) {
	parser, _, spy := newParserWithSpy("full", false, 1.0)
	input := []byte("\x1b[31m")
	parser.Parse(input, 0)

	if len(spy.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spy.events))
	}
	ev := spy.events[0]
	if ev.RawBytes == nil {
		t.Errorf("RawBytes should be set for capture_level=full")
	}
	if ev.PayloadHash == "" {
		t.Errorf("PayloadHash should be set for capture_level=full")
	}
}

func TestEventWriterCaptureLevelSummary(t *testing.T) {
	parser, _, spy := newParserWithSpy("summary", false, 1.0)
	parser.Parse([]byte("\x1b[31m"), 0)

	if len(spy.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spy.events))
	}
	ev := spy.events[0]
	if ev.RawBytes != nil {
		t.Errorf("RawBytes should be nil for capture_level=summary, got %v", ev.RawBytes)
	}
	if ev.PayloadHash == "" {
		t.Errorf("PayloadHash should be set for capture_level=summary")
	}
	if len(ev.PayloadHash) != 16 {
		t.Errorf("PayloadHash should be 16 hex chars, got %q (len=%d)", ev.PayloadHash, len(ev.PayloadHash))
	}
}

func TestEventWriterSamplingRateZero(t *testing.T) {
	parser, _, spy := newParserWithSpy("summary", false, 0.0)
	// Parse many sequences - none should be emitted at 0.0 sampling rate
	for i := 0; i < 100; i++ {
		parser.Parse([]byte("\x1b[31m"), 0)
	}

	if len(spy.events) != 0 {
		t.Errorf("expected 0 events at sampling rate 0.0, got %d", len(spy.events))
	}
}

func TestPartialBufferCap(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "test-session")
	parser.SetEnabled(true)

	// Inject an oversized partial buffer directly
	parser.partialBuffer = make([]byte, 5000)
	parser.partialBuffer[0] = 0x1b // starts with ESC

	// Parse something; findPartialEscapeAtEnd should reset the oversized buffer
	parser.Parse([]byte("hello"), 0)

	// The oversized partial buffer should have been cleared
	if len(parser.partialBuffer) > 4096 {
		t.Errorf("partialBuffer should have been capped, got len=%d", len(parser.partialBuffer))
	}
}

func TestEventWriterSequenceFields(t *testing.T) {
	parser, _, spy := newParserWithSpy("summary", false, 1.0)
	parser.Parse([]byte("\x1b[A"), 0) // Cursor Up

	if len(spy.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spy.events))
	}
	ev := spy.events[0]
	if ev.SequenceType != string(CategoryCursor) {
		t.Errorf("SequenceType = %q, want %q", ev.SequenceType, string(CategoryCursor))
	}
	if !strings.HasPrefix(ev.SequenceSubtype, "Cursor") {
		t.Errorf("SequenceSubtype = %q, want prefix 'Cursor'", ev.SequenceSubtype)
	}
	if ev.Stage != StagePTYRead {
		t.Errorf("Stage = %q, want %q", ev.Stage, StagePTYRead)
	}
	if ev.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want 'test-session'", ev.SessionID)
	}
	if ev.ByteLen != 3 { // \x1b[A = 3 bytes
		t.Errorf("ByteLen = %d, want 3", ev.ByteLen)
	}
}

// BenchmarkEscapeParser4KB measures Stage 1 parse overhead on a realistic 4KB PTY chunk
// (AC-7: must be < 50µs). The chunk is ~10% escape sequences, 90% plain text — representative
// of shell output or editor rendering (not pathological escape-only traffic).
// Run with: go test -bench=BenchmarkEscapeParser4KB -benchtime=5s ./pkg/analytics/
func BenchmarkEscapeParser4KB(b *testing.B) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "bench-session")
	parser.SetEnabled(true)
	parser.SetEventWriter(NoopEscapeEventWriter{}, "summary", true, 1.0)

	// Realistic chunk: ~10% escape traffic (1-2 sequences per 60-byte line)
	// Simulates a colorized ls or shell prompt output
	chunk := make([]byte, 0, 4096)
	line := []byte("\x1b[32m/usr/local/bin/program\x1b[0m  1234 bytes  modified 2026-05-14\n")
	for len(chunk) < 4096 {
		chunk = append(chunk, line...)
	}
	chunk = chunk[:4096]

	b.ResetTimer()
	b.SetBytes(4096)
	for i := 0; i < b.N; i++ {
		parser.Parse(chunk, int64(i)*4096)
	}
}

// BenchmarkEscapeParserNoWriter measures the baseline parse cost with no event writer.
func BenchmarkEscapeParserNoWriter(b *testing.B) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	parser := NewEscapeCodeParser(store, "bench-session")
	parser.SetEnabled(true)

	line := []byte("\x1b[32m/usr/local/bin/program\x1b[0m  1234 bytes  modified 2026-05-14\n")
	chunk := make([]byte, 0, 4096)
	for len(chunk) < 4096 {
		chunk = append(chunk, line...)
	}
	chunk = chunk[:4096]

	b.ResetTimer()
	b.SetBytes(4096)
	for i := 0; i < b.N; i++ {
		parser.Parse(chunk, int64(i)*4096)
	}
}
