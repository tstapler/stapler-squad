// Package termcheck is the termcheck command shared by cmd/termcheck and
// `promoguard termcheck`.
package termcheck

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

// AllowlistPath is the repo-relative allow-list file.
const AllowlistPath = "tools/promo/termcheck/allowlist.txt"

// Exit codes.
const (
	ExitOK       = 0
	ExitFindings = 1
	ExitError    = 2
)

// Run executes termcheck with args (without the program name) and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("termcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "source", "source (tracked files), dist (built site dir) or media-text (DOM text dump dir)")
	repo := fs.String("repo", ".", "repository root (source mode)")
	base := fs.String("base", "", "read the allow-list from this base commit instead of the working tree (CI uses HEAD^1)")
	allowPath := fs.String("allowlist", AllowlistPath, "allow-list path, repo-relative")
	dir := fs.String("dir", "", "directory to scan (dist and media-text modes)")
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	res, err := run(*mode, *repo, *base, *allowPath, *dir)
	if err != nil {
		fmt.Fprintln(stderr, "termcheck:", err)
		return ExitError
	}
	for _, f := range res.Findings {
		fmt.Fprintln(stdout, f)
	}
	for _, s := range res.Stale {
		fmt.Fprintf(stdout, "stale allow-list entry (file no longer contains a banned term): %s\n", s)
	}
	if res.Clean() {
		return ExitOK
	}
	return ExitFindings
}

func run(mode, repoDir, base, allowPath, dir string) (termscan.Result, error) {
	switch mode {
	case "source":
		return runSource(repoDir, base, allowPath)
	case "dist", "media-text":
		if dir == "" {
			return termscan.Result{}, fmt.Errorf("-dir is required for -mode=%s", mode)
		}
		return termscan.Scan(termscan.DirSource{Dir: dir}, termscan.Options{AllPaths: true, ScanNames: mode == "dist"})
	default:
		return termscan.Result{}, fmt.Errorf("unknown -mode %q (want source, dist or media-text)", mode)
	}
}

func runSource(repoDir, base, allowPath string) (termscan.Result, error) {
	r, err := gitbase.Open(repoDir)
	if err != nil {
		return termscan.Result{}, err
	}
	data, err := readAllowlist(r, repoDir, base, allowPath)
	if err != nil {
		return termscan.Result{}, err
	}
	allow, err := termscan.ParseAllowlist(data)
	if err != nil {
		return termscan.Result{}, err
	}
	return termscan.Scan(termscan.GitSource{Repo: r, Root: repoDir}, termscan.Options{Allow: allow})
}

// readAllowlist returns the allow-list bytes; an absent file is an empty list.
func readAllowlist(r *gitbase.Repo, repoDir, base, allowPath string) ([]byte, error) {
	var data []byte
	var err error
	if base != "" {
		data, err = r.ReadBlob(base, allowPath)
		if errors.Is(err, gitbase.ErrNotFound) {
			return nil, nil
		}
		return data, err
	}
	data, err = os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(allowPath)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
