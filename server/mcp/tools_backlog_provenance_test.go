package mcp

import (
	"context"
	"errors"
	"regexp"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

type fakeProvenanceSource struct{ host session.HostIdentity }

func (f fakeProvenanceSource) PRProvenanceComment(item *session.BacklogItemData) (string, bool) {
	return session.FormatPRProvenanceComment(f.host.ID, "ssq://laptop"+session.BacklogItemDeepLinkPath(item)), true
}

type postedComment struct {
	repo   githubpkg.RepoRef
	number int
	body   string
}

func reportPRWithProvenance(t *testing.T, post func(context.Context, githubpkg.RepoRef, int, string) error) (*session.BacklogItemData, session.HostIdentity, string, string) {
	t.Helper()
	storage := newTestBacklogStorage(t)
	host, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	storage.SetPRProvenanceSource(fakeProvenanceSource{host: host})
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, githubpkg.RepoRef, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		postPRComment: post,
	}
	result, err := handler.reportPRCreated(WithSessionUUID(context.Background(), sessionUUID), makeToolReq(map[string]interface{}{
		"item_id": item.ID, "pr_url": "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42), "summary": "Shipped.",
	}))
	require.NoError(t, err)
	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	return fetched, host, item.ID, text.Text
}

func TestReportPRCreated_should_PostProvenanceComment_When_TransitionSucceeds(t *testing.T) {
	var posted []postedComment
	fetched, host, _, out := reportPRWithProvenance(t, func(_ context.Context, ref githubpkg.RepoRef, n int, body string) error {
		posted = append(posted, postedComment{ref, n, body})
		return nil
	})

	assert.Contains(t, out, "recorded")
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	publicID, hasPublic := fetched.PublicID()
	require.True(t, hasPublic)
	require.Len(t, posted, 1)
	assert.Equal(t, 42, posted[0].number)
	assert.Equal(t, "tstapler/stapler-squad", posted[0].repo.String())
	assert.Contains(t, posted[0].body, "ssq://laptop/backlog/v1/"+publicID.String())
	assert.Contains(t, posted[0].body, host.ID.String())
	assert.False(t, regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`).MatchString(posted[0].body), "no IP address in %q", posted[0].body)
	parsed, ok := session.ParsePRProvenanceComment(posted[0].body)
	require.True(t, ok)
	assert.Equal(t, publicID.String(), parsed.ItemID)
}

func TestReportPRCreated_should_StillSucceed_When_ProvenanceCommentFails(t *testing.T) {
	fetched, _, _, out := reportPRWithProvenance(t, func(context.Context, githubpkg.RepoRef, int, string) error {
		return errors.New("github is down")
	})

	assert.Contains(t, out, "recorded", "provenance stamping is best-effort")
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 42, fetched.PrNumber)
}

func TestReportPRCreated_should_NotPostComment_When_NoProvenanceSourceWired(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	calls := 0
	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, githubpkg.RepoRef, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		postPRComment: func(context.Context, githubpkg.RepoRef, int, string) error { calls++; return nil },
	}

	_, err := handler.reportPRCreated(WithSessionUUID(context.Background(), sessionUUID), makeToolReq(map[string]interface{}{
		"item_id": item.ID, "pr_url": "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42), "summary": "Shipped.",
	}))

	require.NoError(t, err)
	assert.Zero(t, calls)
}
