package services

import (
	"claude-squad/ui"
	"sync"
)

// FilteringService handles search and filter operations with thread-safe state management
type FilteringService interface {
	// Search operations
	StartSearch() error
	UpdateSearchQuery(query string) error
	ClearSearch() error
	IsSearchActive() bool
	GetSearchQuery() string

	// Filter operations
	TogglePausedFilter() error
	IsPausedFilterActive() bool

	// Combined state query
	GetFilterState() FilterState
}

// FilterState represents the current filtering state
type FilterState struct {
	SearchActive       bool
	SearchQuery        string
	PausedFilterActive bool
}

// filteringService implements FilteringService
type filteringService struct {
	mu                 sync.Mutex
	list               *ui.List
	searchActive       bool
	searchQuery        string
	pausedFilterActive bool
}

// NewFilteringService creates a new filtering service
func NewFilteringService(list *ui.List) FilteringService {
	return &filteringService{
		list:               list,
		searchActive:       false,
		searchQuery:        "",
		pausedFilterActive: false,
	}
}

// StartSearch activates search mode
func (f *filteringService) StartSearch() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.searchActive = true
	return nil
}

// UpdateSearchQuery updates the search query and applies it to the list
func (f *filteringService) UpdateSearchQuery(query string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.searchQuery = query
	f.list.SearchByTitle(query)
	return nil
}

// ClearSearch deactivates search mode and clears the query
func (f *filteringService) ClearSearch() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.searchActive = false
	f.searchQuery = ""
	f.list.ExitSearchMode()
	return nil
}

// IsSearchActive returns whether search mode is active
func (f *filteringService) IsSearchActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.searchActive
}

// GetSearchQuery returns the current search query
func (f *filteringService) GetSearchQuery() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.searchQuery
}

// TogglePausedFilter toggles the paused session filter
func (f *filteringService) TogglePausedFilter() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pausedFilterActive = !f.pausedFilterActive
	f.list.TogglePausedFilter()
	return nil
}

// IsPausedFilterActive returns whether the paused filter is active
func (f *filteringService) IsPausedFilterActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.pausedFilterActive
}

// GetFilterState returns the complete filtering state
func (f *filteringService) GetFilterState() FilterState {
	f.mu.Lock()
	defer f.mu.Unlock()

	return FilterState{
		SearchActive:       f.searchActive,
		SearchQuery:        f.searchQuery,
		PausedFilterActive: f.pausedFilterActive,
	}
}
