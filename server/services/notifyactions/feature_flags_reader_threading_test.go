package notifyactions

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// flippedDefaults is testFlagRegistry with the named flags' defaults inverted.
func flippedDefaults(names ...string) *featureflags.Registry {
	flip := map[string]bool{}
	for _, n := range names {
		flip[n] = true
	}
	specs := testFlagRegistry().Specs()
	for i := range specs {
		if flip[specs[i].Name] {
			specs[i].Default = !specs[i].Default
		}
	}
	return featureflags.NewRegistry(specs)
}

// Constructor threading: an injected registry whose default differs from the
// process one must change the answer.
func TestNewNotificationActionDispatcher_ShouldUseInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, NewNotificationActionDispatcher(ActionDelegates{}, nil, flippedDefaults(flagsvc.NotificationInlineActionsFlagName, flagsvc.AskUserQuestionStructuredFlagName)).flags.Inline(flagsvc.ScopePrefixAction+"APPROVE"))
	assert.False(t, NewNotificationActionDispatcher(ActionDelegates{}, nil, testFlagRegistry()).flags.Inline(flagsvc.ScopePrefixAction+"APPROVE"))
}
