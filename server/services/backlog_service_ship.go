package services

// backlog_service_ship.go — the "Ship PR" self-service action.
//
// Root-cause context (docs/tasks/backlog-feature-improvement.md, 2026-07-18
// update): PR creation only ever happened via (a) the automated
// pushAndCreatePR path (session/backlog_lifecycle.go), which fires only once
// the review session's tmux process actually exits, or (b) the manual Review
// Queue "Create PR" button on a completely different page. There was no
// affordance on the backlog item detail page itself to ask the agent to ship
// a PR for an item sitting in review with a PASS verdict — this file closes
// that gap by exposing the same one-shot PR-creation mechanism the opt-in
// AutoCreatePR policy already uses (server.ReactiveQueueManager.maybeAutoCreatePR)
// as a direct, on-demand RPC.

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// shipPRPrompt is the one-shot prompt used by TriggerShipPR. Mirrors
// DEFAULT_PR_PROMPT (web-app/src/components/sessions/ReviewQueuePanel.tsx) and
// autoCreatePRPrompt (server/review_queue_manager.go) — three copies of the
// same literal now exist across the manual, opt-in-automatic, and detail-page
// triggers for PR creation. Kept in sync manually, same as the other two; see
// server/review_queue_manager.go's autoCreatePRPrompt doc comment for the
// established rationale (no CI check catches drift today) and for why the
// format is spelled out explicitly rather than left to the agent's judgment.
const shipPRPrompt = "Create a pull request for the changes in this session. Title: use Conventional Commits format (fix:, feat:, etc.). Body: structure as ## Summary (1-3 sentences on why this change was made, tied to the backlog item's problem statement in .backlog-context.md if present), ## What Changed (a short bullet list, not a line-by-line diff restatement), and ## Test plan (a checklist of concrete verification steps such as specific commands or manual checks — not an unqualified claim that tests pass). Keep it concise, no scratch notes."

// PRRunner runs a one-shot LLM prompt against a session's worktree, returning
// the PR URL the prompt produced (or "" if none was created). Defined here —
// the consumer — rather than in the session-management layer, per this repo's
// anti-interface-pollution convention; *services.SessionService satisfies it
// via RunOneShotForSession. Mirrors server.OneShotPRCreator, which the same
// method also satisfies for the review-queue's AutoCreatePR trigger.
type PRRunner interface {
	RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error)
}

// SetOneShotRunner wires the one-shot PR-creation runner used by TriggerShipPR.
// Called post-construction from server/dependencies.go once SessionService is
// available — mirrors SetSessionStopper/SetAutonomousDriverStarter's
// setter-injection pattern used elsewhere in this file's wiring. nil (the
// default) makes TriggerShipPR return CodeUnimplemented.
func (s *BacklogService) SetOneShotRunner(r PRRunner) {
	s.oneShotRunner = r
}

// TriggerShipPR manually runs the same one-shot PR-creation prompt the opt-in
// AutoCreatePR policy uses automatically, for a backlog item that has no PR
// yet. This is the self-service "Ship PR" action on the item detail page —
// closing the gap where the only way to ask the agent to ship a PR was the
// unrelated Review Queue page, or waiting on AutoCreatePR (opt-in, default
// off) to fire on its own.
// +api: backlog:trigger-ship-pr
func (s *BacklogService) TriggerShipPR(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerShipPRRequest],
) (*connect.Response[sessionv1.TriggerShipPRResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	if s.oneShotRunner == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("PR creation is not available on this server"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	if session.BacklogStatus(item.Status) != session.BacklogStatusReview {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q status to ship a PR, got %q", session.BacklogStatusReview, item.Status))
	}
	if item.PrURL != "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item already has a PR: %s", item.PrURL))
	}

	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}
	_, workSession := findMostRecentSessions(sessions)
	if workSession == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no work session found for item %q — nothing to ship", req.Msg.ItemId))
	}

	prURL, runErr := s.oneShotRunner.RunOneShotForSession(ctx, workSession.SessionUUID, shipPRPrompt, int32(autoCreatePRRunTimeoutSeconds))
	if runErr != nil {
		log.WarningLog.Printf("[TriggerShipPR] item=%s session=%s: %v", item.ID, workSession.SessionUUID, runErr)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("could not ship a PR — the work session may not be running (try Restart first): %w", runErr))
	}
	if prURL == "" {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("PR creation ran but no PR URL was found in the output"))
	}

	log.InfoLog.Printf("[TriggerShipPR] item=%s session=%s PR created: %s", item.ID, workSession.SessionUUID, prURL)
	return connect.NewResponse(&sessionv1.TriggerShipPRResponse{PrUrl: prURL}), nil
}

// autoCreatePRRunTimeoutSeconds bounds TriggerShipPR's one-shot call. Matches
// server.autoCreatePRRunTimeout (900s) — kept as a separate constant since
// server/review_queue_manager.go's is unexported and this package cannot
// import the server package (server imports services, not the reverse).
const autoCreatePRRunTimeoutSeconds = 900
