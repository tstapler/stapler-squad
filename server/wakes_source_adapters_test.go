package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
)

func TestSessionStateLabel_should_UseSnakeCaseLifecycleNames(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "active", sessionStateLabel(session.Active))
	assert.Equal(t, "stopped", sessionStateLabel(session.Stopped))
	assert.Equal(t, "permanently_failed", sessionStateLabel(session.PermanentlyFailed))
}
