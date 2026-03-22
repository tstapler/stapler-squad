package vcs

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// JJClient implements the VCS interface for Jujutsu
type JJClient struct {
	repoPath string
}

// NewJJClient creates a new JJ client for the given repository path
func NewJJClient(repoPath string) *JJClient {
	return &JJClient{
		repoPath: repoPath,
	}
}

// Type returns the VCS type
func (j *JJClient) Type() VCSType {
	return VCSTypeJJ
}

// RepoPath returns the repository path
func (j *JJClient) RepoPath() string {
	return j.repoPath
}

// run executes a jj command and returns the output
func (j *JJClient) run(args ...string) (string, error) {
	cmd := exec.Command("jj", args...)
	cmd.Dir = j.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.DebugLog.Printf("[JJ] Running: jj %s", strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("jj %s failed: %s", strings.Join(args, " "), errMsg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetStatus returns the working copy status
func (j *JJClient) GetStatus() (*WorkingCopyStatus, error) {
	output, err := j.run("status")
	if err != nil {
		return nil, err
	}

	status := &WorkingCopyStatus{}

	// Parse jj status output
	// Format varies, but typically shows modified/added/deleted files
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "M ") {
			status.ModifiedFiles++
		} else if strings.HasPrefix(line, "A ") {
			status.AddedFiles++
		} else if strings.HasPrefix(line, "D ") {
			status.DeletedFiles++
		} else if strings.HasPrefix(line, "C ") {
			status.ConflictedFiles++
		}
	}

	status.HasChanges = status.ModifiedFiles > 0 || status.AddedFiles > 0 ||
		status.DeletedFiles > 0 || status.ConflictedFiles > 0

	return status, nil
}

// HasUncommittedChanges checks if there are uncommitted changes
func (j *JJClient) HasUncommittedChanges() (bool, error) {
	// In JJ, the working copy is always a change, so we check if it's empty
	output, err := j.run("diff", "--stat")
	if err != nil {
		return false, err
	}

	// If diff output is not empty, there are changes
	return strings.TrimSpace(output) != "", nil
}

// GetCurrentRevision returns the current revision
func (j *JJClient) GetCurrentRevision() (*Revision, error) {
	// Get current revision info using template
	template := `{change_id}|{commit_id}|{description}|{author}|{author.timestamp()}`
	output, err := j.run("log", "-r", "@", "--no-graph", "-T", template)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(output, "|", 5)
	if len(parts) < 5 {
		return nil, fmt.Errorf("unexpected jj log output: %s", output)
	}

	rev := &Revision{
		ID:          strings.TrimSpace(parts[0]),
		ShortID:     truncateID(strings.TrimSpace(parts[0]), 8),
		Description: strings.TrimSpace(parts[2]),
		Author:      strings.TrimSpace(parts[3]),
		IsCurrent:   true,
	}

	// Parse timestamp if present
	if ts := strings.TrimSpace(parts[4]); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			rev.Timestamp = t
		}
	}

	// Get bookmarks pointing to this revision
	bookmarks, _ := j.getBookmarksForRevision("@")
	rev.Bookmarks = bookmarks

	return rev, nil
}

// GetCurrentBookmark returns the current bookmark name
func (j *JJClient) GetCurrentBookmark() (string, error) {
	bookmarks, err := j.getBookmarksForRevision("@")
	if err != nil {
		return "", err
	}
	if len(bookmarks) > 0 {
		return bookmarks[0], nil
	}
	return "", nil
}

// getBookmarksForRevision gets bookmarks pointing to a revision
func (j *JJClient) getBookmarksForRevision(rev string) ([]string, error) {
	output, err := j.run("bookmark", "list", "--all")
	if err != nil {
		// Bookmarks might not exist yet
		return nil, nil
	}

	// Get the revision's change ID
	changeID, err := j.run("log", "-r", rev, "--no-graph", "-T", "{change_id}")
	if err != nil {
		return nil, err
	}
	changeID = strings.TrimSpace(changeID)

	var bookmarks []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "bookmark_name: change_id commit_id"
		if strings.Contains(line, changeID[:12]) { // Use short ID for matching
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 0 {
				bookmarks = append(bookmarks, strings.TrimSpace(parts[0]))
			}
		}
	}

	return bookmarks, nil
}

// SwitchTo switches to the target revision or bookmark
func (j *JJClient) SwitchTo(target string, opts SwitchOptions) error {
	// 1. Handle uncommitted changes
	hasChanges, err := j.HasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if hasChanges {
		switch opts.ChangeStrategy {
		case KeepAsWIP:
			// Describe current change as WIP so it's not empty
			if err := j.DescribeWIP("WIP: uncommitted changes before workspace switch"); err != nil {
				log.WarningLog.Printf("[JJ] Failed to describe WIP: %v", err)
			}
		case BringAlong:
			// Just describe - the changes will be in the parent chain
			if err := j.DescribeWIP("WIP: changes to bring along"); err != nil {
				log.WarningLog.Printf("[JJ] Failed to describe WIP: %v", err)
			}
		case Abandon:
			if err := j.AbandonChanges(); err != nil {
				return fmt.Errorf("failed to abandon changes: %w", err)
			}
		}
	}

	// 2. Check if target exists
	targetExists := j.revisionExists(target)

	if !targetExists && opts.CreateIfMissing {
		// Create new bookmark
		base := opts.BaseRevision
		if base == "" {
			base = "@-" // Parent of current (after describing WIP, current is on top)
		}
		if err := j.CreateBookmark(target, base); err != nil {
			return fmt.Errorf("failed to create bookmark: %w", err)
		}
		// Now switch to it
		_, err := j.run("new", target)
		return err
	}

	if !targetExists {
		return fmt.Errorf("target revision or bookmark not found: %s", target)
	}

	// 3. Switch to target
	_, err = j.run("new", target)
	if err != nil {
		return fmt.Errorf("failed to switch to %s: %w", target, err)
	}

	log.InfoLog.Printf("[JJ] Switched to %s", target)
	return nil
}

// revisionExists checks if a revision or bookmark exists
func (j *JJClient) revisionExists(rev string) bool {
	_, err := j.run("log", "-r", rev, "--no-graph", "-T", "{change_id}")
	return err == nil
}

// CreateBookmark creates a new bookmark at the specified base
func (j *JJClient) CreateBookmark(name string, base string) error {
	if base == "" {
		base = "@"
	}

	// First, ensure we're at the base revision
	if base != "@" {
		if _, err := j.run("new", base); err != nil {
			return fmt.Errorf("failed to move to base %s: %w", base, err)
		}
	}

	// Create the bookmark at current revision
	_, err := j.run("bookmark", "create", name)
	if err != nil {
		return fmt.Errorf("failed to create bookmark %s: %w", name, err)
	}

	log.InfoLog.Printf("[JJ] Created bookmark %s at %s", name, base)
	return nil
}

// DescribeWIP sets the description for the current change
func (j *JJClient) DescribeWIP(message string) error {
	_, err := j.run("describe", "-m", message)
	return err
}

// AbandonChanges abandons the current change
func (j *JJClient) AbandonChanges() error {
	_, err := j.run("abandon", "@")
	return err
}

// ListBookmarks returns all bookmarks
func (j *JJClient) ListBookmarks() ([]Bookmark, error) {
	output, err := j.run("bookmark", "list", "--all")
	if err != nil {
		return nil, err
	}

	var bookmarks []Bookmark
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse bookmark line
		// Format: "bookmark_name: change_id commit_id" or "bookmark_name@origin: change_id commit_id"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		isRemote := strings.Contains(name, "@")

		// Extract revision ID from rest of line
		rest := strings.TrimSpace(parts[1])
		revParts := strings.Fields(rest)
		revID := ""
		if len(revParts) > 0 {
			revID = revParts[0]
		}

		bookmarks = append(bookmarks, Bookmark{
			Name:       name,
			RevisionID: revID,
			IsRemote:   isRemote,
		})
	}

	return bookmarks, nil
}

// ListRecentRevisions returns recent revisions
func (j *JJClient) ListRecentRevisions(limit int) ([]Revision, error) {
	// Use revset to get recent revisions
	template := `{change_id}|{commit_id}|{description}|{author}|{author.timestamp()}` + "\n"
	revset := fmt.Sprintf("ancestors(@, %d)", limit)

	output, err := j.run("log", "-r", revset, "--no-graph", "-T", template)
	if err != nil {
		return nil, err
	}

	var revisions []Revision
	scanner := bufio.NewScanner(strings.NewReader(output))
	isFirst := true
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		rev := Revision{
			ID:          strings.TrimSpace(parts[0]),
			ShortID:     truncateID(strings.TrimSpace(parts[0]), 8),
			Description: strings.TrimSpace(parts[2]),
			Author:      strings.TrimSpace(parts[3]),
			IsCurrent:   isFirst,
		}
		isFirst = false

		if ts := strings.TrimSpace(parts[4]); ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				rev.Timestamp = t
			}
		}

		revisions = append(revisions, rev)
	}

	return revisions, nil
}

// CreateWorktree creates a new JJ workspace (worktree equivalent)
func (j *JJClient) CreateWorktree(path string, name string) error {
	_, err := j.run("workspace", "add", "--name", name, path)
	if err != nil {
		return fmt.Errorf("failed to create workspace at %s: %w", path, err)
	}

	log.InfoLog.Printf("[JJ] Created workspace %s at %s", name, path)
	return nil
}

// ListWorktrees returns all JJ workspaces
func (j *JJClient) ListWorktrees() ([]Worktree, error) {
	output, err := j.run("workspace", "list")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse workspace line
		// Format: "workspace_name: path (current)" or "workspace_name: path"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		rest := strings.TrimSpace(parts[1])
		isCurrent := strings.Contains(rest, "(current)")
		path := strings.TrimSuffix(strings.TrimSpace(rest), " (current)")

		worktrees = append(worktrees, Worktree{
			Name:      name,
			Path:      path,
			IsCurrent: isCurrent,
		})
	}

	return worktrees, nil
}

// truncateID truncates an ID to the specified length
func truncateID(id string, length int) string {
	if len(id) <= length {
		return id
	}
	return id[:length]
}

// GetJJVersion returns the installed JJ version
func GetJJVersion() (string, error) {
	cmd := exec.Command("jj", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// parseJJTimestamp parses JJ timestamp formats
func parseJJTimestamp(ts string) time.Time {
	// Try various formats
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, ts); err == nil {
			return t
		}
	}

	// Try Unix timestamp
	if i, err := strconv.ParseInt(ts, 10, 64); err == nil {
		return time.Unix(i, 0)
	}

	return time.Time{}
}
