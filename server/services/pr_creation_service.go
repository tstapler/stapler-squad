package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// draftPRDescriptionTimeout bounds the synchronous LLM call inside
// DraftPullRequest. server/server.go sets WriteTimeout: 0 for the whole server
// (streaming connections are long-lived), so nothing else stops a stalled
// headless pool from hanging this RPC forever. 30s matches the "May take
// 5-30 seconds" duration documented for the analogous GenerateSuggestedRule
// RPC (proto/session/v1/session.proto) — that RPC's own 60s figure is the
// client-side AbortController deadline, not a server-side bound, so it isn't
// the right number to reuse here.
const draftPRDescriptionTimeout = 30 * time.Second

// PRCreationService handles the DraftPullRequest/CreatePullRequest RPCs. It was
// extracted from SessionService (Post-Review Revision #3) to match the codebase's
// existing extracted-domain-service pattern (see reviewQueueSvc, workspaceSvc,
// backlogSvc, checkpointSvc on SessionService).
type PRCreationService struct {
	storage                  session.InstanceStore
	eventBus                 *events.EventBus
	headlessPool             *headless.Pool
	backlogLifecycleListener *session.BacklogLifecycleListener
	findInstance             func(string) *session.Instance
	prCreationInFlight       sync.Map

	// vcsReader is a single shared *unfinished.GoGitVCSReader instance reused
	// across every DraftPullRequest call, so its repo/diff/ahead-behind caches
	// (session/unfinished/gogit_vcs_reader.go) actually have something to hit —
	// mirroring session/unfinished/scanner.go's NewScannerWithReader, the only
	// other production caller, which likewise constructs one instance and
	// reuses it rather than allocating a throwaway reader per call.
	vcsReader *unfinished.GoGitVCSReader
}

// NewPRCreationService constructs a PRCreationService. findInstance is a narrow
// function value (not the whole *SessionService) so this service depends only on
// what it needs, per interface-pollution-checklist.md's "define the
// interface/dependency where it's consumed, scoped narrowly" guidance.
func NewPRCreationService(
	storage session.InstanceStore,
	eventBus *events.EventBus,
	headlessPool *headless.Pool,
	backlogLifecycleListener *session.BacklogLifecycleListener,
	findInstance func(string) *session.Instance,
) *PRCreationService {
	return &PRCreationService{
		storage:                  storage,
		eventBus:                 eventBus,
		headlessPool:             headlessPool,
		backlogLifecycleListener: backlogLifecycleListener,
		findInstance:             findInstance,
		vcsReader:                &unfinished.GoGitVCSReader{},
	}
}

// SetHeadlessPool wires the headless LLM pool after construction, mirroring
// SessionService.SetHeadlessPool's propagation to autonomousSvc — headlessPool is
// nil at NewPRCreationService construction time and only becomes available once
// the server finishes startup wiring.
func (s *PRCreationService) SetHeadlessPool(pool *headless.Pool) {
	s.headlessPool = pool
}

// SetBacklogLifecycleListener wires the listener after construction, mirroring
// SessionService.SetBacklogLifecycleListener's field assignment — the listener is
// nil at NewPRCreationService construction time for the same reason.
func (s *PRCreationService) SetBacklogLifecycleListener(l *session.BacklogLifecycleListener) {
	s.backlogLifecycleListener = l
}

// resolveSessionWorktree resolves sessionID to its live *session.Instance and
// *git.GitWorktree, or a connect error with the right code for every failure mode:
// CodeInvalidArgument (empty ID), CodeNotFound (no live instance), or
// CodeFailedPrecondition (instance has no worktree, or GetGitWorktree itself fails
// — e.g. "instance has not been started"). Shared by DraftPullRequest and
// CreatePullRequest (Epic 1.4) so both RPCs resolve a session's worktree identically.
func (s *PRCreationService) resolveSessionWorktree(sessionID string) (*session.Instance, *git.GitWorktree, error) {
	if sessionID == "" {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	inst := s.findInstance(sessionID)
	if inst == nil {
		return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", sessionID))
	}
	if !inst.HasGitWorktree() {
		return nil, nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session has no worktree"))
	}
	wt, err := inst.GetGitWorktree()
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return inst, wt, nil
}

// fallbackPRBody is the deterministic PR body used when there is no diff to describe,
// or when drafting via the LLM is unavailable/fails. This session has no backlog item,
// so buildFallbackPRBody's item-description-driven template doesn't apply verbatim —
// this is the minimal session-shaped equivalent (Task 1.3.1c).
const fallbackPRBody = "## Summary\n\n_No diff description available._"

// sessionGoalText returns inst's active session goal text, or "" if no goal is set.
// This is DraftPullRequest's answer to headless.DraftPRDescription's itemDescription
// argument for a session, which (unlike a backlog item) has no fixed problem-statement
// field (Post-Review Revision #1 / Task 1.3.1c).
func sessionGoalText(inst *session.Instance) string {
	if goal := inst.GetSessionGoal(); goal != nil {
		return goal.Goal
	}
	return ""
}

// +api: session:draft-pull-request
// DraftPullRequest is the read-only preview RPC behind the "Create PR" modal: it
// resolves the session's worktree, short-circuits with the existing PR's info when
// one is already associated with the session, and otherwise resolves the default
// base branch, checks whether the branch has commits ahead of it, and drafts a
// title/body for the user to review before CreatePullRequest (Epic 1.4) actually
// pushes/creates anything.
//
// This method makes no commit/push side effects (no CommitChanges, no PushBranch) —
// Post-Review Revision #2's read-only fix. Note wt.Diff() does run `git add -N .` to
// stage untracked files for diffing (session/git/diff.go), which touches the index
// but not file content or history. Its diff preview is computed via the same
// working-tree-inclusive path GitWorktree.Diff() already uses for the session card's
// diff viewer, not a committed-only diff, so the drafted title/body always describes
// what the user is actually looking at.
func (s *PRCreationService) DraftPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.DraftPullRequestRequest],
) (*connect.Response[sessionv1.DraftPullRequestResponse], error) {
	inst, wt, err := s.resolveSessionWorktree(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	// Existing-PR short-circuit (AC4): skip all diff/draft work below entirely.
	snap := inst.Snapshot()
	if snap.GitHub.GitHubPRURL != "" {
		return connect.NewResponse(&sessionv1.DraftPullRequestResponse{
			ExistingPrUrl:    snap.GitHub.GitHubPRURL,
			ExistingPrNumber: int32(snap.GitHub.GitHubPRNumber),
		}), nil
	}

	baseBranch := s.vcsReader.ResolveDefaultBranch(wt.GetRepoPath())
	// ResolveDefaultBranch returns a remote-qualified short ref name (e.g.
	// "origin/main") whenever refs/remotes/origin/HEAD resolves, and only
	// returns a bare branch name when there's no origin remote at all. `gh pr
	// create --base` rejects a remote-qualified value ("Base ref must be a
	// branch"), so strip a single leading "<remote>/" component before this
	// becomes the response's (and eventually CreatePullRequest's) BaseBranch.
	if _, short, ok := strings.Cut(baseBranch, "/"); ok {
		baseBranch = short
	}

	// HasCommitsAheadOfMain fails open (returns true on error) by contract — ignore
	// the error and trust the bool, matching pushAndCreatePR's existing usage
	// (session/backlog_lifecycle_pr.go).
	hasCommits, _ := wt.HasCommitsAheadOfMain(baseBranch)

	// Working-tree-inclusive diff preview (Post-Review Revision #2) — NOT
	// session.GetGitDiff (committed-only).
	diffStats := wt.Diff()
	var diff string
	if diffStats != nil && diffStats.Error == nil {
		diff = diffStats.Content
	}

	body := fallbackPRBody
	if strings.TrimSpace(diff) != "" {
		// Post-Review Revision #1: guard on s.headlessPool being non-nil first,
		// mirroring RunOneShot's existing guard — never panic on a nil pool.
		if s.headlessPool != nil {
			draftCtx, draftCancel := context.WithTimeout(ctx, draftPRDescriptionTimeout)
			draftedBody, draftCostUSD, draftErr := headless.DraftPRDescription(draftCtx, s.headlessPool, inst.Title, sessionGoalText(inst), diff, wt.GetBranchName())
			draftCancel()
			if concreteStorage, ok := s.storage.(*session.Storage); ok {
				session.CostSinkForSessionUUID(concreteStorage, inst.UUID)(draftCostUSD)
			}
			if draftErr != nil {
				log.Warn("DraftPullRequest: DraftPRDescription failed, using fallback body", "session", req.Msg.SessionId, "err", draftErr)
			} else {
				body = draftedBody
			}
		}
	}

	return connect.NewResponse(&sessionv1.DraftPullRequestResponse{
		Title:           inst.Title,
		Body:            body,
		BaseBranch:      baseBranch,
		HasCommitsAhead: hasCommits,
	}), nil
}

// +api: session:create-pull-request
// CreatePullRequest is the mutating RPC behind the "Create PR" modal's submit
// button: it commits+pushes any remaining dirty state, calls GitWorktree.CreatePR
// mechanically (no agentic RunOneShot turn — Story 1.4.1's AC3), persists the
// resulting PR URL/number on the session with explicit partial-failure
// signaling (Task 1.4.1d), and reconciles a backlog-linked session's item status
// via RecordPRCreatedOutOfBand.
func (s *PRCreationService) CreatePullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.CreatePullRequestRequest],
) (*connect.Response[sessionv1.CreatePullRequestResponse], error) {
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	inst, wt, err := s.resolveSessionWorktree(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	// In-flight guard (pitfalls.md §3): a double-click or a second browser tab
	// must not race two pushes/PR-creations for the same session.
	if _, loaded := s.prCreationInFlight.LoadOrStore(req.Msg.SessionId, true); loaded {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("PR creation already in progress for this session"))
	}
	defer s.prCreationInFlight.Delete(req.Msg.SessionId)

	// Commit dirty state + push — unconditionally, and BEFORE the existing-PR
	// fast path below (pitfalls.md §5): otherwise a second click after new
	// commits would silently never reach GitHub.
	//
	// Unlike pushAndCreatePR's unattended backlog-automation path (which only
	// logs a CommitChanges failure and proceeds), this handler is behind a
	// human-driven review-then-publish flow — the user just reviewed this diff
	// in the session card's diff viewer, so a commit failure here must not
	// silently drop in-scope changes (Post-Review Revision #2).
	commitMsg := fmt.Sprintf("[stapler-squad] work complete for %q (pre-PR)", inst.Title)
	if commitErr := wt.CommitChanges(commitMsg); commitErr != nil {
		return nil, connect.NewError(connect.CodeInternal, commitErr)
	}
	if pushErr := wt.PushBranch(); pushErr != nil {
		return nil, connect.NewError(connect.CodeUnavailable, pushErr)
	}

	// Fast path: reuse a PR the session already has cached rather than calling
	// CreatePR again. alreadyExisted reflects only whether THIS code path took
	// the fast path — CreatePR itself already transparently reuses an existing
	// PR at the gh/findExistingPR level when it does run, but from this
	// handler's perspective that's still a "create" attempt, not a cache hit,
	// so don't expect perfect create-vs-reuse fidelity from a single gh race.
	snap := inst.Snapshot()
	var prURL string
	var prNumber int
	var alreadyExisted bool
	if snap.GitHub.GitHubPRURL != "" && snap.GitHub.GitHubPRNumber > 0 {
		prURL = snap.GitHub.GitHubPRURL
		prNumber = snap.GitHub.GitHubPRNumber
		alreadyExisted = true
	} else {
		prURL, prNumber, err = wt.CreatePR(git.PRCreateOptions{
			Title:      req.Msg.Title,
			Body:       req.Msg.Body,
			BaseBranch: req.Msg.BaseBranch,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeUnavailable, err)
		}
		if prNumber == 0 {
			// BUG-063(a) analog: a PR URL with no resolvable number is unusable
			// downstream (EnablePRAutoMerge, ReconcilePRPending's PrNumberGT(0)
			// filter) — treat it as a creation failure rather than returning it.
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("PR created but its number could not be determined"))
		}
		alreadyExisted = false
	}

	// Persist + publish, with explicit partial-failure signaling (BUG-040
	// analog): a persist failure must never be reported as a create failure —
	// the PR is already real on GitHub — but it also must not be silently
	// swallowed, so the client gets persisted=false/persistError to warn on.
	inst.SetGitHubPR(prURL, prNumber)
	persisted := true
	persistError := ""
	if saveErr := s.storage.SaveInstances([]*session.Instance{inst}); saveErr != nil {
		persisted = false
		persistError = saveErr.Error()
		log.Warn("CreatePullRequest: failed to persist PR URL", "session", req.Msg.SessionId, "err", saveErr)
	} else {
		s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url", "github_pr_number"}))
	}

	// This session may be backlog-linked even though the PR was created via
	// the manual modal rather than the automated pushAndCreatePR path — without
	// this call the item would be left in "review" forever, invisible to
	// ReconcilePRPending (mirrors RunOneShot's existing call site). No-op for
	// non-backlog sessions; nil-checked since the listener is wired in after
	// construction (SetBacklogLifecycleListener).
	if s.backlogLifecycleListener != nil {
		s.backlogLifecycleListener.RecordPRCreatedOutOfBand(ctx, inst.UUID, prURL, prNumber)
	}

	return connect.NewResponse(&sessionv1.CreatePullRequestResponse{
		PrUrl:          prURL,
		PrNumber:       int32(prNumber),
		AlreadyExisted: alreadyExisted,
		Persisted:      persisted,
		PersistError:   persistError,
	}), nil
}
