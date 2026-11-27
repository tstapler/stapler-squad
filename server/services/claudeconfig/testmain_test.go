package claudeconfig

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain gives this binary the hermetic setup server/services' own tests run under,
// with a temp HOME so no test reads or writes the developer's.
func TestMain(m *testing.M) {
	cleanup, err := svctest.UseTempHome()
	if err != nil {
		panic(err)
	}
	code := svctest.Main(m, func() {})
	cleanup()
	os.Exit(code)
}
