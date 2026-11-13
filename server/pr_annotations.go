package server

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/prpoller"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// prNumFromTitle extracts a PR number from a session title following the
// "pr-<number>-..." naming convention (e.g. "pr-1255-actions-spring-boot").
var prNumFromTitle = regexp.MustCompile(`(?i)^pr-(\d+)-`)

// buildPRAnnotations collects the live sessions and worktrees a PR can link to.
// The cache re-reads it (SetAnnotationSource) before every publish. Lives here,
// not in the github package, to avoid an import cycle: github → session → github.
func buildPRAnnotations(poller *prpoller.PRStatusPoller, scanner *unfinished.Scanner) ([]githubpkg.PRAnnotationSession, []githubpkg.PRAnnotationWorktree) {
	ghHosts := config.LoadConfig().GetGitHubEnterpriseHosts()
	enterpriseHosts := make([]string, 0, len(ghHosts))
	for _, h := range ghHosts {
		enterpriseHosts = append(enterpriseHosts, h.Host)
	}
	return annotationSessions(poller, enterpriseHosts), annotationWorktrees(scanner, enterpriseHosts)
}

// annotationsForSnapshot emits one annotation per attached PR so every PR a
// session holds links back to it; a session with no links keeps the single
// legacy entry. Reads the snapshot, never raw fields: actor writes do not hold mu.
func annotationsForSnapshot(title string, snap *session.InstanceSnapshot, enterpriseHosts []string) []githubpkg.PRAnnotationSession {
	if len(snap.GitHub.PRs) > 0 {
		return annotationsForPRs(title, snap)
	}
	repoRef, prNumber, hostUnrecorded := resolveSessionRepo(snap, enterpriseHosts)
	if !repoRef.IsValid() {
		return nil
	}
	// Last resort: extract PR number from session title (e.g. "pr-1255-...").
	if prNumber == 0 {
		if m := prNumFromTitle.FindStringSubmatch(title); m != nil {
			prNumber, _ = strconv.Atoi(m[1])
		}
	}
	return []githubpkg.PRAnnotationSession{{
		ID: title, Branch: snap.Branch, Repo: repoRef, PRNumber: prNumber,
		Status: linkedStatusFor(snap.Status), LastActiveAt: snap.UpdatedAt, HostUnrecorded: hostUnrecorded,
	}}
}

func annotationsForPRs(title string, snap *session.InstanceSnapshot) []githubpkg.PRAnnotationSession {
	out := make([]githubpkg.PRAnnotationSession, 0, len(snap.GitHub.PRs))
	for _, v := range snap.GitHub.PRs {
		host := v.Ref.Host()
		if domain.IsGitHubCom(host) {
			host = ""
		}
		repoRef, err := githubpkg.NewRepoRefWithHost(v.Ref.Owner(), v.Ref.Repo(), host)
		if err != nil {
			continue
		}
		out = append(out, githubpkg.PRAnnotationSession{
			ID: title, Branch: snap.Branch, Repo: repoRef, PRNumber: v.Ref.Number(),
			Status: linkedStatusFor(snap.Status), LastActiveAt: snap.UpdatedAt,
		})
	}
	return out
}

func annotationSessions(poller *prpoller.PRStatusPoller, enterpriseHosts []string) []githubpkg.PRAnnotationSession {
	if poller == nil {
		return nil
	}
	var out []githubpkg.PRAnnotationSession
	for _, inst := range poller.GetInstances() {
		out = append(out, annotationsForSnapshot(inst.Title, inst.Snapshot(), enterpriseHosts)...)
	}
	return out
}

// resolveSessionRepo resolves a full RepoRef (owner + repo) via a 3-tier fallback:
// 1. Direct from DB fields (new sessions written since schema migration).
// 2. Parse from stored PR URL (which may also supply the PR number).
// 3. Infer from git remote.
//
// hostUnrecorded is true only when tier 1 stored no host and neither the git
// remote nor the stored PR URL can confirm one: an empty stored host is also
// what github.com sessions carry, so those are the only ways to tell a legacy
// GHE session apart. Residual: a github.com session with no resolvable remote
// (e.g. a paused session whose worktree is gone) and no PR URL stays
// unrecorded and may legacy-link a same-owner, same-branch GHE PR; legacy
// links are flagged LegacyFallback and never grant nudge linkage.
func resolveSessionRepo(snap *session.InstanceSnapshot, enterpriseHosts []string) (_ githubpkg.RepoRef, prNumber int, hostUnrecorded bool) {
	prNumber = snap.GitHub.GitHubPRNumber
	var repoRef githubpkg.RepoRef
	if snap.GitHub.GitHubOwner != "" && snap.GitHub.GitHubRepo != "" {
		repoRef, _ = githubpkg.NewRepoRefWithHost(snap.GitHub.GitHubOwner, snap.GitHub.GitHubRepo, snap.GitHub.GitHubHost)
		if snap.GitHub.GitHubHost == "" {
			repoRef, hostUnrecorded = confirmLegacyHost(snap, repoRef, enterpriseHosts)
		}
	}
	if !repoRef.IsValid() && snap.GitHub.GitHubPRURL != "" {
		if parsed, err := session.ParseGitHubURLWithHosts(snap.GitHub.GitHubPRURL, enterpriseHosts); err == nil {
			repoRef, _ = githubpkg.NewRepoRefWithHost(parsed.Owner, parsed.Repo, parsed.Host)
			if prNumber == 0 {
				prNumber = parsed.PRNumber
			}
		}
	}
	if !repoRef.IsValid() && snap.Path != "" {
		repoRef, _ = githubpkg.GetOwnerRepoFromRemote(snap.Path, enterpriseHosts)
	}
	return repoRef, prNumber, hostUnrecorded && repoRef.IsValid()
}

// confirmLegacyHost resolves the host of a session that stored owner/repo but
// no host. A remote naming the same repo confirms it. When the remote differs
// (fork or rename) it says nothing reliable about where the stored PR lives, so
// only the stored PR URL's host is trusted; without one the session stays
// unrecorded rather than being stamped with the remote's host.
func confirmLegacyHost(snap *session.InstanceSnapshot, stored githubpkg.RepoRef, enterpriseHosts []string) (_ githubpkg.RepoRef, hostUnrecorded bool) {
	if remote, ok := remoteRepo(snap.Path, enterpriseHosts); ok &&
		strings.EqualFold(remote.Owner(), stored.Owner()) && strings.EqualFold(remote.Repo(), stored.Repo()) {
		return remote, false
	}
	if snap.GitHub.GitHubPRURL != "" {
		if parsed, err := session.ParseGitHubURLWithHosts(snap.GitHub.GitHubPRURL, enterpriseHosts); err == nil {
			withHost, _ := githubpkg.NewRepoRefWithHost(stored.Owner(), stored.Repo(), parsed.Host)
			return withHost, false
		}
	}
	return stored, true
}

func remoteRepo(path string, enterpriseHosts []string) (githubpkg.RepoRef, bool) {
	if path == "" {
		return githubpkg.RepoRef{}, false
	}
	ref, err := githubpkg.GetOwnerRepoFromRemote(path, enterpriseHosts)
	return ref, err == nil && ref.IsValid()
}

func annotationWorktrees(scanner *unfinished.Scanner, enterpriseHosts []string) []githubpkg.PRAnnotationWorktree {
	if scanner == nil {
		return nil
	}
	var out []githubpkg.PRAnnotationWorktree
	for _, r := range scanner.GetAllResults() {
		repoRef, err := githubpkg.GetOwnerRepoFromRemote(r.RepoPath, enterpriseHosts)
		if err != nil || !repoRef.IsValid() || r.Branch == "" {
			continue
		}
		out = append(out, githubpkg.PRAnnotationWorktree{Branch: r.Branch, Repo: repoRef, WorktreePath: r.WorktreePath})
	}
	return out
}

// linkedStatusFor maps a session lifecycle status to the coarse status shown
// on a PR's linked-session list. LastActiveAt uses InstanceSnapshot.UpdatedAt:
// last-meaningful-output time is not published in the snapshot, and reading
// it would need a lock outside the lock-free path.
func linkedStatusFor(st domain.Status) githubpkg.LinkedSessionStatus {
	switch st {
	case domain.Paused, domain.Hibernated:
		return githubpkg.LinkedSessionPaused
	case domain.Stopped, domain.Crashed, domain.PermanentlyFailed, domain.Failed:
		return githubpkg.LinkedSessionStopped
	case domain.Creating, domain.Active, domain.Restoring:
		return githubpkg.LinkedSessionRunning
	default:
		return githubpkg.LinkedSessionUnknown
	}
}

// prPollDegradedEnv selects ADR-001's degraded poll (no reviewThreads) when
// "1" or "true"; the default is the full widened poll.
const prPollDegradedEnv = "STAPLER_SQUAD_PR_POLL_DEGRADED"

func userPRCacheConfigFromEnv() githubpkg.UserPRCacheConfig {
	cfg := githubpkg.DefaultUserPRCacheConfig()
	switch strings.ToLower(os.Getenv(prPollDegradedEnv)) {
	case "1", "true":
		cfg.DegradedDetails = true
	}
	return cfg
}
