package services

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func addHiddenInstance(fix *forkTestFixture, title, uuid string, status session.Status) {
	fix.poller.AddInstance(&session.Instance{
		Title: title, UUID: uuid, Status: status, Program: "claude", Path: "/tmp/test", Hidden: true,
	})
}

func outcomesByTitle(resp *sessionv1.ListSessionsResponse) map[string]string {
	m := map[string]string{}
	for _, s := range resp.Sessions {
		m[s.Title] = s.BackgroundOutcome
	}
	return m
}

func TestListSessions_should_populateBackgroundOutcome_When_hiddenOnly_And_leaveEmptyWhenNot(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	ctx := context.Background()

	item, err := fix.storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "outcome"})
	require.NoError(t, err)
	_, err = fix.storage.CreateItemSessionWithVerdict(ctx,
		session.ItemSessionData{ItemID: item.ID, SessionUUID: "c0000000-0000-0000-0000-000000000001", SessionRole: session.SessionRoleReview},
		session.ReviewVerdictData{OverallOutcome: "PASS"})
	require.NoError(t, err)

	addHiddenInstance(fix, "done", "c0000000-0000-0000-0000-000000000001", session.Stopped)
	addHiddenInstance(fix, "crashed", "c0000000-0000-0000-0000-000000000002", session.Crashed)
	addHiddenInstance(fix, "bare", "c0000000-0000-0000-0000-000000000003", session.Stopped)

	only, err := fix.svc.ListSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{HiddenOnly: true}))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"done": "completed", "crashed": "failed", "bare": "unknown"}, outcomesByTitle(only.Msg))

	all, err := fix.svc.ListSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{IncludeHidden: true}))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"done": "", "crashed": "", "bare": ""}, outcomesByTitle(all.Msg))
}

type countingOutcomeLookup struct {
	batchCalls atomic.Int32
	lastLen    atomic.Int32
}

func (c *countingOutcomeLookup) ListItemSessionsBySessionUUIDs(_ context.Context, uuids []string) (map[string]session.ItemSessionSummary, error) {
	c.batchCalls.Add(1)
	c.lastLen.Store(int32(len(uuids)))
	return map[string]session.ItemSessionSummary{}, nil
}

func TestListSessions_should_callBatchLookupOnce_When_40HiddenSessions(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	fake := &countingOutcomeLookup{}
	fix.svc.outcomeLookup = fake
	for i := 0; i < 40; i++ {
		addHiddenInstance(fix, fmt.Sprintf("h-%02d", i), fmt.Sprintf("d0000000-0000-0000-0000-%012d", i), session.Stopped)
	}

	resp, err := fix.svc.ListSessions(context.Background(), connect.NewRequest(&sessionv1.ListSessionsRequest{HiddenOnly: true}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Sessions, 40)
	assert.EqualValues(t, 1, fake.batchCalls.Load())
	assert.EqualValues(t, 40, fake.lastLen.Load())

	_, err = fix.svc.ListSessions(context.Background(), connect.NewRequest(&sessionv1.ListSessionsRequest{IncludeHidden: true}))
	require.NoError(t, err)
	assert.EqualValues(t, 1, fake.batchCalls.Load(), "non-hidden_only requests must not query")
}
