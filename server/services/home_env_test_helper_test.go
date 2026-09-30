package services

import (
	"os"
	"sync"
	"testing"
)

// homeEnvMu guards every test in this package that mutates the process-global
// HOME environment variable. HOME is read live (uncached) by
// config.GetConfigDirForDir via os.UserHomeDir() (config/config.go:125), so
// two tests that both point HOME somewhere else at the same time can each
// observe the other's value mid-test — including during t.TempDir()'s
// end-of-test os.RemoveAll, which then intermittently fails with "directory
// not empty" because a concurrently-running test using the wrong (stale or
// swapped) HOME wrote into it.
//
// t.Parallel()'s own guard only prevents calling t.Setenv after a test has
// gone parallel within that same test's goroutine — it does nothing to stop
// two *different* serial tests, or a serial test and an already-dispatched
// parallel test, from mutating HOME concurrently in this package's ~100
// files. Any new test in this package that reads or writes HOME must take
// this lock (withFakeHome/withFakeHomeAsFile do this for you); a bare
// t.Setenv("HOME", ...) reintroduces the exact race this file exists to
// close.
var homeEnvMu sync.Mutex

// withFakeHome points HOME at a fresh temp directory for the duration of t,
// serialized against every other HOME-mutating test in this package via
// homeEnvMu. The lock is acquired before HOME is touched and released via
// t.Cleanup, so it is always released even if t later calls t.Fatal.
func withFakeHome(t *testing.T) string {
	t.Helper()
	homeEnvMu.Lock()
	t.Cleanup(homeEnvMu.Unlock)

	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// withFakeHomeAsFile points HOME at a regular file rather than a directory,
// for tests that need ~/whatever's parent to fail MkdirAll. Serialized via
// homeEnvMu like withFakeHome.
func withFakeHomeAsFile(t *testing.T) string {
	t.Helper()
	homeEnvMu.Lock()
	t.Cleanup(homeEnvMu.Unlock)

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
// after each call (AC1) by acquiring and releasing it twice in sequence; a
// helper that failed to release via t.Cleanup would deadlock here. Each call
// runs in its own subtest so t.Cleanup actually fires between them — t.Cleanup
// callbacks registered on the same *testing.T all run at the end of that test
// function (LIFO), not immediately after the call that registered them, so
// calling withFakeHome(t) twice against one shared t would deadlock the
// second Lock() waiting on a cleanup that can only run after the test
// returns.
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
