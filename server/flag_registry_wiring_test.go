package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

// flipFlagDefault returns a copy of the process registry with name's default inverted.
func flipFlagDefault(name string) *featureflags.Registry {
	specs := flagsvc.FlagRegistry().Specs()
	for i := range specs {
		if specs[i].Name == name {
			specs[i].Default = !specs[i].Default
		}
	}
	return featureflags.NewRegistry(specs)
}

func TestBuildServiceDeps_ShouldFeedCoreFlagRegistryToAttentionSource(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDeps()
	require.NoError(t, err)
	core.FlagRegistry = flipFlagDefault("attention_ack_bypass_fix")

	svc, err := BuildServiceDeps(core)
	require.NoError(t, err)
	_, rules := svc.AttentionOptions()
	assert.True(t, rules.AckBypassFix, "BuildServiceDeps resolves attention flags through core.FlagRegistry")
}

func TestWireGuidanceMirror_ShouldGateOnServiceRegistry(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDeps()
	require.NoError(t, err)
	svc, err := services.NewSessionServiceFromConfig(flipFlagDefault("askuserquestion_structured"))
	require.NoError(t, err)
	deps := &ServerDependencies{SessionService: svc, Storage: core.Storage, EventBus: core.EventBus}

	wireGuidanceMirror(&services.ApprovalHandler{}, deps, context.Background(), func(func()) {})

	mirror := svc.GuidanceMirror()
	require.NotNil(t, mirror)
	assert.True(t, mirror.Enabled(), "mirror gate reads the service's injected registry default")
}
