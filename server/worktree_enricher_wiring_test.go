package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// BuildRuntimeDeps calls installWorktreeEnricher before LoadInstances; it must
// register the enricher that instance loading consults and the shutdown hook must stop it.
func TestInstallWorktreeEnricher_RegistersAndStops(t *testing.T) {
	t.Cleanup(func() { session.SetWorktreeEnricher(nil) })
	e := installWorktreeEnricher(context.Background(), session.WorktreeEnricherConfig{})
	require.Same(t, e, session.ActiveWorktreeEnricher())
	session.StopWorktreeEnricher()
	require.Nil(t, session.ActiveWorktreeEnricher())
}
