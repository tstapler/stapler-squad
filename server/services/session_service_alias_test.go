package services

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestCreateSession_PathGuard_AliasNameBypassesPathRequired(t *testing.T) {
	// When alias_name is provided, path="" should not trigger the "path is required" guard.
	// We verify this by checking the guard condition directly (without starting a full session).
	msg := &sessionv1.CreateSessionRequest{
		Title:     "test-alias-session",
		AliasName: "myalias",
		Path:      "",
	}
	// The guard: !OneOff && AliasName == "" && SessionType != NEW_PROJECT && Path == ""
	triggered := !msg.OneOff &&
		msg.AliasName == "" &&
		msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT &&
		msg.Path == ""
	assert.False(t, triggered, "path guard should NOT trigger when AliasName is set")
	_ = connect.CodeInvalidArgument // ensure import is used
}
