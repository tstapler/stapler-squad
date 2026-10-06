package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestTmuxSessionKillAllowed_OwnershipPolicy(t *testing.T) {
	orig := readSessionOwnerUUID
	t.Cleanup(func() { readSessionOwnerUUID = orig })

	stub := func(marker string, err error) {
		readSessionOwnerUUID = func(context.Context, tmux.Socket, string) (string, error) { return marker, err }
	}

	tests := []struct {
		name    string
		marker  string
		err     error
		allowed []string
		want    bool
	}{
		{"owner matches", "uuid-a", nil, []string{"uuid-a"}, true},
		{"owner is a different instance", "uuid-other", nil, []string{"uuid-a"}, false},
		{"empty allowed list refuses", "uuid-a", nil, nil, false},
		{"marker never set (pre-fix pane) refuses", "", errors.New("unknown variable: STAPLER_SESSION_UUID"), []string{"uuid-a"}, false},
		{"session already gone is a no-op kill", "", errors.New("can't find session: x"), []string{"uuid-a"}, true},
		{"no tmux server running is a no-op kill", "", errors.New("no server running on /tmp/x"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub(tt.marker, tt.err)
			require.Equal(t, tt.want, tmuxSessionKillAllowed(t.Context(), "staplersquad_x", tt.allowed, false))
		})
	}
}

func TestKillTmuxSessionByTitle_RefusedReturnsSentinel(t *testing.T) {
	orig := readSessionOwnerUUID
	t.Cleanup(func() { readSessionOwnerUUID = orig })
	readSessionOwnerUUID = func(context.Context, tmux.Socket, string) (string, error) { return "uuid-other", nil }

	err := (&SessionService{}).KillTmuxSessionByTitle(t.Context(), "some title", "uuid-a")
	require.ErrorIs(t, err, ErrTmuxKillRefused)
}

func TestIsTmuxSessionAbsentText(t *testing.T) {
	for _, s := range []string{"can't find session: x", "no such session: x", "no server running on /tmp/x", "error connecting to /tmp/x"} {
		require.True(t, isTmuxSessionAbsentText(s), s)
	}
	require.True(t, isTmuxSessionAbsentText("No Server Running"))
	require.False(t, isTmuxSessionAbsentText("permission denied"))
}

func TestTmuxSessionKillAllowed_AllowUnmarked(t *testing.T) {
	orig := readSessionOwnerUUID
	t.Cleanup(func() { readSessionOwnerUUID = orig })
	stub := func(marker string, err error) {
		readSessionOwnerUUID = func(context.Context, tmux.Socket, string) (string, error) { return marker, err }
	}

	stub("", errors.New("unknown variable: STAPLER_SESSION_UUID"))
	require.True(t, tmuxSessionKillAllowed(t.Context(), "staplersquad_x", []string{"uuid-a"}, true), "unmarked pane is killable when allowed")
	require.False(t, tmuxSessionKillAllowed(t.Context(), "staplersquad_x", []string{"uuid-a"}, false))

	stub("", errors.New("context deadline exceeded"))
	require.False(t, tmuxSessionKillAllowed(t.Context(), "staplersquad_x", []string{"uuid-a"}, true), "an unreadable marker is never treated as unmarked")

	stub("uuid-other", nil)
	require.False(t, tmuxSessionKillAllowed(t.Context(), "staplersquad_x", []string{"uuid-a"}, true), "a foreign marker is refused even when unmarked panes are allowed")
}

// deleteNonLiveSession deletes a stored, non-live session and returns the tmux
// sessions kill-session was issued for, given what show-environment reports
// for the pane (marker, err).
func deleteNonLiveSession(t *testing.T, title, uuid, marker string, markerErr error) (killed []string) {
	t.Helper()
	origRead, origKill := readSessionOwnerUUID, runTmuxKillSession
	t.Cleanup(func() { readSessionOwnerUUID, runTmuxKillSession = origRead, origKill })
	readSessionOwnerUUID = func(context.Context, tmux.Socket, string) (string, error) { return marker, markerErr }
	runTmuxKillSession = func(_ context.Context, name string) ([]byte, error) {
		killed = append(killed, name)
		return nil, nil
	}

	svc := NewSessionService(createTestStorage(t), events.NewEventBus(100))
	require.NoError(t, svc.storage.AddInstance(&session.Instance{
		Title: title, UUID: uuid, Path: "/tmp/test", Status: session.Paused, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	_, err := svc.DeleteSession(t.Context(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: uuid}))
	require.NoError(t, err)
	svc.Shutdown() // waits for the tracked tmux cleanup goroutine
	return killed
}

func TestDeleteSession_NonLive_KillsOwnPane(t *testing.T) {
	const uuid = "aaaaaaaa-0000-0000-0000-000000000001"
	require.Equal(t, []string{"staplersquad_del-own"}, deleteNonLiveSession(t, "del-own", uuid, uuid, nil))
}

// A pre-marker pane left under the deleted session's name must not outlive it.
func TestDeleteSession_NonLive_KillsMarkerlessPane(t *testing.T) {
	const uuid = "aaaaaaaa-0000-0000-0000-000000000002"
	killed := deleteNonLiveSession(t, "del-unmarked", uuid, "", errors.New("unknown variable: STAPLER_SESSION_UUID"))
	require.Equal(t, []string{"staplersquad_del-unmarked"}, killed)
}

// A pane stamped for a different session is never killed by this delete.
func TestDeleteSession_NonLive_RefusesForeignPane(t *testing.T) {
	const uuid = "aaaaaaaa-0000-0000-0000-000000000003"
	require.Empty(t, deleteNonLiveSession(t, "del-foreign", uuid, "bbbbbbbb-0000-0000-0000-000000000009", nil))
}
