package terminal

import (
	"bytes"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestDeltaGenerator_BasicOperations tests core delta generation functionality
func TestDeltaGenerator_BasicOperations(t *testing.T) {
	tests := []struct {
		name     string
		cols     int
		rows     int
		output1  string
		output2  string
		expected string // Expected behavior description
	}{
		{
			name:     "single line addition",
			cols:     80,
			rows:     24,
			output1:  "Hello World",
			output2:  "Hello World\nSecond Line",
			expected: "should generate replace_line delta for new line",
		},
		{
			name:     "line modification",
			cols:     80,
			rows:     24,
			output1:  "Hello World",
			output2:  "Hello Universe",
			expected: "should generate replace_line delta for changed line",
		},
		{
			name:     "cursor positioning",
			cols:     80,
			rows:     24,
			output1:  "Line 1\nLine 2\nLine 3",
			output2:  "Line 1\nModified Line 2\nLine 3",
			expected: "should position cursor correctly after line changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dg := NewDeltaGenerator(tt.cols, tt.rows)

			// First delta (initial state)
			delta1 := dg.GenerateDelta([]byte(tt.output1))
			if delta1 == nil {
				t.Fatal("GenerateDelta returned nil")
			}

			// Verify delta type is correct
			var _ *sessionv1.TerminalDelta = delta1

			// Verify initial state
			if delta1.FromState != 0 {
				t.Errorf("Expected FromState=0, got %d", delta1.FromState)
			}
			if delta1.ToState != 1 {
				t.Errorf("Expected ToState=1, got %d", delta1.ToState)
			}

			// Verify delta has expected structure
			if len(delta1.Lines) == 0 && tt.output1 != "" {
				t.Errorf("Expected lines for non-empty output, got %d lines", len(delta1.Lines))
			}
			if delta1.Dimensions == nil {
				t.Error("Expected dimensions in delta")
			} else {
				if delta1.Dimensions.Rows != uint32(tt.rows) {
					t.Errorf("Expected dimensions rows=%d, got %d", tt.rows, delta1.Dimensions.Rows)
				}
				if delta1.Dimensions.Cols != uint32(tt.cols) {
					t.Errorf("Expected dimensions cols=%d, got %d", tt.cols, delta1.Dimensions.Cols)
				}
			}

			// Second delta (state change)
			delta2 := dg.GenerateDelta([]byte(tt.output2))
			if delta2 == nil {
				t.Fatal("Second GenerateDelta returned nil")
			}

			// Verify version progression
			if delta2.FromState != 1 {
				t.Errorf("Expected FromState=1, got %d", delta2.FromState)
			}
			if delta2.ToState != 2 {
				t.Errorf("Expected ToState=2, got %d", delta2.ToState)
			}

			// Verify cursor position is valid
			if delta2.Cursor == nil {
				t.Fatal("Cursor position missing from delta")
			}
			if delta2.Cursor.Row >= uint32(tt.rows) {
				t.Errorf("Cursor row %d exceeds terminal rows %d", delta2.Cursor.Row, tt.rows)
			}
			if delta2.Cursor.Col >= uint32(tt.cols) {
				t.Errorf("Cursor col %d exceeds terminal cols %d", delta2.Cursor.Col, tt.cols)
			}

			t.Logf("✅ %s: %s", tt.name, tt.expected)
		})
	}
}

// TestDeltaGenerator_EdgeCases tests edge cases and error conditions
func TestDeltaGenerator_EdgeCases(t *testing.T) {
	t.Run("empty output handling", func(t *testing.T) {
		dg := NewDeltaGenerator(80, 24)

		// Test empty input
		delta := dg.GenerateDelta([]byte(""))
		if delta == nil {
			t.Fatal("GenerateDelta returned nil for empty input")
		}

		// Should have valid cursor position even with no content
		if delta.Cursor == nil {
			t.Fatal("Cursor missing for empty input")
		}
		if delta.Cursor.Row != 0 {
			t.Errorf("Expected cursor row 0 for empty input, got %d", delta.Cursor.Row)
		}
	})

	t.Run("large output truncation", func(t *testing.T) {
		dg := NewDeltaGenerator(80, 10) // Small terminal

		// Generate output larger than terminal
		var largeOutput bytes.Buffer
		for i := 0; i < 20; i++ {
			largeOutput.WriteString("Line ")
			largeOutput.WriteString(string(rune('0' + i)))
			largeOutput.WriteString("\n")
		}

		delta := dg.GenerateDelta(largeOutput.Bytes())
		if delta == nil {
			t.Fatal("GenerateDelta returned nil for large input")
		}

		// Should truncate to terminal size
		if len(delta.Lines) > 10 {
			t.Errorf("Expected max 10 lines, got %d", len(delta.Lines))
		}

		// Cursor should be within bounds
		if delta.Cursor.Row >= 10 {
			t.Errorf("Cursor row %d exceeds terminal rows", delta.Cursor.Row)
		}
	})

	t.Run("dimension updates", func(t *testing.T) {
		dg := NewDeltaGenerator(80, 24)

		// Initial content
		dg.GenerateDelta([]byte("Initial content"))

		// Update dimensions
		dg.UpdateDimensions(100, 30)

		// Generate new delta
		delta := dg.GenerateDelta([]byte("New content after resize"))

		// Verify dimensions in delta
		if delta.Dimensions == nil {
			t.Fatal("Dimensions missing from delta after resize")
		}
		if delta.Dimensions.Cols != 100 {
			t.Errorf("Expected cols=100, got %d", delta.Dimensions.Cols)
		}
		if delta.Dimensions.Rows != 30 {
			t.Errorf("Expected rows=30, got %d", delta.Dimensions.Rows)
		}
	})
}

// TestDeltaGenerator_PeriodicFullSync tests MOSH-inspired periodic full sync
func TestDeltaGenerator_PeriodicFullSync(t *testing.T) {
	dg := NewDeltaGenerator(80, 24)

	// Generate many deltas to trigger periodic sync
	for i := 0; i < 60; i++ {
		output := "Content change " + string(rune('0' + (i % 10)))
		delta := dg.GenerateDelta([]byte(output))

		if i < 49 {
			// Should be incremental deltas
			if delta.FullSync {
				t.Errorf("Unexpected full sync at delta %d", i)
			}
		} else if i == 49 {
			// Should trigger full sync at 50th delta
			if !delta.FullSync {
				t.Errorf("Expected full sync at delta 50, got incremental")
			}
			if delta.FromState != 0 {
				t.Errorf("Full sync should have FromState=0, got %d", delta.FromState)
			}
		}
	}
}

// TestDeltaGenerator_CursorPositioning tests cursor position calculation
func TestDeltaGenerator_CursorPositioning(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dg := NewDeltaGenerator(80, 24)
			delta := dg.GenerateDelta([]byte(tt.output))

			if delta.Cursor == nil {
				t.Fatal("Cursor position missing")
			}

			if delta.Cursor.Row != tt.expectedRow {
				t.Errorf("Expected cursor row %d, got %d", tt.expectedRow, delta.Cursor.Row)
			}
			if delta.Cursor.Col != tt.expectedCol {
				t.Errorf("Expected cursor col %d, got %d", tt.expectedCol, delta.Cursor.Col)
			}
		})
	}
}

// TestDeltaGenerator_ThreadSafety tests concurrent access
func TestDeltaGenerator_ThreadSafety(t *testing.T) {
	dg := NewDeltaGenerator(80, 24)

	// Run concurrent operations
	done := make(chan bool, 2)

	// Goroutine 1: Generate deltas
	go func() {
		for i := 0; i < 100; i++ {
			output := "Content " + string(rune('0' + (i % 10)))
			delta := dg.GenerateDelta([]byte(output))
			if delta == nil {
				t.Errorf("GenerateDelta returned nil in goroutine")
			}
		}
		done <- true
	}()

	// Goroutine 2: Update dimensions
	go func() {
		for i := 0; i < 10; i++ {
			dg.UpdateDimensions(80+i, 24+i)
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done
}

// TestStripANSIBytes tests ANSI escape sequence removal
func TestStripANSIBytes(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run("strip_"+tt.input, func(t *testing.T) {
			result := stripANSIBytes([]byte(tt.input))
			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

// BenchmarkDeltaGeneration benchmarks delta generation performance
func BenchmarkDeltaGeneration(b *testing.B) {
	dg := NewDeltaGenerator(120, 50)

	// Generate test content
	var content bytes.Buffer
	for i := 0; i < 50; i++ {
		content.WriteString("Line ")
		content.WriteString(string(rune('A' + (i % 26))))
		content.WriteString(" with some content that changes\n")
	}

	testContent := content.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		delta := dg.GenerateDelta(testContent)
		if delta == nil {
			b.Fatal("GenerateDelta returned nil")
		}
	}
}