package importer

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

func TestMain(m *testing.M) {
	os.Exit(sessiontest.Main(m))
}
