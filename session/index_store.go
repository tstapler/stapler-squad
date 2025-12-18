package session

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IndexStore handles persistence of the search index to disk.
// It uses Gob encoding for efficient storage and atomic writes for safety.
type IndexStore struct {
	indexDir string
	mu       sync.Mutex
}

// IndexVersion tracks metadata about the persisted index.
type IndexVersion struct {
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	DocumentCount int       `json:"document_count"`
	TermCount    int       `json:"term_count"`
}

const (
	// CurrentIndexVersion is the schema version for the index format
	CurrentIndexVersion = 1

	// Index file names
	invertedIndexFile = "inverted_index.gob"
	docStoreFile      = "doc_store.gob"
	versionFile       = "index_version.json"
	syncMetadataFile  = "sync_metadata.json"
)

// NewIndexStore creates a new IndexStore that persists to ~/.claude/search_index/
func NewIndexStore() (*IndexStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	indexDir := filepath.Join(home, ".claude", "search_index")
	return NewIndexStoreWithDir(indexDir)
}

// NewIndexStoreWithDir creates an IndexStore with a custom directory.
// Useful for testing.
func NewIndexStoreWithDir(indexDir string) (*IndexStore, error) {
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create index directory: %w", err)
	}

	return &IndexStore{
		indexDir: indexDir,
	}, nil
}

// Save persists the inverted index and document store to disk.
// Uses atomic writes (write to temp file, then rename) to prevent corruption.
func (s *IndexStore) Save(index *InvertedIndex, docStore *DocumentStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Save inverted index
	if err := s.saveGob(invertedIndexFile, index); err != nil {
		return fmt.Errorf("failed to save inverted index: %w", err)
	}

	// Save document store
	if err := s.saveGob(docStoreFile, docStore); err != nil {
		return fmt.Errorf("failed to save document store: %w", err)
	}

	// Save version metadata
	version := IndexVersion{
		Version:       CurrentIndexVersion,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		DocumentCount: index.GetTotalDocs(),
		TermCount:     index.GetTermCount(),
	}
	if err := s.saveVersion(version); err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}

	return nil
}

// Load reads the inverted index and document store from disk.
// Returns error if files don't exist or are corrupted.
func (s *IndexStore) Load() (*InvertedIndex, *DocumentStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check version compatibility
	version, err := s.loadVersion()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load version: %w", err)
	}
	if version.Version != CurrentIndexVersion {
		return nil, nil, fmt.Errorf("incompatible index version: got %d, want %d",
			version.Version, CurrentIndexVersion)
	}

	// Load inverted index
	var index InvertedIndex
	if err := s.loadGob(invertedIndexFile, &index); err != nil {
		return nil, nil, fmt.Errorf("failed to load inverted index: %w", err)
	}

	// Load document store
	var docStore DocumentStore
	if err := s.loadGob(docStoreFile, &docStore); err != nil {
		return nil, nil, fmt.Errorf("failed to load document store: %w", err)
	}

	return &index, &docStore, nil
}

// Exists returns true if a persisted index exists.
func (s *IndexStore) Exists() bool {
	versionPath := filepath.Join(s.indexDir, versionFile)
	_, err := os.Stat(versionPath)
	return err == nil
}

// GetVersion returns the version metadata of the persisted index.
func (s *IndexStore) GetVersion() (*IndexVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadVersion()
}

// Delete removes all persisted index files including sync metadata.
func (s *IndexStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	files := []string{invertedIndexFile, docStoreFile, versionFile, syncMetadataFile}
	for _, file := range files {
		path := filepath.Join(s.indexDir, file)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete %s: %w", file, err)
		}
	}

	return nil
}

// GetIndexDir returns the directory where index files are stored.
func (s *IndexStore) GetIndexDir() string {
	return s.indexDir
}

// saveGob writes a struct to a gob file using atomic write pattern.
func (s *IndexStore) saveGob(filename string, data interface{}) error {
	finalPath := filepath.Join(s.indexDir, filename)
	tempPath := finalPath + ".tmp"

	// Write to temp file
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(data); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return err
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}

	// Atomic rename
	if err := os.Rename(tempPath, finalPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

// loadGob reads a struct from a gob file.
func (s *IndexStore) loadGob(filename string, data interface{}) error {
	path := filepath.Join(s.indexDir, filename)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	return decoder.Decode(data)
}

// saveVersion writes version metadata as JSON.
func (s *IndexStore) saveVersion(version IndexVersion) error {
	path := filepath.Join(s.indexDir, versionFile)
	tempPath := path + ".tmp"

	data, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

// loadVersion reads version metadata from JSON.
func (s *IndexStore) loadVersion() (*IndexVersion, error) {
	path := filepath.Join(s.indexDir, versionFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var version IndexVersion
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, err
	}

	return &version, nil
}

// SaveSyncMetadata persists the sync metadata to disk using atomic write.
func (s *IndexStore) SaveSyncMetadata(meta *IndexSyncMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.indexDir, syncMetadataFile)
	tempPath := path + ".tmp"

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync metadata: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write sync metadata temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename sync metadata file: %w", err)
	}

	return nil
}

// LoadSyncMetadata reads the sync metadata from disk.
// Returns nil, nil if metadata doesn't exist (fresh index).
func (s *IndexStore) LoadSyncMetadata() (*IndexSyncMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.indexDir, syncMetadataFile)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil // No metadata yet, this is fine
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read sync metadata: %w", err)
	}

	var meta IndexSyncMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync metadata: %w", err)
	}

	// Validate version
	if meta.Version != CurrentSyncMetadataVersion {
		return nil, fmt.Errorf("incompatible sync metadata version: got %d, want %d",
			meta.Version, CurrentSyncMetadataVersion)
	}

	return &meta, nil
}

// SyncMetadataExists returns true if sync metadata exists on disk.
func (s *IndexStore) SyncMetadataExists() bool {
	path := filepath.Join(s.indexDir, syncMetadataFile)
	_, err := os.Stat(path)
	return err == nil
}

// DeleteSyncMetadata removes the sync metadata file.
func (s *IndexStore) DeleteSyncMetadata() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.indexDir, syncMetadataFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete sync metadata: %w", err)
	}
	return nil
}
