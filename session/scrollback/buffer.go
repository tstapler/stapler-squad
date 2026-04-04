package scrollback

import (
	"sync"
	"time"
)

// ScrollbackEntry represents a single entry in the scrollback buffer.
type ScrollbackEntry struct {
	Timestamp time.Time
	Data      []byte
	Sequence  uint64
}

// CircularBuffer implements a thread-safe fixed-size circular buffer for terminal scrollback.
// When the buffer is full, new entries overwrite the oldest entries.
type CircularBuffer struct {
	entries  []ScrollbackEntry
	head     int    // Index of next write position
	tail     int    // Index of oldest entry
	size     int    // Current number of entries
	maxSize  int    // Maximum capacity
	sequence uint64 // Monotonically increasing sequence number
	mutex    sync.RWMutex
	dirty    bool // Indicates buffer has unsaved changes
}

// NewCircularBuffer creates a new circular buffer with the specified maximum size.
func NewCircularBuffer(maxSize int) *CircularBuffer {
	return &CircularBuffer{
		entries:  make([]ScrollbackEntry, maxSize),
		head:     0,
		tail:     0,
		size:     0,
		maxSize:  maxSize,
		sequence: 0,
		dirty:    false,
	}
}

// Append adds new data to the buffer. Returns the created entry and whether an old entry was evicted.
func (b *CircularBuffer) Append(data []byte) (ScrollbackEntry, bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Create entry with current sequence
	b.sequence++
	entry := ScrollbackEntry{
		Timestamp: time.Now(),
		Data:      append([]byte(nil), data...), // Deep copy
		Sequence:  b.sequence,
	}

	evicted := false
	if b.size == b.maxSize {
		// Buffer is full, overwrite oldest entry
		evicted = true
		b.tail = (b.tail + 1) % b.maxSize
	} else {
		b.size++
	}

	// Write entry at head position
	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.maxSize
	b.dirty = true

	return entry, evicted
}

// GetRange retrieves entries starting from the specified sequence number, up to limit entries.
// Returns entries in chronological order (oldest first).
func (b *CircularBuffer) GetRange(fromSeq uint64, limit int) []ScrollbackEntry {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.size == 0 {
		return nil
	}

	result := make([]ScrollbackEntry, 0, limit)
	idx := b.tail

	for i := 0; i < b.size && len(result) < limit; i++ {
		entry := b.entries[idx]
		if entry.Sequence >= fromSeq {
			result = append(result, entry)
		}
		idx = (idx + 1) % b.maxSize
	}

	return result
}

// GetLastN retrieves the last N entries from the buffer.
// Returns entries in chronological order (oldest first).
func (b *CircularBuffer) GetLastN(n int) []ScrollbackEntry {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.size == 0 {
		return nil
	}

	// Calculate how many entries to return
	count := n
	if count > b.size {
		count = b.size
	}

	result := make([]ScrollbackEntry, 0, count)

	// Start from (head - count) position
	startOffset := b.size - count
	idx := (b.tail + startOffset) % b.maxSize

	for i := 0; i < count; i++ {
		result = append(result, b.entries[idx])
		idx = (idx + 1) % b.maxSize
	}

	return result
}

// GetAll retrieves all entries in the buffer.
// Returns entries in chronological order (oldest first).
func (b *CircularBuffer) GetAll() []ScrollbackEntry {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.size == 0 {
		return nil
	}

	result := make([]ScrollbackEntry, 0, b.size)
	idx := b.tail

	for i := 0; i < b.size; i++ {
		result = append(result, b.entries[idx])
		idx = (idx + 1) % b.maxSize
	}

	return result
}

// Size returns the current number of entries in the buffer.
func (b *CircularBuffer) Size() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.size
}

// MaxSize returns the maximum capacity of the buffer.
func (b *CircularBuffer) MaxSize() int {
	return b.maxSize
}

// CurrentSequence returns the current sequence number (last assigned).
func (b *CircularBuffer) CurrentSequence() uint64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.sequence
}

// IsDirty returns whether the buffer has unsaved changes.
func (b *CircularBuffer) IsDirty() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.dirty
}

// MarkClean marks the buffer as having no unsaved changes.
func (b *CircularBuffer) MarkClean() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.dirty = false
}

// Clear removes all entries from the buffer.
func (b *CircularBuffer) Clear() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.head = 0
	b.tail = 0
	b.size = 0
	b.dirty = true
}

// GetOldestSequence returns the sequence number of the oldest entry in the buffer.
// Returns 0 if buffer is empty.
func (b *CircularBuffer) GetOldestSequence() uint64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.size == 0 {
		return 0
	}

	return b.entries[b.tail].Sequence
}

// GetNewestSequence returns the sequence number of the newest entry in the buffer.
// Returns 0 if buffer is empty.
func (b *CircularBuffer) GetNewestSequence() uint64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.size == 0 {
		return 0
	}

	// Last written entry is at (head - 1)
	idx := (b.head - 1 + b.maxSize) % b.maxSize
	return b.entries[idx].Sequence
}
