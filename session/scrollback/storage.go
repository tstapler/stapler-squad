package scrollback

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

// ScrollbackStorage defines the interface for persistent scrollback storage.
type ScrollbackStorage interface {
	// Write appends entries to storage
	Write(sessionID string, entries []ScrollbackEntry) error

	// Read retrieves entries starting from the specified sequence number
	Read(sessionID string, fromSeq uint64, limit int) ([]ScrollbackEntry, error)

	// ReadTail retrieves the last N bytes from storage
	ReadTail(sessionID string, bytes int64) ([]byte, error)

	// Truncate removes old entries, keeping only the specified number of bytes
	Truncate(sessionID string, keepBytes int64) error

	// Delete removes all storage for the session
	Delete(sessionID string) error

	// Size returns the current storage size in bytes
	Size(sessionID string) (int64, error)
}

// FileScrollbackStorage implements ScrollbackStorage using line-delimited JSON files.
type FileScrollbackStorage struct {
	basePath         string
	compressionType  string // "none", "zstd", or "gzip"
	compressionLevel int    // Compression level
	fileMutex        sync.Mutex
	fileLocks        map[string]*sync.Mutex
	locksGuard       sync.Mutex
	zstdEncoder      *zstd.Encoder // Reusable zstd encoder
	zstdDecoder      *zstd.Decoder // Reusable zstd decoder
}

// storedEntry represents the JSON format for stored scrollback entries.
type storedEntry struct {
	Timestamp int64  `json:"ts"`   // Unix timestamp in milliseconds
	Sequence  uint64 `json:"seq"`  // Sequence number
	Data      string `json:"data"` // Base64-encoded data
}

// NewFileScrollbackStorage creates a new file-based storage instance.
func NewFileScrollbackStorage(basePath string, compressionType string, compressionLevel int) *FileScrollbackStorage {
	storage := &FileScrollbackStorage{
		basePath:         basePath,
		compressionType:  compressionType,
		compressionLevel: compressionLevel,
		fileLocks:        make(map[string]*sync.Mutex),
	}

	// Initialize zstd encoder/decoder if using zstd
	if compressionType == "zstd" {
		// Create encoder with specified level
		var level zstd.EncoderLevel
		switch {
		case compressionLevel <= 3:
			level = zstd.SpeedFastest
		case compressionLevel <= 6:
			level = zstd.SpeedDefault
		case compressionLevel <= 9:
			level = zstd.SpeedBetterCompression
		default:
			level = zstd.SpeedBestCompression
		}

		encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(level))
		storage.zstdEncoder = encoder

		decoder, _ := zstd.NewReader(nil)
		storage.zstdDecoder = decoder
	}

	return storage
}

// getFileLock returns a mutex for the specified session file.
func (s *FileScrollbackStorage) getFileLock(sessionID string) *sync.Mutex {
	s.locksGuard.Lock()
	defer s.locksGuard.Unlock()

	if lock, exists := s.fileLocks[sessionID]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	s.fileLocks[sessionID] = lock
	return lock
}

// getFilePath returns the file path for the specified session.
func (s *FileScrollbackStorage) getFilePath(sessionID string) string {
	var filename string
	switch s.compressionType {
	case "zstd":
		filename = "scrollback.jsonl.zst"
	case "gzip":
		filename = "scrollback.jsonl.gz"
	default:
		filename = "scrollback.jsonl"
	}
	return filepath.Join(s.basePath, sessionID, filename)
}

// Write appends entries to the scrollback file.
func (s *FileScrollbackStorage) Write(sessionID string, entries []ScrollbackEntry) error {
	if len(entries) == 0 {
		return nil
	}

	lock := s.getFileLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	filePath := s.getFilePath(sessionID)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create scrollback directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open scrollback file: %w", err)
	}
	defer file.Close()

	var writer io.Writer = file
	var gzipWriter *gzip.Writer
	var zstdWriter *zstd.Encoder

	switch s.compressionType {
	case "gzip":
		gzipWriter = gzip.NewWriter(file)
		writer = gzipWriter
		defer gzipWriter.Close()
	case "zstd":
		zstdWriter = s.zstdEncoder
		zstdWriter.Reset(file)
		writer = zstdWriter
		defer zstdWriter.Close()
	}

	// Write entries as line-delimited JSON
	encoder := json.NewEncoder(writer)
	for _, entry := range entries {
		stored := storedEntry{
			Timestamp: entry.Timestamp.UnixMilli(),
			Sequence:  entry.Sequence,
			Data:      string(entry.Data), // Store raw bytes as string
		}
		if err := encoder.Encode(stored); err != nil {
			return fmt.Errorf("failed to encode entry: %w", err)
		}
	}

	// Flush compression writers
	if gzipWriter != nil {
		if err := gzipWriter.Flush(); err != nil {
			return fmt.Errorf("failed to flush gzip writer: %w", err)
		}
	}
	if zstdWriter != nil {
		if err := zstdWriter.Flush(); err != nil {
			return fmt.Errorf("failed to flush zstd writer: %w", err)
		}
	}

	return nil
}

// Read retrieves entries starting from the specified sequence number.
func (s *FileScrollbackStorage) Read(sessionID string, fromSeq uint64, limit int) ([]ScrollbackEntry, error) {
	lock := s.getFileLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	filePath := s.getFilePath(sessionID)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No scrollback yet
		}
		return nil, fmt.Errorf("failed to open scrollback file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file
	var gzipReader *gzip.Reader

	switch s.compressionType {
	case "gzip":
		gzipReader, err = gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	case "zstd":
		s.zstdDecoder.Reset(file)
		reader = s.zstdDecoder
	}

	scanner := bufio.NewScanner(reader)
	entries := make([]ScrollbackEntry, 0, limit)

	for scanner.Scan() && len(entries) < limit {
		var stored storedEntry
		if err := json.Unmarshal(scanner.Bytes(), &stored); err != nil {
			// Skip corrupted lines
			continue
		}

		if stored.Sequence >= fromSeq {
			entry := ScrollbackEntry{
				Timestamp: timeFromMillis(stored.Timestamp),
				Sequence:  stored.Sequence,
				Data:      []byte(stored.Data),
			}
			entries = append(entries, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading scrollback file: %w", err)
	}

	return entries, nil
}

// ReadTail retrieves the last N bytes from the scrollback file.
func (s *FileScrollbackStorage) ReadTail(sessionID string, bytes int64) ([]byte, error) {
	lock := s.getFileLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	filePath := s.getFilePath(sessionID)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open scrollback file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	size := stat.Size()
	if size == 0 {
		return nil, nil
	}

	// Determine read offset
	var offset int64
	if bytes >= size {
		offset = 0
	} else {
		offset = size - bytes
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek in file: %w", err)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// Truncate removes old entries to keep file size under limit.
func (s *FileScrollbackStorage) Truncate(sessionID string, keepBytes int64) error {
	lock := s.getFileLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	filePath := s.getFilePath(sessionID)

	stat, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to truncate
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if stat.Size() <= keepBytes {
		return nil // File is within limits
	}

	// Read entries, keep only recent ones
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file
	var gzipReader *gzip.Reader

	switch s.compressionType {
	case "gzip":
		gzipReader, err = gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	case "zstd":
		s.zstdDecoder.Reset(file)
		reader = s.zstdDecoder
	}

	// Read all entries
	scanner := bufio.NewScanner(reader)
	var allEntries []storedEntry
	for scanner.Scan() {
		var stored storedEntry
		if err := json.Unmarshal(scanner.Bytes(), &stored); err != nil {
			continue
		}
		allEntries = append(allEntries, stored)
	}

	if len(allEntries) == 0 {
		return nil
	}

	// Keep approximately the last keepBytes worth of entries
	// Estimate: each entry is roughly the same size
	estimatedEntrySize := stat.Size() / int64(len(allEntries))
	keepCount := int(keepBytes / estimatedEntrySize)
	if keepCount >= len(allEntries) {
		return nil // Keep all entries
	}

	startIndex := len(allEntries) - keepCount
	keptEntries := allEntries[startIndex:]

	// Write truncated file
	tempPath := filePath + ".tmp"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempPath)

	var writer io.Writer = tempFile
	var gzipWriter *gzip.Writer
	var zstdWriter *zstd.Encoder

	switch s.compressionType {
	case "gzip":
		gzipWriter = gzip.NewWriter(tempFile)
		writer = gzipWriter
	case "zstd":
		zstdWriter = s.zstdEncoder
		zstdWriter.Reset(tempFile)
		writer = zstdWriter
	}

	encoder := json.NewEncoder(writer)
	for _, entry := range keptEntries {
		if err := encoder.Encode(entry); err != nil {
			tempFile.Close()
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	if gzipWriter != nil {
		gzipWriter.Close()
	}
	if zstdWriter != nil {
		zstdWriter.Close()
	}
	tempFile.Close()

	// Replace original with truncated file
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("failed to replace file: %w", err)
	}

	return nil
}

// Delete removes the scrollback file for the session.
func (s *FileScrollbackStorage) Delete(sessionID string) error {
	lock := s.getFileLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	filePath := s.getFilePath(sessionID)

	err := os.Remove(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete scrollback file: %w", err)
	}

	return nil
}

// Size returns the current size of the scrollback file.
func (s *FileScrollbackStorage) Size(sessionID string) (int64, error) {
	filePath := s.getFilePath(sessionID)

	stat, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}

	return stat.Size(), nil
}

// Helper function to convert milliseconds to time.Time
func timeFromMillis(ms int64) time.Time {
	return time.Unix(ms/1000, (ms%1000)*1000000)
}
