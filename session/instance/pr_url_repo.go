package instance

import (
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
)

// PRURLRepo returns the repo a pull request URL points at. ok is false for
// anything that is not a PR URL on github.com or a configured enterprise host.
func PRURLRepo(prURL string) (github.RepoRef, bool) {
	parsed, err := ParseGitHubURLWithHosts(prURL, EnterpriseHostsForRemoteParsing())
	if err != nil || parsed.Type != GitHubRefTypePR {
		return github.RepoRef{}, false
	}
	host := parsed.Host
	if domain.IsGitHubCom(host) {
		host = "" // RepoRef spells github.com as ""
	}
	ref, err := github.NewRepoRefWithHost(parsed.Owner, parsed.Repo, host)
	return ref, err == nil
}

// ResolvePRRepo is the repo to query for a tracked PR: the one its URL names,
// else repoPath's origin remote. A workspace and its PR can be different repos.
func ResolvePRRepo(prURL, repoPath string) (github.RepoRef, error) {
	if ref, ok := PRURLRepo(prURL); ok {
		return ref, nil
	}
	return github.GetOwnerRepoFromRemote(repoPath, EnterpriseHostsForRemoteParsing())
}

// CrossRepoPR returns the PR's repo when it is known to differ from repoPath's
// origin. An unparseable URL or unreadable remote reports false.
func CrossRepoPR(prURL, repoPath string) (github.RepoRef, bool) {
	pr, ok := PRURLRepo(prURL)
	if !ok {
		return github.RepoRef{}, false
	}
	local, err := github.GetOwnerRepoFromRemote(repoPath, EnterpriseHostsForRemoteParsing())
	if err != nil || !local.IsValid() || local.PRKey(0) == pr.PRKey(0) {
		return github.RepoRef{}, false
	}
	return pr, true
}
