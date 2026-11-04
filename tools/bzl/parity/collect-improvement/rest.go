package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"gopkg.in/yaml.v3"
)

func decodeFiles(b []byte) ([]string, error) {
	var fs []struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(b, &fs); err != nil {
		return nil, err
	}
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Filename
	}
	return out, nil
}

// MeasureT3 derives the worktree rows from an `s6measure cold` result: T3-second-cold is the second-worktree
// median over the first Bazel cold build (target 0.25); T3-disk is the median output base over the make baseline.
func MeasureT3(d *collect.Doc, s6 collect.S6Result, base collect.WorktreeBaseline, g collect.Gate, src string) {
	first, ok := s6.First()
	walls := s6.SecondWalls()
	if !ok || len(walls) == 0 {
		d.Unmeasured("T3-second-cold", "s6measure result lacks a successful first and second build", collect.Meta{Source: src})
		d.Unmeasured("T3-disk", "s6measure result lacks a successful build", collect.Meta{Source: src})
		return
	}
	rs := s6.Readings(g.Max)
	d.SetTimed("T3-second-cold", collect.Median(walls)/first.WallS, rs, g, src+": second median / first cold")
	var kb []float64
	for _, b := range s6.Builds {
		if b.Exit == 0 && b.OutputBaseKB > 0 {
			kb = append(kb, float64(b.OutputBaseKB))
		}
	}
	if len(kb) == 0 || base.DiskKB.Median <= 0 {
		d.Unmeasured("T3-disk", "no output_base_kb in the result or no make baseline", collect.Meta{Source: src})
		return
	}
	d.Set("T3-disk", collect.Median(kb)/base.DiskKB.Median, collect.Meta{Source: src + ": output_base_kb median / baseline disk_kb_worktree"})
}

// EditRunner performs one edit-build-test cycle on a config/ file.
type EditRunner interface {
	Edit(i int) error
	Test() (time.Duration, error)
	Restore() error
}

// FileEditRunner appends a comment to a non-test Go file and runs a test command; Restore puts the file back.
type FileEditRunner struct {
	Path string
	Orig []byte
	Dir  string
	Argv []string
	Env  []string
}

// Edit implements EditRunner.
func (f *FileEditRunner) Edit(i int) error {
	if f.Orig == nil {
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		f.Orig = b
	}
	return os.WriteFile(f.Path, append(append([]byte(nil), f.Orig...), []byte(fmt.Sprintf("\n// t4 edit %d\n", i))...), 0o644)
}

// Test implements EditRunner.
func (f *FileEditRunner) Test() (time.Duration, error) {
	cmd := exec.Command(f.Argv[0], f.Argv[1:]...) //nolint:norawexec // timed test run, waited on
	cmd.Dir, cmd.Env = f.Dir, append(os.Environ(), f.Env...)
	t0 := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return time.Since(t0), fmt.Errorf("%w: %s", err, tail(string(out)))
	}
	return time.Since(t0), nil
}

// Restore implements EditRunner.
func (f *FileEditRunner) Restore() error {
	if f.Orig == nil {
		return nil
	}
	return os.WriteFile(f.Path, f.Orig, 0o644)
}

func tail(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

// MeasureT4 records the median warm edit-build-test latency. The file is restored on every exit path, and a
// noisy host is refused before the file is touched.
func MeasureT4(d *collect.Doc, r EditRunner, g collect.Gate, runs int, src string) {
	if g.Require && !g.PreOK() {
		d.SetTimed("T4", 0, []collect.Reading{g.End(g.Begin())}, g, src)
		return
	}
	defer func() { _ = r.Restore() }()
	if err := r.Edit(0); err != nil {
		d.Unmeasured("T4", err.Error(), collect.Meta{Source: src})
		return
	}
	if _, err := r.Test(); err != nil { // warm the caches on the first edit
		d.Unmeasured("T4", "warm-up failed: "+err.Error(), collect.Meta{Source: src})
		return
	}
	i := 0
	vals, rs, err := collect.Sample(g, runs, func() (float64, error) {
		i++
		if err := r.Edit(i); err != nil {
			return 0, err
		}
		dur, err := r.Test()
		return dur.Seconds(), err
	})
	if err != nil && err != collect.ErrNotQuiet {
		d.Unmeasured("T4", err.Error(), collect.Meta{Source: src})
		return
	}
	d.SetTimed("T4", collect.Median(vals), rs, g, src)
}

var (
	regenCmd   = regexp.MustCompile(`make[^&;|]* (proto-gen|ent-gen|sync)($|[ &;|])`)
	buildCmd   = regexp.MustCompile(`make[^&;|]* (build|test|lint|ci|ready)`)
	regenError = regexp.MustCompile(`gen/[^"]* not found|undefined: [^"]*sessionv1`)
)

// SessionRegen is the regen-step count of one transcript.
type SessionRegen struct {
	Steps       int
	BuildActive bool
}

// ScanTranscript counts manual regen steps (make proto-gen / ent-gen / sync invocations) plus the build errors
// they answer (missing gen/ output) in one transcript, as regen-steps.sh does. The count includes intentional
// runs, so it is an upper bound.
func ScanTranscript(path string) (SessionRegen, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionRegen{}, err
	}
	defer f.Close()
	var s SessionRegen
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	for sc.Scan() {
		line := sc.Bytes()
		isBash := strings.Contains(string(line), `"name":"Bash"`)
		if !isBash && !regenError.Match(line) {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		var blocks []struct {
			Type    string                   `json:"type"`
			Name    string                   `json:"name"`
			Input   struct{ Command string } `json:"input"`
			Content json.RawMessage          `json:"content"`
		}
		if json.Unmarshal(ev.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch {
			case ev.Type == "assistant" && b.Type == "tool_use" && b.Name == "Bash":
				if regenCmd.MatchString(b.Input.Command) {
					s.Steps++
				}
				s.BuildActive = s.BuildActive || buildCmd.MatchString(b.Input.Command)
			case ev.Type == "user" && b.Type == "tool_result" && regenError.Match(b.Content):
				s.Steps++
			}
		}
	}
	return s, sc.Err()
}

// MeasureT6 counts regen steps over the newest n transcripts modified on or after since. Before the flip (no
// since date) the row is UNMEASURED and reported only. The baseline median is 0, so target 0.10 x 0 = 0 and the
// row cannot detect improvement; the result is flagged degenerate in the metadata, not hidden.
func MeasureT6(d *collect.Doc, glob string, since time.Time, n int, baselineMedian float64) error {
	if since.IsZero() {
		d.Unmeasured("T6", "UNMEASURED before the flip: pass -since <flip date> to count post-flip transcripts", collect.Meta{})
		return nil
	}
	files, err := filepath.Glob(glob)
	if err != nil {
		return err
	}
	type tf struct {
		p string
		t time.Time
	}
	var fs []tf
	for _, p := range files {
		if st, err := os.Stat(p); err == nil && !st.ModTime().Before(since) {
			fs = append(fs, tf{p, st.ModTime()})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].t.After(fs[j].t) })
	if len(fs) > n {
		fs = fs[:n]
	}
	if len(fs) == 0 {
		d.Unmeasured("T6", "no transcripts on or after "+since.Format("2006-01-02"), collect.Meta{})
		return nil
	}
	var all, active []float64
	for _, f := range fs {
		s, err := ScanTranscript(f.p)
		if err != nil {
			return err
		}
		all = append(all, float64(s.Steps))
		if s.BuildActive {
			active = append(active, float64(s.Steps))
		}
	}
	src := fmt.Sprintf("regen-step scan of %d transcripts since %s (upper bound: counts intentional runs)", len(all), since.Format("2006-01-02"))
	if baselineMedian == 0 {
		src += "; DEGENERATE: baseline median is 0, so the 0.10 x baseline target is 0 and cannot detect improvement"
	}
	d.Set("T6", collect.Median(all), collect.Meta{Source: src})
	d.Set("T6_p90", collect.Percentile(all, 0.9), collect.Meta{})
	d.Set("T6_n", float64(len(all)), collect.Meta{})
	if len(active) > 0 {
		d.Set("T6_build_active_median", collect.Median(active), collect.Meta{})
	}
	return nil
}

// Hermeticity is the output of `hermeticity.sh --system make|bazel`.
type Hermeticity struct {
	H1Count     int      `json:"h1_host_executables"`
	H1List      []string `json:"h1_host_executable_list"`
	H1Network   int      `json:"h1_network_destinations"`
	H2OK        bool     `json:"h2_path_scrubbed_build_ok"`
	H3          any      `json:"h3"`
	H4          int      `json:"h4_downgrades"`
	LoadAvg1    *float64 `json:"loadavg1"`
	LoadAvg1End *float64 `json:"loadavg1_end"`
}

func loadHermeticity(path string) (Hermeticity, error) {
	var h Hermeticity
	b, err := os.ReadFile(path)
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(b, &h)
}

func (h Hermeticity) hostExecs(allow map[string]bool) int {
	if len(h.H1List) == 0 {
		return h.H1Count
	}
	n := 0
	for _, e := range h.H1List {
		if !allow[e] {
			n++
		}
	}
	return n
}

// MeasureT7 folds the T7 clauses into one number the evaluator can compare with its "at most" target: the H1
// host-executable ratio (Bazel over make, allow-listed prerequisites excluded) plus 1 for each violated absolute
// clause (network destinations != 0, H2 false, H3 != 0, H4 != accepted downgrades), so any violation exceeds 0.50.
// The Bazel result must record a quiet host (loadavg1 <= 2.0 before and after the build).
func MeasureT7(d *collect.Doc, bazel, makeBase Hermeticity, allow map[string]bool, accepted int, g collect.Gate, src string) {
	var rs []collect.Reading
	if bazel.LoadAvg1 != nil {
		r := collect.Reading{Pre: *bazel.LoadAvg1, Post: *bazel.LoadAvg1}
		if bazel.LoadAvg1End != nil {
			r.Post = *bazel.LoadAvg1End
		}
		r.Quiet = r.Pre <= g.Max && r.Post <= g.Max
		rs = append(rs, r)
	}
	if len(rs) == 0 {
		d.Unmeasured("T7", "NOT QUIET: the Bazel hermeticity result records no loadavg1, so quiet cannot be shown", collect.Meta{Source: src})
		return
	}
	base := makeBase.hostExecs(allow)
	if base == 0 {
		d.Unmeasured("T7", "make baseline has no host executables", collect.Meta{Source: src})
		return
	}
	v := float64(bazel.hostExecs(allow)) / float64(base)
	var violated []string
	if bazel.H1Network != 0 {
		violated = append(violated, "network destinations "+strconv.Itoa(bazel.H1Network)+" != 0")
	}
	if !bazel.H2OK {
		violated = append(violated, "H2 false")
	}
	if h3, ok := bazel.H3.(float64); !ok || h3 != 0 {
		violated = append(violated, fmt.Sprintf("H3 %v != 0", bazel.H3))
	}
	if bazel.H4 != accepted {
		violated = append(violated, fmt.Sprintf("H4 %d != accepted %d", bazel.H4, accepted))
	}
	v += float64(len(violated))
	s := src + ": H1 ratio + 1 per violated clause"
	if len(violated) > 0 {
		s += " [" + strings.Join(violated, "; ") + "]"
	}
	d.SetTimed("T7", v, rs, g, s)
}

// AcceptedDowngrades reads `hermetic_downgrades_accepted` (a list) from the owner attestations; absent means 0.
func AcceptedDowngrades(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var doc struct {
		Accepted []any `yaml:"hermetic_downgrades_accepted"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return 0, err
	}
	return len(doc.Accepted), nil
}

func readAllow(path string) (map[string]bool, error) {
	m := map[string]bool{}
	if path == "" {
		return m, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			m[l] = true
		}
	}
	return m, nil
}

var e2eLine = regexp.MustCompile(`ratio=([0-9.]+) e2e_escapes=([0-9]+)`)

// ParseE2EAvoided reads the output of tools/bzl/e2eavoided: status from the `T8:` line, and the ratio and the
// escape count from the summary line.
func ParseE2EAvoided(out string) (status string, ratio float64, escapes int, ok bool) {
	for _, l := range strings.Split(out, "\n") {
		if s, found := strings.CutPrefix(l, "T8: "); found {
			status = strings.TrimSpace(s)
		}
		if m := e2eLine.FindStringSubmatch(l); m != nil {
			ratio, _ = strconv.ParseFloat(m[1], 64)
			escapes, _ = strconv.Atoi(m[2])
			ok = true
		}
	}
	return status, ratio, escapes, ok && status != ""
}

// MeasureT8 wires the e2eavoided tool into the measurements JSON. The value is the selective/baseline ratio plus
// one per e2e escape (so any escape exceeds the 0.50 target; e2e_escapes must be 0, not waivable).
func MeasureT8(d *collect.Doc, out string) {
	status, ratio, escapes, ok := ParseE2EAvoided(out)
	switch {
	case !ok:
		d.Unmeasured("T8", "could not parse e2eavoided output", collect.Meta{Source: "e2eavoided"})
	case status == "UNMEASURED":
		d.Unmeasured("T8", "e2eavoided: no sampled PR has both a baseline full run and a selective shadow run", collect.Meta{Source: "e2eavoided"})
	default:
		d.Set("T8", ratio+float64(escapes), collect.Meta{Source: fmt.Sprintf("e2eavoided: ratio %.3f + %d escapes", ratio, escapes)})
	}
}
