// Package analytics provides terminal escape code extraction and analysis
package analytics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// EscapeCategory represents the type of escape sequence
type EscapeCategory string

const (
	CategoryCSI     EscapeCategory = "CSI"     // Control Sequence Introducer \x1b[
	CategoryOSC     EscapeCategory = "OSC"     // Operating System Command \x1b]
	CategoryDCS     EscapeCategory = "DCS"     // Device Control String \x1bP
	CategoryPM      EscapeCategory = "PM"      // Privacy Message \x1b^
	CategoryAPC     EscapeCategory = "APC"     // Application Program Command \x1b_
	CategorySOS     EscapeCategory = "SOS"     // Start of String \x1bX
	CategoryC1      EscapeCategory = "C1"      // C1 control codes
	CategorySimple  EscapeCategory = "Simple"  // Simple 2-char escapes
	CategoryDECPriv EscapeCategory = "DECPriv" // DEC Private modes \x1b[?
	CategorySGR     EscapeCategory = "SGR"     // Select Graphic Rendition (colors/styles)
	CategoryCursor  EscapeCategory = "Cursor"  // Cursor positioning
	CategoryErase   EscapeCategory = "Erase"   // Screen/line erase
	CategoryScroll  EscapeCategory = "Scroll"  // Scroll region
	CategoryCharset EscapeCategory = "Charset" // Character set selection
	CategoryUnknown EscapeCategory = "Unknown" // Unknown sequence
)

// ParsedEscapeCode represents a single parsed escape sequence
type ParsedEscapeCode struct {
	RawBytes    []byte         // Original bytes
	HexEncoded  string         // Hex representation for display
	Category    EscapeCategory // Type of sequence
	Description string         // Human-readable description
	StartOffset int            // Position in original data
	EndOffset   int            // End position in original data
}

// EscapeCodeParser extracts escape sequences from terminal output
type EscapeCodeParser struct {
	store             *EscapeCodeStore
	sessionID         string
	enabled           bool
	partialBuffer     []byte // Buffer for partial escape sequences between chunks
	writer            EscapeEventWriter
	captureLevel      string // "full", "summary", "off"
	redactOSCPayloads bool
	samplingRate      float64
	chunkSeqNum       int64 // incremented per Parse call (Stage 1)
	stage2ChunkSeqNum int64 // incremented per ParseStage2 call (Stage 2, independent counter)
	correlator        *MangleCorrelator
	totalSequences    int64 // total escape sequences emitted
	totalMangled      int64 // total sequences flagged as mangled
}

// ParserStats holds lifetime counters for a parser session.
type ParserStats struct {
	TotalSequences int64
	TotalMangled   int64
	Dropped        int64
}

// GetStats returns lifetime counters for this parser.
func (p *EscapeCodeParser) GetStats() ParserStats {
	return ParserStats{
		TotalSequences: p.totalSequences,
		TotalMangled:   p.totalMangled,
	}
}

// NewEscapeCodeParser creates a new parser with the given store and session ID
func NewEscapeCodeParser(store *EscapeCodeStore, sessionID string) *EscapeCodeParser {
	return &EscapeCodeParser{
		store:         store,
		sessionID:     sessionID,
		enabled:       false,
		partialBuffer: nil,
	}
}

// SetEventWriter configures the event writer and capture settings.
func (p *EscapeCodeParser) SetEventWriter(w EscapeEventWriter, captureLevel string, redactOSC bool, samplingRate float64) {
	p.writer = w
	p.captureLevel = captureLevel
	p.redactOSCPayloads = redactOSC
	p.samplingRate = samplingRate
}

// SetCorrelator attaches a MangleCorrelator to this parser for Stage 1/2 mangle detection.
func (p *EscapeCodeParser) SetCorrelator(c *MangleCorrelator) {
	p.correlator = c
}

// SetEnabled enables or disables escape code parsing
func (p *EscapeCodeParser) SetEnabled(enabled bool) {
	p.enabled = enabled
}

// IsEnabled returns whether parsing is enabled
func (p *EscapeCodeParser) IsEnabled() bool {
	return p.enabled
}

// Parse extracts all escape sequences from data and records them to the store.
// sessionSeq is the cumulative PTY byte offset at the start of this chunk.
// Returns the original data unchanged (passthrough).
func (p *EscapeCodeParser) Parse(data []byte, sessionSeq int64) []byte {
	p.chunkSeqNum++

	if !p.enabled || p.store == nil || len(data) == 0 {
		return data
	}

	// Prepend any partial buffer from previous chunk
	var parseData []byte
	if len(p.partialBuffer) > 0 {
		parseData = make([]byte, len(p.partialBuffer)+len(data))
		copy(parseData, p.partialBuffer)
		copy(parseData[len(p.partialBuffer):], data)
		p.partialBuffer = nil
	} else {
		parseData = data
	}

	// Extract all escape sequences
	codes := p.extractEscapeSequences(parseData)

	// Record each code to the store and emit events
	for _, code := range codes {
		p.store.Record(p.sessionID, code.RawBytes, code.Category, code.Description)
		p.emitEvent(code, sessionSeq)
	}

	// Check if data ends with a partial escape sequence
	partial := p.findPartialEscapeAtEnd(parseData)
	if len(partial) > 0 {
		p.partialBuffer = partial
	}

	return data
}

// ParseStage2 performs a secondary parse pass over data (a coalesced transport frame)
// and emits EscapeEventRecord entries with Stage=StageTransport.
// sessionSeq is the cumulative transport byte offset at the start of this frame.
// ParseStage2 uses its own independent chunk counter (stage2ChunkSeqNum) for sampling
// so that calling it independently does not interfere with the Stage 1 counter.
func (p *EscapeCodeParser) ParseStage2(data []byte, sessionSeq int64) {
	if !p.enabled || p.writer == nil || p.captureLevel == "off" || len(data) == 0 {
		return
	}

	p.stage2ChunkSeqNum++

	codes := p.extractEscapeSequences(data)
	for _, code := range codes {
		p.emitEventWithStageAndSeq(code, sessionSeq, StageTransport, p.stage2ChunkSeqNum)
	}
}

// emitEvent sends an EscapeEventRecord to the writer if configured.
func (p *EscapeCodeParser) emitEvent(code ParsedEscapeCode, sessionSeq int64) {
	p.emitEventWithStageAndSeq(code, sessionSeq, StagePTYRead, p.chunkSeqNum)
}

// emitEventWithStageAndSeq sends an EscapeEventRecord with a specified stage and explicit chunk counter.
// The chunkSeq parameter is used for sampling decisions.
func (p *EscapeCodeParser) emitEventWithStageAndSeq(code ParsedEscapeCode, sessionSeq int64, stage Stage, chunkSeq int64) {
	if p.writer == nil || p.captureLevel == "off" {
		return
	}

	// Apply sampling: use the provided chunkSeq with modulo check
	if p.samplingRate < 1.0 && (chunkSeq%1000) >= int64(p.samplingRate*1000) {
		return
	}

	// Determine subtype from description (first word)
	subtype := code.Description
	if idx := strings.IndexByte(subtype, ' '); idx >= 0 {
		subtype = subtype[:idx]
	}

	record := EscapeEventRecord{
		SessionID:       p.sessionID,
		Stage:           stage,
		SequenceType:    string(code.Category),
		SequenceSubtype: subtype,
		ByteLen:         len(code.RawBytes),
		WallTime:        time.Now(),
		SessionSeq:      sessionSeq + int64(code.StartOffset),
	}

	// Compute payload hash — FNV-64a for summary (fast), SHA-256 for full (collision-resistant)
	switch p.captureLevel {
	case "full":
		h := sha256.Sum256(code.RawBytes)
		record.PayloadHash = hex.EncodeToString(h[:])[:16]
		record.RawBytes = code.RawBytes
	case "summary":
		h := fnv.New64a()
		h.Write(code.RawBytes)
		record.PayloadHash = fmt.Sprintf("%016x", h.Sum64())
	}

	// Apply OSC redaction
	if code.Category == CategoryOSC && p.redactOSCPayloads {
		cmd := extractOSCCommand(code.RawBytes)
		switch cmd {
		case "52":
			record.PayloadHash = ""
			record.RawBytes = nil
			record.SequenceSubtype = "clipboard"
		case "0", "1", "2", "7":
			record.PayloadHash = ""
			record.RawBytes = nil
		}
	}

	// Record Stage 1 observation for mangle correlation
	if p.correlator != nil && record.PayloadHash != "" {
		p.correlator.RecordStage1(record.SessionID, record.SessionSeq, record.PayloadHash, record.ByteLen)
	}

	p.totalSequences++
	if record.Mangled {
		p.totalMangled++
	}

	p.writer.WriteEscapeEvent(context.Background(), record)
}

// extractOSCCommand extracts the OSC command number string from raw OSC bytes.
// Raw bytes are: ESC ] <cmd> ; <payload> <terminator>
func extractOSCCommand(rawBytes []byte) string {
	if len(rawBytes) < 4 {
		return ""
	}
	// Skip ESC and ]
	content := rawBytes[2:]
	// Find ';' or terminator
	end := bytes.IndexAny(content, ";\x07\x9c")
	if end < 0 {
		// Check for ESC \ (ST)
		for i, b := range content {
			if b == 0x1b {
				end = i
				break
			}
		}
		if end < 0 {
			end = len(content)
		}
	}
	return string(content[:end])
}

// extractEscapeSequences finds all escape sequences in the data
func (p *EscapeCodeParser) extractEscapeSequences(data []byte) []ParsedEscapeCode {
	var codes []ParsedEscapeCode
	i := 0

	for i < len(data) {
		// Look for ESC character (0x1b)
		if data[i] != 0x1b {
			i++
			continue
		}

		// Found an ESC, try to parse a complete sequence
		code, consumed := p.parseSequenceAt(data, i)
		if consumed > 0 && code != nil {
			codes = append(codes, *code)
			i += consumed
		} else {
			// Not a valid sequence or incomplete, skip the ESC
			i++
		}
	}

	return codes
}

// parseSequenceAt attempts to parse an escape sequence starting at offset
// Returns the parsed code and number of bytes consumed
func (p *EscapeCodeParser) parseSequenceAt(data []byte, offset int) (*ParsedEscapeCode, int) {
	if offset >= len(data) || data[offset] != 0x1b {
		return nil, 0
	}

	// Need at least 2 bytes for any escape sequence
	if offset+1 >= len(data) {
		return nil, 0
	}

	secondByte := data[offset+1]

	switch secondByte {
	case '[': // CSI sequence
		return p.parseCSI(data, offset)
	case ']': // OSC sequence
		return p.parseOSC(data, offset)
	case 'P': // DCS sequence
		return p.parseStringSequence(data, offset, CategoryDCS, "Device Control String")
	case '^': // PM sequence
		return p.parseStringSequence(data, offset, CategoryPM, "Privacy Message")
	case '_': // APC sequence
		return p.parseStringSequence(data, offset, CategoryAPC, "Application Program Command")
	case 'X': // SOS sequence
		return p.parseStringSequence(data, offset, CategorySOS, "Start of String")
	case '(', ')', '*', '+': // Character set designation
		return p.parseCharset(data, offset)
	case '7', '8': // Save/restore cursor
		return p.parseSimpleEscape(data, offset)
	case 'D', 'E', 'H', 'M', 'N', 'O', 'Z', 'c': // Other simple escapes
		return p.parseSimpleEscape(data, offset)
	default:
		// Check for C1 control codes (0x40-0x5F range for second byte)
		if secondByte >= 0x40 && secondByte <= 0x5F {
			return p.parseSimpleEscape(data, offset)
		}
		return nil, 0
	}
}

// parseCSI parses a CSI sequence: ESC [ params... final
func (p *EscapeCodeParser) parseCSI(data []byte, offset int) (*ParsedEscapeCode, int) {
	if offset+2 >= len(data) {
		return nil, 0
	}

	// Find the terminator (letter A-Z or a-z)
	end := offset + 2
	isPrivate := false
	hasParams := false

	// Check for DEC private mode marker '?'
	if end < len(data) && data[end] == '?' {
		isPrivate = true
		end++
	}

	// Scan for terminator
	for end < len(data) {
		b := data[end]
		// Valid parameter characters: 0-9, ;, :, <, =, >, ?
		if b >= 0x30 && b <= 0x3F {
			hasParams = true
			end++
			continue
		}
		// Valid intermediate characters: space through /
		if b >= 0x20 && b <= 0x2F {
			end++
			continue
		}
		// Terminator: letter
		if (b >= 0x40 && b <= 0x5A) || (b >= 0x61 && b <= 0x7A) {
			end++
			rawBytes := data[offset:end]
			category, description := p.categorizeCSI(rawBytes, isPrivate, hasParams)
			return &ParsedEscapeCode{
				RawBytes:    rawBytes,
				HexEncoded:  hex.EncodeToString(rawBytes),
				Category:    category,
				Description: description,
				StartOffset: offset,
				EndOffset:   end,
			}, end - offset
		}
		// Invalid character - not a valid CSI sequence
		return nil, 0
	}

	// No terminator found - incomplete sequence
	return nil, 0
}

// parseOSC parses an OSC sequence: ESC ] ... BEL or ESC ] ... ESC \
func (p *EscapeCodeParser) parseOSC(data []byte, offset int) (*ParsedEscapeCode, int) {
	if offset+2 >= len(data) {
		return nil, 0
	}

	// Look for BEL (0x07) or ST (ESC \)
	for end := offset + 2; end < len(data) && end-offset < 65536; end++ {
		// BEL terminator
		if data[end] == 0x07 {
			rawBytes := data[offset : end+1]
			return &ParsedEscapeCode{
				RawBytes:    rawBytes,
				HexEncoded:  hex.EncodeToString(rawBytes),
				Category:    CategoryOSC,
				Description: p.describeOSC(rawBytes),
				StartOffset: offset,
				EndOffset:   end + 1,
			}, end - offset + 1
		}
		// ESC \ terminator (ST)
		if data[end] == 0x1b && end+1 < len(data) && data[end+1] == '\\' {
			rawBytes := data[offset : end+2]
			return &ParsedEscapeCode{
				RawBytes:    rawBytes,
				HexEncoded:  hex.EncodeToString(rawBytes),
				Category:    CategoryOSC,
				Description: p.describeOSC(rawBytes),
				StartOffset: offset,
				EndOffset:   end + 2,
			}, end - offset + 2
		}
	}

	return nil, 0
}

// parseStringSequence parses DCS, PM, APC, SOS sequences ending with ST
func (p *EscapeCodeParser) parseStringSequence(data []byte, offset int, category EscapeCategory, baseDesc string) (*ParsedEscapeCode, int) {
	if offset+2 >= len(data) {
		return nil, 0
	}

	// Look for ST (ESC \) or single-byte ST (0x9C)
	for end := offset + 2; end < len(data); end++ {
		// ESC \ terminator
		if data[end] == 0x1b && end+1 < len(data) && data[end+1] == '\\' {
			rawBytes := data[offset : end+2]
			return &ParsedEscapeCode{
				RawBytes:    rawBytes,
				HexEncoded:  hex.EncodeToString(rawBytes),
				Category:    category,
				Description: baseDesc,
				StartOffset: offset,
				EndOffset:   end + 2,
			}, end - offset + 2
		}
		// Single-byte ST (C1)
		if data[end] == 0x9C {
			rawBytes := data[offset : end+1]
			return &ParsedEscapeCode{
				RawBytes:    rawBytes,
				HexEncoded:  hex.EncodeToString(rawBytes),
				Category:    category,
				Description: baseDesc,
				StartOffset: offset,
				EndOffset:   end + 1,
			}, end - offset + 1
		}
	}

	return nil, 0
}

// parseCharset parses character set designation sequences
func (p *EscapeCodeParser) parseCharset(data []byte, offset int) (*ParsedEscapeCode, int) {
	if offset+2 >= len(data) {
		return nil, 0
	}

	// ESC ( X, ESC ) X, ESC * X, ESC + X
	rawBytes := data[offset : offset+3]
	var desc string
	switch data[offset+1] {
	case '(':
		desc = "Designate G0 character set"
	case ')':
		desc = "Designate G1 character set"
	case '*':
		desc = "Designate G2 character set"
	case '+':
		desc = "Designate G3 character set"
	}
	if len(data) > offset+2 {
		switch data[offset+2] {
		case 'B':
			desc += " (ASCII)"
		case '0':
			desc += " (DEC Special Graphics)"
		case 'A':
			desc += " (UK)"
		}
	}

	return &ParsedEscapeCode{
		RawBytes:    rawBytes,
		HexEncoded:  hex.EncodeToString(rawBytes),
		Category:    CategoryCharset,
		Description: desc,
		StartOffset: offset,
		EndOffset:   offset + 3,
	}, 3
}

// parseSimpleEscape parses simple 2-byte escape sequences
func (p *EscapeCodeParser) parseSimpleEscape(data []byte, offset int) (*ParsedEscapeCode, int) {
	if offset+1 >= len(data) {
		return nil, 0
	}

	rawBytes := data[offset : offset+2]
	desc := DescribeSimpleEscape(data[offset+1])

	return &ParsedEscapeCode{
		RawBytes:    rawBytes,
		HexEncoded:  hex.EncodeToString(rawBytes),
		Category:    CategorySimple,
		Description: desc,
		StartOffset: offset,
		EndOffset:   offset + 2,
	}, 2
}

// categorizeCSI determines the category and description of a CSI sequence
func (p *EscapeCodeParser) categorizeCSI(rawBytes []byte, isPrivate bool, hasParams bool) (EscapeCategory, string) {
	if len(rawBytes) < 3 {
		return CategoryCSI, "Unknown CSI"
	}

	// Get the final byte (command)
	finalByte := rawBytes[len(rawBytes)-1]

	// Extract parameter string
	paramStart := 2
	if isPrivate {
		paramStart = 3
	}
	paramEnd := len(rawBytes) - 1
	paramStr := ""
	if paramEnd > paramStart {
		paramStr = string(rawBytes[paramStart:paramEnd])
	}

	if isPrivate {
		return p.describeDECPrivate(rawBytes, finalByte, paramStr)
	}

	return p.describeStandardCSI(rawBytes, finalByte, paramStr)
}

// describeDECPrivate describes DEC private mode sequences
func (p *EscapeCodeParser) describeDECPrivate(rawBytes []byte, finalByte byte, paramStr string) (EscapeCategory, string) {
	mode := paramStr
	isSet := finalByte == 'h'
	isReset := finalByte == 'l'

	action := ""
	if isSet {
		action = "Enable"
	} else if isReset {
		action = "Disable"
	}

	desc := GetDECPrivateModeDescription(mode)
	if desc != "" {
		if action != "" {
			return CategoryDECPriv, action + " " + desc
		}
		return CategoryDECPriv, desc
	}

	return CategoryDECPriv, "DEC Private Mode " + mode
}

// describeStandardCSI describes standard CSI sequences
func (p *EscapeCodeParser) describeStandardCSI(rawBytes []byte, finalByte byte, paramStr string) (EscapeCategory, string) {
	switch finalByte {
	// Cursor movement
	case 'A':
		return CategoryCursor, "Cursor Up" + p.formatParams(paramStr)
	case 'B':
		return CategoryCursor, "Cursor Down" + p.formatParams(paramStr)
	case 'C':
		return CategoryCursor, "Cursor Forward" + p.formatParams(paramStr)
	case 'D':
		return CategoryCursor, "Cursor Back" + p.formatParams(paramStr)
	case 'E':
		return CategoryCursor, "Cursor Next Line" + p.formatParams(paramStr)
	case 'F':
		return CategoryCursor, "Cursor Previous Line" + p.formatParams(paramStr)
	case 'G':
		return CategoryCursor, "Cursor Column" + p.formatParams(paramStr)
	case 'H', 'f':
		return CategoryCursor, "Cursor Position" + p.formatParams(paramStr)

	// Erase operations
	case 'J':
		return CategoryErase, GetEraseInDisplayDescription(paramStr)
	case 'K':
		return CategoryErase, GetEraseInLineDescription(paramStr)

	// SGR (Select Graphic Rendition)
	case 'm':
		return CategorySGR, DescribeSGR(paramStr)

	// Scroll region
	case 'r':
		return CategoryScroll, "Set Scroll Region" + p.formatParams(paramStr)

	// Line operations
	case 'L':
		return CategoryErase, "Insert Lines" + p.formatParams(paramStr)
	case 'M':
		return CategoryErase, "Delete Lines" + p.formatParams(paramStr)
	case 'P':
		return CategoryErase, "Delete Characters" + p.formatParams(paramStr)
	case '@':
		return CategoryErase, "Insert Characters" + p.formatParams(paramStr)
	case 'X':
		return CategoryErase, "Erase Characters" + p.formatParams(paramStr)

	// Tabs
	case 'g':
		return CategoryCSI, "Tab Clear" + p.formatParams(paramStr)

	// Save/Restore cursor
	case 's':
		return CategoryCursor, "Save Cursor Position"
	case 'u':
		return CategoryCursor, "Restore Cursor Position"

	// Scrolling
	case 'S':
		return CategoryScroll, "Scroll Up" + p.formatParams(paramStr)
	case 'T':
		return CategoryScroll, "Scroll Down" + p.formatParams(paramStr)

	// Mode setting
	case 'h':
		return CategoryCSI, "Set Mode" + p.formatParams(paramStr)
	case 'l':
		return CategoryCSI, "Reset Mode" + p.formatParams(paramStr)

	// Device attributes
	case 'c':
		return CategoryCSI, "Device Attributes" + p.formatParams(paramStr)
	case 'n':
		return CategoryCSI, "Device Status Report" + p.formatParams(paramStr)

	default:
		return CategoryCSI, "CSI " + string(finalByte) + p.formatParams(paramStr)
	}
}

// describeOSC describes an OSC sequence
func (p *EscapeCodeParser) describeOSC(rawBytes []byte) string {
	if len(rawBytes) < 4 {
		return "Operating System Command"
	}

	// Extract the command number
	content := rawBytes[2 : len(rawBytes)-1] // Skip ESC ] and terminator
	if len(content) == 0 {
		return "Operating System Command"
	}

	// Find semicolon separator
	semicolon := -1
	for i, b := range content {
		if b == ';' {
			semicolon = i
			break
		}
	}

	cmdStr := ""
	if semicolon > 0 {
		cmdStr = string(content[:semicolon])
	} else {
		cmdStr = string(content)
	}

	return GetOSCDescription(cmdStr)
}

// formatParams formats parameter string for display
func (p *EscapeCodeParser) formatParams(params string) string {
	if params == "" {
		return ""
	}
	return " (" + params + ")"
}

// findPartialEscapeAtEnd checks if data ends with a partial escape sequence
func (p *EscapeCodeParser) findPartialEscapeAtEnd(data []byte) []byte {
	// Cap partial buffer to prevent unbounded growth from malformed sequences
	if len(p.partialBuffer) > 4096 {
		p.partialBuffer = nil
	}

	if len(data) == 0 {
		return nil
	}

	// Look for ESC in the last 50 bytes (escape sequences rarely exceed this)
	scanLen := 50
	if len(data) < scanLen {
		scanLen = len(data)
	}

	for i := len(data) - 1; i >= len(data)-scanLen; i-- {
		if data[i] == 0x1b {
			// Found an ESC - check if sequence is complete
			remaining := data[i:]
			_, consumed := p.parseSequenceAt(remaining, 0)
			if consumed == 0 {
				// Sequence is incomplete - buffer it
				return remaining
			}
			// Sequence is complete
			return nil
		}
	}

	return nil
}
