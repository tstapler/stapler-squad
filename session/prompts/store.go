package prompts

import "github.com/linkdata/deadlock"

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxEntries = 500

// PromptEntry represents a single stored prompt with usage metadata.
type PromptEntry struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Label     string    `json:"label"`
	UsedCount int       `json:"used_count"`
	LastUsed  time.Time `json:"last_used"`
	CreatedAt time.Time `json:"created_at"`
}

// PromptStore manages a persistent JSON-backed collection of prompt entries.
type PromptStore struct {
	mu       deadlock.Mutex
	filePath string
}

// NewPromptStore returns a new PromptStore backed by the given file path.
func NewPromptStore(filePath string) *PromptStore {
	return &PromptStore{filePath: filePath}
}

// Load reads and deserializes the prompt entries from disk.
// Returns an empty (non-nil) slice and no error when the file does not exist.
func (s *PromptStore) Load() ([]PromptEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// load is the internal (unlocked) implementation of Load.
func (s *PromptStore) load() ([]PromptEntry, error) {
	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		return []PromptEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("prompts: read %s: %w", s.filePath, err)
	}

	var entries []PromptEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("prompts: unmarshal %s: %w", s.filePath, err)
	}
	if entries == nil {
		entries = []PromptEntry{}
	}
	return entries, nil
}

// Save atomically writes entries to disk using a temp file + os.Rename.
func (s *PromptStore) Save(entries []PromptEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(entries)
}

// save is the internal (unlocked) implementation of Save.
func (s *PromptStore) save(entries []PromptEntry) error {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
		return fmt.Errorf("prompts: mkdir %s: %w", filepath.Dir(s.filePath), err)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("prompts: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.filePath), ".prompts-*.tmp")
	if err != nil {
		return fmt.Errorf("prompts: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("prompts: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("prompts: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("prompts: rename temp file: %w", err)
	}
	return nil
}

// entryID computes a deterministic ID from the prompt text using SHA-256.
func entryID(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

// RecordUsage upserts a prompt entry by text, incrementing UsedCount and
// updating LastUsed. After upsert, the list is trimmed to 500 entries by
// evicting the entries with the oldest LastUsed timestamps.
func (s *PromptStore) RecordUsage(text string) (PromptEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return PromptEntry{}, err
	}

	id := entryID(text)
	now := time.Now()
	found := false

	for i, e := range entries {
		if e.ID == id {
			entries[i].UsedCount++
			entries[i].LastUsed = now
			found = true
			break
		}
	}

	if !found {
		entries = append(entries, PromptEntry{
			ID:        id,
			Text:      text,
			UsedCount: 1,
			LastUsed:  now,
			CreatedAt: now,
		})
	}

	// Trim to maxEntries by evicting oldest LastUsed entries.
	if len(entries) > maxEntries {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].LastUsed.After(entries[j].LastUsed)
		})
		entries = entries[:maxEntries]
	}

	if err := s.save(entries); err != nil {
		return PromptEntry{}, err
	}

	// Return the upserted entry.
	for _, e := range entries {
		if e.ID == id {
			return e, nil
		}
	}
	// Should not happen; id was evicted only if there were >500 entries all newer.
	return PromptEntry{}, fmt.Errorf("prompts: entry %s not found after save", id)
}

// List returns entries sorted by LastUsed descending. If limit <= 0, all
// entries are returned.
func (s *PromptStore) List(limit int) ([]PromptEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastUsed.After(entries[j].LastUsed)
	})

	if limit > 0 && limit < len(entries) {
		return entries[:limit], nil
	}
	return entries, nil
}

// Delete removes the entry with the given ID. It is not an error if the ID
// does not exist.
func (s *PromptStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return err
	}

	filtered := entries[:0]
	for _, e := range entries {
		if e.ID != id {
			filtered = append(filtered, e)
		}
	}

	return s.save(filtered)
}
