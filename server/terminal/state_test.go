package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestStateGenerator_BasicOperations tests core state generation functionality
func TestStateGenerator_BasicOperations(t *testing.T) {
	tests := []struct {
		name     string
		cols     int
		rows     int
		output1  string
		output2  string
		expected string // Expected behavior description
	}{
		{
			name:     "single line creation",
			cols:     80,
			rows:     24,
			output1:  "Hello World",
			output2:  "Hello World\nSecond Line",
			expected: "should generate complete state with all lines",
		},
		{
			name:     "line modification",
			cols:     80,
			rows:     24,
			output1:  "Hello World",
			output2:  "Hello Universe",
			expected: "should generate new complete state with modified content",
		},
		{
			name:     "multi-line content",
			cols:     80,
			rows:     24,
			output1:  "Line 1\nLine 2\nLine 3",
			output2:  "Line 1\nModified Line 2\nLine 3\nLine 4",
			expected: "should handle multi-line states correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg := NewStateGenerator(tt.cols, tt.rows)

			// First state generation
			state1 := sg.GenerateState([]byte(tt.output1))
			if state1 == nil {
				t.Fatal("GenerateState returned nil")
			}

			// Verify state structure
			if state1.Sequence != 1 {
				t.Errorf("Expected sequence=1, got %d", state1.Sequence)
			}

			if state1.Dimensions == nil {
				t.Error("Expected dimensions in state")
			} else {
				if state1.Dimensions.Rows != uint32(tt.rows) {
					t.Errorf("Expected dimensions rows=%d, got %d", tt.rows, state1.Dimensions.Rows)
				}
				if state1.Dimensions.Cols != uint32(tt.cols) {
					t.Errorf("Expected dimensions cols=%d, got %d", tt.cols, state1.Dimensions.Cols)
				}
			}

			// Verify lines are padded to terminal size
			if len(state1.Lines) != tt.rows {
				t.Errorf("Expected %d lines (padded to terminal size), got %d", tt.rows, len(state1.Lines))
			}

			// Verify cursor position exists
			if state1.Cursor == nil {
				t.Fatal("Cursor position missing from state")
			}

			// Verify compression metadata exists
			if state1.Compression == nil {
				t.Error("Expected compression metadata")
			}

			// Second state generation (sequence should increment)
			state2 := sg.GenerateState([]byte(tt.output2))
			if state2 == nil {
				t.Fatal("Second GenerateState returned nil")
			}

			// Verify sequence progression
			if state2.Sequence != 2 {
				t.Errorf("Expected sequence=2, got %d", state2.Sequence)
			}

			// Verify cursor position is valid
			if state2.Cursor.Row >= uint32(tt.rows) {
				t.Errorf("Cursor row %d exceeds terminal rows %d", state2.Cursor.Row, tt.rows)
			}
			if state2.Cursor.Col >= uint32(tt.cols) {
				t.Errorf("Cursor col %d exceeds terminal cols %d", state2.Cursor.Col, tt.cols)
			}

			t.Logf("✅ %s: %s", tt.name, tt.expected)
		})
	}
}

// TestStateGenerator_EdgeCases tests edge cases and error conditions
func TestStateGenerator_EdgeCases(t *testing.T) {
	t.Run("empty output handling", func(t *testing.T) {
		sg := NewStateGenerator(80, 24)

		// Test empty input
		state := sg.GenerateState([]byte(""))
		if state == nil {
			t.Fatal("GenerateState returned nil for empty input")
		}

		// Should have padded lines
		if len(state.Lines) != 24 {
			t.Errorf("Expected 24 padded lines, got %d", len(state.Lines))
		}

		// All lines should be marked as empty
		for i, line := range state.Lines {
			if line.Attributes == nil || !line.Attributes.IsEmpty {
				t.Errorf("Line %d should be marked as empty", i)
			}
		}

		// Cursor should be at origin
		if state.Cursor.Row != 0 || state.Cursor.Col != 0 {
			t.Errorf("Expected cursor at (0,0), got (%d,%d)", state.Cursor.Row, state.Cursor.Col)
		}
	})

	t.Run("large output truncation", func(t *testing.T) {
		sg := NewStateGenerator(80, 10) // Small terminal

		// Generate output larger than terminal
		var largeOutput bytes.Buffer
		for i := 0; i < 20; i++ {
			largeOutput.WriteString("Line ")
			largeOutput.WriteString(string(rune('A' + (i % 26))))
			largeOutput.WriteString(" with content\n")
		}

		state := sg.GenerateState(largeOutput.Bytes())
		if state == nil {
			t.Fatal("GenerateState returned nil for large input")
		}

		// Should have exactly terminal size lines
		if len(state.Lines) != 10 {
			t.Errorf("Expected exactly 10 lines, got %d", len(state.Lines))
		}

		// Cursor should be within bounds
		if state.Cursor.Row >= 10 {
			t.Errorf("Cursor row %d exceeds terminal rows", state.Cursor.Row)
		}
	})

	t.Run("dimension updates", func(t *testing.T) {
		sg := NewStateGenerator(80, 24)

		// Initial state
		sg.GenerateState([]byte("Initial content"))

		// Update dimensions
		sg.UpdateDimensions(100, 30)

		// Generate new state
		state := sg.GenerateState([]byte("New content after resize"))

		// Verify dimensions in state
		if state.Dimensions == nil {
			t.Fatal("Dimensions missing from state after resize")
		}
		if state.Dimensions.Cols != 100 {
			t.Errorf("Expected cols=100, got %d", state.Dimensions.Cols)
		}
		if state.Dimensions.Rows != 30 {
			t.Errorf("Expected rows=30, got %d", state.Dimensions.Rows)
		}

		// Should have correct number of padded lines
		if len(state.Lines) != 30 {
			t.Errorf("Expected 30 lines after resize, got %d", len(state.Lines))
		}
	})
}

// TestStateGenerator_CompressionDictionary tests dictionary learning functionality
func TestStateGenerator_CompressionDictionary(t *testing.T) {
	sg := NewStateGenerator(80, 24)

	// Generate states with repeated patterns
	patterns := []string{
		"Loading...",
		"Progress: [████████████████████████████████████████] 100%",
		"Error: Connection failed",
		"Loading...", // Repeat pattern
	}

	for i, pattern := range patterns {
		state := sg.GenerateState([]byte(pattern))

		if state.Compression == nil {
			t.Fatalf("Step %d: Expected compression metadata", i+1)
		}

		if state.Compression.Dictionary == nil {
			t.Fatalf("Step %d: Expected dictionary metadata", i+1)
		}

		// Dictionary should learn patterns over time
		if i > 0 {
			if state.Compression.Dictionary.PatternCount == 0 {
				t.Errorf("Step %d: Dictionary should have learned patterns", i+1)
			}
		}
	}

	// Check final compression stats
	stats := sg.GetCompressionStats()
	if patterns := stats["patterns"].(int); patterns == 0 {
		t.Error("Dictionary should have learned patterns")
	}

	t.Logf("Final compression stats: %+v", stats)
}

// TestStateGenerator_CursorPositioning tests cursor position calculation
func TestStateGenerator_CursorPositioning(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectedRow uint32
		expectedCol uint32
	}{
		{
			name:        "single line",
			output:      "Hello",
			expectedRow: 0,
			expectedCol: 5,
		},
		{
			name:        "multiple lines",
			output:      "Line 1\nLine 2\nLine 3",
			expectedRow: 2,
			expectedCol: 6,
		},
		{
			name:        "with ANSI codes",
			output:      "Hello \x1b[31mRed\x1b[0m World",
			expectedRow: 0,
			expectedCol: 15, // "Hello Red World" = 15 visible chars
		},
		{
			name:        "empty line",
			output:      "",
			expectedRow: 0,
			expectedCol: 0,
		},
		{
			name:        "trailing empty lines",
			output:      "Content\n\n\n",
			expectedRow: 0, // Last non-empty line
			expectedCol: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg := NewStateGenerator(80, 24)
			state := sg.GenerateState([]byte(tt.output))

			if state.Cursor == nil {
				t.Fatal("Cursor position missing")
			}

			if state.Cursor.Row != tt.expectedRow {
				t.Errorf("Expected cursor row %d, got %d", tt.expectedRow, state.Cursor.Row)
			}
			if state.Cursor.Col != tt.expectedCol {
				t.Errorf("Expected cursor col %d, got %d", tt.expectedCol, state.Cursor.Col)
			}
		})
	}
}

// TestStateGenerator_LineAttributes tests line attribute analysis
func TestStateGenerator_LineAttributes(t *testing.T) {
	sg := NewStateGenerator(80, 24)

	tests := []struct {
		input       string
		expectEmpty bool
		expectASCII bool
	}{
		{
			input:       "",
			expectEmpty: true,
			expectASCII: true,
		},
		{
			input:       "Hello World",
			expectEmpty: false,
			expectASCII: true,
		},
		{
			input:       "Hello \x1b[31mRed\x1b[0m",
			expectEmpty: false,
			expectASCII: false, // Contains ESC sequences
		},
		{
			input:       "Tab\there",
			expectEmpty: false,
			expectASCII: true, // TAB is allowed in ASCII-like
		},
	}

	for i, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			state := sg.GenerateState([]byte(tt.input))

			if len(state.Lines) == 0 {
				t.Fatal("No lines in generated state")
			}

			line := state.Lines[0] // Check first line
			if line.Attributes == nil {
				t.Fatal("Line attributes missing")
			}

			if line.Attributes.IsEmpty != tt.expectEmpty {
				t.Errorf("Test %d: Expected isEmpty=%v, got %v", i, tt.expectEmpty, line.Attributes.IsEmpty)
			}

			if line.Attributes.AsciiOnly != tt.expectASCII {
				t.Errorf("Test %d: Expected asciiOnly=%v, got %v", i, tt.expectASCII, line.Attributes.AsciiOnly)
			}

			// Pattern hash should be present (even for empty content)
			if line.Attributes.PatternHash == nil {
				t.Error("Pattern hash should be present")
			}
		})
	}
}

// TestStateGenerator_ThreadSafety tests concurrent access
func TestStateGenerator_ThreadSafety(t *testing.T) {
	sg := NewStateGenerator(80, 24)

	// Run concurrent operations
	done := make(chan bool, 3)

	// Goroutine 1: Generate states
	go func() {
		for i := 0; i < 50; i++ {
			output := "Content " + string(rune('A'+(i%26)))
			state := sg.GenerateState([]byte(output))
			if state == nil {
				t.Errorf("GenerateState returned nil in goroutine")
			}
		}
		done <- true
	}()

	// Goroutine 2: Update dimensions
	go func() {
		for i := 0; i < 10; i++ {
			sg.UpdateDimensions(80+i, 24+i)
			time.Sleep(1 * time.Millisecond) // Small delay
		}
		done <- true
	}()

	// Goroutine 3: Read compression stats
	go func() {
		for i := 0; i < 20; i++ {
			stats := sg.GetCompressionStats()
			if stats == nil {
				t.Error("GetCompressionStats returned nil")
			}
			time.Sleep(2 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done
	<-done
}

// TestStateGenerator_MonotonicSequencing tests sequence number progression
func TestStateGenerator_MonotonicSequencing(t *testing.T) {
	sg := NewStateGenerator(80, 24)

	var lastSequence uint64 = 0

	for i := 0; i < 100; i++ {
		state := sg.GenerateState([]byte("test content"))

		if state.Sequence <= lastSequence {
			t.Errorf("Sequence not monotonic: %d <= %d", state.Sequence, lastSequence)
		}

		lastSequence = state.Sequence
	}

	// Test reset
	sg.Reset()
	state := sg.GenerateState([]byte("after reset"))
	if state.Sequence != 1 {
		t.Errorf("Expected sequence=1 after reset, got %d", state.Sequence)
	}
}

// TestStateGenerator_StripANSIBytes tests ANSI escape sequence removal in StateGenerator
func TestStateGenerator_StripANSIBytes(t *testing.T) {
	sg := NewStateGenerator(80, 24)

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			input:    "\x1b[31mRed\x1b[0m Text",
			expected: "Red Text",
		},
		{
			input:    "\x1b[2K\x1b[1;1HCleared",
			expected: "Cleared",
		},
		{
			input:    "",
			expected: "",
		},
		{
			input:    "\x1b[38;5;208mOrange\x1b[0m",
			expected: "Orange",
		},
	}

	for i, tt := range tests {
		t.Run("strip_test_"+string(rune('A'+i)), func(t *testing.T) {
			result := sg.stripANSIBytes([]byte(tt.input))
			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

// TestStateGenerator_ScrollbackInfo tests scrollback information generation
func TestStateGenerator_ScrollbackInfo(t *testing.T) {
	sg := NewStateGenerator(80, 5) // Small terminal for testing

	// Generate content with multiple lines
	content := strings.Join([]string{
		"Line 1",
		"Line 2",
		"Line 3",
		"Line 4",
		"Line 5",
	}, "\n")

	state := sg.GenerateState([]byte(content))

	if state.Scrollback == nil {
		t.Fatal("Scrollback info missing")
	}

	scrollback := state.Scrollback
	if scrollback.TotalLines != 5 {
		t.Errorf("Expected totalLines=5, got %d", scrollback.TotalLines)
	}

	if scrollback.FirstVisible != 0 {
		t.Errorf("Expected firstVisible=0, got %d", scrollback.FirstVisible)
	}

	if scrollback.LastVisible != 4 {
		t.Errorf("Expected lastVisible=4, got %d", scrollback.LastVisible)
	}
}

// BenchmarkStateGeneration benchmarks state generation performance
func BenchmarkStateGeneration(b *testing.B) {
	sg := NewStateGenerator(120, 50)

	// Generate test content
	var content bytes.Buffer
	for i := 0; i < 50; i++ {
		content.WriteString("Line ")
		content.WriteString(string(rune('A' + (i % 26))))
		content.WriteString(" with terminal content and ANSI codes \x1b[31mRed\x1b[0m\n")
	}

	testContent := content.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := sg.GenerateState(testContent)
		if state == nil {
			b.Fatal("GenerateState returned nil")
		}
	}
}

// BenchmarkCompressionDictionaryUpdate benchmarks dictionary learning
func BenchmarkCompressionDictionaryUpdate(b *testing.B) {
	sg := NewStateGenerator(120, 50)

	testPatterns := [][]byte{
		[]byte("Loading... Please wait"),
		[]byte("Progress: [████████████████████████████████████████] 100%"),
		[]byte("Error: Connection timeout"),
		[]byte("Success: Operation completed"),
		[]byte("\x1b[31mError:\x1b[0m Something went wrong"),
		[]byte("\x1b[32mSuccess:\x1b[0m All done!"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pattern := testPatterns[i%len(testPatterns)]
		sg.GenerateState(pattern)
	}
}

func TestStateGenerator_SanitizeUTF8Bytes(t *testing.T) {
	sg := NewStateGenerator(80, 24)

	tests := []struct {
		name     string
		input    []byte
		expected string
		desc     string
	}{
		{
			name:     "valid_utf8",
			input:    []byte("Hello, 世界!"),
			expected: "Hello, 世界!",
			desc:     "Valid UTF-8 should pass through unchanged",
		},
		{
			name:     "ansi_color_codes",
			input:    []byte("\x1b[31mRed text\x1b[0m"),
			expected: "\x1b[31mRed text\x1b[0m",
			desc:     "ANSI escape sequences should be preserved",
		},
		{
			name:     "invalid_utf8_bytes",
			input:    []byte{0xff, 0xfe, 0x41, 0x42}, // Invalid UTF-8 + AB
			expected: "��AB",
			desc:     "Invalid UTF-8 bytes should be replaced with replacement characters",
		},
		{
			name:     "control_characters",
			input:    []byte("Hello\x01\x02World"),
			expected: "Hello  World",
			desc:     "Non-terminal control characters should be replaced with spaces",
		},
		{
			name:     "terminal_control_chars",
			input:    []byte("Line1\tTab\nNewline\rReturn"),
			expected: "Line1\tTab\nNewline\rReturn",
			desc:     "Terminal control chars (tab, newline, carriage return) should be preserved",
		},
		{
			name:     "mixed_ansi_and_invalid",
			input:    []byte("\x1b[32mGreen\x1b[0m\xff\xfeInvalid"),
			expected: "\x1b[32mGreen\x1b[0m��Invalid",
			desc:     "ANSI codes should be preserved while invalid bytes are replaced",
		},
		{
			name:     "empty_input",
			input:    []byte{},
			expected: "",
			desc:     "Empty input should remain empty",
		},
		{
			name:     "bell_and_backspace",
			input:    []byte("Text\x07Bell\x08Backspace"),
			expected: "Text\x07Bell\x08Backspace",
			desc:     "Bell and backspace characters should be preserved",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := sg.sanitizeUTF8Bytes(tc.input)
			resultStr := string(result)

			if resultStr != tc.expected {
				t.Errorf("sanitizeUTF8Bytes() failed\nTest: %s\nInput: %v\nExpected: %q\nGot: %q",
					tc.desc, tc.input, tc.expected, resultStr)
			}

			// Verify result is valid UTF-8
			if !utf8.Valid(result) {
				t.Errorf("sanitizeUTF8Bytes() produced invalid UTF-8 for test: %s", tc.name)
			}
		})
	}
}