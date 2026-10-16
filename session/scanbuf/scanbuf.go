// Package scanbuf provides a pooled buffer for bufio.Scanner instances that
// need to handle large JSONL lines (base64-encoded tool output, etc.).
// Allocating a fresh multi-MB buffer per scan is a significant allocation
// source for pollers that scan many files per tick; pooling amortizes it.
package scanbuf

import "sync"

// MaxTokenSize is the maximum line size supported by buffers from this pool.
// Pass it as bufio.Scanner's max — the scanner grows its buffer up to this
// ceiling only when a line actually requires it.
const MaxTokenSize = 10 * 1024 * 1024

// initialBufSize is the buffer size actually allocated by the pool. The vast
// majority of scanned lines are far smaller than MaxTokenSize; starting small
// and letting bufio.Scanner grow the buffer on the rare oversized line avoids
// pinning a full 10MB per pooled buffer under concurrent scans.
const initialBufSize = 64 * 1024

var pool = sync.Pool{
	New: func() any {
		buf := make([]byte, initialBufSize)
		return &buf
	},
}

// Get returns a buffer for use as bufio.Scanner's initial buffer (pass
// scanbuf.MaxTokenSize as the scanner's max size). The caller must return it
// via Put when done.
func Get() *[]byte {
	return pool.Get().(*[]byte)
}

// Put returns a buffer acquired from Get back to the pool.
func Put(buf *[]byte) {
	pool.Put(buf)
}
