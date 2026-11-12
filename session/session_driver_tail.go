package session

import (
	"strings"
	"unsafe"

	"github.com/spaolacci/murmur3"

	"github.com/tstapler/stapler-squad/session/detection"
)

// statusDetectionTailBytes bounds the window the dialog latch matches and hashes.
const statusDetectionTailBytes = detection.StatusDetectionTailBytes

// tailContent returns the last n bytes of s, snapped forward to the next
// newline boundary so we never split a line mid-way through.
// If s is shorter than n bytes, s itself is returned unchanged.
func tailContent(s string, n int) string {
	if len(s) <= n {
		return s
	}
	tail := s[len(s)-n:]
	// Advance to the next newline so we start on a clean line boundary.
	if nl := strings.IndexByte(tail, '\n'); nl >= 0 {
		tail = tail[nl+1:]
	}
	return tail
}

// hashString returns a murmur3-64 hash of s without allocating a []byte copy.
func hashString(s string) uint64 {
	if len(s) == 0 {
		return murmur3.Sum64(nil)
	}
	return murmur3.Sum64(unsafe.Slice(unsafe.StringData(s), len(s)))
}
