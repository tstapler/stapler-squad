package config

import (
	"claude-squad/log"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	
	"github.com/gofrs/flock"
)

const (
	StateFileName     = "state.json"
	InstancesFileName = "instances.json"
)

// InstanceStorage handles instance-related operations
type InstanceStorage interface {
	// SaveInstances saves the raw instance data
	SaveInstances(instancesJSON json.RawMessage) error
	// GetInstances returns the raw instance data
	GetInstances() json.RawMessage
	// DeleteAllInstances removes all stored instances
	DeleteAllInstances() error
}

// AppState handles application-level state
type AppState interface {
	// GetHelpScreensSeen returns the bitmask of seen help screens
	GetHelpScreensSeen() uint32
	// SetHelpScreensSeen updates the bitmask of seen help screens
	SetHelpScreensSeen(seen uint32) error
}

// StateManager combines instance storage and app state management
type StateManager interface {
	InstanceStorage
	AppState
	
	// RefreshState reloads state from disk to detect changes made by other processes
	RefreshState() error
	
	// Close releases any resources held by the state manager
	Close() error
}

// UIState represents UI preferences that persist between sessions
type UIState struct {
	// HidePaused controls whether paused sessions are filtered out
	HidePaused bool `json:"hide_paused"`
	// CategoryExpanded maps category names to their expanded state
	CategoryExpanded map[string]bool `json:"category_expanded"`
	// SearchMode tracks if search mode was active
	SearchMode bool `json:"search_mode"`
	// SearchQuery holds the last search query
	SearchQuery string `json:"search_query"`
	// SelectedIdx tracks the last selected session index
	SelectedIdx int `json:"selected_idx"`
}

// State represents the application state that persists between sessions
type State struct {
	// HelpScreensSeen is a bitmask tracking which help screens have been shown
	HelpScreensSeen uint32 `json:"help_screens_seen"`
	// Instances stores the serialized instance data as raw JSON
	InstancesData json.RawMessage `json:"instances"`
	// UI stores the UI preferences and state
	UI UIState `json:"ui"`
	
	// Lock file for coordinating state access across processes
	lockFile    *flock.Flock `json:"-"` // Not serialized
	lockTimeout time.Duration `json:"-"` // Not serialized
}

const (
	// DefaultLockTimeout is the default timeout for acquiring locks
	DefaultLockTimeout = 5 * time.Second
	// LockFileName is the name of the lock file
	LockFileName = "state.lock"
)

// DefaultState returns the default state
func DefaultState() *State {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		// Return a minimal state without locking if we can't get the config dir
		return &State{
			HelpScreensSeen: 0,
			InstancesData:   json.RawMessage("[]"),
			UI: UIState{
				HidePaused:       false,
				CategoryExpanded: make(map[string]bool),
				SearchMode:       false,
				SearchQuery:      "",
				SelectedIdx:      0,
			},
		}
	}
	
	// Initialize the lock file
	lockPath := filepath.Join(configDir, LockFileName)
	fileLock := flock.New(lockPath)
	
	return &State{
		HelpScreensSeen: 0,
		InstancesData:   json.RawMessage("[]"),
		UI: UIState{
			HidePaused:       false,
			CategoryExpanded: make(map[string]bool),
			SearchMode:       false,
			SearchQuery:      "",
			SelectedIdx:      0,
		},
		lockFile:        fileLock,
		lockTimeout:     DefaultLockTimeout,
	}
}

// LoadState loads the state from disk with locking. If it cannot be done, we return the default state.
func LoadState() *State {
	// Get the default state which includes locking capabilities
	state := DefaultState()
	
	// Attempt to load from disk with a shared read lock
	if err := state.loadFromDisk(); err != nil {
		log.WarningLog.Printf("failed to load state from disk: %v", err)
		// We already have the default state, so just continue
	}
	
	return state
}

// loadFromDisk loads state from disk with a shared read lock
func (s *State) loadFromDisk() error {
	// Skip if we don't have a lock file initialized
	if s.lockFile == nil {
		log.WarningLog.Printf("lock file not initialized, loading state without locking")
		return s.loadFromDiskWithoutLocking()
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), s.lockTimeout)
	defer cancel()
	
	// Try to acquire a shared read lock with retries
	locked, err := s.lockFile.TryRLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire read lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire read lock within timeout")
	}
	defer s.lockFile.Unlock()
	
	// Now that we have a lock, load the state
	return s.loadFromDiskWithoutLocking()
}

// loadFromDiskWithoutLocking loads state from disk without locking
// This is used internally by loadFromDisk after acquiring a lock
func (s *State) loadFromDiskWithoutLocking() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	
	statePath := filepath.Join(configDir, StateFileName)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet - keep the default state
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}
	
	// Parse the state file
	var newState State
	if err := json.Unmarshal(data, &newState); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}
	
	// Update our fields but keep the lock file and timeout
	s.HelpScreensSeen = newState.HelpScreensSeen
	s.InstancesData = newState.InstancesData
	
	// Update UI state, ensuring CategoryExpanded map is initialized
	s.UI = newState.UI
	if s.UI.CategoryExpanded == nil {
		s.UI.CategoryExpanded = make(map[string]bool)
	}
	
	return nil
}

// No longer needed as LoadState now includes locking

// SaveState saves the state to disk with locking
func SaveState(state *State) error {
	// Try to merge with any existing state on disk before saving
	if err := state.mergeWithExistingState(); err != nil {
		log.WarningLog.Printf("failed to merge with existing state: %v", err)
		// Continue with save anyway
	}
	
	return state.saveToDisk()
}

// mergeWithExistingState loads existing state from disk and merges it with current state
// This ensures we don't overwrite changes made by other processes
func (s *State) mergeWithExistingState() error {
	// Create a new state to load from disk
	diskState := DefaultState()
	
	// Try to load existing state from disk
	if err := diskState.loadFromDisk(); err != nil {
		return fmt.Errorf("failed to load existing state for merging: %w", err)
	}
	
	// Merge instances by deserializing both sets
	var ourInstances []json.RawMessage
	var diskInstances []json.RawMessage
	
	// Deserialize our instances
	if len(s.InstancesData) > 0 {
		if err := json.Unmarshal(s.InstancesData, &ourInstances); err != nil {
			return fmt.Errorf("failed to unmarshal our instances: %w", err)
		}
	}
	
	// Deserialize disk instances
	if len(diskState.InstancesData) > 0 {
		if err := json.Unmarshal(diskState.InstancesData, &diskInstances); err != nil {
			return fmt.Errorf("failed to unmarshal disk instances: %w", err)
		}
	}
	
	// If we don't have any instances, just use the disk instances
	if len(ourInstances) == 0 {
		s.InstancesData = diskState.InstancesData
		return nil
	}
	
	// If there are no disk instances, we don't need to merge
	if len(diskInstances) == 0 {
		return nil
	}
	
	// For merging, create maps to identify instances by "title" field
	ourInstanceMap := make(map[string]json.RawMessage)
	diskInstanceMap := make(map[string]json.RawMessage)
	
	// Build map of our instances by title
	for _, instance := range ourInstances {
		// Parse instance to get title
		var parsed struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(instance, &parsed); err != nil {
			log.WarningLog.Printf("failed to parse instance: %v", err)
			continue
		}
		if parsed.Title != "" {
			ourInstanceMap[parsed.Title] = instance
		}
	}
	
	// Build map of disk instances by title
	for _, instance := range diskInstances {
		// Parse instance to get title
		var parsed struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(instance, &parsed); err != nil {
			log.WarningLog.Printf("failed to parse disk instance: %v", err)
			continue
		}
		if parsed.Title != "" && !containsInstance(ourInstances, parsed.Title) {
			// Only add disk instances that aren't already in our instances
			diskInstanceMap[parsed.Title] = instance
		}
	}
	
	// Now create a merged list with both sets of instances
	mergedInstances := ourInstances // Start with our instances
	
	// Add disk instances that aren't in our instances
	for title, instance := range diskInstanceMap {
		if _, exists := ourInstanceMap[title]; !exists {
			log.InfoLog.Printf("merging instance from disk: %s", title)
			mergedInstances = append(mergedInstances, instance)
		}
	}
	
	// Re-serialize the merged instances
	mergedData, err := json.Marshal(mergedInstances)
	if err != nil {
		return fmt.Errorf("failed to marshal merged instances: %w", err)
	}
	
	// Update our instances
	s.InstancesData = mergedData
	
	return nil
}

// containsInstance checks if instances contains an instance with the given title
func containsInstance(instances []json.RawMessage, title string) bool {
	for _, instance := range instances {
		var parsed struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(instance, &parsed); err != nil {
			continue
		}
		if parsed.Title == title {
			return true
		}
	}
	return false
}

// saveToDisk saves state to disk with an exclusive write lock
func (s *State) saveToDisk() error {
	// Skip locking if lock file isn't initialized
	if s.lockFile == nil {
		log.WarningLog.Printf("lock file not initialized, saving state without locking")
		return s.saveToDiskWithoutLocking()
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), s.lockTimeout)
	defer cancel()
	
	// Try to acquire an exclusive write lock with retries
	locked, err := s.lockFile.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire write lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire write lock within timeout")
	}
	defer s.lockFile.Unlock()
	
	// Now that we have a lock, save the state
	return s.saveToDiskWithoutLocking()
}

// saveToDiskWithoutLocking saves state to disk without locking
// This is used internally by saveToDisk after acquiring a lock
func (s *State) saveToDiskWithoutLocking() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	statePath := filepath.Join(configDir, StateFileName)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	
	// Write to a temporary file first to ensure atomicity
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary state file: %w", err)
	}
	
	// Atomically rename the temporary file to the actual file
	if err := os.Rename(tmpPath, statePath); err != nil {
		// Try to clean up the temporary file
		os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically update state file: %w", err)
	}
	
	return nil
}

// InstanceStorage interface implementation

// SaveInstances saves the raw instance data with locking and merging
func (s *State) SaveInstances(instancesJSON json.RawMessage) error {
	s.InstancesData = instancesJSON
	return SaveState(s)
}

// GetInstances returns the raw instance data after refreshing from disk
func (s *State) GetInstances() json.RawMessage {
	// Refresh state from disk first to get latest changes
	if err := s.RefreshState(); err != nil {
		log.WarningLog.Printf("failed to refresh state: %v", err)
		// Continue with current state
	}
	return s.InstancesData
}

// DeleteAllInstances removes all stored instances with locking
func (s *State) DeleteAllInstances() error {
	s.InstancesData = json.RawMessage("[]")
	return SaveState(s)
}

// AppState interface implementation

// GetHelpScreensSeen returns the bitmask of seen help screens
func (s *State) GetHelpScreensSeen() uint32 {
	return s.HelpScreensSeen
}

// SetHelpScreensSeen updates the bitmask of seen help screens
func (s *State) SetHelpScreensSeen(seen uint32) error {
	s.HelpScreensSeen = seen
	return SaveState(s)
}

// RefreshState reloads state from disk with locking
func (s *State) RefreshState() error {
	return s.loadFromDisk()
}

// Close releases any locks held by this state
func (s *State) Close() error {
	if s.lockFile != nil {
		return s.lockFile.Unlock()
	}
	return nil
}

// UI State Management Methods

// GetUIState returns a copy of the current UI state
func (s *State) GetUIState() UIState {
	// Refresh from disk first to get latest changes
	if err := s.RefreshState(); err != nil {
		log.WarningLog.Printf("failed to refresh UI state: %v", err)
	}
	return s.UI
}

// SetHidePaused updates the hide paused filter state
func (s *State) SetHidePaused(hidePaused bool) error {
	s.UI.HidePaused = hidePaused
	return SaveState(s)
}

// SetCategoryExpanded updates the expanded state for a category
func (s *State) SetCategoryExpanded(category string, expanded bool) error {
	if s.UI.CategoryExpanded == nil {
		s.UI.CategoryExpanded = make(map[string]bool)
	}
	s.UI.CategoryExpanded[category] = expanded
	return SaveState(s)
}

// GetCategoryExpanded returns whether a category is expanded (defaults to true for new categories)
func (s *State) GetCategoryExpanded(category string) bool {
	if s.UI.CategoryExpanded == nil {
		return true // Default to expanded for new categories
	}
	expanded, exists := s.UI.CategoryExpanded[category]
	if !exists {
		return true // Default to expanded for new categories
	}
	return expanded
}

// SetSearchMode updates the search mode state
func (s *State) SetSearchMode(searchMode bool, query string) error {
	s.UI.SearchMode = searchMode
	s.UI.SearchQuery = query
	return SaveState(s)
}

// GetSearchState returns the current search mode and query
func (s *State) GetSearchState() (bool, string) {
	return s.UI.SearchMode, s.UI.SearchQuery
}

// SetSelectedIndex updates the selected session index
func (s *State) SetSelectedIndex(index int) error {
	s.UI.SelectedIdx = index
	return SaveState(s)
}

// GetSelectedIndex returns the last selected session index
func (s *State) GetSelectedIndex() int {
	return s.UI.SelectedIdx
}
