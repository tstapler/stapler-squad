package services

import (
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/prompts"
)

// newPromptTestService builds a SessionService with an isolated PromptStore
// backed by a temp directory so tests never share or leak state.
func newPromptTestService(t *testing.T) *SessionService {
	t.Helper()
	svc := NewSessionService(createTestStorage(t), events.NewEventBus(10))
	svc.promptStore = prompts.NewPromptStore(t.TempDir() + "/prompts.json")
	return svc
}

// ─── ListPromptHistory ────────────────────────────────────────────────────────

func TestListPromptHistory_EmptyInitially(t *testing.T) {
	svc := newPromptTestService(t)
	resp, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Entries)
}

func TestListPromptHistory_ReturnsEntriesAfterRecord(t *testing.T) {
	svc := newPromptTestService(t)

	svc.promptStore.RecordUsage("write unit tests")
	svc.promptStore.RecordUsage("fix the linter")

	resp, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Entries, 2)

	texts := make([]string, len(resp.Msg.Entries))
	for i, e := range resp.Msg.Entries {
		texts[i] = e.Text
	}
	assert.Contains(t, texts, "write unit tests")
	assert.Contains(t, texts, "fix the linter")
}

func TestListPromptHistory_EntryFieldsPopulated(t *testing.T) {
	svc := newPromptTestService(t)
	svc.promptStore.RecordUsage("deploy to production")

	resp, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Entries, 1)

	entry := resp.Msg.Entries[0]
	assert.NotEmpty(t, entry.Id)
	assert.Equal(t, "deploy to production", entry.Text)
	assert.GreaterOrEqual(t, entry.UsedCount, int32(1))
	assert.NotNil(t, entry.LastUsed)
}

// ─── DeletePromptHistory ──────────────────────────────────────────────────────

func TestDeletePromptHistory_EmptyID(t *testing.T) {
	svc := newPromptTestService(t)
	_, err := svc.DeletePromptHistory(t.Context(), connect.NewRequest(&sessionv1.DeletePromptHistoryRequest{Id: ""}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestDeletePromptHistory_RemovesEntry(t *testing.T) {
	svc := newPromptTestService(t)
	svc.promptStore.RecordUsage("do something")

	listResp, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.Entries, 1)
	id := listResp.Msg.Entries[0].Id

	_, err = svc.DeletePromptHistory(t.Context(), connect.NewRequest(&sessionv1.DeletePromptHistoryRequest{Id: id}))
	require.NoError(t, err)

	listResp2, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	assert.Empty(t, listResp2.Msg.Entries)
}

func TestDeletePromptHistory_DeleteOneKeepsOthers(t *testing.T) {
	svc := newPromptTestService(t)
	svc.promptStore.RecordUsage("keep me")
	svc.promptStore.RecordUsage("delete me")

	listResp, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.Entries, 2)

	var deleteID string
	for _, e := range listResp.Msg.Entries {
		if e.Text == "delete me" {
			deleteID = e.Id
		}
	}
	require.NotEmpty(t, deleteID)

	_, err = svc.DeletePromptHistory(t.Context(), connect.NewRequest(&sessionv1.DeletePromptHistoryRequest{Id: deleteID}))
	require.NoError(t, err)

	listResp2, err := svc.ListPromptHistory(t.Context(), connect.NewRequest(&sessionv1.ListPromptHistoryRequest{}))
	require.NoError(t, err)
	require.Len(t, listResp2.Msg.Entries, 1)
	assert.Equal(t, "keep me", listResp2.Msg.Entries[0].Text)
}
