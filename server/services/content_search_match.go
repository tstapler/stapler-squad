package services

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	kibibyte          = 1024
	mebibyte          = 1024 * kibibyte
	searchSniffBytes  = 8 * kibibyte
	searchFileCap     = 2 * mebibyte // default per-file read cap
	searchMaxPerFile  = 50
	searchLineClamp   = 512
	searchLeadContext = 128 // bytes of line kept before a match in a clamped window
	scanReaderSize    = 64 * kibibyte
	scanCtxEveryLines = 512
	scanLineKeepCap   = 256 * kibibyte // larger line buffers are not returned to the pool
)

// SearchMatch is one occurrence of the pattern in one line. Start and End are UTF-16
// code-unit offsets into Text (End exclusive), the unit JavaScript string indexing uses.
type SearchMatch struct {
	Path       string
	Line       int
	Text       string
	Start, End int
	ByteOffset int64
}

// fileScan is the outcome of scanning one file.
type fileScan struct {
	Matches []SearchMatch
	Binary  bool // NUL in the first 8 KB: nothing was matched
	Capped  bool // the read stopped at the byte limit
}

type scanBuf struct {
	br   *bufio.Reader
	line []byte
}

var scanBufPool = sync.Pool{New: func() any {
	return &scanBuf{br: bufio.NewReaderSize(nil, scanReaderSize)}
}}

// lineRef locates one line of one file.
type lineRef struct {
	Path   string
	No     int
	Offset int64
}

// span is a half-open byte range inside a line.
type span struct{ Start, End int }

// scanContent finds matches of re in r. It reads at most limit bytes through a pooled
// buffered reader, never the whole file, which is what keeps search memory independent
// of file size. It is filesystem-independent so it can be tested over any io.Reader.
func scanContent(ctx context.Context, r io.Reader, path string, re *regexp.Regexp, limit int64) (fileScan, error) {
	lr := &io.LimitedReader{R: r, N: limit}
	sb := scanBufPool.Get().(*scanBuf)
	defer func() {
		sb.br.Reset(nil)
		if cap(sb.line) > scanLineKeepCap {
			sb.line = nil
		}
		scanBufPool.Put(sb)
	}()
	sb.br.Reset(lr)

	var res fileScan
	head, err := sb.br.Peek(searchSniffBytes)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return res, err
	}
	if bytes.IndexByte(head, 0) >= 0 {
		res.Binary = true
		return res, nil
	}
	err = scanLines(ctx, sb, path, re, &res)
	res.Capped = lr.N == 0
	return res, err
}

// scanLines splits the buffered reader into lines (joining over-long ones in sb.line)
// and collects matches until EOF, the per-file cap or ctx cancellation.
func scanLines(ctx context.Context, sb *scanBuf, path string, re *regexp.Regexp, res *fileScan) error {
	var offset int64
	lineNo := 0
	for {
		chunk, rerr := sb.br.ReadSlice('\n')
		if errors.Is(rerr, bufio.ErrBufferFull) {
			sb.line = append(sb.line, chunk...)
			continue
		}
		full := chunk
		if len(sb.line) > 0 {
			sb.line = append(sb.line, chunk...)
			full = sb.line
		}
		if len(full) > 0 {
			lineNo++
			res.Matches = appendLineMatches(res.Matches, lineRef{path, lineNo, offset}, full, re)
			offset += int64(len(full))
		}
		sb.line = sb.line[:0]
		if len(res.Matches) >= searchMaxPerFile {
			res.Matches = res.Matches[:searchMaxPerFile]
			return nil
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			return rerr
		}
		if lineNo%scanCtxEveryLines == 0 {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
		}
	}
}

// appendLineMatches adds one SearchMatch per non-empty occurrence in the line.
func appendLineMatches(dst []SearchMatch, ref lineRef, raw []byte, re *regexp.Regexp) []SearchMatch {
	line := bytes.TrimRight(raw, "\r\n")
	if len(line) == 0 {
		return dst
	}
	for _, loc := range re.FindAllIndex(line, searchMaxPerFile) {
		if loc[1] == loc[0] {
			continue
		}
		dst = append(dst, buildMatch(ref, line, span{loc[0], loc[1]}))
	}
	return dst
}

// buildMatch clamps the line to a window of at most searchLineClamp bytes that contains
// the match start, and converts the range to UTF-16 units of the (UTF-8 sanitized) window.
func buildMatch(ref lineRef, line []byte, m span) SearchMatch {
	ws, we := 0, len(line)
	if len(line) > searchLineClamp {
		ws = max(0, m.Start-searchLeadContext)
		for ws < m.Start && !utf8.RuneStart(line[ws]) {
			ws++
		}
		we = min(len(line), ws+searchLineClamp)
		for we > m.Start && we < len(line) && !utf8.RuneStart(line[we]) {
			we--
		}
	}
	end := min(m.End, we)
	text := strings.Map(func(r rune) rune { return r }, string(line[ws:we])) // each invalid byte becomes U+FFFD
	start := utf16Len(line[ws:m.Start])
	return SearchMatch{
		Path: ref.Path, Line: ref.No, Text: text,
		Start: start, End: start + utf16Len(line[m.Start:end]),
		ByteOffset: ref.Offset,
	}
}

// utf16Len counts UTF-16 code units; an invalid byte counts one, like the U+FFFD it becomes.
func utf16Len(b []byte) int {
	n := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
		b = b[size:]
	}
	return n
}
