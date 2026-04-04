package analytics

import (
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// EscapeCodeEntry represents a tracked escape sequence
type EscapeCodeEntry struct {
	Code          string         `json:"code"`          // Hex-encoded sequence
	HumanReadable string         `json:"humanReadable"` // Description
	Category      EscapeCategory `json:"category"`      // Type of sequence
	Count         int64          `json:"count"`         // How many times seen
	FirstSeen     time.Time      `json:"firstSeen"`     // First occurrence
	LastSeen      time.Time      `json:"lastSeen"`      // Most recent occurrence
	SessionIDs    []string       `json:"sessionIds"`    // Sessions that produced this code
}

// EscapeCodeStats provides aggregated statistics
type EscapeCodeStats struct {
	Enabled        bool                     `json:"enabled"`
	TotalCodes     int64                    `json:"totalCodes"`     // Total codes recorded
	UniqueCodes    int                      `json:"uniqueCodes"`    // Unique sequences
	CategoryCounts map[EscapeCategory]int64 `json:"categoryCounts"` // Count by category
	TopCodes       []EscapeCodeEntry        `json:"topCodes"`       // Most frequent codes
	RecentCodes    []EscapeCodeEntry        `json:"recentCodes"`    // Recently seen codes
}

// EscapeCodeStore provides thread-safe storage for escape code analytics
type EscapeCodeStore struct {
	mu         sync.RWMutex
	enabled    bool
	entries    map[string]*EscapeCodeEntry // key: hex-encoded code
	totalCount int64
	maxEntries int // Limit to prevent unbounded growth
}

// NewEscapeCodeStore creates a new store with default settings
func NewEscapeCodeStore() *EscapeCodeStore {
	return &EscapeCodeStore{
		enabled:    false,
		entries:    make(map[string]*EscapeCodeEntry),
		maxEntries: 10000, // Reasonable limit
	}
}

// SetEnabled enables or disables tracking
func (s *EscapeCodeStore) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// IsEnabled returns whether tracking is enabled
func (s *EscapeCodeStore) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// Record adds or updates an escape code entry
func (s *EscapeCodeStore) Record(sessionID string, rawBytes []byte, category EscapeCategory, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled {
		return
	}

	hexCode := hex.EncodeToString(rawBytes)
	now := time.Now()
	s.totalCount++

	entry, exists := s.entries[hexCode]
	if exists {
		entry.Count++
		entry.LastSeen = now
		// Track session if not already tracked (limit to 100 sessions per code)
		if len(entry.SessionIDs) < 100 {
			found := false
			for _, id := range entry.SessionIDs {
				if id == sessionID {
					found = true
					break
				}
			}
			if !found {
				entry.SessionIDs = append(entry.SessionIDs, sessionID)
			}
		}
	} else {
		// Check if we need to evict old entries
		if len(s.entries) >= s.maxEntries {
			s.evictOldEntries()
		}

		s.entries[hexCode] = &EscapeCodeEntry{
			Code:          hexCode,
			HumanReadable: description,
			Category:      category,
			Count:         1,
			FirstSeen:     now,
			LastSeen:      now,
			SessionIDs:    []string{sessionID},
		}
	}
}

// evictOldEntries removes the least recently used entries when at capacity
// Must be called with lock held
func (s *EscapeCodeStore) evictOldEntries() {
	// Keep entries with high counts and recent usage
	type scored struct {
		key   string
		score float64
	}

	var entries []scored
	now := time.Now()

	for key, entry := range s.entries {
		// Score based on count and recency
		ageSecs := now.Sub(entry.LastSeen).Seconds()
		score := float64(entry.Count) / (ageSecs + 1)
		entries = append(entries, scored{key, score})
	}

	// Sort by score (lowest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score < entries[j].score
	})

	// Remove bottom 10%
	removeCount := len(entries) / 10
	if removeCount < 1 {
		removeCount = 1
	}

	for i := 0; i < removeCount && i < len(entries); i++ {
		delete(s.entries, entries[i].key)
	}
}

// GetAll returns all entries
func (s *EscapeCodeStore) GetAll() []EscapeCodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]EscapeCodeEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		result = append(result, *entry)
	}

	// Sort by count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result
}

// GetBySession returns entries for a specific session
func (s *EscapeCodeStore) GetBySession(sessionID string) []EscapeCodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []EscapeCodeEntry
	for _, entry := range s.entries {
		for _, id := range entry.SessionIDs {
			if id == sessionID {
				result = append(result, *entry)
				break
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result
}

// GetByCategory returns entries for a specific category
func (s *EscapeCodeStore) GetByCategory(category EscapeCategory) []EscapeCodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []EscapeCodeEntry
	for _, entry := range s.entries {
		if entry.Category == category {
			result = append(result, *entry)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result
}

// GetStats returns aggregated statistics
func (s *EscapeCodeStore) GetStats() EscapeCodeStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := EscapeCodeStats{
		Enabled:        s.enabled,
		TotalCodes:     s.totalCount,
		UniqueCodes:    len(s.entries),
		CategoryCounts: make(map[EscapeCategory]int64),
	}

	// Calculate category counts
	for _, entry := range s.entries {
		stats.CategoryCounts[entry.Category] += entry.Count
	}

	// Get top codes by count
	allEntries := make([]EscapeCodeEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		allEntries = append(allEntries, *entry)
	}

	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Count > allEntries[j].Count
	})

	if len(allEntries) > 20 {
		stats.TopCodes = allEntries[:20]
	} else {
		stats.TopCodes = allEntries
	}

	// Get recent codes by LastSeen
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].LastSeen.After(allEntries[j].LastSeen)
	})

	if len(allEntries) > 10 {
		stats.RecentCodes = allEntries[:10]
	} else {
		stats.RecentCodes = allEntries
	}

	return stats
}

// Clear removes all entries
func (s *EscapeCodeStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = make(map[string]*EscapeCodeEntry)
	s.totalCount = 0
}

// Export returns all data as JSON
func (s *EscapeCodeStore) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := struct {
		Enabled    bool              `json:"enabled"`
		TotalCount int64             `json:"totalCount"`
		Entries    []EscapeCodeEntry `json:"entries"`
		ExportedAt time.Time         `json:"exportedAt"`
	}{
		Enabled:    s.enabled,
		TotalCount: s.totalCount,
		ExportedAt: time.Now(),
	}

	for _, entry := range s.entries {
		data.Entries = append(data.Entries, *entry)
	}

	// Sort by count
	sort.Slice(data.Entries, func(i, j int) bool {
		return data.Entries[i].Count > data.Entries[j].Count
	})

	return json.MarshalIndent(data, "", "  ")
}

// Global store instance
var globalStore *EscapeCodeStore
var globalStoreMu sync.Once

// GetGlobalStore returns the singleton escape code store
func GetGlobalStore() *EscapeCodeStore {
	globalStoreMu.Do(func() {
		globalStore = NewEscapeCodeStore()
	})
	return globalStore
}
