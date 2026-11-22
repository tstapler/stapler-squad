package driver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/instance"
)

func TestRegister_should_InstallProductionControl_When_Called(t *testing.T) {
	Register()
	Register() // idempotent
	assert.IsType(t, sessionDriverControl{}, instance.DriverControlHook)
}
