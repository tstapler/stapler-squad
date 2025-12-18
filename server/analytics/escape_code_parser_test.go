package analytics

import (
	"testing"
)

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
			parser.Parse(tt.input)

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
			parser.Parse(tt.input)

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
			parser.Parse(tt.input)

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
			parser.Parse(tt.input)

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
	parser.Parse(input)

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
	parser.Parse([]byte("Hello \x1b[31"))

	// Should have no complete entries yet
	entries := store.GetAll()
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries (partial), got %d", len(entries))
	}

	// Complete the sequence
	parser.Parse([]byte("m World"))

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

	parser.Parse([]byte("\x1b[31m"))

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

	parser.Parse([]byte("\x1b[31m"))

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
	parser.Parse([]byte("\x1b[31m\x1b[32m\x1b[0m")) // 3 SGR
	parser.Parse([]byte("\x1b[A\x1b[B"))             // 2 Cursor
	parser.Parse([]byte("\x1b[?25h"))                // 1 DECPriv

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
