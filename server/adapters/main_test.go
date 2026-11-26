package adapters

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
)

// This package no longer imports session, whose init() fills the Instance process-manager hook.
func TestMain(m *testing.M) {
	instance.InstallHermeticHooksForTest()
	os.Exit(m.Run())
}
