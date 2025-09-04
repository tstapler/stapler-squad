package overlay

import (
	"claude-squad/log"
	"claude-squad/ui/fuzzy"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DirectoryInfo represents a directory in the file system
type DirectoryInfo struct {
	// Full path to the directory
	Path string

	// Display name (relative to parent)
	Name string

	// Whether this is a directory
	IsDir bool

	// Last modified time
	ModTime time.Time

	// Parent directory path
	ParentPath string

	// Depth from root
	Depth int
}

// GetSearchText returns text used for fuzzy matching
func (d DirectoryInfo) GetSearchText() string {
	return d.Name
}

// GetDisplayText returns text to display in UI
func (d DirectoryInfo) GetDisplayText() string {
	if d.IsDir {
		indent := strings.Repeat("  ", d.Depth)
		return indent + "📁 " + d.Name
	}
	return d.Name
}

// GetID returns unique identifier
func (d DirectoryInfo) GetID() string {
	return d.Path
}

// DirectoryCache caches directory listings for faster browsing
type DirectoryCache struct {
	// Map of directory path to its entries
	cache map[string][]DirectoryInfo

	// Last time each directory was read
	lastRead map[string]time.Time

	// Recent directories
	recents []string

	mutex sync.RWMutex
}

// NewDirectoryCache creates a new directory cache
func NewDirectoryCache() *DirectoryCache {
	return &DirectoryCache{
		cache:    make(map[string][]DirectoryInfo),
		lastRead: make(map[string]time.Time),
		recents:  []string{},
		mutex:    sync.RWMutex{},
	}
}

// GetEntries gets cached directory entries, reading from disk if needed
func (c *DirectoryCache) GetEntries(path string) ([]DirectoryInfo, error) {
	path = filepath.Clean(path)

	c.mutex.RLock()
	entries, ok := c.cache[path]
	lastRead, _ := c.lastRead[path]
	c.mutex.RUnlock()

	// If cache is fresh (less than 5 seconds old), use it
	if ok && time.Since(lastRead) < 5*time.Second {
		return entries, nil
	}

	// Otherwise read from disk
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	// Convert to DirectoryInfo
	entries = make([]DirectoryInfo, 0, len(dirEntries))
	for _, entry := range dirEntries {
		// Skip hidden files/directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(path, entry.Name())

		// Only include directories
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				// Skip entries we can't get info for
				continue
			}

			entries = append(entries, DirectoryInfo{
				Path:       fullPath,
				Name:       entry.Name(),
				IsDir:      true,
				ModTime:    info.ModTime(),
				ParentPath: path,
				Depth:      0, // Will be set when needed
			})
		}
	}

	// Sort by name
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	// Update cache
	c.mutex.Lock()
	c.cache[path] = entries
	c.lastRead[path] = time.Now()
	c.mutex.Unlock()

	return entries, nil
}

// AddRecentDirectory adds a directory to the recents list
func (c *DirectoryCache) AddRecentDirectory(path string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if already in recents
	for i, dir := range c.recents {
		if dir == path {
			// Move to front
			if i > 0 {
				copy(c.recents[1:i+1], c.recents[0:i])
				c.recents[0] = path
			}
			return
		}
	}

	// Add to front
	c.recents = append([]string{path}, c.recents...)

	// Limit to 10 recents
	if len(c.recents) > 10 {
		c.recents = c.recents[:10]
	}
}

// GetRecentDirectories returns the list of recent directories
func (c *DirectoryCache) GetRecentDirectories() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Create a copy to avoid race conditions
	result := make([]string, len(c.recents))
	copy(result, c.recents)

	return result
}

// DirectoryLoader provides an async loader for the fuzzy search directory browser
type DirectoryLoader struct {
	// Current directory being browsed
	currentPath string

	// Directory cache
	cache *DirectoryCache

	// Reference to repository root for relative paths
	repoRoot string

	// Current breadcrumb path components
	breadcrumbs []string
}

// NewDirectoryLoader creates a new directory browser
func NewDirectoryLoader(repoRoot string) *DirectoryLoader {
	cache := NewDirectoryCache()

	return &DirectoryLoader{
		currentPath: repoRoot,
		cache:       cache,
		repoRoot:    repoRoot,
		breadcrumbs: []string{filepath.Base(repoRoot)},
	}
}

// AsyncLoad implements fuzzy.AsyncLoader interface
func (l *DirectoryLoader) AsyncLoad(query string) ([]fuzzy.SearchItem, error) {
	// Special case for query starting with "/" - treat as path
	if strings.HasPrefix(query, "/") || strings.HasPrefix(query, "~") {
		path := query
		// Expand tilde
		if strings.HasPrefix(query, "~") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			path = home + query[1:]
		}

		// Try to load the directory
		entries, err := l.cache.GetEntries(path)
		if err != nil {
			return nil, err
		}

		// Set as current path
		l.currentPath = path

		// Update breadcrumbs
		l.updateBreadcrumbs()

		// Convert to search items
		items := make([]fuzzy.SearchItem, len(entries))
		for i, entry := range entries {
			// Update depth based on repo root
			entry.Depth = l.calculateDepth(entry.Path)
			items[i] = entry
		}

		return items, nil
	}

	// Regular directory listing
	entries, err := l.cache.GetEntries(l.currentPath)
	if err != nil {
		return nil, err
	}

	// Convert to search items
	items := make([]fuzzy.SearchItem, 0, len(entries)+1)

	// Add special ".." entry if not at repo root
	if l.currentPath != l.repoRoot {
		parentPath := filepath.Dir(l.currentPath)
		items = append(items, DirectoryInfo{
			Path:       parentPath,
			Name:       "..",
			IsDir:      true,
			ParentPath: filepath.Dir(parentPath),
			Depth:      0,
		})
	}

	// Add special "<repository root>" entry
	items = append(items, DirectoryInfo{
		Path:       l.repoRoot,
		Name:       "<Repository Root>",
		IsDir:      true,
		ParentPath: filepath.Dir(l.repoRoot),
		Depth:      0,
	})

	// Add directory entries
	for _, entry := range entries {
		// Update depth based on repo root
		entry.Depth = l.calculateDepth(entry.Path)
		items = append(items, entry)
	}

	// If we have a query, also add recent directories
	if query != "" {
		recentPaths := l.cache.GetRecentDirectories()
		for _, path := range recentPaths {
			// Skip if path is already in items
			duplicate := false
			for _, item := range items {
				if item.GetID() == path {
					duplicate = true
					break
				}
			}

			if !duplicate {
				// Get info about the directory
				info, err := os.Stat(path)
				if err != nil {
					log.WarningLog.Printf("Error getting info for recent dir %s: %v", path, err)
					continue
				}

				items = append(items, DirectoryInfo{
					Path:       path,
					Name:       "Recent: " + filepath.Base(path),
					IsDir:      true,
					ModTime:    info.ModTime(),
					ParentPath: filepath.Dir(path),
					Depth:      0,
				})
			}
		}
	}

	return items, nil
}

// calculateDepth calculates the depth of a path relative to the repo root
func (l *DirectoryLoader) calculateDepth(path string) int {
	// Make paths relative to repo root
	relPath, err := filepath.Rel(l.repoRoot, path)
	if err != nil {
		return 0
	}

	// Count path separators
	return strings.Count(relPath, string(filepath.Separator))
}

// ChangeDirectory changes the current directory
func (l *DirectoryLoader) ChangeDirectory(path string) error {
	// Verify the path exists and is a directory
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fs.ErrNotExist
	}

	l.currentPath = path
	l.updateBreadcrumbs()
	l.cache.AddRecentDirectory(path)

	return nil
}

// GetCurrentPath returns the current directory path
func (l *DirectoryLoader) GetCurrentPath() string {
	return l.currentPath
}

// GetRelativePath returns the path relative to the repo root
func (l *DirectoryLoader) GetRelativePath() string {
	relPath, err := filepath.Rel(l.repoRoot, l.currentPath)
	if err != nil {
		return ""
	}

	if relPath == "." {
		return ""
	}

	return relPath
}

// GetBreadcrumbs returns the path components for navigation
func (l *DirectoryLoader) GetBreadcrumbs() []string {
	return l.breadcrumbs
}

// updateBreadcrumbs updates the breadcrumb path components
func (l *DirectoryLoader) updateBreadcrumbs() {
	// Create path relative to repo root
	relPath, err := filepath.Rel(l.repoRoot, l.currentPath)
	if err != nil {
		// Fall back to absolute path components
		l.breadcrumbs = strings.Split(l.currentPath, string(filepath.Separator))
		return
	}

	// Special case for repo root
	if relPath == "." {
		l.breadcrumbs = []string{filepath.Base(l.repoRoot)}
		return
	}

	// Start with repo name
	breadcrumbs := []string{filepath.Base(l.repoRoot)}

	// Add path components
	components := strings.Split(relPath, string(filepath.Separator))
	breadcrumbs = append(breadcrumbs, components...)

	l.breadcrumbs = breadcrumbs
}
