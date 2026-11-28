package fileops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/fsops"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func TestFileService_SetFlagRegistry_ShouldRouteOpsAndSearchThroughRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	svc := NewFileService(&fakeWorkspaceProvider{existingDir: t.TempDir()})
	beforeEditing, beforeViewer := svc.ops.policy.Flags.Editing(ctx), svc.search.ViewerV2Enabled(ctx)

	svc.SetFlagRegistry(flippedDefaults(flagsvc.FilesEditingFlagName, flagsvc.FilesViewerV2FlagName))

	assert.Equal(t, !beforeEditing, svc.ops.policy.Flags.Editing(ctx))
	assert.Equal(t, !beforeViewer, svc.search.ViewerV2Enabled(ctx))
}

func TestFileService_SetFlagRegistry_ShouldPanicOnNil(t *testing.T) {
	t.Parallel()
	assert.PanicsWithValue(t, "fileops: FileService.SetFlagRegistry requires a non-nil registry",
		func() { NewFileService(nil).SetFlagRegistry(nil) })
}

func TestNewFileService_ShouldReadFlagsFromDefaultRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFileService(&fakeWorkspaceProvider{existingDir: t.TempDir()})
	ctx := t.Context()
	want := testFlagRegistry().Default(flagsvc.FilesViewerV2FlagName)
	assert.NotPanics(t, func() {
		assert.Equal(t, want, svc.search.ViewerV2Enabled(ctx))
		assert.Equal(t, testFlagRegistry().Default(flagsvc.FilesEditingFlagName), svc.ops.policy.Flags.Editing(ctx))
	})
}

func TestNewFileOps_ShouldReadFlagsFromDefaultRegistry_WhenPolicyHasNone(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ops := NewFileOps(FilePolicy{}, fsops.OsRootBackend{})
	ctx := t.Context()
	assert.NotPanics(t, func() {
		assert.Equal(t, testFlagRegistry().Default(flagsvc.FilesEditingFlagName), ops.policy.Flags.Editing(ctx))
	})
}
