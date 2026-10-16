package services

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/headless"
)

// selectingAIClient routes rules generation through the headless backend
// selector when settings point it away from claude; otherwise it defers to the
// original AIClient chain (claude CLI -> gemini -> agy -> opencode -> HTTP) so
// default behavior is unchanged.
type selectingAIClient struct {
	fallback AIClient
}

// WrapRulesAIClient returns fallback unchanged when it is nil (rules generation
// stays disabled), else a selector-aware wrapper.
func WrapRulesAIClient(fallback AIClient) AIClient {
	if fallback == nil {
		return nil
	}
	return &selectingAIClient{fallback: fallback}
}

func (c *selectingAIClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	sel := headless.DefaultSelector()
	if sel == nil || sel.Requested(headless.FeatureKeyRulesGeneration) == headless.BackendClaude {
		return c.fallback.Complete(ctx, systemPrompt, userPrompt)
	}
	client := &headless.SelectingClient{Selector: sel}
	out, err := client.CallBlocking(ctx, headless.FeatureKeyRulesGeneration, systemPrompt, userPrompt, headless.CallOptions{}, headless.DiscardCost)
	if err != nil {
		return "", fmt.Errorf("rules generation via selected backend: %w", err)
	}
	return out, nil
}
