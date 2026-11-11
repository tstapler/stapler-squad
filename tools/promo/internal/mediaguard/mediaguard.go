// Package mediaguard fails PRs that add binary media outside an allow-list or
// add oversized files. Every gate input (allow-list, CODEOWNERS) is read from the
// base blob so a PR cannot change the rules it is judged by.
package mediaguard

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

// Gate input and label names.
const (
	AllowlistPath = "tools/promo/mediaguard/allowlist.txt"
	Label         = "allow-media"
	// DefaultMaxBytes is the size backstop for added files.
	DefaultMaxBytes = 500 * 1024
)

// Exit codes.
const (
	ExitOK       = 0
	ExitFindings = 1
	ExitError    = 2
)

func mediaExtensions() map[string]bool {
	m := map[string]bool{}
	for _, e := range strings.Fields("png jpg jpeg gif webp avif webm mp4 mov") {
		m[e] = true
	}
	return m
}

func isMedia(p string) bool {
	return mediaExtensions()[strings.ToLower(strings.TrimPrefix(path.Ext(p), "."))]
}

// Violation is one file the guard rejects.
type Violation struct {
	Path   string
	Size   int64
	Reason string
}

// Allowlist is a list of doublestar globs.
type Allowlist []string

// ParseAllowlist reads one glob per line; blank lines and # comments are skipped.
func ParseAllowlist(data []byte) (Allowlist, error) {
	var out Allowlist
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !doublestar.ValidatePattern(line) {
			return nil, fmt.Errorf("allow-list line %d: invalid glob %q", i+1, line)
		}
		out = append(out, line)
	}
	return out, nil
}

// Allows reports whether p matches any glob.
func (a Allowlist) Allows(p string) bool {
	for _, g := range a {
		if ok, _ := doublestar.Match(g, p); ok {
			return true
		}
	}
	return false
}

// Check classifies the PR's changes. Deletions are ignored; modified media must
// also be allow-listed; the size ceiling applies to added files only.
func Check(ch gitbase.Changes, allow Allowlist, maxBytes int64) []Violation {
	var out []Violation
	for _, c := range ch.Changed {
		if allow.Allows(c.Path) {
			continue
		}
		switch {
		case isMedia(c.Path):
			out = append(out, Violation{Path: c.Path, Size: c.Size, Reason: "binary media not in " + AllowlistPath})
		case c.Added && c.Size > maxBytes:
			out = append(out, Violation{Path: c.Path, Size: c.Size, Reason: fmt.Sprintf("added file exceeds the %d byte ceiling", maxBytes)})
		}
	}
	return out
}

// Run executes mediaguard against the process environment.
func Run(args []string, stdout, stderr io.Writer) int { return RunEnv(args, stdout, stderr, os.Getenv) }

// RunEnv is Run with an injectable environment (for GITHUB_STEP_SUMMARY).
func RunEnv(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("mediaguard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoDir := fs.String("repo", ".", "repository root")
	base := fs.String("base", "", "base commit (default HEAD^1, valid only on a pull_request merge ref)")
	labelled := fs.Bool("allow-media", false, "the PR currently carries the "+Label+" label")
	events := fs.String("events-json", "", "file with the PR issue events (gh api repos/{owner}/{repo}/issues/{n}/events --paginate)")
	maxBytes := fs.Int64("max-bytes", DefaultMaxBytes, "size ceiling for added files")
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	code, err := run(*repoDir, *base, *labelled, *events, *maxBytes, stdout, getenv("GITHUB_STEP_SUMMARY"))
	if err != nil {
		fmt.Fprintln(stderr, "mediaguard:", err)
		return ExitError
	}
	return code
}

func run(repoDir, base string, labelled bool, eventsFile string, maxBytes int64, stdout io.Writer, summaryPath string) (int, error) {
	r, err := gitbase.Open(repoDir)
	if err != nil {
		return 0, err
	}
	changes, err := r.Changes(base)
	if err != nil {
		return 0, err
	}
	allow, err := readAllowlist(r, base)
	if err != nil {
		return 0, err
	}
	violations := Check(changes, allow, maxBytes)
	if len(violations) == 0 {
		return ExitOK, nil
	}
	var total int64
	for _, v := range violations {
		total += v.Size
		fmt.Fprintf(stdout, "%s: %s (%d bytes)\n", v.Path, v.Reason, v.Size)
	}
	if !labelled {
		return ExitFindings, nil
	}
	decision, err := decideOverride(r, base, eventsFile)
	if err != nil {
		return 0, err
	}
	if !decision.Allowed {
		fmt.Fprintf(stdout, "%s label ignored: %s\n", Label, decision.Reason)
		return ExitFindings, nil
	}
	note := fmt.Sprintf("notice: %s label applied by %s overrides the media guard for %d file(s), %d bytes added", Label, decision.Actor, len(violations), total)
	fmt.Fprintln(stdout, note)
	if summaryPath != "" {
		if err := appendFile(summaryPath, "### mediaguard override\n\n"+note+"\n"); err != nil {
			return 0, err
		}
	}
	return ExitOK, nil
}

func readAllowlist(r *gitbase.Repo, base string) (Allowlist, error) {
	data, err := r.ReadBlob(base, AllowlistPath)
	if errors.Is(err, gitbase.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseAllowlist(data)
}

func appendFile(p, text string) error {
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write job summary: %w", err)
	}
	defer f.Close()
	_, err = io.WriteString(f, text)
	return err
}
