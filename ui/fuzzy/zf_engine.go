package fuzzy

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"claude-squad/log"

	_ "github.com/mattn/go-sqlite3"
)

// ZFSearchEngine implements Nathan Craddock's ZF fuzzy finder approach with SQLite GIN indexing
// Key features:
// 1. Filename priority matching - filenames rank higher than path matches
// 2. Strict path matching - path segments cannot span directory boundaries
// 3. Space-separated tokens - progressive narrowing with multiple terms
// 4. SQLite GIN indexing for fast directory-based searches
// 5. Intent recognition - automatic switching between matching strategies
type ZFSearchEngine struct {
	db             *sql.DB
	dbPath         string
	ftsAvailable   bool         // Whether FTS5 is available in SQLite
	mutex          sync.RWMutex
	indexedDirs    map[string]time.Time // Track indexed directories and their modification times
	cacheTimeout   time.Duration
	minScore       float64
	maxResults     int
	caseSensitive  bool
	filenameBoost  float64 // Score multiplier for filename matches
	pathMatchBoost float64 // Score multiplier for strict path matches
}

// ZFSearchResult extends SearchResult with ZF-specific scoring information
type ZFSearchResult struct {
	SearchResult
	MatchType    ZFMatchType // Type of match (filename, path, fuzzy)
	TokenMatches []TokenMatch // Individual token match details
}

// ZFMatchType represents the type of match found
type ZFMatchType int

const (
	MatchFilename ZFMatchType = iota // Match found in filename only
	MatchPath                        // Strict path match (respecting directory boundaries)
	MatchFuzzy                       // Standard fuzzy match fallback
)

// TokenMatch represents how a single query token matched
type TokenMatch struct {
	Token     string      // The query token
	MatchType ZFMatchType // How this token matched
	Score     float64     // Score for this token match
	Matches   []int       // Character positions for highlighting
}

// ZFConfig contains configuration for the ZF search engine
type ZFConfig struct {
	DBPath         string        // Path to SQLite database file
	CacheTimeout   time.Duration // How long to cache directory indexes
	MinScore       float64       // Minimum score threshold
	MaxResults     int           // Maximum results to return
	FilenameBoost  float64       // Score multiplier for filename matches (default: 2.0)
	PathMatchBoost float64       // Score multiplier for path matches (default: 1.5)
}

// DefaultZFConfig returns sensible defaults for ZF search
func DefaultZFConfig() ZFConfig {
	return ZFConfig{
		DBPath:         filepath.Join(os.TempDir(), "claude_squad_search.db"),
		CacheTimeout:   24 * time.Hour, // Re-index directories daily
		MinScore:       0.2,
		MaxResults:     100,
		FilenameBoost:  2.0, // Filename matches get 2x score boost
		PathMatchBoost: 1.5, // Path matches get 1.5x score boost
	}
}

// NewZFSearchEngine creates a new ZF-inspired search engine with SQLite indexing
func NewZFSearchEngine(config ZFConfig) (*ZFSearchEngine, error) {
	engine := &ZFSearchEngine{
		dbPath:         config.DBPath,
		indexedDirs:    make(map[string]time.Time),
		cacheTimeout:   config.CacheTimeout,
		minScore:       config.MinScore,
		maxResults:     config.MaxResults,
		caseSensitive:  false, // Start with case-insensitive, enable smartcase later
		filenameBoost:  config.FilenameBoost,
		pathMatchBoost: config.PathMatchBoost,
	}

	// Initialize SQLite database
	if err := engine.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize search database: %w", err)
	}

	return engine, nil
}

// initDatabase creates the SQLite database and tables with GIN indexing
func (zf *ZFSearchEngine) initDatabase() error {
	var err error
	zf.db, err = sql.Open("sqlite3", zf.dbPath+"?cache=shared&mode=rwc")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Enable performance optimizations
	pragmas := []string{
		"PRAGMA journal_mode = WAL",           // Write-ahead logging for better concurrency
		"PRAGMA synchronous = NORMAL",         // Balance safety vs performance
		"PRAGMA cache_size = 10000",           // 10MB cache
		"PRAGMA temp_store = MEMORY",          // Store temp tables in memory
		"PRAGMA mmap_size = 268435456",        // 256MB memory-mapped I/O
		"PRAGMA optimize",                      // Enable query planner optimizations
	}

	for _, pragma := range pragmas {
		if _, err := zf.db.Exec(pragma); err != nil {
			log.WarningLog.Printf("Failed to set pragma %s: %v", pragma, err)
		}
	}

	// Create main files table
	createFilesTable := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		directory TEXT NOT NULL,
		full_path TEXT NOT NULL UNIQUE,
		filename TEXT NOT NULL,
		parent_dirs TEXT NOT NULL, -- Slash-separated parent directory names
		indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		file_size INTEGER,
		modified_time DATETIME
	);`

	if _, err := zf.db.Exec(createFilesTable); err != nil {
		return fmt.Errorf("failed to create files table: %w", err)
	}

	// Create GIN-style indexes for fast text search
	// SQLite doesn't have native GIN, but we can use FTS5 (Full-Text Search) for similar functionality
	createFTSTable := `
	CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
		full_path,
		filename,
		parent_dirs,
		content='files',
		content_rowid='id',
		tokenize='porter ascii'
	);`

	if _, err := zf.db.Exec(createFTSTable); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			log.WarningLog.Printf("FTS5 module not available in SQLite, falling back to regular SQL search: %v", err)
			zf.ftsAvailable = false
		} else {
			return fmt.Errorf("failed to create FTS table: %w", err)
		}
	} else {
		zf.ftsAvailable = true
		log.InfoLog.Printf("FTS5 module available, using full-text search indexing")
	}

	// Create indexes for common queries
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_files_directory ON files(directory)",
		"CREATE INDEX IF NOT EXISTS idx_files_filename ON files(filename)",
		"CREATE INDEX IF NOT EXISTS idx_files_indexed_at ON files(indexed_at)",
		"CREATE INDEX IF NOT EXISTS idx_files_modified_time ON files(modified_time)",
	}

	for _, index := range indexes {
		if _, err := zf.db.Exec(index); err != nil {
			log.WarningLog.Printf("Failed to create index: %v", err)
		}
	}

	// Create triggers to keep FTS table in sync (only if FTS5 is available)
	if zf.ftsAvailable {
		triggers := []string{
			`CREATE TRIGGER IF NOT EXISTS files_fts_insert AFTER INSERT ON files BEGIN
				INSERT INTO files_fts(rowid, full_path, filename, parent_dirs)
				VALUES (new.id, new.full_path, new.filename, new.parent_dirs);
			END;`,
			`CREATE TRIGGER IF NOT EXISTS files_fts_delete AFTER DELETE ON files BEGIN
				INSERT INTO files_fts(files_fts, rowid, full_path, filename, parent_dirs)
				VALUES('delete', old.id, old.full_path, old.filename, old.parent_dirs);
			END;`,
			`CREATE TRIGGER IF NOT EXISTS files_fts_update AFTER UPDATE ON files BEGIN
				INSERT INTO files_fts(files_fts, rowid, full_path, filename, parent_dirs)
				VALUES('delete', old.id, old.full_path, old.filename, old.parent_dirs);
				INSERT INTO files_fts(rowid, full_path, filename, parent_dirs)
				VALUES (new.id, new.full_path, new.filename, new.parent_dirs);
			END;`,
		}

		for _, trigger := range triggers {
			if _, err := zf.db.Exec(trigger); err != nil {
				log.WarningLog.Printf("Failed to create FTS trigger: %v", err)
			}
		}
	}

	log.InfoLog.Printf("ZF search engine database initialized at %s", zf.dbPath)
	return nil
}

// IndexDirectory indexes all files in a directory for fast searching
func (zf *ZFSearchEngine) IndexDirectory(dirPath string) error {
	zf.mutex.Lock()
	defer zf.mutex.Unlock()

	// Check if directory was recently indexed
	if lastIndexed, exists := zf.indexedDirs[dirPath]; exists {
		if time.Since(lastIndexed) < zf.cacheTimeout {
			log.InfoLog.Printf("Directory %s was recently indexed, skipping", dirPath)
			return nil
		}
	}

	log.InfoLog.Printf("Indexing directory: %s", dirPath)
	start := time.Now()

	// Remove old entries for this directory
	_, err := zf.db.Exec("DELETE FROM files WHERE directory = ?", dirPath)
	if err != nil {
		return fmt.Errorf("failed to clean old directory index: %w", err)
	}

	// Walk directory and collect files
	var fileCount int
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.WarningLog.Printf("Error walking path %s: %v", path, err)
			return nil // Continue walking
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Extract path components
		filename := filepath.Base(path)
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			relPath = path
		}

		// Build parent directory path for search
		parentDirs := strings.Join(strings.Split(filepath.Dir(relPath), string(filepath.Separator)), "/")
		if parentDirs == "." {
			parentDirs = ""
		}

		// Insert file record
		_, err = zf.db.Exec(`
			INSERT OR REPLACE INTO files
			(directory, full_path, filename, parent_dirs, file_size, modified_time)
			VALUES (?, ?, ?, ?, ?, ?)`,
			dirPath, path, filename, parentDirs, info.Size(), info.ModTime())

		if err != nil {
			log.WarningLog.Printf("Failed to index file %s: %v", path, err)
			return nil // Continue indexing other files
		}

		fileCount++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// Update indexing timestamp
	zf.indexedDirs[dirPath] = time.Now()

	duration := time.Since(start)
	log.InfoLog.Printf("Indexed %d files in %s (took %v)", fileCount, dirPath, duration)

	return nil
}

// Search performs ZF-style fuzzy search with the following priority:
// 1. Filename matches (highest priority)
// 2. Strict path matches (respecting directory boundaries)
// 3. Fuzzy matches (fallback)
func (zf *ZFSearchEngine) Search(query string, directories []string) ([]ZFSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return []ZFSearchResult{}, nil
	}

	zf.mutex.RLock()
	defer zf.mutex.RUnlock()

	// Implement smartcase: case-sensitive if query contains uppercase
	caseSensitive := zf.containsUppercase(query)

	// Tokenize query by spaces (ZF feature)
	tokens := zf.tokenizeQuery(query)
	if len(tokens) == 0 {
		return []ZFSearchResult{}, nil
	}

	// Build SQL query based on directories to search
	var dirConditions []string
	var args []interface{}

	if len(directories) > 0 {
		for _, dir := range directories {
			dirConditions = append(dirConditions, "directory = ?")
			args = append(args, dir)
		}
	} else {
		// Search all indexed directories
		dirConditions = append(dirConditions, "1=1")
	}

	dirClause := strings.Join(dirConditions, " OR ")

	var sqlQuery string
	if zf.ftsAvailable {
		// Use FTS for initial filtering, then apply ZF ranking
		ftsQuery := zf.buildFTSQuery(tokens)

		sqlQuery = fmt.Sprintf(`
			SELECT f.id, f.full_path, f.filename, f.parent_dirs
			FROM files f
			JOIN files_fts fts ON f.id = fts.rowid
			WHERE (%s) AND files_fts MATCH ?
			ORDER BY rank
			LIMIT ?`, dirClause)

		args = append(args, ftsQuery, zf.maxResults*2) // Get more results for re-ranking
	} else {
		// Fallback to regular SQL with LIKE queries (slower but functional)
		var likeConditions []string
		for _, token := range tokens {
			// Create LIKE conditions for each token to match filename or full path
			likeConditions = append(likeConditions, "(filename LIKE ? OR full_path LIKE ?)")
			args = append(args, "%"+token+"%", "%"+token+"%")
		}
		likeClause := strings.Join(likeConditions, " AND ")

		sqlQuery = fmt.Sprintf(`
			SELECT id, full_path, filename, parent_dirs
			FROM files
			WHERE (%s) AND (%s)
			ORDER BY filename
			LIMIT ?`, dirClause, likeClause)

		args = append(args, zf.maxResults*2) // Get more results for re-ranking
	}

	rows, err := zf.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var candidates []FileCandidate
	for rows.Next() {
		var candidate FileCandidate
		err := rows.Scan(&candidate.ID, &candidate.FullPath, &candidate.Filename, &candidate.ParentDirs)
		if err != nil {
			log.WarningLog.Printf("Failed to scan search result: %v", err)
			continue
		}
		candidates = append(candidates, candidate)
	}

	// Apply ZF ranking algorithm
	results := zf.rankCandidates(tokens, candidates, caseSensitive)

	// Filter by minimum score and limit results
	var filteredResults []ZFSearchResult
	for _, result := range results {
		if result.Score >= zf.minScore && len(filteredResults) < zf.maxResults {
			filteredResults = append(filteredResults, result)
		}
	}

	log.InfoLog.Printf("ZF search for '%s' found %d results", query, len(filteredResults))
	return filteredResults, nil
}

// FileCandidate represents a file candidate from the database
type FileCandidate struct {
	ID         int
	FullPath   string
	Filename   string
	ParentDirs string
}

// containsUppercase checks if query contains uppercase letters (for smartcase)
func (zf *ZFSearchEngine) containsUppercase(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

// tokenizeQuery splits query into space-separated tokens
func (zf *ZFSearchEngine) tokenizeQuery(query string) []string {
	tokens := strings.Fields(strings.TrimSpace(query))
	// Remove empty tokens
	var filtered []string
	for _, token := range tokens {
		if token != "" {
			filtered = append(filtered, token)
		}
	}
	return filtered
}

// buildFTSQuery creates an FTS query from tokens
func (zf *ZFSearchEngine) buildFTSQuery(tokens []string) string {
	// For FTS5, we want all tokens to match (AND behavior)
	var ftsTokens []string
	for _, token := range tokens {
		// Escape FTS special characters and add prefix matching
		escapedToken := strings.ReplaceAll(token, `"`, `""`)
		ftsTokens = append(ftsTokens, fmt.Sprintf(`"%s"*`, escapedToken))
	}
	return strings.Join(ftsTokens, " AND ")
}

// rankCandidates applies ZF ranking algorithm to candidates
func (zf *ZFSearchEngine) rankCandidates(tokens []string, candidates []FileCandidate, caseSensitive bool) []ZFSearchResult {
	var results []ZFSearchResult

	for _, candidate := range candidates {
		// Score each candidate using ZF algorithm
		result := zf.scoreCandidate(tokens, candidate, caseSensitive)
		if result.Score > 0 {
			results = append(results, result)
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// scoreCandidate implements ZF's scoring algorithm with filename priority and strict path matching
func (zf *ZFSearchEngine) scoreCandidate(tokens []string, candidate FileCandidate, caseSensitive bool) ZFSearchResult {
	result := ZFSearchResult{
		SearchResult: SearchResult{
			Item: &FileSearchItem{
				ID:       fmt.Sprintf("%d", candidate.ID),
				FullPath: candidate.FullPath,
				Filename: candidate.Filename,
			},
			Score:   0,
			Matches: []int{},
		},
		TokenMatches: make([]TokenMatch, 0, len(tokens)),
	}

	var totalScore float64
	var allMatches []int
	allTokensMatched := true

	for _, token := range tokens {
		tokenMatch := zf.scoreToken(token, candidate, caseSensitive)
		if tokenMatch.Score == 0 {
			// If any token doesn't match, the candidate is rejected
			allTokensMatched = false
			break
		}

		result.TokenMatches = append(result.TokenMatches, tokenMatch)
		totalScore += tokenMatch.Score
		allMatches = append(allMatches, tokenMatch.Matches...)

		// Update overall match type to the highest priority match found
		if tokenMatch.MatchType < result.MatchType || result.MatchType == 0 {
			result.MatchType = tokenMatch.MatchType
		}
	}

	if !allTokensMatched {
		return ZFSearchResult{} // Return zero score if not all tokens matched
	}

	// Average the token scores
	result.Score = totalScore / float64(len(tokens))
	result.Matches = allMatches

	// Sort matches for consistent highlighting
	sort.Ints(result.Matches)

	return result
}

// scoreToken scores a single token against a candidate using ZF priority: filename > path > fuzzy
func (zf *ZFSearchEngine) scoreToken(token string, candidate FileCandidate, caseSensitive bool) TokenMatch {
	if !caseSensitive {
		token = strings.ToLower(token)
	}

	filename := candidate.Filename
	fullPath := candidate.FullPath
	parentDirs := candidate.ParentDirs

	if !caseSensitive {
		filename = strings.ToLower(filename)
		fullPath = strings.ToLower(fullPath)
		parentDirs = strings.ToLower(parentDirs)
	}

	// 1. Try filename matching first (highest priority)
	if score, matches := zf.fuzzyMatchString(token, filename); score > 0 {
		return TokenMatch{
			Token:     token,
			MatchType: MatchFilename,
			Score:     score * zf.filenameBoost, // Apply filename boost
			Matches:   matches,
		}
	}

	// 2. Try strict path matching (ZF innovation)
	if strings.Contains(token, "/") {
		if score, matches := zf.strictPathMatch(token, fullPath, parentDirs); score > 0 {
			return TokenMatch{
				Token:     token,
				MatchType: MatchPath,
				Score:     score * zf.pathMatchBoost, // Apply path boost
				Matches:   matches,
			}
		}
	}

	// 3. Fallback to fuzzy matching on full path
	if score, matches := zf.fuzzyMatchString(token, fullPath); score > 0 {
		return TokenMatch{
			Token:     token,
			MatchType: MatchFuzzy,
			Score:     score, // No boost for fuzzy matches
			Matches:   matches,
		}
	}

	// No match found
	return TokenMatch{
		Token:     token,
		MatchType: MatchFuzzy,
		Score:     0,
		Matches:   []int{},
	}
}

// strictPathMatch implements ZF's strict path matching where path segments cannot span directories
func (zf *ZFSearchEngine) strictPathMatch(token, fullPath, parentDirs string) (float64, []int) {
	// Split token by path separators
	tokenParts := strings.Split(strings.Trim(token, "/"), "/")
	pathParts := strings.Split(strings.Trim(parentDirs, "/"), "/")

	if len(tokenParts) == 0 || len(pathParts) == 0 {
		return 0, []int{}
	}

	// Try to match token parts against path parts in order
	var matches []int
	tokenIdx := 0
	baseScore := 0.0

	for pathIdx := 0; pathIdx < len(pathParts) && tokenIdx < len(tokenParts); pathIdx++ {
		pathPart := pathParts[pathIdx]
		tokenPart := tokenParts[tokenIdx]

		// Try fuzzy matching within this path segment
		if score, partMatches := zf.fuzzyMatchString(tokenPart, pathPart); score > 0 {
			// Adjust match positions to full path
			pathOffset := strings.Index(fullPath, pathPart)
			if pathOffset >= 0 {
				for _, match := range partMatches {
					matches = append(matches, pathOffset+match)
				}
			}

			baseScore += score
			tokenIdx++
		}
	}

	// All token parts must match for strict path matching
	if tokenIdx < len(tokenParts) {
		return 0, []int{}
	}

	// Calculate final score based on how well tokens matched path segments
	finalScore := baseScore / float64(len(tokenParts))

	// Bonus for consecutive segment matches
	if len(tokenParts) > 1 {
		finalScore *= 1.2
	}

	return finalScore, matches
}

// fuzzyMatchString performs fuzzy string matching similar to the existing implementation
func (zf *ZFSearchEngine) fuzzyMatchString(pattern, text string) (float64, []int) {
	// Reuse the existing fuzzy matching logic from fuzzy.go
	return fuzzyMatch(pattern, text)
}

// Close closes the database connection
func (zf *ZFSearchEngine) Close() error {
	zf.mutex.Lock()
	defer zf.mutex.Unlock()

	if zf.db != nil {
		return zf.db.Close()
	}
	return nil
}

// GetStats returns statistics about the search index
func (zf *ZFSearchEngine) GetStats() (map[string]interface{}, error) {
	zf.mutex.RLock()
	defer zf.mutex.RUnlock()

	stats := make(map[string]interface{})

	// Count total files
	var totalFiles int
	err := zf.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&totalFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to count files: %w", err)
	}
	stats["total_files"] = totalFiles

	// Count indexed directories
	stats["indexed_directories"] = len(zf.indexedDirs)

	// Database file size
	if dbInfo, err := os.Stat(zf.dbPath); err == nil {
		stats["database_size_bytes"] = dbInfo.Size()
	}

	return stats, nil
}