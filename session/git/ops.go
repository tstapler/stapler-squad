package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// FetchBranch fetches a specific branch from the origin remote.
func FetchBranch(repoPath, branchName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "origin", branchName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to fetch branch: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to fetch branch: %w", err)
	}
	return nil
}

// CandidateDefaultBranches are tried, in order, by ResolveDefaultBranchSHA when the
// caller doesn't know the repo's actual default branch name. Mirrors the candidate list
// session/unfinished's GoGitVCSReader.ResolveDefaultBranch already uses. "main" stays
// first so the common case costs exactly one fetch. Exported so other packages needing
// the same candidate list (session.RecoverBaseCommitSHA, GitWorktree.initBaseCommitSHA)
// share one definition instead of re-declaring the literal.
var CandidateDefaultBranches = []string{"main", "master", "develop", "trunk"} //nolint:gochecknoglobals

// ResolveDefaultBranchSHA finds repoPath's real default branch and returns its freshly-
// fetched origin tip SHA, trying CandidateDefaultBranches in order via ResolveOriginBranchSHA
// until one exists on origin. Exists because a repo's default branch can be named anything
// (this repo's own sibling dotfiles project uses "master", not "main") — a caller that
// hardcodes "main" gets a fetch failure on every single call against such a repo, which used
// to silently fall through to branching from the caller's ambient HEAD instead. Querying each
// candidate by fetch (rather than inspecting locally-cached remote-tracking refs) works even
// when the repo has never been fetched before, since a nonexistent branch fails fast at the
// fetch step itself.
func ResolveDefaultBranchSHA(repoPath string) (branch, sha string, err error) {
	var errs []error
	for _, candidate := range CandidateDefaultBranches {
		if candidateSHA, fetchErr := ResolveOriginBranchSHA(repoPath, candidate); fetchErr == nil {
			return candidate, candidateSHA, nil
		} else {
			errs = append(errs, fetchErr)
		}
	}
	return "", "", fmt.Errorf("no candidate default branch (%v) exists on origin: %w", CandidateDefaultBranches, errors.Join(errs...))
}

// ResolveOriginBranchSHA fetches mainBranch from origin into repoPath and returns the
// resulting origin/mainBranch tip commit SHA. Unlike a bare `rev-parse HEAD` against
// repoPath's own checkout, this always fetches first, so the returned SHA reflects
// origin's true current tip rather than whatever repoPath happened to have checked
// out last — the gap that let a new backlog work session's worktree branch from a
// days-stale local checkout instead of the real main tip (see setupNewWorktree's
// "branch from current HEAD" comment, and CreateBacklogWorktree's use of this func).
func ResolveOriginBranchSHA(repoPath, mainBranch string) (string, error) {
	if err := FetchBranch(repoPath, mainBranch); err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", mainBranch, err)
	}
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}
	ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", mainBranch), true)
	if err != nil {
		return "", fmt.Errorf("failed to resolve origin/%s: %w", mainBranch, err)
	}
	return ref.Hash().String(), nil
}

// ResolveLocalBranchSHA returns the tip commit SHA of the local branch (refs/heads/<branchName>)
// in repoPath, without fetching. Used as a same-branch-only fallback when ResolveOriginBranchSHA's
// fetch fails (offline, no origin remote) — still guarantees the caller branches from branchName
// itself, just not guaranteed fresh, unlike falling back to repoPath's ambient HEAD which could be
// checked out to any branch a concurrent process last left it on.
func ResolveLocalBranchSHA(repoPath, branchName string) (string, error) {
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return "", fmt.Errorf("failed to resolve local branch %s: %w", branchName, err)
	}
	return ref.Hash().String(), nil
}

// ResolveDefaultLocalBranchSHA is ResolveDefaultBranchSHA's fully-offline counterpart:
// tries CandidateDefaultBranches against repoPath's own local refs (no fetch), for use
// when every origin candidate fetch has already failed.
func ResolveDefaultLocalBranchSHA(repoPath string) (branch, sha string, err error) {
	var errs []error
	for _, candidate := range CandidateDefaultBranches {
		if candidateSHA, localErr := ResolveLocalBranchSHA(repoPath, candidate); localErr == nil {
			return candidate, candidateSHA, nil
		} else {
			errs = append(errs, localErr)
		}
	}
	return "", "", fmt.Errorf("no candidate default branch (%v) exists locally: %w", CandidateDefaultBranches, errors.Join(errs...))
}

// ResolveWorktreeBaseCommit resolves the commit new backlog-item worktrees
// should fork from: origin's default branch tip, falling back to a local
// default branch when the origin fetch fails (offline, no origin remote).
// baseSHA == "" (with err == nil) means repoPath has no commits yet
// (IsUnbornRepo) — the only case it's safe for a caller to fall back to
// branching from ambient HEAD, since no other branch can exist to
// accidentally fork from instead. Any other resolution failure is returned
// as an error rather than silently falling back to ambient HEAD, which is
// what let new work fork from whatever branch repoPath's checkout happened
// to be sitting on (e.g. an agent's own in-progress feature branch) instead
// of main. Shared by session.CreateBacklogWorktree and TriggerTriage's
// isolated triage worktree (server/services/backlog_service_triage.go).
func ResolveWorktreeBaseCommit(repoPath string) (defaultBranch, baseSHA string, err error) {
	defaultBranch, baseSHA, fetchErr := ResolveDefaultBranchSHA(repoPath)
	if fetchErr == nil {
		return defaultBranch, baseSHA, nil
	}
	defaultBranch, baseSHA, localErr := ResolveDefaultLocalBranchSHA(repoPath)
	if localErr == nil {
		return defaultBranch, baseSHA, nil
	}
	if IsUnbornRepo(repoPath) {
		return "", "", nil
	}
	return "", "", fmt.Errorf("resolve default branch (origin fetch failed: %w, local lookup failed: %v)", fetchErr, localErr)
}

// ResolveExplicitBranchSHA resolves branchName's tip commit SHA for a caller
// that deliberately opted out of the default-branch behavior (BacklogItem.
// BaseBranch) — origin's copy first (fresh fetch), falling back to a local
// branch of the same name when the origin fetch fails (offline, no origin
// remote). Unlike ResolveWorktreeBaseCommit, there is no unborn-repo/ambient-
// HEAD fallback here: an explicitly requested branch that doesn't exist
// anywhere is always a hard error, never silently substituted.
func ResolveExplicitBranchSHA(repoPath, branchName string) (string, error) {
	if sha, err := ResolveOriginBranchSHA(repoPath, branchName); err == nil {
		return sha, nil
	} else if sha, localErr := ResolveLocalBranchSHA(repoPath, branchName); localErr == nil {
		return sha, nil
	} else {
		return "", fmt.Errorf("resolve branch %q (origin fetch failed: %w, local lookup failed: %v)", branchName, err, localErr)
	}
}

// IsUnbornRepo reports whether repoPath is a git repository with zero commits (HEAD
// points at a branch ref that doesn't exist yet, e.g. right after `git init`). Distinct
// from "no candidate default branch found": that can also mean a repo with real commit
// history whose default branch just isn't named main/master/develop/trunk, which is not
// safe to fall back to ambient HEAD for (see CreateBacklogWorktree). An unborn repo has
// no commit history at all, so there is nothing to misattribute either way.
func IsUnbornRepo(repoPath string) bool {
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return false
	}
	_, err = repo.Head()
	return errors.Is(err, plumbing.ErrReferenceNotFound)
}

// IsCommitOnMain reports whether sha has actually landed on mainBranch — either the
// local branch (a commit merged directly to main without ever going through a PR) or
// origin's copy (a PR merged remotely on GitHub that hasn't been pulled locally yet).
// Approval (a passing review verdict) and shipping are different questions; this
// answers only the second one, and does so by checking ancestry rather than trusting
// any cached "PR merged" flag, since that flag can be stale, absent (no PR was ever
// opened), or simply wrong for a manually-merged branch.
//
// Uses go-git rather than shelling out (repo convention — see
// the `prefer-go-git-over-subshells` skill). The origin fetch is best-effort: a
// failure (offline, no such remote, nothing new) does not fail the whole check, since
// the local-main check alone still answers the "merged directly to main locally" case.
func IsCommitOnMain(repoPath, mainBranch, sha string) (bool, error) {
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return false, fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}

	refSpec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", mainBranch, mainBranch))
	if fetchErr := repo.Fetch(&git.FetchOptions{RemoteName: "origin", RefSpecs: []config.RefSpec{refSpec}}); fetchErr != nil && !errors.Is(fetchErr, git.NoErrAlreadyUpToDate) {
		log.Warn("IsCommitOnMain: fetch origin failed, falling back to local main only", "repoPath", repoPath, "err", fetchErr)
	}

	shaCommit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return false, fmt.Errorf("failed to resolve commit %s in %s: %w", sha, repoPath, err)
	}

	onLocal, localErr := isAncestorOfRef(repo, shaCommit, plumbing.NewBranchReferenceName(mainBranch))
	if localErr != nil {
		log.Warn("IsCommitOnMain: local main ref check failed, falling back to origin/main", "repoPath", repoPath, "err", localErr)
	} else if onLocal {
		return true, nil
	}

	return isAncestorOfRef(repo, shaCommit, plumbing.NewRemoteReferenceName("origin", mainBranch))
}

// isAncestorOfRef resolves ref to its commit and reports whether commit is an
// ancestor of it (i.e. commit is already contained in ref's history).
func isAncestorOfRef(repo *git.Repository, commit *object.Commit, ref plumbing.ReferenceName) (bool, error) {
	r, err := repo.Reference(ref, true)
	if err != nil {
		return false, fmt.Errorf("failed to resolve ref %s: %w", ref, err)
	}
	target, err := repo.CommitObject(r.Hash())
	if err != nil {
		return false, fmt.Errorf("failed to resolve commit for ref %s: %w", ref, err)
	}
	return commit.IsAncestor(target)
}

// BranchStatus describes branchName's position relative to mainBranch.
type BranchStatus struct {
	// BranchExists is false once the branch has been deleted (e.g. after a
	// "delete branch on merge" or manual cleanup). AheadOfMain/BehindMain are
	// only meaningful when true.
	BranchExists bool
	AheadOfMain  int
	BehindMain   int
}

// BranchAheadBehind reports branchName's commit position relative to
// mainBranch: how many commits are on the branch but not on main (ahead), and
// vice versa (behind) — mirroring `git rev-list --left-right --count
// branch...main`. Checks the local branch ref only; a branch already deleted
// locally reports BranchExists=false rather than an error, since that's the
// expected state for a shipped, cleaned-up item, not a failure.
func BranchAheadBehind(repoPath, branchName, mainBranch string) (BranchStatus, error) {
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return BranchStatus{}, fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}

	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return BranchStatus{BranchExists: false}, nil
	}
	mainRef, err := repo.Reference(plumbing.NewBranchReferenceName(mainBranch), true)
	if err != nil {
		return BranchStatus{}, fmt.Errorf("failed to resolve local %s: %w", mainBranch, err)
	}

	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return BranchStatus{}, fmt.Errorf("failed to resolve commit for %s: %w", branchName, err)
	}
	mainCommit, err := repo.CommitObject(mainRef.Hash())
	if err != nil {
		return BranchStatus{}, fmt.Errorf("failed to resolve commit for %s: %w", mainBranch, err)
	}

	ahead, err := countCommitsNotAncestorOf(branchCommit, mainCommit)
	if err != nil {
		return BranchStatus{}, fmt.Errorf("failed to count commits ahead of %s: %w", mainBranch, err)
	}
	behind, err := countCommitsNotAncestorOf(mainCommit, branchCommit)
	if err != nil {
		return BranchStatus{}, fmt.Errorf("failed to count commits behind %s: %w", mainBranch, err)
	}

	return BranchStatus{BranchExists: true, AheadOfMain: ahead, BehindMain: behind}, nil
}

// BehindOriginMain fetches mainBranch from origin into worktreePath and reports how
// many commits origin/mainBranch has that worktreePath's currently checked-out HEAD
// does not — i.e. how far the checked-out branch has drifted behind main. Unlike
// BranchAheadBehind (which reads repoPath's LOCAL mainBranch ref and does no fetch of
// its own — fine for a UI badge computed against a repo some other process keeps
// fresh, but silently stale otherwise), this always fetches first, so the count can
// never be stale the way a check against an unfetched local branch ref would be.
// MergeMainIntoWorktree already fetches and merges against this exact
// origin/mainBranch ref, so this reuses the same reference point rather than
// introducing a second, possibly inconsistent notion of "how far behind" (BUG-044).
func BehindOriginMain(worktreePath, mainBranch string) (int, error) {
	if err := FetchBranch(worktreePath, mainBranch); err != nil {
		return 0, fmt.Errorf("failed to fetch %s: %w", mainBranch, err)
	}

	repo, err := OpenRepo(worktreePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open git repo at %s: %w", worktreePath, err)
	}
	head, err := repo.Head()
	if err != nil {
		return 0, fmt.Errorf("failed to resolve HEAD in %s: %w", worktreePath, err)
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return 0, fmt.Errorf("failed to resolve HEAD commit: %w", err)
	}
	mainRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", mainBranch), true)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve origin/%s: %w", mainBranch, err)
	}
	mainCommit, err := repo.CommitObject(mainRef.Hash())
	if err != nil {
		return 0, fmt.Errorf("failed to resolve commit for origin/%s: %w", mainBranch, err)
	}

	behind, err := countCommitsNotAncestorOf(mainCommit, headCommit)
	if err != nil {
		return 0, fmt.Errorf("failed to count commits behind origin/%s: %w", mainBranch, err)
	}
	return behind, nil
}

// countCommitsNotAncestorOfCap bounds the walk in countCommitsNotAncestorOf so a
// long-diverged pair of branches can't turn a status check into an unbounded scan;
// a UI badge only ever needs to distinguish "a few commits" from "many".
const countCommitsNotAncestorOfCap = 500

// countCommitsNotAncestorOf walks back from "from" via parent edges, counting
// commits until each path reaches one that is an ancestor of "target" (i.e. the
// merge-base on that path) — equivalent to `git rev-list from ^target --count`.
func countCommitsNotAncestorOf(from, target *object.Commit) (int, error) {
	seen := map[plumbing.Hash]bool{from.Hash: true}
	queue := []*object.Commit{from}
	count := 0
	for len(queue) > 0 && count < countCommitsNotAncestorOfCap {
		c := queue[0]
		queue = queue[1:]
		isAncestor, err := c.IsAncestor(target)
		if err != nil {
			return count, err
		}
		if isAncestor {
			continue
		}
		count++
		if err := c.Parents().ForEach(func(p *object.Commit) error {
			if !seen[p.Hash] {
				seen[p.Hash] = true
				queue = append(queue, p)
			}
			return nil
		}); err != nil {
			return count, err
		}
	}
	return count, nil
}

// FileStat describes one file's change between two commits, as returned by
// FileStatsBetween. Path is the file's path as of headSHA — for a rename
// this is the new path, not the old one. Status is one of "added",
// "deleted", "renamed", or "modified".
type FileStat struct {
	Path      string
	Status    string
	Additions int
	Deletions int
}

// ShippedCommit describes one commit in the range shipped by a work session.
type ShippedCommit struct {
	SHA        string
	Summary    string // first line of the commit message
	AuthorAt   time.Time
	AuthorName string
}

// CommitInfo returns the summary line, author and author timestamp for a single
// resolved commit hash in the repo at repoPath, read via go-git — no subshell
// (the `prefer-go-git-over-subshells` skill).
func CommitInfo(repoPath, sha string) (ShippedCommit, error) {
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return ShippedCommit{}, fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}
	c, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return ShippedCommit{}, fmt.Errorf("failed to resolve commit %s: %w", sha, err)
	}
	summary, _, _ := strings.Cut(c.Message, "\n")
	return ShippedCommit{
		SHA:        c.Hash.String(),
		Summary:    strings.TrimSpace(summary),
		AuthorAt:   c.Author.When,
		AuthorName: c.Author.Name,
	}, nil
}

// listShippedCommitsCap bounds ListShippedCommits the same way
// countCommitsNotAncestorOfCap bounds the ahead/behind walk — a UI commit list
// only ever needs "the last several", not an unbounded history dump.
const listShippedCommitsCap = 100

// ListShippedCommits returns the commits reachable from headSHA but not from
// baseSHA — i.e. what a work session's commit range actually shipped — newest
// first, like a PR's "Commits" tab. Both SHAs must already be resolved commit
// hashes (not branch names): the caller typically has these directly from
// GitWorktreeData.BaseCommitSHA and the work session's LastCommitSha, which
// remain valid even after the branch itself has been deleted post-merge.
// truncated reports whether the walk hit listShippedCommitsCap before
// exhausting the range — the caller may want to surface that a longer list
// exists than what's returned.
func ListShippedCommits(ctx context.Context, repoPath, baseSHA, headSHA string) (commits []ShippedCommit, truncated bool, err error) {
	return listShippedCommitsWithCap(ctx, repoPath, baseSHA, headSHA, listShippedCommitsCap)
}

// listShippedCommitsWithCap is ListShippedCommits' implementation with the
// cap taken as a parameter rather than read from the package-level
// listShippedCommitsCap constant, so a test can exercise the truncated==true
// path against a small fixture (e.g. cap=3) instead of needing a 100+-commit
// repo.
func listShippedCommitsWithCap(ctx context.Context, repoPath, baseSHA, headSHA string, maxCommits int) ([]ShippedCommit, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	repo, err := OpenRepo(repoPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}

	head, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, false, fmt.Errorf("failed to resolve commit %s: %w", headSHA, err)
	}
	base, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return nil, false, fmt.Errorf("failed to resolve commit %s: %w", baseSHA, err)
	}

	var commits []ShippedCommit
	seen := map[plumbing.Hash]bool{head.Hash: true}
	queue := []*object.Commit{head}
	for len(queue) > 0 && len(commits) < maxCommits {
		if err := ctx.Err(); err != nil {
			return commits, len(commits) >= maxCommits, err
		}
		c := queue[0]
		queue = queue[1:]
		isAncestor, err := c.IsAncestor(base)
		if err != nil {
			return commits, len(commits) >= maxCommits, err
		}
		if isAncestor {
			continue
		}
		summary, _, _ := strings.Cut(c.Message, "\n")
		commits = append(commits, ShippedCommit{
			SHA:        c.Hash.String(),
			Summary:    strings.TrimSpace(summary),
			AuthorAt:   c.Author.When,
			AuthorName: c.Author.Name,
		})
		if err := c.Parents().ForEach(func(p *object.Commit) error {
			if !seen[p.Hash] {
				seen[p.Hash] = true
				queue = append(queue, p)
			}
			return nil
		}); err != nil {
			return commits, len(commits) >= maxCommits, err
		}
	}
	return commits, len(commits) >= maxCommits, nil
}

// AggregateDiffStat is the files-changed/additions/deletions summary
// returned by DiffStatBetween — a DiffShortstat-equivalent for a branch's
// range vs. its base. Distinct from DiffStats (session/git/diff.go), which
// holds working-tree diff content, not a committed-range summary.
type AggregateDiffStat struct {
	FilesChanged int
	Additions    int
	Deletions    int
}

// DiffStatBetween returns the aggregate files-changed/additions/deletions
// between baseSHA and headSHA, summing FileStatsBetween's per-file counts.
// ctx only guards against starting once the caller's deadline has passed —
// FileStatsBetween's go-git patch computation takes no ctx, so this can't
// bound a hang mid-computation.
func DiffStatBetween(ctx context.Context, repoPath, baseSHA, headSHA string) (AggregateDiffStat, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return AggregateDiffStat{}, err
	}
	stats, err := FileStatsBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		return AggregateDiffStat{}, err
	}
	var additions, deletions int
	for _, s := range stats {
		additions += s.Additions
		deletions += s.Deletions
	}
	return AggregateDiffStat{FilesChanged: len(stats), Additions: additions, Deletions: deletions}, nil
}

// DiffContentBetween returns the unified-diff text and added/removed line
// counts between baseSHA and headSHA (both resolved as commits, not the
// working tree — equivalent to `git diff baseSHA..headSHA`), using go-git's
// typed diff API instead of a safeexec shell-out (the
// `prefer-go-git-over-subshells` skill). Distinct from GitWorktree.Diff
// (session/git/diff.go), which diffs the working tree against a single ref
// (via `git add -N .` + `git diff <sha>`) and has no go-git equivalent —
// see that function's doc comment for why the shell-out stays there.
func DiffContentBetween(repoPath, baseSHA, headSHA string) (*DiffStats, error) {
	if baseSHA == headSHA {
		return &DiffStats{}, nil
	}
	patch, err := diffPatchBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		return nil, err
	}
	stats := &DiffStats{Content: patch.String()}
	// Counts via chunk.Type()/chunk.Content() on the typed patch, not by
	// string-prefix-scanning patch.String() — a "-"/"+" prefix check would
	// misclassify a content line that itself starts with "--"/"++" (e.g. a
	// removed `-- comment` or an added `++i`) as a diff header line.
	// FileStatsBetween below uses the same approach per-file.
	for _, fp := range patch.FilePatches() {
		for _, chunk := range fp.Chunks() {
			lines := countChunkLines(chunk.Content())
			switch chunk.Type() {
			case fdiff.Add:
				stats.Added += lines
			case fdiff.Delete:
				stats.Removed += lines
			}
		}
	}
	return stats, nil
}

// countChunkLines counts the lines in a diff chunk's content, crediting a
// final non-newline-terminated line (no-newline-at-EOF) as one more line.
func countChunkLines(content string) int {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if content[len(content)-1] != '\n' {
		lines++
	}
	return lines
}

// FileStatsBetween returns the per-file diff-stat summary (path, status,
// additions, deletions) for every file that changed between baseSHA and
// headSHA in the repo at repoPath, using go-git's typed diff API — no
// safeexec shell-out (the `prefer-go-git-over-subshells` skill).
//
// Renames are reported as a single entry keyed by the file's new path, not a
// delete+add pair: go-git's FilePatch.Files() already exposes the from/to
// path pair needed to detect this directly, so unlike object.Patch.Stats()
// (whose FileStat.Name collapses a rename into a single "old => new" display
// string) this walks Patch.FilePatches() itself to keep the old and new
// paths distinct. Binary files are silently omitted — go-git produces zero
// diff chunks for them (the same signal it uses to skip submodule-ref-only
// changes), so there is no meaningful addition/deletion count to report;
// this mirrors go-git's own Stats() behavior rather than the "0/0 entry"
// shape one might expect, a discrepancy confirmed against go-git v5.14.0's
// source (getFileStatsFromFilePatches in plumbing/object/patch.go) and a
// throwaway spike before this function was written.
func FileStatsBetween(repoPath, baseSHA, headSHA string) ([]FileStat, error) {
	if baseSHA == headSHA {
		return nil, nil
	}

	patch, err := diffPatchBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		return nil, err
	}

	var stats []FileStat
	for _, fp := range patch.FilePatches() {
		chunks := fp.Chunks()
		if len(chunks) == 0 {
			// Binary file (or submodule ref update) — no line-level diff to
			// report; see the doc comment above.
			continue
		}

		from, to := fp.Files()
		stat := FileStat{}
		switch {
		case from == nil:
			stat.Status = "added"
			stat.Path = to.Path()
		case to == nil:
			stat.Status = "deleted"
			stat.Path = from.Path()
		case from.Path() != to.Path():
			stat.Status = "renamed"
			stat.Path = to.Path()
		default:
			stat.Status = "modified"
			stat.Path = to.Path()
		}

		for _, chunk := range chunks {
			lines := countChunkLines(chunk.Content())
			switch chunk.Type() {
			case fdiff.Add:
				stat.Additions += lines
			case fdiff.Delete:
				stat.Deletions += lines
			}
		}

		stats = append(stats, stat)
	}

	return stats, nil
}

// diffPatchBetween opens repoPath, resolves baseSHA/headSHA, and returns the
// object.Patch between them — the shared resolve+diff step behind both
// FileStatsBetween and DiffHashBetween.
func diffPatchBetween(repoPath, baseSHA, headSHA string) (*object.Patch, error) {
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}

	baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve commit %s in %s: %w", baseSHA, repoPath, err)
	}
	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve commit %s in %s: %w", headSHA, repoPath, err)
	}

	patch, err := baseCommit.Patch(headCommit)
	if err != nil {
		return nil, fmt.Errorf("failed to diff %s..%s in %s: %w", baseSHA, headSHA, repoPath, err)
	}
	return patch, nil
}

// DiffHashBetween returns a stable content hash of the diff between baseSHA
// and headSHA in the repo at repoPath — the actual added/removed line
// content per file, not just per-file counts (a per-file
// path+status+addition-count+deletion-count tuple can collide across two
// genuinely different edits, e.g. two different single-line replacements on
// the same file both show as one addition and one deletion; hashing the
// actual line text avoids that false-collision shape). Feeds
// session.IsFlakyVerdictFlipFlop's DiffHash comparison: the same code
// reviewed twice must hash identically; any real change to the diff must
// not. Unlike a hash of the (possibly token-capped) prompt text sent to a
// reviewer, this is computed from the full, untruncated diff, so two
// different diffs that happen to share a truncated prefix can never
// collide here.
func DiffHashBetween(repoPath, baseSHA, headSHA string) (string, error) {
	if baseSHA == headSHA {
		return fmt.Sprintf("%x", sha256.Sum256(nil)), nil
	}

	patch, err := diffPatchBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		return "", err
	}

	return diffHashFromFilePatches(patch.FilePatches()), nil
}

// diffHashFromFilePatches is DiffHashBetween's core, split out so a test can
// drive it with fake fdiff.FilePatch values instead of coaxing a real git
// repository into producing a specific go-git edge case.
func diffHashFromFilePatches(filePatches []fdiff.FilePatch) string {
	type fileDigest struct {
		path    string
		content string
	}
	digests := make([]fileDigest, 0, len(filePatches))
	for _, fp := range filePatches {
		chunks := fp.Chunks()
		if len(chunks) == 0 {
			// Binary file (or submodule ref update) — no line-level diff to
			// report; see FileStatsBetween's doc comment. Also covers symlink
			// changes: go-git's Files() returns (nil, nil) for non-regular-file
			// tree entries regardless of add/modify/delete, which would
			// otherwise panic below on the assumption that at most one side is
			// nil.
			continue
		}

		from, to := fp.Files()
		path := ""
		status := ""
		switch {
		case from == nil && to == nil:
			// go-git can return (nil, nil) from Files() even when Chunks() is
			// non-empty -- observed in production on symlink/gitlink type-change
			// entries whose textual content still produces chunks. Skip rather
			// than falling into the single-sided cases below, which assume at
			// most one side is nil and would dereference the other.
			continue
		case from == nil:
			status, path = "added", to.Path()
		case to == nil:
			status, path = "deleted", from.Path()
		case from.Path() != to.Path():
			status, path = "renamed:"+from.Path(), to.Path()
		default:
			status, path = "modified", to.Path()
		}

		var b strings.Builder
		b.WriteString(status)
		b.WriteByte('\n')
		for _, chunk := range chunks {
			fmt.Fprintf(&b, "%d:%s\n", chunk.Type(), chunk.Content())
		}
		digests = append(digests, fileDigest{path: path, content: b.String()})
	}
	sort.Slice(digests, func(i, j int) bool { return digests[i].path < digests[j].path })

	h := sha256.New()
	for _, d := range digests {
		h.Write([]byte(d.path))
		h.Write([]byte{'\n'})
		h.Write([]byte(d.content))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// CheckoutBranch checks out a branch in an existing repository.
func CheckoutBranch(repoPath, branchName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "checkout", branchName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to checkout branch: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to checkout branch: %w", err)
	}
	return nil
}

// RemoteURL returns the URL of the named remote (usually "origin") for a local repo.
func RemoteURL(repoPath, remote string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", remote)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to get remote URL: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeMainResult describes the outcome of MergeMainIntoWorktree.
type MergeMainResult struct {
	// UpToDate is true when the worktree's branch already contained everything
	// from mainBranch — nothing was merged in.
	UpToDate bool
	// Merged is true when the merge (including a fast-forward) brought in new
	// commits from mainBranch.
	Merged bool
	// Conflicted is true when merging mainBranch produced conflicts. The merge is
	// always aborted before returning, so the worktree is left clean either way —
	// callers never have to clean up a half-merged tree.
	Conflicted bool
	// ConflictedFiles lists the paths that conflicted. Populated only when
	// Conflicted is true.
	ConflictedFiles []string
}

// MergeMainIntoWorktree fetches mainBranch from origin and merges it into whatever
// branch is currently checked out in worktreePath. It never leaves the worktree in a
// conflicted state: on conflict it aborts the merge immediately (via `git merge
// --abort`) and reports the conflicting paths, so the caller can hand that context to
// whoever resolves it rather than leaving a half-merged working tree behind for the
// next thing that touches it.
func MergeMainIntoWorktree(worktreePath, mainBranch string) (*MergeMainResult, error) {
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer fetchCancel()
	fetchCmd := safeexec.CommandContext(fetchCtx, "git", "-C", worktreePath, "fetch", "origin", mainBranch)
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %s (%w)", mainBranch, out, err)
	}

	// Capture HEAD before the merge so up-to-date can be detected by comparing SHAs
	// rather than parsing merge output text ("Already up to date." is locale- and
	// git-version-dependent, e.g. older git prints "Already up-to-date.").
	beforeSHA, headErr := getHeadCommitSHA(worktreePath)
	if headErr != nil {
		return nil, fmt.Errorf("failed to resolve HEAD before merge: %w", headErr)
	}

	mergeCtx, mergeCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer mergeCancel()
	mergeCmd := safeexec.CommandContext(mergeCtx, "git", "-C", worktreePath, "merge", "--no-edit", "origin/"+mainBranch)
	mergeOut, mergeErr := mergeCmd.CombinedOutput()
	if mergeErr == nil {
		afterSHA, headErr := getHeadCommitSHA(worktreePath)
		if headErr != nil {
			return nil, fmt.Errorf("failed to resolve HEAD after merge: %w", headErr)
		}
		if afterSHA == beforeSHA {
			return &MergeMainResult{UpToDate: true}, nil
		}
		return &MergeMainResult{Merged: true}, nil
	}

	// The merge failed. Distinguish real conflicts (recoverable — abort and report)
	// from any other git failure (propagate as-is; aborting a non-conflict failure
	// could mask the real problem).
	conflictFiles, conflictErr := conflictedFiles(worktreePath)
	if conflictErr != nil || len(conflictFiles) == 0 {
		return nil, fmt.Errorf("failed to merge %s: %s (%w)", mainBranch, mergeOut, mergeErr)
	}

	abortCtx, abortCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer abortCancel()
	abortCmd := safeexec.CommandContext(abortCtx, "git", "-C", worktreePath, "merge", "--abort")
	if abortOut, abortErr := abortCmd.CombinedOutput(); abortErr != nil {
		return nil, fmt.Errorf("merge of %s conflicted in %v, and merge --abort failed: %s (%w)", mainBranch, conflictFiles, abortOut, abortErr)
	}

	return &MergeMainResult{Conflicted: true, ConflictedFiles: conflictFiles}, nil
}

// conflictedFiles returns the paths with unresolved merge conflicts in worktreePath.
func conflictedFiles(worktreePath string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", worktreePath, "diff", "--name-only", "--diff-filter=U")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
