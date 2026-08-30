package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
)

// PRFixSpawner can reopen a pr_pending item for rework when CI checks fail or
// reviewers request changes. The fixContext string contains a summary of the
// failures/comments to pass as context to the new work session.
type PRFixSpawner interface {
	AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error
	// HasActiveWorkSession resolves itemID's current active work session, if
	// any (nil if none). Side-effect-free. Returning the resolved session
	// itself — not just a bool — lets remediatePRFixWithBackoffGate pass it
	// into AutoReopenForPRFixWithKnownSession so the backoff-bypass decision
	// and the steer-vs-spawn decision share one observation of mutable
	// state instead of two independent reads (TOCTOU: the session could end
	// between two separate queries).
	HasActiveWorkSession(ctx context.Context, itemID string) (*ItemSessionSummary, error)
	// AutoReopenForPRFixWithKnownSession behaves like AutoReopenForPRFix,
	// except knownActive (from a prior HasActiveWorkSession call in the same
	// tick) is trusted as a floor on activity: if a fresh internal check no
	// longer finds an active session, knownActive still prevents the spawn
	// branch from firing.
	AutoReopenForPRFixWithKnownSession(ctx context.Context, itemID, fixContext string, knownActive *ItemSessionSummary) error
}

// OneShotShipRunner runs a one-shot LLM prompt against a session's worktree,
// returning the PR URL the prompt produced (or "" if none was found in its
// output). Defined here — the consumer — per this repo's anti-interface-
// pollution convention (the `interface-pollution-checklist` skill);
// *services.SessionService satisfies it via RunOneShotForSession, wired in
// production via SetOneShotShipRunner from server/dependencies.go. Mirrors
// services.PRRunner (server/services/backlog_service_ship.go), which the same
// method also satisfies for the manual "Ship PR" self-service action —
// intentionally not shared/exported from that package, since importing it
// here would pull server/services (which imports this package) into an
// import cycle.
//
// Used by shipViaAgentOrFallback to close the gap flagged in PR #189's
// "deliberately out of scope" section: when the work session that earned a
// PASS verdict has already exited, the only PR-creation mechanism available
// was pushAndCreatePR's mechanical `git push` + `gh pr create` — no CI
// reaction, no merge-conflict resolution. RunOneShotForSession lets us run
// the same agent-driven ship flow a still-live session would have run itself
// (see /backlog/ship's ship.md, which drives /github:pr-ship) as a headless
// one-shot against the ended session's worktree — it only needs the
// session's Instance/worktree to still be resolvable, not a live tmux
// process (see RunOneShot's use of findInstance + GetEffectiveRootDir).
type OneShotShipRunner interface {
	RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error)
}

// agentShipPrompt is the one-shot prompt used by shipViaAgentOrFallback.
// Deliberately NOT the same literal as services.shipPRPrompt
// (server/services/backlog_service_ship.go) — that prompt is a plain-English
// "create a PR" ask with no conflict-resolution or CI-reaction instructions,
// fine for its own use case (a human clicking "Ship PR" on a review-status
// item that hasn't necessarily finished /backlog/review's protocol). Here we
// are resuming exactly the step a still-live work session would have taken
// next per taskProtocolBlock rules 8-9 (PASS -> run /backlog/ship), so we
// invoke that same slash command directly: WriteSlashCommands
// (session/backlog_commands.go) already wrote ship.md into this worktree at
// session-spawn time, and nothing cleans it up before the item leaves
// "review" (CleanupSlashCommands is not wired to fire on review exit — see
// its call sites), so it is still present. ship.md's own instructions run
// /github:pr-ship (local CI, code review, remote CI, and actual
// merge-conflict resolution — the whole reason this path was added) and
// already special-case "review already returned PASS" by skipping the
// redundant re-review step, exactly matching the state we call this in.
const agentShipPrompt = "/backlog/ship"

// oneShotShipTimeoutSeconds bounds shipViaAgentOrFallback's one-shot call.
// Set to the RunOneShot handler's own hard ceiling (server/services/
// session_service.go clamps TimeoutSeconds to max 1800s) rather than
// TriggerShipPR's shorter 900s: /github:pr-ship does more work than
// services.shipPRPrompt's plain PR-creation ask (it also waits on CI and
// resolves merge conflicts), so it needs more headroom, and this path runs
// unattended — there's no human waiting on an RPC response to bound it.
const oneShotShipTimeoutSeconds = 1800

// prPendingChecker is the subset of GitWorktree's PR-status behavior that
// ReconcilePRPending depends on. Defined here (the consumer) rather than in
// package git, scoped to exactly what's called.
type prPendingChecker interface {
	IsPRMerged(prNumber int) (bool, error)
	GetPRStatus(prNumber int) (*git.PRStatus, error)
	ClosePR(prNumber int, comment string) error
}

// prCreator is the subset of GitWorktree's push/PR-creation behavior that
// pushAndCreatePR depends on. Defined here (the consumer), scoped to exactly
// what's called, mirroring prPendingChecker above.
type prCreator interface {
	CommitChanges(commitMessage string) error
	PushBranch() error
	CreatePR(opts git.PRCreateOptions) (prURL string, prNumber int, err error)
	EnablePRAutoMerge(prNumber int) error
	RequestCopilotReview(prNumber int) error
	HasCommitsAheadOfMain(mainBranch string) (bool, error)
}

// defaultPRCreatorFactory constructs the push/PR-creation client for a given
// worktree. This is the production default installed by newListenerBase;
// SetPRCreatorFactory overrides it in tests.
func defaultPRCreatorFactory(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
	return git.NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA)
}

// defaultPRPendingCheckerFactory constructs the PR-status checker for a given
// repo path. This is the production default installed by newListenerBase;
// SetPRPendingCheckerFactory overrides it in tests.
func defaultPRPendingCheckerFactory(repoPath string) prPendingChecker {
	return git.NewGitWorktreeFromStorage(repoPath, repoPath, "", "", "")
}

// defaultOrphanedPRFinder resolves repoPath's GitHub owner/repo from its git
// remote, then looks up an existing PR for branch. Returns github.ErrNoPR
// unchanged when no PR exists — see reconcileOrphanedAgentPRs, which treats
// that as "no match yet", not a failure.
func defaultOrphanedPRFinder(ctx context.Context, repoPath, branch string) (*github.PRInfo, error) {
	ref, err := github.GetOwnerRepoFromRemote(repoPath)
	if err != nil {
		return nil, err
	}
	if !ref.IsValid() {
		return nil, fmt.Errorf("could not resolve a GitHub owner/repo from the git remote at %s", repoPath)
	}
	return github.GetPRForBranch(ctx, ref.Owner(), ref.Repo(), branch)
}

// defaultPRByNumberFinder resolves repoPath's GitHub owner/repo from its git
// remote, then looks up prNumber directly (immutable-number-keyed, not
// branch-name-keyed — see github.GetPRByNumber's doc comment). This is the
// production default installed by newListenerBase for
// verifyPRHeadBranchMatchesTracked's live-GitHub re-check.
func defaultPRByNumberFinder(ctx context.Context, repoPath string, prNumber int) (*github.PRInfo, error) {
	ref, err := github.GetOwnerRepoFromRemote(repoPath)
	if err != nil {
		return nil, err
	}
	if !ref.IsValid() {
		return nil, fmt.Errorf("could not resolve a GitHub owner/repo from the git remote at %s", repoPath)
	}
	return github.GetPRByNumber(ctx, ref.Owner(), ref.Repo(), prNumber)
}

// reconcilePRPendingWithoutPRItems is the pr_pending_no_pr detector (BUG-040):
// flags any item stuck in pr_pending status with no PR reference at all
// (pr_number == 0). This shape is otherwise structurally invisible: every
// downstream reconciler, including ReconcilePRPending itself, is gated by
// FindPRPendingItems' PrNumberGT(0) filter, so an item that reaches pr_pending
// with pr_number still 0 has nothing left in this codebase that will ever
// touch it again. Two write-ordering bugs that produced exactly this shape —
// pushAndCreatePR's best-effort field persist, and ReconcilePRPending's
// closed-PR branch clearing fields before confirming a reopen succeeded —
// were found and fixed alongside this detector; this function is the
// structural backstop so any *future* mistake with the same shape is still
// visible and retryable from /unfinished instead of a silent permanent
// stall. Detection + notification only: there is no known-safe automated
// remediation here (the item's PR history is gone), so unlike most other
// reasons this one has no wired TriggerRemediationNow action — a human has
// to decide whether to push a fresh PR or investigate further. Best-effort:
// query/notify failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcilePRPendingWithoutPRItems(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusPRPending)},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcilePRPendingWithoutPRItems list error: %v", err)
		return
	}

	for _, item := range items {
		if item.PrNumber != 0 {
			continue
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonPRPendingNoPR, BacklogStatusPRPending,
			"item is pr_pending but has no PR reference (pr_number=0) — every downstream reconciler requires PrNumber, so this item is otherwise invisible and permanently stuck")
		if markErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePRPendingWithoutPRItems MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePRPendingWithoutPRItems FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonPRPendingNoPR)
		if !ok || row.NotifiedAt != nil {
			continue
		}

		log.WarningLog().Printf("[BacklogLifecycle] item %s is pr_pending with no PR reference", item.ID)
		l.notify(item.ID,
			"Backlog item stuck: pr_pending with no PR",
			fmt.Sprintf("%s — this item is marked pr_pending but has no PR number or URL on record, so it cannot be polled or auto-recovered. Use /unfinished to retry it manually.", item.Title),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonPRPendingNoPR); notifyErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePRPendingWithoutPRItems MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}
	// No resolve pass needed here: selfHealStuck (status-anchored) clears this
	// reason once the item leaves 'pr_pending' (successfully reopened for a
	// fresh attempt, or manually recovered).
}

// reconcileOrphanedAgentPRs is the Epic 3.2 reconciliation backstop from "PR
// Metadata Capture Fix" (project_plans/backlog-agent-communication): an agent
// driving its own shipping via /backlog:ship -> gh pr create can crash or be
// killed after the PR genuinely exists on GitHub but before it ever calls
// report_pr_created (Epic 3.1, server/mcp/tools_backlog.go) to report it
// back. Without this sweep such an item is invisible forever: it sits in
// review with pr_number==0, and — unlike reconcilePRPendingWithoutPRItems'
// BUG-040 case — there is no dedicated StuckReason for "review, no PR
// recorded, but GitHub actually has one": StuckReasonAbandonedReview already
// covers "review, no PR, no session, genuinely nothing shipped", so this is
// deliberately a backstop that self-heals immediately on a match, not a new
// StuckReason/human-visible flag (see ADR-001 and this project's plan.md,
// Epic 3.2's own scope note).
//
// Deliberately narrow: only items in review status, with no PR reference
// recorded yet (pr_number == 0 — the cheap in-process filter applied before
// any GitHub API call), and no live work/review session (hasActiveSession) —
// an item still being actively worked or reviewed is left alone; its own
// normal flow will eventually report or create the PR. On a match, reuses
// SetBacklogItemPRAndTransition (Epic 3.1's own primary-write path,
// session/storage.go) — no duplicate PR-field-writing logic. On no match (no
// open PR yet for the item's branch), this is an expected no-op, retried
// next tick. Best-effort: query/GitHub failures are logged, never returned —
// same discipline as every other detector in this sweep.
func (l *BacklogLifecycleListener) reconcileOrphanedAgentPRs(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusReview)},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileOrphanedAgentPRs list error: %v", err)
		return
	}

	for _, item := range items {
		if item.PrNumber != 0 || item.RepoPath == "" {
			continue
		}

		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileOrphanedAgentPRs ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		if hasActiveSession(sessions) {
			continue // still legitimately in flight — its own normal flow will report/create the PR
		}

		// ListItemSessions orders ascending by CreatedAt — keep overwriting so
		// this ends up holding the most recent work session, mirroring
		// mostRecentWorkCommitShippedToMain's identical pattern above.
		var lastWorkSessionUUID string
		for _, is := range sessions {
			if is.Role == SessionRoleWork {
				lastWorkSessionUUID = is.SessionUUID
			}
		}
		if lastWorkSessionUUID == "" {
			continue // never had a work session — nothing could have shipped a PR
		}
		wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWorkSessionUUID)
		if wtErr != nil || wt.BranchName == "" {
			continue
		}

		// NOTE: this still looks up by branch name (github.GetPRForBranch via getOrphanedPRFinder), so it has the same blind spot report_pr_created had before the number-keyed fix in tools_github.go's VerifyPRMatchesBranch — a PR opened from a fallback branch is invisible here too. Not fixed here (out of scope per project_plans/report-pr-created-branch-mismatch/requirements.md); a future fast-follow could reuse VerifyPRMatchesBranch/GetPRByNumber's shape.
		info, prErr := l.getOrphanedPRFinder()(ctx, item.RepoPath, wt.BranchName)
		if prErr != nil {
			if !errors.Is(prErr, github.ErrNoPR) {
				log.DebugLog().Printf("[BacklogLifecycle] reconcileOrphanedAgentPRs GetPRForBranch item=%s branch=%s: %v", item.ID, wt.BranchName, prErr)
			}
			continue // no PR yet (or a transient lookup failure) — retried next tick
		}
		if info.State != "open" {
			continue // a closed/merged PR for this branch is handled by other reconcilers, not this backstop
		}

		summary := fmt.Sprintf("Reconciliation backstop: found an existing open PR #%d for this item's branch %q with no report_pr_created call on record.", info.Number, wt.BranchName)
		// nil guard: this reconciler only ever lists review-status items
		// (filter above), so it never hits the reassignment path.
		if setErr := l.storage.SetBacklogItemPRAndTransition(ctx, &item, info.HTMLURL, info.Number, summary, nil); setErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileOrphanedAgentPRs SetBacklogItemPRAndTransition item=%s pr=%d: %v", item.ID, info.Number, setErr)
			continue
		}
		log.InfoLog().Printf("[BacklogLifecycle] reconcileOrphanedAgentPRs item=%s → pr_pending (recovered PR #%d %s, never reported)", item.ID, info.Number, info.HTMLURL)
	}
}

// mostRecentWorkCommitShippedToMain finds itemID's most recent work session
// and reports whether its current tip commit (resolveLatestWorkCommit, NOT
// the session's stale LastCommitSha field — see that function's doc comment)
// has landed on bounceMainBranch. Mirrors BacklogService.isCodeShippedToMain's
// "keep overwriting while scanning ascending-by-CreatedAt" pattern
// (server/services/backlog_service_lifecycle.go) for finding the most recent
// work session, but — unlike that method — deliberately does NOT treat "no
// commit resolvable" as shipped: isCodeShippedToMain's caller uses it as a
// block-a-transition guard, where "nothing to verify" should not block; this
// caller uses it as a fire-a-transition trigger, where "nothing to verify"
// must never fire one. Returns ("", false) when there is no work session or
// no commit could be resolved for it.
func (l *BacklogLifecycleListener) mostRecentWorkCommitShippedToMain(ctx context.Context, itemID, repoPath string) (sha string, shipped bool) {
	itemSessions, err := l.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] mostRecentWorkCommitShippedToMain ListItemSessions item=%s: %v", itemID, err)
		return "", false
	}
	var lastWorkSessionUUID string
	for _, is := range itemSessions {
		// ListItemSessions orders ascending by CreatedAt — keep overwriting so
		// this ends up holding the *most recent* work session.
		if is.Role == SessionRoleWork {
			lastWorkSessionUUID = is.SessionUUID
		}
	}
	if lastWorkSessionUUID == "" {
		return "", false // no work session ever ran — nothing to confirm shipped
	}
	sha = l.resolveLatestWorkCommit(ctx, lastWorkSessionUUID, repoPath)
	if sha == "" {
		return "", false // nothing resolvable — nothing to confirm shipped
	}
	// A freshly spawned worktree's HEAD is, by construction, its own base
	// commit until the agent makes its first commit — and a base commit is
	// always an ancestor of main (that is literally where the branch came
	// from), so the IsCommitOnMain check below would trivially return true
	// here even though zero work has happened yet. Confirmed live 2026-07-22:
	// item e1fb6825, spawned 55 seconds earlier with zero commits, was
	// auto-marked done citing its own base commit as "shipped to main
	// without a PR" — the same false-positive shape resolveLatestWorkCommit's
	// doc comment already fixed for the *stale-field* case (2026-07-21), but
	// not for a live-resolved SHA that happens to equal its own base. Guard
	// explicitly: on a distinct feature branch, sha == base means no new
	// commits exist yet, so there's nothing to have shipped. Scoped to
	// non-main branches only — a work session whose branch IS bounceMainBranch
	// (work committed directly to main, no separate feature branch ever used)
	// legitimately has sha == base == "shipped" by construction; that case
	// must still fall through to the IsCommitOnMain check below unchanged.
	if wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWorkSessionUUID); wtErr == nil &&
		wt.BranchName != bounceMainBranch && wt.BaseCommitSHA != "" && sha == wt.BaseCommitSHA {
		return sha, false // no new commits yet on this branch — nothing to have shipped
	}
	onMain, mainErr := git.IsCommitOnMain(repoPath, bounceMainBranch, sha)
	if mainErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] mostRecentWorkCommitShippedToMain IsCommitOnMain item=%s sha=%s: %v", itemID, sha, mainErr)
		return sha, false
	}
	return sha, onMain
}

// dashboardBaseURLFn resolves the base URL used to build a clickable deep
// link back to a backlog item from a PR body. Defaults to the same
// localhost:8543 address as services.hookBaseURLFn (session can't import
// server/services — that would be an import cycle — so this mirrors that
// package's lazy-base-URL pattern independently); server.go overrides it at
// startup via SetDashboardBaseURLFn with the real bound address.
var dashboardBaseURLFn = func() string { return "http://localhost:8543" }

// SetDashboardBaseURLFn overrides the base URL used by backlogItemLink.
func SetDashboardBaseURLFn(fn func() string) { dashboardBaseURLFn = fn }

// backlogItemLink returns a clickable deep link to itemID's detail view in
// the web UI (see web-app/src/components/backlog/BacklogItemPanel.tsx's
// `/backlog?item=` href), so a PR body can point a reviewer at the backlog
// item instead of making them paste a bare UUID into a search box.
func backlogItemLink(itemID string) string {
	return dashboardBaseURLFn() + "/backlog?item=" + itemID
}

// buildFallbackPRBody composes a PR body from the backlog item's own data —
// used when no headless pool is configured, GetGitDiff fails, or
// headless.DraftPRDescription errors out. Previously this fallback was a bare
// "Automated PR for backlog item: X\n\nItem ID: Y" one-liner (see PRs #147/#148
// on this repo), which explains nothing about why the change was made and gives
// a reviewer no verification checklist. Description ties the PR back to the
// item's own problem statement (the "why"); the item's acceptance criteria
// double as a test plan checklist since they are the only verification steps
// this code path has available without an LLM call.
func buildFallbackPRBody(item *BacklogItemData) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Summary\n%s\n\nBacklog item: %s\n", sanitizeField(item.Description, 1000), backlogItemLink(item.ID))

	if criteria, _ := ParseAcCriteria(item.AcceptanceCriteria); len(criteria) > 0 {
		sb.WriteString("\n## Test plan\n")
		for _, c := range criteria {
			box := "[ ]"
			if c.Status == domain.AcStatusDone {
				box = "[x]"
			}
			fmt.Fprintf(&sb, "- %s %s\n", box, sanitizeField(c.Text, 300))
		}
	}
	return sb.String()
}

// shipViaAgentOrFallback is handleReviewSessionExited's PASS-verdict entry
// point for shipping a PR when the work session that earned the verdict is
// no longer live to run /backlog/ship itself (or forcePush is set — see
// handleReviewSessionExited's doc comment). It tries the agent-driven path
// first — running agentShipPrompt (/backlog/ship, which drives
// /github:pr-ship: local CI, code review, remote CI, and actual
// merge-conflict resolution) as a headless one-shot via the wired
// OneShotShipRunner — and only falls back to the mechanical pushAndCreatePR
// (bare `git push` + `gh pr create`, no CI reaction, no conflict resolution)
// when that isn't available or didn't work. This mirrors the design already
// used for AutoCreatePR/TriggerShipPR-style flows: prefer the agent, keep
// the mechanical path as a backstop rather than deleting it outright, since
// pushAndCreatePR is still the right tool for attemptPushRemediation (a
// purely mechanical retry after a purely mechanical fetch+merge — no LLM
// judgment involved there) and remains a working fallback of last resort
// here when the agent-driven attempt itself fails to produce a PR (e.g. the
// session's Instance is no longer tracked live, or the one-shot call itself
// errors/times out).
//
// Deliberately does NOT special-case "no worktree at all" before trying the
// one-shot: if OneShotShipRunner is wired but there is genuinely nothing to
// point it at, RunOneShotForSession fails fast (its own findInstance lookup
// misses) and this falls through to pushAndCreatePR, which performs the
// exact same worktree-presence check it always has and reaches the exact
// same two pre-existing outcomes depending on what's actually missing:
//   - No worktree recorded in storage at all: pushAndCreatePR's
//     fallbackToDone("no worktree") transitions straight to done, unchanged
//     from before this fix (see
//     TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive).
//   - A worktree is recorded but the directory itself is gone from disk
//     (e.g. cleanupItemWorktreesExcept already ran): the mechanical git
//     commands fail with a filesystem error, which pushAndCreatePR's
//     existing stayInReviewAndNotify path already turns into a durable
//     StuckReasonPushFailed row and an operator notification — the item
//     stays in review rather than silently becoming done, and the PASS
//     verdict is not dropped. No new StuckReason was added for this case:
//     from an operator's perspective it is the same actionable signal
//     ("PASS verdict, no PR, needs a manual look") pushAndCreatePR's push/PR
//     failures already surface, and both remediation paths are identical
//     (investigate manually, or retry).
func (l *BacklogLifecycleListener) shipViaAgentOrFallback(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
	runner := l.getOneShotShipRunner()
	if runner == nil {
		log.InfoLog().Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s: no OneShotShipRunner wired, using mechanical push directly", item.ID)
		l.pushAndCreatePR(ctx, item, is)
		return
	}

	prURL, err := runner.RunOneShotForSession(ctx, is.SessionUUID, agentShipPrompt, oneShotShipTimeoutSeconds)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s session=%s: agent-driven ship failed (%v), falling back to mechanical push", item.ID, is.SessionUUID, err)
		l.pushAndCreatePR(ctx, item, is)
		return
	}
	if prURL == "" {
		log.WarningLog().Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s session=%s: agent-driven ship ran but produced no PR URL, falling back to mechanical push", item.ID, is.SessionUUID)
		l.pushAndCreatePR(ctx, item, is)
		return
	}

	log.InfoLog().Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s session=%s: agent shipped PR via one-shot /backlog/ship: %s", item.ID, is.SessionUUID, prURL)

	// Persist the PR fields and transition explicitly rather than relying
	// solely on the RunOneShot -> RecordPRCreatedOutOfBand side effect the
	// production *services.SessionService implementation performs as part of
	// RunOneShotForSession — that side effect only fires because
	// SessionService happens to hold a pointer back to this same listener
	// (server/dependencies.go's sessionService.SetBacklogLifecycleListener).
	// A test fake — or any future OneShotShipRunner implementation — has no
	// such obligation, so this path must be self-sufficient.
	// resolveToPRPending's transition is guarded by an ExpectedStatus
	// precondition, so if the side effect already made this transition, the
	// call below simply no-ops with a (deliberately ignored) precondition
	// error rather than double-applying anything.
	prNumber := 0
	if ref, parseErr := ParseGitHubURL(prURL); parseErr == nil {
		prNumber = ref.PRNumber
	}
	// BUG-063: prNumber<=0 (an unparseable/irrelevant prURL — e.g. the agent's
	// final output happened to mention an unrelated existing PR rather than
	// one it just created) must NOT fall through to the unconditional
	// resolveToPRPending below. Doing so was the exact mechanism that landed
	// an item in pr_pending with pr_number still 0: permanently invisible to
	// every downstream reconciler's PrNumberGT(0) filter, with nothing left
	// to retry. This mirrors the identical BUG-040 shape pushAndCreatePR was
	// already fixed for (see its own PR-field-persist-failure handling below)
	// — that fix was never propagated to this sibling call site until now.
	// We can't tell whether the agent's one-shot actually created a real PR
	// we simply failed to parse, so — like pushAndCreatePR's own persist
	// failure — the safe choice is to stay in review and let a human (or the
	// next TriggerReReview) sort it out, not silently retry PR creation and
	// risk a duplicate.
	if prNumber <= 0 {
		l.stayInReviewAndNotify(ctx, item.ID, item.Title,
			fmt.Sprintf("agent-driven ship via one-shot /backlog/ship produced an unusable PR reference (%q)", prURL),
			fmt.Errorf("could not parse a PR number from the one-shot ship output"))
		return
	}
	prURLCopy, prNumCopy := prURL, prNumber
	if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURLCopy,
		PrNumber: &prNumCopy,
	}, nil); updateErr != nil {
		l.stayInReviewAndNotify(ctx, item.ID, item.Title,
			fmt.Sprintf("failed to persist PR #%d fields from agent-driven ship", prNumber), updateErr)
		return
	}
	if transErr := l.resolveToPRPending(ctx, item.ID, "agent-driven ship via one-shot /backlog/ship", "shipViaAgentOrFallback"); transErr != nil {
		// May just be a harmless race with RunOneShot's own RecordPRCreatedOutOfBand
		// side effect already landing the same transition — handlePRPendingTransitionFailed
		// re-checks the item's current status before doing anything, so that case is a
		// silent no-op there. Anything else (a genuine drift) gets recovered immediately
		// if safe, or picked up by the next reconcileDriftedPRItems tick.
		l.handlePRPendingTransitionFailed(ctx, item.ID, "shipViaAgentOrFallback", transErr)
	}
}

// pushAndCreatePR commits any dirty state, pushes the branch, creates a GitHub PR,
// stores the PR URL and number on the item, then transitions to pr_pending.
// Falls back to a direct done transition only when there was genuinely nothing to
// ship (no worktree). If code was committed but push/PR-creation fails, the item
// stays in review and a notification is published — see stayInReviewAndNotify.
// Used both as shipViaAgentOrFallback's mechanical backstop (agent-driven ship
// unavailable or failed) and directly by attemptPushRemediation (a push retry
// after a purely mechanical fetch+merge — no LLM judgment needed there, so it
// skips shipViaAgentOrFallback and calls this directly).
func (l *BacklogLifecycleListener) pushAndCreatePR(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
	fallbackToDone := func(reason string) {
		log.InfoLog().Printf("[BacklogLifecycle] pushAndCreatePR item=%s falling back to done: %s", item.ID, reason)
		// No status precondition: item may be at review or ready depending on when
		// the PASS verdict was delivered relative to other transitions.
		if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil, TriggeredBySystem); transErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] pushAndCreatePR fallback done item=%s: %v", item.ID, transErr)
			// A PASS verdict already confirmed the work; there was nothing to
			// ship, so done was the correct terminal state — a failure here
			// leaves the item stuck with no further signal.
			l.notifyTransitionFailed(item.ID, item.Title, fmt.Sprintf("%s, so the item should have moved to done, but the transition failed", reason), transErr)
		}
	}

	wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		fallbackToDone("no worktree")
		return
	}

	g := l.getPRCreatorFactory()(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA)

	// Commit any remaining dirty state.
	commitMsg := fmt.Sprintf("[claudesquad] work complete for %q (pre-PR)", item.Title)
	if commitErr := g.CommitChanges(commitMsg); commitErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR commit item=%s: %v", item.ID, commitErr)
	}

	// Push branch to origin.
	if pushErr := g.PushBranch(); pushErr != nil {
		l.stayInReviewAndNotify(ctx, item.ID, item.Title, "push failed", pushErr)
		return
	}

	// Create (or locate existing) PR.
	var prURL string
	var prNumber int
	if item.PrNumber > 0 && item.PrURL != "" {
		// PR already exists from a previous attempt — just use it.
		prURL = item.PrURL
		prNumber = item.PrNumber
		log.InfoLog().Printf("[BacklogLifecycle] pushAndCreatePR item=%s reusing existing PR #%d", item.ID, prNumber)
	} else {
		// Pre-flight (BUG-063): a branch with zero commits ahead of main has
		// genuinely nothing to ship — CreatePR below would fail with gh's "No
		// commits between X and Y" error, which is not a retryable push/PR
		// failure. A PASS verdict already confirmed the work (it's often
		// already shipped by an earlier, unrelated PR), so route this case
		// through fallbackToDone exactly like the "no worktree at all" case
		// above, rather than leaving the item stuck in review forever behind
		// an unresolvable push_failed row. Any error from the check itself is
		// treated as inconclusive (HasCommitsAheadOfMain returns true), so a
		// broken check never blocks a real PR creation attempt.
		if hasCommits, aheadErr := g.HasCommitsAheadOfMain(bounceMainBranch); aheadErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR HasCommitsAheadOfMain item=%s: %v; proceeding with PR creation attempt", item.ID, aheadErr)
		} else if !hasCommits {
			fallbackToDone(fmt.Sprintf("branch %s has no commits ahead of %s — nothing to ship", wt.BranchName, bounceMainBranch))
			return
		}

		prTitle := item.Title
		prBody := buildFallbackPRBody(item)
		if pool := l.getHeadlessPool(); pool != nil {
			diff, _, diffErr := GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
			if diffErr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR GetGitDiff for description item=%s: %v; using fallback body", item.ID, diffErr)
			} else {
				drafted, draftCostUSD, draftErr := headless.DraftPRDescription(ctx, pool, item.Title, item.Description, diff, wt.BranchName)
				if draftCostUSD > 0 {
					if costErr := l.storage.UpdateItemSessionCost(ctx, is.ID, draftCostUSD); costErr != nil {
						log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR failed to persist PR-description cost item=%s: %v", item.ID, costErr)
					}
				}
				if draftErr != nil {
					log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR DraftPRDescription item=%s: %v; using fallback body", item.ID, draftErr)
				} else if drafted != "" {
					prBody = strings.TrimRight(drafted, "\n") + "\n\nBacklog item: " + backlogItemLink(item.ID) + "\n"
				}
			}
		}
		var prErr error
		prURL, prNumber, prErr = g.CreatePR(git.PRCreateOptions{Title: prTitle, Body: prBody})
		if prErr != nil {
			l.stayInReviewAndNotify(ctx, item.ID, item.Title, "PR creation failed", prErr)
			return
		}
		// Cache PR URL + number on the item so the reconciler and UI can use
		// them. This persist is load-bearing, not best-effort (BUG-040): every
		// downstream reconciler (ReconcilePRPending's FindPRPendingItems query,
		// EnablePRAutoMerge below) requires a real PrNumber/PrURL on the STORED
		// item, not just the local prURL/prNumber variables here. Previously a
		// failure here was only logged, and pushAndCreatePR proceeded
		// unconditionally to resolveToPRPending below — landing the item in
		// pr_pending with pr_number=0/pr_url="", permanently invisible to
		// FindPRPendingItems' PrNumberGT(0) filter and everything downstream of
		// it, with nothing left to retry. Treat a persist failure exactly like
		// a push/PR-creation failure: stay in review so a human (or the next
		// TriggerReReview) can retry, rather than silently entering that dead
		// end.
		prURLCopy := prURL
		prNumCopy := prNumber
		if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
			PrURL:    &prURLCopy,
			PrNumber: &prNumCopy,
		}, nil); updateErr != nil {
			l.stayInReviewAndNotify(ctx, item.ID, item.Title, fmt.Sprintf("failed to persist new PR #%d fields", prNumber), updateErr)
			return
		}
	}

	// Enable GitHub auto-merge so the PR merges automatically once CI passes.
	// Best-effort: repos without branch protection or auto-merge enabled will fail here.
	// ReconcilePRPending still polls and will detect the merge if one happens some other
	// way, but nothing will ever *initiate* the merge for this PR without auto-merge — the
	// operator must merge it manually, so this needs a notification, not just a log line
	// (same silent-failure pattern found and fixed elsewhere in this codebase — see
	// docs/tasks/backlog-feature-improvement.md).
	if autoErr := g.EnablePRAutoMerge(prNumber); autoErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR auto-merge item=%s pr=%d: %v", item.ID, prNumber, autoErr)
		l.notify(item.ID,
			"Auto-merge not enabled",
			fmt.Sprintf("%s — PR #%d could not be set to auto-merge (%v). It will need to be merged manually once checks pass.", item.Title, prNumber, autoErr),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
	} else {
		log.InfoLog().Printf("[BacklogLifecycle] pushAndCreatePR item=%s PR #%d auto-merge enabled", item.ID, prNumber)
	}

	// Request a GitHub Copilot review so async Copilot feedback has a chance
	// to land before the item goes unwatched at pr_pending. Best-effort: a
	// missing Copilot review is a missed nicety, not a missed auto-merge path
	// (lower notification priority than the auto-merge failure above).
	if reviewErr := g.RequestCopilotReview(prNumber); reviewErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] pushAndCreatePR RequestCopilotReview item=%s pr=%d: %v", item.ID, prNumber, reviewErr)
		l.notify(item.ID,
			"Copilot review not requested",
			fmt.Sprintf("%s — PR #%d could not get a Copilot review request (%v).", item.Title, prNumber, reviewErr),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			1, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
		)
	} else {
		log.InfoLog().Printf("[BacklogLifecycle] pushAndCreatePR item=%s PR #%d Copilot review requested", item.ID, prNumber)
	}

	// Transition to pr_pending.
	if transErr := l.resolveToPRPending(ctx, item.ID, "", "pushAndCreatePR"); transErr != nil {
		l.handlePRPendingTransitionFailed(ctx, item.ID, "pushAndCreatePR", transErr)
		return
	}
	log.InfoLog().Printf("[BacklogLifecycle] pushAndCreatePR item=%s → pr_pending (PR #%d %s)", item.ID, prNumber, prURL)
}

// stayInReviewAndNotify handles push/PR-creation failures for both
// pushAndCreatePR and shipViaAgentOrFallback (BUG-063). Unlike fallbackToDone,
// this must NOT transition the item: pushAndCreatePR's callers may have
// committed code to the worktree that never reached GitHub, and
// shipViaAgentOrFallback's caller cannot tell whether the agent-driven
// one-shot actually created a real PR it just failed to parse/persist a
// reference to — in both cases marking the item done or pr_pending would
// risk silently discarding real work or duplicating a PR. The item stays in
// review — a human can retry via TriggerReReview, or fix the underlying
// issue (auth, network, branch protection, a storage error) and let the next
// review pass retry.
func (l *BacklogLifecycleListener) stayInReviewAndNotify(ctx context.Context, itemID, itemTitle, reason string, err error) {
	log.WarningLog().Printf("[BacklogLifecycle] item=%s: %s: %v — leaving in review", itemID, reason, err)

	notifyToast := func() {
		l.notify(itemID,
			"PR creation failed",
			fmt.Sprintf("%s — %s: %v. Retry or investigate manually.", itemTitle, reason, err),
			7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}

	// Durable push_failed row (Story 2.1.6). Also doubles as the ephemeral
	// toast's dedup key below.
	er := l.storage.repo
	applied, markErr := er.MarkStuck(ctx, itemID, domain.StuckReasonPushFailed, BacklogStatusReview,
		fmt.Sprintf("%s: %v", reason, err))
	if markErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] MarkStuck(push_failed) item=%s: %v", itemID, markErr)
		return
	}
	if !applied {
		return
	}

	// Notify-once dedup (same pattern as markAbandonedReview and the other
	// stuck reasons): MarkStuckNotified only flips notified_at nil -> now
	// once per open stuck-state row, so repeated calls for the same
	// still-open failure (e.g. a non-fast-forward push retried every
	// reconciliation tick) skip the ephemeral ERROR toast after the first —
	// this is what was previously firing a fresh "PR creation failed" toast
	// every few seconds with no dedup. The toast fires again only once the
	// row is resolved (push/PR succeeds) and later reopens on a new failure.
	notifiedNow, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonPushFailed)
	if notifyErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] MarkStuckNotified(push_failed) item=%s: %v", itemID, notifyErr)
		return
	}
	if !notifiedNow {
		return
	}
	notifyToast()
}

// resolveToPRPending performs the transition+resolve tail shared by every
// path that moves a backlog item from review to pr_pending because a PR now
// exists: the status transition itself, then — on success — resolving any
// open push_failed/abandoned_review rows immediately rather than waiting for
// the self-heal sweep's next tick (Task 2.1.5a). note is attached to the
// transition's audit event; caller identifies the log prefix used by the
// (always best-effort) stuck-resolution calls. Returns the transition error,
// if any, so callers can apply their own logging/fallback behavior.
func (l *BacklogLifecycleListener) resolveToPRPending(ctx context.Context, itemID, note, caller string) error {
	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusReview), Note: note}
	if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, itemID, BacklogStatusPRPending, precondition, TriggeredBySystem); transErr != nil {
		return transErr
	}
	l.resolveStuckLogged(ctx, l.storage.repo, itemID, domain.StuckReasonPushFailed, caller)
	l.resolveStuckLogged(ctx, l.storage.repo, itemID, domain.StuckReasonAbandonedReview, caller)
	return nil
}

// recoverDriftedPRItem attempts to recover a single item whose real, cached
// PR reference (prNumber/prUrl) has drifted out of ReconcilePRPending's view
// — see FindDriftedPRItems' doc comment for the drift mechanism. Recovery is
// a single CAS transition back to pr_pending, scoped to the item's own
// currently-observed status/updated_at so a genuine concurrent transition
// (e.g. a fresh work/review session starting between the caller's read and
// this write) simply loses the CAS and is left alone rather than clobbered —
// the same "anchor on reality, never force" discipline BUG-026's fix
// established for TransitionBacklogItemStatus itself. Callers must have
// already confirmed no active work/review session exists for this item
// (hasActiveSession) before calling — this function does not re-check.
// Returns true if the item was recovered. Best-effort: errors are logged,
// never returned.
func (l *BacklogLifecycleListener) recoverDriftedPRItem(ctx context.Context, item *BacklogItemData, caller string) bool {
	updatedAt := item.UpdatedAt
	precondition := &BacklogItemPrecondition{
		ExpectedStatus:    item.Status,
		ExpectedUpdatedAt: &updatedAt,
		Note: fmt.Sprintf("self-heal (%s): recovered from drift — item has PR #%d (%s) cached but status was %q, not pr_pending",
			caller, item.PrNumber, item.PrURL, item.Status),
	}
	if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusPRPending, precondition, TriggeredBySystem); transErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] recoverDriftedPRItem(%s) item=%s: recovery transition failed (likely a concurrent legitimate transition, will retry next tick): %v", caller, item.ID, transErr)
		return false
	}
	log.WarningLog().Printf("[BacklogLifecycle] recoverDriftedPRItem(%s) item=%s: recovered from status drift — PR #%d (%s) was stranded at status %q with no active session; transitioned back to pr_pending", caller, item.ID, item.PrNumber, item.PrURL, item.Status)
	l.notify(item.ID,
		"Backlog item recovered from stuck state",
		fmt.Sprintf("%s — had an open PR (#%d) but its status had drifted away from tracking; automatically recovered and resumed polling.", item.Title, item.PrNumber),
		10, // sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
		1,  // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
	)
	return true
}

// handlePRPendingTransitionFailed is called when resolveToPRPending fails
// after prNumber/prUrl were already durably persisted on the item — the
// exact drift mechanism FindDriftedPRItems' doc comment describes (a
// concurrent legitimate event, e.g. markAbandonedReview's grace period
// respawning a review pass while an agent-driven ship is still mid-flight,
// wins the race and moves status away from "review" before this call's own
// CAS-gated transition to pr_pending lands). Rather than silently leaving
// the item stranded until the periodic reconcileDriftedPRItems sweep's next
// tick (ReconcileStuck runs every 60s — server/dependencies.go), this
// attempts the same recovery immediately: if nothing is actively working the
// item right now, transition it straight back to pr_pending so it re-enters
// ReconcilePRPending's view in this same tick. If something IS actively
// working it (a legitimate concurrent event genuinely owns the item now),
// recovery correctly declines — the periodic sweep remains the backstop for
// whenever that session later ends without itself resolving to pr_pending.
// Also correctly no-ops when the "failure" was actually a harmless race with
// another writer that already landed the same transition (e.g.
// RecordPRCreatedOutOfBand beating shipViaAgentOrFallback to it) — the
// re-fetched item's status is checked before attempting anything.
func (l *BacklogLifecycleListener) handlePRPendingTransitionFailed(ctx context.Context, itemID, caller string, transErr error) {
	log.WarningLog().Printf("[BacklogLifecycle] %s pr_pending transition item=%s failed after PR fields were already persisted — item may be stranded with a real PR outside pr_pending tracking until self-heal recovers it: %v", caller, itemID, transErr)

	sessions, sessErr := l.storage.ListItemSessions(ctx, itemID)
	if sessErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] %s handlePRPendingTransitionFailed ListItemSessions item=%s: %v", caller, itemID, sessErr)
		return
	}
	if hasActiveSession(sessions) {
		log.InfoLog().Printf("[BacklogLifecycle] %s handlePRPendingTransitionFailed item=%s: active session found, leaving recovery to the next reconcileDriftedPRItems tick", caller, itemID)
		return
	}
	item, getErr := l.storage.GetBacklogItem(ctx, itemID)
	if getErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] %s handlePRPendingTransitionFailed GetBacklogItem item=%s: %v", caller, itemID, getErr)
		return
	}
	if item.PrNumber <= 0 || item.PrURL == "" ||
		item.Status == string(BacklogStatusPRPending) || item.Status == string(BacklogStatusDone) || item.Status == string(BacklogStatusArchived) {
		return // already recovered, or resolved to a terminal state, by the time we got here
	}
	l.recoverDriftedPRItem(ctx, item, caller)
}

// reconcileDriftedPRItems is the periodic self-heal detector for the drift
// class FindDriftedPRItems queries: items with a real, cached PR reference
// whose status has fallen out of ReconcilePRPending's view with nothing left
// actively working on them. Registered immediately before ReconcilePRPending
// (Task: PR-lifecycle drift self-heal) so a recovered item is picked up by
// the merge/CI polling sweep in the very same tick rather than waiting an
// extra cycle. Best-effort: query failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileDriftedPRItems(ctx context.Context, er *EntRepository) {
	items, err := er.FindDriftedPRItems(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileDriftedPRItems query error: %v", err)
		return
	}
	for _, item := range items {
		itemData := backlogItemToData(item)
		l.recoverDriftedPRItem(ctx, &itemData, "reconcileDriftedPRItems")
	}
}

// reconcilePushFailedItems retries the push+PR flow for every open
// push_failed stuck row still anchored at "review" — see the doc comment on
// its ReconcileStuck call site for why this periodic sweep is needed at all
// (pushAndCreatePR itself only ever runs in response to a review-session
// event, so an item with no active session would otherwise never get a
// second attempt). While the item remains at "review", resolution happens
// through resolveToPRPending once a retried push succeeds — selfHealStuck's
// terminal-anchor case only backstops the item reaching done/archived some
// other way (see its doc comment), so this loop's own row-status filter
// below still only needs to consider "review" as an active retry target.
// Best-effort: query failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcilePushFailedItems(ctx context.Context, er *EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcilePushFailedItems FindOpenStuckStates error: %v", err)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonPushFailed {
			continue
		}
		if row.ItemStatus != BacklogStatusReview {
			continue // no longer applicable to this item's current state
		}
		l.retryPushFailedWithBackoffGate(ctx, row.ItemID, row.ItemTitle)
	}
}

// retryPushFailedWithBackoffGate dispatches attemptPushRemediation through
// the shared remediation backoff gate (Storage.RemediationDue,
// session/backlog_remediation.go) — the "push_failed" reason's remediation
// action per docs/tasks/backlog-stuck-item-auto-remediation.md Phase B.
// Mirrors autoReopenWithBackoffGate's shape (bare goroutine, no semaphore —
// a git fetch+merge+push is seconds, not the minutes a headless LLM respawn
// can take, so the reviewSem markAbandonedReview/ReconcileStuck's
// review-gate respawns share is not needed here). Best-effort: gate
// query/write errors are logged, never returned, and fail OPEN (still
// attempts the retry) rather than silently stranding the item — same
// rationale as autoReopenWithBackoffGate.
func (l *BacklogLifecycleListener) retryPushFailedWithBackoffGate(ctx context.Context, itemID, itemTitle string) {
	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonPushFailed)
	if gateErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] retryPushFailedWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see autoReopenWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated push retry has been attempted %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog().Printf("[BacklogLifecycle] retryPushFailedWithBackoffGate item=%s: push_failed remediation backoff not yet due, skipping retry", itemID)
		return
	}

	go func() {
		l.attemptPushRemediation(l.shutdownCtx, itemID, itemTitle)
	}()
}

// attemptPushRemediation is the push_failed remediation action dispatched by
// retryPushFailedWithBackoffGate once RemediationDue grants an attempt. It
// fetches the branch's current remote ref and merges it into the worktree
// (via the injected branchReconciler — git.MergeMainIntoWorktree in
// production, despite the "main" name it fetches+merges whatever branch
// name is passed; here that's the item's OWN branch, which reconciles the
// exact non-fast-forward-rejection shape live-repro'd on 2026-07-20:
// c2ad7bf3-91bf-4d47-8654-0f2f20869080's stelekit branch was rejected
// because something else advanced origin's copy of the same branch name).
// On a clean merge (or if the branch was already up to date — e.g. a
// previous remediation attempt already fixed it but the push itself failed
// for an unrelated transient reason), retries the full push+PR flow via
// pushAndCreatePR, which resolves the push_failed row on success through its
// existing resolveToPRPending call. A real content conflict is NOT
// auto-resolved — per the task scope, merge conflicts need a human; the
// item is left stuck (still governed by the normal backoff schedule, so it
// eventually parks after MaxRemediationAttempts) with a notification naming
// the conflicting files.
func (l *BacklogLifecycleListener) attemptPushRemediation(ctx context.Context, itemID, itemTitle string) {
	item, err := l.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] attemptPushRemediation GetBacklogItem item=%s: %v", itemID, err)
		return
	}
	if item.Status != string(BacklogStatusReview) {
		log.DebugLog().Printf("[BacklogLifecycle] attemptPushRemediation item=%s: status is now %s, not review — skipping", itemID, item.Status)
		return
	}

	sessions, err := l.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] attemptPushRemediation ListItemSessions item=%s: %v", itemID, err)
		return
	}
	var lastWork *ItemSessionSummary
	for i := range sessions {
		// Ascending by CreatedAt (ListItemSessions' query order) — keep
		// overwriting so this ends up holding the *most recent* work
		// session, mirroring the identical pattern in ReconcilePRPending's
		// ship-snapshot path.
		if sessions[i].Role == SessionRoleWork {
			s := sessions[i]
			lastWork = &s
		}
	}
	if lastWork == nil {
		log.WarningLog().Printf("[BacklogLifecycle] attemptPushRemediation item=%s: no work session found, cannot retry push", itemID)
		return
	}

	wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		log.WarningLog().Printf("[BacklogLifecycle] attemptPushRemediation item=%s: no worktree available (%v), cannot retry push", itemID, wtErr)
		return
	}

	result, mergeErr := l.getBranchReconciler()(wt.WorktreePath, wt.BranchName)
	if mergeErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] attemptPushRemediation item=%s: fetch/merge of origin/%s failed: %v — will retry on next backoff window", itemID, wt.BranchName, mergeErr)
		return
	}
	if result.Conflicted {
		log.WarningLog().Printf("[BacklogLifecycle] attemptPushRemediation item=%s: origin/%s conflicts with the local worktree in %v — cannot auto-resolve", itemID, wt.BranchName, result.ConflictedFiles)
		l.notify(itemID,
			"Manual rebase needed",
			fmt.Sprintf("%s — the remote branch has diverged in a way that conflicts with this item's committed work (%s). Automated retry cannot resolve real content conflicts; resolve manually and push, or use Reset to try again automatically after fixing it.", itemTitle, strings.Join(result.ConflictedFiles, ", ")),
			7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
		return
	}

	log.InfoLog().Printf("[BacklogLifecycle] attemptPushRemediation item=%s: origin/%s reconciled (upToDate=%v merged=%v), retrying push", itemID, wt.BranchName, result.UpToDate, result.Merged)
	l.pushAndCreatePR(ctx, item, *lastWork)
}

// RecordPRCreatedOutOfBand records a PR that was created for workSessionUUID
// through a path other than pushAndCreatePR and transitions the linked
// backlog item straight to pr_pending via the shared resolveToPRPending tail.
// (Named "Record", not "Notify", to avoid confusion with l.notify — the
// user-facing toast helper used elsewhere in this file; this method mutates
// backlog-item state, it doesn't just surface a message.)
//
// Why this exists: pushAndCreatePR is the *only* place that ever writes
// pr_pending, but it is reached exclusively via the automated
// handleReviewSessionExited(PASS) → pushAndCreatePR call chain. The Review
// Queue's manual "Create PR" button (web-app/src/components/sessions/
// ReviewQueuePanel.tsx) drives a completely separate path —
// SessionService.RunOneShot (server/services/session_service.go) — that runs
// an ad hoc `claude -p <prompt>` in the worktree and only ever persists the
// resulting PR URL onto the *session* record (inst.SetGitHubPR). It has no
// knowledge of backlog items at all, so a backlog-linked item whose PR was
// created this way never left "review" — ReconcilePRPending's FindPRPendingItems
// query structurally cannot find it, since it only looks at items already in
// pr_pending. Left in "review", the item instead accumulates
// in_progress↔review bounce churn from unrelated reconciliation and eventually
// reports stuck-reason BOUNCING instead of the correct pr_ready_unmerged. This
// is the root cause traced in docs/tasks/backlog-feature-improvement.md's
// "second, compounding root cause" note for PR #157.
//
// No-op if the listener is disabled, the caller has no PR info, the session
// isn't backlog-linked, or the item isn't currently "review" (avoids
// clobbering any other in-flight transition). That guard narrows, but does
// not eliminate, a race with a concurrent pushAndCreatePR call on the same
// item: TransitionBacklogItemStatus's precondition check is a read-then-write
// (Get, check in memory, then Save) rather than a true atomic compare-and-
// swap, so both calls can observe "review" and both succeed. That's harmless
// here — both write the same target status and equivalent PR fields — but it
// means two BacklogStatusEvent audit rows can be written instead of one, not
// that exactly one call is guaranteed to win.
//
// Known limitation (not fixed here — see PR description): unlike
// pushAndCreatePR, this does not attempt EnablePRAutoMerge, since it has no
// worktree/git handle to call it with; a PR created via this path currently
// requires a manual merge. extractPRURL's freeform-text parsing also means
// prURL/prNumber are not independently verified against GitHub before being
// persisted — acceptable for this single-operator tool's threat model, but
// worth knowing if RunOneShot's trust boundary ever changes.
func (l *BacklogLifecycleListener) RecordPRCreatedOutOfBand(ctx context.Context, workSessionUUID, prURL string, prNumber int) {
	if !l.enabled.Load() || prURL == "" || prNumber <= 0 {
		return
	}
	is, err := l.storage.GetItemSessionBySessionUUID(ctx, workSessionUUID)
	if err != nil {
		// Not backlog-linked (or lookup failed) — nothing to reconcile. Debug,
		// not Error: the overwhelming majority of RunOneShot callers are
		// non-backlog sessions, so this is the expected common case.
		log.DebugLog().Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand GetItemSessionBySessionUUID(%s): %v", workSessionUUID, err)
		return
	}
	item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand GetBacklogItem session=%s item=%s: %v", workSessionUUID, is.BacklogItemID, err)
		return
	}
	if item.Status != string(BacklogStatusReview) {
		// Only review→pr_pending is a valid transition here. If the item is
		// already pr_pending (e.g. pushAndCreatePR beat us to it) or anywhere
		// else, leave it alone rather than fighting the item's real owner.
		return
	}

	prURLCopy, prNumCopy := prURL, prNumber
	if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURLCopy,
		PrNumber: &prNumCopy,
	}, nil); updateErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand store PR fields item=%s: %v", item.ID, updateErr)
	}

	note := "PR created via manual Review Queue Create-PR flow (RunOneShot), not the automated pushAndCreatePR path"
	if transErr := l.resolveToPRPending(ctx, item.ID, note, "RecordPRCreatedOutOfBand"); transErr != nil {
		l.handlePRPendingTransitionFailed(ctx, item.ID, "RecordPRCreatedOutOfBand", transErr)
		return
	}
	log.InfoLog().Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand item=%s session=%s → pr_pending (PR #%d %s, via manual RunOneShot flow)", item.ID, workSessionUUID, prNumber, prURL)
}

// CaptureShipSnapshot durably captures the GitHub PR/review/CI state and the
// per-file diff stats for item at the moment its PR merges, so that data
// survives worktree cleanup once the item reaches "done" — the core
// unified-vcs-widget requirement. It is a free function, not a method on
// BacklogLifecycleListener: it needs no state from that type beyond
// *Storage, which is passed explicitly here (per
// the `interface-pollution-checklist` skill, a method only earns its
// receiver when it genuinely needs the type's other state).
//
// Two data groups are captured independently — a failure in one must never
// discard a success in the other:
//   - Group A (GitHub): mapped from the already-fetched prStatus.
//     CaptureShipSnapshot makes no GitHub call of its own. prStatus == nil
//     means group A already failed before this function was even called
//     (e.g. the caller's own GetPRStatus errored) — that's a valid input,
//     not a bug. PRStatus does not expose a raw CI-conclusion string
//     (worktree_git.go:330-345's field list), so ShippedCheckConclusion is
//     derived from CIFailing as "failure"/"success" — a minor, accepted
//     fidelity gap versus Session.githubCheckConclusion.
//   - Group B (file stats): computed independently via
//     git.FileStatsBetween(item.RepoPath, wt.BaseCommitSHA, lastWork.LastCommitSha),
//     JSON-encoded into ShippedFileStats.
//
// Whichever group(s) succeed are written via one storage.UpdateBacklogItem
// call. ShippedSnapshotCaptureFailed is set true whenever either group
// failed; ShippedSnapshotAt is set whenever at least one group succeeded.
// ShippedCheckConclusion is never written as "failed" — that field holds
// only genuine CI-conclusion values; ShippedSnapshotCaptureFailed is the
// dedicated signal for a capture failure.
//
// CaptureShipSnapshot always returns nil: it never blocks the pr_pending →
// done transition, regardless of how many groups failed. Blocking done on a
// GitHub API hiccup or a pruned base SHA would leave a genuinely-merged item
// stuck in pr_pending forever, so this fails closed on data completeness,
// not on the workflow itself.
//
// No in-process cache/memoization is introduced here — every call is a
// direct write-through via UpdateBacklogItem. If a future caching layer is
// added on top of this function, it must return the locally-computed
// snapshot value rather than re-reading a cache slot after a lock is
// released, per docs/explanation/concurrency-patterns.md.
func CaptureShipSnapshot(ctx context.Context, storage *Storage, item *BacklogItemData, prStatus *git.PRStatus, lastWork *ItemSessionSummary, wt *GitWorktreeData) error {
	var update BacklogItemUpdate
	groupAFailed := false
	groupBFailed := false
	anySucceeded := false

	// Group A: GitHub PR/CI/review state, from the already-fetched prStatus.
	if prStatus != nil {
		approvedCount := prStatus.ApprovedCount
		changesReqCount := prStatus.ChangesRequestedCount
		conclusion := "success"
		if prStatus.CIFailing {
			conclusion = "failure"
		}
		update.ShippedApprovedCount = &approvedCount
		update.ShippedChangesReqCount = &changesReqCount
		update.ShippedCheckConclusion = &conclusion
		anySucceeded = true
	} else {
		groupAFailed = true
		log.WarningLog().Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=github: prStatus unavailable", item.ID, item.PrNumber)
	}

	// Group B: per-file diff stats, independent of group A's outcome.
	if lastWork != nil && wt != nil {
		stats, statsErr := git.FileStatsBetween(item.RepoPath, wt.BaseCommitSHA, lastWork.LastCommitSha)
		if statsErr != nil {
			groupBFailed = true
			log.WarningLog().Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=file-stats: %v", item.ID, item.PrNumber, statsErr)
		} else if encoded, jsonErr := json.Marshal(stats); jsonErr != nil {
			groupBFailed = true
			log.WarningLog().Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=file-stats: marshal: %v", item.ID, item.PrNumber, jsonErr)
		} else {
			encodedStr := string(encoded)
			update.ShippedFileStats = &encodedStr
			anySucceeded = true
		}
	} else {
		groupBFailed = true
		log.WarningLog().Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=file-stats: worktree/last-work data unavailable", item.ID, item.PrNumber)
	}

	if groupAFailed || groupBFailed {
		captureFailed := true
		update.ShippedSnapshotCaptureFailed = &captureFailed
	}
	if anySucceeded {
		now := time.Now()
		update.ShippedSnapshotAt = &now
	}

	if _, updateErr := storage.UpdateBacklogItem(ctx, item.ID, update, nil); updateErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d: UpdateBacklogItem failed: %v", item.ID, item.PrNumber, updateErr)
	}

	return nil
}

// remediatePRFixWithBackoffGate wraps fixSpawner.AutoReopenForPRFix with the
// shared remediation backoff gate (Storage.RemediationDue,
// session/backlog_remediation.go) — the fix for the MAJOR bug flagged in
// docs/tasks/backlog-feature-improvement.md's 2026-07-28 entry:
// ReconcilePRPending's CI-failing/blocked-review/conflict branch (and its
// sibling closed-without-merging branch) called AutoReopenForPRFix directly
// on every ~60s reconciliation tick with no backoff, unlike every other
// remediation call site in this file (autoReopenWithBackoffGate,
// retryPushFailedWithBackoffGate, remediateStaleWorkWithBackoffGate,
// retryOrphanedTriageWithBackoffGate) — a PR that keeps failing CI could get
// a fresh fix session respawned indefinitely.
//
// Mirrors markAbandonedReview's shape (the one other *WithBackoffGate-family
// helper that both opens/refreshes its own row AND dispatches in the same
// call, rather than being fed by a separate periodic detector): MarkStuck
// opens or refreshes the durable pr_needs_fix row for itemID this tick
// (idempotent — a no-op refresh if already open), notifies once on first
// sighting, then RemediationDue gates the actual dispatch. Best-effort
// throughout: MarkStuck/FindOpenStuckStates/RemediationDue errors are
// logged, never returned, and fail OPEN (still attempts the fix) rather than
// silently stranding the item — same rationale as every sibling helper.
//
// Returns attempted=false when the backoff gate is not yet due (or MarkStuck
// determined the item is no longer in pr_pending) — the caller must treat
// this exactly like "nothing happened this tick" and MUST NOT run any
// AutoReopenForPRFix-result-dependent logic (e.g. the closed-branch's
// BUG-040 field-clearing), since nothing was actually attempted.
//
// Before consulting RemediationDue, this checks fixSpawner.HasActiveWorkSession:
// an active session skips the backoff gate entirely for that tick (it always
// steers, never spawns). Only the no-active-session (spawn) path is gated by
// RemediationDue.
func (l *BacklogLifecycleListener) remediatePRFixWithBackoffGate(ctx context.Context, er *EntRepository, fixSpawner PRFixSpawner, itemID, itemTitle, fixCtx string) (attempted bool, err error) {
	applied, markErr := er.MarkStuck(ctx, itemID, domain.StuckReasonPRNeedsFix, BacklogStatusPRPending, fixCtx)
	if markErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate MarkStuck item=%s: %v", itemID, markErr)
	}
	if applied {
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate FindOpenStuckStates item=%s: %v", itemID, findErr)
		} else if row, ok := findOpenStuckStateFor(rows, itemID, domain.StuckReasonPRNeedsFix); ok && row.NotifiedAt == nil {
			l.notify(itemID,
				"PR needs attention",
				fmt.Sprintf("%s — the PR has failing CI, blocking reviews, or a merge conflict. An automated fix attempt will run on the standard backoff schedule.", itemTitle),
				8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
				2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
			)
			if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonPRNeedsFix); notifyErr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate MarkStuckNotified item=%s: %v", itemID, notifyErr)
			}
		}
	}

	// Bypass the backoff gate entirely for the steer path: steering an
	// already-active session has its own tighter, content-aware throttle
	// (5-min cooldown + reason-signature dedup, two-tick conflict debounce)
	// and can't create a duplicate session, so it must not be slowed by the
	// 30m->72h backoff that exists solely to prevent a spawn storm on the
	// no-active-session branch. active is threaded into
	// AutoReopenForPRFixWithKnownSession below rather than discarded, so
	// this decision and that call's steer-vs-spawn decision share one query.
	active, activeErr := fixSpawner.HasActiveWorkSession(ctx, itemID)
	if activeErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate HasActiveWorkSession item=%s: %v", itemID, activeErr)
		// fail open — fall through to the due-gated path below, same rationale
		// as every other best-effort check in this function.
	} else if active != nil {
		log.InfoLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate item=%s: active session found, steering unconditionally (bypassing backoff gate)", itemID)
		return true, fixSpawner.AutoReopenForPRFixWithKnownSession(ctx, itemID, fixCtx, active)
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonPRNeedsFix)
	if gateErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see autoReopenWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated PR-fix retry has been attempted %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate item=%s: pr_needs_fix remediation backoff not yet due, skipping fix spawn", itemID)
		return false, nil
	}

	// Trigger-source tag (webhook vs poller-tick) at the single funnel both
	// ReconcilePRPending's 60s-tick loop and TriggerPRFixForEvent's on-demand call
	// share for an actual fix attempt — the data point AC8's "% triggered by webhook
	// vs poller" measurement needs, since TriggerFireEvent alone only ever sees the
	// webhook side (the poller never writes to that table, by design — AC3).
	log.InfoLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate item=%s: attempting fix (trigger_source=%s)", itemID, prFixTriggerSourceFrom(ctx))

	return true, fixSpawner.AutoReopenForPRFix(ctx, itemID, fixCtx)
}

// reconcilePRPendingItem runs ReconcilePRPending's per-item reconciliation logic for
// exactly one item, on demand. Extracted (ADR-002) so a webhook-triggered caller
// (TriggerPRFixForEvent) can invoke the identical logic for a single item without
// duplicating it or waiting for ReconcilePRPending's next 60s tick. This is a pure
// move of the loop body previously inlined in ReconcilePRPending — every `continue`
// became a `return` — with zero behavior change.
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity relocated verbatim from ReconcilePRPending
func (l *BacklogLifecycleListener) reconcilePRPendingItem(ctx context.Context, er *EntRepository, item *ent.BacklogItem) {
	if item.PrNumber == 0 || item.PrURL == "" {
		return
	}
	repoPath := item.RepoPath
	if repoPath == "" {
		return
	}
	g := l.getPRPendingCheckerFactory()(repoPath)

	// 1. Check if the PR has been merged → done.
	merged, mergedErr := g.IsPRMerged(item.PrNumber)
	if mergedErr != nil {
		log.DebugLog().Printf("[BacklogLifecycle] ReconcilePRPending IsPRMerged item=%s pr=%d: %v", item.ID, item.PrNumber, mergedErr)
		return
	}
	if merged {
		// Capture the durable ship snapshot (GitHub PR/CI/review state +
		// per-file diff stats) synchronously, before the done transition —
		// never as a background goroutine — so the data is written before
		// the worktree is eligible for cleanup (Story 3.3.1). prStatus is
		// fetched here at the merge-detection point specifically for the
		// snapshot; a fetch error is passed through as prStatus == nil
		// rather than skipping capture entirely, since CaptureShipSnapshot
		// treats a nil prStatus as "group A already failed" and still
		// captures group B (file stats) independently.
		snapshotPRStatus, snapshotStatusErr := g.GetPRStatus(item.PrNumber)
		if snapshotStatusErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending GetPRStatus (ship snapshot) item=%s pr=%d: %v", item.ID, item.PrNumber, snapshotStatusErr)
			snapshotPRStatus = nil
		}

		itemData := backlogItemToData(item)

		var lastWork *ItemSessionSummary
		if sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID.String()); sessErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending ListItemSessions (ship snapshot) item=%s: %v", item.ID, sessErr)
		} else {
			for i := range sessions {
				// Ascending by CreatedAt (ListItemSessions' query order) —
				// keep overwriting so this ends up holding the *most
				// recent* work session, mirroring
				// backlog_service_ship_status.go:51-58.
				if sessions[i].Role == SessionRoleWork {
					lastWork = &sessions[i]
				}
			}
		}

		var wt *GitWorktreeData
		if lastWork != nil {
			if wtData, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID); wtErr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending GetWorktreeDataBySessionUUID (ship snapshot) item=%s session=%s: %v", item.ID, lastWork.SessionUUID, wtErr)
			} else {
				wt = &wtData
			}
		}

		// Story 6 guard (adversarial-review.md's Blocker): re-verify, via a
		// live GitHub lookup, that PR #item.PrNumber's head branch still
		// matches this item's currently-tracked branch before treating the
		// merge as this item's own and auto-completing it. wt == nil (no
		// work session, or a GetWorktreeDataBySessionUUID failure above) is
		// treated identically to a definitive mismatch — fail closed.
		var trackedBranch string
		if wt != nil {
			trackedBranch = wt.BranchName
		}
		if matches, verifyErr := l.verifyPRHeadBranchMatchesTracked(ctx, item.RepoPath, trackedBranch, item.PrNumber); verifyErr != nil || !matches {
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-done transition (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)
			return
		}

		if capErr := CaptureShipSnapshot(ctx, l.storage, &itemData, snapshotPRStatus, lastWork, wt); capErr != nil {
			// CaptureShipSnapshot always returns nil today; this branch
			// exists defensively in case that contract ever changes, and
			// must never block the done transition below.
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending CaptureShipSnapshot item=%s pr=%d: %v", item.ID, item.PrNumber, capErr)
		}

		precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusPRPending)}
		if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID.String(), BacklogStatusDone, precondition, TriggeredBySystem); transErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] ReconcilePRPending done transition item=%s: %v", item.ID, transErr)
			// PR #%d is already confirmed merged — the item is left at
			// pr_pending with nothing else surfacing this until the next
			// tick retries it.
			l.notifyTransitionFailed(item.ID.String(), item.Title, fmt.Sprintf("PR #%d was confirmed merged but the item's transition to done failed", item.PrNumber), transErr)
		} else {
			log.InfoLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s → done (PR #%d merged)", item.ID, item.PrNumber)
			// The item just reached done — resolve pr_ready_unmerged
			// immediately (Task 2.1.5a) rather than waiting for the
			// self-heal sweep's next tick.
			l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending")
			l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRNeedsFix, "ReconcilePRPending")
			// The PR is merged, so ship.md's "must still exist for a
			// possible one-shot /backlog/ship re-invocation" constraint
			// (see CleanupSlashCommands' doc comment) no longer applies —
			// this is the first point in the lifecycle where scaffolding
			// cleanup is safe. Best-effort: the worktree directory is
			// often already gone by now (Instance.Kill/Pause deletes it
			// independently), in which case these are no-ops.
			if wt != nil && wt.WorktreePath != "" {
				if cleanupErr := CleanupBacklogContextFile(wt.WorktreePath); cleanupErr != nil {
					log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending CleanupBacklogContextFile item=%s: %v", item.ID, cleanupErr)
				}
				if cleanupErr := CleanupSlashCommands(wt.WorktreePath); cleanupErr != nil {
					log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending CleanupSlashCommands item=%s: %v", item.ID, cleanupErr)
				}
			}
		}
		return
	}

	// 2. PR still open — check CI status and reviews.
	prStatus, statusErr := g.GetPRStatus(item.PrNumber)
	if statusErr != nil {
		log.DebugLog().Printf("[BacklogLifecycle] ReconcilePRPending GetPRStatus item=%s pr=%d: %v", item.ID, item.PrNumber, statusErr)
		return
	}

	fixSpawner := l.getPRFixSpawner()

	// 2b. Closed without merging (human rejected it) — IsPRMerged already returned
	// false above, and without this check a closed PR reads identically to a
	// healthy open one (no failing CI, no blocking review, no conflict), so the
	// loop below would poll it forever. Clear the cached PR fields so the next
	// pushAndCreatePR call creates a fresh PR instead of reusing the closed one.
	if prStatus.IsClosed {
		// Before assuming a closed-without-merging PR means the item's own
		// code needs fixing, check whether its work already landed on main
		// through some other path — the same BUG-032 shape, recurring: a PR
		// can be closed (by a human, or by an autonomous session itself,
		// e.g. running `gh pr close` directly from the worktree, bypassing
		// this reconciler entirely) specifically because it was already
		// superseded, not because it's broken. Without this check here,
		// AutoReopenForPRFix below would spawn a wasted rework cycle for
		// work that's already shipped — exactly the waste BUG-032 fixed for
		// the CI-failing/blocked/conflicting branch below, but missed for
		// this sibling "closed" branch. See BUG-036.
		supersededItemData := backlogItemToData(item)
		if superseded := l.closeIfSupersededByMain(ctx, g, &supersededItemData); superseded {
			return
		}

		closedPrURL, closedPrNum := item.PrURL, item.PrNumber
		// A closed-without-merging PR can never be pr_ready_unmerged again
		// under this pr_number; resolve immediately regardless of whether
		// the reopen below succeeds (self-heal would also catch this
		// once/if the status moves off pr_pending, but that may not
		// happen if no PRFixSpawner is configured below).
		l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending/closed")
		if fixSpawner == nil {
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s: PR #%d closed without merging but no PRFixSpawner configured", item.ID, closedPrNum)
			return
		}
		fixCtx := fmt.Sprintf("PR #%d (%s) was closed without merging. Investigate why, address any concerns, and open a fresh PR.", closedPrNum, closedPrURL)
		if !l.verifyPRAssociationForFixSpawn(ctx, item.ID.String(), item.RepoPath, closedPrNum) {
			fixCtx = unverifiedPRAssociationDisclaimer + fixCtx
		}
		log.InfoLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s → in_progress: PR #%d closed without merging", item.ID, closedPrNum)
		attempted, fixErr := l.remediatePRFixWithBackoffGate(ctx, er, fixSpawner, item.ID.String(), item.Title, fixCtx)
		if !attempted {
			// Backoff not yet due — same as before this fix existed for a
			// call that never happened: nothing was attempted, so nothing
			// downstream (the BUG-040 field-clearing below) applies. Retry
			// on a later tick once the gate opens.
			return
		}
		if fixErr != nil {
			// Do NOT clear the PR fields below — see BUG-040. A failed
			// reopen leaves the item in pr_pending; keeping the closed
			// PR's fields intact means the item is still visible/retryable
			// (and, once the pr_pending_no_pr detector below lands, would
			// have been caught even if this ordering fix regressed).
			log.ErrorLog().Printf("[BacklogLifecycle] ReconcilePRPending AutoReopenForPRFix (closed) item=%s: %v", item.ID, fixErr)
			return
		}

		// BUG-040: only clear the stale PR reference once AutoReopenForPRFix
		// is confirmed to have actually transitioned the item off
		// pr_pending. AutoReopenForPRFix has legitimate no-op paths (an
		// active work session already running, the rework cap) that return
		// nil without transitioning anything — clearing unconditionally
		// here (the pre-fix behavior) produced exactly this bug's dead end:
		// pr_pending with no PR reference and nothing left to retry, since
		// FindPRPendingItems' PrNumberGT(0) filter then excludes the item
		// from every future tick of this very function.
		refreshed, refreshErr := l.storage.GetBacklogItem(ctx, item.ID.String())
		if refreshErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending re-fetch after AutoReopenForPRFix (closed) item=%s: %v", item.ID, refreshErr)
			return
		}
		if BacklogStatus(refreshed.Status) == BacklogStatusPRPending {
			// A no-op guard fired inside AutoReopenForPRFix — leave the
			// closed PR reference in place so this is retried on a later
			// tick instead of being silently lost.
			log.InfoLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s: AutoReopenForPRFix (closed) left item in pr_pending; not clearing PR fields", item.ID)
			return
		}

		emptyURL, zeroNum := "", 0
		if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID.String(), BacklogItemUpdate{
			PrURL:                      &emptyURL,
			PrNumber:                   &zeroNum,
			ClearPrFeedbackAddressedAt: true,
		}, nil); updateErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] ReconcilePRPending clear closed PR fields item=%s: %v", item.ID, updateErr)
		}
		return
	}

	// hasNewFeedback is true only when there's substantive PR review
	// feedback (a COMMENTED review or plain comment) newer than the
	// per-item dedup watermark — so already-addressed feedback never
	// re-triggers a fix session on a later tick.
	hasNewFeedback := prStatus.HasReviewFeedback &&
		(item.PrFeedbackAddressedAt == nil || prStatus.LatestFeedbackAt.After(*item.PrFeedbackAddressedAt))

	if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts && !hasNewFeedback {
		// PR is open and healthy — wait for merge. Story 2.1.1: flag it
		// pr_ready_unmerged once it's been solo-ready (prReadyToMergeSolo)
		// past the threshold, using ONLY the already-fetched prStatus — no
		// second GitHub API call. Deliberately NOT gated on
		// github.DerivePRPriority(info)==PRPriorityReady, which requires
		// ApprovedCount>0 and is a permanent false-negative on a
		// self-authored single-user PR (pre-mortem F1; see
		// session/stuck_decisions.go prReadyToMergeSolo doc).
		info := &github.PRInfo{
			State:                 "open",
			IsDraft:               prStatus.IsDraft,
			ChangesRequestedCount: prStatus.ChangesRequestedCount,
			Mergeable:             prStatus.Mergeable,
			ApprovedCount:         prStatus.ApprovedCount,
		}
		if prStatus.CIFailing {
			info.CheckConclusion = "failure"
		}

		if prReadyToMergeSolo(info) {
			l.markPRReadyUnmerged(ctx, er, item.ID.String(), item.Title)
		} else {
			l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending")
		}
		// Poll-shaped resolve (pre-mortem F2): the PR is healthy again
		// while the item is still pr_pending — a same-status clear
		// selfHealStuck structurally cannot see (mirrors the
		// PRReadyUnmerged handling immediately above).
		l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRNeedsFix, "ReconcilePRPending/healthy")
		return
	}

	// Poll-shaped resolve (else-branch, pre-mortem F2): the PR just
	// became CI-failing/blocked/conflicting while the item is still
	// pr_pending — a same-status clear the status-anchored self-heal
	// sweep structurally cannot see.
	l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending/unhealthy")

	// 2c. Before spawning another "fix the PR" rework cycle, check whether this
	// item's own work already landed on main through some other path (BUG-032:
	// live incident where a PR kept failing CI/showing conflicts purely because
	// it had drifted stale behind an already-shipped fix — not because its own
	// code was wrong — and each "fix" cycle wasted a full rework+review round
	// against an empty/irrelevant diff before a human-equivalent check finally
	// caught it). Reuses the same IsCommitOnMain trust boundary
	// GetBacklogItemShipStatus already relies on elsewhere in this codebase for
	// "did this item's code actually ship" — not a new, less-verified standard.
	supersededItemData := backlogItemToData(item)
	if superseded := l.closeIfSupersededByMain(ctx, g, &supersededItemData); superseded {
		return
	}

	// 3. CI failure, review changes requested, or merge conflict → spawn fix session.
	if fixSpawner == nil {
		log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s: CI/review issues found but no PRFixSpawner configured", item.ID)
		return
	}
	fixCtx := fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", item.PrNumber, item.PrURL, prStatus.FeedbackText)
	if !l.verifyPRAssociationForFixSpawn(ctx, item.ID.String(), item.RepoPath, item.PrNumber) {
		fixCtx = unverifiedPRAssociationDisclaimer + fixCtx
	}
	log.InfoLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s → in_progress for PR fix (CI=%v, reviews=%v, conflict=%v, feedback=%v)",
		item.ID, prStatus.CIFailing, prStatus.HasBlockingReviews, prStatus.HasConflicts, hasNewFeedback)

	if hasNewFeedback {
		logFeedbackBatchCoverage(item.ID.String(), prStatus)
	}

	attempted, fixErr := l.remediatePRFixWithBackoffGate(ctx, er, fixSpawner, item.ID.String(), item.Title, fixCtx)
	if fixErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] ReconcilePRPending AutoReopenForPRFix item=%s: %v", item.ID, fixErr)
	} else if attempted && hasNewFeedback {
		watermark := prStatus.LatestFeedbackAt
		if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID.String(), BacklogItemUpdate{
			PrFeedbackAddressedAt: &watermark,
		}, nil); updateErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending persist PrFeedbackAddressedAt item=%s: %v", item.ID, updateErr)
		} else {
			log.InfoLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s PrFeedbackAddressedAt advanced to %s (PR #%d)", item.ID, watermark.Format(time.RFC3339), item.PrNumber)
		}
	}
}

// ReconcilePRPending polls items in pr_pending status. It transitions to done
// when the PR is merged, and spawns a fix session when CI fails or reviewers
// request changes.
func (l *BacklogLifecycleListener) ReconcilePRPending(ctx context.Context, er *EntRepository) {
	items, err := er.FindPRPendingItems(ctx)
	if err != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] ReconcilePRPending query error: %v", err)
		return
	}
	for _, item := range items {
		l.reconcilePRPendingItem(ctx, er, item)
	}
}

// findPRPendingItemForEvent resolves (repoFullName, prNumber) to a tracked
// pr_pending BacklogItem, matching on both the PR number AND the repo identity
// (a PR number collision across two different tracked repos must not match).
func findPRPendingItemForEvent(ctx context.Context, er *EntRepository, repoFullName string, prNumber int) (*ent.BacklogItem, bool) {
	items, err := er.FindPRPendingItems(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] findPRPendingItemForEvent FindPRPendingItems: %v", err)
		return nil, false
	}
	for _, item := range items {
		if item.PrNumber != prNumber {
			continue
		}
		ref, refErr := github.GetOwnerRepoFromRemote(item.RepoPath)
		if refErr != nil || !ref.IsValid() {
			continue
		}
		if ref.String() == repoFullName {
			return item, true
		}
	}
	return nil, false
}

// TriggerPRFixForEvent satisfies services.PRFixEventRouter (defined in the
// consuming package, per the `interface-pollution-checklist` skill). It
// looks up the pr_pending item tracking (repoFullName, prNumber) and, if
// found, runs the same per-item reconciliation ReconcilePRPending's 60s tick
// would eventually run for it — see ADR-002 for why the full body (merge
// check included) is reused, not just the CI/review-check half.
// TriggerPRFixForEvent's item lookup runs on the caller's ctx (a fast DB query,
// safe to share the HTTP request's lifetime), but the actual reconciliation is
// dispatched onto l.shutdownCtx in a goroutine rather than run inline. reconcilePRPendingItem
// makes multiple sequential GitHub API calls and can spawn a full session (worktree +
// tmux), which routinely exceeds GitHub's ~10s webhook delivery timeout; running it on
// r.Context() would risk GitHub reporting the delivery failed/retried mid-reconciliation
// and cancelling that work partway through (e.g. AutoReopenForPRFix's "restore original
// notes regardless of spawn outcome" step would itself be cancelled). This mirrors the
// existing async-dispatch shape every other remediation call site in this file already
// uses (e.g. retryPushFailedWithBackoffGate below). matched=true means "a tracked item
// was found and reconciliation was queued," not "reconciliation completed" — consistent
// with fired_success's existing documented scope (see the webhook handler's Observability
// Plan: it was never meant to imply a fix session was spawned).
func (l *BacklogLifecycleListener) TriggerPRFixForEvent(ctx context.Context, repoFullName string, prNumber int) (matched bool, err error) {
	if !l.enabled.Load() {
		return false, nil
	}
	er := l.storage.repo
	item, found := findPRPendingItemForEvent(ctx, er, repoFullName, prNumber)
	if !found {
		return false, nil
	}
	log.InfoLog().Printf("[BacklogLifecycle] TriggerPRFixForEvent item=%s repo=%s pr=%d: reconciling now (webhook-triggered)", item.ID, repoFullName, prNumber)
	go func() {
		l.reconcilePRPendingItem(withPRFixTriggerSource(l.shutdownCtx, prFixTriggerSourceWebhook), er, item)
	}()
	return true, nil
}

// prFixTriggerSourceKey tags a reconciliation's ctx with which caller triggered it
// (ReconcilePRPending's tick loop vs. TriggerPRFixForEvent's webhook call), so
// remediatePRFixWithBackoffGate's fix-attempt log can distinguish them for AC8's
// webhook-vs-poller metric — TriggerFireEvent rows alone only see the webhook side.
type prFixTriggerSourceKey struct{}

const (
	prFixTriggerSourceWebhook = "webhook"
	prFixTriggerSourcePoller  = "poller"
)

func withPRFixTriggerSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, prFixTriggerSourceKey{}, source)
}

// prFixTriggerSourceFrom defaults to "poller" — ReconcilePRPending's loop (the only
// other caller of reconcilePRPendingItem) doesn't tag its ctx, since that's the
// baseline 60s-tick path.
func prFixTriggerSourceFrom(ctx context.Context) string {
	if v, ok := ctx.Value(prFixTriggerSourceKey{}).(string); ok && v != "" {
		return v
	}
	return prFixTriggerSourcePoller
}

// logFeedbackBatchCoverage logs the count and authors of every substantive
// feedback item (commentReviews review + substantive plain comments) included
// in a hasNewFeedback dispatch, but only when the dispatch covers more than
// one — a single-item dispatch needs no extra log line. The timestamp
// watermark this feature uses for dedup advances past the whole batch on
// dispatch, not per-item, so a partially-addressed multi-item batch is
// otherwise silently unresolved forever; this line is the one place an
// operator can discover it happened (pre-mortem.md P1).
func logFeedbackBatchCoverage(itemID string, prStatus *git.PRStatus) {
	authors := prStatus.FeedbackAuthors()
	if len(authors) <= 1 {
		return
	}
	log.InfoLog().Printf("[BacklogLifecycle] ReconcilePRPending item=%s dispatching PR-fix session covering %d feedback item(s) from [%s] — watermark advances to %s regardless of which items the session actually addresses",
		itemID, len(authors), strings.Join(authors, ", "), prStatus.LatestFeedbackAt.Format(time.RFC3339))
}

// verifyPRHeadBranchMatchesTracked re-verifies, via a live GitHub lookup,
// that prNumber's real head branch still equals the item's currently-tracked
// branch (trackedBranch) — the guard Story 6 adds in response to
// adversarial-review.md's Blocker, called immediately before any of
// closeIfSupersededByMain/ReconcilePRPending/reconcileBouncingItems treats
// item.PrNumber as ground truth for an automated GitHub-mutating or
// completing action. Fails closed in both directions: an empty
// trackedBranch (the caller couldn't resolve the item's own tracked branch)
// returns false without even calling the finder, and a finder error (e.g. a
// transient GitHub failure) also returns false — neither is ever read as a
// verified match.
func (l *BacklogLifecycleListener) verifyPRHeadBranchMatchesTracked(ctx context.Context, repoPath, trackedBranch string, prNumber int) (bool, error) {
	if trackedBranch == "" {
		return false, fmt.Errorf("verifyPRHeadBranchMatchesTracked: no tracked branch to verify PR #%d against", prNumber)
	}
	info, err := l.getPRByNumberFinder()(ctx, repoPath, prNumber)
	if err != nil {
		return false, err
	}
	return info.HeadRef == trackedBranch, nil
}

// unverifiedPRAssociationDisclaimer is prepended (Task 6.3a) to a spawned
// fix session's context whenever verifyPRAssociationForFixSpawn can't
// confirm a PR's head branch still matches the item's tracked branch —
// disclosing that the association is unverified rather than briefing the
// spawned session to investigate/fix it as established fact.
const unverifiedPRAssociationDisclaimer = "NOTE: this PR's association with this backlog item could not be verified (its head branch does not match — or no longer matches — the item's tracked branch, possibly because it was linked via report_pr_created's override_reason path). Confirm this PR is actually relevant to this item's work before investigating or commenting on it. "

// verifyPRAssociationForFixSpawn independently resolves itemIDStr's
// currently-tracked branch (its most recent work session's worktree data,
// mirroring closeIfSupersededByMain's identical session-lookup loop) and
// re-runs verifyPRHeadBranchMatchesTracked against prNumber. Used at Task
// 6.3a's two fixCtx-building call sites in ReconcilePRPending and Task 6.5's
// reconcileBouncingItems done-transition guard — deliberately re-run rather
// than threaded through closeIfSupersededByMain's return value, since that
// function returns false for several reasons unrelated to branch
// verification and its return value alone can't distinguish "guard tripped"
// from "nothing to verify yet" (see plan.md's Task 6.3a rationale). Fails
// closed identically to verifyPRHeadBranchMatchesTracked's own contract: no
// work session, a GetWorktreeDataBySessionUUID error, or the guard itself
// erroring all count as "unverified", never "verified".
func (l *BacklogLifecycleListener) verifyPRAssociationForFixSpawn(ctx context.Context, itemIDStr, repoPath string, prNumber int) bool {
	sessions, sessErr := l.storage.ListItemSessions(ctx, itemIDStr)
	if sessErr != nil {
		return false
	}
	var lastWork *ItemSessionSummary
	for i := range sessions {
		// Ascending by CreatedAt (ListItemSessions' query order) — keep
		// overwriting so this ends up holding the *most recent* work
		// session, mirroring the identical pattern elsewhere in this file.
		if sessions[i].Role == SessionRoleWork {
			lastWork = &sessions[i]
		}
	}
	if lastWork == nil {
		return false
	}
	wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)
	if wtErr != nil {
		return false
	}
	matches, verifyErr := l.verifyPRHeadBranchMatchesTracked(ctx, repoPath, wt.BranchName, prNumber)
	return verifyErr == nil && matches
}

// closeIfSupersededByMain checks whether item's last known work-session commit
// has already landed on mainBranch through some other path (BUG-032: live
// incident where a PR kept failing CI/showing conflicts purely because it had
// drifted stale behind an already-shipped fix — not because its own code was
// wrong — and each "fix" cycle wasted a full rework+review round against an
// empty/irrelevant diff before a manual check finally caught it). Reuses the
// same IsCommitOnMain trust boundary GetBacklogItemShipStatus already relies
// on elsewhere in this codebase for "did this item's code actually ship" —
// this is not a new, less-verified standard, just a new call site for an
// existing one.
//
// Returns true if the item was closed out this way (caller should skip its
// own CI-fix-spawn handling for this item this tick). Returns false — the
// caller proceeds with its normal path — whenever this can't be determined:
// no work session, no recorded commit SHA, an IsCommitOnMain error, or the
// commit genuinely isn't on main yet.
//
// BUG-047: this check must never run against the session's pre-work base
// commit. That commit is by construction already an ancestor of main, so
// IsCommitOnMain on it is unconditionally true — and until LastCommitSha was
// split from BaseCommitSha and given a live refresh
// (refreshWorkSessionGitActivity), the base SHA was the only value the field
// ever held. Confirmed live 2026-08-05: item d6ddbef3's real fix on branch
// backlog/stapler-squad-fix-idle-reviewer-wedge (PR #342, reviewed and
// CI-green) was closed unmerged as "superseded" against base SHA 1a751723 — an
// unrelated commit from ~24h before that work even started. Hence both the
// resolveLatestWorkCommit call and the explicit BaseCommitSha guard below: the
// guard is the belt to the refresh's braces, so a session whose HEAD cannot be
// resolved this tick can never fall back onto its own base commit and close a
// live PR. Best-effort throughout: secondary
// failures (the GitHub close call, the field clear) are logged, never block
// the done transition, which is the one write that actually matters once the
// commit is confirmed shipped.
func (l *BacklogLifecycleListener) closeIfSupersededByMain(ctx context.Context, checker prPendingChecker, item *BacklogItemData) bool {
	sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] closeIfSupersededByMain ListItemSessions item=%s: %v", item.ID, sessErr)
		return false
	}
	var lastWork *ItemSessionSummary
	for i := range sessions {
		// Ascending by CreatedAt (ListItemSessions' query order) — keep
		// overwriting so this ends up holding the *most recent* work session,
		// mirroring the identical pattern elsewhere in this file.
		if sessions[i].Role == SessionRoleWork {
			lastWork = &sessions[i]
		}
	}
	if lastWork == nil {
		return false
	}

	// Resolve the session's real current tip rather than trusting a stored
	// field, and refuse to act on the pre-work base commit under any
	// circumstances — see this function's doc comment (BUG-047).
	lastCommitSha := l.resolveLatestWorkCommit(ctx, lastWork.SessionUUID, item.RepoPath)
	if lastCommitSha == "" {
		lastCommitSha = lastWork.LastCommitSha
	}
	if lastCommitSha == "" || lastCommitSha == lastWork.BaseCommitSha {
		return false
	}
	// BUG-065: the guard above only fires when BaseCommitSha is a real,
	// non-empty value. BaseCommitSha reads "" both for ItemSession rows written
	// before BaseCommitSha/LastCommitSha were split into separate fields, and
	// for any session that spawned and died/was retried before its base commit
	// was ever seeded — GetBaseCommitSHAsForSessions (storage_backlog.go)
	// already documents and works around this exact legacy-row shape. When
	// BaseCommitSha is unknown, fall back to this session's own bookkeeping of
	// whether it ever authored anything: CommitCountSinceSpawn is written
	// alongside LastCommitSha every reconciliation tick
	// (refreshWorkSessionGitActivity) and is 0 for a session that has made no
	// commits since spawn, regardless of what lastCommitSha resolves to. A
	// resolved commit from a session with zero commits since spawn is, by
	// construction, that session's own pre-work snapshot — not real authored
	// work — the same conclusion the BaseCommitSha check above reaches when it
	// has the data to reach it at all.
	//
	// Live incident 2026-08-06: this exact gap closed PR #307 (a real,
	// reviewed, CI-green "user-extensible agent detection plugins" feature,
	// commit c64d94cf8) as "superseded" against 32f504c803 — that session's own
	// spawn-time base, resolved fresh from a worktree that had never advanced —
	// because BaseCommitSha read empty for that row, letting the base commit
	// slip straight through the equality guard with no fallback to catch it.
	if lastWork.BaseCommitSha == "" && lastWork.CommitCountSinceSpawn == 0 {
		return false
	}

	onMain, mainErr := git.IsCommitOnMain(item.RepoPath, bounceMainBranch, lastCommitSha)
	if mainErr != nil {
		log.DebugLog().Printf("[BacklogLifecycle] closeIfSupersededByMain IsCommitOnMain item=%s sha=%s: %v", item.ID, lastCommitSha, mainErr)
		return false
	}
	if !onMain {
		return false
	}

	log.WarningLog().Printf("[BacklogLifecycle] closeIfSupersededByMain item=%s: last commit %s is already on %s — PR #%d is superseded, closing instead of spawning another fix cycle",
		item.ID, lastCommitSha, bounceMainBranch, item.PrNumber)

	// Story 6 guard (adversarial-review.md's Blocker): re-verify, via a live
	// GitHub lookup, that PR #item.PrNumber's head branch still matches this
	// item's currently-tracked branch before auto-closing it. Without this,
	// a PR attached via report_pr_created's override_reason path (by
	// construction, a head-branch mismatch) could be auto-closed on the
	// strength of item.PrNumber alone.
	wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)
	if wtErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] closeIfSupersededByMain item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-close (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)
		return false
	}
	if matches, verifyErr := l.verifyPRHeadBranchMatchesTracked(ctx, item.RepoPath, wt.BranchName, item.PrNumber); verifyErr != nil || !matches {
		log.WarningLog().Printf("[BacklogLifecycle] closeIfSupersededByMain item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-close (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)
		return false
	}

	closeComment := fmt.Sprintf(
		"Closing as superseded: this branch's last known commit (%s) is already present on %s, so this item's work has already shipped through another path. No further fix is needed here.",
		lastCommitSha, bounceMainBranch)
	if closeErr := checker.ClosePR(item.PrNumber, closeComment); closeErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] closeIfSupersededByMain ClosePR item=%s pr=%d: %v", item.ID, item.PrNumber, closeErr)
		// Still proceed — the item's code is on main regardless of whether the
		// close-comment API call itself succeeded.
	}

	closedPrNum := item.PrNumber
	emptyURL, zeroNum := "", 0
	if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &emptyURL,
		PrNumber: &zeroNum,
	}, nil); updateErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] closeIfSupersededByMain clear PR fields item=%s: %v", item.ID, updateErr)
	}

	precondition := &BacklogItemPrecondition{
		ExpectedStatus: string(BacklogStatusPRPending),
		Note: fmt.Sprintf("self-heal: PR #%d closed as superseded — commit %s already on %s",
			closedPrNum, lastCommitSha, bounceMainBranch),
	}
	if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, precondition, TriggeredBySystem); transErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] closeIfSupersededByMain done transition item=%s: %v", item.ID, transErr)
		return false
	}

	l.notify(item.ID,
		"Backlog item already shipped — stale PR closed",
		fmt.Sprintf("%s — PR #%d had fallen behind an already-shipped fix; closed as superseded and marked done automatically.", item.Title, closedPrNum),
		10, // sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
		1,  // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
	)
	return true
}

// markPRReadyUnmerged marks/refreshes the durable pr_ready_unmerged row for
// itemID and notifies once it has been solo-ready (stuckPRReady) past
// prReadyThreshold — DB-backed notify-once dedup via notified_at, and
// first_detected_at survives restarts (unlike a process-uptime timer).
// Best-effort: errors are logged, never returned.
func (l *BacklogLifecycleListener) markPRReadyUnmerged(ctx context.Context, er *EntRepository, itemID, itemTitle string) {
	applied, err := er.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending,
		"PR is green, mergeable, and unmerged")
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] markPRReadyUnmerged MarkStuck item=%s: %v", itemID, err)
		return
	}
	if !applied {
		return
	}
	rows, findErr := er.FindOpenStuckStates(ctx)
	if findErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] markPRReadyUnmerged FindOpenStuckStates item=%s: %v", itemID, findErr)
		return
	}
	row, ok := findOpenStuckStateFor(rows, itemID, domain.StuckReasonPRReadyUnmerged)
	if !ok || row.NotifiedAt != nil || !stuckPRReady(row.FirstDetectedAt, time.Now()) {
		return
	}
	log.InfoLog().Printf("[BacklogLifecycle] item %s PR #%d ready to merge (unmerged past threshold)", itemID, row.PrNumber)
	l.notify(itemID,
		"PR ready to merge",
		fmt.Sprintf("%s — PR #%d is green, mergeable, and has been ready to merge for over %s. Merge it on GitHub.", itemTitle, row.PrNumber, prReadyThreshold),
		8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
		2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
	)
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonPRReadyUnmerged); notifyErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] markPRReadyUnmerged MarkStuckNotified item=%s: %v", itemID, notifyErr)
	}
}
