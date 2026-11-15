package historyadapters

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

func TestMain(m *testing.M) {
	instance.InstallHermeticHooksForTest()
	os.Exit(sessiontest.Main(m))
}
