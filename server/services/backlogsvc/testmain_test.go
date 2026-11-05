package backlogsvc

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain gives this binary the same hermetic setup as server/services' own: no non-loopback network, an isolated
// Claude CLI and state dir, a pre-seeded headless capability self-check (see svctest.Main), and a SessionDriver
// leak check after the run. Nothing here depends on the services package's DNS seam.
func TestMain(m *testing.M) {
	os.Exit(svctest.Main(m, func() {}))
}
