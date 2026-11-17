package backlogsvc

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func newWaiveService(t *testing.T) (*BacklogService, string) {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	ac, err := domain.SerializeAcCriteria([]domain.AcCriterion{
		{Index: 0, Text: "needs macOS proof", Status: domain.AcStatusPending},
	})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{
		Title:              "waive item",
		Status:             string(domain.BacklogStatusInProgress),
		AcceptanceCriteria: ac,
	})
	require.NoError(t, err)
	return NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil), item.ID
}

func TestWaiveAcceptanceCriterion_should_RecordWaiverAndReturnItem_When_ReasonGiven(t *testing.T) {
	t.Parallel()
	svc, id := newWaiveService(t)

	resp, err := svc.WaiveAcceptanceCriterion(t.Context(), connect.NewRequest(&sessionv1.WaiveAcceptanceCriterionRequest{
		ItemId: id, CriterionIndex: 0, Reason: "owner accepts",
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Item.AcceptanceCriteria, 1)
	assert.Equal(t, string(domain.AcStatusWaived), resp.Msg.Item.AcceptanceCriteria[0].Status)
	assert.Equal(t, "owner accepts", resp.Msg.Item.AcceptanceCriteria[0].WaiverReason)
}

func TestWaiveAcceptanceCriterion_should_MapErrorCodes_When_InputInvalid(t *testing.T) {
	t.Parallel()
	svc, id := newWaiveService(t)
	cases := []struct {
		name string
		req  *sessionv1.WaiveAcceptanceCriterionRequest
		code connect.Code
	}{
		{"blank reason", &sessionv1.WaiveAcceptanceCriterionRequest{ItemId: id, Reason: "  "}, connect.CodeInvalidArgument},
		{"index out of range", &sessionv1.WaiveAcceptanceCriterionRequest{ItemId: id, CriterionIndex: 9, Reason: "r"}, connect.CodeInvalidArgument},
		{"unknown item", &sessionv1.WaiveAcceptanceCriterionRequest{ItemId: "bl_01HXYZABCDEFGHJKMNPQRSTVWX", Reason: "r"}, connect.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.WaiveAcceptanceCriterion(t.Context(), connect.NewRequest(tc.req))
			require.Error(t, err)
			assert.Equal(t, tc.code, connect.CodeOf(err))
		})
	}
}
