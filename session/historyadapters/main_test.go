package historyadapters

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

func TestMain(m *testing.M) {
	instance.InstallHermeticHooksForTest()
	// The hermetic helper installs no-op history hooks; these tests exercise the real ones.
	instance.HistoryRegistryHook = defaultHistoryRegistry
	instance.HistoryPorterHook = PortSessionHistory
	os.Exit(sessiontest.Main(m))
}
