package unfinished

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
)

// Read buffers dominated the scanner's allocations (75.9% of S1 alloc_space,
// baseline.md), because every untracked and modified file was read into a
// fresh slice. These helpers read through a pooled buffer instead; the
// retained readFileIfSmall is the allocation oracle for the gate test.

const (
	pooledBufCap = 32 << 10
	// maxPooledBuf stops one large file from pinning memory in the pool.
	maxPooledBuf = 1 << 20
)

var fileBufPool = sync.Pool{New: func() any {
	b := make([]byte, 0, pooledBufCap)
	return &b
}}

func releaseFileBuf(p *[]byte) {
	if p == nil || cap(*p) > maxPooledBuf {
		return
	}
	*p = (*p)[:0]
	fileBufPool.Put(p)
}

// openSmallFile opens path and reports ok only when its size is within
// maxUntrackedFileSize (fstat follows symlinks, so a link to a large file is
// skipped rather than read in full).
func openSmallFile(path string) (*os.File, int64, bool) {
	f, err := os.Open(path) // #nosec G304 -- path comes from an internal worktree walk, not user input
	if err != nil {
		return nil, 0, false
	}
	fi, err := f.Stat()
	if err != nil || fi.Size() > maxUntrackedFileSize {
		_ = f.Close()
		return nil, 0, false
	}
	return f, fi.Size(), true
}

// readSmallFilePooled reads path into a pooled buffer with the semantics of
// readFileIfSmall. The caller must releaseFileBuf the returned handle once it
// is done with the data, which must not be retained.
func readSmallFilePooled(path string) (*[]byte, bool) {
	f, size, ok := openSmallFile(path)
	if !ok {
		return nil, false
	}
	defer f.Close()
	bp := fileBufPool.Get().(*[]byte)
	buf := (*bp)[:0]
	if need := int(size) + 1; cap(buf) < need { // +1 observes EOF without a growth step
		buf = make([]byte, 0, need)
	}
	for {
		n, err := f.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err != nil {
			if errors.Is(err, io.EOF) {
				*bp = buf
				return bp, true
			}
			*bp = buf
			releaseFileBuf(bp)
			return nil, false
		}
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
	}
}

// countFileLines streams path through a pooled buffer and returns what
// countBytesLines would return for its content, allocating nothing per file.
func countFileLines(path string) (int, bool) {
	f, _, ok := openSmallFile(path)
	if !ok {
		return 0, false
	}
	defer f.Close()
	bp := fileBufPool.Get().(*[]byte)
	defer releaseFileBuf(bp)
	buf := (*bp)[:cap(*bp)]
	lines, total := 0, 0
	var last byte
	for {
		n, err := f.Read(buf)
		if n > 0 {
			lines += bytes.Count(buf[:n], []byte{'\n'})
			last = buf[n-1]
			total += n
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return 0, false
			}
			break
		}
	}
	if total > 0 && last != '\n' {
		lines++
	}
	return lines, true
}
