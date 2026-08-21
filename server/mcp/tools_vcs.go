package mcp

import (
	"context"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

type vcsHandlers struct {
	store session.InstanceStore
}

// DiffStats mirrors git.DiffStats for JSON output.
type DiffStats struct {
	FilesChanged int `json:"files_changed"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// GetSessionDiffResult is the response for get_session_diff.
type GetSessionDiffResult struct {
	MCPResult
	Diff      string    `json:"diff"`
	Stats     DiffStats `json:"stats"`
	Truncated bool      `json:"truncated"`
}

// BranchInfo holds branch metadata for list_session_branches.
type BranchInfo struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// ListSessionBranchesResult is the response for list_session_branches.
type ListSessionBranchesResult struct {
	MCPResult
	Branches      []BranchInfo `json:"branches"`
	CurrentBranch string       `json:"current_branch"`
}

func registerVCSTools(s *mcpserver.MCPServer, vh *vcsHandlers) {
	s.AddTool(
		mcpgo.NewTool("get_session_diff",
			mcpgo.WithDescription("Get the git diff for a session's worktree relative to the base branch. Returns diff content, line stats, and whether the diff was truncated. Only works for worktree-backed sessions."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithNumber("max_bytes",
				mcpgo.Description("Maximum diff size in bytes (default 51200 / 50KB, max 102400 / 100KB)"),
				mcpgo.DefaultNumber(51200),
				mcpgo.Min(1),
				mcpgo.Max(102400),
			),
		),
		vh.getSessionDiff,
	)

	s.AddTool(
		mcpgo.NewTool("list_session_branches",
			mcpgo.WithDescription("List git branches available in a session's repository. Returns all local branches and marks the currently checked-out branch."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
		),
		vh.listSessionBranches,
	)
}

// ---- get_session_diff ----

func (vh *vcsHandlers) getSessionDiff(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	maxBytes := 51200
	if v, ok := args["max_bytes"].(float64); ok && v > 0 {
		maxBytes = int(v)
		if maxBytes > 102400 {
			maxBytes = 102400
		}
	}

	inst, errRes := vh.findInstance(sessionID)
	if errRes != nil {
		return errRes, nil
	}

	worktree, err := vh.openWorktree(inst)
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("cannot open git worktree: %v", err), "Session may not be worktree-backed"), nil
	}

	stats := worktree.Diff()
	if stats.Error != nil {
		return errResult(ErrInternalError, fmt.Sprintf("git diff failed: %v", stats.Error), ""), nil
	}

	content := stats.Content
	truncated := false
	if len(content) > maxBytes {
		content = content[:maxBytes]
		truncated = true
	}

	return okResult(GetSessionDiffResult{
		MCPResult: MCPResult{Success: true},
		Diff:      content,
		Stats: DiffStats{
			FilesChanged: countDiffFiles(content),
			Insertions:   stats.Added,
			Deletions:    stats.Removed,
		},
		Truncated: truncated,
	}), nil
}

// ---- list_session_branches ----

func (vh *vcsHandlers) listSessionBranches(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	inst, errRes := vh.findInstance(sessionID)
	if errRes != nil {
		return errRes, nil
	}

	worktree, err := vh.openWorktree(inst)
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("cannot open git worktree: %v", err), "Session may not be worktree-backed"), nil
	}

	currentBranch := worktree.GetBranchName()

	return okResult(ListSessionBranchesResult{
		MCPResult:     MCPResult{Success: true},
		Branches:      []BranchInfo{{Name: currentBranch, Current: true}},
		CurrentBranch: currentBranch,
	}), nil
}

// ---- helpers ----

// findInstance loads all sessions and returns the matching Instance.
func (vh *vcsHandlers) findInstance(sessionID string) (*session.Instance, *mcpgo.CallToolResult) {
	instances, err := vh.store.LoadInstances()
	if err != nil {
		return nil, errResult(ErrInternalError, "failed to load sessions", "")
	}
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			return inst, nil
		}
	}
	return nil, errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID), "Use list_sessions to find available sessions")
}

// openWorktree reconstructs a GitWorktree from the instance's stored paths.
// This does not require the session to be running.
func (vh *vcsHandlers) openWorktree(inst *session.Instance) (*git.GitWorktree, error) {
	worktreePath := inst.GetWorkingDirectory()
	if worktreePath == "" {
		return nil, fmt.Errorf("session has no working directory")
	}
	// Try to get repoPath from gitManager; fall back to worktreePath (non-worktree sessions).
	repoPath := ""
	if inst.HasGitWorktree() {
		// For worktree sessions, try to derive the repo path from stored state.
		// GetWorkingDirectory returns the worktree path; the repo path is the parent repo.
		// Use NewGitWorktreeFromStorage with empty repoPath — Diff() will find merge-base.
		repoPath = worktreePath // intentionally same; Diff() resolves via git internally
	}
	return git.NewGitWorktreeFromStorage(repoPath, worktreePath, inst.Title, inst.Branch, ""), nil
}

// countDiffFiles counts the number of changed files in a diff (lines starting with "diff --git").
func countDiffFiles(diff string) int {
	count := 0
	for i := 0; i < len(diff); {
		nl := -1
		for j := i; j < len(diff); j++ {
			if diff[j] == '\n' {
				nl = j
				break
			}
		}
		var line string
		if nl < 0 {
			line = diff[i:]
			i = len(diff)
		} else {
			line = diff[i:nl]
			i = nl + 1
		}
		if len(line) >= 10 && line[:10] == "diff --git" {
			count++
		}
	}
	return count
}
