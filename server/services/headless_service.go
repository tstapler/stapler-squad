package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/headless"
)

// Compile-time check: HeadlessService must implement HeadlessServiceHandler.
var _ sessionv1connect.HeadlessServiceHandler = (*HeadlessService)(nil)

// HeadlessService implements the RunHeadlessCall streaming RPC.
type HeadlessService struct {
	pool *headless.Pool
}

// NewHeadlessService creates a HeadlessService backed by the given pool.
// pool may be nil; in that case RunHeadlessCall returns CodeUnavailable.
func NewHeadlessService(pool *headless.Pool) *HeadlessService {
	return &HeadlessService{pool: pool}
}

// maxPromptBytes is the per-field byte limit for system_prompt and user_prompt.
const maxPromptBytes = 100_000

// RunHeadlessCall streams LLM output chunks back to the caller.
// It validates the feature_key and prompt sizes, applies a timeout, then drains the pool channel.
func (s *HeadlessService) RunHeadlessCall(
	ctx context.Context,
	req *connect.Request[sessionv1.RunHeadlessCallRequest],
	stream *connect.ServerStream[sessionv1.RunHeadlessCallResponse],
) error {
	if s.pool == nil {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("headless pool is unavailable (claude binary not found)"))
	}

	featureKey := headless.FeatureKey(req.Msg.FeatureKey)
	if !headless.AllowedFeatureKeys[featureKey] || featureKey == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid feature_key %q: must be one of %s", req.Msg.FeatureKey, headless.AllowedFeatureKeyList()))
	}

	if len(req.Msg.SystemPrompt) > maxPromptBytes {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("system_prompt exceeds maximum size of %d bytes", maxPromptBytes))
	}
	if len(req.Msg.UserPrompt) > maxPromptBytes {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("user_prompt exceeds maximum size of %d bytes", maxPromptBytes))
	}

	// Apply timeout.
	timeoutSecs := int(req.Msg.TimeoutSeconds)
	timeout := headless.DefaultCallTimeout
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
	}
	if timeout > headless.MaxCallTimeout {
		timeout = headless.MaxCallTimeout
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Forward model override from the request so callers can select a specific model.
	ch, err := s.pool.CallWithOptions(callCtx, featureKey, req.Msg.SystemPrompt, req.Msg.UserPrompt, headless.CallOptions{
		Model: req.Msg.Model,
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("headless call start: %w", err))
	}

	for chunk := range ch {
		if chunk.Err != nil {
			// Send error chunk and stop.
			if sendErr := stream.Send(&sessionv1.RunHeadlessCallResponse{
				IsError:      true,
				ErrorMessage: chunk.Err.Error(),
				Done:         true,
			}); sendErr != nil {
				return sendErr
			}
			return nil
		}
		resp := &sessionv1.RunHeadlessCallResponse{
			Text:    chunk.Text,
			Done:    chunk.Done,
			CostUsd: chunk.CostUSD,
		}
		if sendErr := stream.Send(resp); sendErr != nil {
			return sendErr
		}
		if chunk.Done {
			return nil
		}
	}

	// Channel closed: either context cancelled or goroutine exited without sending Done.
	if ctx.Err() != nil {
		return nil // client disconnected; return nil per WatchSessions pattern
	}

	// Send final done message.
	return stream.Send(&sessionv1.RunHeadlessCallResponse{Done: true})
}
