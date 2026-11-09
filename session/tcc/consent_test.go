package tcc

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
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

func epermGate(t *testing.T, persist func(string, bool) error) (*ConsentGate, string) {
	t.Helper()
	g := NewConsentGate(func(string) Result { return Result{Protected: true, Root: "/p", Service: FilesAndFolders} }, nil, nil)
	g.SetDeniedStore(nil, persist)
	require.False(t, g.Ready("/p/repo"))
	return g, g.Roots()[0].ID
}

func TestConsentGate_ObserveResultPersistsEPERMOnly(t *testing.T) {
	t.Parallel()
	type op struct {
		id     string
		denied bool
	}
	ops := make(chan op, 4)
	g, id := epermGate(t, func(id string, d bool) error { ops <- op{id, d}; return nil })
	g.ObserveResult("/p/repo", fmt.Errorf("open: %w", syscall.EACCES))
	require.False(t, g.Roots()[0].Denied)
	g.ObserveResult("/p/repo", fmt.Errorf("open: %w", syscall.EPERM))
	require.True(t, g.Roots()[0].Denied)
	require.Equal(t, op{id, true}, <-ops)
}

func TestConsentGate_PersistOrderFollowsStateChanges(t *testing.T) {
	t.Parallel()
	first := make(chan struct{})
	var mu sync.Mutex
	var got []bool
	done := make(chan struct{})
	g, id := epermGate(t, func(_ string, d bool) error {
		mu.Lock()
		n := len(got)
		got = append(got, d)
		mu.Unlock()
		if n == 0 {
			<-first // hold persist(true) while Recheck queues persist(false)
		} else {
			close(done)
		}
		return nil
	})
	g.ObserveResult("/p/repo", syscall.EPERM)
	paths, err := g.Recheck(id)
	require.NoError(t, err)
	require.Equal(t, []string{"/p/repo"}, paths)
	close(first)
	<-done
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []bool{true, false}, got)
}

func TestConsentGate_SetDeniedStoreRestoresDenial(t *testing.T) {
	t.Parallel()
	g := NewConsentGate(func(string) Result { return Result{Protected: true, Root: "/p", Service: FilesAndFolders} }, nil, nil)
	probe := NewConsentGate(func(string) Result { return Result{Protected: true, Root: "/p"} }, nil, nil)
	probe.Ready("/p/repo")
	id := probe.Roots()[0].ID
	g.SetDeniedStore(map[string]bool{id: true}, nil)
	g.Ready("/p/repo")
	require.True(t, g.Roots()[0].Denied)
}
