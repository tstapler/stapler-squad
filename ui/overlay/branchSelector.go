package overlay

import (
	"claude-squad/log"
	"claude-squad/ui/fuzzy"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// BranchInfo represents information about a Git branch
type BranchInfo struct {
	// Name of the branch
	Name string

	// Full ref name (refs/heads/...)
	RefName string

	// Whether the branch is the current branch
	Current bool

	// Whether the branch is remote
	Remote bool

	// Remote name (if applicable)
	RemoteName string

	// Last commit hash on the branch
	CommitSHA string

	// Last commit message
	CommitMessage string

	// Last commit author
	CommitAuthor string

	// Last commit date
	CommitDate time.Time
}

// GetSearchText returns text used for fuzzy matching
func (b BranchInfo) GetSearchText() string {
	return b.Name
}

// GetDisplayText returns text to display in UI
func (b BranchInfo) GetDisplayText() string {
	prefix := "  "
	if b.Current {
		prefix = "* "
	}

	branchName := b.Name
	if b.Remote {
		branchName = fmt.Sprintf("remotes/%s/%s", b.RemoteName, b.Name)
	}

	// Format commit date
	age := time.Since(b.CommitDate)
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

	// Trim commit message if too long
	commitMsg := b.CommitMessage
	if len(commitMsg) > 30 {
		commitMsg = commitMsg[:27] + "..."
	}

	return fmt.Sprintf("%s%s - %s (%s)", prefix, branchName, commitMsg, ageStr)
}

// GetID returns unique identifier
func (b BranchInfo) GetID() string {
	return b.Name
}

// BranchCache caches branch information
type BranchCache struct {
	branches   map[string]BranchInfo
	lastUpdate time.Time
	mutex      sync.RWMutex
}

// NewBranchCache creates a new branch cache
func NewBranchCache() *BranchCache {
	return &BranchCache{
		branches:   make(map[string]BranchInfo),
		lastUpdate: time.Time{},
		mutex:      sync.RWMutex{},
	}
}

// GetBranch gets a branch from the cache
func (c *BranchCache) GetBranch(name string) (BranchInfo, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	branch, ok := c.branches[name]
	return branch, ok
}

// AddBranch adds a branch to the cache
func (c *BranchCache) AddBranch(branch BranchInfo) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.branches[branch.Name] = branch
}

// GetAllBranches returns all branches
func (c *BranchCache) GetAllBranches() []BranchInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	branches := make([]BranchInfo, 0, len(c.branches))
	for _, branch := range c.branches {
		branches = append(branches, branch)
	}

	// Sort branches: current branch first, then local branches, then remote branches
	sort.Slice(branches, func(i, j int) bool {
		// Current branch comes first
		if branches[i].Current && !branches[j].Current {
			return true
		}
		if !branches[i].Current && branches[j].Current {
			return false
		}

		// Local branches before remote branches
		if !branches[i].Remote && branches[j].Remote {
			return true
		}
		if branches[i].Remote && !branches[j].Remote {
			return false
		}

		// Alphabetical order within same type
		return branches[i].Name < branches[j].Name
	})

	return branches
}

// GetLocalBranches returns only local branches
func (c *BranchCache) GetLocalBranches() []BranchInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	branches := make([]BranchInfo, 0)
	for _, branch := range c.branches {
		if !branch.Remote {
			branches = append(branches, branch)
		}
	}

	// Sort branches: current branch first, then alphabetical
	sort.Slice(branches, func(i, j int) bool {
		// Current branch comes first
		if branches[i].Current && !branches[j].Current {
			return true
		}
		if !branches[i].Current && branches[j].Current {
			return false
		}

		// Alphabetical order
		return branches[i].Name < branches[j].Name
	})

	return branches
}

// ScanForBranches scans for Git branches in a repository
func (c *BranchCache) ScanForBranches(repoPath string, includeRemotes bool) error {
	// Get local branches
	if err := c.scanLocalBranches(repoPath); err != nil {
		return err
	}

	// Get remote branches if requested
	if includeRemotes {
		if err := c.scanRemoteBranches(repoPath); err != nil {
			// Non-fatal error, just log it
			log.WarningLog.Printf("Error scanning remote branches: %v", err)
		}
	}

	c.lastUpdate = time.Now()

	return nil
}

// scanLocalBranches scans for local Git branches
func (c *BranchCache) scanLocalBranches(repoPath string) error {
	// Run git command to list all local branches with verbose info
	cmd := exec.Command(
		"git",
		"branch",
		"--list",
		"--verbose",
		"--no-color",
		"--format=%(refname:short) %(objectname:short) %(authordate:iso) %(authorname) %(subject)")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse branch info
		parts := strings.SplitN(line, " ", 5)
		if len(parts) < 5 {
			continue
		}

		name := parts[0]
		sha := parts[1]

		// Parse date
		dateStr := parts[2] + " " + parts[3] // Format: 2023-01-01 12:00:00 +0000
		date, err := time.Parse("2006-01-02 15:04:05 -0700", dateStr)
		if err != nil {
			date = time.Now() // Fallback
		}

		author := parts[4]
		message := ""
		if len(parts) > 5 {
			message = parts[5]
		}

		// Check if current branch
		current := strings.HasPrefix(line, "* ")

		c.mutex.Lock()
		c.branches[name] = BranchInfo{
			Name:          name,
			RefName:       "refs/heads/" + name,
			Current:       current,
			Remote:        false,
			CommitSHA:     sha,
			CommitMessage: message,
			CommitAuthor:  author,
			CommitDate:    date,
		}
		c.mutex.Unlock()
	}

	return nil
}

// scanRemoteBranches scans for remote Git branches
func (c *BranchCache) scanRemoteBranches(repoPath string) error {
	// Run git command to list all remote branches with verbose info
	cmd := exec.Command(
		"git",
		"branch",
		"--remote",
		"--verbose",
		"--no-color",
		"--format=%(refname:short) %(objectname:short) %(authordate:iso) %(authorname) %(subject)")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse branch info
		parts := strings.SplitN(line, " ", 5)
		if len(parts) < 5 {
			continue
		}

		// Parse remote and branch name
		fullName := parts[0]
		remoteParts := strings.SplitN(fullName, "/", 2)
		if len(remoteParts) < 2 {
			continue
		}

		remoteName := remoteParts[0]
		name := remoteParts[1]

		// Skip HEAD references
		if name == "HEAD" {
			continue
		}

		sha := parts[1]

		// Parse date
		dateStr := parts[2] + " " + parts[3]
		date, err := time.Parse("2006-01-02 15:04:05 -0700", dateStr)
		if err != nil {
			date = time.Now() // Fallback
		}

		author := parts[4]
		message := ""
		if len(parts) > 5 {
			message = parts[5]
		}

		// Create a unique key for the remote branch
		key := "remote/" + remoteName + "/" + name

		c.mutex.Lock()
		c.branches[key] = BranchInfo{
			Name:          name,
			RefName:       "refs/remotes/" + remoteName + "/" + name,
			Current:       false,
			Remote:        true,
			RemoteName:    remoteName,
			CommitSHA:     sha,
			CommitMessage: message,
			CommitAuthor:  author,
			CommitDate:    date,
		}
		c.mutex.Unlock()
	}

	return nil
}

// BranchLoader provides an async loader for the fuzzy search component
type BranchLoader struct {
	cache          *BranchCache
	repoPath       string
	includeRemotes bool
}

// NewBranchLoader creates a new branch loader
func NewBranchLoader(repoPath string, includeRemotes bool) *BranchLoader {
	cache := NewBranchCache()

	// Start a background scan for branches
	go func() {
		if err := cache.ScanForBranches(repoPath, includeRemotes); err != nil {
			log.WarningLog.Printf("Error scanning for branches: %v", err)
		}
	}()

	return &BranchLoader{
		cache:          cache,
		repoPath:       repoPath,
		includeRemotes: includeRemotes,
	}
}

// AsyncLoad implements fuzzy.AsyncLoader interface
func (l *BranchLoader) AsyncLoad(query string) ([]fuzzy.SearchItem, error) {
	// Scan for branches if cache is empty or stale
	if l.cache.lastUpdate.IsZero() || time.Since(l.cache.lastUpdate) > 30*time.Second {
		if err := l.cache.ScanForBranches(l.repoPath, l.includeRemotes); err != nil {
			return nil, err
		}
	}

	// Get branches from cache
	var branches []BranchInfo
	if l.includeRemotes {
		branches = l.cache.GetAllBranches()
	} else {
		branches = l.cache.GetLocalBranches()
	}

	// Convert to search items
	items := make([]fuzzy.SearchItem, len(branches))
	for i, branch := range branches {
		items[i] = branch
	}

	return items, nil
}

// GetRepoPath returns the repository path
func (l *BranchLoader) GetRepoPath() string {
	return l.repoPath
}

// GetBranchByName returns a branch by name
func (l *BranchLoader) GetBranchByName(name string) (BranchInfo, bool) {
	return l.cache.GetBranch(name)
}
