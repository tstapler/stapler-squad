package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// --- report_pr_created ---

// reportPRCreatedAllowedSourceStatuses is the whitelist of source statuses
// report_pr_created may act on. Consulted before any PR verification or
// storage write so a structurally ineligible status (e.g. ready, idea, done)
// gets a specific rejection instead of falling through to the generic
// CAS-race message.
//
// Scope boundary (Story 2.1.4): intentionally built-in-only, not sourced
// from any stage engine — same "does not automatically inherit" resolution
// as allowedSelfResolveSourceStatuses in tools_backlog.go.
var reportPRCreatedAllowedSourceStatuses = map[session.BacklogStatus]bool{
	session.BacklogStatusReview:    true,
	session.BacklogStatusPRPending: true,
}

// sessionBranch resolves the branch sessionUUID is working on, via the
// overridable resolveSessionBranch seam when set, otherwise the real
// worktree lookup. See backlogHandlers.resolveSessionBranch's doc comment.
func (h *backlogHandlers) sessionBranch(ctx context.Context, sessionUUID string) (string, error) {
	if h.resolveSessionBranch != nil {
		return h.resolveSessionBranch(ctx, sessionUUID)
	}
	wt, err := h.storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	if err != nil {
		return "", err
	}
	return wt.BranchName, nil
}

// verifyPR runs the GitHub cross-check via the overridable verifyPRMatchesBranch
// seam when set, otherwise the real VerifyPRMatchesBranch (tools_github.go).
func (h *backlogHandlers) verifyPR(ctx context.Context, owner, repo string, prNumber int, expectedBranch string) (PRVerification, error) {
	if h.verifyPRMatchesBranch != nil {
		return h.verifyPRMatchesBranch(ctx, owner, repo, prNumber, expectedBranch)
	}
	return VerifyPRMatchesBranch(ctx, owner, repo, prNumber, expectedBranch)
}

// callerGitHubLogin resolves the GitHub login this server is authenticated
// as, via the overridable resolveCallerGitHubLogin seam when set, otherwise
// the real githubpkg.GetCurrentUserLogin.
func (h *backlogHandlers) callerGitHubLogin(ctx context.Context) (string, error) {
	if h.resolveCallerGitHubLogin != nil {
		return h.resolveCallerGitHubLogin(ctx)
	}
	return githubpkg.GetCurrentUserLogin(ctx)
}

// decideOverridePolicy is the pure decision function behind
// report_pr_created's fallback-branch override path. It takes the
// GitHub-verified PRVerification plus the caller's override_reason and
// resolved GitHub identity, and decides whether the self-reported PR may be
// recorded even though its head branch (per GitHub) doesn't match this
// item's tracked branch. It is a pure function of its three inputs (no ctx,
// no I/O) so the branching itself — the part architecture-review.md flagged
// as needing isolation from reportPRCreated's storage/item/session
// machinery — is directly unit-testable (see TestDecideOverridePolicy).
//
// Every accept==false outcome here ends up surfaced by reportPRCreated as
// ErrInvalidArgument; code is not that MCP-level code but an internal
// discriminator distinguishing *which* of the four rejection reasons fired,
// since reportPRCreated needs to build a different, prNumber/branch-specific
// message for each and none of that request context is available inside
// this function. msg carries whatever case-specific text decideOverridePolicy
// *can* build from v/overrideReason/callerLogin alone; reportPRCreated
// composes the final, exact message around it.
//
// Check ordering — exists, then matched (fast path), then reason, then
// author, then state — is load-bearing: existence can never be overridden,
// so it's checked first regardless of everything else. Author-match is
// checked before the state gate so a PR failing both surfaces the more
// fundamental "this isn't your PR" reason rather than a "not open/merged,
// try again later" reason that invites a misleading retry.
//
// forceOverride skips the Matched fast path — reportPRCreated sets it true
// on the reassignment path (item already pr_pending, correcting the tracked
// PR to a different number), where a matching branch alone is not enough:
// AC1/AC9 require override_reason and self-authorship on every reassignment,
// not just ones whose head branch happens to differ from the tracked branch.
func decideOverridePolicy(v PRVerification, overrideReason, callerLogin string, forceOverride bool) (accept bool, code connect.Code, msg string) {
	if !v.Exists {
		return false, connect.CodeNotFound, "PR does not exist"
	}
	if v.Matched && !forceOverride {
		return true, connect.Code(0), ""
	}
	if overrideReason == "" {
		return false, connect.CodeInvalidArgument, "override_reason is required when the PR's head branch does not match this item's tracked branch"
	}
	if callerLogin == "" || v.Author == "" || v.Author != callerLogin {
		return false, connect.CodePermissionDenied, fmt.Sprintf(
			"PR was authored by %q, not your own GitHub identity (%q) — the override path can only attach PRs you authored yourself. Refusing to record it.",
			v.Author, callerLogin)
	}
	if v.State != githubpkg.PRStateOpen && v.State != githubpkg.PRStateMerged {
		return false, connect.CodeFailedPrecondition, fmt.Sprintf(
			"PR is %s (not open or merged) — refusing to record it even with override_reason.", v.State)
	}
	return true, connect.Code(0), ""
}

// reportPRCreated records a PR the calling work session created itself
// (typically via /backlog:ship -> gh pr create, outside the mechanical
// pushAndCreatePR path — see session/backlog_lifecycle.go) back onto the
// backlog item. Role: work only. This closes the gap named in "PR Metadata
// Capture Fix" (project_plans/backlog-agent-communication, Epic 3.1): only
// the system-driven mechanical push path used to write pr_url/pr_number, so
// an agent-driven PR could exist on GitHub with the item never reflecting
// it. See SetBacklogItemPRAndTransition for the shared primary-write path
// (also used by the reconciliation backstop, Epic 3.2) and
// VerifyPRMatchesBranch for the GitHub cross-check this handler performs
// before trusting the self-reported pr_url/pr_number.
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity relocated verbatim by the tools_backlog.go split (sdd:fix-hotspot, 2026-09-12); reducing it is a separate follow-up, not a file move
func (h *backlogHandlers) reportPRCreated(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	callerUUID, err := callerSessionUUID(ctx)
	if err != nil {
		return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
	}

	args := req.GetArguments()

	itemID, ok := args["item_id"].(string)
	if !ok || itemID == "" {
		return errResult(ErrInvalidArgument, "item_id is required", ""), nil
	}
	if err := validateUUID(itemID); err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}

	prURL, ok := args["pr_url"].(string)
	if !ok || prURL == "" {
		return errResult(ErrInvalidArgument, "pr_url is required", ""), nil
	}

	prNumberF, ok := args["pr_number"].(float64)
	if !ok || prNumberF <= 0 {
		return errResult(ErrInvalidArgument, "pr_number is required and must be > 0", ""), nil
	}
	prNumber := int(prNumberF)

	summary, ok := args["summary"].(string)
	if !ok || summary == "" {
		return errResult(ErrInvalidArgument, "summary is required", ""), nil
	}
	if len(summary) > 1000 {
		return errResult(ErrInvalidArgument, "summary must be <= 1000 characters", ""), nil
	}

	// override_reason is optional — only required when GitHub's view of the
	// PR's head branch doesn't match this item's tracked branch. See
	// decideOverridePolicy below.
	overrideReason, _ := args["override_reason"].(string)
	overrideReason = strings.TrimSpace(overrideReason)
	if len(overrideReason) > 500 {
		return errResult(ErrInvalidArgument, "override_reason must be <= 500 characters", ""), nil
	}

	// Verify session is linked to item (disambiguates ITEM_NOT_FOUND vs PERMISSION_DENIED).
	itemSession, errRes := h.resolveItemLink(ctx, callerUUID, itemID)
	if errRes != nil {
		return errRes, nil
	}
	if itemSession.Role != session.SessionRoleWork {
		return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'work' role may report a created PR", itemSession.Role), ""), nil
	}

	item, getErr := h.getBacklogItemFor(ctx, itemID)
	if getErr != nil {
		if errors.Is(getErr, session.ErrNotFound) {
			return errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), ""), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("get backlog item: %v", getErr), ""), nil
	}

	// AC6: reject a structurally ineligible status with a message naming the
	// item's actual status, before any PR verification or storage write is
	// attempted — without this, an item in e.g. ready/idea/done would fall
	// through to SetBacklogItemPRAndTransition's ErrPreconditionFailed and
	// surface as the generic "item state changed since your last read..."
	// CAS-race message below, indistinguishable from a genuine concurrent
	// write race (AC7's message, which this check must not alter).
	if !reportPRCreatedAllowedSourceStatuses[session.BacklogStatus(item.Status)] {
		return errResult(ErrInvalidArgument, fmt.Sprintf(
			"item %s is at status %q — report_pr_created is only allowed from status 'review' or 'pr_pending'", itemID, item.Status), ""), nil
	}

	// Idempotency: already pr_pending with this exact PR number is a no-op success.
	if item.Status == string(session.BacklogStatusPRPending) && item.PrNumber == prNumber {
		return mcpgo.NewToolResultText(fmt.Sprintf(
			"PR #%d already recorded for item %s (status already pr_pending) — no changes made.", prNumber, itemID,
		)), nil
	}

	// Reassignment: the item already has a *different* PR tracked
	// (status already pr_pending — the idempotent same-number case returned
	// above). This requires strictly more than the first-time recording
	// path: override_reason is mandatory unconditionally (AC1, checked here
	// before any network call — even a matching branch doesn't excuse it),
	// and the currently tracked PR is hard-checked for already being merged
	// (AC2) before anything else, since a merged PR's association must never
	// be silently swapped.
	isReassignment := item.Status == string(session.BacklogStatusPRPending)
	if isReassignment && overrideReason == "" {
		return errResult(ErrInvalidArgument, fmt.Sprintf(
			"item %s already has PR #%d tracked (status pr_pending) — reassigning it to PR #%d requires override_reason explaining why, even if the new PR's branch matches this item's tracked branch. "+
				"Retry with override_reason set, e.g. override_reason=\"tracked branch was polluted by another session; opened a clean PR instead and closed the original\".",
			itemID, item.PrNumber, prNumber), ""), nil
	}
	if isReassignment && item.PrNumber > 0 {
		// Fail CLOSED on a parse failure here, same as a verification
		// failure below — this is the one check the tool description
		// promises has no override, so an unparseable stored PrURL must
		// never silently skip it and let the reassignment through.
		curRef, curParseErr := session.ParseGitHubURLWithHosts(item.PrURL, h.enterpriseHosts())
		if curParseErr != nil {
			return errResult(ErrInternalError, fmt.Sprintf(
				"could not parse the currently tracked PR URL (%q) to verify it isn't merged before reassigning — retry, or contact an operator if this persists: %v",
				item.PrURL, curParseErr), ""), nil
		}
		curVerification, curErr := h.verifyPR(ctx, curRef.Owner, curRef.Repo, item.PrNumber, "")
		if curErr != nil {
			return errResult(ErrInternalError, fmt.Sprintf("could not verify the currently tracked PR #%d against GitHub — retry: %v", item.PrNumber, curErr), ""), nil
		}
		if curVerification.State == githubpkg.PRStateMerged {
			return errResult(ErrInvalidArgument, fmt.Sprintf(
				"item %s's currently tracked PR #%d is already merged — refusing to reassign it to PR #%d, even with override_reason. "+
					"A merged PR's association with this item cannot be changed; open a new backlog item if further work is needed.",
				itemID, item.PrNumber, prNumber), ""), nil
		}
	}

	// Parse the reported URL to extract owner/repo, and cross-check it
	// against the reported pr_number — a typo'd URL/number pair fails fast
	// here, before any network call.
	ref, parseErr := session.ParseGitHubURLWithHosts(prURL, h.enterpriseHosts())
	if parseErr != nil || ref.Owner == "" || ref.Repo == "" {
		return errResult(ErrInvalidArgument, fmt.Sprintf("pr_url is not a recognizable GitHub PR URL: %v", parseErr), ""), nil
	}
	if ref.PRNumber != 0 && ref.PRNumber != prNumber {
		return errResult(ErrInvalidArgument, fmt.Sprintf("pr_url references PR #%d but pr_number=%d was given — these must match", ref.PRNumber, prNumber), ""), nil
	}

	// Resolve this session's own branch to verify the reported PR against —
	// the whole point of this check is to refuse a self-reported PR number
	// for a branch that isn't even this item's own.
	branch, branchErr := h.sessionBranch(ctx, callerUUID)
	if branchErr != nil || branch == "" {
		return errResult(ErrInternalError, "could not resolve this session's git branch to verify the reported PR", ""), nil
	}

	verification, verifyErr := h.verifyPR(ctx, ref.Owner, ref.Repo, prNumber, branch)
	if verifyErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("could not verify PR #%d against GitHub — retry: %v", prNumber, verifyErr), ""), nil
	}

	// Only resolve the caller's own GitHub identity when we're actually on a
	// path decideOverridePolicy could accept: never on the fast path (no
	// identity lookup needed — the item's own tracked branch is already
	// trusted), never when the PR doesn't exist (existence can never be
	// overridden regardless of authorship), and never when override_reason
	// is empty (a call already doomed to reject for a missing reason
	// shouldn't pay for a GitHub API call it doesn't need). On the
	// reassignment path (AC9), the identity check is mandatory even when the
	// branch matches — isReassignment is already only ever true here with a
	// non-empty overrideReason (the early reject above guarantees it).
	var callerLogin string
	if verification.Exists && overrideReason != "" && (isReassignment || !verification.Matched) {
		login, loginErr := h.callerGitHubLogin(ctx)
		if loginErr != nil {
			return errResult(ErrInternalError, fmt.Sprintf("could not resolve your GitHub identity to verify the override — retry: %v", loginErr), ""), nil
		}
		callerLogin = login
	}

	accept, code, _ := decideOverridePolicy(verification, overrideReason, callerLogin, isReassignment)
	if !accept {
		var msg string
		switch code {
		case connect.CodeNotFound:
			msg = fmt.Sprintf("PR #%d does not exist in %s/%s on GitHub — refusing to record it. Double-check the PR number/URL.",
				prNumber, ref.Owner, ref.Repo)
		case connect.CodePermissionDenied:
			msg = fmt.Sprintf(
				"PR #%d was authored by %q, not your own GitHub identity (%q) — the override path can only attach PRs you authored yourself. Refusing to record it.",
				prNumber, verification.Author, callerLogin)
		case connect.CodeFailedPrecondition:
			msg = fmt.Sprintf("PR #%d is %s (not open or merged) — refusing to record it even with override_reason.",
				prNumber, verification.State)
		default: // connect.CodeInvalidArgument — missing override_reason (AC3)
			msg = fmt.Sprintf(
				"PR #%d's head branch on GitHub is %q, not this item's tracked branch %q — refusing to record it. "+
					"If %q was polluted (e.g. by another session sharing this worktree) and you opened this PR from a clean fallback branch instead, "+
					"retry this exact call with an additional override_reason argument explaining why, e.g. "+
					"override_reason=\"tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead\". "+
					"The override path additionally requires that PR #%d was authored by your own GitHub identity — it cannot be used to attach a PR someone/something else opened. "+
					"If PR #%d is unrelated to this item, do not retry — find and report the correct PR instead.",
				prNumber, verification.ActualHeadBranch, branch, branch, prNumber, prNumber)
		}
		return errResult(ErrInvalidArgument, msg, ""), nil
	}

	// Build the reassignment guard from what was already verified above:
	// override_reason is non-empty (rejected earlier otherwise), the
	// currently tracked PR is confirmed not merged (rejected earlier
	// otherwise — or there was no PR to check), and decideOverridePolicy
	// just accepted with forceOverride=true, which requires
	// verification.Author == callerLogin. nil when this isn't a
	// reassignment — SetBacklogItemPRAndTransition ignores it in that case.
	var guard *session.PRReassignmentGuard
	if isReassignment {
		guard = &session.PRReassignmentGuard{
			OverrideReason:      overrideReason,
			CurrentPRMerged:     false,
			NewPRAuthorVerified: true,
		}
	}

	if setErr := h.storage.SetBacklogItemPRAndTransition(ctx, item, prURL, prNumber, summary, guard); setErr != nil {
		if errors.Is(setErr, session.ErrPreconditionFailed) {
			// AC8: the item's status changed out from under this call between
			// our read above and the atomic write (another action resolved it,
			// or a racing report_pr_created call won first) — a friendly,
			// actionable message, not a raw internal error (mirrors
			// report_duplicate's identical CAS-failure message).
			return errResult(ErrInternalError, "item state changed since your last read (another action already resolved it, or a concurrent report_pr_created call won first) — call get_backlog_item to see its current status", ""), nil
		}
		if errors.Is(setErr, session.ErrPRReassignmentNotAllowed) {
			// Should be unreachable — this handler always constructs a valid
			// guard whenever isReassignment is true — but surfaces distinctly
			// rather than as a generic internal error if storage's own
			// contract check ever disagrees with the handler's.
			return errResult(ErrInternalError, fmt.Sprintf("reassignment rejected by storage layer: %v", setErr), ""), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("record PR: %v", setErr), ""), nil
	}

	log.InfoLog().Printf("[mcp:report_pr_created] session=%s item=%s PR #%d %s", callerUUID, itemID, prNumber, prURL)

	if isReassignment || !verification.Matched {
		// The override path was actually taken (not the fast path) — audit
		// it, since this path has no technical human gate. Gated on
		// isReassignment too (not just !verification.Matched): a
		// same-branch reassignment still went through the mandatory
		// override_reason + author-identity check (forceOverride), so it
		// must be audited even though verification.Matched is true.
		log.Warn("report_pr_created: recording PR via override",
			"session", callerUUID,
			"item", itemID,
			"reassignment", isReassignment,
			"previous_pr_number", item.PrNumber,
			"pr_number", prNumber,
			"actual_head_branch", verification.ActualHeadBranch,
			"tracked_branch", branch,
			"pr_author", verification.Author,
			"override_reason", overrideReason,
		)
	}

	return mcpgo.NewToolResultText(fmt.Sprintf(
		"PR #%d recorded for item %s. Item transitioned to pr_pending.", prNumber, itemID,
	)), nil
}
