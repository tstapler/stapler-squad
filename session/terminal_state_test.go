package session

import (
	"strings"
	"testing"
)

func TestNewTerminalState(t *testing.T) {
	state := NewTerminalState(25, 80)

	if state.Rows != 25 {
		t.Errorf("Expected 25 rows, got %d", state.Rows)
	}
	if state.Cols != 80 {
		t.Errorf("Expected 80 cols, got %d", state.Cols)
	}
	if state.CursorRow != 0 || state.CursorCol != 0 {
		t.Errorf("Expected cursor at (0,0), got (%d,%d)", state.CursorRow, state.CursorCol)
	}
	if !state.CursorVisible {
		t.Error("Expected cursor to be visible")
	}
	if state.Version != 0 {
		t.Errorf("Expected version 0, got %d", state.Version)
	}

	// Check grid is initialized with spaces
	for i := 0; i < state.Rows; i++ {
		for j := 0; j < state.Cols; j++ {
			if state.Grid[i][j].Char != ' ' {
				t.Errorf("Expected space at (%d,%d), got %c", i, j, state.Grid[i][j].Char)
			}
		}
	}
}

func TestProcessOutput_SimpleText(t *testing.T) {
	state := NewTerminalState(25, 80)
	err := state.ProcessOutput([]byte("Hello World"))

	if err != nil {
		t.Fatalf("ProcessOutput failed: %v", err)
	}

	// Check text was written
	expected := "Hello World"
	for i, char := range expected {
		if state.Grid[0][i].Char != char {
			t.Errorf("At position %d: expected %c, got %c", i, char, state.Grid[0][i].Char)
		}
	}

	// Check cursor moved
	if state.CursorCol != len(expected) {
		t.Errorf("Expected cursor at col %d, got %d", len(expected), state.CursorCol)
	}

	// Check version incremented
	if state.Version != 1 {
		t.Errorf("Expected version 1, got %d", state.Version)
	}
}

func TestProcessOutput_Newline(t *testing.T) {
	state := NewTerminalState(25, 80)
	state.ProcessOutput([]byte("Line 1\nLine 2"))

	// Check first line
	if state.Grid[0][0].Char != 'L' {
		t.Errorf("Expected 'L' at (0,0), got %c", state.Grid[0][0].Char)
	}

	// Check second line
	if state.Grid[1][0].Char != 'L' {
		t.Errorf("Expected 'L' at (1,0), got %c", state.Grid[1][0].Char)
	}

	// Check cursor position
	if state.CursorRow != 1 {
		t.Errorf("Expected cursor at row 1, got %d", state.CursorRow)
	}
}

func TestProcessOutput_CarriageReturn(t *testing.T) {
	state := NewTerminalState(25, 80)
	state.ProcessOutput([]byte("Hello\rWorld"))

	// Carriage return should move cursor to start of line
	// "World" should overwrite "Hello"
	expected := "World"
	for i, char := range expected {
		if state.Grid[0][i].Char != char {
			t.Errorf("At position %d: expected %c, got %c", i, char, state.Grid[0][i].Char)
		}
	}
}

func TestProcessOutput_Tab(t *testing.T) {
	state := NewTerminalState(25, 80)
	state.ProcessOutput([]byte("A\tB"))

	// 'A' at column 0, then tab advances to column 8, 'B' at column 8
	if state.Grid[0][0].Char != 'A' {
		t.Errorf("Expected 'A' at column 0, got %c", state.Grid[0][0].Char)
	}
	if state.Grid[0][8].Char != 'B' {
		t.Errorf("Expected 'B' at column 8, got %c", state.Grid[0][8].Char)
	}
}

func TestProcessOutput_SetTabStop(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Move to col 4 and set tab stop
	// \x1b[1;5H moves the cursor
	// \x1bH sets the tab stop
	state.ProcessOutput([]byte("\x1b[1;5H\x1bH"))

	if !state.TabStops[4] {
		t.Errorf("Expected tab stop at column 4 to be set")
	}
}

func TestProcessOutput_ClearTabStop(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Default tab stop at 8 should exist
	if !state.TabStops[8] {
		t.Errorf("Expected default tab stop at 8")
	}

	// Move to col 8 and clear tab stop
	state.ProcessOutput([]byte("\x1b[1;9H\x1b[0g"))

	if state.TabStops[8] {
		t.Errorf("Expected tab stop at column 8 to be cleared")
	}
}

func TestProcessOutput_ClearAllTabStops(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Default tab stops should exist
	if len(state.TabStops) == 0 {
		t.Errorf("Expected default tab stops")
	}

	// Clear all tab stops
	state.ProcessOutput([]byte("\x1b[3g"))

	if len(state.TabStops) != 0 {
		t.Errorf("Expected all tab stops to be cleared, got %d", len(state.TabStops))
	}
}

func TestProcessOutput_TabWithCustomStops(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Clear all tab stops
	state.ProcessOutput([]byte("\x1b[3g"))

	// Set custom tab stops at 4 and 12
	state.ProcessOutput([]byte("\x1b[1;5H\x1bH"))
	state.ProcessOutput([]byte("\x1b[1;13H\x1bH"))

	// Go to origin and print A \t B \t C \t D
	state.ProcessOutput([]byte("\x1b[1;1HA\tB\tC\tD"))

	if state.Grid[0][0].Char != 'A' {
		t.Errorf("Expected 'A' at column 0, got %c", state.Grid[0][0].Char)
	}
	if state.Grid[0][4].Char != 'B' {
		t.Errorf("Expected 'B' at column 4, got %c", state.Grid[0][4].Char)
	}
	if state.Grid[0][12].Char != 'C' {
		t.Errorf("Expected 'C' at column 12, got %c", state.Grid[0][12].Char)
	}
	// No more tab stops, D should be at the end of the line
	if state.Grid[0][79].Char != 'D' {
		t.Errorf("Expected 'D' at column 79, got %c", state.Grid[0][79].Char)
	}
}

func TestProcessOutput_CursorMovement(t *testing.T) {
	tests := []struct {
		name        string
		sequence    string
		expectedRow int
		expectedCol int
	}{
		{"Cursor Up", "\x1b[A", -1, 0}, // Will be clamped to 0
		{"Cursor Down", "\x1b[B", 1, 0},
		{"Cursor Forward", "\x1b[C", 0, 1},
		{"Cursor Backward", "\x1b[D", 0, 0},          // Will be clamped to 0
		{"Cursor Position 5,10", "\x1b[5;10H", 4, 9}, // 1-based to 0-based
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewTerminalState(25, 80)
			state.ProcessOutput([]byte(tt.sequence))

			if tt.expectedRow >= 0 && state.CursorRow != tt.expectedRow {
				t.Errorf("Expected cursor row %d, got %d", tt.expectedRow, state.CursorRow)
			}
			if tt.expectedCol >= 0 && state.CursorCol != tt.expectedCol {
				t.Errorf("Expected cursor col %d, got %d", tt.expectedCol, state.CursorCol)
			}
		})
	}
}

func TestProcessOutput_ClearScreen(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Fill screen with text
	state.ProcessOutput([]byte("AAAAA\nBBBBB\nCCCCC"))

	// Clear screen
	state.ProcessOutput([]byte("\x1b[2J"))

	// Check all cells are spaces
	for i := 0; i < state.Rows; i++ {
		for j := 0; j < state.Cols; j++ {
			if state.Grid[i][j].Char != ' ' {
				t.Errorf("Expected space at (%d,%d) after clear, got %c", i, j, state.Grid[i][j].Char)
			}
		}
	}
}

func TestProcessOutput_ClearLine(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Write text
	state.ProcessOutput([]byte("Hello World"))

	// Move cursor to middle of line
	state.CursorCol = 5

	// Clear from cursor to end of line
	state.ProcessOutput([]byte("\x1b[K"))

	// Check first 5 characters remain
	expected := "Hello"
	for i, char := range expected {
		if state.Grid[0][i].Char != char {
			t.Errorf("At position %d: expected %c, got %c", i, char, state.Grid[0][i].Char)
		}
	}

	// Check rest of line is spaces
	for i := 5; i < state.Cols; i++ {
		if state.Grid[0][i].Char != ' ' {
			t.Errorf("Expected space at position %d, got %c", i, state.Grid[0][i].Char)
		}
	}
}

func TestProcessOutput_SGR_Bold(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Enable bold and write text
	state.ProcessOutput([]byte("\x1b[1mBold"))

	// Check style is applied
	if !state.CurrentStyle.Bold {
		t.Error("Expected bold style to be active")
	}

	// Check characters have bold style
	for i := 0; i < 4; i++ {
		if !state.Grid[0][i].Style.Bold {
			t.Errorf("Expected bold style at position %d", i)
		}
	}
}

func TestProcessOutput_SGR_Colors(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Set foreground color
	state.ProcessOutput([]byte("\x1b[31mRed"))

	// Check style has color
	if state.CurrentStyle.FgColor != "color1" {
		t.Errorf("Expected color1 foreground, got %s", state.CurrentStyle.FgColor)
	}

	// Set background color
	state.ProcessOutput([]byte("\x1b[42m"))

	if state.CurrentStyle.BgColor != "color2" {
		t.Errorf("Expected color2 background, got %s", state.CurrentStyle.BgColor)
	}
}

func TestProcessOutput_SGR_Reset(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Enable bold and color
	state.ProcessOutput([]byte("\x1b[1;31m"))

	// Verify styles are set
	if !state.CurrentStyle.Bold || state.CurrentStyle.FgColor != "color1" {
		t.Error("Expected bold and color to be set")
	}

	// Reset
	state.ProcessOutput([]byte("\x1b[0m"))

	// Check styles are reset
	if state.CurrentStyle.Bold || state.CurrentStyle.FgColor != "" {
		t.Error("Expected styles to be reset")
	}
}

func TestProcessOutput_CursorVisibility(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Hide cursor
	state.ProcessOutput([]byte("\x1b[?25l"))
	if state.CursorVisible {
		t.Error("Expected cursor to be hidden")
	}

	// Show cursor
	state.ProcessOutput([]byte("\x1b[?25h"))
	if !state.CursorVisible {
		t.Error("Expected cursor to be visible")
	}
}

func TestScrolling(t *testing.T) {
	state := NewTerminalState(5, 10) // Small terminal for testing

	// Fill screen
	for i := 0; i < 5; i++ {
		state.ProcessOutput([]byte(string(rune('A' + i))))
		state.ProcessOutput([]byte("\n"))
	}

	// Cursor should be at row 5 (past end), causing scroll
	if state.CursorRow != 4 {
		t.Errorf("Expected cursor at row 4 after scroll, got %d", state.CursorRow)
	}

	// First line should be 'B' (original first line 'A' scrolled off)
	if state.Grid[0][0].Char != 'B' {
		t.Errorf("Expected 'B' at top after scroll, got %c", state.Grid[0][0].Char)
	}
}

func TestResize(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Write some text
	state.ProcessOutput([]byte("Hello"))

	// Resize smaller
	state.Resize(10, 40)

	if state.Rows != 10 || state.Cols != 40 {
		t.Errorf("Expected 10x40, got %dx%d", state.Rows, state.Cols)
	}

	// Check text is preserved
	if state.Grid[0][0].Char != 'H' {
		t.Errorf("Expected 'H' after resize, got %c", state.Grid[0][0].Char)
	}

	// Resize larger
	state.Resize(30, 100)

	if state.Rows != 30 || state.Cols != 100 {
		t.Errorf("Expected 30x100, got %dx%d", state.Rows, state.Cols)
	}

	// Check text is still preserved
	if state.Grid[0][0].Char != 'H' {
		t.Errorf("Expected 'H' after second resize, got %c", state.Grid[0][0].Char)
	}
}

func TestClone(t *testing.T) {
	state := NewTerminalState(25, 80)
	state.ProcessOutput([]byte("Test"))
	state.CurrentStyle.Bold = true

	clone := state.Clone()

	// Check dimensions match
	if clone.Rows != state.Rows || clone.Cols != state.Cols {
		t.Errorf("Clone dimensions don't match: %dx%d vs %dx%d",
			clone.Rows, clone.Cols, state.Rows, state.Cols)
	}

	// Check content matches
	if clone.Grid[0][0].Char != 'T' {
		t.Errorf("Clone content doesn't match, expected 'T', got %c", clone.Grid[0][0].Char)
	}

	// Check cursor matches
	if clone.CursorRow != state.CursorRow || clone.CursorCol != state.CursorCol {
		t.Errorf("Clone cursor doesn't match")
	}

	// Check version matches
	if clone.Version != state.Version {
		t.Errorf("Clone version doesn't match: %d vs %d", clone.Version, state.Version)
	}

	// Verify it's a deep copy - modifying clone shouldn't affect original
	clone.Grid[0][0].Char = 'X'
	if state.Grid[0][0].Char != 'T' {
		t.Error("Modifying clone affected original state")
	}
}

func TestGenerateDelta_FullSync(t *testing.T) {
	state := NewTerminalState(25, 80)
	state.ProcessOutput([]byte("Hello"))

	// Generate full sync delta (nil previous state)
	delta := state.GenerateDelta(nil)

	if delta.GetDelta() == nil {
		t.Fatal("Expected delta, got nil")
	}

	d := delta.GetDelta()
	if !d.FullSync {
		t.Error("Expected full sync flag to be true")
	}

	if d.ToState != state.Version {
		t.Errorf("Expected to_state %d, got %d", state.Version, d.ToState)
	}

	if d.Cursor.Row != 0 || d.Cursor.Col != 5 {
		t.Errorf("Expected cursor at (0,5), got (%d,%d)", d.Cursor.Row, d.Cursor.Col)
	}

	if len(d.Lines) != state.Rows {
		t.Errorf("Expected %d lines in full sync, got %d", state.Rows, len(d.Lines))
	}
}

func TestGenerateDelta_Incremental(t *testing.T) {
	// Create initial state
	oldState := NewTerminalState(25, 80)
	oldState.ProcessOutput([]byte("Line 1\nLine 2\nLine 3"))

	// Clone and modify
	newState := oldState.Clone()
	newState.ProcessOutput([]byte("\x1b[2;1H")) // Move to line 2, col 1
	newState.ProcessOutput([]byte("Modified"))

	// Generate incremental delta
	delta := newState.GenerateDelta(oldState)

	d := delta.GetDelta()
	if d.FullSync {
		t.Error("Expected incremental delta, got full sync")
	}

	if d.FromState != oldState.Version {
		t.Errorf("Expected from_state %d, got %d", oldState.Version, d.FromState)
	}

	if d.ToState != newState.Version {
		t.Errorf("Expected to_state %d, got %d", newState.Version, d.ToState)
	}

	// Should only have changed line
	if len(d.Lines) == 0 {
		t.Error("Expected at least one line delta")
	}

	// Verify changed line is line 1 (0-indexed)
	foundModifiedLine := false
	for _, lineDelta := range d.Lines {
		if lineDelta.LineNumber == 1 {
			foundModifiedLine = true
			break
		}
	}

	if !foundModifiedLine {
		t.Error("Expected modified line 1 in delta")
	}
}

func TestGenerateDelta_DimensionChange(t *testing.T) {
	oldState := NewTerminalState(25, 80)
	newState := NewTerminalState(30, 100)

	delta := newState.GenerateDelta(oldState)

	d := delta.GetDelta()
	if !d.FullSync {
		t.Error("Expected full sync when dimensions change")
	}

	if d.Dimensions == nil {
		t.Fatal("Expected dimensions in delta")
	}

	if d.Dimensions.Rows != 30 || d.Dimensions.Cols != 100 {
		t.Errorf("Expected dimensions 30x100, got %dx%d",
			d.Dimensions.Rows, d.Dimensions.Cols)
	}
}

func TestGetLineText(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Write text with trailing spaces
	state.ProcessOutput([]byte("Hello    "))

	// Get line text (should trim trailing spaces)
	lineText := state.getLineText(0)

	expected := "Hello"
	if lineText != expected {
		t.Errorf("Expected '%s', got '%s'", expected, lineText)
	}
}

func TestGetLineText_WithStyles(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Write bold text
	state.ProcessOutput([]byte("\x1b[1mBold\x1b[0m Normal"))

	lineText := state.getLineText(0)

	// Should include ANSI codes
	if !containsString(lineText, "\x1b[1m") {
		t.Error("Expected bold ANSI code in line text")
	}

	if !containsString(lineText, "\x1b[0m") {
		t.Error("Expected reset ANSI code in line text")
	}
}

func TestComplexOutput(t *testing.T) {
	state := NewTerminalState(25, 80)

	// Simulate complex terminal output (like tmux status line)
	output := "\x1b[2J\x1b[H" + // Clear screen and home cursor
		"Session: test\n" + // Line 1
		"\x1b[32mActive\x1b[0m\n" + // Line 2 with color
		"\x1b[24;1H" + // Move to last line
		"\x1b[7m[0] bash\x1b[0m " + // Reverse video status
		"\x1b[33m10:30\x1b[0m" // Yellow time

	err := state.ProcessOutput([]byte(output))
	if err != nil {
		t.Fatalf("ProcessOutput failed: %v", err)
	}

	// Check cursor is on last line
	if state.CursorRow != 23 {
		t.Errorf("Expected cursor at row 23, got %d", state.CursorRow)
	}

	// Check some content
	if state.Grid[0][0].Char != 'S' {
		t.Errorf("Expected 'S' at (0,0), got %c", state.Grid[0][0].Char)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || strings.Contains(s, substr)))
}

func TestInsertLines(t *testing.T) {
	state := NewTerminalState(10, 80)

	// Fill first 5 lines
	for i := 0; i < 5; i++ {
		state.ProcessOutput([]byte(string(rune('A'+i)) + "\n"))
	}

	// Move cursor to line 2
	state.CursorRow = 2
	state.CursorCol = 0

	// Insert 2 lines
	state.insertLines(2)

	// Lines should be shifted down:
	// 0: A
	// 1: B
	// 2: (blank) - inserted
	// 3: (blank) - inserted
	// 4: C
	// 5: D
	// 6: E

	if state.Grid[0][0].Char != 'A' {
		t.Errorf("Expected 'A' at line 0, got %c", state.Grid[0][0].Char)
	}
	if state.Grid[1][0].Char != 'B' {
		t.Errorf("Expected 'B' at line 1, got %c", state.Grid[1][0].Char)
	}
	if state.Grid[2][0].Char != ' ' {
		t.Errorf("Expected space at line 2 (inserted), got %c", state.Grid[2][0].Char)
	}
	if state.Grid[4][0].Char != 'C' {
		t.Errorf("Expected 'C' at line 4 (shifted), got %c", state.Grid[4][0].Char)
	}
}

func TestDeleteLines(t *testing.T) {
	state := NewTerminalState(10, 80)

	// Fill first 5 lines
	for i := 0; i < 5; i++ {
		state.ProcessOutput([]byte(string(rune('A'+i)) + "\n"))
	}

	// Move cursor to line 2
	state.CursorRow = 2
	state.CursorCol = 0

	// Delete 2 lines
	state.deleteLines(2)

	// Lines should be shifted up:
	// 0: A
	// 1: B
	// 2: E (was line 4, lines 2-3 deleted)
	// 3: (blank)
	// 4: (blank)

	if state.Grid[0][0].Char != 'A' {
		t.Errorf("Expected 'A' at line 0, got %c", state.Grid[0][0].Char)
	}
	if state.Grid[1][0].Char != 'B' {
		t.Errorf("Expected 'B' at line 1, got %c", state.Grid[1][0].Char)
	}
	if state.Grid[2][0].Char != 'E' {
		t.Errorf("Expected 'E' at line 2 (shifted up), got %c", state.Grid[2][0].Char)
	}
}
