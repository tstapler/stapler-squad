package backlogsvc

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func TestDismissWorkflowSuggestion(t *testing.T) {
	t.Parallel()
	storage, _ := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "x", Status: string(session.BacklogStatusIdea)})
	require.NoError(t, err)
	sugID, err := storage.AppendWorkflowSuggestion(t.Context(), item.ID, session.WorkflowSuggestionData{ActionType: "operator_note", Recommendation: "r"})
	require.NoError(t, err)

	cases := []struct {
		name          string
		itemID, sugID string
		wantCode      connect.Code
		ok            bool
	}{
		{"empty ids", "", "", connect.CodeInvalidArgument, false},
		{"unknown suggestion", item.ID, "missing", connect.CodeNotFound, false},
		{"success", item.ID, sugID, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.DismissWorkflowSuggestion(t.Context(), connect.NewRequest(&sessionv1.DismissWorkflowSuggestionRequest{ItemId: tc.itemID, SuggestionId: tc.sugID}))
			if tc.ok {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantCode, connect.CodeOf(err))
		})
	}
}
