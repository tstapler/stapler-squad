package terminal

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"bytes"
)

// DeltaGenerator generates MOSH-style delta compression for terminal output.
// Tracks terminal state and produces TerminalDelta messages containing only changed lines.
// Reduces bandwidth by 70-90% for typical terminal usage (especially animations).
type DeltaGenerator struct {
	lastLines            *LineRingBuffer // Previous terminal state (ring buffer for O(1) operations)
	version              uint64          // State version counter for synchronization
	cols                 int             // Terminal columns (for line wrapping)
	rows                 int             // Terminal rows (viewport size)
	deltasSinceFullSync  int             // Count of deltas since last full sync (MOSH-inspired)
	fullSyncInterval     int             // Send full sync every N deltas (default: 50)
}

// NewDeltaGenerator creates a new delta generator with initial dimensions.
func NewDeltaGenerator(cols, rows int) *DeltaGenerator {
	return &DeltaGenerator{
		lastLines:           NewLineRingBuffer(rows), // Fixed-size ring buffer for viewport
		version:             0,
		cols:                cols,
		rows:                rows,
		deltasSinceFullSync: 0,
		fullSyncInterval:    50, // Send full sync every 50 deltas (MOSH-inspired self-healing)
	}
}

// GenerateDelta compares new output with previous state and generates a delta.
// Returns TerminalDelta protobuf message with only changed lines.
func (dg *DeltaGenerator) GenerateDelta(output []byte) *sessionv1.TerminalDelta {
	// Split output into lines (preserving ANSI escape sequences)
	newLines := splitIntoBytesLines(output)

	// CRITICAL FIX: Truncate lines to terminal row limit to prevent out-of-bounds line numbers
	// This prevents generating deltas with line numbers beyond the terminal viewport
	// when buffered output from a larger terminal size is processed after a resize
	if len(newLines) > dg.rows {
		originalLineCount := len(newLines)
		// Keep only the last N rows (most recent output, what user sees in viewport)
		newLines = newLines[len(newLines)-dg.rows:]
		log.DebugLog.Printf("[DeltaGenerator] Truncated %d lines to fit %d row terminal (kept last %d lines)",
			originalLineCount, dg.rows, len(newLines))
	}

	// Increment version
	fromVersion := dg.version
	dg.version++

	// MOSH-inspired: Periodically send full sync for self-healing
	dg.deltasSinceFullSync++
	shouldSendFullSync := dg.deltasSinceFullSync >= dg.fullSyncInterval

	if shouldSendFullSync {
		log.InfoLog.Printf("[DeltaGenerator] Sending periodic full sync (after %d deltas)", dg.deltasSinceFullSync)
		return dg.GenerateFullSync(output, dg.cols, dg.rows)
	}

	// Generate line deltas
	lineDeltas := make([]*sessionv1.LineDelta, 0)

	// Compare line by line and generate deltas for changes
	maxLines := len(newLines)
	if dg.lastLines.Size() > maxLines {
		maxLines = dg.lastLines.Size()
	}

	for i := 0; i < maxLines; i++ {
		var newLine []byte
		var oldLine []byte

		if i < len(newLines) {
			newLine = newLines[i]
		}
		if i < dg.lastLines.Size() {
			oldLine = dg.lastLines.Get(i)
		}

		// Line changed - generate delta
		if !bytes.Equal(newLine, oldLine) {
			if len(newLine) == 0 {
				// Line cleared
				lineDeltas = append(lineDeltas, &sessionv1.LineDelta{
					LineNumber: uint32(i),
					Operation: &sessionv1.LineDelta_ClearLine{
						ClearLine: true,
					},
				})
			} else {
				// Line replaced
				lineDeltas = append(lineDeltas, &sessionv1.LineDelta{
					LineNumber: uint32(i),
					Operation: &sessionv1.LineDelta_ReplaceLine{
						ReplaceLine: newLine,
					},
				})
			}
		}
	}

	// Update state using ring buffer (O(N) but no allocations)
	dg.lastLines.SetAll(newLines)

	// Extract cursor position (simplified - assumes cursor at end)
	// In production, would parse ANSI cursor position sequences
	var cursorRow uint32 = 0
	if len(newLines) > 0 {
		cursorRow = uint32(len(newLines) - 1)
	}
	cursorCol := uint32(0)
	if len(newLines) > 0 {
		// Count visible characters in last line (strip ANSI)
		lastLine := newLines[len(newLines)-1]
		cursorCol = uint32(len(stripANSIBytes(lastLine)))
	}

	return &sessionv1.TerminalDelta{
		FromState: fromVersion,
		ToState:   dg.version,
		Lines:     lineDeltas,
		Cursor: &sessionv1.CursorPosition{
			Row:     cursorRow,
			Col:     cursorCol,
			Visible: true,
		},
		FullSync: false,
		Dimensions: &sessionv1.TerminalDimensions{
			Rows: uint32(dg.rows),
			Cols: uint32(dg.cols),
		},
	}
}

// GenerateFullSync generates a full state sync (used for initial load or recovery).
// Sends complete terminal state as a delta with full_sync=true.
func (dg *DeltaGenerator) GenerateFullSync(output []byte, cols, rows int) *sessionv1.TerminalDelta {
	// Update dimensions
	dg.cols = cols
	dg.rows = rows

	// Split output into lines
	newLines := splitIntoBytesLines(output)

	// Truncate to terminal row limit to prevent out-of-bounds line numbers
	// This can happen when tmux captures entire scrollback (e.g., 100 lines)
	// but the terminal only has N rows (e.g., 73 rows -> valid indices 0-72)
	if len(newLines) > rows {
		originalLineCount := len(newLines)
		// Keep only the last N rows (most recent output)
		// This matches what the user sees in their terminal viewport
		newLines = newLines[len(newLines)-rows:]
		log.DebugLog.Printf("[DeltaGenerator] Truncated %d lines to fit %d row terminal (kept last %d lines)",
			originalLineCount, rows, len(newLines))
	}

	// Reset version and full sync counter
	dg.version = 1
	dg.deltasSinceFullSync = 0 // Reset counter after full sync

	// Generate line deltas for all lines
	lineDeltas := make([]*sessionv1.LineDelta, 0, len(newLines))
	for i, line := range newLines {
		lineDeltas = append(lineDeltas, &sessionv1.LineDelta{
			LineNumber: uint32(i),
			Operation: &sessionv1.LineDelta_ReplaceLine{
				ReplaceLine: line,
			},
		})
	}

	// Update state using ring buffer (handles truncation automatically)
	dg.lastLines.SetAll(newLines)

	// Cursor position (fixed: prevent uint32 overflow when len(newLines) == 0)
	var cursorRow uint32 = 0
	if len(newLines) > 0 {
		cursorRow = uint32(len(newLines) - 1)
	}
	cursorCol := uint32(0)
	if len(newLines) > 0 {
		lastLine := newLines[len(newLines)-1]
		cursorCol = uint32(len(stripANSIBytes(lastLine)))
	}

	return &sessionv1.TerminalDelta{
		FromState: 0,
		ToState:   dg.version,
		Lines:     lineDeltas,
		Cursor: &sessionv1.CursorPosition{
			Row:     cursorRow,
			Col:     cursorCol,
			Visible: true,
		},
		FullSync: true,
		Dimensions: &sessionv1.TerminalDimensions{
			Rows: uint32(rows),
			Cols: uint32(cols),
		},
	}
}

// UpdateDimensions updates terminal dimensions (called on resize).
func (dg *DeltaGenerator) UpdateDimensions(cols, rows int) {
	oldCols, oldRows := dg.cols, dg.rows
	dg.cols = cols
	dg.rows = rows
	// Update ring buffer capacity to match new viewport size
	dg.lastLines.UpdateDimensions(rows)

	// Log dimension changes for debugging resize-related issues
	log.InfoLog.Printf("[DeltaGenerator] Dimensions updated: %dx%d -> %dx%d (version %d)",
		oldCols, oldRows, cols, rows, dg.version)
}

// Reset resets the delta generator state (called on reconnect).
func (dg *DeltaGenerator) Reset() {
	dg.lastLines.Clear()
	dg.version = 0
	dg.deltasSinceFullSync = 0 // Reset full sync counter on reconnect
}

// splitIntoBytesLines splits terminal output into lines, preserving ANSI escape sequences.
// Each line includes its ANSI formatting codes as raw bytes.
func splitIntoBytesLines(output []byte) [][]byte {
	if len(output) == 0 {
		return [][]byte{}
	}

	// Split by newline byte
	lines := bytes.Split(output, []byte("\n"))

	// Remove trailing empty line if present
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// stripANSIBytes removes ANSI escape sequences from raw bytes.
// Used for calculating cursor position based on visible characters.
func stripANSIBytes(b []byte) []byte {
	var result bytes.Buffer
	inEscape := false

	for i := 0; i < len(b); i++ {
		if b[i] == '\x1b' {
			inEscape = true
			continue
		}

		if inEscape {
			// End of escape sequence
			if b[i] >= 'A' && b[i] <= 'Z' || b[i] >= 'a' && b[i] <= 'z' {
				inEscape = false
			}
			continue
		}

		result.WriteByte(b[i])
	}

	return result.Bytes()
}
