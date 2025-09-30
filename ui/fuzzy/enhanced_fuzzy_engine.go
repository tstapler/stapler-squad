package fuzzy

import (
	"encoding/gob"
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

// EnhancedFuzzyEngine provides ZF-style search without SQLite dependency
// Uses sahilm/fuzzy with intelligent indexing for performance
type EnhancedFuzzyEngine struct {
	// Multi-level indexing for performance
	filenameIndex map[string][]int    // filename -> file indices
	pathIndex     map[string][]int    // path segment -> file indices
	fullIndex     []EnhancedFileEntry // complete file list

	// Configuration
	rootPath       string
	ignorePatterns []string
	maxResults     int
	maxDepth       int
	filenameBoost  float64
	pathBoost      float64
	minScore       float64

	// Performance optimization
	queryCache    map[string][]ZFSearchResult
	cacheTimeout  time.Duration
	lastCacheTime time.Time

	// Persistence
	indexFile     string
	lastModTime   time.Time
	indexedDirs   map[string]time.Time

	// Concurrency
	mutex sync.RWMutex
}

// EnhancedFileEntry represents an indexed file with search metadata
type EnhancedFileEntry struct {
	ID         int
	FullPath   string
	Filename   string
	ParentDirs string
	Directory  string
	Extension  string
	IndexedAt  time.Time
}

// String implements fuzzy.Source interface for sahilm/fuzzy
func (e EnhancedFileEntry) String() string {
	return e.FullPath
}

// EnhancedFuzzyConfig holds configuration for the enhanced fuzzy engine
type EnhancedFuzzyConfig struct {
	RootPath       string
	IgnorePatterns []string
	MaxResults     int
	MaxDepth       int
	FilenameBoost  float64
	PathBoost      float64
	MinScore       float64
	CacheTimeout   time.Duration
	IndexFile      string
}

// NewEnhancedFuzzyEngine creates a new enhanced fuzzy search engine
func NewEnhancedFuzzyEngine(config EnhancedFuzzyConfig) (*EnhancedFuzzyEngine, error) {
	engine := &EnhancedFuzzyEngine{
		rootPath:       config.RootPath,
		ignorePatterns: config.IgnorePatterns,
		maxResults:     config.MaxResults,
		maxDepth:       config.MaxDepth,
		filenameBoost:  config.FilenameBoost,
		pathBoost:      config.PathBoost,
		minScore:       config.MinScore,
		cacheTimeout:   config.CacheTimeout,
		indexFile:      config.IndexFile,

		filenameIndex:  make(map[string][]int),
		pathIndex:      make(map[string][]int),
		queryCache:     make(map[string][]ZFSearchResult),
		indexedDirs:    make(map[string]time.Time),
		lastCacheTime:  time.Now(),
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
	if engine.pathBoost == 0 {
		engine.pathBoost = 1.5
	}
	if engine.minScore == 0 {
		engine.minScore = 0.1
	}

	// Try to load existing index
	if engine.indexFile != "" {
		if err := engine.loadIndex(); err != nil {
			log.WarningLog.Printf("Could not load existing index: %v", err)
		}
	}

	log.InfoLog.Printf("Enhanced fuzzy search engine initialized (using sahilm/fuzzy with indexing)")
	return engine, nil
}

// IndexDirectory indexes all files in the specified directory with intelligent caching
func (e *EnhancedFuzzyEngine) IndexDirectory(dirPath string) error {
	startTime := time.Now()

	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if directory needs re-indexing
	if lastIndexed, exists := e.indexedDirs[absPath]; exists {
		if info, err := os.Stat(absPath); err == nil {
			if info.ModTime().Before(lastIndexed.Add(e.cacheTimeout)) {
				log.InfoLog.Printf("Directory %s is up-to-date, skipping re-index", dirPath)
				return nil
			}
		}
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	var fileEntries []EnhancedFileEntry
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
		for _, pattern := range e.ignorePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				return nil
			}
			if matched, _ := filepath.Match(pattern, path); matched {
				return nil
			}
		}

		// Create enhanced file entry
		relPath, _ := filepath.Rel(absPath, path)
		parentDir := filepath.Dir(relPath)
		if parentDir == "." {
			parentDir = ""
		}

		entry := EnhancedFileEntry{
			ID:         fileCount,
			FullPath:   path,
			Filename:   info.Name(),
			ParentDirs: parentDir,
			Directory:  filepath.Dir(path),
			Extension:  filepath.Ext(path),
			IndexedAt:  time.Now(),
		}

		fileEntries = append(fileEntries, entry)
		fileCount++

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// Build indexes
	e.rebuildIndexes(fileEntries)
	e.indexedDirs[absPath] = time.Now()

	// Save index if persistence is enabled
	if e.indexFile != "" {
		if err := e.saveIndex(); err != nil {
			log.WarningLog.Printf("Failed to save index: %v", err)
		}
	}

	duration := time.Since(startTime)
	log.InfoLog.Printf("Indexed %d files in %s (took %v)", fileCount, dirPath, duration)

	return nil
}

// rebuildIndexes creates filename and path indexes for fast lookups
func (e *EnhancedFuzzyEngine) rebuildIndexes(files []EnhancedFileEntry) {
	e.fullIndex = files
	e.filenameIndex = make(map[string][]int)
	e.pathIndex = make(map[string][]int)

	for i, file := range files {
		// Index by filename (case-insensitive)
		filename := strings.ToLower(file.Filename)
		e.filenameIndex[filename] = append(e.filenameIndex[filename], i)

		// Index by path segments
		pathParts := strings.Split(strings.ToLower(file.ParentDirs), string(filepath.Separator))
		for _, part := range pathParts {
			if part != "" {
				e.pathIndex[part] = append(e.pathIndex[part], i)
			}
		}

		// Index by extension
		if file.Extension != "" {
			ext := strings.ToLower(file.Extension)
			e.pathIndex[ext] = append(e.pathIndex[ext], i)
		}
	}

	// Clear query cache when index changes
	e.queryCache = make(map[string][]ZFSearchResult)
}

// Search performs ZF-style fuzzy search with filename priority and path matching
func (e *EnhancedFuzzyEngine) Search(query string, directories []string) ([]ZFSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return []ZFSearchResult{}, nil
	}

	e.mutex.RLock()
	defer e.mutex.RUnlock()

	// Check query cache first
	cacheKey := fmt.Sprintf("%s|%v", query, directories)
	if cached, exists := e.queryCache[cacheKey]; exists {
		if time.Since(e.lastCacheTime) < e.cacheTimeout {
			return cached, nil
		}
	}

	// Filter files by directories if specified
	var searchFiles []EnhancedFileEntry
	if len(directories) > 0 {
		dirSet := make(map[string]bool)
		for _, dir := range directories {
			dirSet[dir] = true
		}

		for _, file := range e.fullIndex {
			if dirSet[file.Directory] {
				searchFiles = append(searchFiles, file)
			}
		}
	} else {
		searchFiles = e.fullIndex
	}

	if len(searchFiles) == 0 {
		return []ZFSearchResult{}, nil
	}

	// Phase 1: Filename-priority search (ZF algorithm)
	results := e.searchWithPriority(query, searchFiles)

	// Phase 2: Apply scoring and limits
	results = e.applyScoring(query, results)

	// Cache results
	if len(results) > 0 {
		e.queryCache[cacheKey] = results
		e.lastCacheTime = time.Now()
	}

	return results, nil
}

// searchWithPriority implements ZF-style priority matching
func (e *EnhancedFuzzyEngine) searchWithPriority(query string, files []EnhancedFileEntry) []ZFSearchResult {
	// Phase 1: Filename matches (highest priority)
	filenameResults := e.searchFilenames(query, files)

	// Phase 2: Path matches (medium priority)
	pathResults := e.searchPaths(query, files)

	// Phase 3: Full fuzzy search (lowest priority)
	fuzzyResults := e.searchFuzzy(query, files)

	// Combine results with priority weighting
	var combined []ZFSearchResult

	// Add filename matches with highest boost
	for _, result := range filenameResults {
		result.Score *= e.filenameBoost
		result.MatchType = MatchFilename
		combined = append(combined, result)
	}

	// Add path matches with medium boost (avoid duplicates)
	for _, result := range pathResults {
		if !e.containsPath(combined, result.SearchResult.Item.GetID()) {
			result.Score *= e.pathBoost
			result.MatchType = MatchPath
			combined = append(combined, result)
		}
	}

	// Add fuzzy matches (avoid duplicates)
	for _, result := range fuzzyResults {
		if !e.containsPath(combined, result.SearchResult.Item.GetID()) {
			result.MatchType = MatchFuzzy
			combined = append(combined, result)
		}
	}

	return combined
}

// searchFilenames searches only filenames for exact/prefix matches
func (e *EnhancedFuzzyEngine) searchFilenames(query string, files []EnhancedFileEntry) []ZFSearchResult {
	queryLower := strings.ToLower(query)
	var candidates []EnhancedFileEntry

	// Find files whose filenames contain the query
	for _, file := range files {
		if strings.Contains(strings.ToLower(file.Filename), queryLower) {
			candidates = append(candidates, file)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Use sahilm/fuzzy for scoring filename matches
	matches := fuzzy.FindFrom(query, EnhancedFileSlice(candidates))
	return e.convertMatches(matches, candidates)
}

// searchPaths searches path segments for matches
func (e *EnhancedFuzzyEngine) searchPaths(query string, files []EnhancedFileEntry) []ZFSearchResult {
	queryLower := strings.ToLower(query)
	var candidates []EnhancedFileEntry

	// Find files whose paths contain the query
	for _, file := range files {
		if strings.Contains(strings.ToLower(file.ParentDirs), queryLower) {
			candidates = append(candidates, file)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	matches := fuzzy.FindFrom(query, EnhancedFileSlice(candidates))
	return e.convertMatches(matches, candidates)
}

// searchFuzzy performs full fuzzy search across all files
func (e *EnhancedFuzzyEngine) searchFuzzy(query string, files []EnhancedFileEntry) []ZFSearchResult {
	matches := fuzzy.FindFrom(query, EnhancedFileSlice(files))
	return e.convertMatches(matches, files)
}

// convertMatches converts sahilm/fuzzy matches to ZFSearchResult format
func (e *EnhancedFuzzyEngine) convertMatches(matches fuzzy.Matches, files []EnhancedFileEntry) []ZFSearchResult {
	var results []ZFSearchResult

	for _, match := range matches {
		if match.Index >= len(files) {
			continue
		}

		file := files[match.Index]

		// Calculate score based on match quality
		score := float64(len(match.MatchedIndexes)) / float64(len(file.FullPath))
		if score > 1.0 {
			score = 1.0
		}

		result := ZFSearchResult{
			SearchResult: SearchResult{
				Item: &FileSearchItem{
				ID:       file.FullPath,
				FullPath: file.FullPath,
				Filename: file.Filename,
			},
				Score:   score,
				Matches: match.MatchedIndexes,
			},
			MatchType: MatchFuzzy,
		}

		results = append(results, result)
	}

	return results
}

// applyScoring applies final scoring and filtering
func (e *EnhancedFuzzyEngine) applyScoring(query string, results []ZFSearchResult) []ZFSearchResult {
	// Filter by minimum score
	var filtered []ZFSearchResult
	for _, result := range results {
		if result.Score >= e.minScore {
			filtered = append(filtered, result)
		}
	}

	// Sort by score (highest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	// Apply result limit
	if len(filtered) > e.maxResults {
		filtered = filtered[:e.maxResults]
	}

	return filtered
}

// containsPath checks if results already contain a specific path
func (e *EnhancedFuzzyEngine) containsPath(results []ZFSearchResult, path string) bool {
	for _, result := range results {
		if result.SearchResult.Item.GetID() == path {
			return true
		}
	}
	return false
}

// EnhancedFileSlice implements fuzzy.Source interface
type EnhancedFileSlice []EnhancedFileEntry

func (s EnhancedFileSlice) String(i int) string {
	return s[i].FullPath
}

func (s EnhancedFileSlice) Len() int {
	return len(s)
}


// saveIndex persists the index to disk
func (e *EnhancedFuzzyEngine) saveIndex() error {
	file, err := os.Create(e.indexFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	data := struct {
		FullIndex   []EnhancedFileEntry
		IndexedDirs map[string]time.Time
		Timestamp   time.Time
	}{
		FullIndex:   e.fullIndex,
		IndexedDirs: e.indexedDirs,
		Timestamp:   time.Now(),
	}

	return encoder.Encode(data)
}

// loadIndex loads the index from disk
func (e *EnhancedFuzzyEngine) loadIndex() error {
	file, err := os.Open(e.indexFile)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	var data struct {
		FullIndex   []EnhancedFileEntry
		IndexedDirs map[string]time.Time
		Timestamp   time.Time
	}

	if err := decoder.Decode(&data); err != nil {
		return err
	}

	// Check if index is too old
	if time.Since(data.Timestamp) > e.cacheTimeout {
		return fmt.Errorf("index is too old")
	}

	e.fullIndex = data.FullIndex
	e.indexedDirs = data.IndexedDirs
	e.rebuildIndexes(e.fullIndex)

	return nil
}

// Close cleans up resources
func (e *EnhancedFuzzyEngine) Close() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.indexFile != "" {
		if err := e.saveIndex(); err != nil {
			log.WarningLog.Printf("Failed to save index on close: %v", err)
		}
	}

	e.fullIndex = nil
	e.filenameIndex = nil
	e.pathIndex = nil
	e.queryCache = nil

	return nil
}

// GetStats returns search engine statistics
func (e *EnhancedFuzzyEngine) GetStats() (map[string]interface{}, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return map[string]interface{}{
		"engine_type":      "enhanced_fuzzy",
		"total_files":      len(e.fullIndex),
		"indexed_dirs":     len(e.indexedDirs),
		"filename_entries": len(e.filenameIndex),
		"path_entries":     len(e.pathIndex),
		"cache_entries":    len(e.queryCache),
		"max_results":      e.maxResults,
		"filename_boost":   e.filenameBoost,
		"path_boost":       e.pathBoost,
	}, nil
}

// DefaultEnhancedFuzzyConfig returns sensible defaults for enhanced fuzzy search
func DefaultEnhancedFuzzyConfig() EnhancedFuzzyConfig {
	indexFile := filepath.Join(os.TempDir(), "claude_squad_enhanced_search.gob")

	return EnhancedFuzzyConfig{
		MaxResults:     100,
		MaxDepth:       10,
		FilenameBoost:  2.0,
		PathBoost:      1.5,
		MinScore:       0.1,
		CacheTimeout:   24 * time.Hour,
		IndexFile:      indexFile,
		IgnorePatterns: []string{
			"*.git*",
			"node_modules",
			"*.tmp",
			"*.log",
			".DS_Store",
			"__pycache__",
			"*.pyc",
			".pytest_cache",
			"coverage.out",
		},
	}
}