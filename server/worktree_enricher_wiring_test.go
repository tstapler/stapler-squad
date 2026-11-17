package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/instance"
)

// BuildRuntimeDeps calls installWorktreeEnricher before LoadInstances; it must
// register the enricher that instance loading consults and the shutdown hook must stop it.
func TestInstallWorktreeEnricher_RegistersAndStops(t *testing.T) {
	t.Cleanup(func() { instance.SetWorktreeEnricher(nil) })
	e := installWorktreeEnricher(context.Background(), instance.WorktreeEnricherConfig{})
	require.Same(t, e, instance.ActiveWorktreeEnricher())
	instance.StopWorktreeEnricher()
	require.Nil(t, instance.ActiveWorktreeEnricher())
}
