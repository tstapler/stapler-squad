package services

import (
	"bytes"
	"io"
	"unicode/utf8"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// Range constants from ADR-036: 400-line / 128 KiB default windows, 4,000-line /
// 1 MiB hard caps, a line over 256 KiB switches a region to byte-window mode.
const (
	defaultRangeLines  = 400
	maxRangeLines      = 4000
	defaultRangeBytes  = 128 << 10
	maxRangeBytes      = 1 << 20
	longLineBytes      = 256 << 10
	maxRunePrefixBytes = utf8.UTFMax - 1
)

// rangeWindow is the raw bytes of one window plus where they sit in the file.
type rangeWindow struct {
	Start     int64 // raw byte offset of Raw[0]
	Raw       []byte
	StartLine int64 // 1-based line at Start; 0 when unknown without an index
	Partial   bool  // ends mid-line because of the byte cap
}

// rangeLimits are the clamped caps for one request.
type rangeLimits struct {
	Lines int
	Bytes int64
}

func clampRangeLimits(wantLines int32, maxBytes int64) rangeLimits {
	l := rangeLimits{Lines: int(wantLines), Bytes: maxBytes}
	if l.Lines <= 0 {
		l.Lines = defaultRangeLines
	}
	l.Lines = min(l.Lines, maxRangeLines)
	if l.Bytes <= 0 {
		l.Bytes = defaultRangeBytes
	}
	l.Bytes = min(l.Bytes, maxRangeBytes)
	return l
}

func readAtFull(r io.ReaderAt, off, n int64) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	got, err := r.ReadAt(buf, off)
	if got == len(buf) || (err == io.EOF && got > 0) {
		return buf[:got], nil
	}
	if err == nil || err == io.EOF {
		return buf[:got], nil
	}
	return nil, err
}

// firstLineFor returns the 1-based first line of a line window.
func firstLineFor(dir sessionv1.RangeDir, anchor int64, lines int) int64 {
	var first int64
	switch dir {
	case sessionv1.RangeDir_RANGE_DIR_FORWARD:
		first = anchor
	case sessionv1.RangeDir_RANGE_DIR_BACKWARD:
		first = anchor - int64(lines) + 1
	default:
		first = anchor - int64(lines)/2
	}
	return max(first, 1)
}

// cutAtLines keeps at most wantLines whole lines of raw. atEOF means raw reaches
// the end of the file, so an unterminated last line is complete.
func cutAtLines(raw []byte, wantLines int, atEOF bool) (out []byte, partial bool) {
	idx, count := 0, 0
	for count < wantLines {
		i := bytes.IndexByte(raw[idx:], '\n')
		if i < 0 {
			break
		}
		idx += i + 1
		count++
	}
	switch {
	case count == wantLines:
		return raw[:idx], false
	case atEOF:
		return raw, false
	}
	if last := bytes.LastIndexByte(raw, '\n'); last >= 0 {
		return raw[:last+1], false
	}
	return truncateOnRuneBoundary(raw), true
}

// byteWindowStart resolves a byte anchor to a raw start offset and length.
func byteWindowStart(a *sessionv1.ByteAnchor, size int64, lim rangeLimits) (start, length int64) {
	length = a.Length
	if length <= 0 || length > lim.Bytes {
		length = lim.Bytes
	}
	switch a.Dir {
	case sessionv1.RangeDir_RANGE_DIR_BACKWARD:
		start = a.Offset - length
	case sessionv1.RangeDir_RANGE_DIR_AROUND, sessionv1.RangeDir_RANGE_DIR_UNSPECIFIED:
		start = a.Offset - length/2
	case sessionv1.RangeDir_RANGE_DIR_FORWARD:
		start = a.Offset
	}
	return min(max(start, 0), size), length
}

// readByteWindow reads [start, start+length) snapped to rune boundaries: the
// start moves back over at most three continuation bytes, the end drops an
// incomplete trailing rune unless the window reaches the end of the file.
func readByteWindow(f io.ReaderAt, size, start, length int64) (rangeWindow, error) {
	pre := min(start, int64(maxRunePrefixBytes))
	buf, err := readAtFull(f, start-pre, pre+min(length, size-start))
	if err != nil {
		return rangeWindow{}, err
	}
	k := pre
	for shifted := 0; shifted < maxRunePrefixBytes && k > 0 && k < int64(len(buf)) && !utf8.RuneStart(buf[k]); shifted++ {
		k--
	}
	raw := buf[k:]
	win := rangeWindow{Start: start - pre + k, Raw: raw}
	if win.Start+int64(len(raw)) < size {
		win.Raw = truncateOnRuneBoundary(raw)
		win.Partial = len(win.Raw) == 0 || win.Raw[len(win.Raw)-1] != '\n'
	}
	return win, nil
}

func maxLineLen(raw []byte) int {
	longest := 0
	for len(raw) > 0 {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			i = len(raw)
		}
		longest = max(longest, i)
		raw = raw[min(i+1, len(raw)):]
	}
	return longest
}
