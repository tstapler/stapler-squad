package services

import (
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// walkSourceDirs returns root and every Go-source directory beneath it, so a
// scanner covers a new leaf under server/services/<leaf>/ with no edit. It
// skips fixture, generated, vendored and hidden directories.
func walkSourceDirs(t testing.TB, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			switch name := d.Name(); {
			case name == "testdata", name == "vendor", name == "node_modules", name == "gen", strings.HasPrefix(name, "."):
				return filepath.SkipDir
			}
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(dirs)
	return dirs
}

// servicesSourceDirs is walkSourceDirs over this package and all its leaves.
func servicesSourceDirs(t testing.TB) []string { return walkSourceDirs(t, ".") }

// nonScannedServicesDirs lists immediate subdirectories the scanners skip by design.
var nonScannedServicesDirs = map[string]string{
	"testdata": "fixtures, not Go source",
}

func TestServicesScannerDiscovery_WalksNewLeafDirs_WhenOneIsAdded(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, d := range []string{"newleaf", "newleaf/inner", "testdata/x", ".hidden", "vendor/v"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := walkSourceDirs(t, root)
	want := []string{root, filepath.Join(root, "newleaf"), filepath.Join(root, "newleaf", "inner")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("walkSourceDirs = %v, want %v", got, want)
	}
}

// TestServicesScannerDiscovery_CoversEveryLeafDir fails when a subdirectory of
// server/services is neither scanned nor documented in nonScannedServicesDirs.
func TestServicesScannerDiscovery_CoversEveryLeafDir(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	scanned := map[string]bool{}
	for _, d := range servicesSourceDirs(t) {
		scanned[filepath.ToSlash(d)] = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		_, excluded := nonScannedServicesDirs[e.Name()]
		if excluded == scanned[e.Name()] {
			t.Errorf("leaf dir %s: scanned=%v but excluded=%v; scanners must cover every leaf or list it in nonScannedServicesDirs", e.Name(), scanned[e.Name()], excluded)
		}
	}
	for _, leaf := range []string{"backlogsvc", "wsconn", "sessionretention", "webhooktrigger", "unfinishedwork"} {
		if !scanned[leaf] {
			t.Errorf("known leaf %s not scanned", leaf)
		}
	}
}

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
	// The helper file itself, and the documented exceptions that need raw,
	// uncontended control of both variables (see their doc comments).
	// inttest is its own test binary, so its helper definitions are the
	// leaf's counterpart of the helper file above.
	exempt := map[string]bool{
		"home_env_test_helper_test.go":         true,
		"session_service_search_index_test.go": true,
		"inttest/env_helpers_test.go":          true,
	}
	guarded := map[string]bool{"HOME": true, "STAPLER_SQUAD_TEST_DIR": true}
	setters := map[string]bool{"Setenv": true, "Unsetenv": true}

	var files []string
	for _, dir := range servicesSourceDirs(t) {
		matches, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		if dir == "." && len(matches) == 0 {
			t.Fatalf("no *_test.go files found in %s; guard would pass vacuously", dir)
		}
		files = append(files, matches...)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if exempt[filepath.ToSlash(file)] {
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
