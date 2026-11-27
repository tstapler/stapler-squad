package rules

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain gives this binary the hermetic setup server/services' own tests run under.
func TestMain(m *testing.M) {
	os.Exit(svctest.Main(m, func() {}))
}
