package server

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// prNumFromTitle extracts a PR number from a session title following the
// "pr-<number>-..." naming convention (e.g. "pr-1255-actions-spring-boot").
var prNumFromTitle = regexp.MustCompile(`(?i)^pr-(\d+)-`)

// annotateUserPRCache annotates the cached UserPR list now. The cache itself
// re-reads buildPRAnnotations (SetAnnotationSource) before every publish, so
// production code only needs this for an out-of-band refresh. Lives here (not
// in the github package) to avoid an import cycle: github → session → github.
func annotateUserPRCache(cache *githubpkg.UserPRCache, poller *session.PRStatusPoller, scanner *unfinished.Scanner) {
	cache.Annotate(buildPRAnnotations(poller, scanner))
}

// buildPRAnnotations collects the live sessions and worktrees a PR can link to.
func buildPRAnnotations(poller *session.PRStatusPoller, scanner *unfinished.Scanner) ([]githubpkg.PRAnnotationSession, []githubpkg.PRAnnotationWorktree) {
	ghHosts := config.LoadConfig().GetGitHubEnterpriseHosts()
	enterpriseHosts := make([]string, 0, len(ghHosts))
	for _, h := range ghHosts {
		enterpriseHosts = append(enterpriseHosts, h.Host)
	}
	return annotationSessions(poller, enterpriseHosts), annotationWorktrees(scanner, enterpriseHosts)
}

func annotationSessions(poller *session.PRStatusPoller, enterpriseHosts []string) []githubpkg.PRAnnotationSession {
	if poller == nil {
		return nil
	}
	var out []githubpkg.PRAnnotationSession
	for _, inst := range poller.GetInstances() {
		// Use Snapshot() — actor-based writes (SetGitHubPRNumber etc.) do not hold
		// mu, so direct field reads would race with concurrent poller updates.
		snap := inst.Snapshot()
		repoRef, prNumber := resolveSessionRepo(snap, enterpriseHosts)
		if !repoRef.IsValid() {
			continue
		}
		// Last resort: extract PR number from session title (e.g. "pr-1255-...").
		if prNumber == 0 {
			if m := prNumFromTitle.FindStringSubmatch(inst.Title); m != nil {
				prNumber, _ = strconv.Atoi(m[1])
			}
		}
		out = append(out, githubpkg.PRAnnotationSession{
			ID:           inst.Title,
			Branch:       snap.Branch,
			Repo:         repoRef,
			PRNumber:     prNumber,
			Status:       linkedStatusFor(snap.Status),
			LastActiveAt: snap.UpdatedAt,
		})
	}
	return out
}

// resolveSessionRepo resolves a full RepoRef (owner + repo) via a 3-tier fallback:
// 1. Direct from DB fields (new sessions written since schema migration).
// 2. Parse from stored PR URL (which may also supply the PR number).
// 3. Infer from git remote.
func resolveSessionRepo(snap *session.InstanceSnapshot, enterpriseHosts []string) (githubpkg.RepoRef, int) {
	prNumber := snap.GitHub.GitHubPRNumber
	var repoRef githubpkg.RepoRef
	if snap.GitHub.GitHubOwner != "" && snap.GitHub.GitHubRepo != "" {
		repoRef, _ = githubpkg.NewRepoRefWithHost(snap.GitHub.GitHubOwner, snap.GitHub.GitHubRepo, snap.GitHub.GitHubHost)
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
	return repoRef, prNumber
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
func linkedStatusFor(st session.Status) githubpkg.LinkedSessionStatus {
	switch st {
	case session.Paused, session.Hibernated:
		return githubpkg.LinkedSessionPaused
	case session.Stopped, session.Crashed, session.PermanentlyFailed, session.Failed:
		return githubpkg.LinkedSessionStopped
	case session.Creating, session.Active, session.Restoring:
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
