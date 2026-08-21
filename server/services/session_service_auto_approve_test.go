package services

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// TestUpdateSession_AutoApprove_StoppedSession_PersistsAcrossReload is the AC9
// regression guard (persists across reload, readable via API) exercised at the RPC +
// real-storage level, not just the ent_repository unit level: enable auto_approve on a
// Stopped session, then reload from storage exactly like a server restart would.
func TestUpdateSession_AutoApprove_StoppedSession_PersistsAcrossReload(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:     "stopped-auto-approve-session",
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == "stopped-auto-approve-session" {
			addInstanceToPoller(fix.poller, li)
			break
		}
	}

	enable := true
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:          "stopped-auto-approve-session",
		AutoApprove: &enable,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.True(t, resp.Msg.Session.AutoApprove)

	reloaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := findInstanceByTitle(t, reloaded, "stopped-auto-approve-session")
	assert.True(t, found.AutoApprove, "auto_approve must survive a reload")
}

// TestUpdateSession_should_RejectAutoApprove_When_ProgramUnsupported mirrors the
// CreateSession-path guard: the post-creation toggle must not let an RPC caller bypass
// the client-side checkbox gate on an unsupported agent.
func TestUpdateSession_should_RejectAutoApprove_When_ProgramUnsupported(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:     "unsupported-agent-session",
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "codex",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == "unsupported-agent-session" {
			addInstanceToPoller(fix.poller, li)
			break
		}
	}

	enable := true
	_, err = fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:          "unsupported-agent-session",
		AutoApprove: &enable,
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
