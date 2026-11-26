package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

func TestNewAttentionOptionsSource_ShouldResolveThroughInjectedRegistry(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir()) // empty config: registry defaults decide
	specs := flagsvc.FlagRegistry().Specs()
	for i := range specs {
		if specs[i].Name == "attention_ack_bypass_fix" {
			specs[i].Default = true
		}
	}
	_, r := newAttentionOptionsSource(featureflags.NewRegistry(specs))()
	assert.True(t, r.AckBypassFix, "injected registry default applies")
	_, r = newAttentionOptionsSource(flagsvc.FlagRegistry())()
	assert.False(t, r.AckBypassFix)
}
