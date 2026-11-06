// Package scanstate persists where a background scanner stopped reading each
// JSONL transcript, so a restart resumes at the appended bytes instead of
// re-reading every transcript from byte 0.
package scanstate

import (
	"bufio"
	"bytes"
	"hash/fnv"
	"io"
	"io/fs"

	"github.com/tstapler/stapler-squad/session/scanbuf"
)

// fingerprintWindow is how many bytes at each end of the consumed prefix are hashed.
const fingerprintWindow = 4096

// Position is a scanner's resume point in one file.
type Position struct {
	// Size and ModTimeNs are the file's stat when last scanned.
	Size      int64
	ModTimeNs int64
	// Offset is the byte just past the last newline-terminated line consumed.
	Offset int64
	// HeadHash and TailHash fingerprint the first and last fingerprintWindow
	// bytes of [0, Offset). Both zero means "unset" (legacy offset-only seeds).
	HeadHash uint64
	TailHash uint64
}

// Capture builds the Position for a scan that consumed [0, offset) of f, whose
// stat was fi.
func Capture(f io.ReaderAt, fi fs.FileInfo, offset int64) (Position, error) {
	head, tail, err := fingerprint(f, offset)
	if err != nil {
		return Position{}, err
	}
	return Position{
		Size:      max(fi.Size(), offset),
		ModTimeNs: fi.ModTime().UnixNano(),
		Offset:    offset,
		HeadHash:  head,
		TailHash:  tail,
	}, nil
}

// Unchanged reports whether fi shows nothing was appended since p was captured,
// without opening the file.
func (p Position) Unchanged(fi fs.FileInfo) bool {
	return p.Offset == p.Size && fi.Size() == p.Size && fi.ModTime().UnixNano() == p.ModTimeNs
}

// Resumable reports whether scanning may continue at p.Offset: the file has not
// shrunk below it and the bytes already consumed still hash the same. A shrunk,
// truncated, rotated, or rewritten file is not resumable and must be rescanned
// from byte 0. An in-place rewrite that preserves both file ends, the size and
// the first/last 4 KiB of the consumed prefix is not detected.
func (p Position) Resumable(f io.ReaderAt, size int64) bool {
	if p.Offset == 0 {
		return true
	}
	if size < p.Offset {
		return false
	}
	if p.HeadHash == 0 && p.TailHash == 0 {
		return true
	}
	head, tail, err := fingerprint(f, p.Offset)
	return err == nil && head == p.HeadHash && tail == p.TailHash
}

func fingerprint(f io.ReaderAt, offset int64) (head, tail uint64, err error) {
	if offset <= 0 {
		return 0, 0, nil
	}
	if head, err = hashRange(f, 0, min(offset, fingerprintWindow)); err != nil {
		return 0, 0, err
	}
	tailStart := max(0, offset-fingerprintWindow)
	tail, err = hashRange(f, tailStart, offset-tailStart)
	return head, tail, err
}

func hashRange(f io.ReaderAt, off, n int64) (uint64, error) {
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, off); err != nil {
		return 0, err
	}
	h := fnv.New64a()
	_, _ = h.Write(buf)
	return h.Sum64(), nil
}

// ScanComplete feeds each newline-terminated line of r to fn (without the
// newline) and returns how many bytes those lines consumed. A final line with
// no terminating newline is not consumed: it may still be mid-write, so it is
// returned as tail (a copy) and left for the next scan. A line longer than
// maxLine stops the scan with bufio.ErrTooLong; consumed still counts the lines
// before it.
func ScanComplete(r io.Reader, maxLine int, fn func(line []byte)) (consumed int64, tail []byte, err error) {
	sc := bufio.NewScanner(r)
	bufPtr := scanbuf.Get()
	defer scanbuf.Put(bufPtr)
	sc.Buffer(*bufPtr, maxLine)
	sc.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			consumed += int64(i + 1)
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			tail = append([]byte(nil), data...)
		}
		return 0, nil, nil
	})
	for sc.Scan() {
		fn(sc.Bytes())
	}
	return consumed, tail, sc.Err()
}
