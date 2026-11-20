package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// Slice 4 readers: each resolves its default from the registry it is given, so an injected registry
// whose default differs from the process one must change the answer on an unset flag.

func TestPerFeatureReaders_ShouldUseInjectedRegistryDefault(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	flip := func(name string) *featureflags.Registry { return flippedDefaults(name) }

	cases := []struct {
		name string
		flag string
		read func(*featureflags.Registry) bool
	}{
		{"changelog", ChangelogFlagName, func(r *featureflags.Registry) bool { return ChangelogEnabled(cfg, r) }},
		{"update-check", UpdateCheckFlagName, func(r *featureflags.Registry) bool { return UpdateCheckFlagEnabled(cfg, r) }},
		{"sdd-default-pipeline", featureflags.SddDefaultPipelineFlagName, SddDefaultPipelineEnabled},
		{"github-progress-comments", githubIssueProgressCommentsFlagName, GitHubIssueProgressCommentsEnabled},
		{"program-cli-probe", programCLIFlagProbeFlagName, ProgramCLIFlagProbeEnabled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			global := c.read(nil)
			assert.Equal(t, !global, c.read(flip(c.flag)), "injected registry's default, not the process one")
		})
	}
}

func TestWorkflowEngineMode_ShouldUseInjectedRegistryDefaults(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	reg := flippedDefaults(config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag)

	assert.NotEqual(t, WorkflowEngineModeFromConfig(cfg, nil), WorkflowEngineModeFromConfig(cfg, reg))
	assert.Equal(t, WorkflowEngineModeFromConfig(cfg, reg), NewWorkflowEngineModeSource(config.LoadConfig, nil, reg).Mode(),
		"the cached source threads its registry into every resolve")
}

func TestFilesFlags_ShouldUseInjectedRegistryDefault(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	names := []string{filesEditingFlagName, filesViewerV2FlagName, filesMergePhoneFlagName,
		filesRangeReadFlagName, filesDeltaWriteFlagName, filesEditLargeFlagName}
	reg := flippedDefaults(names...)
	global, injected := globalFilesFlags{}, globalFilesFlags{reader: flagReader{reg}}

	for i, pair := range [][2]func(context.Context) bool{
		{global.Editing, injected.Editing}, {global.ViewerV2, injected.ViewerV2}, {global.MergePhone, injected.MergePhone},
		{global.RangeRead, injected.RangeRead}, {global.DeltaWrite, injected.DeltaWrite}, {global.EditLarge, injected.EditLarge},
	} {
		assert.NotEqual(t, pair[0](ctx), pair[1](ctx), names[i])
	}
}

func TestFileService_SetFlagRegistry_ShouldRouteOpsAndSearchThroughRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	svc := NewFileService(&fakeWorkspaceProvider{existingDir: t.TempDir()})
	beforeEditing, beforeViewer := svc.ops.policy.Flags.Editing(ctx), svc.search.viewerV2(ctx)

	svc.SetFlagRegistry(flippedDefaults(filesEditingFlagName, filesViewerV2FlagName))

	assert.Equal(t, !beforeEditing, svc.ops.policy.Flags.Editing(ctx))
	assert.Equal(t, !beforeViewer, svc.search.viewerV2(ctx))
}

func TestNewGatedSessionService_ShouldInjectRegistryIntoFileService(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	injected := newGatedSessionService(svctest.CreateTestStorage(t), flippedDefaults(filesEditingFlagName))
	global := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())

	assert.NotEqual(t, global.fileSvc.ops.policy.Flags.Editing(ctx), injected.fileSvc.ops.policy.Flags.Editing(ctx))
}

func TestGlobalOffStatusDetail_ShouldReadInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	gate := config.HiddenSessionGateFeatureFlag
	reg := flippedDefaults(gate)

	assert.NotEqual(t, flagReader{}.globalOffStatusDetail(cfg), flagReader{reg}.globalOffStatusDetail(cfg))

	// The gate defaults on in the process registry, so only the injected (flipped) one reports shadow.
	injected := newGatedSessionService(svctest.CreateTestStorage(t), reg)
	global := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())
	assert.NotContains(t, global.featureFlagSvc.statusDetailFor(gate, cfg), gateShadowStatusDetail)
	assert.Contains(t, injected.featureFlagSvc.statusDetailFor(gate, cfg), gateShadowStatusDetail)
}
