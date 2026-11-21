package mcp

import (
	"context"
	"errors"

	githubpkg "github.com/tstapler/stapler-squad/github"
)

// ErrGitHubUnavailable is returned by every GitHub call when the composition
// root injected no GitHub port.
var ErrGitHubUnavailable = errors.New("GitHub access is not wired into this MCP server")

// GitHub is the slice of the github package the MCP tools call over the
// network. The composition root supplies it (WithGitHub); a fake lets the
// tools run without GitHub credentials or HTTP.
type GitHub interface {
	GetIssue(ctx context.Context, account githubpkg.AccountRef, repo githubpkg.RepoRef, number int) (*githubpkg.IssueResult, error)
	GetPR(ctx context.Context, account githubpkg.AccountRef, repo githubpkg.RepoRef, number int) (*githubpkg.PRResult, error)
	GetPRByNumber(ctx context.Context, repo githubpkg.RepoRef, number int) (*githubpkg.PRInfo, error)
	GetCommit(ctx context.Context, account githubpkg.AccountRef, repo githubpkg.RepoRef, sha string) (*githubpkg.CommitResult, error)
	// GetCurrentUserLogin resolves the authenticated login on host ("" is github.com).
	GetCurrentUserLogin(ctx context.Context, host string) (string, error)
}

// WithGitHub supplies the GitHub client behind import_github_issue,
// report_duplicate, report_pr_created and its PR cross-check. Without it those
// calls fail with ErrGitHubUnavailable.
func WithGitHub(g GitHub) CoreOption {
	return func(c *coreConfig) { c.github = g }
}

// GitHubFromOptions returns the GitHub port the options carry, or nil.
// It lets wiring tests prove each transport injects one.
func GitHubFromOptions(opts ...CoreOption) GitHub {
	return newCoreConfig(opts).github
}

// githubOrUnavailable maps an absent port to one that fails every call.
func githubOrUnavailable(g GitHub) GitHub {
	if g == nil {
		return unavailableGitHub{}
	}
	return g
}

type unavailableGitHub struct{}

func (unavailableGitHub) GetIssue(context.Context, githubpkg.AccountRef, githubpkg.RepoRef, int) (*githubpkg.IssueResult, error) {
	return nil, ErrGitHubUnavailable
}

func (unavailableGitHub) GetPR(context.Context, githubpkg.AccountRef, githubpkg.RepoRef, int) (*githubpkg.PRResult, error) {
	return nil, ErrGitHubUnavailable
}

func (unavailableGitHub) GetPRByNumber(context.Context, githubpkg.RepoRef, int) (*githubpkg.PRInfo, error) {
	return nil, ErrGitHubUnavailable
}

func (unavailableGitHub) GetCommit(context.Context, githubpkg.AccountRef, githubpkg.RepoRef, string) (*githubpkg.CommitResult, error) {
	return nil, ErrGitHubUnavailable
}

func (unavailableGitHub) GetCurrentUserLogin(context.Context, string) (string, error) {
	return "", ErrGitHubUnavailable
}
