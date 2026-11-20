package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// Nil is a programming error at every site that takes a registry; a site that builds its own default
// must hold a real one so the first flag read cannot panic.
func TestFlagRegistrySites_ShouldPanicOnNil(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewFeatureFlagServiceWithRegistry(nil) })
	assert.Panics(t, func() { NewFileService(nil).SetFlagRegistry(nil) })
	assert.Panics(t, func() { flagReader{}.enabled(nil, "x") })
}

func TestNewFeatureFlagService_ShouldHoldDefaultRegistry(t *testing.T) {
	t.Parallel()
	require.NotNil(t, NewFeatureFlagService().reg())
}

func TestNewSessionService_ShouldHoldDefaultFlagRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewSessionService(svctest.CreateTestStorage(t), nil)
	require.NotNil(t, svc.InjectedFlagRegistry(), "a nil registry would panic every flagReader built from the session service")
}

func TestNewFileService_ShouldReadFlagsFromDefaultRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFileService(&fakeWorkspaceProvider{existingDir: t.TempDir()})
	ctx := t.Context()
	want := testFlagRegistry().Default(filesViewerV2FlagName)
	assert.NotPanics(t, func() {
		assert.Equal(t, want, svc.search.viewerV2(ctx))
		assert.Equal(t, testFlagRegistry().Default(filesEditingFlagName), svc.ops.policy.Flags.Editing(ctx))
	})
}
