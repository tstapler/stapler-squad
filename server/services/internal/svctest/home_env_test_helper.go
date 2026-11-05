package services

import (
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
