package driver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/instance"
)

// Instance code reaches the driver only through this hook; a binary that links session but not
// this package has none and panics on the first driver start.
func TestInit_should_RegisterDriverControl_When_PackageIsLinked(t *testing.T) {
	t.Parallel()
	assert.IsType(t, sessionDriverControl{}, instance.DriverControlHook)
}
