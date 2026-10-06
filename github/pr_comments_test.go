package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostPRCommentREST_should_PostJSONBodyToIssueCommentsEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotType string
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth, gotType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(resetGhBaseURL(ts))
	t.Setenv("GITHUB_TOKEN", "tok-123")
	repo, err := NewRepoRef("acme", "widgets")
	require.NoError(t, err)

	require.NoError(t, PostPRCommentREST(context.Background(), repo, 7, "hello"))

	assert.Equal(t, http.MethodPost, gotMethod, "the constructor must build a real POST, not a GET")
	assert.Equal(t, "/repos/acme/widgets/issues/7/comments", gotPath)
	assert.Equal(t, "Bearer tok-123", gotAuth)
	assert.Equal(t, "application/json", gotType)
	assert.Equal(t, "hello", gotBody["body"])
}

func TestPostPRCommentREST_should_ReturnError_When_ResponseNot2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(resetGhBaseURL(ts))
	t.Setenv("GITHUB_TOKEN", "tok-123")
	repo, err := NewRepoRef("acme", "widgets")
	require.NoError(t, err)

	err = PostPRCommentREST(context.Background(), repo, 7, "hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestPostPRCommentREST_should_ReturnNotAuthenticated_When_NoToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	// A non-github.com host has no env-var fallback, so no token resolves.
	ghe, err := NewRepoRefWithHost("acme", "widgets", "ghe.example.invalid")
	require.NoError(t, err)

	assert.ErrorIs(t, PostPRCommentREST(context.Background(), ghe, 7, "hello"), ErrNotAuthenticated)
}
