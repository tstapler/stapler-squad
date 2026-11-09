package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

// T5.3.0: the caller-identity check is server-side (the MCP session credential in
// ctx), so it holds regardless of the dispatcher's --allowedTools.

func reportPRArgs(itemID string, extra map[string]interface{}) map[string]interface{} {
	args := map[string]interface{}{
		"item_id":   itemID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

func TestReportPRCreated_ShouldReject_WhenCallerSessionNotLinkedToItem(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	item, linkedUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	handler := &backlogHandlers{storage: storage}

	t.Run("unlinked caller is rejected", func(t *testing.T) {
		ctx := WithSessionUUID(context.Background(), uuid.New().String())
		result, err := callReportPR(handler, ctx, makeToolReq(reportPRArgs(item.ID, nil)))
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])
		fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
		require.NoError(t, gerr)
		assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "nothing was written")
		assert.Zero(t, fetched.PrNumber)
	})

	t.Run("a request field cannot name the linked session", func(t *testing.T) {
		ctx := WithSessionUUID(context.Background(), uuid.New().String())
		args := reportPRArgs(item.ID, map[string]interface{}{"session_uuid": linkedUUID, "session_id": linkedUUID, "caller_session_uuid": linkedUUID})
		result, err := callReportPR(handler, ctx, makeToolReq(args))
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])
	})

	t.Run("no credential in context is rejected", func(t *testing.T) {
		result, err := callReportPR(handler, context.Background(), makeToolReq(reportPRArgs(item.ID, nil)))
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])
	})

	t.Run("linked caller is accepted", func(t *testing.T) {
		h := &backlogHandlers{
			storage:            storage,
			resolveSessionRepo: testSessionRepo, resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
			verifyPRMatchesBranch: func(context.Context, githubpkg.RepoRef, int, string) (PRVerification, error) {
				return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			},
		}
		ctx := WithSessionUUID(context.Background(), linkedUUID)
		_, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, nil)))
		require.NoError(t, err)
		fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
		require.NoError(t, gerr)
		assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
		assert.Equal(t, 42, fetched.PrNumber)
	})
}

func TestReportPRCreated_ShouldRejectMismatchedReplacesNumber_AndAcceptTrackedNumber(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	n := 21
	url := "https://github.com/tstapler/stapler-squad/pull/21"
	_, err := storage.UpdateBacklogItem(context.Background(), item.ID, session.BacklogItemUpdate{PrNumber: &n, PrURL: &url}, nil)
	require.NoError(t, err)
	_, err = storage.RawTransitionBacklogItemStatus(context.Background(), item.ID, session.BacklogStatusPRPending, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	h := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	args := reportPRArgs(item.ID, map[string]interface{}{"replaces_pr_number": float64(99), "override_reason": "bad PR"})
	result, err := callReportPR(h, ctx, makeToolReq(args))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Equal(t, ErrInvalidArgument, m["error"].(map[string]interface{})["code"])

	// The tracked number passes the replaces check and reaches the old reassignment
	// guard, which still demands override_reason.
	args = reportPRArgs(item.ID, map[string]interface{}{"replaces_pr_number": float64(21)})
	result, err = callReportPR(h, ctx, makeToolReq(args))
	require.NoError(t, err)
	m = parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Contains(t, m["error"].(map[string]interface{})["message"], "requires override_reason")
}

func pendingItemWithPR21(t *testing.T, storage *session.Storage, itemID string) {
	t.Helper()
	n, url := 21, "https://github.com/tstapler/stapler-squad/pull/21"
	_, err := storage.UpdateBacklogItem(context.Background(), itemID, session.BacklogItemUpdate{PrNumber: &n, PrURL: &url}, nil)
	require.NoError(t, err)
	_, err = storage.RawTransitionBacklogItemStatus(context.Background(), itemID, session.BacklogStatusPRPending, nil, session.TriggeredBySystem)
	require.NoError(t, err)
}

func additiveHandler(storage *session.Storage, scopeOn bool) *backlogHandlers {
	return &backlogHandlers{
		storage:               storage,
		backlogReaderOverride: func() bool { return scopeOn },
		resolveSessionRepo:    testSessionRepo, resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, githubpkg.RepoRef, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
}

func TestReportPRCreated_ShouldAttachAdditionalPR_WhenScopeOnAndNoReplacesNumber(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	pendingItemWithPR21(t, storage, item.ID)
	h := additiveHandler(storage, true)
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, nil)))
	require.NoError(t, err)

	require.False(t, result.IsError, "no override_reason is required: %v", result.Content)
	fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, gerr)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 42, fetched.PrNumber, "the open follow-up outranks the stale scalar")
	views, lerr := storage.ItemPRs(context.Background(), item.ID)
	require.NoError(t, lerr)
	var followup *session.PRView
	for i := range views {
		if views[i].Ref.Number() == 42 {
			followup = &views[i]
		}
	}
	require.NotNil(t, followup, "#42 is in the derived set")
	assert.Equal(t, session.PRRoleFollowup, followup.Role)
}

func TestReportPRCreated_ShouldKeepReassignmentRules_WhenScopeOffOrReplacesNumberGiven(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	pendingItemWithPR21(t, storage, item.ID)
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	for name, tc := range map[string]struct {
		scope bool
		extra map[string]interface{}
	}{
		"scope off":                     {false, nil},
		"scope on, explicit correction": {true, map[string]interface{}{"replaces_pr_number": float64(21)}},
	} {
		result, err := callReportPR(additiveHandler(storage, tc.scope), ctx, makeToolReq(reportPRArgs(item.ID, tc.extra)))
		require.NoError(t, err, name)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool), name)
		assert.Contains(t, m["error"].(map[string]interface{})["message"], "requires override_reason", name)
	}
}

func TestReportPRCreated_ShouldStoreCanonicalURL_AndRejectNonHTTPScheme(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	h := additiveHandler(storage, true)
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{
		"pr_url": "javascript://github.com/tstapler/stapler-squad/pull/42",
	})))
	require.NoError(t, err)
	assert.False(t, parseResult(t, result)["success"].(bool))

	result, err = callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{
		"pr_url": "https://github.com/tstapler/stapler-squad/pull/42/files",
	})))
	require.NoError(t, err)
	require.False(t, result.IsError, "%v", result.Content)
	fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, gerr)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/42", fetched.PrURL)
}

func TestReportPRCreated_ShouldRejectAdditivePR_WhenInForeignRepo(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	pendingItemWithPR21(t, storage, item.ID)
	var posted int
	h := additiveHandler(storage, true)
	h.postPRComment = func(context.Context, githubpkg.RepoRef, int, string) error { posted++; return nil }
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{
		"pr_url": "https://github.com/victim/other/pull/42",
	})))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Contains(t, m["error"].(map[string]interface{})["message"], "same repository")
	assert.Zero(t, posted, "no provenance comment on a foreign repo")
	views, lerr := storage.ItemPRs(context.Background(), item.ID)
	require.NoError(t, lerr)
	for _, v := range views {
		assert.NotEqual(t, 42, v.Ref.Number(), "the foreign PR was not attached")
	}
}

func TestReportPRCreated_ShouldNotStampProvenance_WhenReassignedToForeignRepo(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	pendingItemWithPR21(t, storage, item.ID)
	host, err := hostclaim.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	storage.SetPRProvenanceSource(fakeProvenanceSource{host: host})
	var posted int
	h := additiveHandler(storage, true)
	h.resolveCallerGitHubLogin = func(context.Context) (string, error) { return "tstapler", nil }
	h.postPRComment = func(context.Context, githubpkg.RepoRef, int, string) error { posted++; return nil }
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{
		"pr_url": "https://github.com/other/repo/pull/42", "replaces_pr_number": float64(21), "override_reason": "moved the work",
	})))
	require.NoError(t, err)
	require.False(t, result.IsError, "%v", result.Content)
	assert.Zero(t, posted, "provenance is stamped only in the item's own repo")
}

// callReportPR drives the inner handler with the UUID carried on ctx, standing in
// for the identity gate that reportPRCreatedTool applies in production.
func callReportPR(h *backlogHandlers, ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	uuid, _ := sessionUUIDFromContext(ctx)
	return h.reportPRCreated(ctx, uuid, req)
}

// testSessionRepo is the repo every fixture session works in.
func testSessionRepo(context.Context, string) (githubpkg.RepoRef, error) {
	return githubpkg.NewRepoRef("tstapler", "stapler-squad")
}

func TestReportPRCreated_ShouldRequireOverride_WhenFirstPRIsInForeignOrUnresolvedRepo(t *testing.T) {
	for name, repoFn := range map[string]func(context.Context, string) (githubpkg.RepoRef, error){
		"foreign repo": func(context.Context, string) (githubpkg.RepoRef, error) {
			return githubpkg.NewRepoRef("tstapler", "other-project")
		},
		"unresolvable session repo": func(context.Context, string) (githubpkg.RepoRef, error) {
			return githubpkg.RepoRef{}, errors.New("no origin remote")
		},
	} {
		t.Run(name, func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
			host, err := hostclaim.LoadOrCreateHostIdentity(t.TempDir())
			require.NoError(t, err)
			storage.SetPRProvenanceSource(fakeProvenanceSource{host: host})
			var posted int
			h := additiveHandler(storage, true)
			h.resolveSessionRepo = repoFn
			h.postPRComment = func(context.Context, githubpkg.RepoRef, int, string) error { posted++; return nil }
			ctx := WithSessionUUID(context.Background(), sessionUUID)

			result, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, nil)))
			require.NoError(t, err)
			m := parseResult(t, result)
			require.False(t, m["success"].(bool))
			assert.Contains(t, m["error"].(map[string]interface{})["message"], "override_reason")
			assert.Zero(t, posted, "no provenance comment")
			fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
			require.NoError(t, gerr)
			assert.Zero(t, fetched.PrNumber, "the foreign PR did not become the item's anchor")

			h.resolveCallerGitHubLogin = func(context.Context) (string, error) { return "someone-else", nil }
			result, err = callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{"override_reason": "fork work"})))
			require.NoError(t, err)
			require.False(t, parseResult(t, result)["success"].(bool), "a PR the operator did not author is refused even with a reason")
			assert.Zero(t, posted)

			h.resolveCallerGitHubLogin = func(context.Context) (string, error) { return "tstapler", nil }
			result, err = callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{"override_reason": "fork work"})))
			require.NoError(t, err)
			require.False(t, result.IsError, "%v", result.Content)
			assert.Zero(t, posted, "an accepted off-repo PR is still never stamped")
		})
	}
}

func TestReportPRCreated_ShouldAcceptSchemelessURL_AndNormalizeRepoIdentity(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	h := additiveHandler(storage, true)
	ctx := WithSessionUUID(context.Background(), sessionUUID)
	result, err := callReportPR(h, ctx, makeToolReq(reportPRArgs(item.ID, map[string]interface{}{
		"pr_url": "github.com/tstapler/stapler-squad/pull/42",
	})))
	require.NoError(t, err)
	require.False(t, result.IsError, "%v", result.Content)

	a, _ := githubpkg.NewRepoRefWithHost("Acme", "App.git", "www.GitHub.com")
	b, _ := githubpkg.NewRepoRef("acme", "app")
	assert.True(t, sameRepo(a, b))
	assert.Equal(t, "https://github.com/o/r/pull/5", normalizeReportedPRURL(" github.com/o/r/pull/5 "))
	assert.Equal(t, "o/r#5", normalizeReportedPRURL("o/r#5"))
}

func TestReportPRCreated_ShouldNameUnparseableTrackedURL_WhenAdditive(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	pendingItemWithPR21(t, storage, item.ID)
	n, bad := 21, "not a url"
	_, err := storage.UpdateBacklogItem(context.Background(), item.ID, session.BacklogItemUpdate{PrNumber: &n, PrURL: &bad}, nil)
	require.NoError(t, err)
	h := additiveHandler(storage, true)
	result, err := callReportPR(h, WithSessionUUID(context.Background(), sessionUUID), makeToolReq(reportPRArgs(item.ID, nil)))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Contains(t, m["error"].(map[string]interface{})["message"], "could not determine the repository")
}
