package fuzzy

import (
	"sort"
	"strings"
	"sync"
	"time"

	"claude-squad/log"
)

// SearchItem represents an item that can be searched using fuzzy search
type SearchItem interface {
	// GetSearchText returns the text used for fuzzy matching
	GetSearchText() string

	// GetDisplayText returns the text to display in the UI
	GetDisplayText() string

	// GetID returns a unique identifier for the item
	GetID() string
}

// SearchResult represents a result from a fuzzy search
type SearchResult struct {
	// Item is the original search item
	Item SearchItem

	// Score represents how well the item matched the query (higher is better)
	Score float64

	// Matches contains the indices of matching characters for highlighting
	Matches []int
}

// AsyncLoader is a function that loads search items asynchronously
type AsyncLoader func(query string) ([]SearchItem, error)

// FuzzySearcher handles fuzzy searching with async loading and debouncing
type FuzzySearcher struct {
	items         []SearchItem
	query         string
	results       []SearchResult
	debounceTimer *time.Timer
	asyncLoader   AsyncLoader
	loading       bool
	error         error

	// Configuration
	debounceMs int
	minScore   float64
	maxResults int

	// Thread safety
	mutex sync.RWMutex
}

// FuzzySearcherConfig contains configuration options for fuzzy search
type FuzzySearcherConfig struct {
	// How long to wait for additional keystrokes before triggering search (ms)
	DebounceMs int

	// Minimum score for a result to be included (0-1)
	MinScore float64

	// Maximum number of results to return
	MaxResults int
}

// DefaultConfig returns default configuration settings for fuzzy search
func DefaultConfig() FuzzySearcherConfig {
	return FuzzySearcherConfig{
		DebounceMs: 300,
		MinScore:   0.3,
		MaxResults: 50,
	}
}

// NewFuzzySearcher creates a new fuzzy searcher
func NewFuzzySearcher(config FuzzySearcherConfig) *FuzzySearcher {
	return &FuzzySearcher{
		items:      []SearchItem{},
		results:    []SearchResult{},
		debounceMs: config.DebounceMs,
		minScore:   config.MinScore,
		maxResults: config.MaxResults,
		mutex:      sync.RWMutex{},
	}
}

// SetItems sets the list of items to search
func (fs *FuzzySearcher) SetItems(items []SearchItem) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	fs.items = items
}

// GetItems returns the current list of items
func (fs *FuzzySearcher) GetItems() []SearchItem {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	return fs.items
}

// SetQuery updates the search query and triggers a debounced search
func (fs *FuzzySearcher) SetQuery(query string, callback func()) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	fs.query = query

	// Cancel any pending debounce timer
	if fs.debounceTimer != nil {
		fs.debounceTimer.Stop()
	}

	// Start a new debounce timer
	fs.debounceTimer = time.AfterFunc(time.Duration(fs.debounceMs)*time.Millisecond, func() {
		fs.performSearch()
		if callback != nil {
			callback()
		}
	})
}

// SetAsyncLoader sets a function to load items asynchronously
func (fs *FuzzySearcher) SetAsyncLoader(loader AsyncLoader) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	fs.asyncLoader = loader
}

// GetQuery returns the current search query
func (fs *FuzzySearcher) GetQuery() string {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	return fs.query
}

// GetResults returns the current search results
func (fs *FuzzySearcher) GetResults() []SearchResult {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	return fs.results
}

// IsLoading returns whether an async load is in progress
func (fs *FuzzySearcher) IsLoading() bool {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	return fs.loading
}

// GetError returns any error from the last search
func (fs *FuzzySearcher) GetError() error {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	return fs.error
}

// performSearch executes the fuzzy search algorithm
func (fs *FuzzySearcher) performSearch() {
	fs.mutex.Lock()

	// Get the query while we hold the lock
	query := fs.query

	// If we have an async loader, use it
	if fs.asyncLoader != nil {
		fs.loading = true
		fs.mutex.Unlock()

		// Execute the loader in a goroutine
		go func() {
			items, err := fs.asyncLoader(query)

			fs.mutex.Lock()
			defer fs.mutex.Unlock()

			fs.loading = false
			if err != nil {
				fs.error = err
				log.ErrorLog.Printf("Error in async fuzzy search loader: %v", err)
				return
			}

			fs.items = items
			fs.searchItems(query)
		}()
		return
	}

	// Otherwise search the existing items
	defer fs.mutex.Unlock()
	fs.searchItems(query)
}

// searchItems performs the actual fuzzy search on items
func (fs *FuzzySearcher) searchItems(query string) {
	if query == "" {
		// Empty query returns all items in original order
		fs.results = make([]SearchResult, len(fs.items))
		for i, item := range fs.items {
			fs.results[i] = SearchResult{
				Item:    item,
				Score:   1.0,
				Matches: []int{},
			}
		}
		return
	}

	// Search each item
	results := make([]SearchResult, 0, len(fs.items))
	for _, item := range fs.items {
		score, matches := fuzzyMatch(query, item.GetSearchText())
		if score >= fs.minScore {
			results = append(results, SearchResult{
				Item:    item,
				Score:   score,
				Matches: matches,
			})
		}
	}

	// Sort results by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit number of results
	if len(results) > fs.maxResults {
		results = results[:fs.maxResults]
	}

	fs.results = results
}

// fuzzyMatch calculates a score between 0 and 1 for how well the pattern matches the text
// It also returns the indices of matching characters for highlighting
func fuzzyMatch(pattern, text string) (float64, []int) {
	// Simple case: empty pattern
	if len(pattern) == 0 {
		return 1.0, []int{}
	}

	// Simple case: exact match
	if pattern == text {
		matches := make([]int, len(pattern))
		for i := range pattern {
			matches[i] = i
		}
		return 1.0, matches
	}

	// Convert to lowercase for case-insensitive matching
	patternLower := strings.ToLower(pattern)
	textLower := strings.ToLower(text)

	// Simple case: prefix match (heavily weighted)
	if strings.HasPrefix(textLower, patternLower) {
		matches := make([]int, len(pattern))
		for i := range pattern {
			matches[i] = i
		}
		return 0.9, matches
	}

	// Simple case: contains match (moderately weighted)
	if strings.Contains(textLower, patternLower) {
		index := strings.Index(textLower, patternLower)
		matches := make([]int, len(pattern))
		for i := range pattern {
			matches[i] = index + i
		}
		return 0.8, matches
	}

	// Fuzzy matching - find each pattern character in the text
	matches := make([]int, 0, len(pattern))

	// Algorithm: Find matches of pattern characters in order with smallest gaps
	var i, j int
	for i < len(pattern) && j < len(text) {
		if strings.ToLower(string(pattern[i])) == strings.ToLower(string(text[j])) {
			matches = append(matches, j)
			i++
		}
		j++
	}

	// If we didn't match all pattern chars, no match
	if i < len(pattern) {
		return 0.0, []int{}
	}

	// Calculate score based on:
	// 1. Percentage of matched characters
	// 2. How consecutive the matches are (fewer gaps = better)
	// 3. Position of first match (earlier = better)

	matchRatio := float64(len(pattern)) / float64(len(text))

	// Calculate gaps between matches
	gapPenalty := 0.0
	for i := 1; i < len(matches); i++ {
		gap := matches[i] - matches[i-1] - 1
		if gap > 0 {
			gapPenalty += float64(gap) / float64(len(text))
		}
	}

	// Position bonus - earlier matches are better
	positionBonus := 0.0
	if len(matches) > 0 {
		positionBonus = 0.1 * (1.0 - float64(matches[0])/float64(len(text)))
	}

	// Calculate final score
	score := matchRatio - gapPenalty + positionBonus

	// Normalize score to 0-1 range
	score = score * 0.7 // Scale down fuzzy matches compared to prefix/exact

	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}

	return score, matches
}

// BasicStringItem is a simple implementation of SearchItem for string-only items
type BasicStringItem struct {
	ID   string
	Text string
}

func (i BasicStringItem) GetSearchText() string {
	return i.Text
}

func (i BasicStringItem) GetDisplayText() string {
	return i.Text
}

func (i BasicStringItem) GetID() string {
	return i.ID
}

// NewBasicStringItems creates a slice of BasicStringItem from a slice of strings
func NewBasicStringItems(items []string) []SearchItem {
	result := make([]SearchItem, len(items))
	for i, item := range items {
		result[i] = BasicStringItem{
			ID:   item,
			Text: item,
		}
	}
	return result
}
