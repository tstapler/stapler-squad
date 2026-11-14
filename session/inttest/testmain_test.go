package inttest

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// TestMain installs the flag registry the composition root installs in production, so code under test that
// reads services.FeatureFlagDefault (for example the backlog default pipeline) resolves instead of panicking.
func TestMain(m *testing.M) {
	featureflags.SetCurrent(services.FlagRegistry())
	os.Exit(m.Run())
}
