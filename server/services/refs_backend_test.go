package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitwiring"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend (gitprovider), so none of them is parallel.

func TestCheckBranchDivergence_RoutesCountCommitsThroughBackend(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		err   error
		want  bool
	}{
		{"ahead", 3, nil, true},
		{"level", 0, nil, false},
		{"no origin/HEAD", 0, backend.ErrRefNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := gitfake.NewRig(t)
			rig.Install(t)
			rig.GoGit.OnCountCommits = func(_ backend.RepoLocation, rng backend.RangeSpec) (int, error) {
				assert.Equal(t, backend.RangeSpec{Exclude: "origin/HEAD", Include: "HEAD"}, rng)
				return tc.count, tc.err
			}

			assert.Equal(t, tc.want, checkBranchDivergence("/work"))
			rig.RequireOnlyGoGit(t, backend.OpCountCommits)
		})
	}
}

// The session service must share the process-wide router, not build a second one whose
// counters and shadow state would drift from the one gitprovider hands session/git.
func TestSessionService_GitBackendDefaultsToTheSharedRouter(t *testing.T) {
	assert.Same(t, gitwiring.Shared(), (&SessionService{}).gitBackendOrDefault())
}
