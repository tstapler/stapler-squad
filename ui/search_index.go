package ui

import (
	"claude-squad/session"
	"context"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sahilm/fuzzy"
	"github.com/schollz/closestmatch"
)

// SortMode defines how sessions should be sorted
type SortMode int

const (
	// SortByLastActivity sorts by LastMeaningfulOutput timestamp (default, most recent first)
	SortByLastActivity SortMode = iota
	// SortByCreationDate sorts by CreatedAt timestamp
	SortByCreationDate
	// SortByTitleAZ sorts alphabetically by session title
	SortByTitleAZ
	// SortByRepository sorts alphabetically by repository path
	SortByRepository
	// SortByBranch sorts alphabetically by branch name
	SortByBranch
	// SortByStatus sorts by session status (Running > Ready > NeedsApproval > Loading > Paused)
	SortByStatus
)

// SortDirection defines ascending or descending sort order
type SortDirection int

const (
	// SortDescending sorts from high to low (newest first for dates, Z-A for strings)
	SortDescending SortDirection = iota
	// SortAscending sorts from low to high (oldest first for dates, A-Z for strings)
	SortAscending
)

// String returns the human-readable name for the sort mode
func (s SortMode) String() string {
	switch s {
	case SortByLastActivity:
		return "Last Activity"
	case SortByCreationDate:
		return "Creation Date"
	case SortByTitleAZ:
		return "Title"
	case SortByRepository:
		return "Repository"
	case SortByBranch:
		return "Branch"
	case SortByStatus:
		return "Status"
	default:
		return "Unknown"
	}
}

// ShortName returns an abbreviated name for UI display
func (s SortMode) ShortName() string {
	switch s {
	case SortByLastActivity:
		return "Activity"
	case SortByCreationDate:
		return "Created"
	case SortByTitleAZ:
		return "Title"
	case SortByRepository:
		return "Repo"
	case SortByBranch:
		return "Branch"
	case SortByStatus:
		return "Status"
	default:
		return "?"
	}
}

// String returns the human-readable name for the sort direction
func (d SortDirection) String() string {
	switch d {
	case SortDescending:
		return "Descending"
	case SortAscending:
		return "Ascending"
	default:
		return "Unknown"
	}
}

// Icon returns an icon for the sort direction
func (d SortDirection) Icon() string {
	switch d {
	case SortDescending:
		return "↓"
	case SortAscending:
		return "↑"
	default:
		return ""
	}
}

// SearchIndex provides optimized fuzzy search for sessions using hybrid indexing
type SearchIndex struct {
	// Primary fast matcher for pre-filtering
	closestMatch *closestmatch.ClosestMatch

	// Category optimization for instant category filtering
	categoryIndex map[string][]*session.Instance

	// Program optimization for instant program filtering
	programIndex map[string][]*session.Instance

	// Tag optimization for instant tag filtering
	tagIndex map[string][]*session.Instance

	// Session lookup by search text
	sessionMap map[string]*session.Instance

	// All sessions for fallback sahilm/fuzzy search
	allSessions []*session.Instance

	// Sort configuration
	sortMode      SortMode      // Current sort mode
	sortDirection SortDirection // Current sort direction

	// Sorted cache for performance optimization
	sortedCache      []*session.Instance // Cached sorted results
	sortedCacheValid bool                // Whether the sorted cache is valid
	sortedCacheMode  SortMode            // Sort mode used for cache
	sortedCacheDir   SortDirection       // Sort direction used for cache

	// Index management
	version      int
	needsRebuild bool
	mutex        sync.RWMutex
}

// NewSearchIndex creates a new optimized search index
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		categoryIndex:    make(map[string][]*session.Instance),
		programIndex:     make(map[string][]*session.Instance),
		tagIndex:         make(map[string][]*session.Instance),
		sessionMap:       make(map[string]*session.Instance),
		allSessions:      make([]*session.Instance, 0),
		sortMode:         SortByLastActivity, // Default: most recently active first
		sortDirection:    SortDescending,     // Default: newest/highest first
		sortedCache:      make([]*session.Instance, 0),
		sortedCacheValid: false,
		needsRebuild:     true,
	}
}

// RebuildIndex rebuilds the search index from the current session list
func (idx *SearchIndex) RebuildIndex(sessions []*session.Instance) {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	// Clear existing indices
	idx.categoryIndex = make(map[string][]*session.Instance)
	idx.programIndex = make(map[string][]*session.Instance)
	idx.tagIndex = make(map[string][]*session.Instance)
	idx.sessionMap = make(map[string]*session.Instance)
	idx.allSessions = make([]*session.Instance, len(sessions))
	copy(idx.allSessions, sessions)

	// Build search texts and indices
	searchTexts := make([]string, 0, len(sessions))

	for _, instance := range sessions {
		if instance == nil {
			continue
		}

		// Create combined search text (same as SessionSearchSource)
		searchText := idx.createSearchText(instance)
		searchTexts = append(searchTexts, searchText)

		// Map search text to instance for lookup
		idx.sessionMap[searchText] = instance

		// Build category index
		if instance.Category != "" {
			categoryKey := strings.ToLower(instance.Category)
			idx.categoryIndex[categoryKey] = append(idx.categoryIndex[categoryKey], instance)
		}

		// Build program index
		if instance.Program != "" {
			programKey := strings.ToLower(filepath.Base(instance.Program))
			idx.programIndex[programKey] = append(idx.programIndex[programKey], instance)
		}

		// Build tag index (multi-membership: session can appear in multiple tag groups)
		for _, tag := range instance.GetTags() {
			tagKey := strings.ToLower(tag)
			idx.tagIndex[tagKey] = append(idx.tagIndex[tagKey], instance)
		}
	}

	// Build closestmatch index for fast pre-filtering
	if len(searchTexts) > 0 {
		// Configure for optimal performance with session-sized datasets
		// Higher bag sizes = better accuracy, lower bag sizes = faster performance
		bagSizes := []int{2, 3, 4} // Optimal for fuzzy matching on session titles/metadata
		idx.closestMatch = closestmatch.New(searchTexts, bagSizes)
	}

	idx.version++
	idx.needsRebuild = false
	idx.sortedCacheValid = false // Invalidate sort cache on rebuild
}

// Search performs optimized hybrid fuzzy search
func (idx *SearchIndex) Search(query string, maxResults int) []*session.Instance {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	if idx.needsRebuild || idx.closestMatch == nil {
		// Cannot search without valid index - return empty results
		return []*session.Instance{}
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return idx.allSessions
	}

	// Strategy 1: Category detection for instant filtering
	if categoryInstances := idx.searchByCategory(query); categoryInstances != nil {
		return idx.limitResults(categoryInstances, maxResults)
	}

	// Strategy 2: Program detection for instant filtering
	if programInstances := idx.searchByProgram(query); programInstances != nil {
		return idx.limitResults(programInstances, maxResults)
	}

	// Strategy 3: Tag detection for instant filtering
	if tagInstances := idx.searchByTag(query); tagInstances != nil {
		return idx.limitResults(tagInstances, maxResults)
	}

	// Strategy 4: Hybrid approach - fast pre-filter + high-quality ranking
	return idx.hybridSearch(query, maxResults)
}

// SearchResult represents an intermediate search result with metadata
type SearchResult struct {
	Instances []*session.Instance
	Total     int
	Complete  bool
	Stage     string // "category", "program", "hybrid", "complete"
}

// SearchStream performs streaming search with parallel processing and intermediate results
// Returns a channel that emits intermediate results as they become available
func (idx *SearchIndex) SearchStream(ctx context.Context, query string, maxResults int) <-chan SearchResult {
	results := make(chan SearchResult, 3) // Buffer for up to 3 intermediate results

	go func() {
		defer close(results)

		idx.mutex.RLock()
		defer idx.mutex.RUnlock()

		if idx.needsRebuild || idx.closestMatch == nil {
			results <- SearchResult{Instances: []*session.Instance{}, Total: 0, Complete: true, Stage: "empty"}
			return
		}

		query = strings.TrimSpace(query)
		if query == "" {
			results <- SearchResult{Instances: idx.allSessions, Total: len(idx.allSessions), Complete: true, Stage: "all"}
			return
		}

		// Stage 1: Fast category/program/tag detection (immediate results)
		if categoryInstances := idx.searchByCategory(query); categoryInstances != nil {
			limited := idx.limitResults(categoryInstances, maxResults)
			results <- SearchResult{Instances: limited, Total: len(categoryInstances), Complete: true, Stage: "category"}
			return
		}

		if programInstances := idx.searchByProgram(query); programInstances != nil {
			limited := idx.limitResults(programInstances, maxResults)
			results <- SearchResult{Instances: limited, Total: len(programInstances), Complete: true, Stage: "program"}
			return
		}

		if tagInstances := idx.searchByTag(query); tagInstances != nil {
			limited := idx.limitResults(tagInstances, maxResults)
			results <- SearchResult{Instances: limited, Total: len(tagInstances), Complete: true, Stage: "tag"}
			return
		}

		// Stage 2: Parallel hybrid search with streaming results
		idx.hybridSearchStream(ctx, query, maxResults, results)
	}()

	return results
}

// hybridSearchStream performs parallel fuzzy search with intermediate results
func (idx *SearchIndex) hybridSearchStream(ctx context.Context, query string, maxResults int, results chan<- SearchResult) {
	// Stage 1: Fast pre-filtering (immediate partial results)
	candidateCount := maxResults * 3
	if candidateCount < 50 {
		candidateCount = 50
	}
	if candidateCount > len(idx.allSessions) {
		candidateCount = len(idx.allSessions)
	}

	candidateTexts := idx.closestMatch.ClosestN(query, candidateCount)

	// Convert candidate texts back to session instances
	candidates := make([]*session.Instance, 0, len(candidateTexts))
	for _, text := range candidateTexts {
		if instance, exists := idx.sessionMap[text]; exists {
			candidates = append(candidates, instance)
		}
	}

	// If we have very few candidates, use all sessions
	if len(candidates) < 10 {
		candidates = idx.allSessions
	}

	// Send partial results immediately (basic string matching)
	partialResults := make([]*session.Instance, 0, maxResults)
	queryLower := strings.ToLower(query)
	for _, candidate := range candidates {
		if len(partialResults) >= maxResults {
			break
		}
		searchText := idx.createSearchText(candidate)
		if strings.Contains(strings.ToLower(searchText), queryLower) {
			partialResults = append(partialResults, candidate)
		}
	}

	// Send intermediate results
	select {
	case results <- SearchResult{Instances: partialResults, Total: len(partialResults), Complete: false, Stage: "partial"}:
	case <-ctx.Done():
		return
	}

	// Stage 2: Parallel fuzzy search with multiple workers
	numWorkers := runtime.NumCPU()
	if numWorkers > 4 {
		numWorkers = 4 // Cap at 4 workers to avoid overhead
	}

	chunkSize := len(candidates) / numWorkers
	if chunkSize < 10 {
		chunkSize = 10
	}

	type fuzzyResult struct {
		matches []fuzzy.Match
		chunk   int
	}

	fuzzyResults := make(chan fuzzyResult, numWorkers)
	var wg sync.WaitGroup

	// Launch parallel fuzzy search workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			start := workerID * chunkSize
			end := start + chunkSize
			if end > len(candidates) || workerID == numWorkers-1 {
				end = len(candidates)
			}

			if start >= end {
				return
			}

			chunk := candidates[start:end]
			searchSource := SessionSearchSource{sessions: chunk}
			matches := fuzzy.FindFrom(query, searchSource)

			select {
			case fuzzyResults <- fuzzyResult{matches: matches, chunk: workerID}:
			case <-ctx.Done():
				return
			}
		}(i)
	}

	// Collect and merge results as they arrive
	go func() {
		wg.Wait()
		close(fuzzyResults)
	}()

	allMatches := make([]fuzzy.Match, 0)
	workersCompleted := 0

	for {
		select {
		case result, ok := <-fuzzyResults:
			if !ok {
				// All workers finished, send final results
				// Sort all matches by score
				for i := 0; i < len(allMatches)-1; i++ {
					for j := i + 1; j < len(allMatches); j++ {
						if allMatches[i].Score < allMatches[j].Score {
							allMatches[i], allMatches[j] = allMatches[j], allMatches[i]
						}
					}
				}

				// Convert to final session instances
				finalResults := make([]*session.Instance, 0, maxResults)
				for _, match := range allMatches {
					chunkStart := (match.Index / chunkSize) * chunkSize
					localIndex := match.Index % chunkSize
					if chunkStart+localIndex < len(candidates) {
						finalResults = append(finalResults, candidates[chunkStart+localIndex])
						if len(finalResults) >= maxResults {
							break
						}
					}
				}

				select {
				case results <- SearchResult{Instances: finalResults, Total: len(allMatches), Complete: true, Stage: "complete"}:
				case <-ctx.Done():
				}
				return
			}

			// Merge this worker's results
			allMatches = append(allMatches, result.matches...)
			workersCompleted++

			// Send progressive results every 2 workers or after a delay
			if workersCompleted%2 == 0 || workersCompleted == numWorkers {
				// Sort current matches by score
				currentMatches := make([]fuzzy.Match, len(allMatches))
				copy(currentMatches, allMatches)
				for i := 0; i < len(currentMatches)-1; i++ {
					for j := i + 1; j < len(currentMatches); j++ {
						if currentMatches[i].Score < currentMatches[j].Score {
							currentMatches[i], currentMatches[j] = currentMatches[j], currentMatches[i]
						}
					}
				}

				// Convert to session instances
				progressiveResults := make([]*session.Instance, 0, maxResults)
				for _, match := range currentMatches {
					chunkStart := (match.Index / chunkSize) * chunkSize
					localIndex := match.Index % chunkSize
					if chunkStart+localIndex < len(candidates) {
						progressiveResults = append(progressiveResults, candidates[chunkStart+localIndex])
						if len(progressiveResults) >= maxResults {
							break
						}
					}
				}

				select {
				case results <- SearchResult{
					Instances: progressiveResults,
					Total:     len(currentMatches),
					Complete:  workersCompleted == numWorkers,
					Stage:     "fuzzy-progress",
				}:
				case <-ctx.Done():
					return
				}
			}

		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
			// Timeout to ensure we don't block too long
			continue
		}
	}
}

// searchByCategory checks if query is a category name and returns instant results
func (idx *SearchIndex) searchByCategory(query string) []*session.Instance {
	queryLower := strings.ToLower(query)

	// Check for exact category match
	if instances, exists := idx.categoryIndex[queryLower]; exists {
		return instances
	}

	// Check for category prefix match
	for category, instances := range idx.categoryIndex {
		if strings.HasPrefix(category, queryLower) && len(queryLower) >= 2 {
			return instances
		}
	}

	return nil
}

// searchByProgram checks if query is a program name and returns instant results
func (idx *SearchIndex) searchByProgram(query string) []*session.Instance {
	queryLower := strings.ToLower(query)

	// Check for exact program match
	if instances, exists := idx.programIndex[queryLower]; exists {
		return instances
	}

	// Check for program prefix match
	for program, instances := range idx.programIndex {
		if strings.HasPrefix(program, queryLower) && len(queryLower) >= 2 {
			return instances
		}
	}

	return nil
}

// searchByTag checks if query is a tag name and returns instant results
func (idx *SearchIndex) searchByTag(query string) []*session.Instance {
	queryLower := strings.ToLower(query)

	// Check for exact tag match
	if instances, exists := idx.tagIndex[queryLower]; exists {
		return instances
	}

	// Check for tag prefix match
	for tag, instances := range idx.tagIndex {
		if strings.HasPrefix(tag, queryLower) && len(queryLower) >= 2 {
			return instances
		}
	}

	return nil
}

// hybridSearch performs the two-stage hybrid search: fast pre-filter + fuzzy ranking
func (idx *SearchIndex) hybridSearch(query string, maxResults int) []*session.Instance {
	// Stage 1: Fast pre-filtering with closestmatch
	// Get more candidates than needed to ensure high-quality results after fuzzy ranking
	candidateCount := maxResults * 3
	if candidateCount < 50 {
		candidateCount = 50 // Minimum candidates for good fuzzy ranking
	}
	if candidateCount > len(idx.allSessions) {
		candidateCount = len(idx.allSessions)
	}

	candidateTexts := idx.closestMatch.ClosestN(query, candidateCount)

	// Convert candidate texts back to session instances
	candidates := make([]*session.Instance, 0, len(candidateTexts))
	for _, text := range candidateTexts {
		if instance, exists := idx.sessionMap[text]; exists {
			candidates = append(candidates, instance)
		}
	}

	// If we have very few candidates, use all sessions for better fuzzy results
	if len(candidates) < 10 {
		candidates = idx.allSessions
	}

	// Stage 2: High-quality fuzzy ranking on pre-filtered candidates
	searchSource := SessionSearchSource{sessions: candidates}
	fuzzyMatches := fuzzy.FindFrom(query, searchSource)

	// Convert fuzzy matches back to session instances with ranking preserved
	results := make([]*session.Instance, 0, len(fuzzyMatches))
	for _, match := range fuzzyMatches {
		if match.Index >= 0 && match.Index < len(candidates) {
			results = append(results, candidates[match.Index])
			if len(results) >= maxResults {
				break
			}
		}
	}

	return results
}

// MarkNeedsRebuild marks the index as needing a rebuild
func (idx *SearchIndex) MarkNeedsRebuild() {
	idx.mutex.Lock()
	idx.needsRebuild = true
	idx.mutex.Unlock()
}

// IsIndexValid returns whether the index is valid and ready for searches
func (idx *SearchIndex) IsIndexValid() bool {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return !idx.needsRebuild && idx.closestMatch != nil
}

// GetVersion returns the current index version for cache invalidation
func (idx *SearchIndex) GetVersion() int {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.version
}

// createSearchText creates the searchable text for a session (same as SessionSearchSource)
func (idx *SearchIndex) createSearchText(instance *session.Instance) string {
	parts := []string{
		instance.Title,
		instance.Category,
		instance.Program,
		instance.Branch,
		filepath.Base(instance.Path), // Repository name
		instance.WorkingDir,
		strings.Join(instance.GetTags(), " "), // Tags for multi-dimensional search
	}

	// Filter out empty parts
	filteredParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	return strings.Join(filteredParts, " ")
}

// limitResults limits the results to maxResults
func (idx *SearchIndex) limitResults(instances []*session.Instance, maxResults int) []*session.Instance {
	if len(instances) <= maxResults {
		return instances
	}
	return instances[:maxResults]
}

// GetStats returns index statistics for debugging and monitoring
func (idx *SearchIndex) GetStats() map[string]interface{} {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	categoryCount := 0
	for _, instances := range idx.categoryIndex {
		categoryCount += len(instances)
	}

	programCount := 0
	for _, instances := range idx.programIndex {
		programCount += len(instances)
	}

	tagCount := 0
	for _, instances := range idx.tagIndex {
		tagCount += len(instances)
	}

	return map[string]interface{}{
		"version":          idx.version,
		"total_sessions":   len(idx.allSessions),
		"categories":       len(idx.categoryIndex),
		"programs":         len(idx.programIndex),
		"tags":             len(idx.tagIndex),
		"category_entries": categoryCount,
		"program_entries":  programCount,
		"tag_entries":      tagCount,
		"needs_rebuild":    idx.needsRebuild,
		"index_valid":      idx.closestMatch != nil,
		"sort_mode":        idx.sortMode.String(),
		"sort_direction":   idx.sortDirection.String(),
		"sort_cache_valid": idx.sortedCacheValid,
	}
}

// =============================================================================
// Sort Methods
// =============================================================================

// Sort returns a sorted copy of the provided sessions according to the current sort configuration.
// Uses cached results when available for performance optimization.
// Per ADR-3: Uses LastActivity as secondary sort for stability in non-activity sorts.
func (idx *SearchIndex) Sort(sessions []*session.Instance) []*session.Instance {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	if len(sessions) == 0 {
		return sessions
	}

	// Check if we can use cached results
	if idx.sortedCacheValid &&
		idx.sortedCacheMode == idx.sortMode &&
		idx.sortedCacheDir == idx.sortDirection &&
		len(idx.sortedCache) == len(sessions) {
		// Verify cache is for the same session set by checking first/last elements
		if len(sessions) > 0 && len(idx.sortedCache) > 0 {
			// Quick check: if sessions slice is exactly what we cached, return it
			sameSet := true
			sessionSet := make(map[*session.Instance]bool, len(sessions))
			for _, s := range sessions {
				sessionSet[s] = true
			}
			for _, s := range idx.sortedCache {
				if !sessionSet[s] {
					sameSet = false
					break
				}
			}
			if sameSet {
				return idx.sortedCache
			}
		}
	}

	// Create a copy to avoid modifying the original slice
	sorted := make([]*session.Instance, len(sessions))
	copy(sorted, sessions)

	// Sort based on current mode
	idx.sortSessions(sorted)

	// Cache the results
	idx.sortedCache = sorted
	idx.sortedCacheValid = true
	idx.sortedCacheMode = idx.sortMode
	idx.sortedCacheDir = idx.sortDirection

	return sorted
}

// sortSessions performs the actual sorting based on current sort mode and direction
func (idx *SearchIndex) sortSessions(sessions []*session.Instance) {
	switch idx.sortMode {
	case SortByLastActivity:
		idx.sortByLastActivity(sessions)
	case SortByCreationDate:
		idx.sortByCreationDate(sessions)
	case SortByTitleAZ:
		idx.sortByTitle(sessions)
	case SortByRepository:
		idx.sortByRepository(sessions)
	case SortByBranch:
		idx.sortByBranch(sessions)
	case SortByStatus:
		idx.sortByStatus(sessions)
	default:
		idx.sortByLastActivity(sessions)
	}
}

// sortByLastActivity sorts sessions by LastMeaningfulOutput timestamp
// Falls back to CreatedAt if LastMeaningfulOutput is zero
func (idx *SearchIndex) sortByLastActivity(sessions []*session.Instance) {
	sort.SliceStable(sessions, func(i, j int) bool {
		ti := sessions[i].LastMeaningfulOutput
		if ti.IsZero() {
			ti = sessions[i].CreatedAt
		}
		tj := sessions[j].LastMeaningfulOutput
		if tj.IsZero() {
			tj = sessions[j].CreatedAt
		}

		if idx.sortDirection == SortDescending {
			return ti.After(tj) // Newest first
		}
		return ti.Before(tj) // Oldest first
	})
}

// sortByCreationDate sorts sessions by CreatedAt timestamp
// Uses LastActivity as secondary sort for stability (per ADR-3)
func (idx *SearchIndex) sortByCreationDate(sessions []*session.Instance) {
	sort.SliceStable(sessions, func(i, j int) bool {
		ti := sessions[i].CreatedAt
		tj := sessions[j].CreatedAt

		if ti.Equal(tj) {
			// Secondary sort by LastActivity for stability
			ai := sessions[i].LastMeaningfulOutput
			if ai.IsZero() {
				ai = sessions[i].CreatedAt
			}
			aj := sessions[j].LastMeaningfulOutput
			if aj.IsZero() {
				aj = sessions[j].CreatedAt
			}
			return ai.After(aj) // Most recently active first for ties
		}

		if idx.sortDirection == SortDescending {
			return ti.After(tj) // Newest first
		}
		return ti.Before(tj) // Oldest first
	})
}

// sortByTitle sorts sessions alphabetically by Title
// Uses LastActivity as secondary sort for stability (per ADR-3)
func (idx *SearchIndex) sortByTitle(sessions []*session.Instance) {
	sort.SliceStable(sessions, func(i, j int) bool {
		titleI := strings.ToLower(sessions[i].Title)
		titleJ := strings.ToLower(sessions[j].Title)

		if titleI == titleJ {
			// Secondary sort by LastActivity for stability
			ai := sessions[i].LastMeaningfulOutput
			if ai.IsZero() {
				ai = sessions[i].CreatedAt
			}
			aj := sessions[j].LastMeaningfulOutput
			if aj.IsZero() {
				aj = sessions[j].CreatedAt
			}
			return ai.After(aj) // Most recently active first for ties
		}

		if idx.sortDirection == SortDescending {
			return titleI > titleJ // Z-A
		}
		return titleI < titleJ // A-Z
	})
}

// sortByRepository sorts sessions by repository path (basename)
// Uses LastActivity as secondary sort for stability (per ADR-3)
func (idx *SearchIndex) sortByRepository(sessions []*session.Instance) {
	sort.SliceStable(sessions, func(i, j int) bool {
		repoI := strings.ToLower(filepath.Base(sessions[i].Path))
		repoJ := strings.ToLower(filepath.Base(sessions[j].Path))

		if repoI == repoJ {
			// Secondary sort by LastActivity for stability
			ai := sessions[i].LastMeaningfulOutput
			if ai.IsZero() {
				ai = sessions[i].CreatedAt
			}
			aj := sessions[j].LastMeaningfulOutput
			if aj.IsZero() {
				aj = sessions[j].CreatedAt
			}
			return ai.After(aj) // Most recently active first for ties
		}

		if idx.sortDirection == SortDescending {
			return repoI > repoJ // Z-A
		}
		return repoI < repoJ // A-Z
	})
}

// sortByBranch sorts sessions by branch name
// Uses LastActivity as secondary sort for stability (per ADR-3)
func (idx *SearchIndex) sortByBranch(sessions []*session.Instance) {
	sort.SliceStable(sessions, func(i, j int) bool {
		branchI := strings.ToLower(sessions[i].Branch)
		branchJ := strings.ToLower(sessions[j].Branch)

		// Sessions without branches sort last
		if branchI == "" && branchJ != "" {
			return false
		}
		if branchI != "" && branchJ == "" {
			return true
		}

		if branchI == branchJ {
			// Secondary sort by LastActivity for stability
			ai := sessions[i].LastMeaningfulOutput
			if ai.IsZero() {
				ai = sessions[i].CreatedAt
			}
			aj := sessions[j].LastMeaningfulOutput
			if aj.IsZero() {
				aj = sessions[j].CreatedAt
			}
			return ai.After(aj) // Most recently active first for ties
		}

		if idx.sortDirection == SortDescending {
			return branchI > branchJ // Z-A
		}
		return branchI < branchJ // A-Z
	})
}

// statusPriority returns the sort priority for a session status
// Running has highest priority, Paused has lowest
func statusPriority(status session.Status) int {
	switch status {
	case session.Running:
		return 0
	case session.Ready:
		return 1
	case session.NeedsApproval:
		return 2
	case session.Loading:
		return 3
	case session.Paused:
		return 4
	default:
		return 5
	}
}

// sortByStatus sorts sessions by status priority
// Priority: Running > Ready > NeedsApproval > Loading > Paused
// Uses LastActivity as secondary sort for stability (per ADR-3)
func (idx *SearchIndex) sortByStatus(sessions []*session.Instance) {
	sort.SliceStable(sessions, func(i, j int) bool {
		priI := statusPriority(sessions[i].Status)
		priJ := statusPriority(sessions[j].Status)

		if priI == priJ {
			// Secondary sort by LastActivity for stability
			ai := sessions[i].LastMeaningfulOutput
			if ai.IsZero() {
				ai = sessions[i].CreatedAt
			}
			aj := sessions[j].LastMeaningfulOutput
			if aj.IsZero() {
				aj = sessions[j].CreatedAt
			}
			return ai.After(aj) // Most recently active first for ties
		}

		if idx.sortDirection == SortDescending {
			return priI > priJ // Paused first (reverse priority)
		}
		return priI < priJ // Running first (normal priority)
	})
}

// =============================================================================
// Sort Configuration Methods
// =============================================================================

// SetSortMode sets the sort mode and invalidates the cache
func (idx *SearchIndex) SetSortMode(mode SortMode) {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	if idx.sortMode != mode {
		idx.sortMode = mode
		idx.sortedCacheValid = false
	}
}

// GetSortMode returns the current sort mode
func (idx *SearchIndex) GetSortMode() SortMode {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.sortMode
}

// SetSortDirection sets the sort direction and invalidates the cache
func (idx *SearchIndex) SetSortDirection(dir SortDirection) {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	if idx.sortDirection != dir {
		idx.sortDirection = dir
		idx.sortedCacheValid = false
	}
}

// GetSortDirection returns the current sort direction
func (idx *SearchIndex) GetSortDirection() SortDirection {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.sortDirection
}

// ToggleSortDirection toggles between ascending and descending
func (idx *SearchIndex) ToggleSortDirection() {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	if idx.sortDirection == SortDescending {
		idx.sortDirection = SortAscending
	} else {
		idx.sortDirection = SortDescending
	}
	idx.sortedCacheValid = false
}

// CycleSortMode cycles to the next sort mode
// Sequence: LastActivity → CreationDate → TitleAZ → Repository → Branch → Status → LastActivity
func (idx *SearchIndex) CycleSortMode() {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	idx.sortMode = (idx.sortMode + 1) % 6
	idx.sortedCacheValid = false
}

// InvalidateSortCache marks the sort cache as invalid
// Call this when session data changes but index doesn't need full rebuild
func (idx *SearchIndex) InvalidateSortCache() {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	idx.sortedCacheValid = false
}

// GetSortDescription returns a human-readable description of current sort settings
func (idx *SearchIndex) GetSortDescription() string {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.sortMode.ShortName() + " " + idx.sortDirection.Icon()
}