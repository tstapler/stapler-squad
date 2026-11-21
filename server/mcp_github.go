package server

import (
	"context"

	githubpkg "github.com/tstapler/stapler-squad/github"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
)

// The production client must keep satisfying the MCP tools' port.
var _ servermcp.GitHub = mcpGitHub{}

// NewMCPGitHub is the production GitHub port the HTTP and stdio MCP cores
// share. It wraps the github package's request-building functions, so every
// call keeps their ETag and rate-limit handling. It does not read the
// UserPRCache: the cache holds only open PRs without the author the
// report_pr_created cross-check needs, and its cached login may belong to a
// different account than the default token the identity check must use.
func NewMCPGitHub() servermcp.GitHub { return mcpGitHub{} }

type mcpGitHub struct{}

func (mcpGitHub) GetIssue(ctx context.Context, account githubpkg.AccountRef, repo githubpkg.RepoRef, number int) (*githubpkg.IssueResult, error) {
	return githubpkg.GetIssue(ctx, account, repo, number)
}

func (mcpGitHub) GetPR(ctx context.Context, account githubpkg.AccountRef, repo githubpkg.RepoRef, number int) (*githubpkg.PRResult, error) {
	return githubpkg.GetPR(ctx, account, repo, number)
}

func (mcpGitHub) GetPRByNumber(ctx context.Context, repo githubpkg.RepoRef, number int) (*githubpkg.PRInfo, error) {
	return githubpkg.GetPRByNumber(ctx, repo, number)
}

func (mcpGitHub) GetCommit(ctx context.Context, account githubpkg.AccountRef, repo githubpkg.RepoRef, sha string) (*githubpkg.CommitResult, error) {
	return githubpkg.GetCommit(ctx, account, repo, sha)
}

func (mcpGitHub) GetCurrentUserLogin(ctx context.Context, host string) (string, error) {
	return githubpkg.GetCurrentUserLogin(ctx, host)
}
