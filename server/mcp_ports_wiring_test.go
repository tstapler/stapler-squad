package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	githubpkg "github.com/tstapler/stapler-squad/github"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
)

func TestMCPAutoReopener_should_ReturnNilInterface_When_NoBacklogService(t *testing.T) {
	t.Parallel()
	assert.True(t, mcpAutoReopener(nil) == nil, "a nil service must not become a typed-nil interface")
}

func TestMCPAutoReopener_should_ReturnTheService_When_Present(t *testing.T) {
	t.Parallel()
	svc := &backlogsvc.BacklogService{}
	assert.Same(t, svc, mcpAutoReopener(svc))
}

func TestMCPBacklogRPC_should_ReturnNilInterface_When_NoBacklogService(t *testing.T) {
	t.Parallel()
	assert.True(t, mcpBacklogRPC(nil) == nil, "a nil service must not become a typed-nil interface")
}

func TestMCPBacklogRPC_should_ReturnTheService_When_Present(t *testing.T) {
	t.Parallel()
	svc := &backlogsvc.BacklogService{}
	assert.Same(t, svc, mcpBacklogRPC(svc))
}

func TestMCPUserPRSource_should_ReturnNilInterface_When_NoCache(t *testing.T) {
	t.Parallel()
	assert.True(t, mcpUserPRSource(nil) == nil, "a nil cache must not become a typed-nil interface")
}

func TestMCPUserPRSource_should_ReturnTheCache_When_Present(t *testing.T) {
	t.Parallel()
	cache := githubpkg.NewUserPRCache()
	assert.Same(t, cache, mcpUserPRSource(cache))
}

func TestMCPCoreOptions_should_InjectUserPRCache_When_DepsCarryOne(t *testing.T) {
	t.Parallel()
	cache := githubpkg.NewUserPRCache()
	got := servermcp.UserPRSourceFromOptions(mcpCoreOptions(&ServerDependencies{UserPRCache: cache})...)
	assert.Same(t, cache, got)
}

func TestMCPCoreOptions_should_LeaveUserPRSourceNil_When_DepsHaveNoCache(t *testing.T) {
	t.Parallel()
	got := servermcp.UserPRSourceFromOptions(mcpCoreOptions(&ServerDependencies{})...)
	assert.True(t, got == nil, "no cache must leave the PR tools unregistered, not a typed-nil source")
}
