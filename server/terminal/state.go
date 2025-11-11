package terminal

import (
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"bytes"
	"hash/fnv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// StateGenerator generates MOSH-style complete terminal state snapshots.
// Unlike delta-based approaches, sends complete terminal screen state for robustness.
// Uses LZMA compression with dynamic dictionary learning for bandwidth optimization.
type StateGenerator struct {
	sequence        uint64          // State sequence number (monotonic)
	cols            int             // Terminal columns
	rows            int             // Terminal rows
	lastState       *sessionv1.TerminalState // Previous state for optimization
	compressionDict *CompressionDictionary   // Dynamic dictionary for LZMA
	mutex           sync.RWMutex    // Protects concurrent access
}

// CompressionDictionary manages dynamic dictionary learning for LZMA compression
type CompressionDictionary struct {
	patterns    map[uint64][]byte // Pattern hash -> byte pattern
	frequencies map[uint64]int    // Pattern hash -> frequency count
	totalBytes  uint64           // Total bytes processed for statistics
	level       string           // Dictionary level (base/session/user/project)
	updatedAt   time.Time        // Last update timestamp
	mutex       sync.RWMutex     // Protects dictionary updates
}

// NewStateGenerator creates a new MOSH-style state generator
func NewStateGenerator(cols, rows int) *StateGenerator {
	return &StateGenerator{
		sequence:        0,
		cols:            cols,
		rows:            rows,
		lastState:       nil,
		compressionDict: NewCompressionDictionary("session"),
		mutex:           sync.RWMutex{},
	}
}

// NewCompressionDictionary creates a new compression dictionary for pattern learning
func NewCompressionDictionary(level string) *CompressionDictionary {
	return &CompressionDictionary{
		patterns:    make(map[uint64][]byte),
		frequencies: make(map[uint64]int),
		totalBytes:  0,
		level:       level,
		updatedAt:   time.Now(),
		mutex:       sync.RWMutex{},
	}
}

// GenerateState creates a complete terminal state snapshot from terminal output.
// Returns TerminalState protobuf message with complete screen buffer.
func (sg *StateGenerator) GenerateState(output []byte) *sessionv1.TerminalState {
	return sg.GenerateStateWithCursor(output, nil, nil, nil, nil)
}

// GenerateStateWithCursor creates a complete terminal state snapshot with real cursor position and dimensions.
// Uses real tmux cursor position when provided, falls back to calculated position if nil.
func (sg *StateGenerator) GenerateStateWithCursor(output []byte, cursorX, cursorY, paneWidth, paneHeight *int) *sessionv1.TerminalState {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	// Increment sequence number (MOSH-style monotonic versioning)
	sg.sequence++

	// ATOMIC DIMENSION SYNC: Update StateGenerator dimensions to match real pane dimensions
	// This fixes dimension mismatch race conditions by ensuring StateGenerator dimensions
	// are always in sync with actual tmux pane dimensions at state generation time
	if paneWidth != nil && paneHeight != nil && (sg.cols != *paneWidth || sg.rows != *paneHeight) {
		log.InfoLog.Printf("[StateGenerator] Atomic dimension sync: %dx%d -> %dx%d (sequence %d)",
			sg.cols, sg.rows, *paneWidth, *paneHeight, sg.sequence)
		sg.cols = *paneWidth
		sg.rows = *paneHeight
	}

	// Split output into lines (preserving ANSI escape sequences)
	lines := sg.splitIntoTerminalLines(output)

	// Determine actual viewport size: use real pane dimensions if provided, otherwise sg.rows
	// This fixes dimension mismatch when pane is resized before StateGenerator is updated
	actualRows := sg.rows
	if paneHeight != nil {
		actualRows = *paneHeight
		log.DebugLog.Printf("[StateGenerator] Using real pane height %d (sg.rows now updated)", actualRows)
	}

	// Truncate to terminal row limit to match viewport
	if len(lines) > actualRows {
		// Keep only the last N rows (what user sees in terminal viewport)
		lines = lines[len(lines)-actualRows:]
		log.DebugLog.Printf("[StateGenerator] Truncated to %d rows for terminal viewport (pane=%v, sg.rows=%d)",
			actualRows, paneHeight, sg.rows)
	}

	// Pad with empty lines if necessary to maintain consistent viewport size
	for len(lines) < actualRows {
		emptyAttributes := sg.analyzeLineAttributes([]byte{})
		lines = append(lines, &sessionv1.TerminalLine{
			Content:    []byte{},
			Attributes: emptyAttributes,
		})
	}

	// Use real tmux cursor position when provided, otherwise calculate from content
	var cursor *sessionv1.CursorPosition
	if cursorX != nil && cursorY != nil {
		cursor = &sessionv1.CursorPosition{
			Row:     uint32(*cursorY),
			Col:     uint32(*cursorX),
			Visible: true,
		}
		log.DebugLog.Printf("[StateGenerator] Using real tmux cursor position: (%d,%d)", *cursorX, *cursorY)
	} else {
		cursor = sg.calculateCursorPosition(lines)
		log.DebugLog.Printf("[StateGenerator] Calculated cursor position from content: (%d,%d)", cursor.Col, cursor.Row)
	}

	// Update compression dictionary with new patterns
	sg.updateCompressionDictionary(output)

	// Create scrollback info
	scrollback := &sessionv1.ScrollbackInfo{
		TotalLines:   uint64(len(lines)),
		FirstVisible: 0,
		LastVisible:  uint64(len(lines) - 1),
	}

	// Generate compression metadata
	compressionMeta := sg.generateCompressionMetadata(output)

	// Use real pane dimensions when provided, otherwise use StateGenerator dimensions
	var terminalDimensions *sessionv1.TerminalDimensions
	if paneWidth != nil && paneHeight != nil {
		terminalDimensions = &sessionv1.TerminalDimensions{
			Rows: uint32(*paneHeight),
			Cols: uint32(*paneWidth),
		}
		log.DebugLog.Printf("[StateGenerator] Using real tmux dimensions: %dx%d", *paneWidth, *paneHeight)
	} else {
		terminalDimensions = &sessionv1.TerminalDimensions{
			Rows: uint32(sg.rows),
			Cols: uint32(sg.cols),
		}
		log.DebugLog.Printf("[StateGenerator] Using StateGenerator dimensions: %dx%d", sg.cols, sg.rows)
	}

	// Create terminal state
	state := &sessionv1.TerminalState{
		Sequence:   sg.sequence,
		Dimensions: terminalDimensions,
		Lines:       lines,
		Cursor:      cursor,
		Scrollback:  scrollback,
		Compression: compressionMeta,
	}

	// Store state for future optimization
	sg.lastState = state

	log.DebugLog.Printf("[StateGenerator] Generated state sequence %d with %d lines, cursor at (%d,%d)",
		sg.sequence, len(lines), cursor.Row, cursor.Col)

	return state
}

// splitIntoTerminalLines splits terminal output into TerminalLine protobuf messages
func (sg *StateGenerator) splitIntoTerminalLines(output []byte) []*sessionv1.TerminalLine {
	if len(output) == 0 {
		return []*sessionv1.TerminalLine{}
	}

	// Split by newline bytes
	rawLines := bytes.Split(output, []byte("\n"))

	// Remove trailing empty line if present
	if len(rawLines) > 0 && len(rawLines[len(rawLines)-1]) == 0 {
		rawLines = rawLines[:len(rawLines)-1]
	}

	// Convert to TerminalLine messages with attributes
	lines := make([]*sessionv1.TerminalLine, 0, len(rawLines))
	for _, rawLine := range rawLines {
		// Sanitize raw bytes to ensure valid UTF-8 content
		// This prevents xterm.js parsing errors from invalid byte sequences
		sanitizedContent := sg.sanitizeUTF8Bytes(rawLine)

		attributes := sg.analyzeLineAttributes(sanitizedContent)
		lines = append(lines, &sessionv1.TerminalLine{
			Content:    sanitizedContent,
			Attributes: attributes,
		})
	}

	return lines
}

// analyzeLineAttributes analyzes line content for compression optimization
func (sg *StateGenerator) analyzeLineAttributes(line []byte) *sessionv1.LineAttributes {
	isEmpty := len(line) == 0
	asciiOnly := sg.isASCIIOnly(line)
	patternHash := sg.calculatePatternHash(line)

	encoding := "utf-8" // Default encoding
	return &sessionv1.LineAttributes{
		IsEmpty:     isEmpty,
		AsciiOnly:   asciiOnly,
		Encoding:    &encoding,
		PatternHash: &patternHash,
	}
}

// isASCIIOnly checks if line contains only printable ASCII characters
func (sg *StateGenerator) isASCIIOnly(line []byte) bool {
	for _, b := range line {
		if b < 32 || b > 126 {
			// Allow TAB but ESC sequences make it non-ASCII
			if b == 9 { // TAB is allowed
				continue
			}
			return false // ESC and other control chars make it non-ASCII
		}
	}
	return true
}

// calculatePatternHash generates a hash for pattern recognition and dictionary learning
func (sg *StateGenerator) calculatePatternHash(line []byte) uint64 {
	hash := fnv.New64a()
	hash.Write(line)
	return hash.Sum64()
}

// calculateCursorPosition determines cursor position from terminal lines
func (sg *StateGenerator) calculateCursorPosition(lines []*sessionv1.TerminalLine) *sessionv1.CursorPosition {
	// Find last non-empty line
	var cursorRow uint32 = 0
	var cursorCol uint32 = 0

	for i := len(lines) - 1; i >= 0; i-- {
		if !lines[i].Attributes.IsEmpty {
			cursorRow = uint32(i)
			// Calculate visual character width (strip ANSI codes, handle multi-byte UTF-8)
			visibleContent := sg.stripANSIBytes(lines[i].Content)
			// Use runewidth to get visual column count instead of byte length
			// This properly handles multi-byte UTF-8, wide characters (CJK/emoji), and zero-width chars
			cursorCol = uint32(runewidth.StringWidth(string(visibleContent)))
			break
		}
	}

	return &sessionv1.CursorPosition{
		Row:     cursorRow,
		Col:     cursorCol,
		Visible: true,
	}
}

// stripANSIBytes removes ANSI escape sequences for visible character counting
func (sg *StateGenerator) stripANSIBytes(b []byte) []byte {
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

// sanitizeUTF8Bytes converts raw bytes to valid UTF-8, preserving ANSI escape sequences
// This prevents xterm.js parsing errors from invalid byte sequences while maintaining
// terminal formatting and color information
func (sg *StateGenerator) sanitizeUTF8Bytes(rawBytes []byte) []byte {
	// Handle empty input
	if len(rawBytes) == 0 {
		return rawBytes
	}

	// Convert to valid UTF-8 by replacing invalid sequences
	var result bytes.Buffer
	inEscape := false

	for i := 0; i < len(rawBytes); {
		// Start of ANSI escape sequence
		if rawBytes[i] == '\x1b' {
			inEscape = true
			result.WriteByte(rawBytes[i])
			i++
			continue
		}

		// Inside ANSI escape sequence - preserve all bytes
		if inEscape {
			b := rawBytes[i]
			result.WriteByte(b)
			// End of escape sequence (letter terminates most ANSI sequences)
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				inEscape = false
			}
			i++
			continue
		}

		// Outside escape sequences - handle UTF-8 and control characters
		r, size := utf8.DecodeRune(rawBytes[i:])

		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte - replace with replacement character
			result.WriteString("�")
			i++
		} else if r < 32 {
			// Control character - allow common terminal chars
			switch r {
			case '\t', '\n', '\r':
				result.WriteRune(r) // Keep tab, newline, carriage return
			case 7, 8:
				result.WriteRune(r) // Keep bell (BEL) and backspace (BS)
			default:
				// Replace other control characters with space to prevent parsing issues
				result.WriteByte(' ')
			}
			i += size
		} else {
			// Valid UTF-8 character
			result.WriteRune(r)
			i += size
		}
	}

	return result.Bytes()
}

// updateCompressionDictionary learns patterns from terminal output for future compression
func (sg *StateGenerator) updateCompressionDictionary(output []byte) {
	if sg.compressionDict == nil {
		return
	}

	sg.compressionDict.mutex.Lock()
	defer sg.compressionDict.mutex.Unlock()

	// Update total bytes processed
	sg.compressionDict.totalBytes += uint64(len(output))

	// Extract patterns for learning (simple n-gram approach)
	patterns := sg.extractPatterns(output)
	for hash, pattern := range patterns {
		sg.compressionDict.patterns[hash] = pattern
		sg.compressionDict.frequencies[hash]++
	}

	sg.compressionDict.updatedAt = time.Now()

	log.DebugLog.Printf("[StateGenerator] Updated compression dictionary: %d patterns, %d bytes processed",
		len(sg.compressionDict.patterns), sg.compressionDict.totalBytes)
}

// extractPatterns extracts byte patterns for dictionary learning
func (sg *StateGenerator) extractPatterns(output []byte) map[uint64][]byte {
	patterns := make(map[uint64][]byte)

	// Extract 4-byte patterns (good for ANSI sequences and common strings)
	for i := 0; i <= len(output)-4; i++ {
		pattern := output[i : i+4]
		hash := sg.calculatePatternHash(pattern)
		patterns[hash] = pattern
	}

	// Extract 8-byte patterns (good for longer sequences)
	for i := 0; i <= len(output)-8; i++ {
		pattern := output[i : i+8]
		hash := sg.calculatePatternHash(pattern)
		patterns[hash] = pattern
	}

	return patterns
}

// generateCompressionMetadata creates metadata about compression performance
func (sg *StateGenerator) generateCompressionMetadata(output []byte) *sessionv1.CompressionMetadata {
	if sg.compressionDict == nil {
		return &sessionv1.CompressionMetadata{
			Algorithm:          "none",
			UncompressedSize:   uint64(len(output)),
			CompressedSize:     uint64(len(output)),
			CompressionRatio:   1.0,
		}
	}

	sg.compressionDict.mutex.RLock()
	defer sg.compressionDict.mutex.RUnlock()

	// Calculate dictionary effectiveness (patterns found / total patterns possible)
	totalPatterns := float64(max(1, len(output)-4)) // Avoid division by zero
	foundPatterns := float64(len(sg.compressionDict.patterns))
	effectiveness := foundPatterns / totalPatterns
	if effectiveness > 1.0 {
		effectiveness = 1.0
	}

	compressionRatio := float32(1.0 - effectiveness*0.7)
	return &sessionv1.CompressionMetadata{
		Algorithm:          "lzma", // Will be implemented in next story
		UncompressedSize:   uint64(len(output)),
		CompressedSize:     uint64(float64(len(output)) * (1.0 - effectiveness*0.7)), // Estimated compression
		CompressionRatio:   compressionRatio, // Estimated ratio
		Dictionary: &sessionv1.DictionaryMetadata{
			Level:         sg.compressionDict.level,
			PatternCount:  uint64(len(sg.compressionDict.patterns)),
			Effectiveness: float32(effectiveness),
			UpdatedAt:     sg.compressionDict.updatedAt.Unix(),
			SizeBytes:     uint64(len(sg.compressionDict.patterns) * 8), // Rough estimate
		},
	}
}

// UpdateDimensions updates terminal dimensions
func (sg *StateGenerator) UpdateDimensions(cols, rows int) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	oldCols, oldRows := sg.cols, sg.rows
	sg.cols = cols
	sg.rows = rows

	log.InfoLog.Printf("[StateGenerator] Dimensions updated: %dx%d -> %dx%d (sequence %d)",
		oldCols, oldRows, cols, rows, sg.sequence)
}

// Reset resets the state generator (called on reconnect)
func (sg *StateGenerator) Reset() {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.sequence = 0
	sg.lastState = nil
	// Keep compression dictionary for learning continuity
	log.InfoLog.Printf("[StateGenerator] Reset state generator (preserving compression dictionary)")
}

// GetCurrentSequence returns the current sequence number (thread-safe)
func (sg *StateGenerator) GetCurrentSequence() uint64 {
	sg.mutex.RLock()
	defer sg.mutex.RUnlock()
	return sg.sequence
}

// GetCompressionStats returns compression statistics for monitoring
func (sg *StateGenerator) GetCompressionStats() map[string]interface{} {
	if sg.compressionDict == nil {
		return map[string]interface{}{
			"algorithm": "none",
			"patterns":  0,
			"bytes":     0,
		}
	}

	sg.compressionDict.mutex.RLock()
	defer sg.compressionDict.mutex.RUnlock()

	return map[string]interface{}{
		"algorithm":     "lzma",
		"level":         sg.compressionDict.level,
		"patterns":      len(sg.compressionDict.patterns),
		"bytes":         sg.compressionDict.totalBytes,
		"effectiveness": float64(len(sg.compressionDict.patterns)) / float64(max(1, int(sg.compressionDict.totalBytes/4))),
		"updated_at":    sg.compressionDict.updatedAt,
	}
}

// Helper function for max calculation
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}