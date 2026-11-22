package mcp

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// allowedImports is the exact non-test import set of server/mcp (stdlib excluded). It is the
// dependable control behind the `isolate_server_mcp` depguard row, whose prefix matching is
// order-dependent: it passed session/flowstore and server/services/backlogflow, and blocked
// session/scrollback and server/services/featureflags. Two-sided equality also enforces
// leaf-packages rule 4's "nothing unused", which depguard does not.
//
// To ADD an import: justify it in the PR (server/mcp must not become a second composition root,
// owner decision Q5) and add the row. To REMOVE one (an M4b slice that retires an edge): delete
// its row.
var allowedImports = map[string]string{
	"connectrpc.com/connect":                             "RPC request/response envelopes",
	"github.com/google/uuid":                             "third-party",
	"github.com/mark3labs/mcp-go/client":                 "MCP proxy",
	"github.com/mark3labs/mcp-go/client/transport":       "MCP proxy",
	"github.com/mark3labs/mcp-go/mcp":                    "MCP protocol",
	"github.com/mark3labs/mcp-go/server":                 "MCP server",
	"google.golang.org/protobuf/encoding/protojson":      "flow JSON options",
	"google.golang.org/protobuf/proto":                   "third-party",
	"google.golang.org/protobuf/reflect/protoreflect":    "third-party",
	"google.golang.org/protobuf/types/known/timestamppb": "third-party",
	modulePrefix + "config":                              "config",
	modulePrefix + "gen/proto/go/session/v1":             "wire messages",
	modulePrefix + "github":                              "M4b: value types and the GitHub port",
	modulePrefix + "log":                                 "logging",
	modulePrefix + "pkg/events":                          "event bus (outside the measured set)",
	modulePrefix + "pkg/localtoken":                      "local token",
	modulePrefix + "server/wakes":                        "wake metrics",
	modulePrefix + "session":                             "M4b-4: Storage, InstanceStore",
	modulePrefix + "session/backlog/pipeline":            "pipeline helpers",
	modulePrefix + "session/domain":                      "domain values",
	modulePrefix + "session/domain/backlogdto":           "backlog DTOs",
	modulePrefix + "session/flow":                        "flow query types and SanitizeText",
	modulePrefix + "session/hostclaim":                   "claim verdict values",
	modulePrefix + "session/git":                         "worktree types",
	modulePrefix + "session/instance":                    "instance types",
}

// nonTestImports returns the sorted unique non-stdlib import paths of the non-test .go files in dir.
func nonTestImports(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, err, path)
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			if first, _, _ := strings.Cut(p, "/"); strings.Contains(first, ".") {
				seen[p] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// importSetDiff reports imports missing from allowed and allowed rows that nothing imports.
func importSetDiff(got []string, allowed map[string]string) (unlisted, unused []string) {
	have := map[string]bool{}
	for _, p := range got {
		have[p] = true
		if _, ok := allowed[p]; !ok {
			unlisted = append(unlisted, p)
		}
	}
	for p := range allowed {
		if !have[p] {
			unused = append(unused, p)
		}
	}
	sort.Strings(unused)
	return unlisted, unused
}

const importSetAdvice = "to ADD an import: justify it and add a row to allowedImports; " +
	"to REMOVE one (an M4b slice): delete its row"

func TestServerMCP_should_ImportExactlyTheAllowedSet_When_NonTestSourceScanned(t *testing.T) {
	t.Parallel()
	unlisted, unused := importSetDiff(nonTestImports(t, "."), allowedImports)
	require.Empty(t, unlisted, "server/mcp imports packages missing from allowedImports; "+importSetAdvice)
	require.Empty(t, unused, "allowedImports lists packages server/mcp no longer imports; "+importSetAdvice)
}

func TestImportSetDiff_should_FlagBothSides_When_SetsDiffer(t *testing.T) {
	t.Parallel()
	allowed := map[string]string{"a.com/x": "", "b.com/y": ""}

	unlisted, unused := importSetDiff([]string{"a.com/x", "c.com/z"}, allowed)

	require.Equal(t, []string{"c.com/z"}, unlisted)
	require.Equal(t, []string{"b.com/y"}, unused)
}
