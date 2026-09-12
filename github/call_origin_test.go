package github

import (
	"context"
	"testing"
)

func TestGitHubCallOriginFrom_should_DefaultToPRStatusPoller_When_ContextUntagged(t *testing.T) {
	got := GitHubCallOriginFrom(context.Background())
	if got != OriginPRStatusPoller {
		t.Errorf("GitHubCallOriginFrom(untagged) = %q, want %q", got, OriginPRStatusPoller)
	}
}

func TestWithGitHubCallOrigin_should_RoundTripExactly_When_TaggedWithEachOfFiveConstants(t *testing.T) {
	origins := []CallOrigin{
		OriginInteractive,
		OriginPRStatusPoller,
		OriginWorktreePRPoller,
		OriginBacklogSync,
		OriginWebhookReconcile,
	}

	for _, origin := range origins {
		t.Run(string(origin), func(t *testing.T) {
			ctx := WithGitHubCallOrigin(context.Background(), origin)
			got := GitHubCallOriginFrom(ctx)
			if got != origin {
				t.Errorf("GitHubCallOriginFrom(WithGitHubCallOrigin(ctx, %q)) = %q, want %q", origin, got, origin)
			}
		})
	}
}
