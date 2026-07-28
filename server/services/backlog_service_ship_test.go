package services

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// fakePRRunner is a test double for PRRunner.
type fakePRRunner struct {
	prURL string
	err   error
	// calls records (sessionID, prompt) pairs for each invocation, so tests can
	// assert TriggerShipPR resolved the right work session.
	calls []struct {
		sessionID string
		prompt    string
	}
}

func (f *fakePRRunner) RunOneShotForSession(_ context.Context, sessionID, prompt string, _ int32) (string, error) {
	f.calls = append(f.calls, struct {
		sessionID string
		prompt    string
	}{sessionID, prompt})
	if f.err != nil {
		return "", f.err
	}
	return f.prURL, nil
}

// seedReviewItemWithWorkSession creates a backlog item in review status with a
// single work-role ItemSession, returning the item ID and the work session's
// stable UUID. Shared setup for TriggerShipPR's tests.
func seedReviewItemWithWorkSession(t *testing.T, svc *BacklogService) (itemID, workSessionUUID string) {
	t.Helper()
	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item ready to ship",
		SkipPlanning: true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "done"},
		},
	}))
	require.NoError(t, err)
	itemID = created.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
		string(session.BacklogStatusReview),
	} {
		_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	workSessionUUID = "work-session-uuid"
	_, err = svc.storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: workSessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	return itemID, workSessionUUID
}

// TestTriggerShipPR_HappyPath_RunsOneShotForMostRecentWorkSession verifies the
// core wiring: TriggerShipPR resolves the item's most recent work session and
// delegates to PRRunner.RunOneShotForSession with that session's stable UUID,
// returning the extracted PR URL.
func TestTriggerShipPR_HappyPath_RunsOneShotForMostRecentWorkSession(t *testing.T) {
	svc := newBacklogService(t)
	runner := &fakePRRunner{prURL: "https://github.com/example/repo/pull/7"}
	svc.SetOneShotRunner(runner)

	itemID, workSessionUUID := seedReviewItemWithWorkSession(t, svc)

	resp, err := svc.TriggerShipPR(t.Context(), connect.NewRequest(&sessionv1.TriggerShipPRRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/example/repo/pull/7", resp.Msg.PrUrl)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, workSessionUUID, runner.calls[0].sessionID, "must run the one-shot prompt against the item's work session, not e.g. its review session")
	assert.NotEmpty(t, runner.calls[0].prompt)
}

// TestTriggerShipPR_NotInReviewStatus_ReturnsFailedPrecondition verifies items
// outside review status (e.g. still in_progress) are rejected — Ship PR only
// makes sense once a review verdict is on record.
func TestTriggerShipPR_NotInReviewStatus_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)
	svc.SetOneShotRunner(&fakePRRunner{prURL: "https://github.com/example/repo/pull/7"})

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item still in progress",
		SkipPlanning: true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := created.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
	} {
		_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	_, err = svc.TriggerShipPR(t.Context(), connect.NewRequest(&sessionv1.TriggerShipPRRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// TestTriggerShipPR_AlreadyHasPR_ReturnsFailedPrecondition verifies an item that
// already has a PR is rejected rather than kicking off a redundant one-shot run.
func TestTriggerShipPR_AlreadyHasPR_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)
	runner := &fakePRRunner{prURL: "https://github.com/example/repo/pull/99"}
	svc.SetOneShotRunner(runner)

	itemID, _ := seedReviewItemWithWorkSession(t, svc)
	prURL := "https://github.com/example/repo/pull/1"
	prNumber := 1
	_, err := svc.storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)

	_, err = svc.TriggerShipPR(t.Context(), connect.NewRequest(&sessionv1.TriggerShipPRRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Empty(t, runner.calls, "must not run a redundant one-shot when a PR already exists")
}

// TestTriggerShipPR_NoWorkSession_ReturnsFailedPrecondition verifies an item
// with no work session at all (nothing was ever built) is rejected with a clear
// error rather than a nil-pointer panic or a confusing internal error.
func TestTriggerShipPR_NoWorkSession_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)
	svc.SetOneShotRunner(&fakePRRunner{prURL: "https://github.com/example/repo/pull/7"})

	item, err := svc.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:  "item with no work session",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = svc.TriggerShipPR(t.Context(), connect.NewRequest(&sessionv1.TriggerShipPRRequest{ItemId: item.ID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// TestTriggerShipPR_NoOneShotRunnerWired_ReturnsUnimplemented verifies the
// degrade-gracefully contract other optional-dependency RPCs in this service
// follow (e.g. SessionCreator-dependent handlers) — a server that never wired
// SetOneShotRunner must reject with CodeUnimplemented, not panic.
func TestTriggerShipPR_NoOneShotRunnerWired_ReturnsUnimplemented(t *testing.T) {
	svc := newBacklogService(t)

	itemID, _ := seedReviewItemWithWorkSession(t, svc)

	_, err := svc.TriggerShipPR(t.Context(), connect.NewRequest(&sessionv1.TriggerShipPRRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

// TestTriggerShipPR_RunnerError_ReturnsFailedPrecondition verifies a one-shot
// run failure (e.g. the work session's tmux instance is no longer running) is
// surfaced as an actionable error rather than a generic 500.
func TestTriggerShipPR_RunnerError_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)
	runner := &fakePRRunner{err: errors.New("session not found")}
	svc.SetOneShotRunner(runner)

	itemID, _ := seedReviewItemWithWorkSession(t, svc)

	_, err := svc.TriggerShipPR(t.Context(), connect.NewRequest(&sessionv1.TriggerShipPRRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
