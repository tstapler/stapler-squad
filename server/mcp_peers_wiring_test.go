package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/session"
)

func newPeersWiringStorage(t *testing.T) *session.Storage {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage
}

func TestMCPCoreOptions_should_CarryWorkspacePeers_When_StoragePresent(t *testing.T) {
	t.Parallel()
	opts := mcpCoreOptions(&ServerDependencies{Storage: newPeersWiringStorage(t)})
	assert.NotNil(t, servermcp.WorkspacePeersFromOptions(opts...), "list_workspace_peers needs its lister injected")
}

func TestMCPCoreOptions_should_LeaveWorkspacePeersUnset_When_NoStorage(t *testing.T) {
	t.Parallel()
	assert.True(t, servermcp.WorkspacePeersFromOptions(mcpCoreOptions(&ServerDependencies{})...) == nil)
}

func TestStdioMCPOptions_should_CarryWorkspacePeersLikeHTTP(t *testing.T) {
	t.Parallel()
	storage := newPeersWiringStorage(t)
	assert.NotNil(t, servermcp.WorkspacePeersFromOptions(stdioMCPOptions(storage)...), "stdio must build the peer lister from storage")
}
