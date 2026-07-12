package services

// backlog_service_triage.go — session spawning and triage/review orchestration handlers
// for BacklogService. Covers the full lifecycle of headless triage, review, and re-review.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/headless"
)

// headlessTriageUUIDPrefix is prepended to all synthetic ItemSession UUIDs created by the
// headless triage path. The orphan guard uses this prefix to identify sessions that have no
// live tmux process and can be safely tombstoned on re-trigger.
// headlessReReviewUUIDPrefix is the equivalent prefix for headless re-review sessions.
const (
	headlessTriageUUIDPrefix   = "headless-triage-"
	headlessReReviewUUIDPrefix = "headless-re-review-"
)

// maxAutoReworkIterations caps how many automated work sessions can be spawned for a single
// backlog item by the auto-reopen loop. When this ceiling is hit, the item stays in review
// so a human can inspect it rather than spinning indefinitely on a persistent FAIL verdict.
const maxAutoReworkIterations = 3

// maxConcurrentBacklogWorkItems caps how many distinct backlog items may be
// "in_progress" (i.e. have a live work session) at the same time. Fresh spawns
// beyond this cap are rejected with CodeResourceExhausted; reopen/revision
// spawns for an item that's already in_progress don't count against it, since
// they don't add a new concurrent item. Adjust this constant directly — it's
// an operational tuning knob, not a correctness invariant.
//
// Added 2026-07-12 after a kernel OOM caused by too many concurrent agent
// sessions (backlog-spawned and otherwise) exhausting system memory.
const maxConcurrentBacklogWorkItems = 2

// defaultTriageCleanupTimeout bounds the DB writes TriggerTriage's goroutine makes
// after its headless LLM call returns (persist result, update plan_artifacts_path,
// transition idea->ready, mark session ended). See BacklogService.triageCleanupTimeout
// for why this needed to become configurable rather than a global.
const defaultTriageCleanupTimeout = 10 * time.Second

// maxTriageSessionAge is the maximum age of an open triage ItemSession before it is
// treated as orphaned in the re-trigger guard. This prevents a hung or leaked session
// from blocking re-trigger indefinitely.
const maxTriageSessionAge = 2 * time.Hour

// slugify converts s to a lowercase hyphen-delimited slug safe for file paths.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// triageShortTitle extracts the triage-suggested short title from the most recent
// completed triage ItemSession, falling back to a truncated slug of itemTitle.
func triageShortTitle(sessions []session.ItemSessionSummary, itemTitle string) string {
	for i := len(sessions) - 1; i >= 0; i-- {
		s := sessions[i]
		if s.Role != string(session.SessionRoleTriage) || s.TriageResult == "" {
			continue
		}
		var r session.HeadlessTriageResult
		if err := json.Unmarshal([]byte(s.TriageResult), &r); err == nil && r.Title != "" {
			return r.Title
		}
	}
	// Fallback: first 4 words of the slug.
	parts := strings.SplitN(slugify(itemTitle), "-", 5)
	if len(parts) > 4 {
		parts = parts[:4]
	}
	return strings.Join(parts, "-")
}

func (s *BacklogService) SpawnSessionFromItem(
	ctx context.Context,
	req *connect.Request[sessionv1.SpawnSessionFromItemRequest],
) (*connect.Response[sessionv1.SpawnSessionFromItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. If force=true, clear any in-flight sessions and reset status so the normal
	// path below can proceed. Handles both in_progress (stop work session) and review
	// (stop review session + transition back to in_progress so restart begins from
	// the work phase where the git worktree and slash commands are set up).
	if req.Msg.Force && (item.Status == string(session.BacklogStatusInProgress) ||
		item.Status == string(session.BacklogStatusReview)) {
		var forceErr error
		item, forceErr = s.forceResetItem(ctx, item)
		if forceErr != nil {
			return nil, forceErr
		}
	}

	// 3. Validate status. Allow ready (first spawn) or in_progress (re-spawn after reopen).
	isReopen := item.Status == string(session.BacklogStatusInProgress)
	if item.Status != string(session.BacklogStatusReady) && !isReopen {
		log.InfoLog.Printf("[SpawnSessionFromItem] status gate blocked spawn item=%s status=%s autonomous=%v", item.ID, item.Status, req.Msg.Autonomous)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q or %q status to spawn a session, got %q — use TriggerTriage to advance from %q",
				session.BacklogStatusReady, session.BacklogStatusInProgress, item.Status, item.Status))
	}

	// 3b. WIP limit gate (only for fresh spawns; a reopen doesn't add a new concurrent
	// item — it's already counted as in_progress). Not bypassed by Autonomous: the
	// point is to cap total concurrent agent load regardless of how a spawn was
	// triggered.
	if !isReopen {
		inProgress, wipErr := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
			Statuses: []string{string(session.BacklogStatusInProgress)},
		})
		if wipErr != nil {
			log.WarningLog.Printf("[SpawnSessionFromItem] WIP count query failed item=%s: %v; allowing spawn", item.ID, wipErr)
		} else if len(inProgress) >= maxConcurrentBacklogWorkItems {
			log.InfoLog.Printf("[SpawnSessionFromItem] WIP limit blocked spawn item=%s in_progress=%d cap=%d", item.ID, len(inProgress), maxConcurrentBacklogWorkItems)
			return nil, connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("%d backlog items are already in progress (cap %d) — wait for one to finish or review/ship it first",
					len(inProgress), maxConcurrentBacklogWorkItems))
		}
	}

	// 4. Planning gate (only for fresh spawns; on reopen planning is already approved).
	// Autonomous mode bypasses the gate — the driver handles its own planning loop.
	if !isReopen && !item.SkipPlanning && !item.PlanApproved && !req.Msg.Autonomous {
		log.InfoLog.Printf("[SpawnSessionFromItem] planning gate blocked spawn item=%s status=%s autonomous=false", item.ID, item.Status)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("run TriggerTriage and approve the plan before spawning, or use 'Run Autonomously' to skip the planning gate"))
	}

	// 5. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before spawning a session"))
	}

	// 6. Require SessionCreator before doing any DB writes.
	// degraded: sessionCreator unavailable — return CodeUnimplemented so callers can detect the gap.
	if s.sessionCreator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}

	// 7. Snapshot current AC.
	acSnapshot := item.AcceptanceCriteria

	// 8. Load prior sessions for context.
	priorSessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.WarningLog.Printf("[SpawnSessionFromItem] failed to load prior sessions for item %s: %v", item.ID, err)
		priorSessions = nil
	}

	// 8b. Guard against spawning a duplicate work session when one is already active.
	if hasActiveWorkSession(priorSessions) {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a work session is already active for this item; wait for it to finish or kill it first"))
	}

	// 8. Build agent prompt.
	prompt := session.BuildTokenBudgetedPrompt(item, priorSessions)

	// 9. Generate session title.
	// On reopen, append a revision number (r2, r3…) based on how many work sessions
	// already exist so the session list shows distinct, human-readable names.
	repoName := slugify(filepath.Base(item.RepoPath))
	baseTitle := repoName + "-" + triageShortTitle(priorSessions, item.Title)
	title := buildRevisionTitle(baseTitle, isReopen, priorSessions)

	// 10. Create a dedicated git worktree for this work session. Falls back to a plain
	// directory session if the repo is not git-managed (or worktree creation fails for
	// any other reason — e.g. a bare clone, a detached HEAD, or disk quota hit).
	// Files must be written to the session path BEFORE spawning.
	// worktreeMu guards concurrent spawns from interleaving writes to the same path.
	worktreePath, useWorktree, resolveErr := resolveSessionPath(item.RepoPath, slugify(title))
	if resolveErr != nil {
		return nil, resolveErr
	}

	if wErr := s.writeSessionFiles(item, priorSessions, worktreePath); wErr != nil {
		return nil, wErr
	}

	// 11. Spawn session first so we have the real UUID before creating the ItemSession record.
	spawnTags := []string{session.TagBacklogWork}
	if isReopen {
		spawnTags = append(spawnTags, session.TagBacklogRevision)
	}
	if req.Msg.Autonomous {
		spawnTags = append(spawnTags, session.TagAutonomous)
	}
	var inst *session.Instance
	if useWorktree {
		inst, err = s.sessionCreator.CreateWorktreeSession(ctx, title, item.RepoPath, worktreePath, prompt,
			spawnTags, false, false)
	} else {
		inst, err = s.sessionCreator.CreateDirectorySession(ctx, title, worktreePath, prompt,
			spawnTags, false, false)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn session: %w", err))
	}

	// Persist the instance (and its Worktree row, with BaseCommitSha) synchronously now
	// rather than waiting for the next periodic SaveInstances sweep. The review gate looks
	// up worktree data by session UUID as soon as request_review fires from inside the
	// spawned session; without this, a fast work session can request review before the
	// worktree row exists, causing the review gate to fall back to an unreliable diff.
	if saveErr := s.storage.SaveInstances([]*session.Instance{inst}); saveErr != nil {
		log.WarningLog.Printf("[SpawnSessionFromItem] failed to persist instance immediately after spawn item=%s session=%s: %v", item.ID, inst.UUID, saveErr)
	}

	if req.Msg.Autonomous {
		if s.autonomousStarter != nil {
			log.InfoLog.Printf("[SpawnSessionFromItem] starting autonomous driver item=%s session=%s", item.ID, inst.UUID)
			s.autonomousStarter.StartAutonomousDriverForInstance(inst)
		} else {
			log.WarningLog.Printf("[SpawnSessionFromItem] autonomous=true but no driver starter wired item=%s session=%s — session will need manual approval", item.ID, inst.UUID)
		}
	}

	// 12. Create ItemSession with the real session UUID (avoids "<pending>" orphan records on failure).
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
		AcSnapshot:  acSnapshot,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item session: %w", err))
	}

	// 12b. Capture the pre-work HEAD SHA so the review gate can diff base..HEAD across
	// all commits the agent makes (not just HEAD~1..HEAD at review time).
	if baseSHA, shaErr := session.GetGitHeadSHA(worktreePath); shaErr == nil && baseSHA != "" {
		_ = s.storage.UpdateItemSessionGitActivity(ctx, is.ID, baseSHA, "", time.Now(), 0)
		inst.SetDirBaseSHA(baseSHA)
	}

	// 12c. On reopen, clean up git worktrees from prior work sessions now that the
	// new session is safely persisted. Best-effort only — errors are logged, not returned.
	if isReopen {
		s.cleanupItemWorktrees(ctx, priorSessions)
	}

	// 13. Transition item to in_progress (no-op if already in_progress on reopen).
	if !isReopen {
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil); transErr != nil {
			log.ErrorLog.Printf("[SpawnSessionFromItem] failed to transition item to in_progress: %v", transErr)
		}
	}

	return connect.NewResponse(&sessionv1.SpawnSessionFromItemResponse{
		SessionUuid: inst.UUID,
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// forceResetItem stops any in-flight work or review sessions for the item, and — if
// the item is currently in review — transitions it back to in_progress. Used when
// SpawnSessionFromItem is called with Force=true so the caller can re-spawn cleanly.
func (s *BacklogService) forceResetItem(ctx context.Context, item *session.BacklogItemData) (*session.BacklogItemData, error) {
	earlyPrior, _ := s.storage.ListItemSessions(ctx, item.ID)
	for _, ps := range earlyPrior {
		if ps.EndedAt != nil {
			continue
		}
		if ps.Role != string(session.SessionRoleWork) && ps.Role != string(session.SessionRoleReview) {
			continue
		}
		if s.sessionStopper != nil {
			_ = s.sessionStopper.StopSessionByUUID(ctx, ps.SessionUUID)
		}
		_ = s.storage.UpdateItemSessionEnded(ctx, ps.ID, time.Now())
	}
	if item.Status == string(session.BacklogStatusReview) {
		updated, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil)
		if transErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reset item to in_progress for restart: %w", transErr))
		}
		return updated, nil
	}
	return item, nil
}

// hasActiveWorkSession reports whether any of the provided ItemSessions is an
// open (not yet ended) work-role session.
func hasActiveWorkSession(priorSessions []session.ItemSessionSummary) bool {
	for _, ps := range priorSessions {
		if ps.Role == session.SessionRoleWork && ps.EndedAt == nil {
			return true
		}
	}
	return false
}

// buildRevisionTitle returns the session title for a backlog work session. On reopen
// (isReopen=true) it appends "-rN" where N is one past the existing work-session count.
func buildRevisionTitle(baseTitle string, isReopen bool, priorSessions []session.ItemSessionSummary) string {
	if !isReopen {
		return baseTitle
	}
	workCount := 0
	for _, s := range priorSessions {
		if s.Role == string(session.SessionRoleWork) {
			workCount++
		}
	}
	return fmt.Sprintf("%s-r%d", baseTitle, workCount+1)
}

// resolveSessionPath determines the file-system path for a new work session.
// It first tries to create a git worktree; if that fails it falls back to a plain
// directory. Returns the resolved path, whether a worktree was used, and any error.
func resolveSessionPath(repoPath, slug string) (worktreePath string, useWorktree bool, err error) {
	wt, wtErr := session.CreateBacklogWorktree(repoPath, slug)
	if wtErr == nil {
		return wt, true, nil
	}
	log.WarningLog.Printf("[SpawnSessionFromItem] worktree creation failed (%v), falling back to directory mode", wtErr)
	dirPath, pathErr := session.ResolveSessionPath(repoPath)
	if pathErr != nil {
		return "", false, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid repo_path: %w", pathErr))
	}
	if dirErr := session.EnsureDirectorySessionPath(dirPath); dirErr != nil {
		return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to prepare session directory: %w", dirErr))
	}
	return dirPath, false, nil
}

// writeSessionFiles writes the backlog slash-command files and context file to the session
// directory. The write is serialized under worktreeMu to prevent concurrent write races.
func (s *BacklogService) writeSessionFiles(item *session.BacklogItemData, priorSessions []session.ItemSessionSummary, worktreePath string) error {
	s.worktreeMu.Lock()
	defer s.worktreeMu.Unlock()
	if wErr := session.WriteSlashCommands(item, worktreePath); wErr != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("WriteSlashCommands: %w", wErr))
	}
	if wErr := session.WriteBacklogContextFile(item, priorSessions, worktreePath); wErr != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("WriteBacklogContextFile: %w", wErr))
	}
	return nil
}

// AutoReopenAfterFailedReview implements session.AutoReopenSpawner.
// It transitions the item from review back to in_progress and spawns a new
// work session so the review→rework cycle runs without manual intervention.
func (s *BacklogService) AutoReopenAfterFailedReview(ctx context.Context, itemID string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	// Load item to check current status and obtain updated_at for the precondition.
	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}

	// Iteration cap: count prior work sessions so we don't spin forever on a
	// persistent FAIL verdict. Fail-safe: if the DB query errors we cannot know
	// the true count, so we bail rather than risk an unbounded loop.
	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions for cap check: %w", sessErr)
	}
	workCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork {
			workCount++
		}
	}
	if workCount >= maxAutoReworkIterations {
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s has %d work sessions (cap %d); leaving in review for manual action", itemID, workCount, maxAutoReworkIterations)
		return nil
	}

	// Transition review → in_progress with a precondition to guard against races
	// (e.g. concurrent manual reopen firing at the same time).
	updatedAt := item.UpdatedAt
	precondition := &session.BacklogItemPrecondition{
		ExpectedStatus:    string(session.BacklogStatusReview),
		ExpectedUpdatedAt: &updatedAt,
		Note:              "auto-reopened after failed review verdict",
	}
	if _, err := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusInProgress, precondition); err != nil {
		return fmt.Errorf("transition to in_progress: %w", err)
	}

	_, spawnErr := s.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	if spawnErr != nil {
		// Roll back: item should stay in review rather than stranded in in_progress
		// with no active session. ReconcileStuckItems is an eventual fallback, but
		// an explicit rollback provides faster recovery.
		if _, rollbackErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusReview, nil); rollbackErr != nil {
			log.ErrorLog.Printf("[AutoReopenAfterFailedReview] rollback to review failed for item %s: %v", itemID, rollbackErr)
		}
		return fmt.Errorf("spawn session: %w", spawnErr)
	}
	return nil
}

// AutoReopenForPRFix implements session.PRFixSpawner. It transitions the item
// from pr_pending back to in_progress and spawns a new autonomous work session
// pre-loaded with the CI/review failure context so the agent can fix and push.
func (s *BacklogService) AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if session.BacklogStatus(item.Status) != session.BacklogStatusPRPending {
		return fmt.Errorf("item %s is not pr_pending (got %s)", itemID, item.Status)
	}

	// Reuse the same iteration cap as the review rework cycle.
	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions for cap check: %w", sessErr)
	}
	workCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork {
			workCount++
		}
	}
	if workCount >= maxAutoReworkIterations {
		log.InfoLog.Printf("[AutoReopenForPRFix] item %s has %d work sessions (cap %d); leaving in pr_pending for manual action", itemID, workCount, maxAutoReworkIterations)
		return nil
	}

	updatedAt := item.UpdatedAt
	precondition := &session.BacklogItemPrecondition{
		ExpectedStatus:    string(session.BacklogStatusPRPending),
		ExpectedUpdatedAt: &updatedAt,
		Note:              "auto-reopened for PR fix (CI/review)",
	}
	if _, err := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusInProgress, precondition); err != nil {
		return fmt.Errorf("transition to in_progress: %w", err)
	}

	// Prepend the PR failure context to the item's notes so the spawned session
	// prompt includes it. Restore original notes after spawning.
	originalNotes := item.Notes
	prFixNote := fmt.Sprintf("[PR Fix context - PR #%d (%s)]\n%s", item.PrNumber, item.PrURL, fixContext)
	combinedNotes := prFixNote
	if originalNotes != "" {
		combinedNotes = prFixNote + "\n\n---\n\n" + originalNotes
	}
	if _, noteErr := s.storage.UpdateBacklogItem(ctx, itemID, session.BacklogItemUpdate{
		Notes: &combinedNotes,
	}, nil); noteErr != nil {
		log.WarningLog.Printf("[AutoReopenForPRFix] set fix notes item=%s: %v", itemID, noteErr)
	}

	_, spawnErr := s.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))

	// Restore original notes regardless of spawn outcome.
	if _, noteErr := s.storage.UpdateBacklogItem(ctx, itemID, session.BacklogItemUpdate{
		Notes: &originalNotes,
	}, nil); noteErr != nil {
		log.WarningLog.Printf("[AutoReopenForPRFix] restore notes item=%s: %v", itemID, noteErr)
	}

	if spawnErr != nil {
		// Roll back to pr_pending so the reconciler can retry.
		if _, rollbackErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusPRPending, nil); rollbackErr != nil {
			log.ErrorLog.Printf("[AutoReopenForPRFix] rollback to pr_pending failed for item %s: %v", itemID, rollbackErr)
		}
		return fmt.Errorf("spawn session: %w", spawnErr)
	}

	log.InfoLog.Printf("[AutoReopenForPRFix] item %s → in_progress for PR fix session", itemID)
	return nil
}

// TriggerTriage kicks off a headless triage planning call for a backlog item.
// Returns immediately after creating an ItemSession; actual triage runs in a goroutine.
// +api: backlog:trigger-triage
func (s *BacklogService) TriggerTriage(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerTriageRequest],
) (*connect.Response[sessionv1.TriggerTriageResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Status guard — triage is only valid for idea or ready items.
	if item.Status != string(session.BacklogStatusIdea) && item.Status != string(session.BacklogStatusReady) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q or %q status to trigger triage, got %q",
				session.BacklogStatusIdea, session.BacklogStatusReady, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering triage"))
	}

	// 3a. Orphan-aware guard: if an open triage session exists, check whether it is
	// genuinely still running. Headless sessions are always orphaned if not ended
	// (no live tmux session to check) — tombstone them and allow re-trigger.
	existingSessions, listErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if listErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list triage sessions: %w", listErr))
	}
	if err := s.tombstoneOrphanTriageSessions(ctx, req.Msg.ItemId, item.Status, existingSessions); err != nil {
		return nil, err
	}

	// 3b. If re-triggering on a "ready" item, move it back to "idea".
	// Use a precondition so a concurrent work-session spawn (ready→in_progress) that
	// races with this re-triage doesn't drag the item backwards to idea.
	if item.Status == string(session.BacklogStatusReady) {
		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReady)}
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId,
			session.BacklogStatusIdea, precondition); transErr != nil {
			log.WarningLog.Printf("[TriggerTriage] item %s moved past ready before triage reset (race with work-session spawn); aborting re-triage", req.Msg.ItemId)
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("item %s was already moved past ready — a work session may have just started; retry after it completes", req.Msg.ItemId))
		}
	}

	// 3c. Feedback-driven refine: find the most recent completed triage result to
	// revise. Refining requires one to exist — feedback on an item with no completed
	// triage falls back to a confusing fresh run, so reject explicitly instead.
	feedback := strings.TrimSpace(req.Msg.Feedback)
	priorResult, havePrior := findPriorTriageResult(existingSessions)
	if feedback != "" && !havePrior {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no completed triage result to refine for item %s — trigger initial triage first", req.Msg.ItemId))
	}
	nextIteration := priorResult.Iteration + 1

	// 4. Build artifact dir path under ~/.stapler-squad/triage-artifacts/<item-id>/
	//    so triage workers don't write into the item's git repo.
	triageBase, triageBaseErr := s.cfg.TriageArtifactDirOrDefault()
	if triageBaseErr != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to resolve triage artifact dir: %w", triageBaseErr))
	}
	artifactAbsPath := filepath.Join(triageBase, item.ID)

	// 5. Create artifact dir.
	if mkErr := os.MkdirAll(artifactAbsPath, 0o755); mkErr != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create artifact dir %s: %w", artifactAbsPath, mkErr))
	}

	// 6. Require headless pool.
	if s.headlessPool == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("headless pool not available — ensure claude binary is installed"))
	}

	// 7. Build triage prompt — a fresh triage, or a feedback-driven refine of the
	// most recent completed result.
	var triagePrompt string
	if feedback != "" {
		triagePrompt = session.BuildHeadlessRetriagePrompt(item, artifactAbsPath, priorResult, feedback)
	} else {
		triagePrompt = session.BuildHeadlessTriagePrompt(item, artifactAbsPath)
	}

	// 8. Create ItemSession synchronously before goroutine (prevents TOCTOU on orphan guard).
	triageSessionUUID := headlessTriageUUIDPrefix + uuid.New().String()
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: triageSessionUUID,
		SessionRole: session.SessionRoleTriage,
		AcSnapshot:  item.AcceptanceCriteria,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create triage item session: %w", err))
	}

	log.InfoLog.Printf("[TriggerTriage] headless triage started item=%s session=%s path=%s", item.ID, triageSessionUUID, artifactAbsPath)

	// 9. Drive triage asynchronously so the RPC returns immediately.
	itemID := item.ID
	itemRepoPath := item.RepoPath
	isID := is.ID
	iteration := nextIteration
	go func() {
		// Acquire concurrency semaphore (max 8 concurrent triage calls).
		select {
		case s.triageSem <- struct{}{}:
		case <-s.shutdownCtx.Done():
			// cleanupCtx is a separate context for DB writes that must complete even
			// after shutdownCtx is cancelled. Passing shutdownCtx here would cause the
			// write to fail immediately with context.Canceled.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}
		defer func() { <-s.triageSem }()

		triageCtx, cancel := context.WithTimeout(s.shutdownCtx, 30*time.Minute)
		defer cancel()

		raw, callErr := s.headlessPool.CallBlockingWithOptions(triageCtx,
			headless.FeatureKeyTriage,
			headless.HeadlessTriageSystemPrompt(),
			triagePrompt,
			headless.CallOptions{WorkDir: itemRepoPath},
		)

		// cleanupCtx outlives shutdownCtx so DB writes succeed even during graceful
		// shutdown. Created HERE, after CallBlockingWithOptions returns, not before
		// it: the LLM call above routinely takes 7-15 minutes (4 parallel research
		// subagents), so a cleanupCtx created before it would have its 10s budget
		// already expired by the time these persistence calls run below — every
		// successful triage would silently fail to ever mark the item ready. This
		// was a live, 100%-reproducible bug: see the backlog cross-platform audit.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), s.triageCleanupTimeout)
		defer cleanupCancel()

		if callErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] headless triage failed item=%s: %v", itemID, callErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}

		result, parseErr := session.ParseHeadlessTriageResult(raw)
		if parseErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] parse result failed item=%s: %v", itemID, parseErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}
		result.Iteration = iteration
		result.Feedback = feedback

		payloadJSON, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] marshal triage result item=%s: %v", itemID, marshalErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}
		if updateErr := s.storage.UpdateItemSessionTriageResult(cleanupCtx, isID, string(payloadJSON)); updateErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] persist triage result item=%s: %v", itemID, updateErr)
		}

		pap := artifactAbsPath
		update := session.BacklogItemUpdate{PlanArtifactsPath: &pap}
		applyTriageACToUpdate(&result, &update)
		if _, updateErr := s.storage.UpdateBacklogItem(cleanupCtx, itemID, update, nil); updateErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] update plan_artifacts_path item=%s: %v", itemID, updateErr)
		}

		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusIdea)}
		if _, transErr := s.storage.TransitionBacklogItemStatus(cleanupCtx, itemID,
			session.BacklogStatusReady, precondition); transErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] status transition idea→ready item=%s: %v", itemID, transErr)
		}

		_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
		log.InfoLog.Printf("[TriggerTriage] headless triage complete item=%s suggestions=%d tasks=%d",
			itemID, len(result.Suggestions), len(result.Tasks))
	}()

	return connect.NewResponse(&sessionv1.TriggerTriageResponse{
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// CancelTriage stops a running triage session for a backlog item.
// +api: backlog:cancel-triage
func (s *BacklogService) CancelTriage(
	ctx context.Context,
	req *connect.Request[sessionv1.CancelTriageRequest],
) (*connect.Response[sessionv1.CancelTriageResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	existingSessions, err := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sessions: %w", err))
	}

	cancelled := false
	now := time.Now()
	for _, is := range existingSessions {
		if is.Role != string(session.SessionRoleTriage) || is.EndedAt != nil {
			continue
		}
		if s.sessionStopper != nil {
			_ = s.sessionStopper.StopSessionByUUID(ctx, is.SessionUUID)
		}
		_ = s.storage.UpdateItemSessionEnded(ctx, is.ID, now)
		cancelled = true
	}

	return connect.NewResponse(&sessionv1.CancelTriageResponse{Cancelled: cancelled}), nil
}

// TriggerReReview re-runs the review gate for a backlog item.
// +api: backlog:trigger-re-review
func (s *BacklogService) TriggerReReview(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerReReviewRequest],
) (*connect.Response[sessionv1.TriggerReReviewResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Validate item is in review status.
	if item.Status != string(session.BacklogStatusReview) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q status to re-trigger review, got %q", session.BacklogStatusReview, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering re-review"))
	}

	// 4. Find the most recent review and work ItemSessions for this item.
	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}

	mostRecentReviewSession, mostRecentWorkSession := findMostRecentSessions(sessions)

	// 5. Note: We don't need to delete the old verdict; a new one will overwrite it when the re-review
	// session submits its findings via the MCP tool.

	// 6. Get git diff from the most recent work session's worktree using its base SHA.
	// Fall back to item.RepoPath / HEAD~1 only for directory-mode sessions.
	workSessionDiff := s.getWorkSessionDiff(ctx, item.RepoPath, mostRecentWorkSession)

	// 7. Deserialize AC snapshot (from most recent work session or item AC).
	acSnapshot := resolveACSnapshot(mostRecentWorkSession, item.AcceptanceCriteria)

	// 8. Build re-review prompt.
	acSnapshotJSON, _ := json.Marshal(acSnapshot)

	priorVerdictSection := ""
	if mostRecentReviewSession != nil && mostRecentReviewSession.ReviewVerdict != nil {
		rv := mostRecentReviewSession.ReviewVerdict
		priorVerdictSection = fmt.Sprintf("\n## Prior Review Verdict\nOutcome: %s\nSummary: %s\n", rv.OverallOutcome, rv.Summary)
	}

	reReviewPrompt := fmt.Sprintf(`You are re-reviewing a backlog item that previously entered the review state.

# Item: %s

## Description
%s
%s
## Acceptance Criteria (at time of work session)
`, item.Title, item.Description, priorVerdictSection)

	for _, ac := range acSnapshot {
		reReviewPrompt += fmt.Sprintf("%d. %s (status: %s)\n", ac.Index, ac.Text, ac.Status)
	}

	reReviewPrompt += fmt.Sprintf(`
## Recent Changes
The work session made the following changes to the codebase:

%s

## Your Task
Perform a comprehensive review and submit your verdict using the submit_review_verdict MCP tool:
- Assess each acceptance criterion listed above
- Evaluate the diff against the requirements
- For each criterion provide: criterion_index, outcome (PASS/FAIL/PARTIAL), evidence

Call submit_review_verdict with:
  item_id: "%s"
  summary: "<overall summary of your findings>"
  verdicts: [{"criterion_index": N, "outcome": "PASS|FAIL|PARTIAL", "evidence": "<specific evidence>"}]

Do not modify the code. Only write the review verdict.
`, session.SanitizeDiff(workSessionDiff), item.ID)

	// 9. Headless path — preferred when a headless pool is configured.
	// This avoids needing tmux and runs the review inline via LLM call.
	if s.headlessPool != nil {
		headlessPrompt := session.BuildHeadlessReviewPrompt(item, acSnapshot, workSessionDiff, false)
		reviewCtx, reviewCancel := context.WithTimeout(ctx, headless.DefaultCallTimeout)
		defer reviewCancel()

		reviewResult, callErr := s.headlessPool.CallBlockingWithOptions(
			reviewCtx, headless.FeatureKeyReview, headless.HeadlessReviewSystemPrompt(), headlessPrompt, headless.CallOptions{},
		)
		if callErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("headless re-review call failed: %w", callErr))
		}

		overall, perCriterion, reviewSummary := session.ParseHeadlessVerdictResult(reviewResult)
		perCriterionJSON, _ := json.Marshal(perCriterion)

		reviewSessionUUID := headlessReReviewUUIDPrefix + uuid.New().String()
		is, createErr := s.storage.CreateItemSessionWithVerdict(ctx, session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: reviewSessionUUID,
			SessionRole: session.SessionRoleReview,
			AcSnapshot:  session.AcCriteriaJSON(acSnapshotJSON),
		}, session.ReviewVerdictData{
			OverallOutcome: overall,
			PerCriterion:   string(perCriterionJSON),
			Summary:        reviewSummary,
		})
		if createErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review verdict: %w", createErr))
		}
		if endErr := s.storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()); endErr != nil {
			log.WarningLog.Printf("[TriggerReReview] UpdateItemSessionEnded: %v", endErr)
		}

		log.InfoLog.Printf("[TriggerReReview] headless re-review complete for item %s (outcome %s)", item.ID, overall)

		return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
			ItemSession: itemSessionToProto(is, s.buildCostLookup()),
		}), nil
	}

	// 10. Spawn re-review session — AutonomousDriver mode if available, oneShot fallback.
	if s.sessionCreator == nil {
		log.InfoLog.Printf("[TriggerReReview] triggered for item %s but no SessionCreator available", item.ID)
		return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
			ItemSession: &sessionv1.ItemSession{
				Id:          item.ID,
				SessionRole: "re-review-triggered",
			},
		}), nil
	}

	slug := slugify(item.Title)
	title := "re-review:" + slug
	useAutonomous := s.autonomousStarter != nil

	// Kill any stale tmux session with this title so the new session gets a fresh
	// pane and the autonomous driver can deliver its prompt without attaching to an
	// old, idle session that was left behind from a previous (possibly crashed) attempt.
	if s.sessionStopper != nil {
		_ = s.sessionStopper.KillTmuxSessionByTitle(ctx, title)
	}

	inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
		[]string{"backlog:review"}, !useAutonomous /*oneShot*/, true /*hidden*/)
	if spawnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
	}
	if useAutonomous {
		s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
	}

	// 11. Create ItemSession with role=review.
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleReview,
		AcSnapshot:  session.AcCriteriaJSON(acSnapshotJSON),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create re-review item session: %w", err))
	}

	// Capture the pre-review HEAD SHA so diffs against base..HEAD work correctly.
	if baseSHA, shaErr := session.GetGitHeadSHA(item.RepoPath); shaErr == nil && baseSHA != "" {
		_ = s.storage.UpdateItemSessionGitActivity(ctx, is.ID, baseSHA, "", time.Now(), 0)
	}

	log.InfoLog.Printf("[TriggerReReview] spawned re-review session %s for item %s", inst.UUID, item.ID)

	return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// tombstoneOrphanTriageSessions marks any open triage ItemSessions that are no longer
// live as ended. Returns CodeAlreadyExists if a live triage session is genuinely running.
func (s *BacklogService) tombstoneOrphanTriageSessions(ctx context.Context, itemID, itemStatus string, sessions []session.ItemSessionSummary) error {
	for _, is := range sessions {
		if is.Role != string(session.SessionRoleTriage) || is.EndedAt != nil {
			continue
		}
		// Headless triage sessions have no live in-memory instance; treat as orphaned.
		// Sessions older than maxTriageSessionAge are also treated as orphaned to prevent
		// a hung or leaked session from blocking re-trigger indefinitely.
		isHeadless := strings.HasPrefix(is.SessionUUID, headlessTriageUUIDPrefix)
		isStale := time.Since(is.CreatedAt) > maxTriageSessionAge
		notLive := isHeadless || isStale || s.sessionStopper == nil || !s.sessionStopper.IsSessionLive(is.SessionUUID)
		statusAdvanced := itemStatus != string(session.BacklogStatusIdea)
		if notLive || statusAdvanced {
			_ = s.storage.UpdateItemSessionEnded(ctx, is.ID, time.Now())
			continue
		}
		return connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("triage session already running for item %s", itemID))
	}
	return nil
}

// findPriorTriageResult returns the most recent successfully-parsed triage result from
// the provided sessions, along with a boolean indicating whether one was found.
func findPriorTriageResult(sessions []session.ItemSessionSummary) (session.HeadlessTriageResult, bool) {
	for i := len(sessions) - 1; i >= 0; i-- {
		is := sessions[i]
		if is.Role != string(session.SessionRoleTriage) || is.TriageResult == "" {
			continue
		}
		var result session.HeadlessTriageResult
		if jsonErr := json.Unmarshal([]byte(is.TriageResult), &result); jsonErr == nil {
			return result, true
		}
	}
	return session.HeadlessTriageResult{}, false
}

// applyTriageACToUpdate re-indexes and status-normalises the AC criteria from a triage
// result, then writes the serialized JSON into the provided update struct.
func applyTriageACToUpdate(result *session.HeadlessTriageResult, update *session.BacklogItemUpdate) {
	if len(result.AcceptanceCriteria) == 0 {
		return
	}
	// Re-index to ensure 0-based contiguous indices regardless of what the model output.
	for i := range result.AcceptanceCriteria {
		result.AcceptanceCriteria[i].Index = i
		if result.AcceptanceCriteria[i].Status == "" {
			result.AcceptanceCriteria[i].Status = "pending"
		}
	}
	if acJSON, marshalErr := session.SerializeAcCriteria(result.AcceptanceCriteria); marshalErr == nil {
		update.AcceptanceCriteria = &acJSON
	}
}

// findMostRecentSessions returns the most recently created review and work ItemSessions
// from the provided list. Either return value may be nil if no session of that role exists.
func findMostRecentSessions(sessions []session.ItemSessionSummary) (reviewSession, workSession *session.ItemSessionSummary) {
	for i := range sessions {
		is := &sessions[i]
		switch is.Role {
		case session.SessionRoleReview:
			if reviewSession == nil || is.CreatedAt.After(reviewSession.CreatedAt) {
				reviewSession = is
			}
		case session.SessionRoleWork:
			if workSession == nil || is.CreatedAt.After(workSession.CreatedAt) {
				workSession = is
			}
		}
	}
	return
}

// getWorkSessionDiff returns the git diff for the given work session. It prefers the
// session's dedicated worktree path and base SHA; falls back to the item's repo when
// the worktree directory is gone (commits remain accessible via the shared object store).
func (s *BacklogService) getWorkSessionDiff(ctx context.Context, repoPath string, workSession *session.ItemSessionSummary) string {
	if workSession == nil {
		return ""
	}
	diffDir := repoPath
	diffBaseSHA := ""
	diffHeadRef := ""
	wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, workSession.SessionUUID)
	if wtErr == nil && wt.WorktreePath != "" {
		// Try the dedicated worktree first.
		diff, _, diffErr := session.GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
		if diffErr == nil {
			return diff
		}
		// Worktree path is gone — fall through to repo fallback using the same base SHA
		// and an explicit branch ref: repoPath's own checked-out HEAD is not the work
		// branch's tip, so diffing against implicit HEAD would compare against whatever
		// the shared main checkout happens to have, not the agent's actual work.
		log.WarningLog.Printf("[TriggerReReview] GetGitDiff in worktree failed (path gone?): %v; falling back to repo", diffErr)
		diffBaseSHA = wt.BaseCommitSHA
		diffHeadRef = wt.BranchName
	}
	// Fallback: diff in the main repo between base and last commit. Git worktrees
	// share the object store, so commits from any worktree are reachable here.
	if diffBaseSHA == "" && workSession.LastCommitSha != "" {
		diffBaseSHA = workSession.LastCommitSha
	}
	diff, _, diffErr := session.GetGitDiffRef(ctx, diffDir, diffBaseSHA, diffHeadRef)
	if diffErr != nil {
		log.WarningLog.Printf("[TriggerReReview] GetGitDiff fallback in %s failed: %v", diffDir, diffErr)
		return ""
	}
	return diff
}

// resolveACSnapshot returns the acceptance criteria to use for a re-review. It prefers
// the snapshot captured at work-session start; falls back to the item's current AC.
func resolveACSnapshot(workSession *session.ItemSessionSummary, itemAC session.AcCriteriaJSON) []session.AcCriterion {
	if workSession != nil && workSession.AcSnapshot != "" {
		if ac, _ := session.ParseAcCriteria(workSession.AcSnapshot); len(ac) > 0 {
			return ac
		}
	}
	ac, _ := session.ParseAcCriteria(itemAC)
	return ac
}
