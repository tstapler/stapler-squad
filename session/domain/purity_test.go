package domain

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/tstapler/stapler-squad"

// The domain package is the lowest layer: no non-test file may import another
// module package (a direct-import check also rules out transitive ones).
func TestDomainImportsNoOtherModulePackage(t *testing.T) {
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
			if path := strings.Trim(imp.Path.Value, `"`); strings.HasPrefix(path, modulePrefix) {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}

func TestSentinelMessagesStable(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]string{
		ErrNotFound:           "not found",
		ErrConflict:           "conflict",
		ErrPreconditionFailed: "precondition failed: concurrent modification detected",
	} {
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
	}
}

// Subpackages of session/domain may import the standard library and the
// domain package itself, nothing else from this module.
func TestDomainSubpackagesImportOnlyDomain(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*/*.go")
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
			if strings.HasPrefix(path, modulePrefix) && path != modulePrefix+"/session/domain" {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}
