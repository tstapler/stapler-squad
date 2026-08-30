package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type githubHandlers struct {
	cache *githubpkg.UserPRCache
	store session.InstanceStore
}

// GitHubPRSummary is the MCP representation of one open pull request.
type GitHubPRSummary struct {
	Owner             string    `json:"owner"`
	Repo              string    `json:"repo"`
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	URL               string    `json:"url"`
	Branch            string    `json:"branch"`
	BaseBranch        string    `json:"base_branch"`
	IsDraft           bool      `json:"is_draft"`
	CIStatus          string    `json:"ci_status"`
	ApprovedCount     int       `json:"approved_count"`
	ChangesReqCount   int       `json:"changes_req_count"`
	UpdatedAt         time.Time `json:"updated_at"`
	ExistingSessionID string    `json:"existing_session_id,omitempty"`
	WorktreePath      string    `json:"worktree_path,omitempty"`
}

// ListGitHubPRsResult is returned by list_github_prs.
type ListGitHubPRsResult struct {
	MCPResult
	PRs        []GitHubPRSummary `json:"prs"`
	TotalCount int               `json:"total_count"`
	Accounts   []string          `json:"accounts"`
}

// CreateSessionForPRResult is returned by create_session_for_pr.
type CreateSessionForPRResult struct {
	MCPResult
	Session *SessionDetail `json:"session,omitempty"`
}

func registerGitHubTools(s *mcpserver.MCPServer, gh *githubHandlers) {
	s.AddTool(
		mcpgo.NewTool("list_github_prs",
			mcpgo.WithDescription("List open GitHub pull requests authored by the authenticated user(s). Returns live data from the Unfinished Work page — includes PR metadata, CI status, review counts, and any existing Stapler Squad sessions associated with each PR. Use this to understand what work is in progress and which PRs need attention before creating new sessions."),
		),
		gh.listGitHubPRs,
	)

	s.AddTool(
		mcpgo.NewTool("create_session_for_pr",
			mcpgo.WithDescription("Create a new Stapler Squad worktree session for a GitHub pull request. Checks out the PR's branch in a git worktree so work is isolated. If an existing session is already associated with the PR, returns that session's ID instead of creating a duplicate. Auto-detects the local repo path from existing sessions if not provided."),
			mcpgo.WithString("owner",
				mcpgo.Description("GitHub owner (user or org) of the repository"),
				mcpgo.Required(),
			),
			mcpgo.WithString("repo",
				mcpgo.Description("Repository name"),
				mcpgo.Required(),
			),
			mcpgo.WithString("branch",
				mcpgo.Description("PR head branch name"),
				mcpgo.Required(),
			),
			mcpgo.WithNumber("pr_number",
				mcpgo.Description("Pull request number"),
				mcpgo.Required(),
			),
			mcpgo.WithString("path",
				mcpgo.Description("Absolute path to the local repository clone. Auto-detected from existing sessions when omitted."),
			),
			mcpgo.WithString("title",
				mcpgo.Description("Session title (default: owner/repo#number)"),
			),
			mcpgo.WithString("program",
				mcpgo.Description("Program to run: claude or aider (default: claude)"),
				mcpgo.Enum("claude", "aider"),
			),
		),
		gh.createSessionForPR,
	)
}

// ---- list_github_prs ----

func (gh *githubHandlers) listGitHubPRs(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	prs := gh.cache.GetAll()
	accounts := gh.cache.GetCachedLogins()

	summaries := make([]GitHubPRSummary, 0, len(prs))
	for _, pr := range prs {
		s := GitHubPRSummary{
			Owner:           pr.Owner,
			Repo:            pr.Repo,
			Number:          pr.Number,
			Title:           pr.Title,
			URL:             pr.URL,
			Branch:          pr.HeadRef,
			BaseBranch:      pr.BaseRef,
			IsDraft:         pr.IsDraft,
			CIStatus:        pr.CheckConclusion,
			ApprovedCount:   pr.ApprovedCount,
			ChangesReqCount: pr.ChangesReqCount,
			UpdatedAt:       pr.UpdatedAt,
			WorktreePath:    pr.LocalWorktreePath,
		}
		if len(pr.SessionIDs) > 0 {
			s.ExistingSessionID = pr.SessionIDs[0]
		}
		summaries = append(summaries, s)
	}

	return okResult(ListGitHubPRsResult{
		MCPResult:  MCPResult{Success: true},
		PRs:        summaries,
		TotalCount: len(summaries),
		Accounts:   accounts,
	}), nil
}

// ---- create_session_for_pr ----

func (gh *githubHandlers) createSessionForPR(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if !createSessionLimiter.allow("global") {
		return errResult(ErrRateLimitExceeded, "create_session rate limit exceeded (max 3 per minute)", "Wait before creating another session."), nil
	}

	args := req.GetArguments()
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	branch, _ := args["branch"].(string)
	prNumber := 0
	if v, ok := args["pr_number"].(float64); ok {
		prNumber = int(v)
	}
	repoPath, _ := args["path"].(string)
	title, _ := args["title"].(string)
	program, _ := args["program"].(string)

	if owner == "" || repo == "" || branch == "" || prNumber == 0 {
		return errResult(ErrInvalidArgument, "owner, repo, branch, and pr_number are required", ""), nil
	}

	// Check for an existing session for this PR.
	for _, pr := range gh.cache.GetAll() {
		if pr.Owner == owner && pr.Repo == repo && pr.Number == prNumber && len(pr.SessionIDs) > 0 {
			instances, loadErr := gh.store.LoadInstances()
			if loadErr == nil {
				for _, inst := range instances {
					if inst.Title == pr.SessionIDs[0] {
						detail := instanceToDetail(inst)
						return okResult(CreateSessionForPRResult{
							MCPResult: MCPResult{Success: true},
							Session:   &detail,
						}), nil
					}
				}
			}
		}
	}

	// Auto-detect repo path from existing sessions if not provided.
	if repoPath == "" {
		repoPath = detectRepoPath(gh.store, owner, repo)
	}
	if repoPath == "" {
		return errResult(ErrInvalidArgument,
			fmt.Sprintf("could not auto-detect local path for %s/%s; provide the 'path' argument", owner, repo),
			"Pass the absolute path to the local repository clone."), nil
	}

	if !filepath.IsAbs(repoPath) || strings.Contains(repoPath, "..") {
		return errResult(ErrInvalidPath, "path must be absolute and must not contain '..' components", ""), nil
	}
	if _, err := os.Stat(repoPath); err != nil {
		return errResult(ErrInvalidPath, fmt.Sprintf("path does not exist: %v", err), ""), nil
	}

	if title == "" {
		title = fmt.Sprintf("%s/%s#%d", owner, repo, prNumber)
	}
	if program == "" {
		program = "claude"
	}

	// Check for title collision.
	existing, err := gh.store.ListInstanceData()
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
	}
	for _, data := range existing {
		if data.Title == title {
			return errResult(ErrInvalidArgument, fmt.Sprintf("session with title %q already exists", title),
				"The PR may already have a session. Check list_github_prs for existing_session_id."), nil
		}
	}

	cfg := config.LoadConfig()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:       title,
		Path:        repoPath,
		Branch:      branch,
		Program:     program,
		SessionType: session.SessionTypeNewWorktree,
		Tags:        []string{"source:mcp", "pr:" + fmt.Sprintf("%s/%s#%d", owner, repo, prNumber)},
		// Backend consults the session-name override map (tymux-bundled-integration
		// Epic 4.4.1) so a canary override applies through this entry point too;
		// there's no per-request override field on this MCP tool's schema.
		Backend: session.ResolveSessionBackendForTitle(cfg, title, ""),
	})
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("create session: %v", err), ""), nil
	}

	if err := inst.Start(true); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("start session: %v", err), ""), nil
	}

	session.StartSessionDriver(inst, repoPath)

	if injErr := injectMCPConfig(inst.GetEffectiveRootDir()); injErr != nil {
		log.Warn("mcp MCP injection failed for PR session", "title", title, "err", injErr)
	}
	if err := services.InjectHooksConfig(inst.GetEffectiveRootDir(), inst.Title, nil); err != nil {
		log.Warn("mcp hook injection failed for PR session", "title", title, "err", err)
	}

	if err := gh.store.AddInstance(inst); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("save session: %v", err), ""), nil
	}

	detail := instanceToDetail(inst)
	return okResult(CreateSessionForPRResult{
		MCPResult: MCPResult{Success: true},
		Session:   &detail,
	}), nil
}

// PRVerification is the result of cross-checking a self-reported PR number
// against GitHub. It replaces a bare (bool, error) return so callers can see
// *why* a mismatch occurred (a different head branch vs. no PR at all) and
// make a policy decision — e.g. reportPRCreated's override path — instead of
// VerifyPRMatchesBranch having to bake that policy in itself.
//
// The implicit invariant Matched ⇒ Exists is enforced at construction by
// NewPRVerification — every PRVerification value, in production and in
// tests, must be built through NewPRVerification, never a bare struct
// literal, or the invariant has no enforcement point.
type PRVerification struct {
	Exists           bool
	Matched          bool
	ActualHeadBranch string
	State            string
	Author           string
}

// NewPRVerification is the sole constructor for PRVerification. It enforces
// Matched ⇒ Exists: a caller passing matched=true with exists=false has
// produced an illegal state (a PR can't match a branch if it doesn't exist).
// Rather than panic — which would take down the whole handler for what's
// almost certainly a code-level logic bug in this package rather than bad
// external input — the violation is forced to matched=false and logged
// loudly via log.ErrorLog().Printf so it's impossible to miss in logs/tests,
// while production keeps running.
//
// Author is not part of this invariant and is carried through unvalidated;
// it is consumed only by reportPRCreated's override-path author-match gate
// (tools_backlog.go's decideOverridePolicy) — this constructor makes no
// policy decision based on it.
func NewPRVerification(exists, matched bool, actualHeadBranch, state, author string) PRVerification {
	if matched && !exists {
		log.ErrorLog().Printf("NewPRVerification: illegal state matched=true with exists=false (actualHeadBranch=%q, state=%q, author=%q) — forcing matched=false", actualHeadBranch, state, author)
		matched = false
	}
	return PRVerification{
		Exists:           exists,
		Matched:          matched,
		ActualHeadBranch: actualHeadBranch,
		State:            state,
		Author:           author,
	}
}

// VerifyPRMatchesBranch confirms a PR number self-reported to
// report_pr_created (tools_backlog.go, Epic 3.1 of "PR Metadata Capture
// Fix" — project_plans/backlog-agent-communication) genuinely exists on
// GitHub, before that self-report is trusted and persisted. A hallucinated,
// stale, or mistyped PR reference would otherwise silently poison the item
// record — a class of bad data the mechanical pushAndCreatePR path
// (session/backlog_lifecycle.go) never has to guard against, since it only
// ever writes PR data it itself just created.
//
// Root-cause fix: this looks the PR up by its immutable number
// (githubpkg.GetPRByNumber) rather than by branch name
// (githubpkg.GetPRForBranch, the prior implementation). A branch-keyed
// lookup silently matches the wrong PR whenever the branch was reused,
// renamed, or (the confirmed real-world case) polluted by another session
// sharing the same worktree, so the caller opened the PR from a different,
// clean branch — the branch name the item is tracked under and the PR's
// actual head branch legitimately diverge, and a branch-keyed lookup cannot
// tell that apart from an unrelated PR.
//
// Because the lookup is now number-keyed, Matched == false with
// Exists == true is a distinct, *possible-to-accept* outcome — it's a real,
// existing PR for this repo whose head branch just doesn't match the item's
// tracked branch, rather than a fabricated PR number. Whether to accept that
// fallback case is a policy decision made by the caller (reportPRCreated's
// decideOverridePolicy, tools_backlog.go), never by this function — it
// stays a pure fact-reporter about what GitHub actually says.
//
// Returns:
//   - (NewPRVerification(false, false, "", "", ""), nil): no PR exists for
//     prNumber in owner/repo at all (githubpkg.ErrNoPR). Callers must NOT
//     persist on this result, with or without an override — re-asking
//     GitHub the same question will not change the answer.
//   - (NewPRVerification(true, ..., info.HeadRef, info.State, info.Author), nil):
//     the PR exists; Matched reflects whether info.HeadRef == expectedBranch.
//   - (PRVerification{}, err): the lookup itself failed (rate limit, network,
//     auth) — transient. Callers should surface a retryable error rather
//     than treating this as a confirmed mismatch or a confirmed non-existence.
func VerifyPRMatchesBranch(ctx context.Context, owner, repo string, prNumber int, expectedBranch string) (PRVerification, error) {
	info, err := githubpkg.GetPRByNumber(ctx, owner, repo, prNumber)
	if err != nil {
		if errors.Is(err, githubpkg.ErrNoPR) {
			return NewPRVerification(false, false, "", "", ""), nil
		}
		return PRVerification{}, err
	}
	return NewPRVerification(true, info.HeadRef == expectedBranch, info.HeadRef, info.State, info.Author), nil
}

// detectRepoPath looks for an existing session or worktree that belongs to owner/repo
// and returns the base repository path. Returns "" if none found.
func detectRepoPath(store session.InstanceStore, owner, repo string) string {
	instances, err := store.LoadInstances()
	if err != nil {
		return ""
	}
	target := strings.ToLower(owner + "/" + repo)
	for _, inst := range instances {
		if strings.Contains(strings.ToLower(inst.Path), strings.ToLower(repo)) {
			return inst.Path
		}
		if strings.Contains(strings.ToLower(inst.GetWorkingDirectory()), target) {
			dir := inst.GetWorkingDirectory()
			// Walk up to find the git root.
			for dir != "/" && dir != "." {
				if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
					return dir
				}
				dir = filepath.Dir(dir)
			}
		}
	}
	return ""
}
