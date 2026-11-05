// Command e2ecapture records which Go packages one Playwright spec file executed in the server, using a
// `go build -cover -coverpkg=./...` binary (Task 12.1a). One invocation = one spec = one isolated server, so
// the GOCOVERDIR data is attributable to that spec without any change to the Go source.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const modulePath = "github.com/tstapler/stapler-squad"

// Capture is the per-run output; e2eslice merge unions several of them into the manifest.
type Capture struct {
	Spec     string   `json:"spec"`
	GoPkgs   []string `json:"go_pkgs"`
	WebFiles []string `json:"web_files"`
	Routes   []string `json:"routes"`
	RPCs     []string `json:"rpcs"`
	// GoFiles are repo files with at least one executed statement; GoLines their executed line ranges.
	GoFiles []string `json:"go_files,omitempty"`
	// PlaywrightExit is the test runner's exit status (0 = all passed); WallSeconds the invocation wall time.
	PlaywrightExit int               `json:"playwright_exit"`
	WallSeconds    float64           `json:"wall_seconds"`
	GoLines        map[string][]Line `json:"go_lines,omitempty"`
	// GoAll is every instrumented statement-block range per file (identical across specs of one binary).
	GoAll    map[string][]Line `json:"go_all,omitempty"`
	Excluded string            `json:"excluded,omitempty"`
}

var percentLine = regexp.MustCompile(`^\s*(\S+)\s+coverage:\s+([0-9.]+)% of statements`)

// executedPackages parses `go tool covdata percent` output. With -coverpkg=./... every linked package is
// instrumented, so `pkglist` would list all of them; a package counts only if a statement ran (percent > 0).
func executedPackages(percentOut string) []string {
	var pkgs []string
	sc := bufio.NewScanner(strings.NewReader(percentOut))
	for sc.Scan() {
		m := percentLine.FindStringSubmatch(sc.Text())
		if m == nil || !strings.HasPrefix(m[1], modulePath) {
			continue
		}
		if p, err := strconv.ParseFloat(m[2], 64); err == nil && p > 0 {
			pkgs = append(pkgs, m[1])
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

func main() {
	spec := flag.String("spec", "", "spec path relative to the repo root (tests/e2e/x.spec.ts)")
	bin := flag.String("binary", "", "cover-instrumented server binary")
	root := flag.String("root", ".", "repository root")
	tmp := flag.String("tmp", "", "scratch directory (never /tmp; defaults to $TMPDIR)")
	dist := flag.String("dist", "server/web/dist", "web dist directory (source maps for web-file capture), relative to -root")
	project := flag.String("project", "chromium", "Playwright project")
	out := flag.String("out", "", "capture JSON path (default stdout)")
	flag.Parse()
	if *spec == "" || *bin == "" {
		fmt.Fprintln(os.Stderr, "usage: e2ecapture -spec tests/e2e/x.spec.ts -binary BIN [-out FILE]")
		os.Exit(2)
	}
	scratch := *tmp
	if scratch == "" {
		scratch = os.Getenv("TMPDIR")
	}
	if scratch == "" || strings.HasPrefix(scratch, "/tmp") || strings.HasPrefix(scratch, "/dev/shm") {
		fmt.Fprintln(os.Stderr, "e2ecapture: refusing to use /tmp or /dev/shm; set -tmp or TMPDIR to a disk path")
		os.Exit(2)
	}
	covDir, err := os.MkdirTemp(scratch, "cov-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(covDir)
	testDir, err := os.MkdirTemp(scratch, "srv-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(testDir)

	webDir, err := os.MkdirTemp(scratch, "web-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(webDir)
	e2eAbs, _ := filepath.Abs(filepath.Join(*root, "tests/e2e"))
	preload := filepath.Join(e2eAbs, "helpers/web-coverage-preload.js")
	abs, _ := filepath.Abs(*bin)
	cmd := exec.Command("bunx", "playwright", "test", filepath.Base(*spec), "--workers=1", "--project="+*project, "--reporter=line")
	cmd.Dir = filepath.Join(*root, "tests/e2e")
	cmd.Env = append(filterEnv(os.Environ(), "npm_config_user_agent"),
		"TEST_SERVER_BINARY="+abs, "GOCOVERDIR="+covDir, "TEST_SERVER_DIR="+testDir, "E2E_WORKERS=1", "TMPDIR="+scratch,
		"WEBCOV_DIR="+webDir, "WEBCOV_RESOLVE_FROM="+e2eAbs, "NODE_OPTIONS=--require="+preload)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	started := time.Now()
	runErr := cmd.Run()
	wall := time.Since(started).Seconds() // a failing spec still exercised the server; the coverage data is what matters

	pct, err := exec.Command("go", "tool", "covdata", "percent", "-i="+covDir).Output()
	if err != nil {
		fatal(fmt.Errorf("covdata percent (no coverage written; did the server exit cleanly? playwright: %v): %w", runErr, err))
	}
	txt := filepath.Join(covDir, "cov.txt")
	if err := exec.Command("go", "tool", "covdata", "textfmt", "-i="+covDir, "-o="+txt).Run(); err != nil {
		fatal(fmt.Errorf("covdata textfmt: %w", err))
	}
	tb, _ := os.ReadFile(txt)
	lines, allBlocks := parseTextfmtAll(string(tb))
	var goFiles []string
	for f := range lines {
		goFiles = append(goFiles, f)
	}
	sort.Strings(goFiles)
	webFiles, paths, err := webCapture(webDir, filepath.Join(*root, *dist))
	if err != nil {
		fatal(err)
	}
	exit := 0
	if runErr != nil {
		exit = 1
		if ee, ok := runErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		}
	}
	c := Capture{Spec: *spec, GoPkgs: withFileDirs(executedPackages(string(pct)), goFiles), WebFiles: webFiles, Routes: paths, RPCs: rpcsOf(paths),
		GoFiles: goFiles, GoLines: lines, GoAll: allBlocks, PlaywrightExit: exit, WallSeconds: wall}
	if c.WebFiles == nil {
		c.WebFiles = []string{}
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if *out == "" {
		fmt.Println(string(b))
		return
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		fatal(err)
	}
}

func filterEnv(env []string, drop string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, drop+"=") {
			out = append(out, e)
		}
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "e2ecapture:", err)
	os.Exit(1)
}
