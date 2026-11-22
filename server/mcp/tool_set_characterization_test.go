package mcp

import (
	"sort"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
)

// TestNewCore_should_RegisterTheSameToolSetPerWiringShape pins the registered
// tool-name set for each nil-gating shape of NewCore, so a ToolModule refactor
// cannot silently add or drop a tool.
func TestNewCore_should_RegisterTheSameToolSetPerWiringShape(t *testing.T) {
	storage := newTestBacklogStorage(t)
	store := &stubStore{}
	svc := services.NewSessionService(storage, nil)
	off := func() bool { return false }

	got := map[string][]string{
		"bare":                     toolNames(NewCore(store, nil, nil, nil, nil, nil, nil, nil, nil)),
		"storage":                  toolNames(NewCore(store, nil, nil, storage, nil, nil, nil, nil, nil)),
		"storage_backlog_disabled": toolNames(NewCore(store, nil, nil, storage, nil, off, nil, nil, nil)),
		"svc_only":                 toolNames(NewCore(store, realSession{svc}, nil, nil, nil, nil, nil, nil, nil)),
		"pr_only":                  toolNames(NewCore(store, nil, nil, nil, nil, nil, nil, nil, nil, WithUserPRCache(githubpkg.NewUserPRCache()))),
		"svc_storage_pr_backlogsvc": toolNames(NewCore(store, realSession{svc}, nil, storage, nil,
			nil, nil, &backlogsvc.BacklogService{}, nil, WithUserPRCache(githubpkg.NewUserPRCache()))),
	}

	require.Equal(t, wantToolSets, got)
}

func TestDefaultToolModules_should_SkipGatedModulesWithoutTypedNils(t *testing.T) {
	t.Parallel()
	registerOn := mcpserver.NewMCPServer("t", "0")
	var live int
	for _, m := range defaultToolModules(coreDeps{store: &stubStore{}}) {
		if m != nil {
			live++
			m.Register(registerOn)
		}
	}
	// discovery, lifecycle, terminal, vcs: every svc/storage/prCache-gated module is nil.
	require.Equal(t, 4, live)
}

func TestWithToolModules_should_RegisterExtraModulesAfterTheBuiltIns(t *testing.T) {
	t.Parallel()
	var order []string
	extra := toolModuleFunc(func(*mcpserver.MCPServer) { order = append(order, "extra") })
	mods := defaultToolModules(coreDeps{store: &stubStore{}, cfg: newCoreConfig([]CoreOption{WithToolModules(extra)})})
	require.Len(t, mods, 11)
	mods[len(mods)-1].Register(nil)
	require.Equal(t, []string{"extra"}, order)
}

func toolNames(s *mcpserver.MCPServer) []string {
	names := make([]string, 0, len(s.ListTools()))
	for n := range s.ListTools() {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
