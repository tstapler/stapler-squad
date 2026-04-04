package session

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Cell represents a single terminal cell with character and attributes
type Cell struct {
	Char  rune
	Style CellStyle
}

// CellStyle represents text styling attributes
type CellStyle struct {
	FgColor   string
	BgColor   string
	Bold      bool
	Italic    bool
	Underline bool
	Reverse   bool
}

// DefaultStyle returns a default cell style
func DefaultStyle() CellStyle {
	return CellStyle{
		FgColor: "",
		BgColor: "",
	}
}

// TerminalState maintains the current terminal screen state
type TerminalState struct {
	mu sync.RWMutex

	// Terminal dimensions
	Rows int
	Cols int

	// Terminal screen buffer (2D grid)
	Grid [][]Cell

	// Cursor state
	CursorRow     int
	CursorCol     int
	CursorVisible bool

	// Saved cursor state
	SavedCursorRow int
	SavedCursorCol int

	// Current text style for new characters
	CurrentStyle CellStyle

	// State version for delta tracking
	Version uint64
}

// NewTerminalState creates a new terminal state with given dimensions
func NewTerminalState(rows, cols int) *TerminalState {
	state := &TerminalState{
		Rows:           rows,
		Cols:           cols,
		Grid:           make([][]Cell, rows),
		CursorRow:      0,
		CursorCol:      0,
		CursorVisible:  true,
		SavedCursorRow: 0,
		SavedCursorCol: 0,
		CurrentStyle:   DefaultStyle(),
		Version:        0,
	}

	// Initialize grid with empty cells
	for i := 0; i < rows; i++ {
		state.Grid[i] = make([]Cell, cols)
		for j := 0; j < cols; j++ {
			state.Grid[i][j] = Cell{
				Char:  ' ',
				Style: DefaultStyle(),
			}
		}
	}

	return state
}

// ANSI escape sequence patterns
var (
	// CSI (Control Sequence Introducer) pattern: ESC [ params letter
	// Allows '?' for DEC private modes
	csiPattern = regexp.MustCompile(`\x1b\[(\??[0-9;]*)([A-Za-z])`)

	// OSC (Operating System Command) pattern: ESC ] params ST
	oscPattern = regexp.MustCompile(`\x1b\]([^\x07\x1b]*)([\x07]|\x1b\\)`)

	// Simple escape sequences
	simpleEscPattern = regexp.MustCompile(`\x1b([7-8DEHM])`)
)

// ProcessOutput processes terminal output and updates state
func (ts *TerminalState) ProcessOutput(data []byte) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	text := string(data)
	pos := 0

	for pos < len(text) {
		char := text[pos]

		// Handle ANSI escape sequences
		if char == '\x1b' {
			remaining := text[pos:]

			// Try CSI pattern first (most common)
			if match := csiPattern.FindStringSubmatchIndex(remaining); match != nil {
				params := remaining[match[2]:match[3]]
				command := remaining[match[4]:match[5]]
				ts.handleCSI(params, command)
				pos += match[1]
				continue
			}

			// Try OSC pattern
			if match := oscPattern.FindStringSubmatchIndex(remaining); match != nil {
				// OSC sequences don't affect state (title changes, etc)
				pos += match[1]
				continue
			}

			// Try simple escape sequences
			if match := simpleEscPattern.FindStringSubmatchIndex(remaining); match != nil {
				command := remaining[match[2]:match[3]]
				ts.handleSimpleEscape(command)
				pos += match[1]
				continue
			}

			// Unknown escape sequence, skip ESC character
			pos++
			continue
		}

		// Handle control characters
		switch char {
		case '\r': // Carriage return
			ts.CursorCol = 0
		case '\n': // Line feed (also does implicit carriage return in most terminals)
			ts.CursorCol = 0
			ts.CursorRow++
			if ts.CursorRow >= ts.Rows {
				ts.scrollUp()
				ts.CursorRow = ts.Rows - 1
			}
		case '\b': // Backspace
			if ts.CursorCol > 0 {
				ts.CursorCol--
			}
		case '\t': // Tab (advance to next tab stop, typically 8 columns)
			ts.CursorCol = ((ts.CursorCol / 8) + 1) * 8
			if ts.CursorCol >= ts.Cols {
				ts.CursorCol = ts.Cols - 1
			}
		default:
			// Printable character - add to grid
			if char >= 32 && char < 127 {
				if ts.CursorRow < ts.Rows && ts.CursorCol < ts.Cols {
					ts.Grid[ts.CursorRow][ts.CursorCol] = Cell{
						Char:  rune(char),
						Style: ts.CurrentStyle,
					}
					ts.CursorCol++
					if ts.CursorCol >= ts.Cols {
						ts.CursorCol = 0
						ts.CursorRow++
						if ts.CursorRow >= ts.Rows {
							ts.scrollUp()
							ts.CursorRow = ts.Rows - 1
						}
					}
				}
			}
		}

		pos++
	}

	// Increment version after processing output
	ts.Version++

	return nil
}

// handleCSI handles CSI escape sequences (ESC [ params letter)
func (ts *TerminalState) handleCSI(params string, command string) {
	switch command {
	case "A": // Cursor up
		n := parseIntParam(params, 1)
		ts.CursorRow -= n
		if ts.CursorRow < 0 {
			ts.CursorRow = 0
		}
	case "B": // Cursor down
		n := parseIntParam(params, 1)
		ts.CursorRow += n
		if ts.CursorRow >= ts.Rows {
			ts.CursorRow = ts.Rows - 1
		}
	case "C": // Cursor forward
		n := parseIntParam(params, 1)
		ts.CursorCol += n
		if ts.CursorCol >= ts.Cols {
			ts.CursorCol = ts.Cols - 1
		}
	case "D": // Cursor backward
		n := parseIntParam(params, 1)
		ts.CursorCol -= n
		if ts.CursorCol < 0 {
			ts.CursorCol = 0
		}
	case "H", "f": // Cursor position (row;col)
		parts := strings.Split(params, ";")
		row := 1
		col := 1
		if len(parts) >= 1 && parts[0] != "" {
			row = parseIntParam(parts[0], 1)
		}
		if len(parts) >= 2 && parts[1] != "" {
			col = parseIntParam(parts[1], 1)
		}
		// Convert from 1-based to 0-based
		ts.CursorRow = row - 1
		ts.CursorCol = col - 1
		// Clamp to valid range
		if ts.CursorRow < 0 {
			ts.CursorRow = 0
		}
		if ts.CursorRow >= ts.Rows {
			ts.CursorRow = ts.Rows - 1
		}
		if ts.CursorCol < 0 {
			ts.CursorCol = 0
		}
		if ts.CursorCol >= ts.Cols {
			ts.CursorCol = ts.Cols - 1
		}
	case "J": // Erase in display
		n := parseIntParam(params, 0)
		switch n {
		case 0: // Clear from cursor to end of screen
			ts.clearFromCursor(true)
		case 1: // Clear from beginning to cursor
			ts.clearToCursor(true)
		case 2, 3: // Clear entire screen
			ts.clearScreen()
		}
	case "K": // Erase in line
		n := parseIntParam(params, 0)
		switch n {
		case 0: // Clear from cursor to end of line
			ts.clearFromCursor(false)
		case 1: // Clear from beginning of line to cursor
			ts.clearToCursor(false)
		case 2: // Clear entire line
			ts.clearLine(ts.CursorRow)
		}
	case "m": // SGR (Select Graphic Rendition) - text styling
		ts.handleSGR(params)
	case "L": // Insert lines
		n := parseIntParam(params, 1)
		ts.insertLines(n)
	case "M": // Delete lines
		n := parseIntParam(params, 1)
		ts.deleteLines(n)
	case "h": // Set mode
		if params == "?25" {
			ts.CursorVisible = true
		}
	case "l": // Reset mode
		if params == "?25" {
			ts.CursorVisible = false
		}
	}
}

// handleSimpleEscape handles simple escape sequences
func (ts *TerminalState) handleSimpleEscape(command string) {
	switch command {
	case "7": // Save cursor position
		ts.SavedCursorRow = ts.CursorRow
		ts.SavedCursorCol = ts.CursorCol
	case "8": // Restore cursor position
		ts.CursorRow = ts.SavedCursorRow
		ts.CursorCol = ts.SavedCursorCol
		// Clamp to valid range in case of resize
		if ts.CursorRow >= ts.Rows {
			ts.CursorRow = ts.Rows - 1
		}
		if ts.CursorCol >= ts.Cols {
			ts.CursorCol = ts.Cols - 1
		}
		if ts.CursorRow < 0 {
			ts.CursorRow = 0
		}
		if ts.CursorCol < 0 {
			ts.CursorCol = 0
		}
	case "D": // Line feed
		ts.CursorRow++
		if ts.CursorRow >= ts.Rows {
			ts.scrollUp()
			ts.CursorRow = ts.Rows - 1
		}
	case "E": // Next line (CR + LF)
		ts.CursorCol = 0
		ts.CursorRow++
		if ts.CursorRow >= ts.Rows {
			ts.scrollUp()
			ts.CursorRow = ts.Rows - 1
		}
	case "H": // Set tab stop
		// TODO: Implement tab stops if needed
	case "M": // Reverse line feed
		ts.CursorRow--
		if ts.CursorRow < 0 {
			ts.scrollDown()
			ts.CursorRow = 0
		}
	}
}

// handleSGR handles SGR (Select Graphic Rendition) escape sequences
func (ts *TerminalState) handleSGR(params string) {
	if params == "" || params == "0" {
		// Reset all attributes
		ts.CurrentStyle = DefaultStyle()
		return
	}

	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		code := parseIntParam(parts[i], 0)
		switch code {
		case 0: // Reset
			ts.CurrentStyle = DefaultStyle()
		case 1: // Bold
			ts.CurrentStyle.Bold = true
		case 3: // Italic
			ts.CurrentStyle.Italic = true
		case 4: // Underline
			ts.CurrentStyle.Underline = true
		case 7: // Reverse
			ts.CurrentStyle.Reverse = true
		case 22: // Normal intensity (not bold)
			ts.CurrentStyle.Bold = false
		case 23: // Not italic
			ts.CurrentStyle.Italic = false
		case 24: // Not underlined
			ts.CurrentStyle.Underline = false
		case 27: // Not reversed
			ts.CurrentStyle.Reverse = false
		case 30, 31, 32, 33, 34, 35, 36, 37: // Foreground colors
			ts.CurrentStyle.FgColor = fmt.Sprintf("color%d", code-30)
		case 39: // Default foreground color
			ts.CurrentStyle.FgColor = ""
		case 40, 41, 42, 43, 44, 45, 46, 47: // Background colors
			ts.CurrentStyle.BgColor = fmt.Sprintf("color%d", code-40)
		case 49: // Default background color
			ts.CurrentStyle.BgColor = ""
		case 90, 91, 92, 93, 94, 95, 96, 97: // Bright foreground colors
			ts.CurrentStyle.FgColor = fmt.Sprintf("bright-color%d", code-90)
		case 100, 101, 102, 103, 104, 105, 106, 107: // Bright background colors
			ts.CurrentStyle.BgColor = fmt.Sprintf("bright-color%d", code-100)
		case 38, 48: // Extended colors (38 = fg, 48 = bg)
			// Format: 38;5;n (256 colors) or 38;2;r;g;b (RGB)
			if i+2 < len(parts) {
				if parts[i+1] == "5" && i+2 < len(parts) {
					// 256 color mode
					colorIdx := parseIntParam(parts[i+2], 0)
					if code == 38 {
						ts.CurrentStyle.FgColor = fmt.Sprintf("color-%d", colorIdx)
					} else {
						ts.CurrentStyle.BgColor = fmt.Sprintf("color-%d", colorIdx)
					}
					i += 2
				} else if parts[i+1] == "2" && i+4 < len(parts) {
					// RGB mode
					r := parseIntParam(parts[i+2], 0)
					g := parseIntParam(parts[i+3], 0)
					b := parseIntParam(parts[i+4], 0)
					if code == 38 {
						ts.CurrentStyle.FgColor = fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
					} else {
						ts.CurrentStyle.BgColor = fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
					}
					i += 4
				}
			}
		}
	}
}

// Utility functions for grid manipulation

func (ts *TerminalState) scrollUp() {
	// Remove first line and add empty line at bottom
	copy(ts.Grid[0:], ts.Grid[1:])
	ts.Grid[ts.Rows-1] = make([]Cell, ts.Cols)
	for j := 0; j < ts.Cols; j++ {
		ts.Grid[ts.Rows-1][j] = Cell{
			Char:  ' ',
			Style: DefaultStyle(),
		}
	}
}

func (ts *TerminalState) scrollDown() {
	// Insert empty line at top, remove last line
	copy(ts.Grid[1:], ts.Grid[0:ts.Rows-1])
	ts.Grid[0] = make([]Cell, ts.Cols)
	for j := 0; j < ts.Cols; j++ {
		ts.Grid[0][j] = Cell{
			Char:  ' ',
			Style: DefaultStyle(),
		}
	}
}

func (ts *TerminalState) clearScreen() {
	for i := 0; i < ts.Rows; i++ {
		ts.clearLine(i)
	}
}

func (ts *TerminalState) clearLine(row int) {
	if row >= 0 && row < ts.Rows {
		for j := 0; j < ts.Cols; j++ {
			ts.Grid[row][j] = Cell{
				Char:  ' ',
				Style: DefaultStyle(),
			}
		}
	}
}

func (ts *TerminalState) clearFromCursor(toEnd bool) {
	if toEnd {
		// Clear from cursor to end of screen
		for j := ts.CursorCol; j < ts.Cols; j++ {
			ts.Grid[ts.CursorRow][j] = Cell{Char: ' ', Style: DefaultStyle()}
		}
		for i := ts.CursorRow + 1; i < ts.Rows; i++ {
			ts.clearLine(i)
		}
	} else {
		// Clear from cursor to end of line
		for j := ts.CursorCol; j < ts.Cols; j++ {
			ts.Grid[ts.CursorRow][j] = Cell{Char: ' ', Style: DefaultStyle()}
		}
	}
}

func (ts *TerminalState) clearToCursor(fromStart bool) {
	if fromStart {
		// Clear from start of screen to cursor
		for i := 0; i < ts.CursorRow; i++ {
			ts.clearLine(i)
		}
		for j := 0; j <= ts.CursorCol && j < ts.Cols; j++ {
			ts.Grid[ts.CursorRow][j] = Cell{Char: ' ', Style: DefaultStyle()}
		}
	} else {
		// Clear from start of line to cursor
		for j := 0; j <= ts.CursorCol && j < ts.Cols; j++ {
			ts.Grid[ts.CursorRow][j] = Cell{Char: ' ', Style: DefaultStyle()}
		}
	}
}

func (ts *TerminalState) insertLines(n int) {
	// Insert n blank lines at cursor position
	if ts.CursorRow+n >= ts.Rows {
		// Inserting more lines than available, just clear from cursor to end
		for i := ts.CursorRow; i < ts.Rows; i++ {
			ts.clearLine(i)
		}
		return
	}

	// Shift lines down
	copy(ts.Grid[ts.CursorRow+n:], ts.Grid[ts.CursorRow:ts.Rows-n])

	// Clear inserted lines
	for i := 0; i < n; i++ {
		ts.Grid[ts.CursorRow+i] = make([]Cell, ts.Cols)
		for j := 0; j < ts.Cols; j++ {
			ts.Grid[ts.CursorRow+i][j] = Cell{Char: ' ', Style: DefaultStyle()}
		}
	}
}

func (ts *TerminalState) deleteLines(n int) {
	// Delete n lines starting at cursor position
	if ts.CursorRow+n >= ts.Rows {
		// Deleting more lines than available, just clear from cursor to end
		for i := ts.CursorRow; i < ts.Rows; i++ {
			ts.clearLine(i)
		}
		return
	}

	// Shift lines up
	copy(ts.Grid[ts.CursorRow:], ts.Grid[ts.CursorRow+n:])

	// Clear bottom lines
	for i := ts.Rows - n; i < ts.Rows; i++ {
		ts.clearLine(i)
	}
}

// Resize resizes the terminal state
func (ts *TerminalState) Resize(rows, cols int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if rows == ts.Rows && cols == ts.Cols {
		return
	}

	// Create new grid
	newGrid := make([][]Cell, rows)
	for i := 0; i < rows; i++ {
		newGrid[i] = make([]Cell, cols)
		for j := 0; j < cols; j++ {
			if i < ts.Rows && j < ts.Cols {
				// Copy existing content
				newGrid[i][j] = ts.Grid[i][j]
			} else {
				// Fill with empty cells
				newGrid[i][j] = Cell{
					Char:  ' ',
					Style: DefaultStyle(),
				}
			}
		}
	}

	ts.Grid = newGrid
	ts.Rows = rows
	ts.Cols = cols

	// Clamp cursor position
	if ts.CursorRow >= rows {
		ts.CursorRow = rows - 1
	}
	if ts.CursorCol >= cols {
		ts.CursorCol = cols - 1
	}

	ts.Version++
}

// GenerateDelta generates a delta from another state to this state
func (ts *TerminalState) GenerateDelta(fromState *TerminalState) *sessionv1.TerminalData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if fromState != nil {
		fromState.mu.RLock()
		defer fromState.mu.RUnlock()
	}

	delta := &sessionv1.TerminalDelta{
		FromState: 0,
		ToState:   ts.Version,
		Cursor: &sessionv1.CursorPosition{
			Row:     uint32(ts.CursorRow),
			Col:     uint32(ts.CursorCol),
			Visible: ts.CursorVisible,
		},
	}

	// Full sync if no previous state or dimensions changed
	if fromState == nil || fromState.Rows != ts.Rows || fromState.Cols != ts.Cols {
		delta.FullSync = true
		delta.FromState = 0
		delta.Dimensions = &sessionv1.TerminalDimensions{
			Rows: uint32(ts.Rows),
			Cols: uint32(ts.Cols),
		}

		// Include all lines
		delta.Lines = make([]*sessionv1.LineDelta, 0, ts.Rows)
		for i := 0; i < ts.Rows; i++ {
			lineText := ts.getLineText(i)
			delta.Lines = append(delta.Lines, &sessionv1.LineDelta{
				LineNumber: uint32(i),
				Operation: &sessionv1.LineDelta_ReplaceLine{
					ReplaceLine: []byte(lineText),
				},
			})
		}
	} else {
		// Incremental delta
		delta.FromState = fromState.Version
		delta.Lines = make([]*sessionv1.LineDelta, 0)

		// Compare line by line
		for i := 0; i < ts.Rows; i++ {
			if !ts.linesEqual(fromState, i) {
				lineText := ts.getLineText(i)
				delta.Lines = append(delta.Lines, &sessionv1.LineDelta{
					LineNumber: uint32(i),
					Operation: &sessionv1.LineDelta_ReplaceLine{
						ReplaceLine: []byte(lineText),
					},
				})
			}
		}
	}

	return &sessionv1.TerminalData{
		SessionId: "", // Will be set by caller
		Data: &sessionv1.TerminalData_Delta{
			Delta: delta,
		},
	}
}

// GenerateState generates a complete terminal state message (MOSH-style).
// This is the preferred method over GenerateDelta for robust synchronization.
func (ts *TerminalState) GenerateState() *sessionv1.TerminalData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	// Build complete terminal state with all lines
	lines := make([]*sessionv1.TerminalLine, 0, ts.Rows)
	for i := 0; i < ts.Rows; i++ {
		lineText := ts.getLineText(i)
		lines = append(lines, &sessionv1.TerminalLine{
			Content: []byte(lineText),
		})
	}

	state := &sessionv1.TerminalState{
		Sequence: ts.Version,
		Lines:    lines,
		Cursor: &sessionv1.CursorPosition{
			Row:     uint32(ts.CursorRow),
			Col:     uint32(ts.CursorCol),
			Visible: ts.CursorVisible,
		},
		Dimensions: &sessionv1.TerminalDimensions{
			Rows: uint32(ts.Rows),
			Cols: uint32(ts.Cols),
		},
	}

	return &sessionv1.TerminalData{
		SessionId: "", // Will be set by caller
		Data: &sessionv1.TerminalData_State{
			State: state,
		},
	}
}

// linesEqual checks if a line is equal between two states
func (ts *TerminalState) linesEqual(other *TerminalState, row int) bool {
	if row >= ts.Rows || row >= other.Rows {
		return false
	}

	for col := 0; col < ts.Cols && col < other.Cols; col++ {
		if ts.Grid[row][col].Char != other.Grid[row][col].Char {
			return false
		}
		// For now, ignore style differences for simplicity
		// TODO: Include style in comparison if needed
	}

	return true
}

// getLineText returns the text content of a line with ANSI codes
func (ts *TerminalState) getLineText(row int) string {
	if row >= ts.Rows {
		return ""
	}

	var sb strings.Builder
	currentStyle := DefaultStyle()

	for col := 0; col < ts.Cols; col++ {
		cell := ts.Grid[row][col]

		// Add style codes if style changed
		if cell.Style != currentStyle {
			// Reset if needed
			if currentStyle.Bold || currentStyle.Italic || currentStyle.Underline {
				sb.WriteString("\x1b[0m")
				currentStyle = DefaultStyle()
			}

			// Apply new style
			if cell.Style.Bold {
				sb.WriteString("\x1b[1m")
			}
			if cell.Style.Italic {
				sb.WriteString("\x1b[3m")
			}
			if cell.Style.Underline {
				sb.WriteString("\x1b[4m")
			}
			if cell.Style.FgColor != "" {
				// Simplified: just output color code
				// TODO: Parse and output proper ANSI color codes
			}

			currentStyle = cell.Style
		}

		sb.WriteRune(cell.Char)
	}

	// Reset style at end of line
	if currentStyle.Bold || currentStyle.Italic || currentStyle.Underline {
		sb.WriteString("\x1b[0m")
	}

	// Trim trailing spaces
	result := sb.String()
	return strings.TrimRight(result, " ")
}

// Clone creates a deep copy of the terminal state
func (ts *TerminalState) Clone() *TerminalState {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	clone := &TerminalState{
		Rows:           ts.Rows,
		Cols:           ts.Cols,
		Grid:           make([][]Cell, ts.Rows),
		CursorRow:      ts.CursorRow,
		CursorCol:      ts.CursorCol,
		CursorVisible:  ts.CursorVisible,
		SavedCursorRow: ts.SavedCursorRow,
		SavedCursorCol: ts.SavedCursorCol,
		CurrentStyle:   ts.CurrentStyle,
		Version:        ts.Version,
	}

	// Deep copy grid
	for i := 0; i < ts.Rows; i++ {
		clone.Grid[i] = make([]Cell, ts.Cols)
		copy(clone.Grid[i], ts.Grid[i])
	}

	return clone
}

// Helper function to parse integer parameter from escape sequence
func parseIntParam(param string, defaultVal int) int {
	if param == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(param)
	if err != nil {
		return defaultVal
	}
	return val
}

// CreateFullSyncDeltaFromRawContent creates a full-sync delta from raw tmux content.
// This is used when sending initial pane content to clients over WebSocket.
// The raw content is processed into a proper delta to avoid xterm.js parsing errors.
func CreateFullSyncDeltaFromRawContent(rawContent string, cursorRow, cursorCol, rows, cols int) *sessionv1.TerminalData {
	// Create temporary terminal state and process the raw content
	tempState := NewTerminalState(rows, cols)

	// Process the raw ANSI content through our terminal parser
	if err := tempState.ProcessOutput([]byte(rawContent)); err != nil {
		// Log error but continue - we want to send whatever we can
		// (logging happens in ProcessOutput)
	}

	// Override cursor position with actual tmux cursor (our processing might differ)
	tempState.CursorRow = cursorRow
	tempState.CursorCol = cursorCol
	tempState.CursorVisible = true

	// Generate full-sync delta (pass nil as fromState to force full sync)
	delta := tempState.GenerateDelta(nil)

	return delta
}
