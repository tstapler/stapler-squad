package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// backlogIntentCallBudget bounds ParseBacklogItemIntent's headless call. This
// is a synchronous, foreground call the omnibar's review UI awaits with a
// spinner (unlike TriggerTriage's multi-hour backstop for a backgrounded,
// multi-subagent research run) — a WorkDir-less single-shot text call that
// should return in seconds; bounded generously to absorb cold-start/backoff.
const backlogIntentCallBudget = 90 * time.Second

// ParseBacklogItemIntent runs a free-text message through an LLM to produce a
// review draft. Best-effort like MaybeTriggerTriage: failures never return a
// Connect error — the response's error field signals a fallback to
// CreateBacklogItemFromChat.
// +api: backlog:parse-item-intent
func (s *BacklogService) ParseBacklogItemIntent(
	ctx context.Context,
	req *connect.Request[sessionv1.ParseBacklogItemIntentRequest],
) (*connect.Response[sessionv1.ParseBacklogItemIntentResponse], error) {
	message := strings.TrimSpace(req.Msg.Message)
	if message == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("message is required"))
	}
	if s.headlessPool == nil {
		return parseIntentFailure("LLM parsing is unavailable in this deployment"), nil
	}

	callCtx, cancel := context.WithTimeout(ctx, backlogIntentCallBudget)
	defer cancel()

	raw, callErr := s.headlessPool.CallBlocking(callCtx,
		headless.FeatureKeyBacklogIntentParse,
		session.BuildBacklogIntentSystemPrompt(),
		session.BuildBacklogIntentUserPrompt(message, strings.TrimSpace(req.Msg.RepoPath)),
		headless.CallOptions{},
		headless.DiscardCost,
	)
	if callErr != nil {
		log.WarningLog().Printf("[ParseBacklogItemIntent] headless call failed: %v", callErr)
		return parseIntentFailure("couldn't parse this into a structured item"), nil
	}

	draft, parseErr := session.ParseBacklogItemIntentDraft(raw)
	if parseErr != nil {
		log.WarningLog().Printf("[ParseBacklogItemIntent] parse failed: %v", parseErr)
		return parseIntentFailure("couldn't parse this into a structured item"), nil
	}

	return connect.NewResponse(&sessionv1.ParseBacklogItemIntentResponse{
		Draft: &sessionv1.ParsedBacklogItemDraft{
			Title:              draft.Title,
			Description:        draft.Description,
			AcceptanceCriteria: draft.AcceptanceCriteria,
			Confidence:         float32(draft.Confidence),
		},
	}), nil
}

// parseIntentFailure builds the best-effort failure response — draft empty,
// error set — so the caller falls back to CreateBacklogItemFromChat instead
// of losing the user's typed text.
func parseIntentFailure(msg string) *connect.Response[sessionv1.ParseBacklogItemIntentResponse] {
	return connect.NewResponse(&sessionv1.ParseBacklogItemIntentResponse{Error: msg})
}
