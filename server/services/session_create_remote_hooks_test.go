package services

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

type ignoredPermissionHandler struct{}

func (ignoredPermissionHandler) HandlePermissionRequest(http.ResponseWriter, *http.Request) {}

// The pipeline is the only caller of setupRemoteApprovalHooks; without it a
// remote session's permission requests never reach the web UI.
func TestCreateSession_RemoteTarget_ShouldInjectTheRemoteApprovalHook(t *testing.T) {
	srv := startRemoteSessionTestSSHServer(t)
	fix := newRemoteSessionFixture(t, srv)
	fix.svc.SetPermissionRequestHandler(ignoredPermissionHandler{})
	plainDir := filepath.Join(fix.basePath, "hooked")
	require.NoError(t, os.MkdirAll(plainDir, 0o755))

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-hooked",
		Path:        plainDir,
		Program:     "sh",
		SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	settings := filepath.Join(plainDir, ".claude", "settings.local.json")
	socket := sshremote.RemoteApprovalSocketPath(plainDir, fix.svc.FindLiveInstance(resp.Msg.Session.Id).GetStableID())
	wait.RequireEventually(t, func() bool {
		raw, readErr := os.ReadFile(filepath.Clean(settings))
		return readErr == nil && strings.Contains(string(raw), socket)
	}, 30*time.Second, 50*time.Millisecond, "the remote hook config must name the relay socket %s", socket)
}
