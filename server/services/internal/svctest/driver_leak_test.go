package svctest

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/driver"
	"github.com/tstapler/stapler-squad/session/instance"
)

// The leak gate matches goroutine stacks by the driver's package-qualified frame name; a rename or
// move that stops matching would let AwaitNoSessionDrivers report 0 forever.
func TestLiveSessionDriverStacks_should_SeeARunningDriver_When_OneIsStarted(t *testing.T) {
	inst := &instance.Instance{Title: "leak-probe", Status: domain.Active}
	driver.StartSessionDriverForTest(t, inst, t.TempDir())

	assert.NotEmpty(t, LiveSessionDriverStacks())
}
