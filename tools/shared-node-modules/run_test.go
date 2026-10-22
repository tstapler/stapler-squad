package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type harness struct {
	t        *testing.T
	root     string
	installs int
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, root: filepath.Join(t.TempDir(), "shared")}
}

// install stands in for bun: it records the call and fabricates a minimal node_modules.
// A FAIL_INSTALL marker simulates a frozen-lockfile divergence, failing before any mutation.
func (h *harness) install(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "FAIL_INSTALL")); err == nil {
		return os.ErrInvalid
	}
	h.installs++
	require.NoError(h.t, os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755))
	return os.MkdirAll(filepath.Join(dir, "node_modules", ".bun"), 0o755)
}

func (h *harness) run(dir string, disabled bool) (string, error) {
	var out bytes.Buffer
	err := Run(Options{WebAppDir: dir, SharedRoot: h.root, Disabled: disabled, Install: h.install, Log: &out, Warn: &out})
	return out.String(), err
}

func fixture(t *testing.T, lock, plugin string) string {
	dir := filepath.Join(t.TempDir(), "web-app")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-plugin"), 0o755))
	write(t, filepath.Join(dir, "package.json"), `{"name":"fixture","devDependencies":{"my-plugin":"file:./my-plugin"}}`)
	write(t, filepath.Join(dir, "bun.lock"), lock)
	write(t, filepath.Join(dir, "my-plugin", "index.js"), plugin)
	return dir
}

func write(t *testing.T, path, content string) {
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func linkTarget(t *testing.T, dir string) string {
	target, err := os.Readlink(filepath.Join(dir, "node_modules"))
	require.NoError(t, err, "node_modules should be a symlink")
	return target
}

func TestIdenticalWorktreesShareOneInstall(t *testing.T) {
	h := newHarness(t)
	a, b := fixture(t, "lock-v1", "rule 1"), fixture(t, "lock-v1", "rule 1")

	_, err := h.run(a, false)
	require.NoError(t, err)
	_, err = h.run(b, false)
	require.NoError(t, err)

	assert.Equal(t, linkTarget(t, a), linkTarget(t, b))
	assert.Equal(t, 1, h.installs)

	_, err = h.run(a, false) // already linked: fast path
	require.NoError(t, err)
	assert.Equal(t, 1, h.installs)
}

func TestDifferentLocalDependencyContentGetsOwnInstall(t *testing.T) {
	h := newHarness(t)
	a, c := fixture(t, "lock-v1", "rule 1"), fixture(t, "lock-v1", "rule 2")

	_, err := h.run(a, false)
	require.NoError(t, err)
	_, err = h.run(c, false)
	require.NoError(t, err)

	assert.NotEqual(t, linkTarget(t, a), linkTarget(t, c), "identical lockfile must not mask edited local plugin content")
	assert.Equal(t, 2, h.installs)
}

func TestMissingLocalDependencyDirFailsLoudly(t *testing.T) {
	h := newHarness(t)
	d := fixture(t, "lock-v1", "rule 1")
	require.NoError(t, os.RemoveAll(filepath.Join(d, "my-plugin")))

	_, err := h.run(d, false)

	require.ErrorContains(t, err, "my-plugin")
	assert.NoFileExists(t, filepath.Join(d, "node_modules"))
}

func TestOptOutDoesPlainLocalInstall(t *testing.T) {
	h := newHarness(t)
	e := fixture(t, "lock-v1", "rule 1")

	_, err := h.run(e, true)
	require.NoError(t, err)

	info, err := os.Lstat(filepath.Join(e, "node_modules"))
	require.NoError(t, err)
	assert.True(t, info.IsDir() && info.Mode()&os.ModeSymlink == 0, "opt-out must leave a real directory")
}

func TestMigrationMovesVerifiedNodeModulesIntoSharedStore(t *testing.T) {
	h := newHarness(t)
	f := fixture(t, "lock-scenario5", "rule 1")
	require.NoError(t, os.MkdirAll(filepath.Join(f, "node_modules"), 0o755))
	write(t, filepath.Join(f, "node_modules", "ORIGINAL_MARKER"), "original")

	_, err := h.run(f, false)
	require.NoError(t, err)

	assert.Equal(t, 1, h.installs, "only the verify step runs; no reinstall into the shared store")
	_, err = os.Stat(filepath.Join(linkTarget(t, f), "ORIGINAL_MARKER"))
	assert.NoError(t, err, "the local tree was moved, not reinstalled")
}

func TestMigrationDiscardsLocalTreeWhenSharedInstallExists(t *testing.T) {
	h := newHarness(t)
	g, hh := fixture(t, "lock-scenario6", "rule 1"), fixture(t, "lock-scenario6", "rule 1")
	_, err := h.run(g, false)
	require.NoError(t, err)
	before := h.installs
	require.NoError(t, os.MkdirAll(filepath.Join(hh, "node_modules"), 0o755))
	write(t, filepath.Join(hh, "node_modules", "LOCAL_MARKER"), "stale")

	_, err = h.run(hh, false)
	require.NoError(t, err)

	assert.Equal(t, 1, h.installs-before, "only the verify step runs")
	assert.Equal(t, linkTarget(t, g), linkTarget(t, hh))
	assert.NoFileExists(t, filepath.Join(hh, "node_modules", "LOCAL_MARKER"))
}

func TestFailedVerifyLeavesNodeModulesUntouched(t *testing.T) {
	h := newHarness(t)
	i := fixture(t, "lock-scenario7", "rule 1")
	require.NoError(t, os.MkdirAll(filepath.Join(i, "node_modules"), 0o755))
	write(t, filepath.Join(i, "node_modules", "SURVIVOR_MARKER"), "survive")
	write(t, filepath.Join(i, "FAIL_INSTALL"), "")

	_, err := h.run(i, false)

	require.Error(t, err)
	assert.Equal(t, 0, h.installs)
	assert.FileExists(t, filepath.Join(i, "node_modules", "SURVIVOR_MARKER"))
	assert.NoDirExists(t, h.root, "nothing may be created in the shared store before verify succeeds")
}

func TestContentKeyIgnoresFileModes(t *testing.T) {
	a, b := fixture(t, "lock", "rule"), fixture(t, "lock", "rule")
	require.NoError(t, os.Chmod(filepath.Join(b, "my-plugin", "index.js"), 0o755))

	ka, err := contentKey(a, []string{"./my-plugin"})
	require.NoError(t, err)
	kb, err := contentKey(b, []string{"./my-plugin"})
	require.NoError(t, err)

	assert.Equal(t, ka, kb)
}

func TestConcurrentWorktreesInstallOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared")
	var mu sync.Mutex
	installs := 0
	install := func(dir string) error {
		mu.Lock()
		installs++
		mu.Unlock()
		if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
			return err
		}
		return os.MkdirAll(filepath.Join(dir, "node_modules", ".bun"), 0o755)
	}
	dirs := make([]string, 4)
	for i := range dirs {
		dirs[i] = fixture(t, "lock-race", "rule")
	}

	var wg sync.WaitGroup
	for _, d := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out bytes.Buffer
			assert.NoError(t, Run(Options{WebAppDir: d, SharedRoot: root, Install: install, Log: &out, Warn: &out}))
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, installs)
	for _, d := range dirs[1:] {
		assert.Equal(t, linkTarget(t, dirs[0]), linkTarget(t, d))
	}
}

func TestLegacyPnpmTreeIsDiscardedAndReinstalled(t *testing.T) {
	h := newHarness(t)
	j := fixture(t, "lock-scenario8", "rule 1")
	require.NoError(t, os.MkdirAll(filepath.Join(j, "node_modules", ".pnpm"), 0o755))
	write(t, filepath.Join(j, "node_modules", ".modules.yaml"), "")
	write(t, filepath.Join(j, "node_modules", "PNPM_MARKER"), "pnpm-era")

	_, err := h.run(j, false)
	require.NoError(t, err)

	assert.Equal(t, 1, h.installs, "one fresh install, no separate verify step")
	assert.NoFileExists(t, filepath.Join(j, "node_modules", "PNPM_MARKER"), "pnpm-era tree must not be migrated")
	assert.DirExists(t, filepath.Join(j, "node_modules", ".bun"))
}

func TestStampTouchedOnEveryOutcome(t *testing.T) {
	h := newHarness(t)
	a := fixture(t, "lock-stamp", "rule 1")
	stamp := filepath.Join(a, "node_modules", installStamp)

	_, err := h.run(a, false)
	require.NoError(t, err)
	assert.FileExists(t, stamp, "fresh link")

	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(stamp, old, old))
	_, err = h.run(a, false)
	require.NoError(t, err)
	info, err := os.Stat(stamp)
	require.NoError(t, err)
	assert.True(t, info.ModTime().After(old.Add(30*time.Minute)), "fast path must refresh the stamp so make sees node_modules as fresh")
}

func TestBunfigChangesTheKey(t *testing.T) {
	a, b := fixture(t, "lock", "rule"), fixture(t, "lock", "rule")
	write(t, filepath.Join(b, "bunfig.toml"), "[install]\nlinker = \"isolated\"\n")

	ka, err := contentKey(a, nil)
	require.NoError(t, err)
	kb, err := contentKey(b, nil)
	require.NoError(t, err)

	assert.NotEqual(t, ka, kb, "linker settings change the tree layout, so they must change the key")
}

func TestPatchContentChangesTheKeyAndIsSeeded(t *testing.T) {
	a, b := fixture(t, "lock", "rule"), fixture(t, "lock", "rule")
	for _, d := range []string{a, b} {
		require.NoError(t, os.MkdirAll(filepath.Join(d, "patches"), 0o755))
	}
	write(t, filepath.Join(a, "patches", "eval@0.1.8.patch"), "patch 1")
	write(t, filepath.Join(b, "patches", "eval@0.1.8.patch"), "patch 2")

	ka, err := contentKey(a, withPatchesDir(a, nil))
	require.NoError(t, err)
	kb, err := contentKey(b, withPatchesDir(b, nil))
	require.NoError(t, err)
	assert.NotEqual(t, ka, kb, "a patch edit must not reuse an install built from the old patch")

	h := newHarness(t)
	_, err = h.run(a, false)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(linkTarget(t, a), "..", "patches", "eval@0.1.8.patch"), "bun install in the shared dir needs the patch file")
}
