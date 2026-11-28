package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	githubpkg "github.com/tstapler/stapler-squad/github"
)

// Each assertion pins one wiring line of buildGitHubUserService; a dropped
// recorder or token resolver changes no response, so only this notices.
func TestBuildDependencies_should_InstallEveryNudgeCollaborator_When_DepsBuilt(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	deps, err := BuildDependencies()
	require.NoError(t, err)
	newShutdownOnCleanupServer(t, "localhost:0", deps)

	nudger, recorder, fetcher, tokens := deps.GitHubUserService.NudgeWiringForTest()

	assert.Same(t, deps.SessionService, nudger)
	assert.Same(t, deps.Storage, recorder)
	assert.Equal(t, githubpkg.GraphQLPRDetailFetcher{}, fetcher)
	assert.Same(t, deps.UserPRCache, tokens)
}
