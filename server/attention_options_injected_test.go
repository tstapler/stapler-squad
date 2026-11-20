package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

func TestNewAttentionOptionsSource_ShouldResolveThroughInjectedRegistry(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir()) // empty config: registry defaults decide
	specs := services.FlagRegistry().Specs()
	for i := range specs {
		if specs[i].Name == "attention_ack_bypass_fix" {
			specs[i].Default = true
		}
	}
	_, r := newAttentionOptionsSource(featureflags.NewRegistry(specs))()
	assert.True(t, r.AckBypassFix, "injected registry default applies")
	_, r = newAttentionOptionsSource(services.FlagRegistry())()
	assert.False(t, r.AckBypassFix)
}
