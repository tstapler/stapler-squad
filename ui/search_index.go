package ui

import (
	"claude-squad/session"
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sahilm/fuzzy"
	"github.com/schollz/closestmatch"
)

// SearchIndex provides optimized fuzzy search for sessions using hybrid indexing
type SearchIndex struct {
	// Primary fast matcher for pre-filtering
	closestMatch *closestmatch.ClosestMatch

	// Category optimization for instant category filtering
	categoryIndex map[string][]*session.Instance

	// Program optimization for instant program filtering
	programIndex map[string][]*session.Instance

	// Session lookup by search text
	sessionMap map[string]*session.Instance

	// All sessions for fallback sahilm/fuzzy search
	allSessions []*session.Instance

	// Index management
	version      int
	needsRebuild bool
	mutex        sync.RWMutex
}

// NewSearchIndex creates a new optimized search index
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		categoryIndex: make(map[string][]*session.Instance),
		programIndex:  make(map[string][]*session.Instance),
		sessionMap:    make(map[string]*session.Instance),
		allSessions:   make([]*session.Instance, 0),
		needsRebuild:  true,
	}
}

// RebuildIndex rebuilds the search index from the current session list
func (idx *SearchIndex) RebuildIndex(sessions []*session.Instance) {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	// Clear existing indices
	idx.categoryIndex = make(map[string][]*session.Instance)
	idx.programIndex = make(map[string][]*session.Instance)
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

	// Strategy 3: Hybrid approach - fast pre-filter + high-quality ranking
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

		// Stage 1: Fast category/program detection (immediate results)
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

	return map[string]interface{}{
		"version":          idx.version,
		"total_sessions":   len(idx.allSessions),
		"categories":       len(idx.categoryIndex),
		"programs":         len(idx.programIndex),
		"category_entries": categoryCount,
		"program_entries":  programCount,
		"needs_rebuild":    idx.needsRebuild,
		"index_valid":      idx.closestMatch != nil,
	}
}