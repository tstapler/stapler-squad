package backlogsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
	assert.Empty(t, (&BacklogService{}).defaultPipelineModeForNewItem(nil), "process registry default off")
}
