package services

import (
	"context"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/fileops"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/forwardsync"
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
		{"changelog", featureflags.ChangelogFlagName, func(r *featureflags.Registry) bool { return ChangelogEnabled(cfg, r) }},
		{"update-check", featureflags.UpdateCheckFlagName, func(r *featureflags.Registry) bool { return UpdateCheckFlagEnabled(cfg, r) }},
		{"sdd-default-pipeline", featureflags.SddDefaultPipelineFlagName, SddDefaultPipelineEnabled},
		{"github-progress-comments", featureflags.GithubIssueProgressCommentsFlagName, forwardsync.GitHubIssueProgressCommentsEnabled},
		{"program-cli-probe", flagsvc.ProgramCLIFlagProbeFlagName, flagsvc.ProgramCLIFlagProbeEnabled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			global := c.read(testFlagRegistry())
			assert.Equal(t, !global, c.read(flip(c.flag)), "injected registry's default, not the process one")
		})
	}
}

func TestWorkflowEngineMode_ShouldUseInjectedRegistryDefaults(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	reg := flippedDefaults(config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag)

	assert.NotEqual(t, flagsvc.WorkflowEngineModeFromConfig(cfg, testFlagRegistry()), flagsvc.WorkflowEngineModeFromConfig(cfg, reg))
	assert.Equal(t, flagsvc.WorkflowEngineModeFromConfig(cfg, reg), flagsvc.NewWorkflowEngineModeSource(config.LoadConfig, nil, reg).Mode(),
		"the cached source threads its registry into every resolve")
}

func TestFilesFlags_ShouldUseInjectedRegistryDefault(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	names := []string{flagsvc.FilesEditingFlagName, flagsvc.FilesViewerV2FlagName, flagsvc.FilesMergePhoneFlagName,
		flagsvc.FilesRangeReadFlagName, flagsvc.FilesDeltaWriteFlagName, flagsvc.FilesEditLargeFlagName}
	reg := flippedDefaults(names...)
	global, injected := testFilesFlags(), fileops.NewGlobalFilesFlags(reg)

	for i, pair := range [][2]func(context.Context) bool{
		{global.Editing, injected.Editing}, {global.ViewerV2, injected.ViewerV2}, {global.MergePhone, injected.MergePhone},
		{global.RangeRead, injected.RangeRead}, {global.DeltaWrite, injected.DeltaWrite}, {global.EditLarge, injected.EditLarge},
	} {
		assert.NotEqual(t, pair[0](ctx), pair[1](ctx), names[i])
	}
}

func TestNewGatedSessionService_ShouldInjectRegistryIntoFileService(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	injected := newGatedSessionService(svctest.CreateTestStorage(t), flippedDefaults(flagsvc.FilesEditingFlagName))
	global := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())

	servesWrite := func(svc *SessionService) bool {
		resp, err := svc.GetFileTransportCapabilities(ctx, connect.NewRequest(&sessionv1.GetFileTransportCapabilitiesRequest{}))
		require.NoError(t, err)
		return slices.Contains(resp.Msg.Capabilities.Ops, sessionv1.FileOp_FILE_OP_WRITE_FULL)
	}
	assert.NotEqual(t, servesWrite(global), servesWrite(injected))
}

func TestGlobalOffStatusDetail_ShouldReadInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	gate := config.HiddenSessionGateFeatureFlag
	reg := flippedDefaults(gate)

	assert.NotEqual(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.GlobalOffStatusDetail(cfg), flagsvc.FlagReader{Reg: reg}.GlobalOffStatusDetail(cfg))

	// The gate defaults on in the process registry, so only the injected (flipped) one reports shadow.
	injected := newGatedSessionService(svctest.CreateTestStorage(t), reg)
	global := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())
	assert.NotContains(t, statusDetailOf(t, global.featureFlagSvc, gate), flagsvc.GateShadowStatusDetail)
	assert.Contains(t, statusDetailOf(t, injected.featureFlagSvc, gate), flagsvc.GateShadowStatusDetail)
}
