package tcc

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsentGate_ParksUntilPersistedConsent(t *testing.T) {
	t.Parallel()
	var saved string
	g := NewConsentGate(func(string) Result { return Result{Protected: true, Root: "/protected", Service: FilesAndFolders} }, nil, func(id string) error { saved = id; return nil })
	require.False(t, g.Ready("/protected/repo"))
	roots := g.Roots()
	require.Len(t, roots, 1)
	require.NotEqual(t, "/protected", roots[0].ID)
	require.Equal(t, []string{"/protected/repo"}, roots[0].Paths)
	paths, err := g.Continue(roots[0].ID)
	require.NoError(t, err)
	require.Equal(t, roots[0].ID, saved)
	require.Equal(t, roots[0].Paths, paths)
	require.True(t, g.Ready("/protected/repo"))
	require.True(t, g.Roots()[0].Consented)
	restored := NewConsentGate(func(string) Result { return Result{Protected: true, Root: "/protected"} }, map[string]bool{saved: true}, nil)
	require.True(t, restored.Ready("/protected/another"))
}

func TestConsentGate_RejectsUnknownAndFailedSave(t *testing.T) {
	t.Parallel()
	g := NewConsentGate(func(string) Result { return Result{Protected: true, Root: "/protected"} }, nil, func(string) error { return errors.New("disk full") })
	_, err := g.Continue("/arbitrary/path")
	require.ErrorIs(t, err, ErrUnknownRoot)
	require.Empty(t, g.Roots())
	require.False(t, g.Ready("/protected/repo"))
	_, err = g.Continue(g.Roots()[0].ID)
	require.EqualError(t, err, "disk full")
	require.False(t, g.Ready("/protected/repo"))
}

func TestConsentGate_UnprotectedAndConcurrent(t *testing.T) {
	t.Parallel()
	g := NewConsentGate(func(p string) Result {
		if p == "/safe" {
			return Result{}
		}
		return Result{Protected: true, Root: "/protected"}
	}, nil, nil)
	require.True(t, g.Ready("/safe"))
	require.Empty(t, g.Roots())
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { g.Ready("/protected/repo"); g.Roots() })
	}
	wg.Wait()
	require.Len(t, g.Roots(), 1)
	require.Len(t, g.Roots()[0].Paths, 1)
}
