package debuglogs

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain gives this binary a temp HOME, so logs and snapshots stay out of the
// developer's, and server/services' driver-leak check.
func TestMain(m *testing.M) {
	cleanup, err := svctest.UseTempHome()
	if err != nil {
		panic(err)
	}
	code := svctest.Main(m, func() {})
	cleanup()
	os.Exit(code)
}
