package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run exits 0 on a completed replay (misses are reported, not an error), 2 when an input cannot be read.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare-affected", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "repository root (the checkout the sampled paths are replayed against)")
	sample := fs.String("sample", "tools/bzl/baseline/pr-sample.json", "PR sample from tools/bzl/baseline/pr-sample.sh")
	set := fs.String("set", "partial", "sample list to replay: partial or all")
	affected := fs.String("affected-bin", "", "built tools/ci/affected-targets binary (required)")
	nonBazel := fs.String("non-bazel-inputs", "", "non-Bazel inputs file handed to affected-targets (default: the checked-in one)")
	limit := fs.Int("limit", 0, "replay only the first N PRs (0 = all)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *affected == "" {
		fmt.Fprintln(stderr, "compare-affected: -affected-bin is required")
		return 2
	}
	rp, prs, err := setup(*root, *sample, *set, *affected, *nonBazel, *limit)
	if err != nil {
		fmt.Fprintln(stderr, "compare-affected:", err)
		return 2
	}
	sum, err := rp.Replay(prs)
	if err != nil {
		fmt.Fprintln(stderr, "compare-affected:", err)
		return 2
	}
	sum.Write(stdout)
	return 0
}

func setup(root, sample, set, affectedBin, nonBazel string, limit int) (Replayer, []PR, error) {
	var rp Replayer
	build, err := os.ReadFile(filepath.Join(root, ".github/workflows/build.yml"))
	if err != nil {
		return rp, nil, err
	}
	if rp.Regex, err = RegexFromWorkflow(string(build)); err != nil {
		return rp, nil, fmt.Errorf("build.yml: %w", err)
	}
	mcp, err := os.ReadFile(filepath.Join(root, ".github/workflows/mcp-integration.yml"))
	if err != nil {
		return rp, nil, err
	}
	if rp.Patterns, err = PathsFromWorkflow(mcp); err != nil {
		return rp, nil, fmt.Errorf("mcp-integration.yml: %w", err)
	}
	raw, err := os.ReadFile(sample)
	if err != nil {
		return rp, nil, err
	}
	prs, err := LoadSample(raw, set)
	if err != nil {
		return rp, nil, err
	}
	if limit > 0 && limit < len(prs) {
		prs = prs[:limit]
	}
	rp.Affected = func(files []string) (Selection, error) { return runAffected(root, affectedBin, nonBazel, files) }
	rp.Script = func(files []string) (bool, error) { return runScript(root, files) }
	return rp, prs, nil
}

// LoadSample reads the `partial` or `all` list of pr-sample.json.
func LoadSample(raw []byte, set string) ([]PR, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	list, ok := doc[set]
	if !ok {
		return nil, fmt.Errorf("sample has no %q list", set)
	}
	var entries []struct {
		Number int      `json:"number"`
		Paths  []string `json:"paths"`
	}
	if err := json.Unmarshal(list, &entries); err != nil {
		return nil, err
	}
	out := make([]PR, len(entries))
	for i, e := range entries {
		out[i] = PR{Number: e.Number, Files: e.Paths}
	}
	return out, nil
}

// runAffected feeds the PR's paths as modified files; the sample carries no per-file status, so a deleted or
// renamed-away path is replayed as modified (and widens to //... only if it is no longer in the graph).
func runAffected(root, bin, nonBazel string, files []string) (Selection, error) {
	var in strings.Builder
	for _, f := range files {
		in.WriteString("M\t" + f + "\n")
	}
	args := []string{"-root", root, "-name-status", "-"}
	if nonBazel != "" {
		args = append(args, "-non-bazel-inputs", nonBazel)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(in.String())
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return Selection{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	return ParseSelection(&out)
}

// runScript runs detect-feature-changes.sh with the PR's files; exit 0 means a feature change was detected and
// exit 1 none. It reads marker lines from the working tree, so a path deleted since the PR merged cannot match.
func runScript(root string, files []string) (bool, error) {
	cmd := exec.Command("bash", "tools/ci/detect-feature-changes.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CHANGED_FILES="+strings.Join(files, "\n"), "GITHUB_ENV=")
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
