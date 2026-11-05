// Command e2emutate is the S14 mutation backtest (Task 0.15d): it seeds behavioural mutations in files the
// captured specs exercise, runs the selected specs plus a seeded sample of the rest against each mutated server,
// and reports every spec that fails without having been selected (a miss) per selection granularity.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Mutation is one seeded edit: replace the single line `Line` of `File` so its `if` condition is negated.
type Mutation struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // go | web
	File    string `json:"file"`
	Line    int    `json:"line"`
	Package string `json:"package"`
	Handler bool   `json:"rpc_handler,omitempty"`
	Before  string `json:"before"`
	After   string `json:"after"`
}

var goIf = regexp.MustCompile(`^(\s*)if ([^;{:]+?) \{$`)
var webIf = regexp.MustCompile(`^(\s*)if \(([^;{]+)\) \{$`)

// negate returns the mutated line, or "" when the line is not a plain single-line `if` for kind.
func negate(kind, line string) string {
	line = strings.TrimRight(line, " \t\r")
	if kind == "go" {
		if m := goIf.FindStringSubmatch(line); m != nil && !strings.Contains(m[2], "else") {
			return m[1] + "if !(" + m[2] + ") {"
		}
		return ""
	}
	if m := webIf.FindStringSubmatch(line); m != nil {
		return m[1] + "if (!(" + m[2] + ")) {"
	}
	return ""
}

type cap struct {
	Spec     string            `json:"spec"`
	GoFiles  []string          `json:"go_files"`
	GoLines  map[string][]line `json:"go_lines"`
	WebFiles []string          `json:"web_files"`
}
type line [2]int

func loadCaps(dir string) ([]cap, error) {
	fs, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []cap
	for _, f := range fs {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c cap
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func inRanges(n int, rs []line) bool {
	for _, r := range rs {
		if n >= r[0] && n <= r[1] {
			return true
		}
	}
	return false
}

// candidates lists negatable lines per file, executed ones only for Go, with how many specs run the file.
func candidates(root string, caps []cap) (goC, webC map[string][]Mutation, specsOf map[string]int) {
	goC, webC, specsOf = map[string][]Mutation{}, map[string][]Mutation{}, map[string]int{}
	executed := map[string][]line{}
	for _, c := range caps {
		for _, f := range c.GoFiles {
			specsOf[f]++
			executed[f] = append(executed[f], c.GoLines[f]...)
		}
		for _, f := range c.WebFiles {
			specsOf[f]++
		}
	}
	scan := func(f, kind string, ok func(int) bool, into map[string][]Mutation) {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return
		}
		for i, l := range strings.Split(string(b), "\n") {
			if a := negate(kind, l); a != "" && ok(i+1) {
				into[f] = append(into[f], Mutation{Kind: kind, File: f, Line: i + 1, Package: path.Dir(f), Before: l, After: a})
			}
		}
	}
	for f, rs := range executed {
		if strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "gen/") || strings.HasPrefix(f, "session/ent/") || strings.HasPrefix(f, "tools/") {
			continue
		}
		scan(f, "go", func(n int) bool { return inRanges(n, rs) }, goC)
	}
	for f := range specsOf {
		if strings.HasPrefix(f, "web-app/src/") && !strings.Contains(f, ".test.") && !strings.HasSuffix(f, ".css.ts") {
			scan(f, "web", func(int) bool { return true }, webC)
		}
	}
	return
}

// pick selects n mutations over distinct keys (package for Go, file for web), preferring files that few specs
// run (small selections make informative backtests); rng makes the choice reproducible.
func pick(cands map[string][]Mutation, specsOf map[string]int, key func(Mutation) string, n int, min, max int, rng *rand.Rand, used map[string]bool) []Mutation {
	var files []string
	for f, ms := range cands {
		if len(ms) > 0 && specsOf[f] >= min && specsOf[f] <= max {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	rng.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
	var out []Mutation
	for _, f := range files {
		ms := cands[f]
		m := ms[rng.Intn(len(ms))]
		if used[key(m)] {
			continue
		}
		used[key(m)] = true
		out = append(out, m)
		if len(out) == n {
			break
		}
	}
	return out
}

func gen(root, capsDir, out string, seed int64) error {
	caps, err := loadCaps(capsDir)
	if err != nil {
		return err
	}
	goC, webC, specsOf := candidates(root, caps)
	rng := rand.New(rand.NewSource(seed))
	handlers, others := map[string][]Mutation{}, map[string][]Mutation{}
	for f, ms := range goC {
		if strings.HasPrefix(f, "server/services/") {
			handlers[f] = ms
		} else {
			others[f] = ms
		}
	}
	used := map[string]bool{}
	byFile := func(m Mutation) string { return m.File }
	byPkg := func(m Mutation) string { return m.Package }
	var all []Mutation
	h := pick(handlers, specsOf, byFile, 6, 2, 60, rng, used)
	for i := range h {
		h[i].Handler = true
	}
	all = append(all, h...)
	all = append(all, pick(others, specsOf, byPkg, 14, 2, 60, rng, map[string]bool{})...)
	all = append(all, pick(webC, specsOf, byFile, 10, 2, 60, rng, map[string]bool{})...)
	for i := range all {
		all[i].ID = fmt.Sprintf("m%02d", i+1)
	}
	b, _ := json.MarshalIndent(all, "", "  ")
	return os.WriteFile(out, append(b, '\n'), 0o644)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: e2emutate gen|run [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	caps := fs.String("caps", "", "per-spec capture directory")
	mfile := fs.String("mutations", "", "mutations JSON")
	scratch := fs.String("scratch", "", "scratch dir on disk (never /tmp)")
	progress := fs.String("progress", "", "progress.json from e2ebatch (baseline outcomes)")
	e2eslice := fs.String("e2eslice", "", "e2eslice binary")
	baseDist := fs.String("base-dist", "", "baseline web dist directory")
	results := fs.String("results", "", "results JSON (resume-able)")
	seed := fs.Int64("seed", 14, "gen: RNG seed")
	n := fs.Int("count", 30, "gen: mutation count (informational)")
	maxLine := fs.Int("max-file-extra", 4, "run: cap on file-selected-but-not-line-selected specs run per mutation")
	lineRun := fs.Int("line-run", 6, "run: cap on line-selected specs run per mutation (they only show the mutation bites)")
	sample := fs.Int("sample", 5, "run: seeded random specs outside the file-selected set")
	only := fs.String("only", "", "run: comma-separated mutation ids")
	_ = n
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "gen":
		err = gen(*root, *caps, *mfile, *seed)
	case "run":
		err = run(runCfg{root: *root, caps: *caps, mfile: *mfile, scratch: *scratch, progress: *progress, e2eslice: *e2eslice,
			baseDist: *baseDist, results: *results, maxFileSet: *maxLine, lineRun: *lineRun, sample: *sample, only: *only})
	default:
		err = fmt.Errorf("unknown command %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2emutate:", err)
		os.Exit(1)
	}
}

type runCfg struct {
	root, caps, mfile, scratch, progress, e2eslice, baseDist, results, only string
	maxFileSet, sample, lineRun                                             int
}

// Outcome is one spec's result on a mutated server.
type Outcome struct {
	Spec      string `json:"spec"`
	Exit      int    `json:"exit"`
	Rerun     *int   `json:"rerun_exit,omitempty"`
	BaseRerun *int   `json:"unmutated_rerun_exit,omitempty"`
}

// Result is one mutation's backtest record.
type Result struct {
	Mutation  Mutation            `json:"mutation"`
	Selected  map[string]int      `json:"selected_counts"`
	Ran       []Outcome           `json:"ran"`
	Failing   []string            `json:"failing_specs"`
	Misses    map[string][]string `json:"misses"` // granularity -> failing specs it did not select
	Note      string              `json:"note,omitempty"`
	LoadAtRun float64             `json:"load_at_run"`
}

func sh(dir string, env []string, name string, args ...string) ([]byte, error) {
	//nolint:norawexec short-lived run-to-completion spike driver (own timeout/kill)
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	return c.CombinedOutput()
}

func selectSpecs(cfg runCfg, gran string, m Mutation) ([]string, error) {
	out, err := sh(cfg.root, nil, cfg.e2eslice, "select-synth", "-root", ".", "-caps", cfg.caps, "-granularity", gran,
		fmt.Sprintf("%s:%d-%d", m.File, m.Line, m.Line))
	if err != nil {
		return nil, fmt.Errorf("select-synth: %v: %s", err, out)
	}
	var r struct{ Specs []string }
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}
	return r.Specs, nil
}

func apply(root string, m Mutation, mutated bool) error {
	p := filepath.Join(root, m.File)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	ls := strings.Split(string(b), "\n")
	want, to := m.Before, m.After
	if !mutated {
		want, to = m.After, m.Before
	}
	if strings.TrimRight(ls[m.Line-1], "\r") != strings.TrimRight(want, "\r") {
		return fmt.Errorf("%s:%d does not hold the expected text", m.File, m.Line)
	}
	ls[m.Line-1] = to
	return os.WriteFile(p, []byte(strings.Join(ls, "\n")), 0o644)
}

// waitGate pauses while the host is over the agreed load (24) or memory (12 GB) limits.
func waitGate() {
	for {
		load := readLoad()
		mem := 0.0
		if b, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				var kb float64
				if n, _ := fmt.Sscanf(l, "MemAvailable: %f kB", &kb); n == 1 {
					mem = kb / 1048576
				}
			}
		}
		if load <= 24 && mem >= 12 {
			return
		}
		time.Sleep(20 * time.Second)
	}
}

// reapTmux kills tmux servers left behind by --tmux-keep-server whose environment names scratch (so only this
// run's private ssq-e2e-* sockets, never the default socket or another run's).
func reapTmux(scratch string) {
	ents, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, c := range ents {
		cmd, err := os.ReadFile(c)
		if err != nil || !strings.Contains(string(cmd), "tmux\x00-L\x00ssq-e2e-") && !strings.Contains(strings.ReplaceAll(string(cmd), "\x00", " "), "tmux -L ssq-e2e-") {
			continue
		}
		env, err := os.ReadFile(filepath.Join(filepath.Dir(c), "environ"))
		if err != nil || !strings.Contains(string(env), scratch) {
			continue
		}
		var pid int
		fmt.Sscanf(filepath.Base(filepath.Dir(c)), "%d", &pid)
		if p, err := os.FindProcess(pid); err == nil && pid > 1 {
			_ = p.Kill()
		}
	}
}

func runSpec(cfg runCfg, bin, spec string, timeout time.Duration) int {
	waitGate()
	defer reapTmux(cfg.scratch)
	dir, _ := os.MkdirTemp(cfg.scratch, "mut-srv-")
	defer os.RemoveAll(dir)
	abs, _ := filepath.Abs(bin)
	//nolint:norawexec short-lived run-to-completion spike driver (own timeout/kill)
	c := exec.Command("nice", "-n", "19", "ionice", "-c3", "bunx", "playwright", "test", path.Base(spec), "--workers=1", "--project=chromium", "--reporter=line")
	c.Dir = filepath.Join(cfg.root, "tests/e2e")
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "npm_config_user_agent=") {
			env = append(env, e)
		}
	}
	c.Env = append(env, "TEST_SERVER_BINARY="+abs, "TEST_SERVER_DIR="+dir, "E2E_WORKERS=1", "TMPDIR="+cfg.scratch)
	if err := c.Start(); err != nil {
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	case <-time.After(timeout):
		_ = c.Process.Kill()
		<-done
		return 124
	}
}

func baselineGood(progressPath string) (map[string]bool, error) {
	b, err := os.ReadFile(progressPath)
	if err != nil {
		return nil, err
	}
	var p struct {
		Specs map[string]struct{ State string } `json:"specs"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	good := map[string]bool{}
	for s, r := range p.Specs {
		if r.State == "done" {
			good[s] = true
		}
	}
	return good, nil
}

func readLoad() float64 {
	b, _ := os.ReadFile("/proc/loadavg")
	var l float64
	fmt.Sscanf(string(b), "%f", &l)
	return l
}

func buildServer(cfg runCfg, m Mutation, out string) error {
	waitGate()
	distDir := filepath.Join(cfg.root, "server/web/dist")
	if m.Kind == "web" {
		tmp := filepath.Join(cfg.scratch, "dist-"+m.ID)
		defer os.RemoveAll(tmp)
		if o, err := sh(filepath.Join(cfg.root, "web-app"), []string{"WEB_MINIFY=0", "TMPDIR=" + cfg.scratch}, "nice", "-n", "19", "bun", "build/build.ts", "--out", tmp, "--stats", filepath.Join(tmp+".stats.json")); err != nil {
			return fmt.Errorf("web build: %v: %s", err, o)
		}
		_ = os.RemoveAll(distDir)
		if o, err := sh(".", nil, "cp", "-r", tmp, distDir); err != nil {
			return fmt.Errorf("cp dist: %v: %s", err, o)
		}
		defer func() {
			_ = os.RemoveAll(distDir)
			_, _ = sh(".", nil, "cp", "-r", cfg.baseDist, distDir)
		}()
	}
	o, err := sh(cfg.root, []string{"CGO_ENABLED=1", "TMPDIR=" + cfg.scratch, "GOTMPDIR=" + filepath.Join(cfg.scratch, "..", "go")},
		"nice", "-n", "19", "go", "build", "-p", "4", "-o", out, ".")
	if err != nil {
		return fmt.Errorf("go build: %v: %s", err, o)
	}
	return nil
}

func run(cfg runCfg) error {
	b, err := os.ReadFile(cfg.mfile)
	if err != nil {
		return err
	}
	var muts []Mutation
	if err := json.Unmarshal(b, &muts); err != nil {
		return err
	}
	good, err := baselineGood(cfg.progress)
	if err != nil {
		return err
	}
	var specs []string
	for s := range good {
		specs = append(specs, s)
	}
	sort.Strings(specs)
	done := map[string]Result{}
	var all []Result
	if rb, err := os.ReadFile(cfg.results); err == nil {
		_ = json.Unmarshal(rb, &all)
		for _, r := range all {
			done[r.Mutation.ID] = r
		}
	}
	onlySet := map[string]bool{}
	for _, o := range strings.Split(cfg.only, ",") {
		if o != "" {
			onlySet[o] = true
		}
	}
	baseBin := filepath.Join(cfg.scratch, "..", "bin", "stapler-squad-nocover")
	for _, m := range muts {
		if _, ok := done[m.ID]; ok || (len(onlySet) > 0 && !onlySet[m.ID]) {
			continue
		}
		res := Result{Mutation: m, Selected: map[string]int{}, Misses: map[string][]string{}}
		sel := map[string]map[string]bool{}
		for _, g := range []string{"package", "file", "line"} {
			ss, err := selectSpecs(cfg, g, m)
			if err != nil {
				return err
			}
			sel[g] = map[string]bool{}
			for _, s := range ss {
				sel[g][s] = true
			}
			res.Selected[g] = len(ss)
		}
		rng := rand.New(rand.NewSource(int64(len(m.ID))*1000 + int64(m.Line)))
		var fileSel, rest, lineSel []string
		for _, s := range specs {
			switch {
			case sel["line"][s]:
				lineSel = append(lineSel, s)
			case sel["file"][s]:
				fileSel = append(fileSel, s)
			default:
				rest = append(rest, s)
			}
		}
		rng.Shuffle(len(fileSel), func(i, j int) { fileSel[i], fileSel[j] = fileSel[j], fileSel[i] })
		rng.Shuffle(len(lineSel), func(i, j int) { lineSel[i], lineSel[j] = lineSel[j], lineSel[i] })
		if len(lineSel) > cfg.lineRun { // selected specs only show the mutation bites; misses live in the rest
			lineSel = lineSel[:cfg.lineRun]
		}
		if len(fileSel) > cfg.maxFileSet {
			fileSel = fileSel[:cfg.maxFileSet]
		}
		rng.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
		if len(rest) > cfg.sample {
			rest = rest[:cfg.sample]
		}
		runSet := append(append(append([]string{}, lineSel...), fileSel...), rest...)

		if err := apply(cfg.root, m, true); err != nil {
			return err
		}
		bin := filepath.Join(cfg.scratch, "mut-"+m.ID)
		berr := buildServer(cfg, m, bin)
		if rerr := apply(cfg.root, m, false); rerr != nil {
			return rerr
		}
		if berr != nil {
			res.Note = "build failed: " + berr.Error()
			all = append(all, res)
			save(cfg.results, all)
			continue
		}
		res.LoadAtRun = readLoad()
		for _, s := range runSet {
			o := Outcome{Spec: s, Exit: runSpec(cfg, bin, s, 15*time.Minute)}
			if o.Exit != 0 { // rerun once alone on the mutated server, then once on the unmutated one
				r := runSpec(cfg, bin, s, 15*time.Minute)
				o.Rerun = &r
				if r != 0 {
					u := runSpec(cfg, baseBin, s, 15*time.Minute)
					o.BaseRerun = &u
				}
			}
			res.Ran = append(res.Ran, o)
			if o.Exit != 0 && o.Rerun != nil && *o.Rerun != 0 && (o.BaseRerun == nil || *o.BaseRerun == 0) {
				res.Failing = append(res.Failing, s)
			}
		}
		for _, g := range []string{"package", "file", "line"} {
			for _, f := range res.Failing {
				if !sel[g][f] {
					res.Misses[g] = append(res.Misses[g], f)
				}
			}
		}
		_ = os.Remove(bin)
		all = append(all, res)
		save(cfg.results, all)
	}
	return nil
}

func save(p string, rs []Result) {
	b, _ := json.MarshalIndent(rs, "", "  ")
	_ = os.WriteFile(p, append(b, '\n'), 0o644)
}
