package services

import (
	"context"
	"testing"

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
	svc := newGitHubService(t)

	_, err := svc.MergePR(context.Background(), connect.NewRequest(&sessionv1.MergePRRequest{
		Id: "nonexistent-session-id",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}
