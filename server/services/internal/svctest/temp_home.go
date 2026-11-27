package svctest

import (
	"fmt"
	"os"
	"path/filepath"
)

// UseTempHome points HOME at one throwaway directory for the whole test binary, so a test
// whose code exports files under $HOME never touches the developer's. Call it from TestMain
// before Main and run the returned cleanup after. Tests stay free to call t.Parallel;
// WithFakeHomePath still overrides HOME per test.
func UseTempHome() (cleanup func(), err error) {
	if err := pinGoToolDirs(); err != nil {
		return nil, fmt.Errorf("temp HOME: %w", err)
	}
	home, err := os.MkdirTemp("", "ssq-test-home-")
	if err != nil {
		return nil, fmt.Errorf("temp HOME: %w", err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		return nil, fmt.Errorf("temp HOME: %w", err)
	}
	return func() { _ = os.RemoveAll(home) }, nil
}

// pinGoToolDirs keeps the go command's caches where they are once HOME moves, for tests
// that run go (package loading, builds).
func pinGoToolDirs() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // no HOME (a sandbox): nothing to preserve
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(home, "go")
	}
	defaults := map[string]string{
		"GOPATH":     gopath,
		"GOMODCACHE": filepath.Join(gopath, "pkg", "mod"),
		"GOCACHE":    filepath.Join(cache, "go-build"),
	}
	for name, value := range defaults {
		if os.Getenv(name) != "" {
			continue
		}
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	return nil
}
