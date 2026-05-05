// Package services provides the server-side service implementations.
package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"connectrpc.com/connect"
)

const (
	pathCompletionDefaultMax = 100
	pathCompletionHardMax    = 500
	pathCompletionTimeout    = 2 * time.Second
	dirCacheMaxSize          = 256
	dirCacheTTL              = 60 * time.Second
)

// PathCompletionService handles RPC methods for filesystem path completion.
// It caches directory listings in a DirCache to avoid repeated os.ReadDir calls.
type PathCompletionService struct {
	cache *DirCache
}

// NewPathCompletionService creates a PathCompletionService with a DirCache.
func NewPathCompletionService() *PathCompletionService {
	return &PathCompletionService{
		cache: NewDirCache(dirCacheMaxSize, dirCacheTTL),
	}
}

// ListPathCompletions returns filesystem entries matching the given path prefix.
func (p *PathCompletionService) ListPathCompletions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {
	pathPrefix := req.Msg.GetPathPrefix()
	maxResults := int(req.Msg.GetMaxResults())
	directoriesOnly := req.Msg.GetDirectoriesOnly()

	if maxResults <= 0 {
		maxResults = pathCompletionDefaultMax
	}
	if maxResults > pathCompletionHardMax {
		maxResults = pathCompletionHardMax
	}

	// Expand ~ before any path splitting.
	expanded, err := expandTilde(pathPrefix)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Determine whether the exact expanded path exists as a directory.
	// Intentionally directory-only: clients use this for path completion,
	// where only directories are valid navigation targets.
	pathExists := false
	if info, statErr := os.Stat(expanded); statErr == nil {
		pathExists = info.IsDir()
	}

	// Split into base directory and filter prefix.
	baseDir, filterPrefix := splitPathPrefix(expanded)

	// Check whether the base directory exists.
	baseDirInfo, baseDirStatErr := os.Stat(baseDir)
	baseDirExists := baseDirStatErr == nil && baseDirInfo.IsDir()

	if !baseDirExists {
		return connect.NewResponse(&sessionv1.ListPathCompletionsResponse{
			BaseDir:       baseDir,
			BaseDirExists: false,
			PathExists:    pathExists,
		}), nil
	}

	// Capture directory mtime from the already-computed baseDirInfo for cache keying.
	dirMtime := baseDirInfo.ModTime()

	// Check the cache before hitting the filesystem.
	var dirEntries []os.DirEntry
	if cached, ok := p.cache.Get(baseDir); ok {
		dirEntries = cached
	} else {
		// Cache miss: read the directory with a timeout to guard against slow/network filesystems.
		type dirResult struct {
			entries []os.DirEntry
			err     error
		}
		resultCh := make(chan dirResult, 1)

		listCtx, cancel := context.WithTimeout(ctx, pathCompletionTimeout)
		defer cancel()

		go func() {
			entries, readErr := os.ReadDir(baseDir)
			resultCh <- dirResult{entries: entries, err: readErr}
		}()

		select {
		case result := <-resultCh:
			if result.err != nil {
				if os.IsPermission(result.err) {
					return nil, connect.NewError(connect.CodePermissionDenied, result.err)
				}
				if os.IsNotExist(result.err) {
					return connect.NewResponse(&sessionv1.ListPathCompletionsResponse{
						BaseDir:       baseDir,
						BaseDirExists: false,
						PathExists:    pathExists,
					}), nil
				}
				return nil, connect.NewError(connect.CodeInternal, result.err)
			}
			dirEntries = result.entries
			p.cache.Put(baseDir, dirEntries, dirMtime)
		case <-listCtx.Done():
			return nil, connect.NewError(connect.CodeDeadlineExceeded, fmt.Errorf("directory listing timed out"))
		}
	}

	// Filter entries and build the response.
	showHidden := strings.HasPrefix(filterPrefix, ".")
	var protoEntries []*sessionv1.PathEntry
	truncated := false

	for _, entry := range dirEntries {
		name := entry.Name()

		// Skip hidden files unless the filter itself starts with ".".
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		// Skip entries that don't match the filter prefix.
		if filterPrefix != "" && !strings.HasPrefix(name, filterPrefix) {
			continue
		}

		// Determine whether the entry is a directory, following symlinks.
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			fullPath := filepath.Join(baseDir, name)
			if info, statErr := os.Stat(fullPath); statErr == nil {
				isDir = info.IsDir()
			} else {
				// Broken symlink: skip.
				continue
			}
		}

		if directoriesOnly && !isDir {
			continue
		}

		if len(protoEntries) >= maxResults {
			truncated = true
			break
		}

		protoEntries = append(protoEntries, &sessionv1.PathEntry{
			Path:        filepath.Join(baseDir, name),
			Name:        name,
			IsDirectory: isDir,
		})
	}

	return connect.NewResponse(&sessionv1.ListPathCompletionsResponse{
		Entries:       protoEntries,
		BaseDir:       baseDir,
		Truncated:     truncated,
		BaseDirExists: true,
		PathExists:    pathExists,
	}), nil
}

// ListWorktrees returns the git worktrees for a given repository path.
func (p *PathCompletionService) ListWorktrees(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorktreesRequest],
) (*connect.Response[sessionv1.ListWorktreesResponse], error) {
	repoPath := req.Msg.GetRepoPath()

	expanded, err := expandTilde(repoPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	expanded = strings.TrimRight(expanded, "/")

	cmd := safeexec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = expanded
	output, err := cmd.Output()
	if err != nil {
		// Not a git repo or git not available — return empty list gracefully.
		return connect.NewResponse(&sessionv1.ListWorktreesResponse{}), nil
	}

	worktrees := parseWorktreeList(string(output))
	return connect.NewResponse(&sessionv1.ListWorktreesResponse{Worktrees: worktrees}), nil
}

// parseWorktreeList parses the output of 'git worktree list --porcelain'.
func parseWorktreeList(porcelainOutput string) []*sessionv1.WorktreeEntry {
	var result []*sessionv1.WorktreeEntry
	var current *sessionv1.WorktreeEntry
	isFirst := true

	for _, line := range strings.Split(strings.TrimSpace(porcelainOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			current = nil
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current = &sessionv1.WorktreeEntry{
				Path:   strings.TrimPrefix(line, "worktree "),
				IsMain: isFirst,
			}
			isFirst = false
			result = append(result, current)
		} else if current != nil && strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	return result
}

// expandTilde replaces a leading "~" with the user's home directory.
// A lone "~" is treated as "~/" so that splitPathPrefix lists home-directory contents.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not expand ~: %w", err)
	}
	if path == "~" {
		// Treat lone "~" as "~/" so the caller sees home dir entries.
		return home + "/", nil
	}
	if strings.HasPrefix(path, "~/") {
		// Preserve any trailing slash in the user's input.
		return home + path[1:], nil
	}
	// ~username form not supported server-side; return as-is.
	return path, nil
}

// splitPathPrefix splits an expanded path into a base directory and a filter prefix.
// The base directory is everything up to the last slash; the filter prefix is the
// trailing partial segment to match against directory entries.
//
// Examples:
//
//	"/home/user/proj" → ("/home/user", "proj")
//	"/home/user/"     → ("/home/user", "")
//	"/"               → ("/", "")
//	"proj"            → (".", "proj")
//	""                → (".", "")
func splitPathPrefix(expanded string) (baseDir, filterPrefix string) {
	if expanded == "" {
		return ".", ""
	}
	dir := filepath.Dir(expanded)
	base := filepath.Base(expanded)
	// If expanded ends with a separator, Dir strips it and Base returns the last element.
	// Detect trailing separator to treat as "list this directory".
	if expanded[len(expanded)-1] == filepath.Separator || expanded[len(expanded)-1] == '/' {
		return filepath.Clean(expanded), ""
	}
	return dir, base
}
