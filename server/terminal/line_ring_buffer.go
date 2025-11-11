package terminal

import "sync"

// LineRingBuffer is a fixed-size circular buffer for terminal lines.
// Optimized for terminal viewport semantics where old lines scroll out.
// This is more efficient than slice-based storage for terminal delta tracking
// because it provides O(1) append operations with zero allocations after initialization.
type LineRingBuffer struct {
	lines [][]byte     // Fixed-size buffer of terminal lines
	head  int          // Next write position (newest line)
	size  int          // Current number of lines in buffer (up to capacity)
	cap   int          // Maximum capacity (terminal rows)
	mutex sync.RWMutex // Thread-safe access
}

// NewLineRingBuffer creates a ring buffer for terminal viewport.
// capacity should match the terminal row count (e.g., 24, 80, 100).
func NewLineRingBuffer(capacity int) *LineRingBuffer {
	if capacity <= 0 {
		capacity = 24 // Default terminal height
	}
	return &LineRingBuffer{
		lines: make([][]byte, capacity),
		head:  0,
		size:  0,
		cap:   capacity,
	}
}

// Append adds a line to the buffer (overwrites oldest if full).
// This is an O(1) operation with zero allocations after buffer initialization.
func (rb *LineRingBuffer) Append(line []byte) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	rb.lines[rb.head] = line
	rb.head = (rb.head + 1) % rb.cap

	if rb.size < rb.cap {
		rb.size++
	}
}

// Get retrieves line at logical index (0 = oldest visible line, size-1 = newest).
// Returns nil if index is out of bounds.
// This is an O(1) operation.
func (rb *LineRingBuffer) Get(index int) []byte {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	if index < 0 || index >= rb.size {
		return nil
	}

	// Map logical index to physical index in circular buffer
	// Physical index starts at (head - size) and wraps around
	physicalIndex := (rb.head - rb.size + index + rb.cap) % rb.cap
	return rb.lines[physicalIndex]
}

// Size returns the current number of lines in the buffer.
func (rb *LineRingBuffer) Size() int {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return rb.size
}

// Capacity returns the maximum capacity of the buffer.
func (rb *LineRingBuffer) Capacity() int {
	return rb.cap
}

// SetAll replaces buffer contents with new lines (truncates to capacity).
// This is used for full sync operations.
// If newLines exceeds capacity, only the last N lines are kept.
func (rb *LineRingBuffer) SetAll(newLines [][]byte) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	rb.Clear()

	start := 0
	if len(newLines) > rb.cap {
		// Keep only the last N lines (viewport semantics)
		start = len(newLines) - rb.cap
	}

	for i := start; i < len(newLines); i++ {
		rb.lines[rb.head] = newLines[i]
		rb.head = (rb.head + 1) % rb.cap
		rb.size++
	}
}

// Clear resets the buffer to empty state.
func (rb *LineRingBuffer) Clear() {
	// Note: Don't need lock here as this is called from SetAll which holds lock
	rb.head = 0
	rb.size = 0
	// Don't zero out the lines slice - we'll overwrite them on next use
}

// GetAllLines returns all lines in the buffer in chronological order.
// Returns a slice view (not a copy) for efficiency.
// Caller should not modify the returned slices.
func (rb *LineRingBuffer) GetAllLines() [][]byte {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	if rb.size == 0 {
		return nil
	}

	result := make([][]byte, rb.size)
	for i := 0; i < rb.size; i++ {
		physicalIndex := (rb.head - rb.size + i + rb.cap) % rb.cap
		result[i] = rb.lines[physicalIndex]
	}
	return result
}

// UpdateDimensions resizes the buffer to match new terminal dimensions.
// Preserves as many recent lines as possible (up to new capacity).
func (rb *LineRingBuffer) UpdateDimensions(newRows int) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	if newRows == rb.cap {
		return // No change needed
	}

	// Extract current lines
	oldLines := make([][]byte, rb.size)
	for i := 0; i < rb.size; i++ {
		physicalIndex := (rb.head - rb.size + i + rb.cap) % rb.cap
		oldLines[i] = rb.lines[physicalIndex]
	}

	// Create new buffer
	rb.lines = make([][]byte, newRows)
	rb.cap = newRows
	rb.head = 0
	rb.size = 0

	// Restore lines (keeping last N if too many)
	start := 0
	if len(oldLines) > newRows {
		start = len(oldLines) - newRows
	}

	for i := start; i < len(oldLines); i++ {
		rb.lines[rb.head] = oldLines[i]
		rb.head = (rb.head + 1) % rb.cap
		rb.size++
	}
}
