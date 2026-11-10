// Package fixtures builds the on-disk pieces scenarios need at RPC-seeding
// time: a dedicated fictional git repo and canned-terminal programs.
package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// File is one tracked file of the fixture repo.
type File struct {
	Path, Content string
}

// InitRepo creates a git repo at dir with one commit holding files. Author and
// time are fixed and carry no email so the history is deterministic and leaks
// nothing. It uses go-git, never the git CLI.
func InitRepo(dir string, files []File, when time.Time) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		return fmt.Errorf("init fixture repo: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	for _, f := range files {
		full := filepath.Join(dir, f.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o600); err != nil {
			return err
		}
		if _, err := wt.Add(f.Path); err != nil {
			return fmt.Errorf("stage %s: %w", f.Path, err)
		}
	}
	sig := &object.Signature{Name: "demo-dev", When: when}
	if _, err := wt.Commit("Initial fixture commit", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		return fmt.Errorf("commit fixture repo: %w", err)
	}
	return nil
}

// WriteTerminalProgram writes transcript and an executable script that prints
// it then sleeps, returning the script path. The spike (Q5) showed the session
// `program` must be a single path, so inline `sh -c` strings do not work.
func WriteTerminalProgram(dir, name string, transcript []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	txt := filepath.Join(dir, name+".txt")
	if err := os.WriteFile(txt, transcript, 0o600); err != nil {
		return "", err
	}
	script := filepath.Join(dir, name+".sh")
	body := fmt.Sprintf("#!/bin/sh\ncat '%s'\nsleep 3600\n", txt)
	// #nosec G306 -- the script must be executable to be used as a session program.
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return "", err
	}
	return script, nil
}

// FilesViewerRepo is the content of the files-viewer fixture repo.
func FilesViewerRepo() []File {
	return []File{
		{Path: "README.md", Content: "# Docs Site\n\nA fictional documentation site used in the tours.\n"},
		{Path: "docs/getting-started.md", Content: "# Getting started\n\nInstall the toolkit and run the dev server.\n"},
		{Path: "src/index.ts", Content: "export const greeting = \"hello from the demo repo\";\n"},
	}
}
