package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Instance code reaches the session driver only through instance.DriverControlHook, which has no
// init() fallback: if the composition root stops registering it, the first resume of a crashed or
// hibernated session panics in production while every test mains that register it stay green.
func TestBuildCoreDepsWithOptions_ShouldRegisterDriverControl_WhenBuilt(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	_, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)

	assert.NotNil(t, instance.DriverControlHook)
}
