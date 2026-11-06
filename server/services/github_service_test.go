package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newGitHubService creates a GitHubService backed by a fresh in-memory test storage.
func newGitHubService(t *testing.T) *GitHubService {
	t.Helper()
	storage := createTestStorage(t)
	return NewGitHubService(storage)
}

// --------------------------------------------------------------------------
// GetPRInfo
// --------------------------------------------------------------------------

// TestGetPRInfo_EmptySessionID verifies that an empty session_id returns
// CodeInvalidArgument before any storage lookup is attempted.
func TestGetPRInfo_EmptySessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.GetPRInfo(context.Background(), connect.NewRequest(&sessionv1.GetPRInfoRequest{
		Id: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestGetPRInfo_UnknownSessionID verifies that a non-existent session ID returns
// CodeNotFound.
func TestGetPRInfo_UnknownSessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.GetPRInfo(context.Background(), connect.NewRequest(&sessionv1.GetPRInfoRequest{
		Id: "nonexistent-session-id",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// GetPRComments
// --------------------------------------------------------------------------

// TestGetPRComments_EmptySessionID verifies that an empty session_id returns
// CodeInvalidArgument.
func TestGetPRComments_EmptySessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.GetPRComments(context.Background(), connect.NewRequest(&sessionv1.GetPRCommentsRequest{
		Id: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestGetPRComments_UnknownSessionID verifies that a non-existent session ID
// returns CodeNotFound.
func TestGetPRComments_UnknownSessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.GetPRComments(context.Background(), connect.NewRequest(&sessionv1.GetPRCommentsRequest{
		Id: "nonexistent-session-id",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// PostPRComment
// --------------------------------------------------------------------------

// TestPostPRComment_EmptySessionID verifies that an empty session_id returns
// CodeInvalidArgument before any storage lookup.
func TestPostPRComment_EmptySessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.PostPRComment(context.Background(), connect.NewRequest(&sessionv1.PostPRCommentRequest{
		Id:   "",
		Body: "some comment",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestPostPRComment_EmptyComment verifies that an empty body returns
// CodeInvalidArgument even when a session ID is provided.
func TestPostPRComment_EmptyComment(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.PostPRComment(context.Background(), connect.NewRequest(&sessionv1.PostPRCommentRequest{
		Id:   "some-session-id",
		Body: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestPostPRComment_UnknownSessionID verifies that a valid body but non-existent
// session ID returns CodeNotFound.
func TestPostPRComment_UnknownSessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.PostPRComment(context.Background(), connect.NewRequest(&sessionv1.PostPRCommentRequest{
		Id:   "nonexistent-session-id",
		Body: "a comment body",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// ClosePR
// --------------------------------------------------------------------------

// TestClosePR_EmptySessionID verifies that an empty session_id returns
// CodeInvalidArgument.
func TestClosePR_EmptySessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.ClosePR(context.Background(), connect.NewRequest(&sessionv1.ClosePRRequest{
		Id: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestClosePR_UnknownSessionID verifies that a non-existent session ID returns
// CodeNotFound.
func TestClosePR_UnknownSessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.ClosePR(context.Background(), connect.NewRequest(&sessionv1.ClosePRRequest{
		Id: "nonexistent-session-id",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// MergePR
// --------------------------------------------------------------------------

// TestMergePR_EmptySessionID verifies that an empty session_id returns
// CodeInvalidArgument.
func TestMergePR_EmptySessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.MergePR(context.Background(), connect.NewRequest(&sessionv1.MergePRRequest{
		Id: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestMergePR_UnknownSessionID verifies that a non-existent session ID returns
// CodeNotFound.
func TestMergePR_UnknownSessionID(t *testing.T) {
	t.Parallel()
	svc := newGitHubService(t)

	_, err := svc.MergePR(context.Background(), connect.NewRequest(&sessionv1.MergePRRequest{
		Id: "nonexistent-session-id",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// classifyGitHubRateLimitError
// --------------------------------------------------------------------------

// rateLimitedUntilErr builds an error matching rateLimitTransport's fail-fast
// text (github/http_client.go's RoundTrip fmt.Errorf) so tests exercise the
// real wire format, not an invented one.
func rateLimitedUntilErr(resetAt time.Time) error {
	return fmt.Errorf("github: rate limited until %s, skipping request to avoid another guaranteed failure", resetAt.Format(time.RFC3339))
}

// TestClassifyGitHubRateLimitError verifies rateLimitTransport's fail-fast
// error text is reclassified into a reason=transient|exhausted marker based
// on secondaryRateLimitMaxWait (mirrors github/rate_limit.go's
// maxRetryAfterSleep), and that a non-matching error passes through
// unchanged.
func TestClassifyGitHubRateLimitError(t *testing.T) {
	t.Parallel()

	t.Run("transient when reset is within secondaryRateLimitMaxWait", func(t *testing.T) {
		t.Parallel()
		err := rateLimitedUntilErr(time.Now().Add(30 * time.Second))

		got := classifyGitHubRateLimitError(err)

		require.Error(t, got)
		require.Contains(t, got.Error(), "reason=transient")
		require.NotContains(t, got.Error(), "reason=exhausted")
	})

	t.Run("boundary: reset at secondaryRateLimitMaxWait is still transient", func(t *testing.T) {
		t.Parallel()
		// time.Until(resetAt) is re-evaluated inside classifyGitHubRateLimitError
		// a few microseconds after resetAt is computed here, so it always lands
		// at-or-just-under secondaryRateLimitMaxWait -- exercising the "<="
		// boundary (not "<") without wall-clock flakiness.
		err := rateLimitedUntilErr(time.Now().Add(secondaryRateLimitMaxWait))

		got := classifyGitHubRateLimitError(err)

		require.Error(t, got)
		require.Contains(t, got.Error(), "reason=transient")
	})

	t.Run("exhausted when reset is beyond secondaryRateLimitMaxWait", func(t *testing.T) {
		t.Parallel()
		err := rateLimitedUntilErr(time.Now().Add(secondaryRateLimitMaxWait + 5*time.Minute))

		got := classifyGitHubRateLimitError(err)

		require.Error(t, got)
		require.Contains(t, got.Error(), "reason=exhausted")
		require.NotContains(t, got.Error(), "reason=transient")
	})

	t.Run("non-rate-limit error passes through unchanged", func(t *testing.T) {
		t.Parallel()
		err := errors.New("dial tcp: connection refused")

		got := classifyGitHubRateLimitError(err)

		require.Same(t, err, got)
		require.NotContains(t, got.Error(), "reason=")
	})

	t.Run("nil error passes through as nil", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, classifyGitHubRateLimitError(nil))
	})
}
