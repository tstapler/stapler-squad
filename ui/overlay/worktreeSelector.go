package overlay

import (
	"claude-squad/log"
	"claude-squad/ui/fuzzy"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// WorktreeInfo represents information about a Git worktree
type WorktreeInfo struct {
	// Path to the worktree
	Path string

	// Name of the worktree (directory name)
	Name string

	// Branch associated with the worktree
	Branch string

	// Commit SHA of the worktree's HEAD
	CommitSHA string

	// When the worktree was created (approximated from directory mtime)
	CreatedAt time.Time

	// Whether the worktree is active (exists on disk)
	Active bool

	// Repo path that the worktree belongs to
	RepoPath string
}

// GetSearchText returns text used for fuzzy matching
func (w WorktreeInfo) GetSearchText() string {
	return w.Name + " " + w.Branch
}

// GetDisplayText returns text to display in UI
func (w WorktreeInfo) GetDisplayText() string {
	age := time.Since(w.CreatedAt)

	// Format age as friendly string
	ageStr := ""
	if age < 24*time.Hour {
		ageStr = "today"
	} else if age < 48*time.Hour {
		ageStr = "yesterday"
	} else if age < 7*24*time.Hour {
		ageStr = fmt.Sprintf("%d days ago", int(age.Hours()/24))
	} else if age < 30*24*time.Hour {
		ageStr = fmt.Sprintf("%d weeks ago", int(age.Hours()/(24*7)))
	} else {
		ageStr = fmt.Sprintf("%d months ago", int(age.Hours()/(24*30)))
	}

	statusSymbol := "🟢"
	if !w.Active {
		statusSymbol = "⚪"
	}

	return fmt.Sprintf("%s %s (%s) - Created %s", statusSymbol, w.Name, w.Branch, ageStr)
}

// GetID returns unique identifier
func (w WorktreeInfo) GetID() string {
	return w.Path
}

// WorktreeCache caches worktree information
type WorktreeCache struct {
	worktrees  map[string]WorktreeInfo
	lastUpdate time.Time
	mutex      sync.RWMutex
}

// NewWorktreeCache creates a new worktree cache
func NewWorktreeCache() *WorktreeCache {
	return &WorktreeCache{
		worktrees:  make(map[string]WorktreeInfo),
		lastUpdate: time.Time{},
		mutex:      sync.RWMutex{},
	}
}

// GetWorktree gets a worktree from the cache
func (c *WorktreeCache) GetWorktree(path string) (WorktreeInfo, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	worktree, ok := c.worktrees[path]
	return worktree, ok
}

// AddWorktree adds a worktree to the cache
func (c *WorktreeCache) AddWorktree(worktree WorktreeInfo) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.worktrees[worktree.Path] = worktree
}

// GetAllWorktrees returns all worktrees
func (c *WorktreeCache) GetAllWorktrees() []WorktreeInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	worktrees := make([]WorktreeInfo, 0, len(c.worktrees))
	for _, worktree := range c.worktrees {
		worktrees = append(worktrees, worktree)
	}

	// Sort by creation time (most recent first)
	sort.Slice(worktrees, func(i, j int) bool {
		return worktrees[i].CreatedAt.After(worktrees[j].CreatedAt)
	})

	return worktrees
}

// ListWorktreesForRepo lists worktrees for a specific repository
func (c *WorktreeCache) ListWorktreesForRepo(repoPath string) []WorktreeInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	worktrees := make([]WorktreeInfo, 0)
	for _, worktree := range c.worktrees {
		if worktree.RepoPath == repoPath {
			worktrees = append(worktrees, worktree)
		}
	}

	// Sort by creation time (most recent first)
	sort.Slice(worktrees, func(i, j int) bool {
		return worktrees[i].CreatedAt.After(worktrees[j].CreatedAt)
	})

	return worktrees
}

// ScanForWorktrees scans for Git worktrees in a specific repository
func (c *WorktreeCache) ScanForWorktrees(repoPath string) error {
	// Run git command to list all worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse the output
	worktrees := parseGitWorktreeList(string(output), repoPath)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Update cache
	for _, worktree := range worktrees {
		c.worktrees[worktree.Path] = worktree
	}

	c.lastUpdate = time.Now()

	return nil
}

// parseGitWorktreeList parses the output of `git worktree list --porcelain`
func parseGitWorktreeList(output string, repoPath string) []WorktreeInfo {
	worktrees := make([]WorktreeInfo, 0)

	// Split into worktree entries (separated by blank lines)
	worktreeBlocks := strings.Split(output, "\n\n")

	for _, block := range worktreeBlocks {
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")

		// Parse the worktree information
		var path, branch, commit string

		for _, line := range lines {
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, " ", 2)
			if len(parts) != 2 {
				continue
			}

			key, value := parts[0], parts[1]

			switch key {
			case "worktree":
				path = value
			case "branch":
				if strings.HasPrefix(value, "refs/heads/") {
					branch = strings.TrimPrefix(value, "refs/heads/")
				} else {
					branch = value
				}
			case "HEAD":
				commit = value
			}
		}

		// Verify the worktree exists
		active := true
		if _, err := os.Stat(path); os.IsNotExist(err) {
			active = false
		}

		// Get creation time from directory mtime
		var createdAt time.Time
		if active {
			info, err := os.Stat(path)
			if err == nil {
				createdAt = info.ModTime()
			}
		}

		// Create WorktreeInfo
		if path != "" {
			worktrees = append(worktrees, WorktreeInfo{
				Path:      path,
				Name:      filepath.Base(path),
				Branch:    branch,
				CommitSHA: commit,
				CreatedAt: createdAt,
				Active:    active,
				RepoPath:  repoPath,
			})
		}
	}

	return worktrees
}

// WorktreeLoader provides an async loader for the fuzzy search component
type WorktreeLoader struct {
	cache    *WorktreeCache
	repoPath string
}

// NewWorktreeLoader creates a new worktree loader for a specific repository
func NewWorktreeLoader(repoPath string) *WorktreeLoader {
	cache := NewWorktreeCache()

	// Start a background scan for worktrees
	go func() {
		if err := cache.ScanForWorktrees(repoPath); err != nil {
			log.WarningLog.Printf("Error scanning for worktrees: %v", err)
		}
	}()

	return &WorktreeLoader{
		cache:    cache,
		repoPath: repoPath,
	}
}

// AsyncLoad implements fuzzy.AsyncLoader interface
func (l *WorktreeLoader) AsyncLoad(query string) ([]fuzzy.SearchItem, error) {
	// Scan for worktrees if cache is empty or stale
	if l.cache.lastUpdate.IsZero() || time.Since(l.cache.lastUpdate) > 30*time.Second {
		if err := l.cache.ScanForWorktrees(l.repoPath); err != nil {
			return nil, err
		}
	}

	// Get worktrees from cache
	worktrees := l.cache.ListWorktreesForRepo(l.repoPath)

	// Convert to search items
	items := make([]fuzzy.SearchItem, len(worktrees))
	for i, worktree := range worktrees {
		items[i] = worktree
	}

	return items, nil
}

// GetRepoPath returns the repository path
func (l *WorktreeLoader) GetRepoPath() string {
	return l.repoPath
}

// GetWorktreeByPath returns a worktree by path
func (l *WorktreeLoader) GetWorktreeByPath(path string) (WorktreeInfo, bool) {
	return l.cache.GetWorktree(path)
}
