package mux

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RegistryEntry holds the metadata for one socket in the registry.
type RegistryEntry struct {
	SocketPath  string    `json:"socket_path"`
	SessionName string    `json:"session_name"`
	LastSeen    time.Time `json:"last_seen"`
}

// SocketRegistry is a file-backed map of session title → socket path for
// fast reconnection after a restart. It is safe for concurrent use.
//
// The backing file is located at {configDir}/mux-registry.json.
type SocketRegistry struct {
	path    string
	mu      sync.Mutex
	entries map[string]RegistryEntry // key: session title
}

// NewSocketRegistry creates (but does not load) a SocketRegistry backed by
// {configDir}/mux-registry.json.
func NewSocketRegistry(configDir string) *SocketRegistry {
	return &SocketRegistry{
		path:    filepath.Join(configDir, "mux-registry.json"),
		entries: make(map[string]RegistryEntry),
	}
}

// Load reads the registry from disk. If the file does not exist, the registry
// is left empty (not an error).
func (r *SocketRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("socket registry load: %w", err)
	}

	var entries map[string]RegistryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("socket registry parse: %w", err)
	}
	r.entries = entries
	return nil
}

// Save writes the current in-memory registry to disk atomically.
func (r *SocketRegistry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.saveLocked()
}

func (r *SocketRegistry) saveLocked() error {
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("socket registry marshal: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return fmt.Errorf("socket registry mkdir: %w", err)
	}

	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("socket registry write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, r.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("socket registry rename: %w", err)
	}
	return nil
}

// Set stores an entry and immediately persists the registry.
func (r *SocketRegistry) Set(title string, entry RegistryEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[title] = entry
	_ = r.saveLocked()
}

// Get returns the entry for the given title.
func (r *SocketRegistry) Get(title string) (*RegistryEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[title]
	if !ok {
		return nil, false
	}
	return &e, true
}

// Delete removes an entry and immediately persists the registry.
func (r *SocketRegistry) Delete(title string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, title)
	_ = r.saveLocked()
}

// PruneStale removes entries whose LastSeen is older than maxAge AND whose
// socket file no longer exists on disk. Persists after pruning.
func (r *SocketRegistry) PruneStale(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for title, entry := range r.entries {
		if entry.LastSeen.Before(cutoff) {
			if _, err := os.Stat(entry.SocketPath); os.IsNotExist(err) {
				delete(r.entries, title)
			}
		}
	}
	_ = r.saveLocked()
}
