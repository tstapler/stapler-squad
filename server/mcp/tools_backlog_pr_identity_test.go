package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// asVerified marks the unverified caller already on ctx as verified, as the
// HTTP identity middleware does when the session proof checks out.
func asVerified(ctx context.Context) context.Context {
	id, _ := sessionUUIDFromContext(ctx)
	return WithVerifiedSessionUUID(ctx, id)
}

// reportPRIdentityRig registers the real tool set and returns report_pr_created's
// registered handler, so a regression in registration is caught too.
func reportPRIdentityRig(t *testing.T) (mcpserver.ToolHandlerFunc, *session.Storage, *backlogdto.BacklogItemData, string) {
	t.Helper()
	storage := newTestBacklogStorage(t)
	item, workUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	h := &backlogHandlers{
		storage:            storage,
		resolveSessionRepo: testSessionRepo, resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, githubpkg.RepoRef, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	s := mcpserver.NewMCPServer("test", "0.0.0")
	registerBacklogTools(s, h)
	tool := s.GetTool("report_pr_created")
	require.NotNil(t, tool)
	return tool.Handler, storage, item, workUUID
}

func TestReportPRCreatedIdentity_should_OnlyTrustTheVerifiedCredential(t *testing.T) {
	t.Parallel()
	otherUUID := uuid.New().String()
	cases := []struct {
		name        string
		ctx         func(workUUID string) context.Context
		wantCode    string // "" means success
		wantPRSaved bool
	}{
		{"claimed UUID of the linked session without credential is rejected",
			func(w string) context.Context { return WithSessionUUID(context.Background(), w) }, CodeIdentityUnverified, false},
		{"no identity at all is rejected",
			func(string) context.Context { return context.Background() }, CodeIdentityUnverified, false},
		{"verified UUID of a session not linked to the item is rejected",
			func(string) context.Context { return WithVerifiedSessionUUID(context.Background(), otherUUID) }, ErrPermissionDenied, false},
		{"verified linked work session is accepted",
			func(w string) context.Context { return asVerified(WithSessionUUID(context.Background(), w)) }, "", true},
		{"claim of the linked session while verified as another session is rejected",
			func(w string) context.Context {
				return WithVerifiedSessionUUID(WithSessionUUID(context.Background(), w), otherUUID)
			}, ErrPermissionDenied, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler, storage, item, workUUID := reportPRIdentityRig(t)
			res, err := handler(tc.ctx(workUUID), makeToolReq(map[string]interface{}{
				"item_id": item.ID, "pr_url": "https://github.com/tstapler/stapler-squad/pull/42",
				"pr_number": float64(42), "summary": "Shipped.",
			}))
			require.NoError(t, err)
			if tc.wantCode == "" {
				assert.False(t, res.IsError, "legitimate flow must succeed")
			} else {
				assert.Equal(t, tc.wantCode, resultCode(parseResult(t, res)))
			}
			got, err := storage.GetBacklogItem(context.Background(), item.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPRSaved, got.PrNumber == 42, "PR written iff identity verified")
		})
	}
}

func TestReportPRCreatedIdentity_should_RejectVerifiedCallerLinkedToAnotherItem(t *testing.T) {
	t.Parallel()
	handler, storage, _, workUUID := reportPRIdentityRig(t)
	other, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "Other", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	res, err := handler(asVerified(WithSessionUUID(context.Background(), workUUID)), makeToolReq(map[string]interface{}{
		"item_id": other.ID, "pr_url": "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42), "summary": "Shipped.",
	}))
	require.NoError(t, err)
	assert.Equal(t, ErrPermissionDenied, resultCode(parseResult(t, res)))
	got, err := storage.GetBacklogItem(context.Background(), other.ID)
	require.NoError(t, err)
	assert.Zero(t, got.PrNumber)
}
