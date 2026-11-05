package github

import (
	"context"
	"fmt"
	"net/url"
)

// RepoProbe is the answer of a repository metadata GET.
type RepoProbe struct {
	Status int  // HTTP status of GET /repos/{owner}/{repo}
	SSO    bool // the response carried an X-GitHub-SSO header (token not authorized for the org)
}

// ProbeRepo asks GitHub whether the repository itself is visible to the host's
// token. It tells a deleted or renamed PR (repo 200, PR 404) from an invisible
// repository (repo 404/403). The request is host-aware, like every PR call.
func ProbeRepo(ctx context.Context, ref RepoRef) (RepoProbe, error) {
	ctx = WithGitHubCallSite(ctx, "repo.probe")
	path := fmt.Sprintf("repos/%s/%s", url.PathEscape(ref.Owner()), url.PathEscape(ref.Repo()))
	req, err := newGHRequestForHost(ctx, ref.Host(), path)
	if err != nil {
		return RepoProbe{}, fmt.Errorf("build repo probe request: %w", err)
	}
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return RepoProbe{}, fmt.Errorf("repo probe failed: %w", err)
	}
	defer resp.Body.Close()
	return RepoProbe{Status: resp.StatusCode, SSO: resp.Header.Get("X-GitHub-SSO") != ""}, nil
}
