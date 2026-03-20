// Package framebuffer provides Mosh-style terminal state diffing.
//
// This package implements the core State Synchronization Protocol (SSP) algorithm
// inspired by Mosh (Mobile Shell). Instead of sending complete terminal states,
// it generates minimal ANSI escape sequences that transform an old framebuffer
// state into a new one.
//
// Key Design Principles (from Mosh):
//  1. Generate minimal escape sequences, not cell-by-cell diffs
//  2. Use smart cursor positioning (relative vs absolute)
//  3. Batch consecutive cells with the same style
//  4. Skip unchanged rows entirely (fast pointer comparison first)
//  5. Preserve correctness over any terminal emulator (pure ANSI output)
package framebuffer

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/session"
)

// DiffResult contains the generated diff and metadata
type DiffResult struct {
	// DiffBytes contains the minimal ANSI escape sequences to transform old → new
	DiffBytes []byte

	// FromSequence is the sequence number of the old state
	FromSequence uint64

	// ToSequence is the sequence number of the new state
	ToSequence uint64

	// FullRedraw indicates this is a complete redraw (no dependency on old state)
	FullRedraw bool

	// ChangedCells is the count of cells that changed (for metrics)
	ChangedCells int

	// UnchangedCells is the count of cells that didn't change (for metrics)
	UnchangedCells int
}

// Segment represents a contiguous range of changed cells in a row
type Segment struct {
	StartCol int
	EndCol   int // Inclusive
}

// DiffGenerator generates minimal ANSI diffs between terminal states
type DiffGenerator struct {
	// cursorRow and cursorCol track the current output cursor position
	cursorRow int
	cursorCol int

	// currentStyle tracks the current SGR rendition state
	currentStyle session.CellStyle

	// output buffer for building the diff
	output bytes.Buffer
}

// NewDiffGenerator creates a new diff generator
func NewDiffGenerator() *DiffGenerator {
	return &DiffGenerator{
		cursorRow:    0,
		cursorCol:    0,
		currentStyle: session.DefaultStyle(),
	}
}

// GenerateDiff generates minimal ANSI escape sequences to transform old state to new state.
// This is the main entry point for Mosh-style state synchronization.
//
// Algorithm overview:
//  1. Handle dimension changes (requires full redraw)
//  2. Compare row by row using fast generation counter check
//  3. For changed rows, find contiguous changed segments
//  4. Emit minimal cursor movements and character writes
//  5. Position cursor at final location and set visibility
func (g *DiffGenerator) GenerateDiff(old, new *session.TerminalState) *DiffResult {
	g.reset()

	result := &DiffResult{
		FromSequence: 0,
		ToSequence:   new.Version,
	}

	if old != nil {
		result.FromSequence = old.Version
	}

	// Handle nil old state or dimension changes - requires full redraw
	if old == nil || old.Rows != new.Rows || old.Cols != new.Cols {
		result.FullRedraw = true
		g.generateFullRedraw(new)
		result.DiffBytes = g.output.Bytes()
		result.ChangedCells = new.Rows * new.Cols
		return result
	}

	// Compare row by row and generate minimal diff
	for row := 0; row < new.Rows; row++ {
		// Fast path: check if rows are identical using generation counter or pointer
		if g.rowsEqual(old, new, row) {
			result.UnchangedCells += new.Cols
			continue
		}

		// Find changed segments within this row
		segments := g.findChangedSegments(old.Grid[row], new.Grid[row])

		for _, seg := range segments {
			// Move cursor to segment start
			g.moveCursorTo(row, seg.StartCol)

			// Write changed cells with style transitions
			for col := seg.StartCol; col <= seg.EndCol; col++ {
				cell := new.Grid[row][col]

				// Emit style change if needed
				if !g.stylesEqual(cell.Style, g.currentStyle) {
					g.emitStyleTransition(cell.Style)
				}

				// Emit character
				g.emitChar(cell.Char)
				result.ChangedCells++
			}
		}

		// Count unchanged cells in this row
		result.UnchangedCells += new.Cols - g.countChangedCellsInRow(old.Grid[row], new.Grid[row])
	}

	// Position cursor at final location
	g.moveCursorTo(new.CursorRow, new.CursorCol)

	// Handle cursor visibility changes
	if old.CursorVisible != new.CursorVisible {
		if new.CursorVisible {
			g.output.WriteString("\x1b[?25h") // Show cursor
		} else {
			g.output.WriteString("\x1b[?25l") // Hide cursor
		}
	}

	result.DiffBytes = g.output.Bytes()
	return result
}

// reset prepares the generator for a new diff operation
func (g *DiffGenerator) reset() {
	g.output.Reset()
	g.cursorRow = 0
	g.cursorCol = 0
	g.currentStyle = session.DefaultStyle()
}

// generateFullRedraw generates ANSI sequences to draw the entire screen
func (g *DiffGenerator) generateFullRedraw(state *session.TerminalState) {
	// Hide cursor during redraw to prevent flicker
	g.output.WriteString("\x1b[?25l")

	// Clear screen and home cursor
	g.output.WriteString("\x1b[2J\x1b[H")
	g.cursorRow = 0
	g.cursorCol = 0

	// Reset style
	g.output.WriteString("\x1b[0m")
	g.currentStyle = session.DefaultStyle()

	// Draw each row
	for row := 0; row < state.Rows; row++ {
		if row > 0 {
			// Move to next row
			g.output.WriteString("\r\n")
			g.cursorRow = row
			g.cursorCol = 0
		}

		// Find the last non-space character to avoid trailing spaces
		lastNonSpace := -1
		for col := state.Cols - 1; col >= 0; col-- {
			if state.Grid[row][col].Char != ' ' || !g.isDefaultStyle(state.Grid[row][col].Style) {
				lastNonSpace = col
				break
			}
		}

		// Draw cells up to lastNonSpace
		for col := 0; col <= lastNonSpace; col++ {
			cell := state.Grid[row][col]

			// Emit style change if needed
			if !g.stylesEqual(cell.Style, g.currentStyle) {
				g.emitStyleTransition(cell.Style)
			}

			g.emitChar(cell.Char)
		}
	}

	// Position cursor at final location
	g.moveCursorTo(state.CursorRow, state.CursorCol)

	// Show cursor if visible
	if state.CursorVisible {
		g.output.WriteString("\x1b[?25h")
	}
}

// rowsEqual checks if two rows are identical (fast comparison)
func (g *DiffGenerator) rowsEqual(old, new *session.TerminalState, row int) bool {
	if row >= len(old.Grid) || row >= len(new.Grid) {
		return false
	}

	oldRow := old.Grid[row]
	newRow := new.Grid[row]

	if len(oldRow) != len(newRow) {
		return false
	}

	// Cell-by-cell comparison
	for col := 0; col < len(oldRow); col++ {
		if oldRow[col].Char != newRow[col].Char {
			return false
		}
		if !g.stylesEqual(oldRow[col].Style, newRow[col].Style) {
			return false
		}
	}

	return true
}

// findChangedSegments identifies contiguous ranges of changed cells in a row
func (g *DiffGenerator) findChangedSegments(oldRow, newRow []session.Cell) []Segment {
	var segments []Segment
	var currentSegment *Segment

	cols := len(newRow)
	if len(oldRow) < cols {
		cols = len(oldRow)
	}

	for col := 0; col < cols; col++ {
		changed := oldRow[col].Char != newRow[col].Char ||
			!g.stylesEqual(oldRow[col].Style, newRow[col].Style)

		if changed {
			if currentSegment == nil {
				currentSegment = &Segment{StartCol: col}
			}
			currentSegment.EndCol = col
		} else {
			if currentSegment != nil {
				segments = append(segments, *currentSegment)
				currentSegment = nil
			}
		}
	}

	// Handle new row being longer than old row
	if len(newRow) > len(oldRow) {
		if currentSegment == nil {
			currentSegment = &Segment{StartCol: len(oldRow)}
		}
		currentSegment.EndCol = len(newRow) - 1
	}

	// Close any remaining segment
	if currentSegment != nil {
		segments = append(segments, *currentSegment)
	}

	return segments
}

// countChangedCellsInRow counts the number of changed cells between two rows
func (g *DiffGenerator) countChangedCellsInRow(oldRow, newRow []session.Cell) int {
	count := 0
	cols := len(newRow)
	if len(oldRow) < cols {
		cols = len(oldRow)
	}

	for col := 0; col < cols; col++ {
		if oldRow[col].Char != newRow[col].Char ||
			!g.stylesEqual(oldRow[col].Style, newRow[col].Style) {
			count++
		}
	}

	// Count any extra cells in new row as changed
	if len(newRow) > len(oldRow) {
		count += len(newRow) - len(oldRow)
	}

	return count
}

// moveCursorTo emits the most efficient cursor movement sequence
func (g *DiffGenerator) moveCursorTo(row, col int) {
	if g.cursorRow == row && g.cursorCol == col {
		return // Already at target position
	}

	// Calculate movement costs
	deltaRow := row - g.cursorRow
	deltaCol := col - g.cursorCol

	// Cost of absolute positioning: \x1b[row;colH = 6-10 bytes
	absCost := 4 + len(strconv.Itoa(row+1)) + len(strconv.Itoa(col+1))

	// Cost of relative positioning
	relCost := 0
	if deltaRow != 0 {
		relCost += 3 + len(strconv.Itoa(abs(deltaRow))) // \x1b[nA or \x1b[nB
	}
	if deltaCol != 0 {
		if col == 0 {
			relCost += 1 // \r (carriage return)
		} else {
			relCost += 3 + len(strconv.Itoa(abs(deltaCol))) // \x1b[nC or \x1b[nD
		}
	}

	// Use the cheaper option
	if absCost <= relCost || (deltaRow != 0 && deltaCol != 0) {
		// Absolute positioning (1-indexed)
		g.output.WriteString(fmt.Sprintf("\x1b[%d;%dH", row+1, col+1))
	} else {
		// Relative positioning
		if deltaRow > 0 {
			if deltaRow == 1 {
				g.output.WriteString("\x1b[B")
			} else {
				g.output.WriteString(fmt.Sprintf("\x1b[%dB", deltaRow))
			}
		} else if deltaRow < 0 {
			if deltaRow == -1 {
				g.output.WriteString("\x1b[A")
			} else {
				g.output.WriteString(fmt.Sprintf("\x1b[%dA", -deltaRow))
			}
		}

		if deltaCol != 0 {
			if col == 0 {
				g.output.WriteString("\r")
			} else if deltaCol > 0 {
				if deltaCol == 1 {
					g.output.WriteString("\x1b[C")
				} else {
					g.output.WriteString(fmt.Sprintf("\x1b[%dC", deltaCol))
				}
			} else {
				if deltaCol == -1 {
					g.output.WriteString("\x1b[D")
				} else {
					g.output.WriteString(fmt.Sprintf("\x1b[%dD", -deltaCol))
				}
			}
		}
	}

	g.cursorRow = row
	g.cursorCol = col
}

// emitStyleTransition generates the minimal SGR sequence to change from current style to target
func (g *DiffGenerator) emitStyleTransition(target session.CellStyle) {
	// If target is default, just reset
	if g.isDefaultStyle(target) {
		g.output.WriteString("\x1b[0m")
		g.currentStyle = session.DefaultStyle()
		return
	}

	// Build list of SGR codes needed
	var codes []string

	// Check if we need to reset first
	needsReset := false
	if g.currentStyle.Bold && !target.Bold {
		needsReset = true
	}
	if g.currentStyle.Italic && !target.Italic {
		needsReset = true
	}
	if g.currentStyle.Underline && !target.Underline {
		needsReset = true
	}
	if g.currentStyle.Reverse && !target.Reverse {
		needsReset = true
	}

	if needsReset {
		codes = append(codes, "0")
		// After reset, need to re-apply all target attributes
		if target.Bold {
			codes = append(codes, "1")
		}
		if target.Italic {
			codes = append(codes, "3")
		}
		if target.Underline {
			codes = append(codes, "4")
		}
		if target.Reverse {
			codes = append(codes, "7")
		}
	} else {
		// Just add what's new
		if target.Bold && !g.currentStyle.Bold {
			codes = append(codes, "1")
		}
		if target.Italic && !g.currentStyle.Italic {
			codes = append(codes, "3")
		}
		if target.Underline && !g.currentStyle.Underline {
			codes = append(codes, "4")
		}
		if target.Reverse && !g.currentStyle.Reverse {
			codes = append(codes, "7")
		}
	}

	// Handle color changes
	if target.FgColor != g.currentStyle.FgColor || needsReset {
		fgCode := g.colorToSGR(target.FgColor, true)
		if fgCode != "" {
			codes = append(codes, fgCode)
		}
	}

	if target.BgColor != g.currentStyle.BgColor || needsReset {
		bgCode := g.colorToSGR(target.BgColor, false)
		if bgCode != "" {
			codes = append(codes, bgCode)
		}
	}

	// Emit the combined SGR sequence
	if len(codes) > 0 {
		g.output.WriteString("\x1b[")
		g.output.WriteString(strings.Join(codes, ";"))
		g.output.WriteString("m")
	}

	g.currentStyle = target
}

// colorToSGR converts a color string to SGR code
func (g *DiffGenerator) colorToSGR(color string, foreground bool) string {
	if color == "" {
		// Default color
		if foreground {
			return "39"
		}
		return "49"
	}

	// Handle basic colors (color0-7)
	if strings.HasPrefix(color, "color") && len(color) == 6 {
		n, err := strconv.Atoi(color[5:6])
		if err == nil && n >= 0 && n <= 7 {
			if foreground {
				return strconv.Itoa(30 + n)
			}
			return strconv.Itoa(40 + n)
		}
	}

	// Handle bright colors (bright-color0-7)
	if strings.HasPrefix(color, "bright-color") && len(color) == 13 {
		n, err := strconv.Atoi(color[12:13])
		if err == nil && n >= 0 && n <= 7 {
			if foreground {
				return strconv.Itoa(90 + n)
			}
			return strconv.Itoa(100 + n)
		}
	}

	// Handle 256 colors (color-N)
	if strings.HasPrefix(color, "color-") {
		n, err := strconv.Atoi(color[6:])
		if err == nil && n >= 0 && n <= 255 {
			if foreground {
				return fmt.Sprintf("38;5;%d", n)
			}
			return fmt.Sprintf("48;5;%d", n)
		}
	}

	// Handle RGB colors (rgb(r,g,b))
	if strings.HasPrefix(color, "rgb(") && strings.HasSuffix(color, ")") {
		inner := color[4 : len(color)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 3 {
			r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err1 == nil && err2 == nil && err3 == nil {
				if foreground {
					return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
				}
				return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
			}
		}
	}

	return ""
}

// emitChar writes a character and updates cursor position
func (g *DiffGenerator) emitChar(ch rune) {
	if ch == 0 {
		ch = ' ' // Treat null as space
	}
	g.output.WriteRune(ch)
	g.cursorCol++
}

// stylesEqual compares two cell styles for equality
func (g *DiffGenerator) stylesEqual(a, b session.CellStyle) bool {
	return a.FgColor == b.FgColor &&
		a.BgColor == b.BgColor &&
		a.Bold == b.Bold &&
		a.Italic == b.Italic &&
		a.Underline == b.Underline &&
		a.Reverse == b.Reverse
}

// isDefaultStyle checks if a style is the default (no formatting)
func (g *DiffGenerator) isDefaultStyle(s session.CellStyle) bool {
	return s.FgColor == "" &&
		s.BgColor == "" &&
		!s.Bold &&
		!s.Italic &&
		!s.Underline &&
		!s.Reverse
}

// abs returns the absolute value of an integer
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
