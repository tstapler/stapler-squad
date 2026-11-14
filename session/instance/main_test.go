package instance

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/internal/dirwatch"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// TestMain installs the no-op port hooks the staying `session` package would
// otherwise register in init(); it cannot live earlier because it names instance symbols.
func TestMain(m *testing.M) {
	dirwatch.ForceFSNotify(true) // history/worktree tests need synchronous in-kernel event delivery
	InstallHermeticHooksForTest()
	os.Exit(sessiontest.Main(m))
}
