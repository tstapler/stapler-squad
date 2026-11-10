// Command verifybin runs the post-build assertions install-service makes on the Bazel-built
// binary before it stops the live service: stable path, a real `version`, the embedded tmux
// and tymuxd equal to the staged build outputs, and (macOS) the signed copy's designated
// requirement equal to the one the owner recorded. Exit 0 means every check passed.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/codesign"
)

type config struct {
	bin, tmux, tymuxd, codesignBin, drDoc string
	checkCodesign, allowDev               bool
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	var c config
	fs := flag.NewFlagSet("verifybin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.bin, "bin", "", "binary to verify (required)")
	fs.StringVar(&c.tmux, "tmux", "", "staged tmux build output the binary must embed (optional)")
	fs.StringVar(&c.tymuxd, "tymuxd", "", "staged tymuxd build output the binary must embed (optional)")
	fs.BoolVar(&c.checkCodesign, "codesign", false, "compare codesign -d -r- of the binary with the recorded requirement (macOS)")
	fs.StringVar(&c.codesignBin, "codesign-bin", "codesign", "codesign executable")
	fs.StringVar(&c.drDoc, "dr-doc", "", "doc holding the recorded designated requirement (with -codesign)")
	fs.BoolVar(&c.allowDev, "allow-dev", false, "accept the unstamped version \"dev\"")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if c.bin == "" || (c.checkCodesign && c.drDoc == "") {
		fmt.Fprintln(stderr, "verifybin: -bin is required, and -dr-doc with -codesign")
		return 2
	}
	failed := false
	for _, check := range []func(config, io.Writer) error{checkPath, checkVersion, checkEmbedded, checkCodesign} {
		if err := check(c, stdout); err != nil {
			fmt.Fprintln(stderr, "verifybin: FAIL:", err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	fmt.Fprintln(stdout, "verifybin: all checks passed")
	return 0
}

// checkPath rejects a symlink or a path under a bazel-* output tree: the service ExecStart must be a stable copy.
func checkPath(c config, out io.Writer) error {
	info, err := os.Lstat(c.bin)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o100 == 0 {
		return fmt.Errorf("%s must be a regular executable file, not a symlink (mode %v)", c.bin, info.Mode())
	}
	abs, err := filepath.Abs(c.bin)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	if name := bazelOutputComponent(abs); name != "" {
		return fmt.Errorf("%s points into a Bazel output tree (%s); copy it to a stable path first", abs, name)
	}
	if name := bazelOutputComponent(resolved); name != "" {
		return fmt.Errorf("%s resolves into a Bazel output tree (%s); copy it to a stable path first", abs, name)
	}
	fmt.Fprintf(out, "path: ok (%s)\n", abs)
	return nil
}

// bazelOutputComponent returns the first path component that names a Bazel output tree. Only the
// exact convenience symlinks and the execroot are matched: a checkout directory merely named bazel-* is fine.
func bazelOutputComponent(path string) string {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		switch part {
		case "bazel-bin", "bazel-out", "bazel-testlogs", "execroot":
			return part
		}
	}
	return ""
}

func checkVersion(c config, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, c.bin, "version").Output()
	if err != nil {
		return fmt.Errorf("%s version: %w", c.bin, err)
	}
	// The woven binary logs JSON to stdout before the version line, so look for the line rather than take the first.
	var ver string
	found := false
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "stapler-squad version "); ok {
			ver, found = v, true
			break
		}
	}
	if !found || strings.TrimSpace(ver) == "" {
		return fmt.Errorf("version output has no \"stapler-squad version <v>\" line: %q", strings.TrimSpace(string(raw)))
	}
	if strings.TrimSpace(ver) == "dev" && !c.allowDev {
		return fmt.Errorf("version is the unstamped \"dev\"; was the binary built with --config=release (--stamp)?")
	}
	fmt.Fprintf(out, "version: ok (%s)\n", strings.TrimSpace(ver))
	return nil
}

func checkEmbedded(c config, out io.Writer) error {
	if c.tmux == "" && c.tymuxd == "" {
		return nil
	}
	bin, err := os.ReadFile(c.bin)
	if err != nil {
		return err
	}
	var errs []string
	for _, e := range []struct{ name, path string }{{"tmux", c.tmux}, {"tymuxd", c.tymuxd}} {
		if e.path == "" {
			continue
		}
		want, err := os.ReadFile(e.path)
		if err != nil || len(want) == 0 {
			errs = append(errs, fmt.Sprintf("%s reference %s unreadable or empty: %v", e.name, e.path, err))
			continue
		}
		sum := sha256.Sum256(want)
		if !bytes.Contains(bin, want) {
			errs = append(errs, fmt.Sprintf("%s (sha256 %x) is not embedded in %s", e.name, sum, c.bin))
			continue
		}
		fmt.Fprintf(out, "embedded %s: ok (sha256 %x)\n", e.name, sum)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func checkCodesign(c config, out io.Writer) error {
	if !c.checkCodesign {
		return nil
	}
	doc, err := os.ReadFile(c.drDoc)
	if err != nil {
		return err
	}
	want, err := codesign.Recorded(string(doc))
	if err != nil {
		return fmt.Errorf("%s: %w", c.drDoc, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := codesign.Designated(ctx, c.codesignBin, c.bin)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("designated requirement differs from the one recorded in %s\n  recorded: %s\n  signed:   %s", c.drDoc, want, got)
	}
	fmt.Fprintln(out, "codesign: ok (designated requirement matches the recorded one)")
	return nil
}
