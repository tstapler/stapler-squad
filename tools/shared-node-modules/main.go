// Command shared-node-modules materializes web-app/node_modules as a symlink into a
// content-addressed directory shared by every git worktree, instead of a full
// per-worktree bun install.
//
// Why: Bun's install cache already dedupes package bytes across worktrees, but not the
// ~82,000-entry file/symlink tree it materializes into each node_modules, which drove
// filesystem inode pressure across the many worktrees this repo accumulates. See
// docs/explanation/package-management-evaluation.md and bun-migration-feasibility.md.
//
// Usage:
//
//	go run ./tools/shared-node-modules <web-app-dir>
//
// Environment:
//
//	STAPLER_SQUAD_SHARED_NODE_MODULES=0        always do a plain local install
//	STAPLER_SQUAD_SHARED_NODE_MODULES_DIR=...  shared root (default ~/.stapler-squad/shared-node-modules)
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: shared-node-modules <web-app-dir>")
		os.Exit(2)
	}
	opts, err := optionsFromEnv(os.Args[1])
	if err == nil {
		err = Run(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "shared-node-modules:", err)
		os.Exit(1)
	}
}

func optionsFromEnv(webAppDir string) (Options, error) {
	root := os.Getenv("STAPLER_SQUAD_SHARED_NODE_MODULES_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Options{}, err
		}
		root = filepath.Join(home, ".stapler-squad", "shared-node-modules")
	}
	return Options{
		WebAppDir:  webAppDir,
		SharedRoot: root,
		Disabled:   os.Getenv("STAPLER_SQUAD_SHARED_NODE_MODULES") == "0",
		Install:    func(dir string) error { return bunInstall(webAppDir, dir) },
		Log:        os.Stdout,
		Warn:       os.Stderr,
	}, nil
}

// bunInstall installs into target. It runs bun from webAppDir and passes --cwd because
// version managers like asdf resolve the pinned Bun from the working directory's
// .tool-versions, which the shared store directory does not have (the shim exits 126).
func bunInstall(webAppDir, target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := bunInstallCmd(ctx, webAppDir, target)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func bunInstallCmd(ctx context.Context, webAppDir, target string) *exec.Cmd {
	cmd := safeexec.CommandContext(ctx, "bun", "install", "--frozen-lockfile", "--cwd", target)
	cmd.Dir = webAppDir
	cmd.Env = envWithout(os.Environ(), "npm_config_user_agent")
	return cmd
}

// envWithout drops name so Bun sets its own user agent: it keeps an inherited npm/npx
// value, which web-app's preinstall guard (requires "bun/") then rejects.
func envWithout(env []string, name string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return out
}
