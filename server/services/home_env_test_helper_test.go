package services

import (
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// withFakeHomeAsFile points HOME at a regular file rather than a directory,
// for tests that need ~/whatever's parent to fail MkdirAll. Serialized via
// homeEnvMu like withFakeHome.
func withFakeHomeAsFile(t *testing.T) string {
	t.Helper()
	svctest.HomeLock.Acquire(t)

	tmpFile, err := os.CreateTemp("", "not-a-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp file for fake HOME: %v", err)
	}
	name := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file for fake HOME: %v", err)
	}
	t.Cleanup(func() { os.Remove(name) })

	t.Setenv("HOME", name)
	return name
}

// TestWithFakeHome_ReleasesLockOnSuccessiveCalls proves homeEnvMu is released
// after each call by acquiring it from two successive subtests; a helper that
// failed to release via t.Cleanup would deadlock here. Each call runs in its
// own subtest so t.Cleanup (and therefore the unlock) fires between them.
func TestWithFakeHome_ReleasesLockOnSuccessiveCalls(t *testing.T) {
	var first, second string
	t.Run("first", func(t *testing.T) {
		first = svctest.WithFakeHome(t)
		if first == "" {
			t.Fatal("withFakeHome returned empty path")
		}
	})
	t.Run("second", func(t *testing.T) {
		second = svctest.WithFakeHome(t)
		if second == "" {
			t.Fatal("withFakeHome returned empty path")
		}
	})
	if first == second {
		t.Fatal("withFakeHome returned the same temp dir twice")
	}
}

// TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls proves stateEnvMu is
// released after each call, using one subtest per call for the same t.Cleanup
// ordering reason as TestWithFakeHome_ReleasesLockOnSuccessiveCalls.
func TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls(t *testing.T) {
	var first, second string
	t.Run("first", func(t *testing.T) {
		first = svctest.WithIsolatedStateDir(t)
		if first == "" || os.Getenv("STAPLER_SQUAD_TEST_DIR") != first {
			t.Fatalf("STAPLER_SQUAD_TEST_DIR = %q, want %q", os.Getenv("STAPLER_SQUAD_TEST_DIR"), first)
		}
	})
	t.Run("second", func(t *testing.T) {
		second = svctest.WithIsolatedStateDirPath(t, t.TempDir())
		if second == "" {
			t.Fatal("withIsolatedStateDirPath returned empty path")
		}
	})
	if first == second {
		t.Fatal("helpers returned the same temp dir twice")
	}
}

// TestEnvLock_ReentrantWithinOneTestAndSubtests proves a helper called twice in
// one test, and again from a subtest, does not block on its own lock.
func TestEnvLock_ReentrantWithinOneTestAndSubtests(t *testing.T) {
	a := svctest.WithIsolatedStateDir(t)
	b := svctest.WithIsolatedStateDir(t) // would deadlock without re-entrancy
	h1 := svctest.WithFakeHome(t)
	h2 := svctest.WithFakeHome(t)
	if a == b || h1 == h2 {
		t.Fatal("expected distinct temp dirs per call")
	}
	if got := os.Getenv("STAPLER_SQUAD_TEST_DIR"); got != b {
		t.Fatalf("STAPLER_SQUAD_TEST_DIR = %q, want latest %q", got, b)
	}
	t.Run("sub", func(t *testing.T) {
		c := svctest.WithIsolatedStateDir(t)
		if os.Getenv("STAPLER_SQUAD_TEST_DIR") != c {
			t.Fatal("subtest did not set STAPLER_SQUAD_TEST_DIR")
		}
	})
}

// TestNoBareEnvMutationInServicesTests fails when a test file in this package
// sets HOME or STAPLER_SQUAD_TEST_DIR directly, or calls
// envtest.NewIsolatedStateDir, instead of going through withFakeHome /
// withIsolatedStateDir. Without this guard the migration decays as new tests
// are added.
func TestNoBareEnvMutationInServicesTests(t *testing.T) {
	// The helper file itself, and the one documented exception that needs raw,
	// uncontended control of both variables (see its doc comment).
	exempt := map[string]bool{
		"home_env_test_helper_test.go":         true,
		"session_service_search_index_test.go": true,
	}
	guarded := map[string]bool{"HOME": true, "STAPLER_SQUAD_TEST_DIR": true}
	setters := map[string]bool{"Setenv": true, "Unsetenv": true}

	var files []string
	for _, dir := range []string{".", "backlogsvc", "wsconn"} {
		matches, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil || len(matches) == 0 {
			t.Fatalf("no *_test.go files found in %s (err=%v); guard would pass vacuously", dir, err)
		}
		files = append(files, matches...)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if exempt[file] {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pos := fset.Position(call.Pos())
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "envtest" && sel.Sel.Name == "NewIsolatedStateDir" {
				t.Errorf("%s: use withIsolatedStateDir instead of envtest.NewIsolatedStateDir", pos)
			}
			if !setters[sel.Sel.Name] || len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if name, err := strconv.Unquote(lit.Value); err == nil && guarded[name] {
				t.Errorf("%s: bare %s(%q); use withFakeHome / withIsolatedStateDir", pos, sel.Sel.Name, name)
			}
			return true
		})
	}
}
