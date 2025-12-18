package framebuffer

import (
	"strings"
	"testing"

	"claude-squad/session"
)

// TestDiffGenerator_NilOldState verifies full redraw when old state is nil
func TestDiffGenerator_NilOldState(t *testing.T) {
	g := NewDiffGenerator()
	newState := session.NewTerminalState(3, 10)

	// Write some content
	newState.ProcessOutput([]byte("Hello"))
	newState.ProcessOutput([]byte("\nWorld"))

	result := g.GenerateDiff(nil, newState)

	if !result.FullRedraw {
		t.Error("Expected FullRedraw to be true when old state is nil")
	}

	if result.FromSequence != 0 {
		t.Errorf("Expected FromSequence=0, got %d", result.FromSequence)
	}

	if len(result.DiffBytes) == 0 {
		t.Error("Expected non-empty DiffBytes for full redraw")
	}

	// Verify it contains clear screen sequence
	diffStr := string(result.DiffBytes)
	if !strings.Contains(diffStr, "\x1b[2J") {
		t.Error("Full redraw should contain clear screen sequence \\x1b[2J")
	}
}

// TestDiffGenerator_DimensionChange verifies full redraw on dimension change
func TestDiffGenerator_DimensionChange(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(10, 80)
	newState := session.NewTerminalState(20, 100) // Different dimensions

	result := g.GenerateDiff(oldState, newState)

	if !result.FullRedraw {
		t.Error("Expected FullRedraw when dimensions change")
	}
}

// TestDiffGenerator_IdenticalStates verifies minimal output for identical states
func TestDiffGenerator_IdenticalStates(t *testing.T) {
	g := NewDiffGenerator()

	// Create two identical states
	state1 := session.NewTerminalState(3, 10)
	state1.ProcessOutput([]byte("Hello"))

	state2 := state1.Clone()

	result := g.GenerateDiff(state1, state2)

	if result.FullRedraw {
		t.Error("Should not be full redraw for identical states")
	}

	if result.ChangedCells != 0 {
		t.Errorf("Expected 0 changed cells for identical states, got %d", result.ChangedCells)
	}

	// Should only contain cursor positioning at most
	if len(result.DiffBytes) > 20 {
		t.Errorf("Expected minimal diff for identical states, got %d bytes", len(result.DiffBytes))
	}
}

// TestDiffGenerator_SingleCellChange verifies efficient single cell diff
func TestDiffGenerator_SingleCellChange(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 10)
	oldState.ProcessOutput([]byte("Hello"))

	newState := oldState.Clone()
	// Change 'H' to 'h' at position (0,0)
	newState.Grid[0][0].Char = 'h'
	newState.Version++

	result := g.GenerateDiff(oldState, newState)

	if result.FullRedraw {
		t.Error("Should not be full redraw for single cell change")
	}

	if result.ChangedCells != 1 {
		t.Errorf("Expected 1 changed cell, got %d", result.ChangedCells)
	}

	// Diff should be small (cursor position + 1 char)
	if len(result.DiffBytes) > 20 {
		t.Errorf("Diff should be small for single cell change, got %d bytes", len(result.DiffBytes))
	}

	// Should contain the character 'h'
	if !strings.Contains(string(result.DiffBytes), "h") {
		t.Error("Diff should contain the changed character 'h'")
	}
}

// TestDiffGenerator_RowChange verifies efficient row diff
func TestDiffGenerator_RowChange(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(5, 20)
	oldState.ProcessOutput([]byte("Line 1\nLine 2\nLine 3"))

	newState := oldState.Clone()
	// Change line 2
	for i := 0; i < 6; i++ {
		newState.Grid[1][i].Char = rune("CHANGE"[i])
	}
	newState.Version++

	result := g.GenerateDiff(oldState, newState)

	if result.FullRedraw {
		t.Error("Should not be full redraw for row change")
	}

	// Should contain "CHANGE"
	diffStr := string(result.DiffBytes)
	if !strings.Contains(diffStr, "CHANGE") {
		t.Error("Diff should contain 'CHANGE'")
	}

	// Unchanged rows should not increase diff size significantly
	// Row 0 and row 2+ should be skipped
}

// TestDiffGenerator_CursorMovement verifies smart cursor movement
func TestDiffGenerator_CursorMovement(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(10, 80)
	oldState.CursorRow = 0
	oldState.CursorCol = 0

	newState := oldState.Clone()
	newState.CursorRow = 5
	newState.CursorCol = 10
	newState.Version++

	result := g.GenerateDiff(oldState, newState)

	diffStr := string(result.DiffBytes)

	// Should contain cursor positioning sequence
	// Either absolute \x1b[6;11H (1-indexed) or relative movement
	hasAbsolute := strings.Contains(diffStr, "\x1b[6;11H")
	hasRelative := strings.Contains(diffStr, "\x1b[") && (strings.Contains(diffStr, "B") || strings.Contains(diffStr, "C"))

	if !hasAbsolute && !hasRelative {
		t.Error("Diff should contain cursor movement sequence")
	}
}

// TestDiffGenerator_CursorVisibilityChange verifies cursor show/hide
func TestDiffGenerator_CursorVisibilityChange(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 10)
	oldState.CursorVisible = true

	newState := oldState.Clone()
	newState.CursorVisible = false
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	// Should contain cursor hide sequence
	if !strings.Contains(diffStr, "\x1b[?25l") {
		t.Error("Diff should contain cursor hide sequence \\x1b[?25l")
	}

	// Test opposite direction
	oldState.CursorVisible = false
	newState.CursorVisible = true
	newState.Version++

	result = g.GenerateDiff(oldState, newState)
	diffStr = string(result.DiffBytes)

	// Should contain cursor show sequence
	if !strings.Contains(diffStr, "\x1b[?25h") {
		t.Error("Diff should contain cursor show sequence \\x1b[?25h")
	}
}

// TestDiffGenerator_StyleChange verifies SGR sequence generation
func TestDiffGenerator_StyleChange(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 10)
	oldState.ProcessOutput([]byte("Plain"))

	newState := oldState.Clone()
	// Make first character bold
	newState.Grid[0][0].Style.Bold = true
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	// Should contain bold SGR sequence \x1b[1m
	if !strings.Contains(diffStr, "\x1b[1m") {
		t.Error("Diff should contain bold SGR sequence \\x1b[1m")
	}
}

// TestDiffGenerator_ColorChange verifies color SGR generation
func TestDiffGenerator_ColorChange(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 10)
	oldState.ProcessOutput([]byte("Color"))

	newState := oldState.Clone()
	// Make first character red (color1 = red)
	newState.Grid[0][0].Style.FgColor = "color1"
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	// Should contain red foreground SGR sequence \x1b[31m
	if !strings.Contains(diffStr, "\x1b[31m") {
		t.Errorf("Diff should contain red foreground SGR \\x1b[31m, got: %q", diffStr)
	}
}

// TestDiffGenerator_256Color verifies 256-color SGR generation
func TestDiffGenerator_256Color(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 10)
	oldState.ProcessOutput([]byte("X"))

	newState := oldState.Clone()
	// Use 256-color (color index 196 = bright red)
	newState.Grid[0][0].Style.FgColor = "color-196"
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	// Should contain 256-color SGR sequence \x1b[38;5;196m
	if !strings.Contains(diffStr, "\x1b[38;5;196m") {
		t.Errorf("Diff should contain 256-color SGR \\x1b[38;5;196m, got: %q", diffStr)
	}
}

// TestDiffGenerator_RGBColor verifies true-color SGR generation
func TestDiffGenerator_RGBColor(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 10)
	oldState.ProcessOutput([]byte("X"))

	newState := oldState.Clone()
	// Use RGB color
	newState.Grid[0][0].Style.FgColor = "rgb(255,128,64)"
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	// Should contain RGB SGR sequence \x1b[38;2;255;128;64m
	if !strings.Contains(diffStr, "\x1b[38;2;255;128;64m") {
		t.Errorf("Diff should contain RGB SGR \\x1b[38;2;255;128;64m, got: %q", diffStr)
	}
}

// TestDiffGenerator_SegmentMerging verifies contiguous changes are batched
func TestDiffGenerator_SegmentMerging(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 20)
	oldState.ProcessOutput([]byte("AAAAAAAAAAAAAAAAAAAA")) // 20 A's

	newState := oldState.Clone()
	// Change positions 5-9 to B's (5 contiguous cells)
	for i := 5; i < 10; i++ {
		newState.Grid[0][i].Char = 'B'
	}
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	if result.ChangedCells != 5 {
		t.Errorf("Expected 5 changed cells, got %d", result.ChangedCells)
	}

	// Should contain "BBBBB" as contiguous output
	if !strings.Contains(diffStr, "BBBBB") {
		t.Error("Contiguous changed cells should be batched together")
	}
}

// TestDiffGenerator_MultipleSegments verifies non-contiguous changes create separate segments
func TestDiffGenerator_MultipleSegments(t *testing.T) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(3, 20)
	oldState.ProcessOutput([]byte("AAAAAAAAAAAAAAAAAAAA")) // 20 A's

	newState := oldState.Clone()
	// Change positions 2-3 and 10-11 (two separate segments)
	newState.Grid[0][2].Char = 'X'
	newState.Grid[0][3].Char = 'X'
	newState.Grid[0][10].Char = 'Y'
	newState.Grid[0][11].Char = 'Y'
	newState.Version++

	result := g.GenerateDiff(oldState, newState)
	diffStr := string(result.DiffBytes)

	if result.ChangedCells != 4 {
		t.Errorf("Expected 4 changed cells, got %d", result.ChangedCells)
	}

	// Should contain both XX and YY
	if !strings.Contains(diffStr, "XX") || !strings.Contains(diffStr, "YY") {
		t.Error("Should contain both changed segments")
	}
}

// TestFindChangedSegments verifies segment detection algorithm
func TestFindChangedSegments(t *testing.T) {
	g := NewDiffGenerator()

	tests := []struct {
		name     string
		oldRow   string
		newRow   string
		expected []Segment
	}{
		{
			name:     "no changes",
			oldRow:   "Hello",
			newRow:   "Hello",
			expected: nil,
		},
		{
			name:     "single char change",
			oldRow:   "Hello",
			newRow:   "Hallo",
			expected: []Segment{{StartCol: 1, EndCol: 1}},
		},
		{
			name:     "contiguous change",
			oldRow:   "Hello",
			newRow:   "HXXXX",
			expected: []Segment{{StartCol: 1, EndCol: 4}},
		},
		{
			name:     "two segments",
			oldRow:   "AAAAAA",
			newRow:   "AXAAXA",
			expected: []Segment{{StartCol: 1, EndCol: 1}, {StartCol: 4, EndCol: 4}},
		},
		{
			name:     "full row change",
			oldRow:   "AAAA",
			newRow:   "BBBB",
			expected: []Segment{{StartCol: 0, EndCol: 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldRow := makeRow(tt.oldRow)
			newRow := makeRow(tt.newRow)

			segments := g.findChangedSegments(oldRow, newRow)

			if len(segments) != len(tt.expected) {
				t.Errorf("Expected %d segments, got %d", len(tt.expected), len(segments))
				return
			}

			for i, seg := range segments {
				if seg.StartCol != tt.expected[i].StartCol || seg.EndCol != tt.expected[i].EndCol {
					t.Errorf("Segment %d: expected (%d,%d), got (%d,%d)",
						i, tt.expected[i].StartCol, tt.expected[i].EndCol, seg.StartCol, seg.EndCol)
				}
			}
		})
	}
}

// TestStylesEqual verifies style comparison
func TestStylesEqual(t *testing.T) {
	g := NewDiffGenerator()

	s1 := session.DefaultStyle()
	s2 := session.DefaultStyle()

	if !g.stylesEqual(s1, s2) {
		t.Error("Default styles should be equal")
	}

	s2.Bold = true
	if g.stylesEqual(s1, s2) {
		t.Error("Styles with different Bold should not be equal")
	}

	s1.Bold = true
	s1.FgColor = "color1"
	s2.FgColor = "color2"
	if g.stylesEqual(s1, s2) {
		t.Error("Styles with different FgColor should not be equal")
	}
}

// TestColorToSGR verifies color code conversion
func TestColorToSGR(t *testing.T) {
	g := NewDiffGenerator()

	tests := []struct {
		color      string
		foreground bool
		expected   string
	}{
		{"", true, "39"},           // Default fg
		{"", false, "49"},          // Default bg
		{"color0", true, "30"},     // Black fg
		{"color1", true, "31"},     // Red fg
		{"color7", false, "47"},    // White bg
		{"bright-color0", true, "90"},   // Bright black fg
		{"bright-color1", false, "101"}, // Bright red bg
		{"color-196", true, "38;5;196"}, // 256-color fg
		{"color-255", false, "48;5;255"}, // 256-color bg
		{"rgb(255,0,0)", true, "38;2;255;0;0"}, // RGB red fg
		{"rgb(0,255,0)", false, "48;2;0;255;0"}, // RGB green bg
	}

	for _, tt := range tests {
		t.Run(tt.color, func(t *testing.T) {
			result := g.colorToSGR(tt.color, tt.foreground)
			if result != tt.expected {
				t.Errorf("colorToSGR(%q, %v) = %q, expected %q",
					tt.color, tt.foreground, result, tt.expected)
			}
		})
	}
}

// BenchmarkDiffGenerator_SmallChange benchmarks small diff generation
func BenchmarkDiffGenerator_SmallChange(b *testing.B) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(24, 80)
	oldState.ProcessOutput([]byte("Hello World"))

	newState := oldState.Clone()
	newState.Grid[0][0].Char = 'h'
	newState.Version++

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.GenerateDiff(oldState, newState)
	}
}

// BenchmarkDiffGenerator_FullScreen benchmarks full screen redraw
func BenchmarkDiffGenerator_FullScreen(b *testing.B) {
	g := NewDiffGenerator()

	newState := session.NewTerminalState(24, 80)
	// Fill with content
	for row := 0; row < 24; row++ {
		for col := 0; col < 80; col++ {
			newState.Grid[row][col].Char = 'X'
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.GenerateDiff(nil, newState)
	}
}

// BenchmarkDiffGenerator_RowChange benchmarks single row change
func BenchmarkDiffGenerator_RowChange(b *testing.B) {
	g := NewDiffGenerator()

	oldState := session.NewTerminalState(24, 80)
	// Fill with content
	for row := 0; row < 24; row++ {
		for col := 0; col < 80; col++ {
			oldState.Grid[row][col].Char = 'A'
		}
	}

	newState := oldState.Clone()
	// Change middle row
	for col := 0; col < 80; col++ {
		newState.Grid[12][col].Char = 'B'
	}
	newState.Version++

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.GenerateDiff(oldState, newState)
	}
}

// Helper function to create a row from string
func makeRow(s string) []session.Cell {
	row := make([]session.Cell, len(s))
	for i, ch := range s {
		row[i] = session.Cell{
			Char:  ch,
			Style: session.DefaultStyle(),
		}
	}
	return row
}
