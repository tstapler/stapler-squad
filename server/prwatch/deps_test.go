package prwatch

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

const modulePath = "github.com/tstapler/stapler-squad"

// goList runs `go list` and returns whitespace-separated fields of its output.
func goList(t *testing.T, args ...string) []string {
	t.Helper()
	out, err := safeexec.CommandContext(context.Background(), "go", append([]string{"list"}, args...)...).Output()
	require.NoError(t, err)
	return strings.Fields(string(out))
}

// Direct imports only: `go list -deps ./session` includes `github`, so a
// transitive check could never pass. Adapters in server/ own the bridging.
func TestPackageDirection_should_NotImportServicesOrMCP(t *testing.T) {
	t.Parallel()
	direct := goList(t, "-f", `{{range .Imports}}{{.}}{{"\n"}}{{end}}`, modulePath+"/server/prwatch/...")

	for _, forbidden := range []string{
		modulePath + "/server/services",
		modulePath + "/server/mcp",
		modulePath + "/server/wakes",
		modulePath + "/github",
		modulePath + "/config",
		modulePath + "/session/git",
	} {
		require.NotContains(t, direct, forbidden)
	}
	require.Contains(t, direct, modulePath+"/server/prwatch/inbox", "sanity: the listing includes real direct imports")
}

// Adapters live in server/, not server/services, so services never needs prwatch.
// -e: tolerate unresolved generated packages (gen/, ent) so the listing still works.
func TestPackageDirection_should_NotBeImportedByServices_When_ListingReverseDeps(t *testing.T) {
	t.Parallel()
	deps := goList(t, "-e", "-deps", modulePath+"/server/services")
	require.NotContains(t, deps, modulePath+"/server/prwatch")
	require.NotContains(t, deps, modulePath+"/server/prwatch/inbox")
	require.Contains(t, deps, modulePath+"/session", "sanity: the list includes real dependencies")
}
