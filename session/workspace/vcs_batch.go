package workspace

import (
	"context"
	"sync"
	"time"

	"claude-squad/session/vc"
)

// BatchVCSProvider collects VCS status for multiple workspaces concurrently.
// It provides efficient batch operations with timeout handling and graceful degradation.
type BatchVCSProvider struct {
	maxConcurrent int           // Maximum concurrent operations
	timeout       time.Duration // Timeout for individual operations
}

// BatchStatusResult contains the result of a batch status operation
type BatchStatusResult struct {
	Path      string        // Workspace path
	Status    *vc.VCSStatus // VCS status (nil if error)
	Error     error         // Error (nil if success)
	Duration  time.Duration // How long the operation took
	IsPartial bool          // True if status is incomplete
}

// NewBatchVCSProvider creates a new batch VCS provider.
func NewBatchVCSProvider(maxConcurrent int, timeout time.Duration) *BatchVCSProvider {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &BatchVCSProvider{
		maxConcurrent: maxConcurrent,
		timeout:       timeout,
	}
}

// GetStatuses collects VCS status for multiple paths concurrently.
// It returns results for all paths, including errors for failed operations.
func (b *BatchVCSProvider) GetStatuses(ctx context.Context, paths []string) []BatchStatusResult {
	results := make([]BatchStatusResult, len(paths))

	// Use semaphore for concurrency control
	sem := make(chan struct{}, b.maxConcurrent)
	var wg sync.WaitGroup

	for i, path := range paths {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = BatchStatusResult{
					Path:  p,
					Error: ctx.Err(),
				}
				return
			}

			// Get status with timeout
			result := b.getStatusWithTimeout(ctx, p)
			results[idx] = result
		}(i, path)
	}

	wg.Wait()
	return results
}

// getStatusWithTimeout gets VCS status for a single path with timeout.
func (b *BatchVCSProvider) getStatusWithTimeout(ctx context.Context, path string) BatchStatusResult {
	start := time.Now()

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	// Create result channel
	resultCh := make(chan BatchStatusResult, 1)

	go func() {
		result := BatchStatusResult{Path: path}

		// Detect VCS type
		vcsType := vc.DetectVCS(path)
		if vcsType == vc.VCSUnknown {
			result.Error = vc.ErrNoVCSFound
			result.Duration = time.Since(start)
			resultCh <- result
			return
		}

		// Create provider
		provider, err := vc.NewProvider(path)
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			resultCh <- result
			return
		}

		// Get status
		status, err := provider.GetStatus()
		if err != nil {
			result.Error = err
			result.IsPartial = true
		}

		result.Status = status
		result.Duration = time.Since(start)
		resultCh <- result
	}()

	// Wait for result or timeout
	select {
	case result := <-resultCh:
		return result
	case <-ctx.Done():
		return BatchStatusResult{
			Path:      path,
			Error:     ctx.Err(),
			Duration:  time.Since(start),
			IsPartial: true,
		}
	}
}

// GetStatusesByRepository groups workspaces by repository root and collects status.
// This is more efficient when multiple workspaces share the same repository.
func (b *BatchVCSProvider) GetStatusesByRepository(ctx context.Context, workspaces []*TrackedWorkspace) map[string][]BatchStatusResult {
	// Group by repository root
	byRepo := make(map[string][]string)
	for _, ws := range workspaces {
		repoRoot := ws.RepositoryRoot
		if repoRoot == "" {
			repoRoot = ws.Path
		}
		byRepo[repoRoot] = append(byRepo[repoRoot], ws.Path)
	}

	// Collect results by repository
	results := make(map[string][]BatchStatusResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for repo, paths := range byRepo {
		wg.Add(1)
		go func(repoRoot string, wsPaths []string) {
			defer wg.Done()

			repoResults := b.GetStatuses(ctx, wsPaths)

			mu.Lock()
			results[repoRoot] = repoResults
			mu.Unlock()
		}(repo, paths)
	}

	wg.Wait()
	return results
}

// SummaryFromResults creates a ChangesSummary from batch results.
func SummaryFromResults(results []BatchStatusResult) *ChangesSummary {
	summary := &ChangesSummary{}
	repos := make(map[string]bool)

	for _, result := range results {
		summary.TotalWorkspaces++

		if result.Status != nil {
			repos[result.Path] = true

			summary.TotalStaged += len(result.Status.StagedFiles)
			summary.TotalUncommitted += len(result.Status.UnstagedFiles)
			summary.TotalUntracked += len(result.Status.UntrackedFiles)
			summary.TotalConflicts += len(result.Status.ConflictFiles)

			if !result.Status.IsClean {
				summary.WorkspacesWithWork++
			}
		}
	}

	summary.TotalRepositories = len(repos)
	return summary
}
