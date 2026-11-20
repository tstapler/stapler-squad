package backlogsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
)

func TestDefaultPipelineModeForNewItem_ShouldUseInjectedRegistryDefault(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	specs := services.FlagRegistry().Specs()
	for i := range specs {
		if specs[i].Name == featureflags.SddDefaultPipelineFlagName {
			specs[i].Default = true
		}
	}
	s := &BacklogService{}
	s.SetFlagRegistry(featureflags.NewRegistry(specs))

	assert.Equal(t, session.DefaultSDDPipelineModeSlug, s.defaultPipelineModeForNewItem(nil), "injected default on")
	assert.Same(t, s.flags, s.FlagRegistry())

	def := &BacklogService{}
	def.SetFlagRegistry(services.FlagRegistry())
	assert.Empty(t, def.defaultPipelineModeForNewItem(nil), "default registry: pipeline default off")
}

func TestNewBacklogService_ShouldStartOnDefaultFlagRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	s := NewBacklogService(nil, nil, nil, nil, nil, nil)
	require.NotNil(t, s.FlagRegistry(), "construction-time default; nil would panic on the first flag read")
	assert.Empty(t, s.defaultPipelineModeForNewItem(nil))
}
