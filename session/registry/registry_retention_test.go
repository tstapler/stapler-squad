package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestRegistryRetain_PreservesExistingActorAndIdentity(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(nil, nil)
	defer registry.Shutdown()
	canonical := &instance.Instance{UUID: "canonical", Title: "legacy-title"}
	actor := instance.NewLiveInstance(canonical)
	release, err := registry.Register(actor)
	require.NoError(t, err)

	replacement := &instance.Instance{UUID: canonical.UUID, Title: canonical.Title}
	require.Same(t, actor, registry.Retain(replacement))
	require.Same(t, actor, registry.Retain(canonical))
	release()
	require.Same(t, actor, registry.Lookup(canonical.UUID))
	require.Same(t, actor, registry.Lookup(canonical.Title))
	require.Equal(t, 0, registry.entries[canonical.UUID].refcount, "retention must not add an Acquire reference")

	registry.ForceRelease(canonical.UUID)
	require.Nil(t, registry.Lookup(canonical.UUID))
	require.Error(t, actor.ContextErrForTest())
}

func TestRegistryRetain_AdoptsUnregisteredActor(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(nil, nil)
	defer registry.Shutdown()
	inst := &instance.Instance{UUID: "adopted"}
	actor := instance.NewLiveInstance(inst)
	require.Same(t, actor, registry.Retain(inst))
	require.Same(t, actor, registry.Retain(inst))
	require.Equal(t, 1, registry.Count())
	registry.Shutdown()
	require.Nil(t, registry.Lookup(inst.UUID))
	require.Error(t, actor.ContextErrForTest())
}

func TestRegistryRelease_KeepsEntryWhileAnotherHolderRemains(t *testing.T) {
	t.Parallel()
	reg := NewRegistry(nil, nil)
	defer reg.Shutdown()
	actor := instance.NewLiveInstance(&instance.Instance{UUID: "shared"})
	releaseFirst, err := reg.Register(actor)
	require.NoError(t, err)
	got, releaseSecond, err := reg.Acquire("shared")
	require.NoError(t, err)
	require.Same(t, actor, got)

	releaseFirst()
	require.Same(t, actor, reg.Lookup("shared"), "one holder remains, entry must survive")
	require.NoError(t, actor.ContextErrForTest())

	releaseSecond()
	require.Nil(t, reg.Lookup("shared"))
	require.Error(t, actor.ContextErrForTest())
}

func TestRegistryRelease_RemovesUnretainedEntryAtZero(t *testing.T) {
	t.Parallel()
	reg := NewRegistry(nil, nil)
	defer reg.Shutdown()
	first := instance.NewLiveInstance(&instance.Instance{UUID: "gone"})
	release, err := reg.Register(first)
	require.NoError(t, err)

	release()
	require.Equal(t, 0, reg.Count())
	replacement := instance.NewLiveInstance(&instance.Instance{UUID: "gone"})
	_, err = reg.Register(replacement)
	require.NoError(t, err, "the released ID must be free to register again")
	require.Same(t, replacement, reg.Lookup("gone"))
}
