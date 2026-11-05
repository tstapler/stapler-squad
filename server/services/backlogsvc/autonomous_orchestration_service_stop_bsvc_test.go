package backlogsvc

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// OBS-7: the operator read path returns the detail unchanged.
func TestStuckRowContext_ReturnedByListStuckBacklogItems(t *testing.T) {
	f := newCharFixture(t, "stop-detail-rpc", session.SessionRoleWork, session.BacklogStatusInProgress)
	svc := NewBacklogService(f.storage, nil, nil, nil, nil, nil)
	ctx := t.Context()
	detail := "stopped: idle_prompt after 2 turns (orchestrator_error)" // any detail string: the read path returns it unchanged

	applied, err := f.storage.MarkStuck(ctx, f.itemID, domain.StuckReasonAutonomousStuck, session.BacklogStatusInProgress, detail)
	require.NoError(t, err)
	require.True(t, applied)

	resp, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Items, 1)
	assert.Equal(t, detail, resp.Msg.Items[0].Context)
}
