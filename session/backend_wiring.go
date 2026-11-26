package session

import (
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/procbackend"
)

// Registers the process-backend factory. procbackend has no init() so a test
// binary can install its own factory; removing this makes Instance.pm() panic
// in every binary that links session.
func init() { instance.ProcessManagerFactoryHook = procbackend.NewProcessManager }
