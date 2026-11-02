package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tcc"
)

func TestConsentEnrichmentProbe_ResumesOriginalKeyForEachRoot(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	work, gitdir, common := filepath.Join(root, "work"), filepath.Join(root, "gitdir"), filepath.Join(root, "common")
	for _, p := range []string{work, gitdir, common} {
		require.NoError(t, os.Mkdir(p, 0700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: ../gitdir\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../common\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(common, "config"), []byte("[remote \"origin\"]\n url = https://github.com/example/guarded.git\n"), 0600))
	gate := tcc.NewConsentGate(func(path string) tcc.Result {
		for _, p := range []string{gitdir, common} {
			if path == p {
				return tcc.Result{Protected: true, Root: p}
			}
		}
		return tcc.Result{}
	}, nil, nil)
	probe := ConsentEnrichmentProbe(gate)
	_, err = probe(work)
	require.True(t, errors.Is(err, errEnrichmentParked))
	require.Len(t, gate.Roots(), 1)
	paths, err := gate.Continue(gate.Roots()[0].ID)
	require.NoError(t, err)
	require.Equal(t, []string{work}, paths)
	_, err = probe(work)
	require.ErrorIs(t, err, errEnrichmentParked)
	require.Len(t, gate.Roots(), 2)
	for _, r := range gate.Roots() {
		if !r.Consented {
			paths, err = gate.Continue(r.ID)
			require.NoError(t, err)
			require.Equal(t, []string{work}, paths)
		}
	}
	info, err := probe(work)
	require.NoError(t, err)
	require.Equal(t, "guarded", info.GitHubRepo)
}
