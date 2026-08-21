// Package scanbuf provides a pooled buffer for bufio.Scanner instances that
// need to handle large JSONL lines (base64-encoded tool output, etc.).
// Allocating a fresh multi-MB buffer per scan is a significant allocation
// source for pollers that scan many files per tick; pooling amortizes it.
package scanbuf

import "sync"

// MaxTokenSize is the maximum line size supported by buffers from this pool.
const MaxTokenSize = 10 * 1024 * 1024

var pool = sync.Pool{
	New: func() any {
		buf := make([]byte, MaxTokenSize)
		return &buf
	},
}

// Get returns a buffer of length MaxTokenSize for use with bufio.Scanner.Buffer.
// The caller must return it via Put when done.
func Get() *[]byte {
	return pool.Get().(*[]byte)
}

// Put returns a buffer acquired from Get back to the pool.
func Put(buf *[]byte) {
	pool.Put(buf)
}
