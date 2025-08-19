package overlay

import (
	"claude-squad/log"
	"claude-squad/ui/fuzzy"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
)

// RepoInfo represents information about a Git repository
type RepoInfo struct {
	// Path to the repository
	Path string
	
	// Name of the repository (directory name)
	Name string
	
	// Last access time
	LastAccessed time.Time
	
	// If the repo is a favorite
	Favorite bool
}

// GetSearchText returns text used for fuzzy matching
func (r RepoInfo) GetSearchText() string {
	return r.Name + " " + r.Path
}

// GetDisplayText returns text to display in UI
func (r RepoInfo) GetDisplayText() string {
	displayName := r.Name
	if r.Favorite {
		displayName = "★ " + displayName
	}
	return displayName + " - " + r.Path
}

// GetID returns unique identifier
func (r RepoInfo) GetID() string {
	return r.Path
}

// RepoCache caches known repositories for faster loading
type RepoCache struct {
	repos      map[string]RepoInfo
	favorites  []string
	recents    []string
	lastUpdate time.Time
	mutex      sync.RWMutex
}

// NewRepoCache creates a new repository cache
func NewRepoCache() *RepoCache {
	return &RepoCache{
		repos:      make(map[string]RepoInfo),
		favorites:  []string{},
		recents:    []string{},
		lastUpdate: time.Time{},
		mutex:      sync.RWMutex{},
	}
}

// GetRepo gets a repository from the cache
func (c *RepoCache) GetRepo(path string) (RepoInfo, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	repo, ok := c.repos[path]
	return repo, ok
}

// AddRepo adds a repository to the cache
func (c *RepoCache) AddRepo(repo RepoInfo) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.repos[repo.Path] = repo
	
	// Update last accessed time
	if !repo.LastAccessed.IsZero() {
		// Check if it's already in recents
		found := false
		for i, path := range c.recents {
			if path == repo.Path {
				// Move to front if found
				if i > 0 {
					copy(c.recents[1:i+1], c.recents[0:i])
					c.recents[0] = path
				}
				found = true
				break
			}
		}
		
		// Add to recents if not found
		if !found {
			c.recents = append([]string{repo.Path}, c.recents...)
			// Limit recents to 10
			if len(c.recents) > 10 {
				c.recents = c.recents[:10]
			}
		}
	}
}

// ToggleFavorite toggles whether a repository is a favorite
func (c *RepoCache) ToggleFavorite(path string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	repo, ok := c.repos[path]
	if !ok {
		return false
	}
	
	repo.Favorite = !repo.Favorite
	c.repos[path] = repo
	
	// Update favorites list
	if repo.Favorite {
		c.favorites = append(c.favorites, path)
	} else {
		for i, fav := range c.favorites {
			if fav == path {
				c.favorites = append(c.favorites[:i], c.favorites[i+1:]...)
				break
			}
		}
	}
	
	return repo.Favorite
}

// ListRecentRepos returns a list of recent repositories
func (c *RepoCache) ListRecentRepos() []RepoInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	repos := make([]RepoInfo, 0, len(c.recents))
	for _, path := range c.recents {
		if repo, ok := c.repos[path]; ok {
			repos = append(repos, repo)
		}
	}
	return repos
}

// ListFavoriteRepos returns a list of favorite repositories
func (c *RepoCache) ListFavoriteRepos() []RepoInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	repos := make([]RepoInfo, 0, len(c.favorites))
	for _, path := range c.favorites {
		if repo, ok := c.repos[path]; ok {
			repos = append(repos, repo)
		}
	}
	return repos
}

// GetAllRepos returns all repositories
func (c *RepoCache) GetAllRepos() []RepoInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	repos := make([]RepoInfo, 0, len(c.repos))
	for _, repo := range c.repos {
		repos = append(repos, repo)
	}
	
	// Sort by favorite status and then name
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Favorite != repos[j].Favorite {
			return repos[i].Favorite
		}
		return repos[i].Name < repos[j].Name
	})
	
	return repos
}

// ScanForRepos scans the filesystem for Git repositories
// searchPaths is a list of paths to search, defaulting to home directory if empty
func (c *RepoCache) ScanForRepos(searchPaths []string) error {
	if len(searchPaths) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		
		// Default search paths
		searchPaths = []string{
			home + "/projects",
			home + "/git",
			home + "/repos",
			home + "/src",
			home + "/go/src",
		}
	}
	
	// Start a goroutine for each search path
	var wg sync.WaitGroup
	reposChan := make(chan RepoInfo)
	errChan := make(chan error, len(searchPaths))
	
	for _, path := range searchPaths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			err := scanPath(path, reposChan)
			if err != nil {
				errChan <- err
			}
		}(path)
	}
	
	// Collect results in a separate goroutine
	go func() {
		wg.Wait()
		close(reposChan)
		close(errChan)
	}()
	
	// Process found repositories
	for repo := range reposChan {
		c.AddRepo(repo)
	}
	
	// Check for errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	
	c.lastUpdate = time.Now()
	
	if len(errs) > 0 {
		return errs[0] // Return first error
	}
	return nil
}

// scanPath recursively scans a path for Git repositories
func scanPath(root string, repos chan<- RepoInfo) error {
	// Check if the root itself is a Git repository
	if isGitRepo(root) {
		repos <- newRepoInfo(root)
		// Don't recurse into .git directories
		if filepath.Base(root) == ".git" {
			return nil
		}
	}
	
	// Start recursive search
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		// Skip errors
		if err != nil {
			log.WarningLog.Printf("Error scanning %s: %v", path, err)
			return filepath.SkipDir
		}
		
		// Skip non-directories
		if !info.IsDir() {
			return nil
		}
		
		// Skip hidden directories
		if strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		
		// Check if this directory is a Git repository
		if isGitRepo(path) {
			repos <- newRepoInfo(path)
			// Skip further recursion into this directory
			return filepath.SkipDir
		}
		
		return nil
	})
}

// isGitRepo checks if a directory is a Git repository
func isGitRepo(path string) bool {
	// Quick check for .git directory
	gitDir := filepath.Join(path, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		return true
	}
	
	// Try to open as Git repo
	_, err := git.PlainOpen(path)
	return err == nil
}

// newRepoInfo creates a new RepoInfo for a repository
func newRepoInfo(path string) RepoInfo {
	name := filepath.Base(path)
	
	// Try to get last access time
	var lastAccessed time.Time
	info, err := os.Stat(path)
	if err == nil {
		lastAccessed = info.ModTime()
	}
	
	return RepoInfo{
		Path:        path,
		Name:        name,
		LastAccessed: lastAccessed,
		Favorite:    false,
	}
}

// RepositoryLoader provides an async loader for the fuzzy search component
type RepositoryLoader struct {
	cache *RepoCache
}

// NewRepositoryLoader creates a new repository loader
func NewRepositoryLoader() *RepositoryLoader {
	cache := NewRepoCache()
	
	// Start a background scan for repositories
	go func() {
		if err := cache.ScanForRepos(nil); err != nil {
			log.WarningLog.Printf("Error scanning for repositories: %v", err)
		}
	}()
	
	return &RepositoryLoader{cache: cache}
}

// AsyncLoad implements the fuzzy.AsyncLoader interface
func (l *RepositoryLoader) AsyncLoad(query string) ([]fuzzy.SearchItem, error) {
	// Convert RepoInfo list to SearchItems
	repos := l.cache.GetAllRepos()
	
	items := make([]fuzzy.SearchItem, len(repos))
	for i, repo := range repos {
		items[i] = repo
	}
	
	return items, nil
}

// AddRepoToRecents adds a repository to the recent list
func (l *RepositoryLoader) AddRepoToRecents(path string) {
	// Check if repo exists
	repo, ok := l.cache.GetRepo(path)
	if !ok {
		// Try to create a new repo entry
		if isGitRepo(path) {
			repo = newRepoInfo(path)
		} else {
			return // Not a valid repo
		}
	}
	
	// Update access time
	repo.LastAccessed = time.Now()
	l.cache.AddRepo(repo)
}

// ToggleFavorite toggles the favorite status of a repository
func (l *RepositoryLoader) ToggleFavorite(path string) bool {
	return l.cache.ToggleFavorite(path)
}

// GetRecentRepos returns recent repositories as search items
func (l *RepositoryLoader) GetRecentRepos() []fuzzy.SearchItem {
	repos := l.cache.ListRecentRepos()
	
	items := make([]fuzzy.SearchItem, len(repos))
	for i, repo := range repos {
		items[i] = repo
	}
	
	return items
}

// GetFavoriteRepos returns favorite repositories as search items
func (l *RepositoryLoader) GetFavoriteRepos() []fuzzy.SearchItem {
	repos := l.cache.ListFavoriteRepos()
	
	items := make([]fuzzy.SearchItem, len(repos))
	for i, repo := range repos {
		items[i] = repo
	}
	
	return items
}