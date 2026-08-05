package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gh "github.com/tstapler/stapler-squad/github"
)

// resetGhBaseURL overrides gh.GhBaseURL for a test and returns a restore
// func. Same pattern as server/services/backlog_github_rpc_test.go's helper
// of the same name.
func resetGhBaseURL(ts *httptest.Server) func() {
	gh.GhBaseURL = ts.URL + "/"
	return func() { gh.GhBaseURL = "https://api.github.com/" }
}

// --- NewPRVerification (gap: plan.md Task 2.1 describes this behavior but
// names no dedicated test for it — see validation.md) ---

// TestNewPRVerification_should_ForceMatchedFalse_When_MatchedTrueButNotExists
// pins the Matched ⇒ Exists invariant NewPRVerification enforces at
// construction: an illegal matched=true/exists=false combination must be
// coerced to matched=false (logged, never panicking), while a legal
// construction passes through unchanged.
func TestNewPRVerification_should_ForceMatchedFalse_When_MatchedTrueButNotExists(t *testing.T) {
	t.Run("illegal construction is coerced", func(t *testing.T) {
		v := NewPRVerification(false, true, "some-branch", gh.PRStateOpen, "tstapler")
		assert.False(t, v.Exists)
		assert.False(t, v.Matched, "matched must be forced to false when exists=false")
		// Non-invariant fields are carried through unvalidated.
		assert.Equal(t, "some-branch", v.ActualHeadBranch)
		assert.Equal(t, gh.PRStateOpen, v.State)
		assert.Equal(t, "tstapler", v.Author)
	})

	t.Run("legal construction passes through unchanged", func(t *testing.T) {
		v := NewPRVerification(true, true, "some-branch", gh.PRStateMerged, "tstapler")
		assert.True(t, v.Exists)
		assert.True(t, v.Matched)
		assert.Equal(t, "some-branch", v.ActualHeadBranch)
		assert.Equal(t, gh.PRStateMerged, v.State)
		assert.Equal(t, "tstapler", v.Author)
	})

	t.Run("exists=true, matched=false is legal (the fallback-branch case) and passes through", func(t *testing.T) {
		v := NewPRVerification(true, false, "feature/y", gh.PRStateOpen, "tstapler")
		assert.True(t, v.Exists)
		assert.False(t, v.Matched)
	})
}

// --- VerifyPRMatchesBranch rewritten body (gap: plan.md Task 2.1 rewrites
// this function but Story 2's task list only updates its callers' seam
// literals, never exercises the real function through an HTTP stub — see
// validation.md) ---

// TestVerifyPRMatchesBranch_should_ReturnMatchedTrue_When_HeadBranchEqualsExpected
// exercises the real VerifyPRMatchesBranch -> GetPRByNumber REST round trip
// via an httptest.Server + GhBaseURL override, confirming the happy/fast
// path (Matched=true) and that Author is threaded through from the
// response's user.login field.
func TestVerifyPRMatchesBranch_should_ReturnMatchedTrue_When_HeadBranchEqualsExpected(t *testing.T) {
	body := map[string]any{
		"number":   326,
		"html_url": "https://github.com/tstapler/stapler-squad/pull/326",
		"state":    "closed",
		"merged":   true,
		"head": map[string]any{
			"ref": "feature/ci-status-diff-viewer",
		},
		"base": map[string]any{
			"ref": "main",
			"repo": map[string]any{
				"full_name": "tstapler/stapler-squad",
			},
		},
		"user": map[string]any{
			"login": "tstapler",
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	v, err := VerifyPRMatchesBranch(context.Background(), "tstapler", "stapler-squad", 326, "feature/ci-status-diff-viewer")
	require.NoError(t, err)
	assert.True(t, v.Exists)
	assert.True(t, v.Matched)
	assert.Equal(t, "feature/ci-status-diff-viewer", v.ActualHeadBranch)
	assert.Equal(t, gh.PRStateMerged, v.State)
	assert.Equal(t, "tstapler", v.Author)
}

// TestVerifyPRMatchesBranch_should_ReturnError_When_GetPRByNumberFails
// confirms (PRVerification{}, err) propagates unchanged from
// GetPRByNumber's error — the exact contract reportPRCreated's transient-
// error handling depends on.
func TestVerifyPRMatchesBranch_should_ReturnError_When_GetPRByNumberFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	v, err := VerifyPRMatchesBranch(context.Background(), "tstapler", "stapler-squad", 326, "feature/ci-status-diff-viewer")
	require.Error(t, err)
	assert.Equal(t, PRVerification{}, v)
}
