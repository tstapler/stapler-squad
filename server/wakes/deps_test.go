package wakes

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// The ports exist so server/services can depend on server/wakes (or a func
// type of its own) without a cycle; the reverse import must never appear.
func TestPackageDirection_should_NotImportServicesOrMCP(t *testing.T) {
	t.Parallel()
	out, err := safeexec.CommandContext(context.Background(), "go", "list", "-deps", "github.com/tstapler/stapler-squad/server/wakes").Output()
	require.NoError(t, err)
	deps := strings.Fields(string(out))
	for _, forbidden := range []string{
		"github.com/tstapler/stapler-squad/server/services",
		"github.com/tstapler/stapler-squad/server/mcp",
	} {
		require.NotContains(t, deps, forbidden)
	}
	require.Contains(t, deps, "github.com/tstapler/stapler-squad/session", "sanity: the list includes real dependencies")
}
