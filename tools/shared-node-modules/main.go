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
	"time"
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
		Install:    bunInstall,
		Log:        os.Stdout,
		Warn:       os.Stderr,
	}, nil
}

func bunInstall(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bun", "install", "--frozen-lockfile")
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
