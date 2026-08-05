//go:build live_github

package github

import (
	"context"
	"testing"
)

// TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326 exercises
// GetPRByNumber against real, unauthenticated api.github.com traffic. PR #326
// (https://github.com/tstapler/stapler-squad/pull/326) is real and publicly
// visible, so no GITHUB_TOKEN is required.
//
// This is the one test in the package that validates the REST response
// shape (base.repo.full_name, user.login, head.ref/base.ref nesting)
// against real GitHub data rather than a hand-written httptest fixture.
// State is deliberately not pinned to a specific value (verified live:
// PR #326 was open, not merged, as of this test's authoring, despite the
// bug report that motivated this fix describing it as already shipped) —
// this test's job is to prove the parsing shape is correct against real
// data, not to assert an external, mutable PR's current lifecycle state.
//
// Deliberately gated behind a dedicated live_github build tag rather than
// this repo's existing integration tag — that tag is scoped to tests
// requiring real tmux (see server/mcp/server_integration_test.go and
// friends), a different and unrelated failure mode from live network
// access to api.github.com. Not run by default go test ./..., make test,
// make test-race, or make test-integration/make ci. Run manually via:
//
//	go test -tags live_github -run TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326 -v ./github/...
func TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326(t *testing.T) {
	info, err := GetPRByNumber(context.Background(), "tstapler", "stapler-squad", 326)
	if err != nil {
		t.Fatalf("GetPRByNumber returned error: %v", err)
	}
	if info.HeadRef != "feature/ci-status-diff-viewer" {
		t.Errorf("HeadRef = %q, want %q", info.HeadRef, "feature/ci-status-diff-viewer")
	}
	switch info.State {
	case PRStateOpen, PRStateClosed, PRStateMerged:
	default:
		t.Errorf("State = %q, want one of %q/%q/%q", info.State, PRStateOpen, PRStateClosed, PRStateMerged)
	}
	if info.BaseRef == "" {
		t.Error("BaseRef is empty, want non-empty")
	}
	if info.Author == "" {
		t.Error("Author is empty, want non-empty")
	}
	t.Logf("real PR #326: state=%s author=%s", info.State, info.Author)
}
