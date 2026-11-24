package tagpoller

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
)

// This package cannot import session, whose init() fills the Instance process-manager hook.
func TestMain(m *testing.M) {
	instance.InstallHermeticHooksForTest()
	os.Exit(m.Run())
}
