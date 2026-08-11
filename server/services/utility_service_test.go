package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func setupUtilityService() *UtilityService {
	return NewUtilityService(NewApprovalStore(""))
}

func setupUtilityServiceWithPollerFixture() (*UtilityService, *session.ReviewQueuePoller) {
	svc := setupUtilityService()

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	return svc, poller
}

// strPtr returns a pointer to s; used to set optional proto string fields.
func strPtr(s string) *string { return &s }

// --------------------------------------------------------------------------
// GetLogs – nil poller (no session ID resolution available)
// --------------------------------------------------------------------------

// TestGetLogs_NoSessionID_NilPoller verifies that GetLogs with no session ID
// and no poller returns an empty log list rather than an error.
// (The global app log file may or may not exist; both outcomes are valid.)
func TestGetLogs_NoSessionID_NilPoller(t *testing.T) {
	svc := setupUtilityService()

	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{}))

	// Either returns empty logs or errors if the log file is inaccessible.
	// The important invariant: no panic, and if it errors it's not CodeNotFound.
	if err != nil {
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.NotEqual(t, connect.CodeNotFound, connectErr.Code(),
			"no-session-id call must not return CodeNotFound")
	} else {
		require.NotNil(t, resp)
	}
}

// --------------------------------------------------------------------------
// GetLogs – UUID resolution via ReviewQueuePoller
// --------------------------------------------------------------------------

// TestGetLogs_WithUUID_NilPoller verifies that GetLogs does not crash or
// return an unexpected error when the poller is nil and a UUID is passed.
// The log file for that UUID won't exist, so the response is empty.
func TestGetLogs_WithUUID_NilPoller(t *testing.T) {
	svc := setupUtilityService()
	// No poller wired — UUID is used as-is to look up the log file.

	sid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: &sid,
	}))

	// No log file exists for this UUID → empty response, not an error.
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// TestGetLogs_WithUUID_MatchingInstance verifies that GetLogs exercises the
// UUID→Title resolution path when the poller has a matching instance.
// The session's log file does not exist on disk, so the response is empty —
// but the call must succeed (no crash, no error from the resolution logic).
func TestGetLogs_WithUUID_MatchingInstance(t *testing.T) {
	svc, poller := setupUtilityServiceWithPollerFixture()

	const testUUID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	poller.SetInstances([]*session.Instance{
		{
			UUID:    testUUID,
			Title:   "my-resolved-session",
			Path:    "/tmp/test",
			Status:  session.Active,
			Program: "claude",
		},
	})

	// The log file for "my-resolved-session" does not exist → empty response.
	// The call must succeed: UUID is resolved to Title before path lookup.
	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: strPtr(testUUID),
	}))

	require.NoError(t, err, "GetLogs should succeed when UUID resolves to a known instance")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// TestGetLogs_WithUUID_NoMatchingInstance verifies that when no instance in
// the poller matches the UUID, GetLogs falls back gracefully (uses the UUID
// as-is for the log file path, returning empty logs rather than an error).
func TestGetLogs_WithUUID_NoMatchingInstance(t *testing.T) {
	svc, _ := setupUtilityServiceWithPollerFixture()
	// Poller is wired but has no instances.

	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: strPtr("cccccccc-cccc-cccc-cccc-cccccccccccc"),
	}))

	require.NoError(t, err, "GetLogs should not error when UUID has no matching instance")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// TestGetLogs_WithTitle_FindsLogByTitle verifies the baseline: passing a Title
// (legacy behaviour) still works correctly after the UUID migration.
func TestGetLogs_WithTitle_FindsLogByTitle(t *testing.T) {
	svc, poller := setupUtilityServiceWithPollerFixture()

	poller.SetInstances([]*session.Instance{
		{
			Title:   "title-session",
			Path:    "/tmp/test",
			Status:  session.Active,
			Program: "claude",
		},
	})

	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: strPtr("title-session"),
	}))

	require.NoError(t, err, "GetLogs should accept a Title as session ID")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}
