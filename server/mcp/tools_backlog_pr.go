package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/wakes"
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
func (h *backlogHandlers) verifyPR(ctx context.Context, ref githubpkg.RepoRef, prNumber int, expectedBranch string) (PRVerification, error) {
	if h.verifyPRMatchesBranch != nil {
		return h.verifyPRMatchesBranch(ctx, ref, prNumber, expectedBranch)
	}
	return VerifyPRMatchesBranch(ctx, ref, prNumber, expectedBranch)
}

// callerGitHubLogin resolves the GitHub login this server is authenticated
// as on host ("" means github.com), via the overridable resolveCallerGitHubLogin
// seam when set (host is ignored on that seam — tests only ever exercise
// github.com), otherwise the real githubpkg.GetCurrentUserLogin.
func (h *backlogHandlers) callerGitHubLogin(ctx context.Context, host string) (string, error) {
	if h.resolveCallerGitHubLogin != nil {
		return h.resolveCallerGitHubLogin(ctx)
	}
	return githubpkg.GetCurrentUserLogin(ctx, host)
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

// prProvenanceStampTimeout bounds the provenance comment so a slow GitHub call
// cannot hold up report_pr_created's response.
const prProvenanceStampTimeout = 10 * time.Second

// stampPRProvenance comments the item's deep link and this host's opaque ID on
// the PR so any host can trace it back (webhook read-back: handlePRFixEvent).
// Best-effort: the PR/item transition already succeeded, so a failure is logged
// and never returned.
func (h *backlogHandlers) stampPRProvenance(ctx context.Context, item *session.BacklogItemData, ref githubpkg.RepoRef, prNumber int) {
	body, ok := h.storage.PRProvenanceComment(item)
	if !ok {
		return
	}
	post := h.postPRComment
	if post == nil {
		post = githubpkg.PostPRCommentREST
	}
	stampCtx, cancel := context.WithTimeout(ctx, prProvenanceStampTimeout)
	defer cancel()
	if err := post(stampCtx, ref, prNumber, body); err != nil {
		log.Warn("report_pr_created.provenance_stamp_failed", "item", item.ID, "pr_number", prNumber, "err", err)
		return
	}
	log.Info("report_pr_created.provenance_stamped", "item", item.ID, "pr_number", prNumber)
}

// reportPRCreatedTool is the registered report_pr_created handler: reportPRCreated
// behind the identity gate, so callerUUID is the verified session credential
// (ADR-007 rule 0), never the spoofable X-Stapler-Session-UUID claim.
func (h *backlogHandlers) reportPRCreatedTool() mcpserver.ToolHandlerFunc {
	gate := &callerIdentityGate{metrics: wakes.NewMetrics(), roleOf: roleLookupFrom(h.storage.GetItemSessionBySessionUUID)}
	return withCallerIdentityGate(gate, toolReportPRCreated, h.reportPRCreated)
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
func (h *backlogHandlers) reportPRCreated(ctx context.Context, callerUUID string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
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

	// replaces_pr_number names the tracked PR a correction replaces; it must be
	// the item's current one, so a stale or guessed number cannot select the
	// reassignment path.
	if raw, present := args["replaces_pr_number"]; present {
		rp, isNum := raw.(float64)
		if !isNum || rp <= 0 || int(rp) != item.PrNumber {
			return errResult(ErrInvalidArgument, fmt.Sprintf(
				"replaces_pr_number=%v is not this item's tracked PR (#%d) — omit it to report an additional PR, or pass the tracked number to replace it",
				raw, item.PrNumber), ""), nil
		}
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
	// With the multi-PR backlog scope on, a different PR reported without
	// replaces_pr_number is an ADDITIONAL PR (a follow-up), not a reassignment: no
	// override_reason, no reassignment guard, status unchanged. The branch check below
	// still applies. Scope off: today's reassignment rules.
	_, replacesPresent := args["replaces_pr_number"]
	additive := item.Status == string(session.BacklogStatusPRPending) && !replacesPresent && h.backlogReaderEnabled()
	isReassignment := item.Status == string(session.BacklogStatusPRPending) && !additive
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
		curRepoRef, curRefErr := githubpkg.NewRepoRefWithHost(curRef.Owner, curRef.Repo, curRef.Host)
		if curRefErr != nil {
			return errResult(ErrInternalError, fmt.Sprintf(
				"could not build a GitHub repo reference for the currently tracked PR (%q): %v", item.PrURL, curRefErr), ""), nil
		}
		curVerification, curErr := h.verifyPR(ctx, curRepoRef, item.PrNumber, "")
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
	prURL = normalizeReportedPRURL(prURL)
	if strings.Contains(prURL, "://") {
		if u, uErr := url.Parse(prURL); uErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return errResult(ErrInvalidArgument, "pr_url must be an http(s) GitHub PR URL", ""), nil
		}
	}
	ref, parseErr := session.ParseGitHubURLWithHosts(prURL, h.enterpriseHosts())
	if parseErr != nil || ref.Owner == "" || ref.Repo == "" {
		return errResult(ErrInvalidArgument, fmt.Sprintf("pr_url is not a recognizable GitHub PR URL: %v", parseErr), ""), nil
	}
	if ref.PRNumber != 0 && ref.PRNumber != prNumber {
		return errResult(ErrInvalidArgument, fmt.Sprintf("pr_url references PR #%d but pr_number=%d was given — these must match", ref.PRNumber, prNumber), ""), nil
	}

	// Store the canonical URL, not the agent's raw one (e.g. .../pull/5/files).
	ref.PRNumber = prNumber
	canonicalPRURL := ref.PRURL()

	// Resolve this session's own branch to verify the reported PR against —
	// the whole point of this check is to refuse a self-reported PR number
	// for a branch that isn't even this item's own.
	branch, branchErr := h.sessionBranch(ctx, callerUUID)
	if branchErr != nil || branch == "" {
		return errResult(ErrInternalError, "could not resolve this session's git branch to verify the reported PR", ""), nil
	}

	repoRef, repoRefErr := githubpkg.NewRepoRefWithHost(ref.Owner, ref.Repo, ref.Host)
	if repoRefErr != nil {
		return errResult(ErrInvalidArgument, fmt.Sprintf("pr_url is not a recognizable GitHub PR URL: %v", repoRefErr), ""), nil
	}
	// VerifyPRMatchesBranch matches on head-branch name alone, so any repo with a
	// same-named branch would pass. Anchor on the repo of the PR the item already
	// tracks, else on the session's own repo. An additional PR must live in the
	// anchor repo; a first or replacement PR elsewhere (or with no resolvable anchor)
	// must take the override path: override_reason plus the operator's own authorship.
	anchor, anchorErr := h.itemAnchorRepo(ctx, item, callerUUID)
	inItemRepo := anchorErr == nil && sameRepo(anchor, repoRef)
	foreignRepo := !inItemRepo && !additive
	if additive && !inItemRepo {
		if anchorErr != nil {
			return errResult(ErrInternalError, fmt.Sprintf(
				"could not determine the repository of the PR this item already tracks, so PR #%d cannot be added as an additional PR: %v", prNumber, anchorErr), ""), nil
		}
		return errResult(ErrInvalidArgument, fmt.Sprintf(
			"PR #%d is in %s/%s, not the repository of the PR this item already tracks (%s) — an additional PR must be in the same repository. "+
				"To move the item to a PR in another repository, use replaces_pr_number with override_reason.",
			prNumber, ref.Owner, ref.Repo, anchor), ""), nil
	}
	if foreignRepo && !isReassignment && overrideReason == "" {
		return errResult(ErrInvalidArgument, fmt.Sprintf(
			"PR #%d is in %s/%s, which is not this item's repository (%s) — refusing to record it without override_reason. "+
				"Retry with override_reason explaining why; the PR must also be authored by your own GitHub identity.",
			prNumber, ref.Owner, ref.Repo, anchorLabel(anchor, anchorErr)), ""), nil
	}
	verification, verifyErr := h.verifyPR(ctx, repoRef, prNumber, branch)
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
	if verification.Exists && overrideReason != "" && (isReassignment || foreignRepo || !verification.Matched) {
		login, loginErr := h.callerGitHubLogin(ctx, repoRef.Host())
		if loginErr != nil {
			return errResult(ErrInternalError, fmt.Sprintf("could not resolve your GitHub identity to verify the override — retry: %v", loginErr), ""), nil
		}
		callerLogin = login
	}

	accept, code, _ := decideOverridePolicy(verification, overrideReason, callerLogin, isReassignment || foreignRepo)
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

	if setErr := h.storage.GatedSetPRAndTransition(ctx, session.TransitionRequest{
		Caller: session.CallerMCPReportPRCreated, Observed: item, TriggeredBy: session.TriggeredByAgent,
	}, session.PRAssociation{URL: canonicalPRURL, Number: prNumber, Summary: summary, Guard: guard, Source: session.AttachSourceReportPRCreated, Additive: additive}); setErr != nil {
		if isTransitionDenied(setErr) {
			return transitionDeniedResult(setErr, session.BacklogStatus(item.Status), session.BacklogStatusPRPending), nil
		}
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
	if inItemRepo {
		h.stampPRProvenance(ctx, item, repoRef, prNumber)
	}

	if isReassignment || foreignRepo || !verification.Matched {
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

	if additive {
		return mcpgo.NewToolResultText(fmt.Sprintf(
			"PR #%d added to item %s as an additional PR. The item stays pr_pending.", prNumber, itemID,
		)), nil
	}
	return mcpgo.NewToolResultText(fmt.Sprintf(
		"PR #%d recorded for item %s. Item transitioned to pr_pending.", prNumber, itemID,
	)), nil
}

// itemAnchorRepo is the repository a reported PR must live in: the repo of the
// PR the item already tracks, else the session's own repo. An error means no
// anchor could be resolved, which callers treat as "not in the item's repo".
func (h *backlogHandlers) itemAnchorRepo(ctx context.Context, item *session.BacklogItemData, sessionUUID string) (githubpkg.RepoRef, error) {
	if item.PrURL != "" {
		cur, err := session.ParseGitHubURLWithHosts(normalizeReportedPRURL(item.PrURL), h.enterpriseHosts())
		if err != nil {
			return githubpkg.RepoRef{}, fmt.Errorf("tracked PR URL %q is unparseable: %w", item.PrURL, err)
		}
		return githubpkg.NewRepoRefWithHost(cur.Owner, cur.Repo, cur.Host)
	}
	return h.sessionRepo(ctx, sessionUUID)
}

// sessionRepo resolves the GitHub repo the session works in: the worktree's
// origin remote, else the session's recorded github_owner/github_repo. The
// resolveSessionRepo seam replaces it in tests.
func (h *backlogHandlers) sessionRepo(ctx context.Context, sessionUUID string) (githubpkg.RepoRef, error) {
	if h.resolveSessionRepo != nil {
		return h.resolveSessionRepo(ctx, sessionUUID)
	}
	if wt, err := h.storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID); err == nil && wt.RepoPath != "" {
		if owner, repo, host, ok := session.OriginGitHubRepo(wt.RepoPath); ok {
			return githubpkg.NewRepoRefWithHost(owner, repo, host)
		}
	}
	data, err := h.storage.FindInstanceDataByID(sessionUUID)
	if err != nil {
		return githubpkg.RepoRef{}, fmt.Errorf("find session: %w", err)
	}
	if data.GitHubOwner == "" || data.GitHubRepo == "" {
		return githubpkg.RepoRef{}, errors.New("the session has no resolvable GitHub repository")
	}
	return githubpkg.NewRepoRefWithHost(data.GitHubOwner, data.GitHubRepo, data.GitHubHost)
}

// sameRepo compares two repos by host, owner and repo, ignoring case, a "www."
// host prefix and a ".git" repo suffix.
func sameRepo(a, b githubpkg.RepoRef) bool { return repoIdentity(a) == repoIdentity(b) }

func repoIdentity(r githubpkg.RepoRef) string {
	host := strings.TrimPrefix(githubpkg.NormalizeHost(r.Host()), "www.")
	return host + "/" + strings.ToLower(r.Owner()) + "/" + strings.TrimSuffix(strings.ToLower(r.Repo()), ".git")
}

func anchorLabel(anchor githubpkg.RepoRef, err error) string {
	if err != nil {
		return "unresolved"
	}
	return anchor.String()
}

// normalizeReportedPRURL trims the agent's pr_url and prefixes https:// onto a
// scheme-less host form such as "github.com/o/r/pull/5", which older agents send.
// "o/r#5" shorthand has no dot in its first segment and is left for the parser.
func normalizeReportedPRURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") {
		return raw
	}
	if first, _, _ := strings.Cut(raw, "/"); strings.Contains(first, ".") && !strings.ContainsAny(first, ":@") {
		return "https://" + raw
	}
	return raw
}
