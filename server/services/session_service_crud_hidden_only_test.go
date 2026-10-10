package services

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func seedHiddenAndVisible(t *testing.T, fix *forkTestFixture, hidden, visible int) {
	t.Helper()
	for i := 0; i < hidden; i++ {
		fix.poller.AddInstance(&session.Instance{
			Title:   fmt.Sprintf("hidden-%03d", i),
			UUID:    fmt.Sprintf("a1000000-0000-0000-0000-%012d", i),
			Status:  session.Active,
			Program: "claude",
			Path:    "/tmp/test",
			Hidden:  true,
		})
	}
	for i := 0; i < visible; i++ {
		fix.poller.AddInstance(&session.Instance{
			Title:   fmt.Sprintf("visible-%03d", i),
			UUID:    fmt.Sprintf("b2000000-0000-0000-0000-%012d", i),
			Status:  session.Active,
			Program: "claude",
			Path:    "/tmp/test",
		})
	}
}

func TestListSessions_ShouldReturn55ForHiddenOnlyAnd185ForDefault_WhenSeeded(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	seedHiddenAndVisible(t, fix, 55, 185)

	only, err := fix.svc.ListSessions(context.Background(),
		connect.NewRequest(&sessionv1.ListSessionsRequest{HiddenOnly: true}))
	require.NoError(t, err)
	assert.Len(t, only.Msg.Sessions, 55)
	for _, s := range only.Msg.Sessions {
		assert.True(t, s.Hidden, "hidden_only must return only hidden sessions: %s", s.Title)
	}

	def, err := fix.svc.ListSessions(context.Background(),
		connect.NewRequest(&sessionv1.ListSessionsRequest{}))
	require.NoError(t, err)
	assert.Len(t, def.Msg.Sessions, 185)
	for _, s := range def.Msg.Sessions {
		assert.False(t, s.Hidden)
	}
}

func TestListSessions_ShouldStillReturnOnlyHidden_WhenHiddenOnlyAndIncludeHiddenFalse(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	seedHiddenAndVisible(t, fix, 2, 3)

	resp, err := fix.svc.ListSessions(context.Background(),
		connect.NewRequest(&sessionv1.ListSessionsRequest{HiddenOnly: true, IncludeHidden: false}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Sessions, 2)

	both, err := fix.svc.ListSessions(context.Background(),
		connect.NewRequest(&sessionv1.ListSessionsRequest{IncludeHidden: true}))
	require.NoError(t, err)
	assert.Len(t, both.Msg.Sessions, 5, "include_hidden alone keeps returning both kinds")
}
