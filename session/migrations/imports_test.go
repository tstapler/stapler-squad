package migrations

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Direct-import half of the leaf guarantee (depguard isolate_migrations is the
// lint half); the transitive half is `scoping boundaries`.
func TestNonTestFiles_should_NotImportSessionOrGitHub(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			require.NotEqual(t, "github.com/tstapler/stapler-squad/session", path, f)
			require.NotEqual(t, "github.com/tstapler/stapler-squad/github", path, f)
			require.False(t, strings.HasPrefix(path, "github.com/tstapler/stapler-squad/server"), f)
		}
	}
}
