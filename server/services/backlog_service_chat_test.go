package services

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func TestDeriveChatItemTitle(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{"short single line", "Add dark mode", "Add dark mode"},
		{"multiline keeps only first line", "Add dark mode\nAlso fix the toggle", "Add dark mode"},
		{"truncates beyond max length", strings.Repeat("a", 100), strings.Repeat("a", chatDerivedTitleMaxLen)},
		{"trims surrounding whitespace on first line", "  Add dark mode  \nrest", "Add dark mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveChatItemTitle(tt.message))
		})
	}
}

// TestCreateBacklogItemFromChat_should_CreateItemViaCreateBacklogItem_When_NoExistingItemId
// is the AC2 data-shape-parity regression guard: the first turn of a chat
// conversation (no existing_item_id) must produce a real BacklogItem through
// the same CreateBacklogItem internal path the structured form uses — title
// and description both come from the raw message, not a separate creation
// path with its own field-mapping logic.
func TestCreateBacklogItemFromChat_should_CreateItemViaCreateBacklogItem_When_NoExistingItemId(t *testing.T) {
	svc := newBacklogService(t)

	resp, err := svc.CreateBacklogItemFromChat(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemFromChatRequest{
		Message: "Add dark mode support",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
	assert.Equal(t, "Add dark mode support", resp.Msg.Item.Title)
	assert.Equal(t, "Add dark mode support", resp.Msg.Item.Description)
	assert.Equal(t, string(session.BacklogStatusIdea), resp.Msg.Item.Status)
}

// TestCreateBacklogItemFromChat_should_RejectEmptyMessage_When_MessageIsWhitespaceOnly
// guards the client+server validation edge case from validation.md: an empty
// or whitespace-only chat message must never reach CreateBacklogItem/
// TriggerTriage.
func TestCreateBacklogItemFromChat_should_RejectEmptyMessage_When_MessageIsWhitespaceOnly(t *testing.T) {
	svc := newBacklogService(t)

	_, err := svc.CreateBacklogItemFromChat(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemFromChatRequest{
		Message: "   ",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestCreateBacklogItemFromChat_should_DelegateToTriggerTriageWithFeedback_When_ExistingItemIdSet
// verifies a refinement turn (existing_item_id set) delegates to TriggerTriage
// with feedback = message, per AC1/AC2 — not a parallel refinement path.
func TestCreateBacklogItemFromChat_should_DelegateToTriggerTriageWithFeedback_When_ExistingItemIdSet(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item awaiting refinement",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	// First triage run must complete (feedback-driven refine requires a prior
	// completed result — see TriggerTriage's 3c guard). Waiting for the item to
	// reach "ready" (not just for the headless call to fire) also clears the
	// triageInFlight single-flight entry the goroutine holds until it finishes
	// persisting the result — see TriggerTriage's own defer at backlog_service_
	// triage.go:2585.
	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: item.ID}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5e9, 5e7, "initial triage must complete before refinement")

	resp, chatErr := svc.CreateBacklogItemFromChat(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemFromChatRequest{
		ExistingItemId: item.ID,
		Message:        "please tighten the acceptance criteria",
	}))
	require.NoError(t, chatErr)
	require.NotNil(t, resp.Msg.Item)
	assert.True(t, resp.Msg.TriageTriggered)

	require.Eventually(t, func() bool { return pool.callCount() == 2 }, 5e9, 5e7, "refinement turn must trigger a second headless call")
}

// TestCreateBacklogItemFromChat_should_UseChatModeRetriagePrompt_When_RefiningExistingItem
// is the AC3 regression guard: a chat-originated refinement turn must use the
// tightened one-question-per-turn retriage prompt (session.
// BuildHeadlessChatRetriagePrompt), not the plain BuildHeadlessRetriagePrompt
// the structured refine-feedback form uses — clarifying questions should be
// surfaced one at a time in a live conversation rather than as a batch dump.
func TestCreateBacklogItemFromChat_should_UseChatModeRetriagePrompt_When_RefiningExistingItem(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item awaiting chat refinement",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: item.ID}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5e9, 5e7, "initial triage must complete before refinement")

	_, chatErr := svc.CreateBacklogItemFromChat(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemFromChatRequest{
		ExistingItemId: item.ID,
		Message:        "please tighten the acceptance criteria",
	}))
	require.NoError(t, chatErr)

	require.Eventually(t, func() bool { return pool.callCount() == 2 }, 5e9, 5e7, "refinement turn must trigger a second headless call")
	gotPrompt := pool.callAt(1).userPrompt
	assert.Contains(t, gotPrompt, "AT MOST ONE",
		"chat-originated refinement must use the tightened one-question-per-turn prompt, got: %s", gotPrompt)
}

// TestCreateBacklogItemFromChat_should_RejectRefinement_When_ItemNotInIdeaOrReadyStatus
// is the edge-case guard from validation.md: TriggerTriage's existing status
// guard must not be bypassed by the chat front door.
func TestCreateBacklogItemFromChat_should_RejectRefinement_When_ItemNotInIdeaOrReadyStatus(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "in-progress item",
		Status:   string(session.BacklogStatusInProgress),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, chatErr := svc.CreateBacklogItemFromChat(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemFromChatRequest{
		ExistingItemId: item.ID,
		Message:        "can we add one more thing",
	}))
	require.Error(t, chatErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(chatErr))
}
