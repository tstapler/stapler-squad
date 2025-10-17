package session

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// CircularBuffer is a thread-safe circular buffer with automatic disk fallback
// when the in-memory buffer fills up. This prevents memory overflow while maintaining
// a history of PTY output for status detection and debugging.
type CircularBuffer struct {
	data     []byte
	size     int
	head     int    // Write position
	tail     int    // Read position
	count    int    // Number of bytes in buffer
	diskFile *os.File
	mu       sync.RWMutex
	wrapped  bool   // True if head has wrapped around past tail
}

const (
	// DefaultBufferSize is 10MB of in-memory buffer
	DefaultBufferSize = 10 * 1024 * 1024
)

// NewCircularBuffer creates a new circular buffer with the specified size in bytes.
// When the buffer fills up, old data is automatically overwritten (circular behavior).
func NewCircularBuffer(size int) *CircularBuffer {
	if size <= 0 {
		size = DefaultBufferSize
	}
	return &CircularBuffer{
		data: make([]byte, size),
		size: size,
		head: 0,
		tail: 0,
		count: 0,
		wrapped: false,
	}
}

// Write appends data to the circular buffer.
// If the buffer is full, the oldest data is overwritten.
// This is an O(1) operation.
func (cb *CircularBuffer) Write(data []byte) (int, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(data) == 0 {
		return 0, nil
	}

	originalLen := len(data)

	// If data is larger than buffer, only keep the last `size` bytes
	// and completely replace buffer contents
	if len(data) > cb.size {
		data = data[len(data)-cb.size:]
		copy(cb.data, data)
		cb.tail = 0
		cb.head = 0 // After filling buffer, head wraps to 0
		cb.count = cb.size
		cb.wrapped = false // Not really wrapped, just filled from start
		return originalLen, nil
	}

	// Normal incremental write
	for _, b := range data {
		cb.data[cb.head] = b
		cb.head = (cb.head + 1) % cb.size

		if cb.count < cb.size {
			cb.count++
		} else {
			// Buffer is full, advance tail (overwrite oldest data)
			cb.tail = (cb.tail + 1) % cb.size
			cb.wrapped = true
		}
	}

	// Return original data length to indicate all bytes were "written"
	return originalLen, nil
}

// GetRecent returns the last n bytes from the buffer.
// If n is larger than the buffer size or the available data, returns all available data.
func (cb *CircularBuffer) GetRecent(n int) []byte {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if n <= 0 || cb.count == 0 {
		return nil
	}

	// Limit n to available data
	if n > cb.count {
		n = cb.count
	}

	result := make([]byte, n)

	// Calculate starting position for recent data
	// If we want last n bytes and count >= n, start at (head - n)
	startPos := (cb.head - n + cb.size) % cb.size

	// Copy data from circular buffer
	for i := 0; i < n; i++ {
		pos := (startPos + i) % cb.size
		result[i] = cb.data[pos]
	}

	return result
}

// GetAll returns all data currently in the buffer.
// Returns a copy to prevent concurrent modification issues.
func (cb *CircularBuffer) GetAll() []byte {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.count == 0 {
		return nil
	}

	result := make([]byte, cb.count)

	if !cb.wrapped || (cb.head == 0 && cb.count == cb.size) {
		// Simple case: data is contiguous from tail to head
		// Or buffer is completely full from index 0
		if cb.head == 0 && cb.count == cb.size {
			// Buffer is completely full, copy everything
			copy(result, cb.data)
		} else {
			copy(result, cb.data[cb.tail:cb.head])
		}
	} else {
		// Data wraps around: copy from tail to end, then from start to head
		firstPartLen := cb.size - cb.tail
		copy(result, cb.data[cb.tail:])
		if cb.head > 0 {
			copy(result[firstPartLen:], cb.data[:cb.head])
		}
	}

	return result
}

// Len returns the number of bytes currently in the buffer.
func (cb *CircularBuffer) Len() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.count
}

// Cap returns the total capacity of the buffer.
func (cb *CircularBuffer) Cap() int {
	return cb.size
}

// Clear resets the buffer to empty state.
func (cb *CircularBuffer) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.head = 0
	cb.tail = 0
	cb.count = 0
	cb.wrapped = false
}

// EnableDiskFallback enables automatic disk fallback when buffer is full.
// The diskPath parameter specifies where to store overflow data.
// This feature is currently a placeholder for future implementation.
func (cb *CircularBuffer) EnableDiskFallback(diskPath string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.diskFile != nil {
		return fmt.Errorf("disk fallback already enabled")
	}

	// Create disk file for overflow storage
	f, err := os.CreateTemp(diskPath, "circular_buffer_*.dat")
	if err != nil {
		return fmt.Errorf("failed to create disk fallback file: %w", err)
	}

	cb.diskFile = f
	return nil
}

// DisableDiskFallback disables disk fallback and removes the disk file.
func (cb *CircularBuffer) DisableDiskFallback() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.diskFile == nil {
		return nil
	}

	// Get file path before closing
	filePath := cb.diskFile.Name()

	// Close and remove disk file
	if err := cb.diskFile.Close(); err != nil {
		return fmt.Errorf("failed to close disk fallback file: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove disk fallback file: %w", err)
	}

	cb.diskFile = nil
	return nil
}

// Close releases resources used by the circular buffer.
// If disk fallback is enabled, it removes the disk file.
func (cb *CircularBuffer) Close() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.diskFile != nil {
		filePath := cb.diskFile.Name()
		if err := cb.diskFile.Close(); err != nil {
			return fmt.Errorf("failed to close disk fallback file: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove disk fallback file: %w", err)
		}
		cb.diskFile = nil
	}

	return nil
}

// WriteTo implements io.WriterTo interface for efficient streaming.
func (cb *CircularBuffer) WriteTo(w io.Writer) (int64, error) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.count == 0 {
		return 0, nil
	}

	var totalWritten int64

	if !cb.wrapped {
		// Simple case: contiguous data from tail to head
		n, err := w.Write(cb.data[cb.tail:cb.head])
		return int64(n), err
	}

	// Data wraps around: write from tail to end, then from start to head
	n1, err := w.Write(cb.data[cb.tail:])
	if err != nil {
		return int64(n1), err
	}
	totalWritten += int64(n1)

	n2, err := w.Write(cb.data[:cb.head])
	totalWritten += int64(n2)
	return totalWritten, err
}
