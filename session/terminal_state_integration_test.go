package session

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"testing"
)

// TestTerminalStateIntegration verifies the complete delta encoding flow
func TestTerminalStateIntegration(t *testing.T) {
	// Create terminal state
	state := NewTerminalState(25, 80)

	// Test 1: Simple text output
	t.Run("SimpleTextOutput", func(t *testing.T) {
		err := state.ProcessOutput([]byte("Hello, World!"))
		if err != nil {
			t.Fatalf("ProcessOutput failed: %v", err)
		}

		// Generate delta (nil previous state = full sync)
		delta := state.GenerateDelta(nil)
		deltaMsg := delta.GetDelta()

		if deltaMsg == nil {
			t.Fatal("Expected delta message, got nil")
		}

		// Verify full sync flag
		if !deltaMsg.FullSync {
			t.Error("Expected full sync for first delta")
		}

		// Verify version
		if deltaMsg.ToState != 1 {
			t.Errorf("Expected version 1, got %d", deltaMsg.ToState)
		}

		// Verify cursor moved
		if deltaMsg.Cursor.Col != 13 {
			t.Errorf("Expected cursor at col 13, got %d", deltaMsg.Cursor.Col)
		}

		// Verify first line contains "Hello, World!"
		if len(deltaMsg.Lines) == 0 {
			t.Fatal("Expected at least one line delta")
		}

		firstLine := deltaMsg.Lines[0]
		if firstLine.LineNumber != 0 {
			t.Errorf("Expected line 0, got %d", firstLine.LineNumber)
		}

		// Check that we have a replace_line operation
		switch op := firstLine.Operation.(type) {
		case *sessionv1.LineDelta_ReplaceLine:
			if len(op.ReplaceLine) == 0 {
				t.Error("Expected replace_line operation with text")
			}
		default:
			t.Errorf("Expected replace_line operation, got %T", firstLine.Operation)
		}
	})

	// Test 2: Incremental updates
	t.Run("IncrementalUpdate", func(t *testing.T) {
		// Clone state for delta generation
		previousState := state.Clone()

		// Add more text
		err := state.ProcessOutput([]byte("\nSecond line"))
		if err != nil {
			t.Fatalf("ProcessOutput failed: %v", err)
		}

		// Generate incremental delta
		delta := state.GenerateDelta(previousState)
		deltaMsg := delta.GetDelta()

		// Verify NOT full sync
		if deltaMsg.FullSync {
			t.Error("Expected incremental delta, got full sync")
		}

		// Verify version incremented
		if deltaMsg.FromState != 1 {
			t.Errorf("Expected from_state 1, got %d", deltaMsg.FromState)
		}
		if deltaMsg.ToState != 2 {
			t.Errorf("Expected to_state 2, got %d", deltaMsg.ToState)
		}

		// Should have delta for second line
		foundSecondLine := false
		for _, line := range deltaMsg.Lines {
			if line.LineNumber == 1 {
				foundSecondLine = true
				break
			}
		}
		if !foundSecondLine {
			t.Error("Expected delta for second line")
		}
	})

	// Test 3: ANSI escape codes
	t.Run("ANSIEscapeCodes", func(t *testing.T) {
		state := NewTerminalState(25, 80)

		// Bold text
		err := state.ProcessOutput([]byte("\x1b[1mBold\x1b[0m Normal"))
		if err != nil {
			t.Fatalf("ProcessOutput failed: %v", err)
		}

		delta := state.GenerateDelta(nil)
		deltaMsg := delta.GetDelta()

		if deltaMsg == nil {
			t.Fatal("Expected delta message")
		}

		// Verify ANSI codes are preserved in line text
		firstLine := deltaMsg.Lines[0]

		// Check that we have a replace_line operation
		switch op := firstLine.Operation.(type) {
		case *sessionv1.LineDelta_ReplaceLine:
			lineText := op.ReplaceLine
			if len(lineText) == 0 {
				t.Fatal("Expected line text")
			}
			// Line should contain ANSI codes for bold
			if len(lineText) < 10 {
				t.Errorf("Expected ANSI codes in line text, got: %q", lineText)
			}
		default:
			t.Errorf("Expected replace_line operation, got %T", firstLine.Operation)
		}
	})

	// Test 4: Cursor movement
	t.Run("CursorMovement", func(t *testing.T) {
		state := NewTerminalState(25, 80)

		// Move cursor to position 5,10
		err := state.ProcessOutput([]byte("\x1b[5;10H"))
		if err != nil {
			t.Fatalf("ProcessOutput failed: %v", err)
		}

		delta := state.GenerateDelta(nil)
		deltaMsg := delta.GetDelta()

		// Verify cursor position (1-based in ANSI, 0-based in state)
		if deltaMsg.Cursor.Row != 4 {
			t.Errorf("Expected cursor row 4, got %d", deltaMsg.Cursor.Row)
		}
		if deltaMsg.Cursor.Col != 9 {
			t.Errorf("Expected cursor col 9, got %d", deltaMsg.Cursor.Col)
		}
	})

	// Test 5: Screen clear
	t.Run("ScreenClear", func(t *testing.T) {
		state := NewTerminalState(25, 80)

		// Write some text
		state.ProcessOutput([]byte("Line 1\nLine 2\nLine 3"))
		previousState := state.Clone()

		// Clear screen
		err := state.ProcessOutput([]byte("\x1b[2J"))
		if err != nil {
			t.Fatalf("ProcessOutput failed: %v", err)
		}

		delta := state.GenerateDelta(previousState)
		deltaMsg := delta.GetDelta()

		// Should have deltas for cleared lines
		if len(deltaMsg.Lines) == 0 {
			t.Error("Expected line deltas for screen clear")
		}
	})

	// Test 6: Terminal resize
	t.Run("TerminalResize", func(t *testing.T) {
		state := NewTerminalState(25, 80)
		previousState := state.Clone()

		// Resize terminal
		state.Resize(30, 100)

		delta := state.GenerateDelta(previousState)
		deltaMsg := delta.GetDelta()

		// Verify full sync due to dimension change
		if !deltaMsg.FullSync {
			t.Error("Expected full sync after resize")
		}

		// Verify dimensions included
		if deltaMsg.Dimensions == nil {
			t.Fatal("Expected dimensions in delta")
		}

		if deltaMsg.Dimensions.Rows != 30 {
			t.Errorf("Expected 30 rows, got %d", deltaMsg.Dimensions.Rows)
		}
		if deltaMsg.Dimensions.Cols != 100 {
			t.Errorf("Expected 100 cols, got %d", deltaMsg.Dimensions.Cols)
		}
	})

	// Test 7: Complex terminal output (like a build process)
	t.Run("ComplexOutput", func(t *testing.T) {
		state := NewTerminalState(25, 80)

		// Simulate build output
		output := "Compiling...\n" +
			"\x1b[32m✓\x1b[0m Build successful\n" +
			"Time: 2.5s\n" +
			"\x1b[33mWarning:\x1b[0m Unused imports\n"

		err := state.ProcessOutput([]byte(output))
		if err != nil {
			t.Fatalf("ProcessOutput failed: %v", err)
		}

		delta := state.GenerateDelta(nil)
		deltaMsg := delta.GetDelta()

		// Should have multiple line deltas
		if len(deltaMsg.Lines) < 4 {
			t.Errorf("Expected at least 4 lines, got %d", len(deltaMsg.Lines))
		}

		// Verify cursor moved to last line
		if deltaMsg.Cursor.Row < 3 {
			t.Errorf("Expected cursor at row 3+, got %d", deltaMsg.Cursor.Row)
		}
	})

	// Test 8: Verify bandwidth savings
	t.Run("BandwidthSavings", func(t *testing.T) {
		state := NewTerminalState(25, 80)

		// Fill screen with text
		for i := 0; i < 20; i++ {
			state.ProcessOutput([]byte("Line of text that fills most of the terminal width\n"))
		}

		previousState := state.Clone()

		// Update single line (typical interactive scenario)
		state.ProcessOutput([]byte("\x1b[10;1H\x1b[2KUpdated line 10"))

		delta := state.GenerateDelta(previousState)
		deltaMsg := delta.GetDelta()

		// Should only have delta for one line (not all 20 lines)
		if len(deltaMsg.Lines) > 2 {
			t.Errorf("Expected 1-2 line deltas, got %d (bandwidth not optimized)", len(deltaMsg.Lines))
		}

		// Verify incremental update
		if deltaMsg.FullSync {
			t.Error("Expected incremental delta for single line update")
		}

		t.Logf("Bandwidth test: %d lines changed out of 20 (90%% savings)", len(deltaMsg.Lines))
	})
}

// TestDeltaCompressionEfficiency measures actual bandwidth savings
func TestDeltaCompressionEfficiency(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Simulate realistic terminal session
	testCases := []struct {
		name     string
		input    string
		expected float64 // Expected bandwidth reduction %
	}{
		{
			name:     "Command prompt",
			input:    "user@host:~$ ",
			expected: 95.0, // Only 1 line out of 25
		},
		{
			name:     "Build progress",
			input:    "\x1b[5;1H[####------] 40%",
			expected: 90.0, // Only progress line updated
		},
		{
			name:     "Vim status line",
			input:    "\x1b[24;1H-- INSERT -- line 42, col 5",
			expected: 90.0, // Only status line
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			previousState := state.Clone()

			// Process output
			err := state.ProcessOutput([]byte(tc.input))
			if err != nil {
				t.Fatalf("ProcessOutput failed: %v", err)
			}

			// Generate delta
			delta := state.GenerateDelta(previousState)
			deltaMsg := delta.GetDelta()

			// Calculate bandwidth reduction
			totalLines := float64(state.Rows)
			changedLines := float64(len(deltaMsg.Lines))
			reduction := ((totalLines - changedLines) / totalLines) * 100

			t.Logf("Bandwidth reduction: %.1f%% (changed %d/%d lines)",
				reduction, len(deltaMsg.Lines), state.Rows)

			if reduction < tc.expected-5 {
				t.Errorf("Expected at least %.1f%% reduction, got %.1f%%",
					tc.expected, reduction)
			}
		})
	}
}
