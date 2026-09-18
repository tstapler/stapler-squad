package services

import (
	"testing"

	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/proto"
)

// TestCreateSessionRequest_should_PreserveRemoteAndSessionTypeIndependently_When_RoundTrippedThroughProto
// is the Story 4.1.1 acceptance test (ssh-remote-workspaces plan.md Epic 4.1, ADR-001): remote
// (field 31, a RemoteTarget referencing a saved RemoteConfig by name) and session_type (field
// 13) are two orthogonal fields on CreateSessionRequest -- ADR-001 deliberately added remote as
// its own field instead of a parallel SESSION_TYPE_REMOTE_* enum value precisely so every
// existing SessionType composes with it. This proves that composition survives a real
// marshal/unmarshal cycle: both fields decode back to their original values, neither one
// clobbers or is inferred from the other.
func TestCreateSessionRequest_should_PreserveRemoteAndSessionTypeIndependently_When_RoundTrippedThroughProto(t *testing.T) {
	original := &sessionv1.CreateSessionRequest{
		Title:       "remote-roundtrip-test",
		Path:        "/repo",
		Branch:      "feature-x",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote: &sessionv1.RemoteTarget{
			RemoteName: "prod-box",
		},
	}

	wire, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &sessionv1.CreateSessionRequest{}
	require.NoError(t, proto.Unmarshal(wire, decoded))

	require.Equal(t, sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE, decoded.GetSessionType(),
		"session_type must survive the round trip unchanged by the presence of remote")
	require.NotNil(t, decoded.GetRemote(), "remote must survive the round trip")
	require.Equal(t, "prod-box", decoded.GetRemote().GetRemoteName(),
		"remote.remote_name must survive the round trip unchanged by the presence of session_type")
}

// TestCreateSessionRequest_should_LeaveRemoteNil_When_NotSet is the backward-compatibility
// guard: a request that never sets remote (every existing client, pre-Epic-4.1) must decode
// with a nil Remote, not a zero-value RemoteTarget -- callers use a nil check to decide
// whether to resolve a remote target at all (Epic 4.2's resolveRemoteTarget).
func TestCreateSessionRequest_should_LeaveRemoteNil_When_NotSet(t *testing.T) {
	original := &sessionv1.CreateSessionRequest{
		Title:       "local-only-test",
		Path:        "/repo",
		SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
	}

	wire, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &sessionv1.CreateSessionRequest{}
	require.NoError(t, proto.Unmarshal(wire, decoded))

	require.Nil(t, decoded.GetRemote())
	require.Equal(t, sessionv1.SessionType_SESSION_TYPE_DIRECTORY, decoded.GetSessionType())
}
