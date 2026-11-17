package wsconn

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// TestMain installs the process flag registry that the composition root installs in production, so
// services.FeatureFlagDefault (read by currentResyncOptions) resolves instead of panicking.
func TestMain(m *testing.M) {
	featureflags.SetCurrent(services.FlagRegistry())
	os.Exit(m.Run())
}
