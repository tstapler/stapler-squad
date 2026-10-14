package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// homeEnvMu guards every test in this package that mutates the process-global
// HOME environment variable. HOME is read live (uncached) by
// config.GetConfigDirForDir via os.UserHomeDir() (config/config.go:125), so a
// writer that resolves its path under another test's HOME could plausibly make
// t.TempDir()'s end-of-test os.RemoveAll fail with "directory not empty". That
// mechanism is a hypothesis: the flake has not been reproduced on main.
//
// The lock is belt-and-braces: t.Setenv panics in parallel tests and serial
// tests run one at a time, so two mutators rarely overlap. It does NOT stop a
// goroutine leaked from one test from reading HOME live during another test —
// that reader side takes no lock. Any new test in this package that reads or
// writes HOME should go through withFakeHome/withFakeHomeAsFile;
// TestNoBareEnvMutationInServicesTests enforces that.
var homeEnvMu sync.Mutex

// homeLock / stateLock pair each mutex with the holder record that makes
// acquisition re-entrant per test — see envLock.acquire.
var (
	homeLock  = &envLock{mu: &homeEnvMu}
	stateLock = &envLock{mu: &stateEnvMu}
)

// withFakeHome points HOME at a fresh temp directory for the duration of t,
// serialized against every other HOME-mutating test in this package via
// homeEnvMu. The lock is acquired before HOME is touched and released via
// t.Cleanup, so it is always released even if t later calls t.Fatal.
func withFakeHome(t *testing.T) string {
	t.Helper()
	// Lock before t.TempDir() so the dir is created under the lock.
	homeLock.acquire(t)
	return withFakeHomePath(t, t.TempDir())
}

// withFakeHomePath is withFakeHome for tests that already have (or must
// pre-populate) the directory HOME should point at.
func withFakeHomePath(t *testing.T, home string) string {
	t.Helper()
	homeLock.acquire(t)

	t.Setenv("HOME", home)
	return home
}

// withFakeHomeAsFile points HOME at a regular file rather than a directory,
// for tests that need ~/whatever's parent to fail MkdirAll. Serialized via
// homeEnvMu like withFakeHome.
func withFakeHomeAsFile(t *testing.T) string {
	t.Helper()
	homeLock.acquire(t)

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

// stateEnvMu guards every test in this package that mutates the process-global
// STAPLER_SQUAD_TEST_DIR environment variable, which config.GetConfigDirForDir
// reads live (Priority 1). It exists for the same reason as homeEnvMu: a
// background goroutine from one test that re-resolves its config path after a
// different test has reassigned the variable writes into (or expects) a
// directory that other test owns and is tearing down. Use withIsolatedStateDir
// rather than a bare t.Setenv or envtest.NewIsolatedStateDir.
//
// Lock order: by convention withFakeHome before withIsolatedStateDir. It is
// advisory: serial tests never overlap, so no deadlock is possible today.
var stateEnvMu sync.Mutex

// withIsolatedStateDir points STAPLER_SQUAD_TEST_DIR at a fresh temp directory
// for the duration of t, serialized against every other state-dir-mutating test
// in this package via stateEnvMu. The lock is released via t.Cleanup, so it is
// released even if t later calls t.Fatal. Like t.Setenv, it must be called
// before t.Parallel(). Repeat calls on one *testing.T (or its subtests) are
// safe: lock acquisition is re-entrant and the latest value wins.
func withIsolatedStateDir(t *testing.T) string {
	t.Helper()
	stateLock.acquire(t)
	return withIsolatedStateDirPath(t, t.TempDir())
}

// withIsolatedStateDirPath is withIsolatedStateDir for tests that already have
// (or must pre-populate) the directory, e.g. to seed a config.json first.
func withIsolatedStateDirPath(t *testing.T, dir string) string {
	t.Helper()
	stateLock.acquire(t)

	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	return dir
}

// TestWithFakeHome_ReleasesLockOnSuccessiveCalls proves homeEnvMu is released
// after each call by acquiring it from two successive subtests; a helper that
// failed to release via t.Cleanup would deadlock here. Each call runs in its
// own subtest so t.Cleanup (and therefore the unlock) fires between them.
func TestWithFakeHome_ReleasesLockOnSuccessiveCalls(t *testing.T) {
	var first, second string
	t.Run("first", func(t *testing.T) {
		first = withFakeHome(t)
		if first == "" {
			t.Fatal("withFakeHome returned empty path")
		}
	})
	t.Run("second", func(t *testing.T) {
		second = withFakeHome(t)
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
		first = withIsolatedStateDir(t)
		if first == "" || os.Getenv("STAPLER_SQUAD_TEST_DIR") != first {
			t.Fatalf("STAPLER_SQUAD_TEST_DIR = %q, want %q", os.Getenv("STAPLER_SQUAD_TEST_DIR"), first)
		}
	})
	t.Run("second", func(t *testing.T) {
		second = withIsolatedStateDirPath(t, t.TempDir())
		if second == "" {
			t.Fatal("withIsolatedStateDirPath returned empty path")
		}
	})
	if first == second {
		t.Fatal("helpers returned the same temp dir twice")
	}
}

// WithFakeHome and WithIsolatedStateDir re-export the helpers above for this
// directory's external services_test package, which cannot see unexported
// identifiers but shares the same test binary (and therefore must share the
// same mutexes).
func WithFakeHome(t *testing.T) string { t.Helper(); return withFakeHome(t) }

func WithIsolatedStateDir(t *testing.T) string { t.Helper(); return withIsolatedStateDir(t) }

// envLock is a mutex plus a record of which test holds it, so the holder (and
// its subtests) can re-enter instead of deadlocking on a lock it already owns.
type envLock struct {
	mu     *sync.Mutex
	guard  sync.Mutex // protects holder
	holder string     // t.Name() of the holder; "" when unheld
}

func (l *envLock) heldBy(name string) bool {
	l.guard.Lock()
	defer l.guard.Unlock()
	return l.holder != "" && (name == l.holder || strings.HasPrefix(name, l.holder+"/"))
}

func (l *envLock) setHolder(name string) {
	l.guard.Lock()
	l.holder = name
	l.guard.Unlock()
}

// acquire takes the mutex for the duration of t, releasing it via t.Cleanup.
// It is re-entrant per test: if t is the current holder, or a subtest of it
// (t.Name() has the holder's name plus "/" as a prefix), it returns without
// locking, so helper-of-helper call chains and subtests that call
// withFakeHome/withIsolatedStateDir again cannot self-deadlock. The holder's
// own t.Cleanup releases the lock, and t.Setenv (called by the caller) restores
// the variable per-t in LIFO order. Only the first acquirer registers cleanup.
//
// Lock order is home, then state; the reverse is not enforced because serial
// tests never overlap, but keep it so a future parallel-capable helper cannot
// deadlock.
func (l *envLock) acquire(t *testing.T) {
	t.Helper()
	if l.heldBy(t.Name()) {
		return
	}
	l.mu.Lock()
	l.setHolder(t.Name())
	t.Cleanup(func() {
		l.setHolder("")
		l.mu.Unlock()
	})
}

// TestEnvLock_ReentrantWithinOneTestAndSubtests proves a helper called twice in
// one test, and again from a subtest, does not block on its own lock.
func TestEnvLock_ReentrantWithinOneTestAndSubtests(t *testing.T) {
	a := withIsolatedStateDir(t)
	b := withIsolatedStateDir(t) // would deadlock without re-entrancy
	h1 := withFakeHome(t)
	h2 := withFakeHome(t)
	if a == b || h1 == h2 {
		t.Fatal("expected distinct temp dirs per call")
	}
	if got := os.Getenv("STAPLER_SQUAD_TEST_DIR"); got != b {
		t.Fatalf("STAPLER_SQUAD_TEST_DIR = %q, want latest %q", got, b)
	}
	t.Run("sub", func(t *testing.T) {
		c := withIsolatedStateDir(t)
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

	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no *_test.go files found (err=%v); guard would pass vacuously", err)
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
