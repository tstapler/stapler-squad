package fuzzy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"claude-squad/log"

	"github.com/sahilm/fuzzy"
)

// SimpleFuzzyEngine provides file search using sahilm/fuzzy instead of SQLite/FTS5
// This eliminates the FTS5 dependency while providing excellent fuzzy search performance
type SimpleFuzzyEngine struct {
	rootPath       string
	ignorePatterns []string
	maxResults     int
	maxDepth       int

	// In-memory file index
	files       []FileEntry
	indexedDirs map[string]time.Time
	mutex       sync.RWMutex

	// Configuration
	cacheTimeout   time.Duration
	minScore       float64
	caseSensitive  bool
	filenameBoost  float64
	pathMatchBoost float64
}

// FileEntry represents a file in the search index
type FileEntry struct {
	ID         int
	FullPath   string
	Filename   string
	ParentDirs string
	Directory  string
	IndexedAt  time.Time
}

// String implements the fuzzy.Source interface for sahilm/fuzzy
func (f FileEntry) String() string {
	// Return the full path for fuzzy matching
	return f.FullPath
}

// SimpleFuzzyConfig holds configuration for the simple fuzzy engine
type SimpleFuzzyConfig struct {
	RootPath       string
	IgnorePatterns []string
	MaxResults     int
	MaxDepth       int
	CacheTimeout   time.Duration
	MinScore       float64
	CaseSensitive  bool
	FilenameBoost  float64
	PathMatchBoost float64
}

// NewSimpleFuzzyEngine creates a new simple fuzzy search engine using sahilm/fuzzy
func NewSimpleFuzzyEngine(config SimpleFuzzyConfig) (*SimpleFuzzyEngine, error) {
	engine := &SimpleFuzzyEngine{
		rootPath:       config.RootPath,
		ignorePatterns: config.IgnorePatterns,
		maxResults:     config.MaxResults,
		maxDepth:       config.MaxDepth,
		indexedDirs:    make(map[string]time.Time),
		cacheTimeout:   config.CacheTimeout,
		minScore:       config.MinScore,
		caseSensitive:  config.CaseSensitive,
		filenameBoost:  config.FilenameBoost,
		pathMatchBoost: config.PathMatchBoost,
	}

	// Set reasonable defaults
	if engine.maxResults == 0 {
		engine.maxResults = 100
	}
	if engine.maxDepth == 0 {
		engine.maxDepth = 10
	}
	if engine.cacheTimeout == 0 {
		engine.cacheTimeout = 5 * time.Minute
	}
	if engine.filenameBoost == 0 {
		engine.filenameBoost = 2.0
	}
	if engine.pathMatchBoost == 0 {
		engine.pathMatchBoost = 1.5
	}

	log.InfoLog.Printf("Simple fuzzy search engine initialized (using sahilm/fuzzy)")
	return engine, nil
}

// IndexDirectory indexes all files in the specified directory
func (s *SimpleFuzzyEngine) IndexDirectory(dirPath string) error {
	startTime := time.Now()
	s.mutex.Lock()
	defer s.mutex.Unlock()

	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	var fileEntries []FileEntry
	fileCount := 0

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check ignore patterns
		for _, pattern := range s.ignorePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				return nil
			}
		}

		// Create file entry
		relPath, _ := filepath.Rel(absPath, path)
		parentDir := filepath.Dir(relPath)
		if parentDir == "." {
			parentDir = ""
		}

		entry := FileEntry{
			ID:         fileCount,
			FullPath:   path,
			Filename:   info.Name(),
			ParentDirs: parentDir,
			Directory:  filepath.Dir(path),
			IndexedAt:  time.Now(),
		}

		fileEntries = append(fileEntries, entry)
		fileCount++

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// Update the index
	s.files = fileEntries
	s.indexedDirs[absPath] = time.Now()

	duration := time.Since(startTime)
	log.InfoLog.Printf("Indexed %d files in %s (took %v)", fileCount, dirPath, duration)

	return nil
}

// Search performs fuzzy search using sahilm/fuzzy
func (s *SimpleFuzzyEngine) Search(query string, directories []string) ([]ZFSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return []ZFSearchResult{}, nil
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Filter files by directories if specified
	var searchFiles []FileEntry
	if len(directories) > 0 {
		dirSet := make(map[string]bool)
		for _, dir := range directories {
			dirSet[dir] = true
		}

		for _, file := range s.files {
			if dirSet[file.Directory] {
				searchFiles = append(searchFiles, file)
			}
		}
	} else {
		searchFiles = s.files
	}

	if len(searchFiles) == 0 {
		return []ZFSearchResult{}, nil
	}

	// Use sahilm/fuzzy to perform the search
	matches := fuzzy.FindFrom(query, FileEntrySlice(searchFiles))

	// Convert to ZFSearchResult format
	var results []ZFSearchResult
	for i, match := range matches {
		if i >= s.maxResults {
			break
		}

		fileEntry := searchFiles[match.Index]

		// Calculate score (sahilm/fuzzy doesn't provide scores, so we approximate)
		score := float64(len(query)) / float64(len(fileEntry.FullPath))
		if score > 1.0 {
			score = 1.0
		}

		// Boost filename matches
		if strings.Contains(strings.ToLower(fileEntry.Filename), strings.ToLower(query)) {
			score *= s.filenameBoost
		}

		result := ZFSearchResult{
			SearchResult: SearchResult{
				Item: &FileSearchItem{
					ID:       fileEntry.FullPath,
					FullPath: fileEntry.FullPath,
					Filename: fileEntry.Filename,
				},
				Score:   score,
				Matches: match.MatchedIndexes, // Use the matched indexes from sahilm/fuzzy
			},
			MatchType: MatchFuzzy, // All matches are fuzzy in this simple implementation
		}

		results = append(results, result)
	}

	// Sort by score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// Close cleans up resources (no-op for simple engine)
func (s *SimpleFuzzyEngine) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.files = nil
	s.indexedDirs = nil

	return nil
}

// GetStats returns search engine statistics
func (s *SimpleFuzzyEngine) GetStats() (map[string]interface{}, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return map[string]interface{}{
		"engine_type":      "simple_fuzzy",
		"total_files":      len(s.files),
		"indexed_dirs":     len(s.indexedDirs),
		"max_results":      s.maxResults,
		"max_depth":        s.maxDepth,
		"cache_timeout":    s.cacheTimeout.String(),
		"filename_boost":   s.filenameBoost,
		"path_match_boost": s.pathMatchBoost,
	}, nil
}

// DefaultSimpleFuzzyConfig returns sensible defaults for simple fuzzy search
func DefaultSimpleFuzzyConfig() SimpleFuzzyConfig {
	return SimpleFuzzyConfig{
		CacheTimeout:   24 * time.Hour, // Re-index directories daily
		MinScore:       0.2,
		MaxResults:     100,
		MaxDepth:       10,
		FilenameBoost:  2.0, // Filename matches get 2x score boost
		PathMatchBoost: 1.5, // Path matches get 1.5x score boost
		IgnorePatterns: []string{
			"*.git*",
			"node_modules",
			"*.tmp",
			"*.log",
			".DS_Store",
		},
	}
}

// FileEntrySlice implements fuzzy.Source interface for sahilm/fuzzy
type FileEntrySlice []FileEntry

func (s FileEntrySlice) String(i int) string {
	return s[i].FullPath
}

func (s FileEntrySlice) Len() int {
	return len(s)
}

