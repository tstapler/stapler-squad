package workspace

import (
	"context"
	"sync"
	"time"
)

// Registry tracks all known workspaces and their status.
// It provides a central point for workspace discovery, status caching,
// and coordination across multiple pods via distributed locking.
type Registry interface {
	// Registration
	Register(ctx context.Context, workspace *TrackedWorkspace) error
	Unregister(ctx context.Context, path string) error
	MarkOrphaned(ctx context.Context, path string) error

	// Query
	Get(ctx context.Context, path string) (*TrackedWorkspace, error)
	List(ctx context.Context, filter *WorkspaceFilter) ([]*TrackedWorkspace, error)
	ListByRepository(ctx context.Context, repoRoot string) ([]*TrackedWorkspace, error)

	// Status
	GetStatus(ctx context.Context, path string, opts StatusRefreshOptions) (*WorkspaceStatus, error)
	GetAllStatuses(ctx context.Context, opts StatusRefreshOptions) ([]*WorkspaceStatus, error)
	GetSummary(ctx context.Context) (*ChangesSummary, error)

	// Batch operations
	RefreshStatuses(ctx context.Context, paths []string, opts StatusRefreshOptions) ([]*WorkspaceStatus, error)

	// Lifecycle
	Close() error
}

// RegistryConfig configures the workspace registry
type RegistryConfig struct {
	// Distributed lock for multi-pod coordination
	Lock DistributedLock

	// Cache invalidation for cross-pod coordination
	Notifier CacheInvalidationNotifier

	// Cache settings
	CacheTTL         time.Duration // How long to cache status (default: 30s)
	MaxCacheSize     int           // Maximum cached entries (default: 1000)
	RefreshBatchSize int           // Max concurrent status refreshes (default: 10)

	// Timeouts
	LockTimeout      time.Duration // Lock acquisition timeout (default: 10s)
	OperationTimeout time.Duration // Individual operation timeout (default: 5s)
}

// DefaultRegistryConfig returns sensible defaults
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		CacheTTL:         30 * time.Second,
		MaxCacheSize:     1000,
		RefreshBatchSize: 10,
		LockTimeout:      10 * time.Second,
		OperationTimeout: 5 * time.Second,
	}
}

// WorkspaceRegistry is the default implementation of Registry.
// It uses in-memory caching with optional distributed locking.
type WorkspaceRegistry struct {
	config RegistryConfig

	// In-memory state (protected by mu)
	mu           sync.RWMutex
	workspaces   map[string]*TrackedWorkspace // keyed by absolute path
	byRepository map[string][]string          // repo root -> workspace paths
	statusCache  map[string]*cachedStatus     // path -> cached status

	// Distributed coordination
	distLock DistributedLock
	notifier CacheInvalidationNotifier

	// Background refresh
	refreshChan chan string
	stopChan    chan struct{}
}

// cachedStatus holds a workspace status with cache metadata
type cachedStatus struct {
	Status    *WorkspaceStatus
	CachedAt  time.Time
	ExpiresAt time.Time
}

// NewRegistry creates a new workspace registry with the given configuration.
func NewRegistry(config RegistryConfig) *WorkspaceRegistry {
	if config.CacheTTL == 0 {
		config = DefaultRegistryConfig()
	}

	r := &WorkspaceRegistry{
		config:       config,
		workspaces:   make(map[string]*TrackedWorkspace),
		byRepository: make(map[string][]string),
		statusCache:  make(map[string]*cachedStatus),
		distLock:     config.Lock,
		notifier:     config.Notifier,
		refreshChan:  make(chan string, 100),
		stopChan:     make(chan struct{}),
	}

	// Start cache invalidation listener if notifier configured
	if r.notifier != nil {
		go r.listenForInvalidations()
	}

	return r
}

// Register adds a workspace to the registry.
func (r *WorkspaceRegistry) Register(ctx context.Context, workspace *TrackedWorkspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Store workspace
	r.workspaces[workspace.Path] = workspace

	// Update repository index
	repoRoot := workspace.RepositoryRoot
	if repoRoot == "" {
		repoRoot = workspace.Path
	}
	r.byRepository[repoRoot] = appendUnique(r.byRepository[repoRoot], workspace.Path)

	return nil
}

// Unregister removes a workspace from the registry.
func (r *WorkspaceRegistry) Unregister(ctx context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	workspace, exists := r.workspaces[path]
	if !exists {
		return nil // Already unregistered
	}

	// Remove from repository index
	repoRoot := workspace.RepositoryRoot
	if repoRoot == "" {
		repoRoot = workspace.Path
	}
	r.byRepository[repoRoot] = removeString(r.byRepository[repoRoot], path)
	if len(r.byRepository[repoRoot]) == 0 {
		delete(r.byRepository, repoRoot)
	}

	// Remove workspace and cached status
	delete(r.workspaces, path)
	delete(r.statusCache, path)

	// Notify other pods
	if r.notifier != nil {
		go func() {
			_ = r.notifier.Publish(context.Background(), path)
		}()
	}

	return nil
}

// MarkOrphaned marks a workspace as orphaned (no active session).
func (r *WorkspaceRegistry) MarkOrphaned(ctx context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	workspace, exists := r.workspaces[path]
	if !exists {
		return nil
	}

	workspace.IsOrphaned = true
	workspace.SessionTitle = ""
	workspace.NeedsAttention = true
	workspace.AttentionReason = "No active session"

	return nil
}

// Get retrieves a tracked workspace by path.
func (r *WorkspaceRegistry) Get(ctx context.Context, path string) (*TrackedWorkspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workspace, exists := r.workspaces[path]
	if !exists {
		return nil, nil
	}
	return workspace, nil
}

// List returns all tracked workspaces matching the filter.
func (r *WorkspaceRegistry) List(ctx context.Context, filter *WorkspaceFilter) ([]*TrackedWorkspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*TrackedWorkspace
	for _, ws := range r.workspaces {
		if matchesFilter(ws, filter) {
			result = append(result, ws)
		}
	}
	return result, nil
}

// ListByRepository returns all workspaces for a given repository root.
func (r *WorkspaceRegistry) ListByRepository(ctx context.Context, repoRoot string) ([]*TrackedWorkspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	paths := r.byRepository[repoRoot]
	result := make([]*TrackedWorkspace, 0, len(paths))
	for _, path := range paths {
		if ws, exists := r.workspaces[path]; exists {
			result = append(result, ws)
		}
	}
	return result, nil
}

// GetStatus retrieves the VCS status for a workspace.
func (r *WorkspaceRegistry) GetStatus(ctx context.Context, path string, opts StatusRefreshOptions) (*WorkspaceStatus, error) {
	// Check cache first
	if !opts.Force {
		if cached := r.getCachedStatus(path, opts.MaxAge); cached != nil {
			return cached, nil
		}
	}

	// Refresh status
	statuses, err := r.RefreshStatuses(ctx, []string{path}, opts)
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, nil
	}
	return statuses[0], nil
}

// GetAllStatuses retrieves VCS status for all tracked workspaces.
func (r *WorkspaceRegistry) GetAllStatuses(ctx context.Context, opts StatusRefreshOptions) ([]*WorkspaceStatus, error) {
	r.mu.RLock()
	paths := make([]string, 0, len(r.workspaces))
	for path := range r.workspaces {
		paths = append(paths, path)
	}
	r.mu.RUnlock()

	return r.RefreshStatuses(ctx, paths, opts)
}

// GetSummary returns aggregated statistics across all workspaces.
func (r *WorkspaceRegistry) GetSummary(ctx context.Context) (*ChangesSummary, error) {
	statuses, err := r.GetAllStatuses(ctx, StatusRefreshOptions{MaxAge: 60 * time.Second})
	if err != nil {
		return nil, err
	}

	summary := &ChangesSummary{}
	repos := make(map[string]bool)

	for _, status := range statuses {
		summary.TotalWorkspaces++

		if status.IsOrphaned {
			summary.OrphanedWorkspaces++
		}

		if status.VCSStatus != nil {
			repos[status.WorkspacePath] = true
			summary.TotalStaged += len(status.VCSStatus.StagedFiles)
			summary.TotalUncommitted += len(status.VCSStatus.UnstagedFiles)
			summary.TotalUntracked += len(status.VCSStatus.UntrackedFiles)
			summary.TotalConflicts += len(status.VCSStatus.ConflictFiles)

			if !status.VCSStatus.IsClean {
				summary.WorkspacesWithWork++
			}
		}
	}

	summary.TotalRepositories = len(repos)
	return summary, nil
}

// RefreshStatuses refreshes VCS status for the given workspace paths.
func (r *WorkspaceRegistry) RefreshStatuses(ctx context.Context, paths []string, opts StatusRefreshOptions) ([]*WorkspaceStatus, error) {
	// This is a placeholder - actual implementation will call VCS providers
	// For now, return cached or empty statuses
	results := make([]*WorkspaceStatus, 0, len(paths))

	for _, path := range paths {
		r.mu.RLock()
		ws, exists := r.workspaces[path]
		r.mu.RUnlock()

		if !exists {
			continue
		}

		status := &WorkspaceStatus{
			WorkspacePath: path,
			SessionTitle:  ws.SessionTitle,
			SessionStatus: ws.SessionStatus,
			IsOrphaned:    ws.IsOrphaned,
			IsWorktree:    ws.WorktreePath != "",
			LastChecked:   time.Now(),
		}

		// Cache the status
		r.cacheStatus(path, status)
		results = append(results, status)
	}

	return results, nil
}

// Close stops background processes and releases resources.
func (r *WorkspaceRegistry) Close() error {
	close(r.stopChan)

	if r.notifier != nil {
		return r.notifier.Close()
	}
	return nil
}

// getCachedStatus returns cached status if fresh, nil otherwise.
func (r *WorkspaceRegistry) getCachedStatus(path string, maxAge time.Duration) *WorkspaceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cached, exists := r.statusCache[path]
	if !exists {
		return nil
	}

	if maxAge == 0 {
		maxAge = r.config.CacheTTL
	}

	if time.Since(cached.CachedAt) > maxAge {
		return nil
	}

	return cached.Status
}

// cacheStatus stores a status in the cache.
func (r *WorkspaceRegistry) cacheStatus(path string, status *WorkspaceStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.statusCache[path] = &cachedStatus{
		Status:    status,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(r.config.CacheTTL),
	}
}

// listenForInvalidations handles cache invalidation events from other pods.
func (r *WorkspaceRegistry) listenForInvalidations() {
	ctx := context.Background()
	_ = r.notifier.Subscribe(ctx, func(workspacePath string) {
		r.mu.Lock()
		defer r.mu.Unlock()

		if workspacePath == "" {
			// Invalidate all
			r.statusCache = make(map[string]*cachedStatus)
		} else {
			delete(r.statusCache, workspacePath)
		}
	})
}

// Helper functions

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func matchesFilter(ws *TrackedWorkspace, filter *WorkspaceFilter) bool {
	if filter == nil {
		return true
	}

	if !filter.IncludeOrphaned && ws.IsOrphaned {
		return false
	}

	if filter.RepositoryRoot != "" && ws.RepositoryRoot != filter.RepositoryRoot {
		return false
	}

	if filter.SessionStatus != nil && ws.SessionStatus != *filter.SessionStatus {
		return false
	}

	return true
}
