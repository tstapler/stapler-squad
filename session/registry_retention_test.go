package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryRetain_PreservesExistingActorAndIdentity(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(nil, nil)
	defer registry.Shutdown()
	canonical := &Instance{UUID: "canonical", Title: "legacy-title"}
	actor := NewLiveInstance(canonical)
	release, err := registry.Register(actor)
	require.NoError(t, err)

	replacement := &Instance{UUID: canonical.UUID, Title: canonical.Title}
	require.Same(t, actor, registry.Retain(replacement))
	require.Same(t, actor, registry.Retain(canonical))
	release()
	require.Same(t, actor, registry.Lookup(canonical.UUID))
	require.Same(t, actor, registry.Lookup(canonical.Title))
	require.Equal(t, 0, registry.entries[canonical.UUID].refcount, "retention must not add an Acquire reference")

	registry.ForceRelease(canonical.UUID)
	require.Nil(t, registry.Lookup(canonical.UUID))
	require.Error(t, actor.ctx.Err())
}

func TestRegistryRetain_AdoptsUnregisteredActor(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(nil, nil)
	defer registry.Shutdown()
	inst := &Instance{UUID: "adopted"}
	actor := NewLiveInstance(inst)
	require.Same(t, actor, registry.Retain(inst))
	require.Same(t, actor, registry.Retain(inst))
	require.Equal(t, 1, registry.Count())
	registry.Shutdown()
	require.Nil(t, registry.Lookup(inst.UUID))
	require.Error(t, actor.ctx.Err())
}
