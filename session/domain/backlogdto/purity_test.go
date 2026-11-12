package backlogdto

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

const (
	modulePrefix = "github.com/tstapler/stapler-squad"
	domainPath   = modulePrefix + "/session/domain"
)

// backlogdto may import only the standard library and session/domain.
func TestImportsOnlyStdlibAndDomain(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, modulePrefix) && path != domainPath {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}
