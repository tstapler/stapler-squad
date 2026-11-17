package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestSessionStateLabel_should_UseSnakeCaseLifecycleNames(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "active", sessionStateLabel(domain.Active))
	assert.Equal(t, "stopped", sessionStateLabel(domain.Stopped))
	assert.Equal(t, "permanently_failed", sessionStateLabel(domain.PermanentlyFailed))
}
