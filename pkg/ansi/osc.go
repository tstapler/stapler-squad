package ansi

import "strings"

// oscBEL and oscST are the two terminators a complete OSC (Operating System
// Command) sequence may use per ECMA-48/xterm convention: a bare BEL byte, or
// the two-byte String Terminator ESC \.
const (
	oscBEL      = 0x07
	oscSTPrefix = 0x1b
	oscSTFinal  = '\\'
)

// findOSCTerminator scans data[start:] for the first OSC terminator (BEL or
// ST) and returns its index (relative to data, not start) and its length (1
// for BEL, 2 for ST). Returns (-1, 0) if no terminator is found.
func findOSCTerminator(data string, start int) (idx int, termLen int) {
	for i := start; i < len(data); i++ {
		switch data[i] {
		case oscBEL:
			return i, 1
		case oscSTPrefix:
			if i+1 < len(data) && data[i+1] == oscSTFinal {
				return i, 2
			}
		}
	}
	return -1, 0
}

// ExtractLastOSC scans data for complete OSC sequences whose numeric command
// matches one of oscNums, terminated by BEL or ST, and returns the payload of
// the last (right-most) complete match, regardless of oscNums order. data
// should already be a bounded window (e.g. a fixed-size PTY tail) — this
// function does not itself cap scan length.
func ExtractLastOSC(data string, oscNums ...string) (payload string, ok bool) {
	if !strings.Contains(data, "\x1b]") {
		return "", false
	}

	bestEnd := -1
	for _, num := range oscNums {
		prefix := "\x1b]" + num + ";"
		searchFrom := 0
		for {
			rel := strings.Index(data[searchFrom:], prefix)
			if rel < 0 {
				break
			}
			start := searchFrom + rel
			payloadStart := start + len(prefix)
			termIdx, termLen := findOSCTerminator(data, payloadStart)
			if termIdx < 0 {
				// No terminator anywhere after this occurrence — no later
				// occurrence of the same num can be complete either, since
				// the terminator search only moves forward.
				break
			}
			end := termIdx + termLen
			if end > bestEnd {
				bestEnd = end
				payload = data[payloadStart:termIdx]
				ok = true
			}
			searchFrom = end
		}
	}
	return payload, ok
}
