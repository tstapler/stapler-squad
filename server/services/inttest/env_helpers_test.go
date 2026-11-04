package inttest

import "testing"

// withFakeHome and withIsolatedStateDir replace the services package's re-exported helpers: this package is its own
// test binary, so the package-wide env mutexes those helpers shared with services' tests no longer apply.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func withIsolatedStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	return dir
}
