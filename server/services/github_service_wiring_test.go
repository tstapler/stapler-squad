package services

import (
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// The PR RPCs must read the service's own storage, not an unset one.
func TestSessionService_should_ServePRRPCsFromItsStorage_When_Constructed(t *testing.T) {
	svc := newProviderWiringService(t)
	require.NoError(t, svc.storage.AddInstance(&instance.Instance{Title: "plain", UUID: "uuid-plain", Path: "/tmp/test", Status: domain.Stopped, Program: "claude"}))

	_, err := svc.GetPRInfo(t.Context(), connect.NewRequest(&sessionv1.GetPRInfoRequest{Id: "uuid-plain"}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), err.Error())
}
