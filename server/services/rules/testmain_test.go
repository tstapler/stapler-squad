package rules

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain gives this binary the hermetic setup server/services' own tests run under.
// The temp HOME keeps rule upserts and deletes from exporting into the developer's.
func TestMain(m *testing.M) {
	cleanup, err := svctest.UseTempHome()
	if err != nil {
		panic(err)
	}
	code := svctest.Main(m, func() {})
	cleanup()
	os.Exit(code)
}
