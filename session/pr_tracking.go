package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// PR Tracking Methods for Instance
// These methods provide GitHub PR lifecycle management for sessions created from PR URLs

// ErrNoPullRequest means the session has no PR for an action to target; an unset
// selector never falls through to PR number 0.
var ErrNoPullRequest = errors.New("no pull request")

// prActionClient is the GitHub surface the PR actions call, so tests can observe
// which PR an action reaches without shelling out to gh.
type prActionClient interface {
	MergePR(ctx context.Context, ref github.RepoRef, number int, method string) error
	ClosePR(ctx context.Context, ref github.RepoRef, number int) error
	PostPRComment(ctx context.Context, ref github.RepoRef, number int, body string) error
	GetPRComments(ctx context.Context, ref github.RepoRef, number int) ([]github.PRComment, error)
	GetPRDiff(ctx context.Context, ref github.RepoRef, number int) (string, error)
}

type realPRActions struct{}

func (realPRActions) MergePR(ctx context.Context, r github.RepoRef, n int, m string) error {
	return github.MergePR(ctx, r, n, m)
}
func (realPRActions) ClosePR(ctx context.Context, r github.RepoRef, n int) error {
	return github.ClosePR(ctx, r, n)
}
func (realPRActions) PostPRComment(ctx context.Context, r github.RepoRef, n int, b string) error {
	return github.PostPRComment(ctx, r, n, b)
}
func (realPRActions) GetPRComments(ctx context.Context, r github.RepoRef, n int) ([]github.PRComment, error) {
	return github.GetPRComments(ctx, r, n)
}
func (realPRActions) GetPRDiff(ctx context.Context, r github.RepoRef, n int) (string, error) {
	return github.GetPRDiff(ctx, r, n)
}

// SetPRActionClient replaces the GitHub client the PR actions use. Call it before
// the instance serves requests.
func (i *Instance) SetPRActionClient(c prActionClient) { i.prActions = c }

func (i *Instance) actionClient() prActionClient {
	if i.prActions != nil {
		return i.prActions
	}
	return realPRActions{}
}

// ResolveTargetPR names the PR an action targets: ref when given (it must belong
// to the session), else the session's active PR, else the legacy single PR.
func (i *Instance) ResolveTargetPR(ref *domain.PRRef) (domain.PRRef, error) { return i.resolvePR(ref) }

// resolvePR names the PR an action targets. A nil ref means the session's active
// PR (the legacy scalars for a session without links). An explicit ref must be
// attached to the session, so an action never reaches a PR the session does not
// hold. A session with no PR at all fails with ErrNoPullRequest.
func (i *Instance) resolvePR(ref *domain.PRRef) (domain.PRRef, error) {
	snap := i.Snapshot()
	if ref != nil {
		return i.resolveExplicitPR(snap, *ref)
	}
	if active := domain.ActivePR(snap.GitHub.PRs, snap.Branch); active != nil {
		return active.Ref, nil
	}
	return i.legacyPRRef()
}

func (i *Instance) resolveExplicitPR(snap *domain.InstanceSnapshot, ref domain.PRRef) (domain.PRRef, error) {
	if ref.IsZero() {
		return domain.PRRef{}, ErrZeroPRRef
	}
	if indexOfPR(snap.GitHub.PRs, ref) >= 0 {
		return ref, nil
	}
	if len(snap.GitHub.PRs) == 0 {
		if legacy, err := i.legacyPRRef(); err == nil && legacy == ref {
			return ref, nil
		}
	}
	return domain.PRRef{}, fmt.Errorf("%w: %s", ErrPRNotAttached, ref)
}

func (i *Instance) legacyPRRef() (domain.PRRef, error) {
	gh := i.GitHub()
	if !gh.IsPRSession() {
		return domain.PRRef{}, fmt.Errorf("instance '%s' is not a PR session: %w", i.Title, ErrNoPullRequest)
	}
	resolved, err := domain.NewPRRef(gh.Host, gh.Owner, gh.Repo, gh.PRNumber)
	if err != nil {
		return domain.PRRef{}, fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	return resolved, nil
}

// PRRepoRef converts a PRRef to the github package's repo reference. github.com is
// the empty host there, matching what the legacy scalars carried.
func PRRepoRef(r domain.PRRef) (github.RepoRef, error) {
	host := r.Host()
	if domain.IsGitHubCom(host) {
		host = ""
	}
	return github.NewRepoRefWithHost(r.Owner(), r.Repo(), host)
}

// RefreshPRInfo fetches the latest PR information from GitHub
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) RefreshPRInfo(ctx context.Context) (*github.PRInfo, error) {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return nil, err
	}
	return i.refreshPRInfoFor(ctx, pr)
}

func (i *Instance) refreshPRInfoFor(ctx context.Context, pr domain.PRRef) (*github.PRInfo, error) {
	log.Info("refreshing PR info", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	ref, err := PRRepoRef(pr)
	if err != nil {
		return nil, fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	prInfo, err := github.GetPRInfoCtx(ctx, ref, pr.Number())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR info for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully refreshed PR info", "session", i.Title, "title", prInfo.Title, "state", prInfo.State)

	return prInfo, nil
}

// GetPRComments fetches all comments on the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRComments(ctx context.Context, target *domain.PRRef) ([]github.PRComment, error) {
	pr, err := i.resolvePR(target)
	if err != nil {
		return nil, err
	}

	log.Info("fetching PR comments", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	ref, err := PRRepoRef(pr)
	if err != nil {
		return nil, fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	comments, err := i.actionClient().GetPRComments(ctx, ref, pr.Number())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR comments for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR comments", "session", i.Title, "count", len(comments))

	return comments, nil
}

// GetPRDiff fetches the diff for the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRDiff(ctx context.Context, target *domain.PRRef) (string, error) {
	pr, err := i.resolvePR(target)
	if err != nil {
		return "", err
	}

	log.Info("fetching PR diff", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	ref, err := PRRepoRef(pr)
	if err != nil {
		return "", fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	diff, err := i.actionClient().GetPRDiff(ctx, ref, pr.Number())
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR diff", "session", i.Title, "bytes", len(diff))

	return diff, nil
}

// PostComment posts a comment to the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) PostComment(ctx context.Context, target *domain.PRRef, body string) error {
	pr, err := i.resolvePR(target)
	if err != nil {
		return err
	}

	if body == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	log.Info("posting comment to PR", "pr", pr.Number(), "session", i.Title)

	ref, err := PRRepoRef(pr)
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	if err := i.actionClient().PostPRComment(ctx, ref, pr.Number(), body); err != nil {
		return fmt.Errorf("failed to post comment to PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully posted comment to PR", "pr", pr.Number(), "session", i.Title)

	return nil
}

// SetCommitStatus posts a commit status to the PR's current head commit via the GitHub
// Statuses API. It re-fetches PR info first so the status always lands on the current
// HEAD SHA rather than a stale one cached before a force-push or rebase.
// Returns an error if this is not a PR session or if the GitHub API call fails.
//
// Currently has no non-test callers; an untagged ctx inherits GitHubCallOriginFrom's
// background default until a caller exists.
func (i *Instance) SetCommitStatus(ctx context.Context, state github.CommitStatusState, statusContext, description string) error {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return err
	}

	prInfo, err := i.RefreshPRInfo(ctx)
	if err != nil {
		return err
	}

	req, err := github.NewCommitStatusRequest(prInfo.HeadSHA, state, statusContext, description)
	if err != nil {
		return fmt.Errorf("invalid commit status request for instance '%s': %w", i.Title, err)
	}

	repo, err := PRRepoRef(pr)
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}

	log.Info("setting commit status on PR", "pr", pr.Number(), "session", i.Title, "sha", prInfo.HeadSHA, "state", state, "context", statusContext)

	if err := github.SetCommitStatus(repo, req); err != nil {
		return fmt.Errorf("failed to set commit status for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully set commit status on PR", "pr", pr.Number(), "session", i.Title, "sha", prInfo.HeadSHA, "state", state)

	return nil
}

// MergePR merges the PR using the specified merge method
// method can be: "merge", "squash", or "rebase"
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) MergePR(ctx context.Context, target *domain.PRRef, method string) error {
	pr, err := i.resolvePR(target)
	if err != nil {
		return err
	}

	// Validate merge method
	switch method {
	case "merge", "squash", "rebase":
		// Valid method
	case "":
		method = "merge" // Default to merge if not specified
	default:
		return fmt.Errorf("invalid merge method '%s': must be 'merge', 'squash', or 'rebase'", method)
	}

	log.Info("merging PR", "pr", pr.Number(), "session", i.Title, "method", method)

	ref, err := PRRepoRef(pr)
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	if err := i.actionClient().MergePR(ctx, ref, pr.Number(), method); err != nil {
		return fmt.Errorf("failed to merge PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully merged PR", "pr", pr.Number(), "session", i.Title)

	return nil
}

// ClosePR closes the PR without merging
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) ClosePR(ctx context.Context, target *domain.PRRef) error {
	pr, err := i.resolvePR(target)
	if err != nil {
		return err
	}

	log.Info("closing PR without merging", "pr", pr.Number(), "session", i.Title)

	ref, err := PRRepoRef(pr)
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	if err := i.actionClient().ClosePR(ctx, ref, pr.Number()); err != nil {
		return fmt.Errorf("failed to close PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully closed PR", "pr", pr.Number(), "session", i.Title)

	return nil
}

// GeneratePRContextPrompt generates a context prompt for Claude based on PR information
// This can be used to initialize a Claude Code session with comprehensive PR context
// Returns an error if this is not a PR session or if the GitHub API call fails
//
// Currently has no non-test callers; an untagged ctx inherits GitHubCallOriginFrom's
// background default until a caller exists.
func (i *Instance) GeneratePRContextPrompt(ctx context.Context, target *domain.PRRef) (string, error) {
	pr, err := i.resolvePR(target)
	if err != nil {
		return "", err
	}

	log.Info("generating PR context prompt", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	// Fetch PR information
	prInfo, err := i.refreshPRInfoFor(ctx, pr)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR info for context prompt: %w", err)
	}

	// Generate prompt with PR description
	prompt := github.GeneratePRPrompt(prInfo, true)

	log.Info("successfully generated PR context prompt", "session", i.Title, "bytes", len(prompt))

	return prompt, nil
}
