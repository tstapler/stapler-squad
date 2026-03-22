package scrollback

import (
	"fmt"
	"sync"
	"time"
)

// ScrollbackConfig configures scrollback behavior.
type ScrollbackConfig struct {
	MaxLines         int           // Maximum lines per session (default: 10000)
	MaxSizeBytes     int64         // Maximum storage size per session (default: 10MB)
	FlushInterval    time.Duration // How often to flush to disk (default: 5s)
	CompressionType  string        // "none", "zstd", or "gzip" (default: "zstd")
	CompressionLevel int           // Compression level (zstd: 1-19, default: 3)
	StoragePath      string        // Base path for storage (default: ~/.stapler-squad/sessions)
}

// DefaultScrollbackConfig returns the default configuration.
func DefaultScrollbackConfig() ScrollbackConfig {
	return ScrollbackConfig{
		MaxLines:         10000,
		MaxSizeBytes:     10 * 1024 * 1024, // 10MB
		FlushInterval:    5 * time.Second,
		CompressionType:  "zstd",
		CompressionLevel: 3, // Zstd level 3: fast compression with good ratio
		StoragePath:      "", // Will be set by caller
	}
}

// ScrollbackManager manages scrollback buffers for multiple sessions.
type ScrollbackManager struct {
	storage      ScrollbackStorage
	buffers      map[string]*CircularBuffer
	config       ScrollbackConfig
	mutex        sync.RWMutex
	stopChan     chan struct{}
	stopOnce     sync.Once
	flushTicker  *time.Ticker
}

// NewScrollbackManager creates a new scrollback manager.
func NewScrollbackManager(config ScrollbackConfig) *ScrollbackManager {
	// Create storage with compression configuration
	storage := NewFileScrollbackStorage(config.StoragePath, config.CompressionType, config.CompressionLevel)

	manager := &ScrollbackManager{
		storage:     storage,
		buffers:     make(map[string]*CircularBuffer),
		config:      config,
		stopChan:    make(chan struct{}),
		flushTicker: time.NewTicker(config.FlushInterval),
	}

	// Start background flush goroutine
	go manager.flushLoop()

	return manager
}

// AppendOutput adds terminal output to the session's scrollback.
func (m *ScrollbackManager) AppendOutput(sessionID string, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	m.mutex.Lock()
	buffer, exists := m.buffers[sessionID]
	if !exists {
		buffer = NewCircularBuffer(m.config.MaxLines)
		m.buffers[sessionID] = buffer
	}
	m.mutex.Unlock()

	// Append to circular buffer
	buffer.Append(data)

	return nil
}

// GetScrollback retrieves scrollback entries starting from the specified sequence.
func (m *ScrollbackManager) GetScrollback(sessionID string, fromSeq uint64, limit int) ([]ScrollbackEntry, error) {
	m.mutex.RLock()
	buffer, exists := m.buffers[sessionID]
	m.mutex.RUnlock()

	var memoryEntries []ScrollbackEntry
	if exists {
		memoryEntries = buffer.GetRange(fromSeq, limit)
		if len(memoryEntries) >= limit {
			return memoryEntries, nil
		}
	}

	// If we need more entries, read from storage
	storageEntries, err := m.storage.Read(sessionID, fromSeq, limit-len(memoryEntries))
	if err != nil {
		return memoryEntries, fmt.Errorf("failed to read from storage: %w", err)
	}

	// Merge entries (storage entries are older)
	result := make([]ScrollbackEntry, 0, len(storageEntries)+len(memoryEntries))
	result = append(result, storageEntries...)
	result = append(result, memoryEntries...)

	return result, nil
}

// GetRecentLines retrieves the last N lines from scrollback.
func (m *ScrollbackManager) GetRecentLines(sessionID string, lines int) ([]byte, error) {
	m.mutex.RLock()
	buffer, exists := m.buffers[sessionID]
	m.mutex.RUnlock()

	if !exists {
		// Try to load from storage
		entries, err := m.storage.Read(sessionID, 0, lines)
		if err != nil {
			return nil, fmt.Errorf("failed to read from storage: %w", err)
		}
		return m.entriesToBytes(entries), nil
	}

	// Get from memory buffer
	entries := buffer.GetLastN(lines)
	return m.entriesToBytes(entries), nil
}

// GetRecentBytes retrieves the last N bytes from scrollback.
func (m *ScrollbackManager) GetRecentBytes(sessionID string, bytes int64) ([]byte, error) {
	// First try storage (has the full history)
	data, err := m.storage.ReadTail(sessionID, bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read tail: %w", err)
	}

	// If storage is empty or we need more, get from memory
	if len(data) < int(bytes) {
		m.mutex.RLock()
		buffer, exists := m.buffers[sessionID]
		m.mutex.RUnlock()

		if exists {
			entries := buffer.GetAll()
			memData := m.entriesToBytes(entries)
			// Combine storage and memory data
			data = append(data, memData...)
		}
	}

	// Truncate to requested size if needed
	if int64(len(data)) > bytes {
		data = data[int64(len(data))-bytes:]
	}

	return data, nil
}

// ClearScrollback removes all scrollback for the session.
func (m *ScrollbackManager) ClearScrollback(sessionID string) error {
	m.mutex.Lock()
	if buffer, exists := m.buffers[sessionID]; exists {
		buffer.Clear()
	}
	delete(m.buffers, sessionID)
	m.mutex.Unlock()

	// Delete from storage
	if err := m.storage.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to delete storage: %w", err)
	}

	return nil
}

// FlushSession flushes a specific session's buffer to storage.
func (m *ScrollbackManager) FlushSession(sessionID string) error {
	m.mutex.RLock()
	buffer, exists := m.buffers[sessionID]
	m.mutex.RUnlock()

	if !exists || !buffer.IsDirty() {
		return nil // Nothing to flush
	}

	// Get all entries
	entries := buffer.GetAll()
	if len(entries) == 0 {
		return nil
	}

	// Write to storage
	if err := m.storage.Write(sessionID, entries); err != nil {
		return fmt.Errorf("failed to write to storage: %w", err)
	}

	// Mark as clean
	buffer.MarkClean()

	// Check if we need to truncate
	size, err := m.storage.Size(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get storage size: %w", err)
	}

	if size > m.config.MaxSizeBytes {
		if err := m.storage.Truncate(sessionID, m.config.MaxSizeBytes); err != nil {
			return fmt.Errorf("failed to truncate storage: %w", err)
		}
	}

	return nil
}

// FlushAll flushes all dirty buffers to storage.
func (m *ScrollbackManager) FlushAll() error {
	m.mutex.RLock()
	sessionIDs := make([]string, 0, len(m.buffers))
	for sessionID := range m.buffers {
		sessionIDs = append(sessionIDs, sessionID)
	}
	m.mutex.RUnlock()

	var lastErr error
	for _, sessionID := range sessionIDs {
		if err := m.FlushSession(sessionID); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Close stops the manager and flushes all buffers.
func (m *ScrollbackManager) Close() error {
	m.stopOnce.Do(func() {
		close(m.stopChan)
		m.flushTicker.Stop()
	})

	return m.FlushAll()
}

// flushLoop periodically flushes dirty buffers to storage.
func (m *ScrollbackManager) flushLoop() {
	for {
		select {
		case <-m.flushTicker.C:
			m.FlushAll()
		case <-m.stopChan:
			return
		}
	}
}

// entriesToBytes converts a slice of entries to raw bytes.
func (m *ScrollbackManager) entriesToBytes(entries []ScrollbackEntry) []byte {
	if len(entries) == 0 {
		return nil
	}

	// Calculate total size
	totalSize := 0
	for _, entry := range entries {
		totalSize += len(entry.Data)
	}

	// Concatenate all data
	result := make([]byte, 0, totalSize)
	for _, entry := range entries {
		result = append(result, entry.Data...)
	}

	return result
}

// GetStats returns statistics about scrollback usage.
func (m *ScrollbackManager) GetStats(sessionID string) (ScrollbackStats, error) {
	m.mutex.RLock()
	buffer, exists := m.buffers[sessionID]
	m.mutex.RUnlock()

	stats := ScrollbackStats{
		SessionID: sessionID,
	}

	if exists {
		stats.MemoryLines = buffer.Size()
		stats.OldestSequence = buffer.GetOldestSequence()
		stats.NewestSequence = buffer.GetNewestSequence()
	}

	size, err := m.storage.Size(sessionID)
	if err != nil {
		return stats, fmt.Errorf("failed to get storage size: %w", err)
	}
	stats.StorageBytes = size

	return stats, nil
}

// ScrollbackStats provides information about scrollback usage.
type ScrollbackStats struct {
	SessionID       string
	MemoryLines     int
	StorageBytes    int64
	OldestSequence  uint64
	NewestSequence  uint64
}
