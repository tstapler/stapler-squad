package mcp

import (
	"context"
	"sync"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// fakeUserPRSource returns canned PRs and logins and counts each call.
type fakeUserPRSource struct {
	mu       sync.Mutex
	prs      []githubpkg.UserPR
	logins   []string
	getAlls  int
	getLogin int
}

func (f *fakeUserPRSource) GetAll() []githubpkg.UserPR {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getAlls++
	return f.prs
}

func (f *fakeUserPRSource) GetCachedLogins() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getLogin++
	return f.logins
}

func toolNamesOf(s *mcpserver.MCPServer) map[string]bool {
	out := map[string]bool{}
	for name := range s.ListTools() {
		out[name] = true
	}
	return out
}

func TestWithUserPRCache_should_RegisterGitHubTools_When_SourcePresent(t *testing.T) {
	t.Parallel()
	with := toolNamesOf(NewCore(&stubStore{}, nil, nil, nil, nil, nil, nil, nil, nil, WithUserPRCache(&fakeUserPRSource{})))
	without := toolNamesOf(NewCore(&stubStore{}, nil, nil, nil, nil, nil, nil, nil, nil))
	assert.True(t, with["list_github_prs"])
	assert.False(t, without["list_github_prs"], "no source must leave the GitHub PR tools unregistered")
}

func TestWithUserPRCache_should_NotRegisterGitHubTools_When_UntypedNil(t *testing.T) {
	t.Parallel()
	names := toolNamesOf(NewCore(&stubStore{}, nil, nil, nil, nil, nil, nil, nil, nil, WithUserPRCache(nil)))
	assert.False(t, names["list_github_prs"])
}

func TestListGitHubPRs_should_ReturnPRsAndLoginsFromThePort(t *testing.T) {
	t.Parallel()
	fake := &fakeUserPRSource{
		prs: []githubpkg.UserPR{
			{Owner: "o", Repo: "r", Number: 7, Title: "T", HeadRef: "feat", SessionIDs: []string{"sess-a"}},
			{Owner: "o", Repo: "r2", Number: 8},
		},
		logins: []string{"alice", "bob"},
	}
	h := &githubHandlers{cache: fake, store: &stubStore{}}

	res, err := h.listGitHubPRs(context.Background(), makeToolReq(nil))
	require.NoError(t, err)
	m := parseResult(t, res)

	assert.EqualValues(t, 2, m["total_count"])
	assert.Equal(t, []interface{}{"alice", "bob"}, m["accounts"])
	prs := m["prs"].([]interface{})
	require.Len(t, prs, 2)
	first := prs[0].(map[string]interface{})
	assert.EqualValues(t, 7, first["number"])
	assert.Equal(t, "feat", first["branch"])
	assert.Equal(t, "sess-a", first["existing_session_id"])
	assert.Equal(t, 1, fake.getAlls)
	assert.Equal(t, 1, fake.getLogin)
}

func TestCreateSessionForPR_should_ShortCircuitOnPortSnapshot(t *testing.T) {
	t.Parallel()
	fake := &fakeUserPRSource{prs: []githubpkg.UserPR{
		{Owner: "o", Repo: "r", Number: 42, SessionIDs: []string{"existing"}},
	}}
	store := &stubStore{instances: []*instance.Instance{
		{Title: "existing", Path: t.TempDir(), Status: domain.Active, Program: "claude"},
	}}
	h := &githubHandlers{cache: fake, store: store}

	res, err := h.createSessionForPR(context.Background(), makeToolReq(map[string]interface{}{
		"owner": "o", "repo": "r", "branch": "b", "pr_number": float64(42),
	}))
	require.NoError(t, err)
	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected the short-circuit result, got: %+v", m)
	assert.Equal(t, "existing", m["session"].(map[string]interface{})["id"])
	assert.Equal(t, 1, fake.getAlls, "the existing-session check must read the port")
}

func TestUserPRSourceFromOptions_should_ReturnInjectedSource(t *testing.T) {
	t.Parallel()
	fake := &fakeUserPRSource{}
	assert.Same(t, fake, UserPRSourceFromOptions(WithUserPRCache(fake)))
	assert.True(t, UserPRSourceFromOptions() == nil)
}
