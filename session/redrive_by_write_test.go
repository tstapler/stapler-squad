package session

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

// The strict bypass inventory (server/services, Story 1.5.9a) accepts the
// re-drive writer's computed Caller because it can only be one of these.
func TestRedriveByWrite_OnlyNamesRegisteredSystemCallers(t *testing.T) {
	require.NotEmpty(t, redriveByWrite)
	for caller := range redriveByWrite {
		spec, ok := domain.LookupTransitionCaller(caller)
		require.True(t, ok, "%s is not registered", caller)
		require.Equal(t, domain.ClassSystem, spec.Class, caller)
	}
}

