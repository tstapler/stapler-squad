package instance

import (
	"regexp"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
)

// GitHub's own name charset. Anything else (say "git@host:o") can make
// `gh --repo` read the value as a URL and send a token to that host.
var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// PRURLRepo returns the repo a pull request URL points at. ok is false for
// anything that is not a PR URL on github.com or a configured enterprise host.
func PRURLRepo(prURL string) (github.RepoRef, bool) {
	parsed, err := ParseGitHubURLWithHosts(prURL, EnterpriseHostsForRemoteParsing())
	if err != nil || parsed.Type != GitHubRefTypePR ||
		!repoNameRe.MatchString(parsed.Owner) || !repoNameRe.MatchString(parsed.Repo) {
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

// CrossRepoPR returns the PR's repo when it cannot be shown to be repoPath's
// origin, including an origin that does not parse. An unparseable PR URL or a
// repo with no readable origin reports false.
func CrossRepoPR(prURL, repoPath string) (github.RepoRef, bool) {
	pr, ok := PRURLRepo(prURL)
	if !ok {
		return github.RepoRef{}, false
	}
	local, err := github.GetOwnerRepoFromRemote(repoPath, EnterpriseHostsForRemoteParsing())
	if err != nil || (local.IsValid() && local.PRKey(0) == pr.PRKey(0)) {
		return github.RepoRef{}, false
	}
	return pr, true
}
