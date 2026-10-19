package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const installComplete = ".install-complete"

// Options configures Run. Install runs `pnpm install --frozen-lockfile` in a directory;
// it is injectable so tests stay fast and offline.
type Options struct {
	WebAppDir  string
	SharedRoot string
	Disabled   bool
	Install    func(dir string) error
	Log        io.Writer
	Warn       io.Writer
}

func (o Options) logf(format string, args ...any) {
	fmt.Fprintf(o.Log, "shared-node-modules: "+format+"\n", args...)
}

// Run links <WebAppDir>/node_modules at a shared install keyed by the lockfile plus the
// content of every local file: dependency (pnpm's lockfile records those with no content
// digest, so the lockfile alone would let a worktree that edited one serve another's
// stale copy).
//
// A pre-existing real node_modules (every worktree that installed before this tool
// existed) is verified with a frozen-lockfile install, then moved into the shared store,
// or discarded if another worktree already seeded that key.
func Run(o Options) error {
	nm := filepath.Join(o.WebAppDir, "node_modules")
	if o.Disabled {
		o.logf("STAPLER_SQUAD_SHARED_NODE_MODULES=0, doing a plain local pnpm install")
		if isSymlink(nm) {
			if err := os.Remove(nm); err != nil {
				return err
			}
		}
		return o.Install(o.WebAppDir)
	}

	deps, err := localFileDeps(o.WebAppDir)
	if err != nil {
		return err
	}

	migrating := exists(nm) && !isSymlink(nm)
	if migrating {
		o.logf("node_modules exists as a real directory (pre-dates shared-node-modules); verifying it against pnpm-lock.yaml before migrating it into the shared store")
		// A genuine package.json/lockfile divergence fails here, before anything is mutated.
		if err := o.Install(o.WebAppDir); err != nil {
			return fmt.Errorf("verifying existing node_modules: %w", err)
		}
	}

	key, err := contentKey(o.WebAppDir, deps)
	if err != nil {
		return err
	}
	shared := filepath.Join(o.SharedRoot, key)
	if linkedAt(nm, shared) {
		return nil
	}

	if err := o.ensureSharedInstall(shared, key, deps, migrating); err != nil {
		return err
	}
	return linkNodeModules(o, nm, shared)
}

func (o Options) ensureSharedInstall(shared, key string, deps []string, migrating bool) error {
	if err := os.MkdirAll(o.SharedRoot, 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(o.SharedRoot, key+".lock"))
	if err != nil {
		return err
	}
	defer unlock()

	marker := filepath.Join(shared, installComplete)
	nm := filepath.Join(o.WebAppDir, "node_modules")
	if exists(marker) {
		o.logf("reusing existing shared install at %s", shared)
		if migrating {
			// Another worktree seeded this key first; this tree is redundant.
			return os.RemoveAll(nm)
		}
		return nil
	}

	if err := seedManifest(o.WebAppDir, shared, deps); err != nil {
		return err
	}
	if migrating {
		o.logf("seeding shared install for %s from this worktree's already-verified node_modules (move, not reinstall)", key)
		if err := moveDir(nm, filepath.Join(shared, "node_modules")); err != nil {
			return err
		}
	} else {
		o.logf("no shared install for %s yet, installing into %s", key, shared)
		if err := o.Install(shared); err != nil {
			return err
		}
	}
	return os.WriteFile(marker, nil, 0o644)
}

func linkNodeModules(o Options, nm, shared string) error {
	if isSymlink(nm) {
		if err := os.Remove(nm); err != nil {
			return err
		}
	} else if exists(nm) {
		fmt.Fprintln(o.Warn, "shared-node-modules: node_modules exists and isn't a symlink we manage; leaving it alone (rm -rf web-app/node_modules first to opt in)")
		return nil
	}
	target := filepath.Join(shared, "node_modules")
	if err := os.Symlink(target, nm); err != nil {
		return err
	}
	o.logf("linked node_modules -> %s", target)
	return nil
}

// localFileDeps returns the file:-protocol dependency directories declared in
// package.json (dependencies, devDependencies, optionalDependencies), sorted.
func localFileDeps(webAppDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(webAppDir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}
	seen := map[string]bool{}
	var dirs []string
	for _, group := range []map[string]string{pkg.Dependencies, pkg.DevDependencies, pkg.OptionalDependencies} {
		for _, spec := range group {
			if dir, ok := strings.CutPrefix(spec, "file:"); ok && !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

// contentKey hashes pnpm-lock.yaml plus every file under each local dependency directory.
// Only file content and relative paths count, not modes or mtimes.
func contentKey(webAppDir string, deps []string) (string, error) {
	h := sha256.New()
	lock, err := fileDigest(filepath.Join(webAppDir, "pnpm-lock.yaml"))
	if err != nil {
		return "", err
	}
	fmt.Fprintf(h, "pnpm-lock.yaml\x00%s\n", lock)
	for _, dep := range deps {
		root := filepath.Join(webAppDir, dep)
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return "", fmt.Errorf("local file: dependency dir missing: %s", dep)
		}
		var files []string
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				files = append(files, p)
			}
			return err
		})
		if err != nil {
			return "", err
		}
		sort.Strings(files)
		for _, f := range files {
			sum, err := fileDigest(f)
			if err != nil {
				return "", err
			}
			rel, _ := filepath.Rel(webAppDir, f)
			fmt.Fprintf(h, "%s\x00%s\n", filepath.ToSlash(rel), sum)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- paths come from the repo checkout being installed
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// seedManifest clears shared and copies in package.json, pnpm-lock.yaml and the local
// file: dependency directories (but not node_modules) for an install to build on.
func seedManifest(webAppDir, shared string, deps []string) error {
	if err := os.RemoveAll(shared); err != nil {
		return err
	}
	if err := os.MkdirAll(shared, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"package.json", "pnpm-lock.yaml"} {
		if err := copyTree(filepath.Join(webAppDir, name), filepath.Join(shared, name)); err != nil {
			return err
		}
	}
	for _, dep := range deps {
		if err := copyTree(filepath.Join(webAppDir, dep), filepath.Join(shared, dep)); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			data, err := os.ReadFile(p) // #nosec G304 -- walking a repo directory
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
	})
}

// moveDir renames src to dst, falling back to mv(1) when they are on different
// filesystems (a pnpm node_modules is a large symlink/hardlink tree; mv preserves it).
func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "mv", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("mv %s %s: %w: %s", src, dst, err, out)
	}
	return nil
}

// lockFile takes an exclusive advisory lock so two worktrees racing on a brand-new key
// don't corrupt it. flock(2) is available on macOS and Linux, unlike the flock(1) binary.
func lockFile(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644) // #nosec G304 -- path is under the shared root
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil { // #nosec G115 -- fd fits in int
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) // #nosec G115
		f.Close()
	}, nil
}

func linkedAt(nm, shared string) bool {
	target, err := os.Readlink(nm)
	return err == nil && target == filepath.Join(shared, "node_modules") && exists(filepath.Join(shared, installComplete))
}

func isSymlink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return !errors.Is(err, fs.ErrNotExist) && err == nil
}
