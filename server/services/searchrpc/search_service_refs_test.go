package searchrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend (gitprovider), so none of them is parallel.

func TestSearchService_CachedBranch_RoutesCurrentBranchThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "feat/x", nil }

	got := NewSearchService(nil, nil, 0).cachedBranch(context.Background(), "/proj")

	assert.Equal(t, "feat/x", got)
	rig.RequireOnlyGoGit(t, backend.OpCurrentBranch)
	assert.Equal(t, backend.Local{Root: "/proj"}, rig.GoGit.Calls()[0].Loc)
}

func TestSearchService_CachedBranch_DetachedUnbornAndErrorsShowNoBranch(t *testing.T) {
	for name, err := range map[string]error{
		"detached": backend.ErrDetachedHead,
		"unborn":   backend.ErrUnborn,
		"failure":  errors.New("not a repo"),
	} {
		t.Run(name, func(t *testing.T) {
			fake := gitfake.New("backend")
			gitfake.Use(t, fake)
			fake.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "", err }

			assert.Empty(t, NewSearchService(nil, nil, 0).cachedBranch(context.Background(), "/proj"))
		})
	}
}
